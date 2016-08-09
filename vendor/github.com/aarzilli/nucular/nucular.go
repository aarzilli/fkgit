package nucular

import (
	"errors"
	"image"
	"image/color"
	"math"
	"strconv"
	"sync/atomic"

	"github.com/aarzilli/nucular/command"
	"github.com/aarzilli/nucular/label"
	nstyle "github.com/aarzilli/nucular/style"
	"github.com/aarzilli/nucular/types"

	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
)

///////////////////////////////////////////////////////////////////////////////////
// CONTEXT & PANELS
///////////////////////////////////////////////////////////////////////////////////

type context struct {
	Input   Input
	Style   nstyle.Style
	Windows []*Window
	Clip    Clipboard
	Scaling float64
	changed int32
}

type UpdateFn func(*MasterWindow, *Window)

type Window struct {
	title        string
	ctx          *context
	idx          int
	flags        WindowFlags
	Bounds       types.Rect
	Scrollbar    image.Point
	cmds         command.Buffer
	widgets      widgetBuffer
	layout       *panel
	close, first bool
	// trigger rectangle of nonblocking windows
	triggerBounds, header types.Rect
	// root of the node tree
	rootNode *treeNode
	// current tree node see TreePush/TreePop
	curNode *treeNode
	// parent window of a popup
	parent *Window
	// helper windows to implement groups
	groupWnd map[string]*Window
	// editor of the active property widget (see PropertyInt, PropertyFloat)
	editor *TextEditor
	// update function
	updateFn UpdateFn
}

type treeNode struct {
	Open     bool
	Children map[string]*treeNode
	Parent   *treeNode
}

type panel struct {
	Flags          WindowFlags
	Bounds         types.Rect
	Offset         *image.Point
	AtX            int
	AtY            int
	MaxX           int
	Width          int
	Height         int
	FooterH        int
	HeaderH        int
	Border         int
	Clip           types.Rect
	Menu           menuState
	Row            rowLayout
	ReservedHeight int
}

type menuState struct {
	X      int
	Y      int
	W      int
	H      int
	Offset image.Point
}

type rowLayout struct {
	Type       int
	Index      int
	Height     int
	Columns    int
	Ratio      []float64
	WidthArr   []int
	ItemWidth  int
	ItemRatio  float64
	ItemHeight int
	ItemOffset int
	Filled     float64
	Item       types.Rect
	TreeDepth  int

	DynamicFreeX, DynamicFreeY, DynamicFreeW, DynamicFreeH float64
}

type WindowFlags int

const (
	WindowBorder WindowFlags = 1 << iota
	WindowBorderHeader
	WindowMovable
	WindowScalable
	WindowClosable
	WindowMinimizable
	WindowDynamic
	WindowNoScrollbar
	WindowNoHScrollbar
	WindowTitle

	windowPrivate
	windowHidden
	windowMinimized
	windowSub
	windowGroup
	windowPopup
	windowNonblock
	windowContextual
	windowCombo
	windowMenu
	windowTooltip

	WindowDefaultFlags = WindowBorder | WindowMovable | WindowScalable | WindowClosable | WindowMinimizable | WindowTitle
)

func contextAllCommands(ctx *context) (nwidgets int, r []command.Command) {
	for _, w := range ctx.Windows {
		r = append(r, w.cmds.Commands...)
	}
	nwidgets = 1
	return
}

func createTreeNode(initialState bool, parent *treeNode) *treeNode {
	return &treeNode{initialState, map[string]*treeNode{}, parent}
}

func createWindow(ctx *context, title string) *Window {
	rootNode := createTreeNode(false, nil)
	r := &Window{ctx: ctx, title: title, rootNode: rootNode, curNode: rootNode, groupWnd: map[string]*Window{}, first: true}
	r.widgets.win = r
	return r
}

func contextBegin(ctx *context, layout *panel) {
	for _, w := range ctx.Windows {
		w.curNode = w.rootNode
		w.close = false
		w.widgets.reset()
		w.cmds.Reset()
	}

	ctx.Windows[0].layout = layout
	panelBegin(ctx, ctx.Windows[0], "")
	layout.Offset = &ctx.Windows[0].Scrollbar
}

func contextEnd(ctx *context) {
	panelEnd(ctx, ctx.Windows[0])
}

func (win *Window) style() *nstyle.Window {
	switch {
	case win.flags&windowCombo != 0:
		return &win.ctx.Style.ComboWindow
	case win.flags&windowContextual != 0:
		return &win.ctx.Style.ContextualWindow
	case win.flags&windowMenu != 0:
		return &win.ctx.Style.MenuWindow
	case win.flags&windowGroup != 0:
		return &win.ctx.Style.GroupWindow
	case win.flags&windowTooltip != 0:
		return &win.ctx.Style.TooltipWindow
	default:
		return &win.ctx.Style.NormalWindow
	}
}

func panelBegin(ctx *context, win *Window, title string) bool {
	style := &ctx.Style
	font := style.Font
	in := &ctx.Input
	layout := win.layout
	wstyle := win.style()

	/* cache style data */
	window_padding := wstyle.Padding
	item_spacing := wstyle.Spacing
	scaler_size := wstyle.ScalerSize

	/* check arguments */
	*layout = panel{}

	if win.flags&windowHidden != 0 {
		return false
	}

	/* window dragging */
	if (win.flags&WindowMovable != 0) && win.toplevel() {
		var move types.Rect
		move.X = win.Bounds.X
		move.Y = win.Bounds.Y
		move.W = win.Bounds.W
		move.H = layout.HeaderH

		if win.idx != 0 {
			move.H = fontHeight(font) + 2.0*wstyle.Header.Padding.Y
			move.H += 2.0 * wstyle.Header.LabelPadding.Y
		} else {
			move.H = window_padding.Y + item_spacing.Y
		}

		incursor := in.Mouse.PrevHoveringRect(move)
		if in.Mouse.Down(mouse.ButtonLeft) && incursor {
			delta := in.Mouse.Delta
			win.Bounds.X = win.Bounds.X + delta.X
			win.Bounds.Y = win.Bounds.Y + delta.Y
		}
	}

	/* panel space with border */
	if win.flags&WindowBorder != 0 {
		layout.Bounds = shrinkRect(win.Bounds, wstyle.Border)
	} else {
		layout.Bounds = win.Bounds
	}

	/* setup panel */
	layout.Border = layout.Bounds.X - win.Bounds.X

	layout.AtX = layout.Bounds.X
	layout.AtY = layout.Bounds.Y
	layout.Width = layout.Bounds.W
	layout.Height = layout.Bounds.H
	layout.MaxX = 0
	layout.Row.Index = 0
	layout.Row.Columns = 0
	layout.Row.Height = 0
	layout.Row.Ratio = nil
	layout.Row.ItemWidth = 0
	layout.Row.ItemRatio = 0
	layout.Row.TreeDepth = 0
	layout.Flags = win.flags
	layout.ReservedHeight = 0

	/* calculate window header */
	if win.flags&windowMinimized != 0 {
		layout.HeaderH = 0
		layout.Row.Height = 0
	} else if win.flags&windowMenu != 0 || win.flags&windowContextual != 0 {
		layout.HeaderH = window_padding.Y
		layout.Row.Height = window_padding.Y
	} else {
		layout.HeaderH = item_spacing.Y + window_padding.Y
		layout.Row.Height = item_spacing.Y + window_padding.Y
	}

	/* calculate window footer height */
	if win.flags&windowNonblock == 0 && (((win.flags&WindowNoScrollbar == 0) && (win.flags&WindowNoHScrollbar == 0)) || (win.flags&WindowScalable != 0)) {
		layout.FooterH = scaler_size.Y + wstyle.FooterPadding.Y
	} else {
		layout.FooterH = 0
	}

	/* calculate the window size */
	if win.flags&WindowNoScrollbar == 0 {
		layout.Width = layout.Bounds.W - wstyle.ScrollbarSize.X
	}
	layout.Height = layout.Bounds.H - (layout.HeaderH + item_spacing.Y + window_padding.Y)
	layout.Height -= layout.FooterH

	/* window header */
	header_active := (win.idx != 0) && (win.flags&WindowTitle != 0)

	var dwh drawableWindowHeader
	dwh.Minimized = layout.Flags&windowMinimized != 0
	dwh.Dynamic = layout.Flags&WindowDynamic != 0
	dwh.Bounds = layout.Bounds
	dwh.HeaderActive = header_active
	dwh.LayoutWidth = layout.Width
	dwh.Style = win.style()

	if header_active {
		/* calculate header bounds */
		dwh.Header.X = layout.Bounds.X
		dwh.Header.Y = layout.Bounds.Y
		dwh.Header.W = layout.Bounds.W

		/* calculate correct header height */
		layout.HeaderH = fontHeight(font) + 2.0*wstyle.Header.Padding.Y

		layout.HeaderH += 2.0 * wstyle.Header.LabelPadding.Y
		layout.Row.Height += layout.HeaderH
		dwh.Header.H = layout.HeaderH

		/* update window height */
		layout.Height = layout.Bounds.H - (dwh.Header.H + 2*item_spacing.Y)

		layout.Height -= layout.FooterH

		dwh.Hovered = ctx.Input.Mouse.HoveringRect(dwh.Header)

		header := dwh.Header

		/* window header title */
		t := FontWidth(font, title)

		dwh.Label.X = header.X + wstyle.Header.Padding.X
		dwh.Label.X += wstyle.Header.LabelPadding.X
		dwh.Label.Y = header.Y + wstyle.Header.LabelPadding.Y
		dwh.Label.H = fontHeight(font) + 2*wstyle.Header.LabelPadding.Y
		dwh.Label.W = t + 2*wstyle.Header.Spacing.X
		dwh.LayoutHeaderH = layout.HeaderH
		dwh.RowHeight = layout.Row.Height
		dwh.Title = title

		win.widgets.Add(types.WidgetStateInactive, layout.Bounds, &dwh)

		var button types.Rect
		/* window close button */
		button.Y = header.Y + wstyle.Header.Padding.Y
		button.H = layout.HeaderH - 2*wstyle.Header.Padding.Y
		button.W = button.H
		if win.flags&WindowClosable != 0 {
			if wstyle.Header.Align == nstyle.HeaderRight {
				button.X = (header.W + header.X) - (button.W + wstyle.Header.Padding.X)
				header.W -= button.W + wstyle.Header.Spacing.X + wstyle.Header.Padding.X
			} else {
				button.X = header.X + wstyle.Header.Padding.X
				header.X += button.W + wstyle.Header.Spacing.X + wstyle.Header.Padding.X
			}

			if doButton(win, label.S(wstyle.Header.CloseSymbol), button, &wstyle.Header.CloseButton, in, false) {
				layout.Flags |= windowHidden
			}
		}

		/* window minimize button */
		if win.flags&WindowMinimizable != 0 {
			if wstyle.Header.Align == nstyle.HeaderRight {
				button.X = (header.W + header.X) - button.W
				if win.flags&WindowClosable == 0 {
					button.X -= wstyle.Header.Padding.X
					header.W -= wstyle.Header.Padding.X
				}

				header.W -= button.W + wstyle.Header.Spacing.X
			} else {
				button.X = header.X
				header.X += button.W + wstyle.Header.Spacing.X + wstyle.Header.Padding.X
			}

			var symbolType label.SymbolType
			if layout.Flags&windowMinimized != 0 {
				symbolType = wstyle.Header.MaximizeSymbol
			} else {
				symbolType = wstyle.Header.MinimizeSymbol
			}
			if doButton(win, label.S(symbolType), button, &wstyle.Header.MinimizeButton, in, false) {
				if layout.Flags&windowMinimized != 0 {
					layout.Flags = layout.Flags & ^windowMinimized
				} else {
					layout.Flags = layout.Flags | windowMinimized
				}
			}
		}
	} else {
		dwh.LayoutHeaderH = layout.HeaderH
		dwh.RowHeight = layout.Row.Height
		win.widgets.Add(types.WidgetStateInactive, layout.Bounds, &dwh)
	}

	/* fix header height for transition between minimized and maximized window state */
	if win.flags&windowMinimized != 0 && layout.Flags&windowMinimized == 0 {
		layout.Row.Height += 2*item_spacing.Y + wstyle.Border
	}

	var dwb drawableWindowBody

	dwb.NoScrollbar = win.flags&WindowNoScrollbar != 0
	dwb.Style = win.style()

	if dwh.Minimized {
		/* draw window background if minimized */
		layout.Row.Height = 0
	}

	/* calculate and set the window clipping rectangle*/
	if win.flags&WindowDynamic == 0 {
		layout.Clip.X = layout.Bounds.X + window_padding.X
		layout.Clip.W = layout.Width - 2*window_padding.X
	} else {
		layout.Clip.X = layout.Bounds.X
		layout.Clip.W = layout.Width
	}

	layout.Clip.H = layout.Bounds.H - (layout.FooterH + layout.HeaderH)
	// FooterH already includes the window padding
	layout.Clip.H -= window_padding.Y
	layout.Clip.Y = layout.Bounds.Y

	/* combo box and menu do not have header space */
	if win.flags&windowCombo == 0 && win.flags&windowMenu == 0 {
		layout.Clip.Y += layout.HeaderH
	}

	clip := unify(win.widgets.Clip, layout.Clip)
	layout.Clip = clip

	dwb.Bounds = layout.Bounds
	dwb.LayoutWidth = layout.Width
	dwb.Clip = layout.Clip
	win.widgets.Clip = dwb.Clip
	win.widgets.Add(types.WidgetStateInactive, dwb.Bounds, &dwb)

	layout.Row.Type = layoutInvalid

	return layout.Flags&windowHidden == 0 && layout.Flags&windowMinimized == 0
}

var nk_null_rect = types.Rect{-8192.0, -8192.0, 16384.0, 16384.0}

func panelEnd(ctx *context, window *Window) {
	var footer = types.Rect{0, 0, 0, 0}

	layout := window.layout
	style := &ctx.Style
	in := &Input{}
	if window.toplevel() {
		in = &ctx.Input
	}
	outclip := nk_null_rect
	if window.flags&windowGroup != 0 {
		outclip = window.parent.widgets.Clip
	}
	window.widgets.Clip = outclip
	window.widgets.Add(types.WidgetStateInactive, outclip, &drawableScissor{outclip})

	wstyle := window.style()

	/* cache configuration data */
	item_spacing := wstyle.Spacing
	window_padding := wstyle.Padding
	scrollbar_size := wstyle.ScrollbarSize
	scaler_size := wstyle.ScalerSize

	/* update the current cursor Y-position to point over the last added widget */
	layout.AtY += layout.Row.Height

	/* draw footer and fill empty spaces inside a dynamically growing panel */
	if layout.Flags&WindowDynamic != 0 && layout.Flags&windowMinimized == 0 {
		layout.Height = layout.AtY - layout.Bounds.Y
		layout.Height = min(layout.Height, layout.Bounds.H)

		// fill horizontal scrollbar space
		{
			var bounds types.Rect
			bounds.X = window.Bounds.X
			bounds.Y = layout.AtY - item_spacing.Y
			bounds.W = window.Bounds.W
			bounds.H = window.Bounds.Y + layout.Height + item_spacing.Y + window.style().Padding.Y - bounds.Y

			window.widgets.Add(types.WidgetStateInactive, bounds, &drawableFillRect{bounds, wstyle.Background})
		}

		if (layout.Offset.X == 0) || (layout.Flags&WindowNoScrollbar != 0) {
			/* special case for dynamic windows without horizontal scrollbar
			 * or hidden scrollbars */
			footer.X = window.Bounds.X
			footer.Y = window.Bounds.Y + layout.Height + item_spacing.Y + window.style().Padding.Y
			footer.W = window.Bounds.W + scrollbar_size.X
			layout.FooterH = 0
			footer.H = 0

			if (layout.Offset.X == 0) && layout.Flags&WindowNoScrollbar == 0 {
				/* special case for windows like combobox, menu require draw call
				 * to fill the empty scrollbar background */
				var bounds types.Rect
				bounds.X = layout.Bounds.X + layout.Width
				bounds.Y = layout.Clip.Y
				bounds.W = scrollbar_size.X
				bounds.H = layout.Height

				window.widgets.Add(types.WidgetStateInactive, bounds, &drawableFillRect{bounds, wstyle.Background})
			}
		} else {
			/* dynamic window with visible scrollbars and therefore bigger footer */
			footer.X = window.Bounds.X
			footer.W = window.Bounds.W + scrollbar_size.X
			footer.H = layout.FooterH
			if (layout.Flags&windowCombo != 0) || (layout.Flags&windowMenu != 0) || (layout.Flags&windowContextual != 0) {
				footer.Y = window.Bounds.Y + layout.Height
			} else {
				footer.Y = window.Bounds.Y + layout.Height + layout.FooterH
			}
			window.widgets.Add(types.WidgetStateInactive, footer, &drawableFillRect{footer, wstyle.Background})

			if layout.Flags&windowCombo == 0 && layout.Flags&windowMenu == 0 {
				/* fill empty scrollbar space */
				var bounds types.Rect
				bounds.X = layout.Bounds.X
				bounds.Y = window.Bounds.Y + layout.Height
				bounds.W = layout.Bounds.W
				bounds.H = layout.Row.Height
				window.widgets.Add(types.WidgetStateInactive, bounds, &drawableFillRect{bounds, wstyle.Background})
			}
		}
	}

	/* scrollbars */
	if layout.Flags&WindowNoScrollbar == 0 && layout.Flags&windowMinimized == 0 {
		var bounds types.Rect
		var scroll_target float64
		var scroll_offset float64
		var scroll_step float64
		var scroll_inc float64
		{
			/* vertical scrollbar */
			bounds.X = layout.Bounds.X + layout.Width
			bounds.Y = layout.Clip.Y
			bounds.W = scrollbar_size.Y
			bounds.H = layout.Clip.H
			if layout.Flags&WindowBorder != 0 {
				bounds.H -= 1
			}

			scroll_offset = float64(layout.Offset.Y)
			scroll_step = float64(layout.Clip.H) * 0.10
			scroll_inc = float64(layout.Clip.H) * 0.01
			scroll_target = float64(layout.AtY - layout.Clip.Y)
			scroll_offset = doScrollbarv(window, bounds, layout.Bounds, scroll_offset, scroll_target, scroll_step, scroll_inc, &ctx.Style.Scrollv, in, style.Font)
			layout.Offset.Y = int(scroll_offset)
		}
		if layout.Flags&WindowNoHScrollbar == 0 {
			/* horizontal scrollbar */
			bounds.X = layout.Bounds.X + window_padding.X
			if layout.Flags&windowSub != 0 {
				bounds.H = scrollbar_size.X
				bounds.Y = layout.Bounds.Y
				if layout.Flags&WindowBorder != 0 {
					bounds.Y++
				}
				bounds.Y += layout.HeaderH + layout.Menu.H + layout.Height
				bounds.W = layout.Clip.W
			} else if layout.Flags&WindowDynamic != 0 {
				bounds.H = min(scrollbar_size.X, layout.FooterH)
				bounds.W = layout.Bounds.W
				bounds.Y = footer.Y
			} else {
				bounds.H = min(scrollbar_size.X, layout.FooterH)
				bounds.Y = layout.Bounds.Y + window.Bounds.H
				bounds.Y -= max(layout.FooterH, scrollbar_size.X)
				bounds.W = layout.Width - 2*window_padding.X
			}

			scroll_offset = float64(layout.Offset.X)
			scroll_target = float64(layout.MaxX - bounds.X)
			scroll_step = float64(layout.MaxX) * 0.05
			scroll_inc = float64(layout.MaxX) * 0.005
			scroll_offset = doScrollbarh(window, bounds, scroll_offset, scroll_target, scroll_step, scroll_inc, &ctx.Style.Scrollh, in, style.Font)
			layout.Offset.X = int(scroll_offset)
		}
	}

	var dsab drawableScalerAndBorders
	dsab.Style = window.style()
	dsab.Bounds = window.Bounds
	dsab.Border = layout.Border
	dsab.HeaderH = layout.HeaderH

	/* scaler */
	if (layout.Flags&WindowScalable != 0) && layout.Flags&windowMinimized == 0 {
		dsab.DrawScaler = true

		dsab.ScalerRect.W = max(0, scaler_size.X-window_padding.X)
		dsab.ScalerRect.H = max(0, scaler_size.Y-window_padding.Y)
		dsab.ScalerRect.X = (layout.Bounds.X + layout.Bounds.W) - (window_padding.X + dsab.ScalerRect.W)
		/* calculate scaler bounds */
		if layout.Flags&WindowDynamic != 0 {
			dsab.ScalerRect.Y = footer.Y + layout.FooterH - scaler_size.Y
		} else {
			dsab.ScalerRect.Y = layout.Bounds.Y + layout.Bounds.H - scaler_size.Y
		}

		/* do window scaling logic */
		if window.toplevel() {
			prev := in.Mouse.Prev
			window_size := wstyle.MinSize

			incursor := dsab.ScalerRect.Contains(prev)

			if in != nil && in.Mouse.Down(mouse.ButtonLeft) && incursor {
				window.Bounds.W = max(window_size.X, window.Bounds.W+in.Mouse.Delta.X)

				/* dragging in y-direction is only possible if static window */
				if layout.Flags&WindowDynamic == 0 {
					window.Bounds.H = max(window_size.Y, window.Bounds.H+in.Mouse.Delta.Y)
				}
			}
		}
	}

	/* window border */
	if layout.Flags&WindowBorder != 0 {
		dsab.DrawBorders = true

		if layout.Flags&windowMinimized != 0 {
			dsab.PaddingY = 2.0*wstyle.Border + window.Bounds.Y + layout.HeaderH
		} else if layout.Flags&WindowDynamic != 0 {
			dsab.PaddingY = layout.FooterH + footer.Y
		} else {
			dsab.PaddingY = layout.Bounds.Y + layout.Bounds.H
		}
		/* select correct border color */
		dsab.BorderColor = wstyle.BorderColor

		/* draw border between header and window body */
		if window.flags&WindowBorderHeader != 0 {
			dsab.DrawHeaderBorder = true
		}
	}

	window.widgets.Add(types.WidgetStateInactive, dsab.Bounds, &dsab)

	if window.flags&windowSub == 0 {
		if layout.Flags&windowHidden != 0 {
			/* window is hidden so clear command buffer  */
			window.widgets.reset()
			window.widgets.prev = []frozenWidget{}
		}
	}

	window.flags = layout.Flags

	/* helper to make sure you have a 'nk_tree_push'
	 * for every 'nk_tree_pop' */
	assert(layout.Row.TreeDepth == 0)
}

// MenubarBegin adds a menubar to the current window.
// A menubar is an area displayed at the top of the window that is unaffected by scrolling.
// Remember to call MenubarEnd when you are done adding elements to the menubar.
func (win *Window) MenubarBegin() {
	layout := win.layout
	if layout.Flags&windowHidden != 0 || layout.Flags&windowMinimized != 0 {
		return
	}

	layout.Menu.X = layout.AtX
	layout.Menu.Y = layout.Bounds.Y + layout.HeaderH
	layout.Menu.W = layout.Width
	layout.Menu.Offset = *layout.Offset
	layout.Offset.Y = 0
}

// MenubarEnd signals that all widgets have been added to the menubar.
func (win *Window) MenubarEnd() {
	layout := win.layout
	if layout.Flags&windowHidden != 0 || layout.Flags&windowMinimized != 0 {
		return
	}

	layout.Menu.H = layout.AtY - layout.Menu.Y
	layout.Clip.Y = layout.Bounds.Y + layout.HeaderH + layout.Menu.H + layout.Row.Height
	layout.Height -= layout.Menu.H
	*layout.Offset = layout.Menu.Offset
	layout.Clip.H -= layout.Menu.H + layout.Row.Height
	layout.AtY = layout.Menu.Y + layout.Menu.H
	win.widgets.Clip = layout.Clip
	win.widgets.Add(types.WidgetStateInactive, layout.Clip, &drawableScissor{layout.Clip})
}

type widgetLayoutStates int

const (
	widgetInvalid = widgetLayoutStates(iota)
	widgetValid
	widgetRom
)

func (win *Window) widget() (widgetLayoutStates, types.Rect) {
	var c *types.Rect = nil

	var bounds types.Rect

	/* allocate space  and check if the widget needs to be updated and drawn */
	panelAllocSpace(&bounds, win)

	c = &win.layout.Clip
	if !c.Intersect(&bounds) {
		return widgetInvalid, bounds
	}

	contains := func(r *types.Rect, b *types.Rect) bool {
		return b.Contains(image.Point{r.X, r.Y}) && b.Contains(image.Point{r.X + r.W, r.Y + r.H})
	}

	if !contains(&bounds, c) {
		return widgetRom, bounds
	}
	return widgetValid, bounds
}

func (win *Window) widgetFitting(item_padding image.Point) (state widgetLayoutStates, bounds types.Rect) {
	/* update the bounds to stand without padding  */
	style := win.style()
	layout := win.layout
	state, bounds = win.widget()
	if layout.Row.Index == 1 {
		bounds.W += style.Padding.X
		bounds.X -= style.Padding.X
	} else {
		bounds.X -= item_padding.X
	}

	if layout.Row.Columns > 0 && layout.Row.Index == layout.Row.Columns {
		bounds.W += style.Padding.X
	} else {
		bounds.W += item_padding.X
	}
	return state, bounds
}

func panelAllocSpace(bounds *types.Rect, win *Window) {
	/* check if the end of the row has been hit and begin new row if so */
	layout := win.layout
	if layout.Row.Columns > 0 && layout.Row.Index >= layout.Row.Columns {
		panelAllocRow(win)
	}

	/* calculate widget position and size */
	layoutWidgetSpace(bounds, win.ctx, win, true)

	layout.Row.Index++
}

func panelAllocRow(win *Window) {
	layout := win.layout
	spacing := win.style().Spacing
	row_height := layout.Row.Height - spacing.Y
	panelLayout(win.ctx, win, row_height, layout.Row.Columns)
}

func panelLayout(ctx *context, win *Window, height int, cols int) {
	/* prefetch some configuration data */
	layout := win.layout

	if height == 0 {
		height = win.LayoutAvailableHeight() - layout.ReservedHeight
	}

	style := win.style()
	item_spacing := style.Spacing
	//panel_padding := style.Padding

	/* update the current row and set the current row layout */
	layout.Row.Index = 0

	layout.AtY += layout.Row.Height
	layout.Row.Columns = cols
	layout.Row.Height = height + item_spacing.Y
	layout.Row.ItemOffset = 0
	if layout.Flags&WindowDynamic != 0 {
		var drect drawableFillRect
		drect.R = types.Rect{layout.Bounds.X, layout.AtY, layout.Bounds.W, height + item_spacing.Y}
		drect.C = style.Background
		win.widgets.Add(types.WidgetStateInactive, drect.R, &drect)
	}
}

const (
	layoutDynamicFixed = iota
	layoutDynamicFree
	layoutDynamic
	layoutStaticFree
	layoutStatic
	layoutInvalid
)

var InvalidLayoutErr = errors.New("invalid layout")

func layoutWidgetSpace(bounds *types.Rect, ctx *context, win *Window, modify bool) {
	layout := win.layout

	/* cache some configuration data */
	style := win.style()
	spacing := style.Spacing
	padding := style.Padding

	/* calculate the usable panel space */
	panel_padding := 2 * padding.X

	panel_spacing := int(float64(layout.Row.Columns-1) * float64(spacing.X))
	panel_space := layout.Width - panel_padding - panel_spacing

	/* calculate the width of one item inside the current layout space */
	item_offset := 0
	item_width := 0
	item_spacing := 0

	switch layout.Row.Type {
	case layoutInvalid:
		panic(InvalidLayoutErr)
	case layoutDynamicFixed:
		/* scaling fixed size widgets item width */
		item_width = int(float64(panel_space) / float64(layout.Row.Columns))

		item_offset = layout.Row.Index * item_width
		item_spacing = layout.Row.Index * spacing.X
	case layoutDynamicFree:
		/* panel width depended free widget placing */
		bounds.X = layout.AtX + int(float64(layout.Width)*layout.Row.DynamicFreeX)
		bounds.X -= layout.Offset.X
		bounds.Y = layout.AtY + int(float64(layout.Row.Height)*layout.Row.DynamicFreeY)
		bounds.Y -= layout.Offset.Y
		bounds.W = int(float64(layout.Width) * layout.Row.DynamicFreeW)
		bounds.H = int(float64(layout.Row.Height) * layout.Row.DynamicFreeH)
		return
	case layoutDynamic:
		/* scaling arrays of panel width ratios for every widget */
		var ratio float64
		if layout.Row.Ratio[layout.Row.Index] < 0 {
			ratio = layout.Row.ItemRatio
		} else {
			ratio = layout.Row.Ratio[layout.Row.Index]
		}

		item_spacing = layout.Row.Index * spacing.X
		item_width = int(ratio * float64(panel_space))
		item_offset = layout.Row.ItemOffset
		if modify {
			layout.Row.ItemOffset += item_width
			layout.Row.Filled += ratio
		}
	case layoutStaticFree:
		/* free widget placing */
		bounds.X = layout.AtX + layout.Row.Item.X

		bounds.W = layout.Row.Item.W
		if ((bounds.X + bounds.W) > layout.MaxX) && modify {
			layout.MaxX = (bounds.X + bounds.W)
		}
		bounds.X -= layout.Offset.X
		bounds.Y = layout.AtY + layout.Row.Item.Y
		bounds.Y -= layout.Offset.Y
		bounds.H = layout.Row.Item.H
		return
	case layoutStatic:
		/* non-scaling array of panel pixel width for every widget */
		item_spacing = layout.Row.Index * spacing.X

		if len(layout.Row.WidthArr) > 0 {
			item_width = layout.Row.WidthArr[layout.Row.Index]
		} else {
			item_width = layout.Row.ItemWidth
		}
		item_offset = layout.Row.ItemOffset
		if modify {
			layout.Row.ItemOffset += item_width
		}

	default:
		assert(false)
	}

	/* set the bounds of the newly allocated widget */
	bounds.W = item_width

	bounds.H = layout.Row.Height - spacing.Y
	bounds.Y = layout.AtY - layout.Offset.Y
	bounds.X = layout.AtX + item_offset + item_spacing + padding.X
	if ((bounds.X + bounds.W) > layout.MaxX) && modify {
		layout.MaxX = bounds.X + bounds.W
	}
	bounds.X -= layout.Offset.X
}

func (ctx *context) scale(x int) int {
	return int(float64(x) * ctx.Scaling)
}

func rowLayoutCtr(win *Window, height int, cols int, width int, scale bool) {
	/* update the current row and set the current row layout */
	if scale {
		height = win.ctx.scale(height)
		width = win.ctx.scale(width)
	}
	panelLayout(win.ctx, win, height, cols)
	win.layout.Row.Type = layoutDynamicFixed

	win.layout.Row.ItemWidth = width
	win.layout.Row.ItemRatio = 0.0
	win.layout.Row.Ratio = nil
	win.layout.Row.ItemOffset = 0
	win.layout.Row.Filled = 0
}

// Reserves space for num rows of the specified height at the bottom
// of the panel.
// If a row of height == 0  is inserted it will take reserved space
// into account.
func (win *Window) LayoutReserveRow(height int, num int) {
	win.LayoutReserveRow(win.ctx.scale(height), num)
}

// Like LayoutReserveRow but with a scaled height.
func (win *Window) LayoutReserveRowScaled(height int, num int) {
	win.layout.ReservedHeight += height*num + win.style().Spacing.Y*num
}

// Starts new row that has cols columns of equal width that automatically
// resize to fill the available space.
// If height == 0 all the row is stretched to fill all the remaining space.
func (win *Window) LayoutRowDynamic(height int, cols int) {
	rowLayoutCtr(win, height, cols, 0, true)
}

// Like LayoutRowDynamic but height is specified in scaled units.
// If height == 0 all the row is stretched to fill all the remaining space.
func (win *Window) LayoutRowDynamicScaled(height int, cols int) {
	rowLayoutCtr(win, height, cols, 0, false)
}

func (win *Window) LayoutRowRatio(height int, ratio ...float64) {
	win.LayoutRowRatioScaled(win.ctx.scale(height), ratio...)
}

// Starts new row with a fixed number of columns of width proportional
// to the size of the window.
// If height == 0 all the row is stretched to fill all the remaining space.
func (win *Window) LayoutRowRatioScaled(height int, ratio ...float64) {
	layout := win.layout
	panelLayout(win.ctx, win, height, len(ratio))

	/* calculate width of undefined widget ratios */
	r := 0.0
	n_undef := 0
	layout.Row.Ratio = ratio
	for i := range ratio {
		if ratio[i] < 0.0 {
			n_undef++
		} else {
			r += ratio[i]
		}
	}

	r = saturateFloat(1.0 - r)
	layout.Row.Type = layoutDynamic
	layout.Row.ItemWidth = 0
	layout.Row.ItemRatio = 0.0
	if r > 0 && n_undef > 0 {
		layout.Row.ItemRatio = (r / float64(n_undef))
	}

	layout.Row.ItemOffset = 0
	layout.Row.Filled = 0
}

// Like LayoutRowStatic but with scaled sizes.
func (win *Window) LayoutRowStaticScaled(height int, width ...int) {
	layout := win.layout
	panelLayout(win.ctx, win, height, len(width))

	nzero := 0
	used := 0
	for i := range width {
		if width[i] == 0 {
			nzero++
		}
		used += width[i]
	}

	if nzero > 0 {
		style := win.style()
		spacing := style.Spacing
		padding := style.Padding
		panel_padding := 2 * padding.X
		panel_spacing := int(float64(len(width)-1) * float64(spacing.X))
		panel_space := layout.Width - panel_padding - panel_spacing

		unused := panel_space - used

		zerowidth := unused / nzero

		for i := range width {
			if width[i] == 0 {
				width[i] = zerowidth
			}
		}
	}

	layout.Row.WidthArr = width
	layout.Row.Type = layoutStatic
	layout.Row.ItemWidth = 0
	layout.Row.ItemRatio = 0.0
	layout.Row.ItemOffset = 0
	layout.Row.ItemOffset = 0
	layout.Row.Filled = 0
}

func (win *Window) LayoutSetWidth(width int) {
	layout := win.layout
	if layout.Row.Type != layoutStatic || len(layout.Row.WidthArr) > 0 {
		panic(WrongLayoutErr)
	}
	layout.Row.ItemWidth = win.ctx.scale(width)
}

func (win *Window) LayoutSetWidthScaled(width int) {
	layout := win.layout
	if layout.Row.Type != layoutStatic || len(layout.Row.WidthArr) > 0 {
		panic(WrongLayoutErr)
	}
	layout.Row.ItemWidth = width
}

// Starts new row with a fixed number of columns with the specfieid widths.
// If no widths are specified the row will never autowrap
// and the width of the next widget can be specified using
// LayoutSetWidth/LayoutSetWidthScaled.
// If height == 0 all the row is stretched to fill all the remaining space.
func (win *Window) LayoutRowStatic(height int, width ...int) {
	for i := range width {
		width[i] = win.ctx.scale(width[i])
	}

	win.LayoutRowStaticScaled(win.ctx.scale(height), width...)
}

// Starts new row that will contain widget_count widgets.
// The size and position of widgets inside this row will be specified
// by callling LayoutSpacePush.
func (win *Window) LayoutSpaceBegin(height int, widget_count int) {
	layout := win.layout
	panelLayout(win.ctx, win, win.ctx.scale(height), widget_count)
	layout.Row.Type = layoutStaticFree

	layout.Row.Ratio = nil
	layout.Row.ItemWidth = 0
	layout.Row.ItemRatio = 0.0
	layout.Row.ItemOffset = 0
	layout.Row.Filled = 0
}

// Starts new row that will contain widget_count widgets.
// The size and position of widgets inside this row will be specified
// by callling LayoutSpacePushRatio.
// If height == 0 all the row is stretched to fill all the remaining space.
func (win *Window) LayoutSpaceBeginRatio(height int, widget_count int) {
	layout := win.layout
	panelLayout(win.ctx, win, win.ctx.scale(height), widget_count)
	layout.Row.Type = layoutDynamicFree

	layout.Row.Ratio = nil
	layout.Row.ItemWidth = 0
	layout.Row.ItemRatio = 0.0
	layout.Row.ItemOffset = 0
	layout.Row.Filled = 0
}

var WrongLayoutErr = errors.New("Command not available with current layout")

// Sets position and size of the next widgets in a Space row layout
func (win *Window) LayoutSpacePush(rect types.Rect) {
	if win.layout.Row.Type != layoutStaticFree {
		panic(WrongLayoutErr)
	}
	rect.X = win.ctx.scale(rect.X)
	rect.Y = win.ctx.scale(rect.Y)
	rect.W = win.ctx.scale(rect.W)
	rect.H = win.ctx.scale(rect.H)
	win.layout.Row.Item = rect
}

// Sets position and size of the next widgets in a Space row layout
func (win *Window) LayoutSpacePushRatio(x, y, w, h float64) {
	if win.layout.Row.Type != layoutDynamicFree {
		panic(WrongLayoutErr)
	}
	win.layout.Row.DynamicFreeX, win.layout.Row.DynamicFreeY, win.layout.Row.DynamicFreeH, win.layout.Row.DynamicFreeW = x, y, w, h
}

func (win *Window) layoutPeek(bounds *types.Rect) {
	layout := win.layout
	y := layout.AtY
	index := layout.Row.Index
	if layout.Row.Columns > 0 && layout.Row.Index >= layout.Row.Columns {
		layout.AtY += layout.Row.Height
		layout.Row.Index = 0
	}

	layoutWidgetSpace(bounds, win.ctx, win, false)
	layout.AtY = y
	layout.Row.Index = index
}

// Returns the position and size of the next widget that will be
// added to the current row.
// Note that the return value is in scaled units.
func (win *Window) WidgetBounds() types.Rect {
	var bounds types.Rect
	win.layoutPeek(&bounds)
	return bounds
}

// Returns remaining available height of win in scaled units.
func (win *Window) LayoutAvailableHeight() int {
	return win.layout.Clip.H - (win.layout.AtY - win.layout.Bounds.Y) - win.style().Spacing.Y - win.layout.Row.Height
}

func (win *Window) LayoutAvailableWidth() int {
	style := win.style()
	return win.layout.Width - style.Padding.X*2 - style.Spacing.X - win.layout.AtX
}

// Will return (false, false) if the next widget is visible, (true,
// false) if it is above the visible area, (false, true) if it is
// below the visible area
func (win *Window) Invisible() (above, below bool) {
	y := win.layout.AtY - win.layout.Offset.Y
	return y < win.layout.Clip.Y, y > (win.layout.Clip.Y + win.layout.Clip.H)
}

func (win *Window) At() image.Point {
	return image.Point{win.layout.AtX - win.layout.Clip.X, win.layout.AtY - win.layout.Clip.Y}
}

///////////////////////////////////////////////////////////////////////////////////
// TREE
///////////////////////////////////////////////////////////////////////////////////

type TreeType int

const (
	TreeNode TreeType = iota
	TreeTab
)

// Creates a new collapsable section inside win. Returns true
// when the section is open. Widgets that are inside this collapsable
// section should be added to win only when this function returns true.
// Once you are done adding elements to the collapsable section
// call TreePop.
// Initial_open will determine whether this collapsable section
// will be initially open.
// Type_ will determine the style of this collapsable section.
func (win *Window) TreePush(type_ TreeType, title string, initial_open bool) bool {
	/* cache some data */
	layout := win.layout
	style := &win.ctx.Style
	panel_padding := win.style().Padding

	/* calculate header bounds and draw background */
	panelLayout(win.ctx, win, fontHeight(style.Font)+2*style.Tab.Padding.Y, 1)
	win.layout.Row.Type = layoutDynamicFixed
	win.layout.Row.ItemWidth = 0
	win.layout.Row.ItemRatio = 0.0
	win.layout.Row.Ratio = nil
	win.layout.Row.ItemOffset = 0
	win.layout.Row.Filled = 0

	widget_state, header := win.widget()

	/* find or create tab persistent state (open/closed) */

	node := win.curNode.Children[title]
	if node == nil {
		node = createTreeNode(initial_open, win.curNode)
		win.curNode.Children[title] = node
	}

	/* update node state */
	in := &Input{}
	if win.toplevel() && widget_state == widgetValid {
		in = &win.ctx.Input
	}

	ws := win.widgets.PrevState(header)
	if buttonBehaviorDo(&ws, header, in, false) {
		node.Open = !node.Open
	}

	/* calculate the triangle bounds */
	var sym types.Rect
	sym.H = fontHeight(style.Font)
	sym.W = sym.H
	sym.Y = header.Y + style.Tab.Padding.Y
	sym.X = header.X + panel_padding.X + style.Tab.Padding.X

	win.widgets.Add(ws, header, &drawableTreeNode{win.style(), type_, header, sym, title})

	/* calculate the triangle points and draw triangle */
	symbolType := style.Tab.SymMaximize
	if node.Open {
		symbolType = style.Tab.SymMinimize
	}
	styleButton := &style.Tab.NodeButton
	if type_ == TreeTab {
		styleButton = &style.Tab.TabButton
	}
	doButton(win, label.S(symbolType), sym, styleButton, in, false)

	/* increase x-axis cursor widget position pointer */
	if node.Open {
		layout.AtX = header.X + layout.Offset.X
		layout.Width = max(layout.Width, 2*panel_padding.X)
		layout.Width -= 2 * panel_padding.X
		layout.Row.TreeDepth++
		win.curNode = node
		return true
	} else {
		return false
	}
}

// TreePop signals that the program is done adding elements to the
// current collapsable section.
func (win *Window) TreePop() {
	layout := win.layout
	panel_padding := win.style().Padding
	layout.AtX -= panel_padding.X
	layout.Width += 2 * panel_padding.X
	assert(layout.Row.TreeDepth != 0)
	win.curNode = win.curNode.Parent
	layout.Row.TreeDepth--
}

///////////////////////////////////////////////////////////////////////////////////
// NON-INTERACTIVE WIDGETS
///////////////////////////////////////////////////////////////////////////////////

// LabelColored draws a text label with the specified background color.
func (win *Window) LabelColored(str string, alignment label.Align, color color.RGBA) {
	var bounds types.Rect
	var text textWidget

	style := &win.ctx.Style
	panelAllocSpace(&bounds, win)
	item_padding := style.Text.Padding

	text.Padding.X = item_padding.X
	text.Padding.Y = item_padding.Y
	text.Background = win.style().Background
	text.Text = color
	win.widgets.Add(types.WidgetStateInactive, bounds, &drawableNonInteractive{bounds, color, false, text, alignment, str, nil, nil})

}

// LabelWrapColored draws a text label with the specified background
// color autowrappping the text.
func (win *Window) LabelWrapColored(str string, color color.RGBA) {
	var bounds types.Rect
	var text textWidget

	style := &win.ctx.Style
	panelAllocSpace(&bounds, win)
	item_padding := style.Text.Padding

	text.Padding.X = item_padding.X
	text.Padding.Y = item_padding.Y
	text.Background = win.style().Background
	text.Text = color
	win.widgets.Add(types.WidgetStateInactive, bounds, &drawableNonInteractive{bounds, color, true, text, "LT", str, nil, nil})
}

// Label draws a text label.
func (win *Window) Label(str string, alignment label.Align) {
	win.LabelColored(str, alignment, win.ctx.Style.Text.Color)
}

// LabelWrap draws a text label, autowrapping its contents.
func (win *Window) LabelWrap(str string) {
	win.LabelWrapColored(str, win.ctx.Style.Text.Color)
}

// Image draws an image.
func (win *Window) Image(img *image.RGBA) {
	var bounds types.Rect
	var s widgetLayoutStates

	if s, bounds = win.widget(); s == 0 {
		return
	}
	win.widgets.Add(types.WidgetStateInactive, bounds, &drawableNonInteractive{Bounds: bounds, Img: img})
}

// Spacing adds empty space
func (win *Window) Spacing(cols int) {
	var nilrect types.Rect
	var i int

	/* spacing over row boundaries */

	layout := win.layout
	if layout.Row.Columns > 0 {
		index := (layout.Row.Index + cols) % layout.Row.Columns
		rows := (layout.Row.Index + cols) / layout.Row.Columns
		if rows != 0 {
			for i = 0; i < rows; i++ {
				panelAllocRow(win)
			}
			cols = index
		}
	}

	/* non table layout need to allocate space */
	if layout.Row.Type != layoutDynamicFixed {
		for i = 0; i < cols; i++ {
			panelAllocSpace(&nilrect, win)
		}
	}
}

// CustomState returns the widget state of a custom widget.
func (win *Window) CustomState() types.WidgetStates {
	bounds := win.WidgetBounds()
	s := widgetValid
	if !win.layout.Clip.Intersect(&bounds) {
		s = widgetInvalid
	}

	ws := win.widgets.PrevState(bounds)
	basicWidgetStateControl(&ws, win.inputMaybe(s), bounds)
	return ws
}

// Custom adds a custom widget.
func (win *Window) Custom(state types.WidgetStates) (bounds types.Rect, out *command.Buffer) {
	var s widgetLayoutStates

	if s, bounds = win.widget(); s == 0 {
		return
	}
	prevstate := win.widgets.PrevState(bounds)
	exitstate := basicWidgetStateControl(&prevstate, win.inputMaybe(s), bounds)
	if state != types.WidgetStateActive {
		state = exitstate
	}
	win.widgets.Add(state, bounds, nil)
	return bounds, &win.cmds
}

///////////////////////////////////////////////////////////////////////////////////
// BUTTON
///////////////////////////////////////////////////////////////////////////////////

func buttonBehaviorDo(state *types.WidgetStates, r types.Rect, i *Input, repeat bool) (ret bool) {
	exitstate := basicWidgetStateControl(state, i, r)

	if *state == types.WidgetStateActive {
		if exitstate == types.WidgetStateHovered {
			if repeat {
				ret = i.Mouse.Down(mouse.ButtonLeft)
			} else {
				ret = i.Mouse.Released(mouse.ButtonLeft)
			}
		}
		if !i.Mouse.Down(mouse.ButtonLeft) {
			*state = exitstate
		}
	}

	return ret
}

func doButton(win *Window, lbl label.Label, r types.Rect, style *nstyle.Button, in *Input, repeat bool) bool {
	out := win.widgets
	if lbl.Kind == label.ColorLabel {
		button := *style
		button.Normal = nstyle.MakeItemColor(lbl.Color)
		button.Hover = nstyle.MakeItemColor(lbl.Color)
		button.Active = nstyle.MakeItemColor(lbl.Color)
		button.Padding = image.Point{0, 0}
		style = &button
	}

	/* calculate button content space */
	var content types.Rect
	content.X = r.X + style.Padding.X + style.Border
	content.Y = r.Y + style.Padding.Y + style.Border
	content.W = r.W - 2*style.Padding.X + style.Border
	content.H = r.H - 2*style.Padding.Y + style.Border

	/* execute button behavior */
	var bounds types.Rect
	bounds.X = r.X - style.TouchPadding.X
	bounds.Y = r.Y - style.TouchPadding.Y
	bounds.W = r.W + 2*style.TouchPadding.X
	bounds.H = r.H + 2*style.TouchPadding.Y
	state := out.PrevState(bounds)
	ok := buttonBehaviorDo(&state, bounds, in, repeat)

	switch lbl.Kind {
	case label.TextLabel:
		if lbl.Align == "" {
			lbl.Align = "CC"
		}
		out.Add(state, bounds, &drawableTextButton{bounds, content, state, style, lbl.Text, lbl.Align})
	case label.SymbolLabel:
		out.Add(state, bounds, &drawableSymbolButton{bounds, content, state, style, lbl.Symbol})
	case label.ImageLabel:
		content.X += style.ImagePadding.X
		content.Y += style.ImagePadding.Y
		content.W -= 2 * style.ImagePadding.X
		content.H -= 2 * style.ImagePadding.Y

		out.Add(state, bounds, &drawableImageButton{bounds, content, state, style, lbl.Img})
	case label.SymbolTextLabel:
		if lbl.Align == "" {
			lbl.Align = "CC"
		}
		font := win.ctx.Style.Font
		var tri types.Rect
		tri.Y = content.Y + (content.H / 2) - fontHeight(font)/2
		tri.W = fontHeight(font)
		tri.H = fontHeight(font)
		if lbl.Align[0] == 'L' {
			tri.X = (content.X + content.W) - (2*style.Padding.X + tri.W)
			tri.X = max(tri.X, 0)
		} else {
			tri.X = content.X + 2*style.Padding.X
		}

		out.Add(state, bounds, &drawableTextSymbolButton{bounds, content, tri, state, style, lbl.Text, lbl.Symbol})

	case label.ImageTextLabel:
		if lbl.Align == "" {
			lbl.Align = "CC"
		}
		var icon types.Rect
		icon.Y = bounds.Y + style.Padding.Y
		icon.H = bounds.H - 2*style.Padding.Y
		icon.W = icon.H
		if lbl.Align[0] == 'L' {
			icon.X = (bounds.X + bounds.W) - (2*style.Padding.X + icon.W)
			icon.X = max(icon.X, 0)
		} else {
			icon.X = bounds.X + 2*style.Padding.X
		}

		icon.X += style.ImagePadding.X
		icon.Y += style.ImagePadding.Y
		icon.W -= 2 * style.ImagePadding.X
		icon.H -= 2 * style.ImagePadding.Y

		out.Add(state, bounds, &drawableTextImageButton{bounds, content, icon, state, style, lbl.Text, lbl.Img})

	case label.ColorLabel:
		out.Add(state, bounds, &drawableSymbolButton{bounds, bounds, state, style, label.SymbolNone})

	}

	return ok
}

// Button adds a button
func (win *Window) Button(lbl label.Label, repeat bool) bool {
	style := &win.ctx.Style
	state, bounds := win.widget()

	if state == 0 {
		return false
	}
	in := win.inputMaybe(state)
	return doButton(win, lbl, bounds, &style.Button, in, repeat)
}

func (win *Window) ButtonText(text string) bool {
	return win.Button(label.T(text), false)
}

///////////////////////////////////////////////////////////////////////////////////
// SELECTABLE
///////////////////////////////////////////////////////////////////////////////////

func doSelectable(out *widgetBuffer, bounds types.Rect, str string, align label.Align, value *bool, style *nstyle.Selectable, in *Input) bool {
	if str == "" {
		return false
	}
	old_value := *value

	/* remove padding */
	var touch types.Rect
	touch.X = bounds.X - style.TouchPadding.X
	touch.Y = bounds.Y - style.TouchPadding.Y
	touch.W = bounds.W + style.TouchPadding.X*2
	touch.H = bounds.H + style.TouchPadding.Y*2

	/* update button */
	state := out.PrevState(bounds)
	if buttonBehaviorDo(&state, touch, in, false) {
		*value = !*value
	}

	out.Add(state, bounds, &drawableSelectable{state, style, *value, bounds, str, align})
	return old_value != *value
}

// SelectableLabel adds a selectable label. Value is a pointer
// to a flag that will be changed to reflect the selected state of
// this label.
// Returns true when the label is clicked.
func (win *Window) SelectableLabel(str string, align label.Align, value *bool) bool {
	style := &win.ctx.Style
	state, bounds := win.widget()
	if state == 0 {
		return false
	}
	in := win.inputMaybe(state)
	return doSelectable(&win.widgets, bounds, str, align, value, &style.Selectable, in)
}

///////////////////////////////////////////////////////////////////////////////////
// SCROLLBARS
///////////////////////////////////////////////////////////////////////////////////

type orientation int

const (
	vertical orientation = iota
	horizontal
)

func scrollbarBehavior(state *types.WidgetStates, in *Input, scroll, cursor, scrollwheel_bounds types.Rect, scroll_offset float64, target float64, scroll_step float64, o orientation) float64 {
	exitstate := basicWidgetStateControl(state, in, cursor)

	if *state == types.WidgetStateActive {
		if !in.Mouse.Down(mouse.ButtonLeft) {
			*state = exitstate
		} else {
			switch o {
			case vertical:
				pixel := in.Mouse.Delta.Y
				delta := (float64(pixel) / float64(scroll.H)) * target
				scroll_offset = clampFloat(0, scroll_offset+delta, target-float64(scroll.H))
			case horizontal:
				pixel := in.Mouse.Delta.X
				delta := (float64(pixel) / float64(scroll.W)) * target
				scroll_offset = clampFloat(0, scroll_offset+delta, target-float64(scroll.W))
			}
		}
	}

	if o == vertical && ((in.Mouse.ScrollDelta < 0) || (in.Mouse.ScrollDelta > 0)) && in.Mouse.HoveringRect(scrollwheel_bounds) {
		/* update cursor by mouse scrolling */
		old_scroll_offset := scroll_offset
		scroll_offset = scroll_offset + scroll_step*float64(-in.Mouse.ScrollDelta)

		if o == vertical {
			scroll_offset = clampFloat(0, scroll_offset, target-float64(scroll.H))
		} else {
			scroll_offset = clampFloat(0, scroll_offset, target-float64(scroll.W))
		}
		used_delta := (scroll_offset - old_scroll_offset) / scroll_step
		residual := float64(in.Mouse.ScrollDelta) + used_delta
		if residual < 0 {
			in.Mouse.ScrollDelta = int(math.Ceil(residual))
		} else {
			in.Mouse.ScrollDelta = int(math.Floor(residual))
		}
	}

	return scroll_offset
}

func doScrollbarv(win *Window, scroll, scrollwheel_bounds types.Rect, offset float64, target float64, step float64, button_pixel_inc float64, style *nstyle.Scrollbar, in *Input, font *types.Face) float64 {
	var cursor types.Rect
	var scroll_step float64
	var scroll_offset float64
	var scroll_off float64
	var scroll_ratio float64

	if scroll.W < 1 {
		scroll.W = 1
	}

	if scroll.H < 2*scroll.W {
		scroll.H = 2 * scroll.W
	}

	if target <= float64(scroll.H) {
		return 0
	}

	/* optional scrollbar buttons */
	if style.ShowButtons {
		var button types.Rect
		button.X = scroll.X
		button.W = scroll.W
		button.H = scroll.W

		scroll_h := float64(scroll.H - 2*button.H)
		scroll_step = minFloat(step, button_pixel_inc)

		/* decrement button */
		button.Y = scroll.Y

		if doButton(win, label.S(style.DecSymbol), button, &style.DecButton, in, true) {
			offset = offset - scroll_step
		}

		/* increment button */
		button.Y = scroll.Y + scroll.H - button.H

		if doButton(win, label.S(style.IncSymbol), button, &style.IncButton, in, true) {
			offset = offset + scroll_step
		}

		scroll.Y = scroll.Y + button.H
		scroll.H = int(scroll_h)
	}

	/* calculate scrollbar constants */
	scroll_step = minFloat(step, float64(scroll.H))

	scroll_offset = clampFloat(0, offset, target-float64(scroll.H))
	scroll_ratio = float64(scroll.H) / target
	scroll_off = scroll_offset / target

	/* calculate scrollbar cursor bounds */
	cursor.H = int(scroll_ratio*float64(scroll.H) - 2)

	cursor.Y = scroll.Y + int(scroll_off*float64(scroll.H)) + 1
	cursor.W = scroll.W - 2
	cursor.X = scroll.X + 1

	/* update scrollbar */
	out := &win.widgets
	state := out.PrevState(scroll)
	scroll_offset = scrollbarBehavior(&state, in, scroll, cursor, scrollwheel_bounds, scroll_offset, target, scroll_step, vertical)

	scroll_off = scroll_offset / target
	cursor.Y = scroll.Y + int(scroll_off*float64(scroll.H))

	out.Add(state, scroll, &drawableScrollbar{state, style, scroll, cursor})

	return scroll_offset
}

func doScrollbarh(win *Window, scroll types.Rect, offset float64, target float64, step float64, button_pixel_inc float64, style *nstyle.Scrollbar, in *Input, font *types.Face) float64 {
	var cursor types.Rect
	var scroll_step float64
	var scroll_offset float64
	var scroll_off float64
	var scroll_ratio float64

	/* scrollbar background */
	if scroll.H < 1 {
		scroll.H = 1
	}

	if scroll.W < 2*scroll.H {
		scroll.Y = 2 * scroll.H
	}

	if target <= float64(scroll.W) {
		return 0
	}

	/* optional scrollbar buttons */
	if style.ShowButtons {
		var scroll_w float64
		var button types.Rect
		button.Y = scroll.Y
		button.W = scroll.H
		button.H = scroll.H

		scroll_w = float64(scroll.W - 2*button.W)
		scroll_step = minFloat(step, button_pixel_inc)

		/* decrement button */
		button.X = scroll.X

		if doButton(win, label.S(style.DecSymbol), button, &style.DecButton, in, true) {
			offset = offset - scroll_step
		}

		/* increment button */
		button.X = scroll.X + scroll.W - button.W

		if doButton(win, label.S(style.IncSymbol), button, &style.IncButton, in, true) {
			offset = offset + scroll_step
		}

		scroll.X = scroll.X + button.W
		scroll.W = int(scroll_w)
	}

	/* calculate scrollbar constants */
	scroll_step = minFloat(step, float64(scroll.W))

	scroll_offset = clampFloat(0, offset, target-float64(scroll.W))
	scroll_ratio = float64(scroll.W) / target
	scroll_off = scroll_offset / target

	/* calculate cursor bounds */
	cursor.W = int(scroll_ratio*float64(scroll.W) - 2)
	cursor.X = scroll.X + int(scroll_off*float64(scroll.W)) + 1
	cursor.H = scroll.H - 2
	cursor.Y = scroll.Y + 1

	/* update scrollbar */
	out := &win.widgets
	state := out.PrevState(scroll)
	scroll_offset = scrollbarBehavior(&state, in, scroll, cursor, types.Rect{0, 0, 0, 0}, scroll_offset, target, scroll_step, horizontal)

	scroll_off = scroll_offset / target
	cursor.X = scroll.X + int(scroll_off*float64(scroll.W))

	out.Add(state, scroll, &drawableScrollbar{state, style, scroll, cursor})

	return scroll_offset
}

///////////////////////////////////////////////////////////////////////////////////
// TOGGLE BOXES
///////////////////////////////////////////////////////////////////////////////////

type toggleType int

const (
	toggleCheck = toggleType(iota)
	toggleOption
)

func toggleBehavior(in *Input, b types.Rect, state *types.WidgetStates, active bool) bool {
	//TODO: rewrite using basicWidgetStateControl
	if in.Mouse.HoveringRect(b) {
		*state = types.WidgetStateHovered
	} else {
		*state = types.WidgetStateInactive
	}
	if *state == types.WidgetStateHovered && in.Mouse.Clicked(mouse.ButtonLeft, b) {
		*state = types.WidgetStateActive
		active = !active
	}

	return active
}

func doToggle(out *widgetBuffer, r types.Rect, active bool, str string, type_ toggleType, style *nstyle.Toggle, in *Input, font *types.Face) bool {
	var bounds types.Rect
	var select_ types.Rect
	var cursor types.Rect
	var label types.Rect
	var cursor_pad int

	r.W = max(r.W, fontHeight(font)+2*style.Padding.X)
	r.H = max(r.H, fontHeight(font)+2*style.Padding.Y)

	/* add additional touch padding for touch screen devices */
	bounds.X = r.X - style.TouchPadding.X
	bounds.Y = r.Y - style.TouchPadding.Y
	bounds.W = r.W + 2*style.TouchPadding.X
	bounds.H = r.H + 2*style.TouchPadding.Y

	/* calculate the selector space */
	select_.W = min(r.H, fontHeight(font)+style.Padding.Y)

	select_.H = select_.W
	select_.X = r.X + style.Padding.X
	select_.Y = (r.Y + style.Padding.Y + (select_.W / 2)) - (fontHeight(font) / 2)
	if type_ == toggleOption {
		cursor_pad = select_.W / 4
	} else {
		cursor_pad = select_.H / 6
	}

	/* calculate the bounds of the cursor inside the selector */
	select_.H = max(select_.W, cursor_pad*2)

	cursor.H = select_.H - cursor_pad*2
	cursor.W = cursor.H
	cursor.X = select_.X + cursor_pad
	cursor.Y = select_.Y + cursor_pad

	/* label behind the selector */
	label.X = r.X + select_.W + style.Padding.X*2
	label.Y = select_.Y
	label.W = max(r.X+r.W, label.X+style.Padding.X)
	label.W -= (label.X + style.Padding.X)
	label.H = select_.W

	/* update selector */
	state := out.PrevState(bounds)
	active = toggleBehavior(in, bounds, &state, active)

	out.Add(state, r, &drawableTogglebox{type_, state, style, active, label, select_, cursor, str})

	return active
}

// OptionText adds a radio button to win. If is_active is true the
// radio button will be drawn selected. Returns true when the button
// is clicked once.
// You are responsible for ensuring that only one radio button is selected at once.
func (win *Window) OptionText(text string, is_active bool) bool {
	style := &win.ctx.Style
	state, bounds := win.widget()
	if state == 0 {
		return false
	}
	in := win.inputMaybe(state)
	is_active = doToggle(&win.widgets, bounds, is_active, text, toggleOption, &style.Option, in, style.Font)
	return is_active
}

// CheckboxText adds a checkbox button to win. Active will contain
// the checkbox value.
// Returns true when value changes.
func (win *Window) CheckboxText(text string, active *bool) bool {
	state, bounds := win.widget()
	if state == 0 {
		return false
	}
	in := win.inputMaybe(state)
	old_active := *active
	*active = doToggle(&win.widgets, bounds, *active, text, toggleCheck, &win.ctx.Style.Checkbox, in, win.ctx.Style.Font)
	return *active != old_active
}

///////////////////////////////////////////////////////////////////////////////////
// SLIDER
///////////////////////////////////////////////////////////////////////////////////

func sliderBehavior(state *types.WidgetStates, cursor *types.Rect, in *Input, style *nstyle.Slider, bounds types.Rect, slider_min float64, slider_value float64, slider_max float64, slider_step float64, slider_steps int) float64 {
	exitstate := basicWidgetStateControl(state, in, bounds)

	if *state == types.WidgetStateActive {
		if !in.Mouse.Down(mouse.ButtonLeft) {
			*state = exitstate
		} else {
			d := in.Mouse.Pos.X - (cursor.X + cursor.W/2.0)
			var pxstep float64 = float64(bounds.W-(2*style.Padding.X)) / float64(slider_steps)

			if math.Abs(float64(d)) >= pxstep {
				steps := float64(int(math.Abs(float64(d)) / pxstep))
				if d > 0 {
					slider_value += slider_step * steps
				} else {
					slider_value -= slider_step * steps
				}
				slider_value = clampFloat(slider_min, slider_value, slider_max)
			}
		}
	}

	return slider_value
}

func doSlider(win *Window, bounds types.Rect, minval float64, val float64, maxval float64, step float64, style *nstyle.Slider, in *Input) float64 {
	var slider_range float64
	var cursor_offset float64
	var cursor types.Rect

	/* remove padding from slider bounds */
	bounds.X = bounds.X + style.Padding.X

	bounds.Y = bounds.Y + style.Padding.Y
	bounds.H = max(bounds.H, 2*style.Padding.Y)
	bounds.W = max(bounds.W, 1+bounds.H+2*style.Padding.X)
	bounds.H -= 2 * style.Padding.Y
	bounds.W -= 2 * style.Padding.Y

	/* optional buttons */
	if style.ShowButtons {
		var button types.Rect
		button.Y = bounds.Y
		button.W = bounds.H
		button.H = bounds.H

		/* decrement button */
		button.X = bounds.X

		if doButton(win, label.S(style.DecSymbol), button, &style.DecButton, in, false) {
			val -= step
		}

		/* increment button */
		button.X = (bounds.X + bounds.W) - button.W

		if doButton(win, label.S(style.IncSymbol), button, &style.IncButton, in, false) {
			val += step
		}

		bounds.X = bounds.X + button.W + style.Spacing.X
		bounds.W = bounds.W - (2*button.W + 2*style.Spacing.X)
	}

	/* make sure the provided values are correct */
	slider_value := clampFloat(minval, val, maxval)
	slider_range = maxval - minval
	slider_steps := int(slider_range / step)

	/* calculate slider virtual cursor bounds */
	cursor_offset = (slider_value - minval) / step

	cursor.H = bounds.H
	cursor.W = bounds.W / (slider_steps + 1)
	cursor.X = bounds.X + int((float64(cursor.W) * cursor_offset))
	cursor.Y = bounds.Y

	out := &win.widgets
	state := out.PrevState(bounds)
	slider_value = sliderBehavior(&state, &cursor, in, style, bounds, minval, slider_value, maxval, step, slider_steps)
	out.Add(state, bounds, &drawableSlider{state, style, bounds, cursor, minval, slider_value, maxval})
	return slider_value
}

// Adds a slider with a floating point value to win.
// Returns true when the slider's value is changed.
func (win *Window) SliderFloat(min_value float64, value *float64, max_value float64, value_step float64) bool {
	style := &win.ctx.Style
	state, bounds := win.widget()
	if state == 0 {
		return false
	}
	in := win.inputMaybe(state)

	old_value := *value
	*value = doSlider(win, bounds, min_value, old_value, max_value, value_step, &style.Slider, in)
	return old_value > *value || old_value < *value
}

// Adds a slider with an integer value to win.
// Returns true when the slider's value changes.
func (win *Window) SliderInt(min int, val *int, max int, step int) bool {
	value := float64(*val)
	ret := win.SliderFloat(float64(min), &value, float64(max), float64(step))
	*val = int(value)
	return ret
}

///////////////////////////////////////////////////////////////////////////////////
// PROGRESS BAR
///////////////////////////////////////////////////////////////////////////////////

func progressBehavior(state *types.WidgetStates, in *Input, r types.Rect, maxval int, value int, modifiable bool) int {
	if !modifiable {
		*state = types.WidgetStateInactive
		return value
	}

	exitstate := basicWidgetStateControl(state, in, r)

	if *state == types.WidgetStateActive {
		if !in.Mouse.Down(mouse.ButtonLeft) {
			*state = exitstate
		} else {
			ratio := maxFloat(0, float64(in.Mouse.Pos.X-r.X)) / float64(r.W)
			value = int(float64(maxval) * ratio)
			if value < 0 {
				value = 0
			}
		}
	}

	if maxval > 0 && value > maxval {
		value = maxval
	}
	return value
}

func doProgress(out *widgetBuffer, bounds types.Rect, value int, maxval int, modifiable bool, style *nstyle.Progress, in *Input) int {
	var prog_scale float64
	var cursor types.Rect

	/* calculate progressbar cursor */
	cursor = padRect(bounds, style.Padding)
	prog_scale = float64(value) / float64(maxval)
	cursor.W = int(float64(cursor.W) * prog_scale)

	/* update progressbar */
	if value > maxval {
		value = maxval
	}

	state := out.PrevState(bounds)
	value = progressBehavior(&state, in, bounds, maxval, value, modifiable)
	out.Add(state, bounds, &drawableProgress{state, style, bounds, cursor, value, maxval})

	return value
}

// Adds a progress bar to win. if is_modifiable is true the progress
// bar will be user modifiable through click-and-drag.
// Returns true when the progress bar values is modified.
func (win *Window) Progress(cur *int, maxval int, is_modifiable bool) bool {
	style := &win.ctx.Style
	state, bounds := win.widget()
	if state == 0 {
		return false
	}

	in := win.inputMaybe(state)
	old_value := *cur
	*cur = doProgress(&win.widgets, bounds, *cur, maxval, is_modifiable, &style.Progress, in)
	return *cur != old_value
}

///////////////////////////////////////////////////////////////////////////////////
// PROPERTY
///////////////////////////////////////////////////////////////////////////////////

type FilterFunc func(c rune) bool

func FilterDefault(c rune) bool {
	return true
}

func FilterDecimal(c rune) bool {
	return !((c < '0' || c > '9') && c != '-')
}

func FilterFloat(c rune) bool {
	return !((c < '0' || c > '9') && c != '.' && c != '-')
}

func (ed *TextEditor) propertyBehavior(ws *types.WidgetStates, in *Input, property types.Rect, label types.Rect, edit types.Rect, empty types.Rect) (drag bool, delta int) {
	if ed.propertyStatus == propertyDefault {
		if buttonBehaviorDo(ws, edit, in, false) {
			ed.propertyStatus = propertyEdit
		} else if in.Mouse.IsClickDownInRect(mouse.ButtonLeft, label, true) {
			ed.propertyStatus = propertyDrag
		} else if in.Mouse.IsClickDownInRect(mouse.ButtonLeft, empty, true) {
			ed.propertyStatus = propertyDrag
		}
	}

	if ed.propertyStatus == propertyDrag {
		if in.Mouse.Released(mouse.ButtonLeft) {
			ed.propertyStatus = propertyDefault
		} else {
			delta = in.Mouse.Delta.X
			drag = true
		}
	}

	if ed.propertyStatus == propertyDefault {
		if in.Mouse.HoveringRect(property) {
			*ws = types.WidgetStateHovered
		} else {
			*ws = types.WidgetStateInactive
		}
	} else {
		*ws = types.WidgetStateActive
	}

	return
}

type doPropertyRet int

const (
	doPropertyStay = doPropertyRet(iota)
	doPropertyInc
	doPropertyDec
	doPropertyDrag
	doPropertySet
)

func (win *Window) doProperty(property types.Rect, name string, text string, filter FilterFunc, in *Input) (ret doPropertyRet, delta int, ed *TextEditor) {
	ret = doPropertyStay
	style := &win.ctx.Style.Property
	font := win.ctx.Style.Font

	// left decrement button
	var left types.Rect
	left.H = fontHeight(font) / 2
	left.W = left.H
	left.X = property.X + style.Border + style.Padding.X
	left.Y = property.Y + style.Border + property.H/2.0 - left.H/2

	// text label
	size := FontWidth(font, name)
	var lblrect types.Rect
	lblrect.X = left.X + left.W + style.Padding.X
	lblrect.W = size + 2*style.Padding.X
	lblrect.Y = property.Y + style.Border
	lblrect.H = property.H - 2*style.Border

	/* right increment button */
	var right types.Rect
	right.Y = left.Y
	right.W = left.W
	right.H = left.H
	right.X = property.X + property.W - (right.W + style.Padding.X)

	ws := win.widgets.PrevState(property)
	oldws := ws
	if ws == types.WidgetStateActive {
		ed = win.editor
	} else {
		ed = &TextEditor{}
		ed.init(win)
		ed.Buffer = []rune(text)
	}

	size = FontWidth(font, string(ed.Buffer)) + style.Edit.CursorSize

	/* edit */
	var edit types.Rect
	edit.W = size + 2*style.Padding.X
	edit.X = right.X - (edit.W + style.Padding.X)
	edit.Y = property.Y + style.Border + 1
	edit.H = property.H - (2*style.Border + 2)

	/* empty left space activator */
	var empty types.Rect
	empty.W = edit.X - (lblrect.X + lblrect.W)
	empty.X = lblrect.X + lblrect.W
	empty.Y = property.Y
	empty.H = property.H

	/* update property */
	old := ed.propertyStatus == propertyEdit

	drag, delta := ed.propertyBehavior(&ws, in, property, lblrect, edit, empty)
	if drag {
		ret = doPropertyDrag
	}
	if ws == types.WidgetStateActive {
		ed.Active = true
		win.editor = ed
	} else if oldws == types.WidgetStateActive {
		win.editor = nil
	}
	ed.win.widgets.Add(ws, property, &drawableProperty{style, property, lblrect, ws, name})

	/* execute right and left button  */
	if doButton(ed.win, label.S(style.SymLeft), left, &style.DecButton, in, false) {
		ret = doPropertyDec
	}
	if doButton(ed.win, label.S(style.SymRight), right, &style.IncButton, in, false) {
		ret = doPropertyInc
	}

	active := ed.propertyStatus == propertyEdit
	if !old && active {
		/* property has been activated so setup buffer */
		ed.Cursor = len(ed.Buffer)
	}
	ed.Flags = EditAlwaysInsertMode | EditNoHorizontalScroll
	ed.doEdit(edit, &style.Edit, in)
	active = ed.Active

	if active && in.Keyboard.Pressed(key.CodeReturnEnter) {
		active = !active
	}

	if old && !active {
		/* property is now not active so convert edit text to value*/
		ed.propertyStatus = propertyDefault
		ed.Active = false
		ret = doPropertySet
	}

	return
}

// Adds a property widget to win for floating point properties.
// A property widget will display a text label, a small text editor
// for the property value and one up and one down button.
// The value can be modified by editing the text value, by clicking
// the up/down buttons (which will increase/decrease the value by step)
// or by clicking and dragging over the label.
// Returns true when the property's value is changed
func (win *Window) PropertyFloat(name string, min float64, val *float64, max, step, inc_per_pixel float64, prec int) (changed bool) {
	s, bounds := win.widget()
	if s == 0 {
		return
	}
	in := win.inputMaybe(s)
	text := strconv.FormatFloat(*val, 'G', prec, 32)
	ret, delta, ed := win.doProperty(bounds, name, text, FilterFloat, in)
	switch ret {
	case doPropertyDec:
		*val -= step
	case doPropertyInc:
		*val += step
	case doPropertyDrag:
		*val += float64(delta) * inc_per_pixel
	case doPropertySet:
		*val, _ = strconv.ParseFloat(string(ed.Buffer), 64)
	}
	changed = ret != doPropertyStay
	if changed {
		*val = clampFloat(min, *val, max)
	}
	return
}

// Same as PropertyFloat but with integer values.
func (win *Window) PropertyInt(name string, min int, val *int, max, step, inc_per_pixel int) (changed bool) {
	s, bounds := win.widget()
	if s == 0 {
		return
	}
	in := win.inputMaybe(s)
	text := strconv.Itoa(*val)
	ret, delta, ed := win.doProperty(bounds, name, text, FilterDecimal, in)
	switch ret {
	case doPropertyDec:
		*val -= step
	case doPropertyInc:
		*val += step
	case doPropertyDrag:
		*val += delta * inc_per_pixel
	case doPropertySet:
		*val, _ = strconv.Atoi(string(ed.Buffer))
	}
	changed = ret != doPropertyStay
	if changed {
		*val = clampInt(min, *val, max)
	}
	return
}

///////////////////////////////////////////////////////////////////////////////////
// POPUP
///////////////////////////////////////////////////////////////////////////////////

func (ctx *context) nonblockOpen(flags WindowFlags, body types.Rect, header types.Rect, updateFn UpdateFn) {
	popup := createWindow(ctx, "")
	popup.idx = len(ctx.Windows)
	popup.updateFn = updateFn
	popup.cmds.UseClipping = true
	ctx.Windows = append(ctx.Windows, popup)

	popup.Bounds = body
	popup.layout = &panel{}
	popup.flags = flags
	popup.flags |= WindowBorder | windowPopup
	popup.flags |= WindowDynamic | windowSub
	popup.flags |= windowNonblock

	popup.header = header
}

// Opens a popup window inside win. Will return true until the
// popup window is closed.
// The contents of the popup window will be updated by updateFn
func (mw *MasterWindow) PopupOpen(title string, flags WindowFlags, rect types.Rect, scale bool, updateFn UpdateFn) {
	go func() {
		mw.uilock.Lock()
		defer mw.uilock.Unlock()
		mw.ctx.popupOpen(title, flags, rect, scale, updateFn)
	}()
}

func (ctx *context) popupOpen(title string, flags WindowFlags, rect types.Rect, scale bool, updateFn UpdateFn) {
	popup := createWindow(ctx, title)
	popup.idx = len(ctx.Windows)
	popup.updateFn = updateFn
	ctx.Windows = append(ctx.Windows, popup)
	popup.cmds.UseClipping = true

	if scale {
		rect.X = ctx.scale(rect.X)
		rect.Y = ctx.scale(rect.Y)
		rect.W = ctx.scale(rect.W)
		rect.H = ctx.scale(rect.H)
	}

	rect.X += ctx.Windows[0].layout.Clip.X
	rect.Y += ctx.Windows[0].layout.Clip.Y

	popup.Bounds = rect
	popup.layout = &panel{}
	popup.flags = flags | WindowBorder | windowSub | windowPopup
}

// Programmatically closes this window
func (win *Window) Close() {
	if win.idx != 0 {
		win.close = true
	}
}

///////////////////////////////////////////////////////////////////////////////////
// CONTEXTUAL
///////////////////////////////////////////////////////////////////////////////////

// Opens a contextual menu with maximum size equal to 'size'.
func (win *Window) ContextualOpen(flags WindowFlags, size image.Point, trigger_bounds types.Rect, updateFn UpdateFn) {
	size.X = win.ctx.scale(size.X)
	size.Y = win.ctx.scale(size.Y)
	if !win.Input().Mouse.Clicked(mouse.ButtonRight, trigger_bounds) {
		return
	}

	var body types.Rect
	body.X = win.ctx.Input.Mouse.Pos.X
	body.Y = win.ctx.Input.Mouse.Pos.Y
	body.W = size.X
	body.H = size.Y

	atomic.AddInt32(&win.ctx.changed, 1)
	win.ctx.nonblockOpen(flags|windowContextual|WindowNoScrollbar, body, types.Rect{}, updateFn)
	popup := win.ctx.Windows[len(win.ctx.Windows)-1]
	popup.triggerBounds = trigger_bounds
}

// MenuItem adds a menu item
func (win *Window) MenuItem(lbl label.Label) bool {
	style := &win.ctx.Style
	state, bounds := win.widgetFitting(style.ContextualButton.Padding)
	if state == 0 {
		return false
	}

	in := win.inputMaybe(state)
	if doButton(win, lbl, bounds, &style.ContextualButton, in, false) {
		win.Close()
		return true
	}

	return false
}

///////////////////////////////////////////////////////////////////////////////////
// TOOLTIP
///////////////////////////////////////////////////////////////////////////////////

const tooltipWindowTitle = "__##Tooltip##__"

// Displays a tooltip window.
func (win *Window) TooltipOpen(width int, scale bool, updateFn UpdateFn) {
	in := &win.ctx.Input

	if scale {
		width = win.ctx.scale(width)
	}

	var bounds types.Rect
	bounds.W = width
	bounds.H = nk_null_rect.H
	bounds.X = (in.Mouse.Pos.X + 1) - win.layout.Clip.X
	bounds.Y = (in.Mouse.Pos.Y + 1) - win.layout.Clip.Y

	win.ctx.popupOpen(tooltipWindowTitle, WindowDynamic|WindowNoScrollbar|windowTooltip, bounds, false, updateFn)
}

// Shows a tooltip window containing the specified text.
func (win *Window) Tooltip(text string) {
	if text == "" {
		return
	}

	/* fetch configuration data */
	padding := win.ctx.Style.TooltipWindow.Padding
	item_spacing := win.ctx.Style.TooltipWindow.Spacing

	/* calculate size of the text and tooltip */
	text_width := FontWidth(win.ctx.Style.Font, text)
	text_height := fontHeight(win.ctx.Style.Font)
	text_width += win.ctx.scale(2*padding.X) + win.ctx.scale(2*item_spacing.X)

	win.TooltipOpen(text_width, false, func(mw *MasterWindow, tw *Window) {
		tw.LayoutRowDynamicScaled(text_height, 1)
		tw.Label(text, "LC")
	})
}

///////////////////////////////////////////////////////////////////////////////////
// COMBO-BOX
///////////////////////////////////////////////////////////////////////////////////

func (ctx *context) comboOpen(height int, is_clicked bool, header types.Rect, updateFn UpdateFn) {
	height = ctx.scale(height)

	if !is_clicked {
		return
	}

	var body types.Rect
	body.X = header.X
	body.W = header.W
	body.Y = header.Y + header.H - 1
	body.H = height

	ctx.nonblockOpen(windowCombo, body, types.Rect{0, 0, 0, 0}, updateFn)
}

// Adds a drop-down list to win.
func (win *Window) Combo(lbl label.Label, height int, updateFn UpdateFn) {
	var is_active bool = false

	s, header := win.widget()
	if s == widgetInvalid {
		return
	}

	in := win.inputMaybe(s)
	state := win.widgets.PrevState(header)
	if buttonBehaviorDo(&state, header, in, false) {
		is_active = true
	}

	switch lbl.Kind {
	case label.ColorLabel:
		win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Color: lbl.Color, Fn: drawableComboColor})
	case label.ImageLabel:
		win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Image: lbl.Img, Fn: drawableComboImage})
	case label.ImageTextLabel:
		win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Image: lbl.Img, Selected: lbl.Text, Fn: drawableComboImageText})
	case label.SymbolLabel:
		win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Symbol: lbl.Symbol, Fn: drawableComboSymbol})
	case label.SymbolTextLabel:
		win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Symbol: lbl.Symbol, Selected: lbl.Text, Fn: drawableComboSymbolText})
	case label.TextLabel:
		win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Selected: lbl.Text, Fn: drawableComboText})
	}

	win.ctx.comboOpen(height, is_active, header, updateFn)
}

// Adds a drop-down list to win. The contents are specified by items,
// with selected being the index of the selected item.
func (win *Window) ComboSimple(items []string, selected *int, item_height int) {
	if len(items) == 0 {
		return
	}

	item_padding := win.ctx.Style.Combo.ButtonPadding.Y
	window_padding := win.style().Padding.Y
	max_height := (len(items)+1)*item_height + item_padding*3 + window_padding*2
	win.Combo(label.T(items[*selected]), max_height, func(mw *MasterWindow, w *Window) {
		w.LayoutRowDynamic(item_height, 1)
		for i := range items {
			if w.MenuItem(label.TA(items[i], "LC")) {
				*selected = i
			}
		}
	})
}

///////////////////////////////////////////////////////////////////////////////////
// MENU
///////////////////////////////////////////////////////////////////////////////////

func (win *Window) menuOpen(is_clicked bool, header types.Rect, width int, updateFn UpdateFn) {
	width = win.ctx.scale(width)

	if !is_clicked {
		return
	}

	var body types.Rect
	body.X = header.X
	body.W = width
	body.Y = header.Y + header.H
	body.H = (win.layout.Bounds.Y + win.layout.Bounds.H) - body.Y

	win.ctx.nonblockOpen(windowMenu|WindowNoScrollbar, body, header, updateFn)
}

// Adds a menu to win with a text label.
func (win *Window) Menu(lbl label.Label, width int, updateFn UpdateFn) {
	state, header := win.widget()
	if state == 0 {
		return
	}
	in := &Input{}
	if !(state == widgetRom || !win.toplevel()) {
		in = &win.ctx.Input
	}
	is_clicked := doButton(win, lbl, header, &win.ctx.Style.MenuButton, in, false)
	win.menuOpen(is_clicked, header, width, updateFn)
}

///////////////////////////////////////////////////////////////////////////////////
// GROUPS
///////////////////////////////////////////////////////////////////////////////////

// Creates a group of widgets.
// Group are useful for creating lists as well as splitting a main
// window into tiled subwindows.
// Items that you want to add to the group should be added to the
// returned window.
func (win *Window) GroupBegin(title string, flags WindowFlags) *Window {
	sw := win.groupWnd[title]
	if sw == nil {
		sw = createWindow(win.ctx, title)
		sw.parent = win
		win.groupWnd[title] = sw
		sw.Scrollbar.X = 0
		sw.Scrollbar.Y = 0
		sw.layout = &panel{}
	}

	sw.curNode = sw.rootNode
	sw.widgets.reset()
	sw.cmds.Reset()
	sw.idx = win.idx

	sw.widgets.Clip = win.widgets.Clip
	sw.cmds.Clip = win.cmds.Clip

	state, bounds := win.widget()
	if state == 0 {
		return nil
	}

	flags |= windowSub | windowGroup

	sw.Bounds = bounds
	sw.flags = flags

	panelBegin(win.ctx, sw, title)

	sw.layout.Offset = &sw.Scrollbar

	return sw
}

// Signals that you are done adding widgets to a group.
func (win *Window) GroupEnd() {
	panelEnd(win.ctx, win)
	win.parent.widgets.prev = append(win.parent.widgets.prev, win.widgets.prev...)
	win.parent.widgets.cur = append(win.parent.widgets.cur, win.widgets.cur...)

	// immediate drawing
	win.parent.cmds.Commands = append(win.parent.cmds.Commands, win.cmds.Commands...)
}
