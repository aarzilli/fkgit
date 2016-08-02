package nucular

import (
	"errors"
	"image"
	"image/color"
	"math"
	"strconv"
	"sync/atomic"

	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
)

type WidgetStates int

const (
	WidgetStateInactive WidgetStates = iota
	WidgetStateHovered
	WidgetStateActive
)

///////////////////////////////////////////////////////////////////////////////////
// CONTEXT & PANELS
///////////////////////////////////////////////////////////////////////////////////

type context struct {
	Input   Input
	Style   Style
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
	Bounds       Rect
	Scrollbar    image.Point
	cmds         CommandBuffer
	widgets      widgetBuffer
	layout       *panel
	close, first bool
	// trigger rectangle of nonblocking windows
	triggerBounds, header Rect
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
	Flags   WindowFlags
	Bounds  Rect
	Offset  *image.Point
	AtX     int
	AtY     int
	MaxX    int
	Width   int
	Height  int
	FooterH int
	HeaderH int
	Border  int
	Clip    Rect
	Menu    menuState
	Row     rowLayout
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
	Item       Rect
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

func contextAllCommands(ctx *context) (nwidgets int, r []Command) {
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
		w.cmds.reset()
	}

	ctx.Windows[0].cmds.Clip = nk_null_rect
	ctx.Windows[0].layout = layout
	panelBegin(ctx, ctx.Windows[0], "")
	layout.Offset = &ctx.Windows[0].Scrollbar
}

func contextEnd(ctx *context) {
	panelEnd(ctx, ctx.Windows[0])
}

func (win *Window) style() *StyleWindow {
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
		var move Rect
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

		incursor := InputIsMousePrevHoveringRect(in, move)
		if InputIsMouseDown(in, mouse.ButtonLeft) && incursor {
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

		dwh.Hovered = InputIsMouseHoveringRect(&ctx.Input, dwh.Header)

		header := dwh.Header

		/* window header title */
		t := fontWidth(font, title)

		dwh.Label.X = header.X + wstyle.Header.Padding.X
		dwh.Label.X += wstyle.Header.LabelPadding.X
		dwh.Label.Y = header.Y + wstyle.Header.LabelPadding.Y
		dwh.Label.H = fontHeight(font) + 2*wstyle.Header.LabelPadding.Y
		dwh.Label.W = t + 2*wstyle.Header.Spacing.X
		dwh.LayoutHeaderH = layout.HeaderH
		dwh.RowHeight = layout.Row.Height
		dwh.Title = title

		win.widgets.Add(WidgetStateInactive, layout.Bounds, &dwh)

		var button Rect
		/* window close button */
		button.Y = header.Y + wstyle.Header.Padding.Y
		button.H = layout.HeaderH - 2*wstyle.Header.Padding.Y
		button.W = button.H
		if win.flags&WindowClosable != 0 {
			if wstyle.Header.Align == HeaderRight {
				button.X = (header.W + header.X) - (button.W + wstyle.Header.Padding.X)
				header.W -= button.W + wstyle.Header.Spacing.X + wstyle.Header.Padding.X
			} else {
				button.X = header.X + wstyle.Header.Padding.X
				header.X += button.W + wstyle.Header.Spacing.X + wstyle.Header.Padding.X
			}

			if doButtonSymbol(&win.widgets, button, wstyle.Header.CloseSymbol, ButtonNormal, &wstyle.Header.CloseButton, in) {
				layout.Flags |= windowHidden
			}
		}

		/* window minimize button */
		if win.flags&WindowMinimizable != 0 {
			if wstyle.Header.Align == HeaderRight {
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

			var symbolType SymbolType
			if layout.Flags&windowMinimized != 0 {
				symbolType = wstyle.Header.MaximizeSymbol
			} else {
				symbolType = wstyle.Header.MinimizeSymbol
			}
			if doButtonSymbol(&win.widgets, button, symbolType, ButtonNormal, &wstyle.Header.MinimizeButton, in) {
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
		win.widgets.Add(WidgetStateInactive, layout.Bounds, &dwh)
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
	win.widgets.Add(WidgetStateInactive, dwb.Bounds, &dwb)

	return layout.Flags&windowHidden == 0 && layout.Flags&windowMinimized == 0
}

func panelEnd(ctx *context, window *Window) {
	var footer = Rect{0, 0, 0, 0}

	layout := window.layout
	style := &ctx.Style
	var in *Input
	if window.toplevel() {
		in = &ctx.Input
	}
	outclip := nk_null_rect
	if window.flags&windowGroup != 0 {
		outclip = window.parent.widgets.Clip
	}
	window.widgets.Clip = outclip
	window.widgets.Add(WidgetStateInactive, outclip, &drawableScissor{outclip})

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
			var bounds Rect
			bounds.X = window.Bounds.X
			bounds.Y = layout.AtY - item_spacing.Y
			bounds.W = window.Bounds.W
			bounds.H = window.Bounds.Y + layout.Height + item_spacing.Y + window.style().Padding.Y - bounds.Y

			window.widgets.Add(WidgetStateInactive, bounds, &drawableFillRect{bounds, wstyle.Background})
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
				var bounds Rect
				bounds.X = layout.Bounds.X + layout.Width
				bounds.Y = layout.Clip.Y
				bounds.W = scrollbar_size.X
				bounds.H = layout.Height

				window.widgets.Add(WidgetStateInactive, bounds, &drawableFillRect{bounds, wstyle.Background})
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
			window.widgets.Add(WidgetStateInactive, footer, &drawableFillRect{footer, wstyle.Background})

			if layout.Flags&windowCombo == 0 && layout.Flags&windowMenu == 0 {
				/* fill empty scrollbar space */
				var bounds Rect
				bounds.X = layout.Bounds.X
				bounds.Y = window.Bounds.Y + layout.Height
				bounds.W = layout.Bounds.W
				bounds.H = layout.Row.Height
				window.widgets.Add(WidgetStateInactive, bounds, &drawableFillRect{bounds, wstyle.Background})
			}
		}
	}

	/* scrollbars */
	if layout.Flags&WindowNoScrollbar == 0 && layout.Flags&windowMinimized == 0 {
		var bounds Rect
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
			scroll_offset = doScrollbarv(&window.widgets, bounds, layout.Bounds, scroll_offset, scroll_target, scroll_step, scroll_inc, &ctx.Style.Scrollv, in, style.Font)
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
			scroll_offset = doScrollbarh(&window.widgets, bounds, scroll_offset, scroll_target, scroll_step, scroll_inc, &ctx.Style.Scrollh, in, style.Font)
			layout.Offset.X = int(scroll_offset)
		}
	}

	var dsab drawableScalerAndBorders
	dsab.Style = window.style()
	dsab.Bounds = window.Bounds
	dsab.Border = layout.Border
	dsab.HeaderH = layout.HeaderH

	/* scaler */
	if (layout.Flags&WindowScalable != 0) && in != nil && layout.Flags&windowMinimized == 0 {
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

			if InputIsMouseDown(in, mouse.ButtonLeft) && incursor {
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

	window.widgets.Add(WidgetStateInactive, dsab.Bounds, &dsab)

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
	win.widgets.Add(WidgetStateInactive, layout.Clip, &drawableScissor{layout.Clip})
}

type widgetLayoutStates int

const (
	widgetInvalid = widgetLayoutStates(iota)
	widgetValid
	widgetRom
)

func (win *Window) widget() (widgetLayoutStates, Rect) {
	var c *Rect = nil

	var bounds Rect

	/* allocate space  and check if the widget needs to be updated and drawn */
	panelAllocSpace(&bounds, win)

	c = &win.layout.Clip
	if !c.Intersect(&bounds) {
		return widgetInvalid, bounds
	}

	contains := func(r *Rect, b *Rect) bool {
		return b.Contains(image.Point{r.X, r.Y}) && b.Contains(image.Point{r.X + r.W, r.Y + r.H})
	}

	if !contains(&bounds, c) {
		return widgetRom, bounds
	}
	return widgetValid, bounds
}

func (win *Window) widgetFitting(item_padding image.Point) (state widgetLayoutStates, bounds Rect) {
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

	if layout.Row.Index == layout.Row.Columns {
		bounds.W += style.Padding.X
	} else {
		bounds.W += item_padding.X
	}
	return state, bounds
}

func panelAllocSpace(bounds *Rect, win *Window) {
	/* check if the end of the row has been hit and begin new row if so */
	layout := win.layout
	if layout.Row.Index >= layout.Row.Columns {
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

	style := win.style()
	item_spacing := style.Spacing
	panel_padding := style.Padding

	/* update the current row and set the current row layout */
	layout.Row.Index = 0

	layout.AtY += layout.Row.Height
	layout.Row.Columns = cols
	layout.Row.Height = height + item_spacing.Y
	layout.Row.ItemOffset = 0
	if layout.Flags&WindowDynamic != 0 {
		var drect drawableFillRect
		drect.R = Rect{layout.Bounds.X, layout.AtY, layout.Bounds.W, height + panel_padding.Y}
		drect.C = style.Background
		win.widgets.Add(WidgetStateInactive, drect.R, &drect)
	}
}

const (
	layoutDynamicFixed = 0
	layoutDynamicFree  = 2
	layoutDynamic      = 3
	layoutStaticFixed  = 4
	layoutStaticFree   = 6
	layoutStatic       = 7
)

func layoutWidgetSpace(bounds *Rect, ctx *context, win *Window, modify bool) {
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
	case layoutStaticFixed:
		/* non-scaling fixed widgets item width */
		item_width = int(layout.Row.ItemWidth)

		item_offset = layout.Row.Index * item_width
		item_spacing = layout.Row.Index * spacing.X
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

		item_width = layout.Row.WidthArr[layout.Row.Index]
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

func rowLayoutCtr(win *Window, dynamic bool, height int, cols int, width int, scale bool) {
	/* update the current row and set the current row layout */
	if scale {
		height = win.ctx.scale(height)
		width = win.ctx.scale(width)
	}
	panelLayout(win.ctx, win, height, cols)
	if dynamic {
		win.layout.Row.Type = layoutDynamicFixed
	} else {
		win.layout.Row.Type = layoutStaticFixed
	}

	win.layout.Row.ItemWidth = width
	win.layout.Row.ItemRatio = 0.0
	win.layout.Row.Ratio = nil
	win.layout.Row.ItemOffset = 0
	win.layout.Row.Filled = 0
}

// Starts new row that has cols columns of equal width that automatically
// resize to fill the available space.
func (win *Window) LayoutRowDynamic(height int, cols int) {
	rowLayoutCtr(win, true, height, cols, 0, true)
}

// Like LayoutRowDynamic but height is specified in scaled units.
func (win *Window) LayoutRowDynamicScaled(height int, cols int) {
	rowLayoutCtr(win, true, height, cols, 0, false)
}

// Starts new row that has cols columns each item_width wide.
func (win *Window) LayoutRowStatic(height int, item_width int, cols int) {
	rowLayoutCtr(win, false, height, cols, item_width, true)
}

// Like LayoutRowStatic but height and item_width are specified in scaled units.
func (win *Window) LayoutRowStaticScaled(height int, item_width int, cols int) {
	rowLayoutCtr(win, false, height, cols, item_width, false)
}

func (win *Window) LayoutRowRatio(height int, ratio ...float64) {
	win.LayoutRowRatioScaled(win.ctx.scale(height), ratio...)
}

// Starts new row with a fixed number of columns of width proportional
// to the size of the window.
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

func (win *Window) LayoutRowFixedScaled(height int, width ...int) {
	layout := win.layout
	panelLayout(win.ctx, win, height, len(width))

	for i := range width {
		width[i] = width[i]
	}

	layout.Row.WidthArr = width
	layout.Row.Type = layoutStatic
	layout.Row.ItemWidth = 0
	layout.Row.ItemRatio = 0.0
	layout.Row.ItemOffset = 0
	layout.Row.ItemOffset = 0
	layout.Row.Filled = 0
}

// Starts new row with a fixed number of columns with the specfieid widths.
func (win *Window) LayoutRowFixed(height int, width ...int) {
	for i := range width {
		width[i] = win.ctx.scale(width[i])
	}

	win.LayoutRowFixedScaled(win.ctx.scale(height), width...)
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

func (win *Window) LayoutSpaceEnd() {
	layout := win.layout
	layout.Row.ItemWidth = 0
	layout.Row.ItemRatio = 0.0
	layout.Row.ItemHeight = 0
	layout.Row.ItemOffset = 0
	layout.Row.Item = Rect{}
}

var WrongLayoutErr = errors.New("Command not available with current layout")

// Sets position and size of the next widgets in a Space row layout
func (win *Window) LayoutSpacePush(rect Rect) {
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

func (win *Window) layoutPeek(bounds *Rect) {
	layout := win.layout
	y := layout.AtY
	index := layout.Row.Index
	if layout.Row.Index >= layout.Row.Columns {
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
func (win *Window) WidgetBounds() Rect {
	var bounds Rect
	win.layoutPeek(&bounds)
	return bounds
}

// Returns remaining available height of win in scaled units.
func (win *Window) LayoutAvailableHeight() int {
	return win.layout.Clip.H - (win.layout.AtY - win.layout.Bounds.Y) - win.style().Spacing.Y - win.layout.Row.Height
}

func (win *Window) LayoutAvailableWidth() int {
	return win.layout.Clip.W - win.layout.AtX - win.style().Spacing.X
}

func (win *Window) BelowTheFold() bool {
	return win.layout.AtY-win.layout.Offset.Y > (win.layout.Clip.Y + win.layout.Clip.H)
}

func (win *Window) At() image.Point {
	return image.Point{win.layout.AtX, win.layout.AtY}
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
	var in *Input
	if win.toplevel() && widget_state == widgetValid {
		in = &win.ctx.Input
	}

	ws := win.widgets.PrevState(header)
	if buttonBehaviorDo(&ws, header, in, ButtonNormal) {
		node.Open = !node.Open
	}

	/* calculate the triangle bounds */
	var sym Rect
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
	doButtonSymbol(&win.widgets, sym, symbolType, ButtonNormal, styleButton, in)

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
func (win *Window) LabelColored(str string, alignment TextAlign, color color.RGBA) {
	var bounds Rect
	var text textWidget

	style := &win.ctx.Style
	panelAllocSpace(&bounds, win)
	item_padding := style.Text.Padding

	text.Padding.X = item_padding.X
	text.Padding.Y = item_padding.Y
	text.Background = win.style().Background
	text.Text = color
	win.widgets.Add(WidgetStateInactive, bounds, &drawableNonInteractive{bounds, color, false, text, alignment, str, nil, nil})

}

// LabelWrapColored draws a text label with the specified background
// color autowrappping the text.
func (win *Window) LabelWrapColored(str string, color color.RGBA) {
	var bounds Rect
	var text textWidget

	style := &win.ctx.Style
	panelAllocSpace(&bounds, win)
	item_padding := style.Text.Padding

	text.Padding.X = item_padding.X
	text.Padding.Y = item_padding.Y
	text.Background = win.style().Background
	text.Text = color
	win.widgets.Add(WidgetStateInactive, bounds, &drawableNonInteractive{bounds, color, true, text, TextAlignLeft, str, nil, nil})
}

// Label draws a text label.
func (win *Window) Label(str string, alignment TextAlign) {
	win.LabelColored(str, alignment, win.ctx.Style.Text.Color)
}

// LabelWrap draws a text label, autowrapping its contents.
func (win *Window) LabelWrap(str string) {
	win.LabelWrapColored(str, win.ctx.Style.Text.Color)
}

// Image draws an image.
func (win *Window) Image(img *image.RGBA) {
	var bounds Rect
	var s widgetLayoutStates

	if s, bounds = win.widget(); s == 0 {
		return
	}
	win.widgets.Add(WidgetStateInactive, bounds, &drawableNonInteractive{Bounds: bounds, Img: img})
}

// Spacing adds empty space
func (win *Window) Spacing(cols int) {
	var nilrect Rect
	var i int

	/* spacing over row boundaries */

	layout := win.layout
	index := (layout.Row.Index + cols) % layout.Row.Columns
	rows := (layout.Row.Index + cols) / layout.Row.Columns
	if rows != 0 {
		for i = 0; i < rows; i++ {
			panelAllocRow(win)
		}
		cols = index
	}

	/* non table layout need to allocate space */
	if layout.Row.Type != layoutDynamicFixed && layout.Row.Type != layoutStaticFixed {
		for i = 0; i < cols; i++ {
			panelAllocSpace(&nilrect, win)
		}
	}

	layout.Row.Index = index
}

// CustomState returns the widget state of a custom widget.
func (win *Window) CustomState(bounds Rect) WidgetStates {
	s := widgetValid
	if !win.layout.Clip.Intersect(&bounds) {
		s = widgetInvalid
	}

	ws := win.widgets.PrevState(bounds)
	basicWidgetStateControl(&ws, win.inputMaybe(s), bounds)
	return ws
}

// Custom adds a custom widget.
func (win *Window) Custom(state WidgetStates) (bounds Rect, out *CommandBuffer) {
	var s widgetLayoutStates

	if s, bounds = win.widget(); s == 0 {
		return
	}
	prevstate := win.widgets.PrevState(bounds)
	exitstate := basicWidgetStateControl(&prevstate, win.inputMaybe(s), bounds)
	if state != WidgetStateActive {
		state = exitstate
	}
	win.widgets.Add(state, bounds, nil)
	return bounds, &win.cmds
}

///////////////////////////////////////////////////////////////////////////////////
// BUTTON
///////////////////////////////////////////////////////////////////////////////////

// Controls the behavior of a button.
type ButtonBehavior int

const (
	// The button will return true once every time the user clicks it.
	ButtonNormal ButtonBehavior = iota
	// The button will return true once for every frame while pressed.
	ButtonRepeater
)

func buttonBehaviorDo(state *WidgetStates, r Rect, i *Input, behavior ButtonBehavior) (ret bool) {
	exitstate := basicWidgetStateControl(state, i, r)

	if *state == WidgetStateActive {
		if exitstate == WidgetStateHovered {
			switch behavior {
			default:
				ret = InputIsMouseDown(i, mouse.ButtonLeft)
			case ButtonNormal:
				ret = InputIsMouseReleased(i, mouse.ButtonLeft)
			}
		}
		if !InputIsMouseDown(i, mouse.ButtonLeft) {
			*state = exitstate
		}
	}

	return ret
}

func doButton(out *widgetBuffer, r Rect, style *StyleButton, in *Input, behavior ButtonBehavior) (ok bool, state WidgetStates, content Rect) {
	/* calculate button content space */
	content.X = r.X + style.Padding.X + style.Border
	content.Y = r.Y + style.Padding.Y + style.Border
	content.W = r.W - 2*style.Padding.X + style.Border
	content.H = r.H - 2*style.Padding.Y + style.Border

	/* execute button behavior */
	var bounds Rect
	bounds.X = r.X - style.TouchPadding.X
	bounds.Y = r.Y - style.TouchPadding.Y
	bounds.W = r.W + 2*style.TouchPadding.X
	bounds.H = r.H + 2*style.TouchPadding.Y
	state = out.PrevState(bounds)
	ok = buttonBehaviorDo(&state, bounds, in, behavior)
	return ok, state, content
}

func doButtonText(out *widgetBuffer, bounds Rect, str string, align TextAlign, behavior ButtonBehavior, style *StyleButton, in *Input) (ret bool) {
	if str == "" {
		return false
	}

	ret, state, content := doButton(out, bounds, style, in, behavior)
	out.Add(state, bounds, &drawableTextButton{bounds, content, state, style, str, align})
	return ret
}

func doButtonSymbol(out *widgetBuffer, bounds Rect, symbol SymbolType, behavior ButtonBehavior, style *StyleButton, in *Input) (ret bool) {
	ret, state, content := doButton(out, bounds, style, in, behavior)
	out.Add(state, bounds, &drawableSymbolButton{bounds, content, state, style, symbol})
	return ret
}

func doButtonImage(out *widgetBuffer, bounds Rect, img *image.RGBA, b ButtonBehavior, style *StyleButton, in *Input) (ret bool) {
	ret, state, content := doButton(out, bounds, style, in, b)
	content.X += style.ImagePadding.X
	content.Y += style.ImagePadding.Y
	content.W -= 2 * style.ImagePadding.X
	content.H -= 2 * style.ImagePadding.Y

	out.Add(state, bounds, &drawableImageButton{bounds, content, state, style, img})

	return ret
}

func doButtonTextSymbol(out *widgetBuffer, bounds Rect, symbol SymbolType, str string, align TextAlign, behavior ButtonBehavior, style *StyleButton, font *Face, in *Input) (ret bool) {
	ret, state, content := doButton(out, bounds, style, in, behavior)
	var tri Rect
	tri.Y = content.Y + (content.H / 2) - fontHeight(font)/2
	tri.W = fontHeight(font)
	tri.H = fontHeight(font)
	if align&TextAlignLeft != 0 {
		tri.X = (content.X + content.W) - (2*style.Padding.X + tri.W)
		tri.X = max(tri.X, 0)
	} else {
		tri.X = content.X + 2*style.Padding.X
	}

	out.Add(state, bounds, &drawableTextSymbolButton{bounds, content, tri, state, style, str, symbol})

	return ret
}

func doButtonTextImage(out *widgetBuffer, bounds Rect, img *image.RGBA, str string, align TextAlign, behavior ButtonBehavior, style *StyleButton, in *Input) (ret bool) {
	var icon Rect

	if str == "" {
		return false
	}

	ret, state, content := doButton(out, bounds, style, in, behavior)
	icon.Y = bounds.Y + style.Padding.Y
	icon.H = bounds.H - 2*style.Padding.Y
	icon.W = icon.H
	if align&TextAlignLeft != 0 {
		icon.X = (bounds.X + bounds.W) - (2*style.Padding.X + icon.W)
		icon.X = max(icon.X, 0)
	} else {
		icon.X = bounds.X + 2*style.Padding.X
	}

	icon.X += style.ImagePadding.X
	icon.Y += style.ImagePadding.Y
	icon.W -= 2 * style.ImagePadding.X
	icon.H -= 2 * style.ImagePadding.Y

	out.Add(state, bounds, &drawableTextImageButton{bounds, content, icon, state, style, str, img})
	return ret
}

// ButtonText adds a button with a text caption
func (win *Window) ButtonText(title string, behavior ButtonBehavior) bool {
	style := &win.ctx.Style
	state, bounds := win.widget()

	if state == 0 {
		return false
	}
	in := win.inputMaybe(state)
	//TODO: collapse call
	return doButtonText(&win.widgets, bounds, title, style.Button.TextAlignment, behavior, &style.Button, in)
}

// ButtonColor adds a button filled with the specified color.
func (win *Window) ButtonColor(color color.RGBA, behavior ButtonBehavior) bool {
	state, bounds := win.widget()
	if state == 0 {
		return false
	}
	in := win.inputMaybe(state)

	button := win.ctx.Style.Button
	button.Normal = MakeStyleItemColor(color)
	button.Hover = MakeStyleItemColor(color)
	button.Active = MakeStyleItemColor(color)
	button.Padding = image.Point{0, 0}
	ret, ws, bounds := doButton(&win.widgets, bounds, &button, in, behavior)
	win.widgets.Add(ws, bounds, &drawableSymbolButton{bounds, bounds, ws, &button, SymbolNone})
	return ret
}

// ButtonSymbol adds a button with the specified symbol as caption.
func (win *Window) ButtonSymbol(symbol SymbolType, behavior ButtonBehavior) bool {
	style := &win.ctx.Style

	state, bounds := win.widget()
	if state == 0 {
		return false
	}
	in := win.inputMaybe(state)
	return doButtonSymbol(&win.widgets, bounds, symbol, behavior, &style.Button, in)
}

// ButtonImage adds a button with the specified image as caption.
func (win *Window) ButtonImage(img *image.RGBA, behavior ButtonBehavior) bool {
	style := &win.ctx.Style

	state, bounds := win.widget()
	if state == 0 {
		return false
	}
	in := win.inputMaybe(state)
	return doButtonImage(&win.widgets, bounds, img, behavior, &style.Button, in)
}

// ButtonSymbolText adds a button with the specified symbol and text as caption
func (win *Window) ButtonSymbolText(symbol SymbolType, text string, align TextAlign, behavior ButtonBehavior) bool {
	style := &win.ctx.Style

	state, bounds := win.widget()
	if state == 0 {
		return false
	}
	in := win.inputMaybe(state)
	return doButtonTextSymbol(&win.widgets, bounds, symbol, text, align, behavior, &style.Button, style.Font, in)
}

// ButtonImageLabel adds a button with the specified image and text as caption
func (win *Window) ButtonImageText(img *image.RGBA, text string, align TextAlign, behavior ButtonBehavior) bool {
	style := &win.ctx.Style

	state, bounds := win.widget()
	if state == 0 {
		return false
	}
	in := win.inputMaybe(state)
	return doButtonTextImage(&win.widgets, bounds, img, text, align, behavior, &style.Button, in)
}

///////////////////////////////////////////////////////////////////////////////////
// SELECTABLE
///////////////////////////////////////////////////////////////////////////////////

func doSelectable(out *widgetBuffer, bounds Rect, str string, align TextAlign, value *bool, style *StyleSelectable, in *Input) bool {
	if str == "" {
		return false
	}
	old_value := *value

	/* remove padding */
	var touch Rect
	touch.X = bounds.X - style.TouchPadding.X
	touch.Y = bounds.Y - style.TouchPadding.Y
	touch.W = bounds.W + style.TouchPadding.X*2
	touch.H = bounds.H + style.TouchPadding.Y*2

	/* update button */
	state := out.PrevState(bounds)
	if buttonBehaviorDo(&state, touch, in, ButtonNormal) {
		*value = !*value
	}

	out.Add(state, bounds, &drawableSelectable{state, style, *value, bounds, str, align})
	return old_value != *value
}

// SelectableLabel adds a selectable label. Value is a pointer
// to a flag that will be changed to reflect the selected state of
// this label.
// Returns true when the label is clicked.
func (win *Window) SelectableLabel(str string, align TextAlign, value *bool) bool {
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

type Orientation int

const (
	Vertical Orientation = iota
	Horizontal
)

func scrollbarBehavior(state *WidgetStates, in *Input, scroll, cursor, scrollwheel_bounds Rect, scroll_offset float64, target float64, scroll_step float64, o Orientation) float64 {
	exitstate := basicWidgetStateControl(state, in, cursor)

	if *state == WidgetStateActive {
		if !InputIsMouseDown(in, mouse.ButtonLeft) {
			*state = exitstate
		} else {
			if o == Vertical {
				pixel := in.Mouse.Delta.Y
				delta := (float64(pixel) / float64(scroll.H)) * target
				scroll_offset = clampFloat(0, scroll_offset+delta, target-float64(scroll.H))
			} else {
				pixel := in.Mouse.Delta.X
				delta := (float64(pixel) / float64(scroll.W)) * target
				scroll_offset = clampFloat(0, scroll_offset+delta, target-float64(scroll.W))
			}
		}
	}

	if o == Vertical && (in != nil) && ((in.Mouse.ScrollDelta < 0) || (in.Mouse.ScrollDelta > 0)) && InputIsMouseHoveringRect(in, scrollwheel_bounds) {
		/* update cursor by mouse scrolling */
		old_scroll_offset := scroll_offset
		scroll_offset = scroll_offset + scroll_step*float64(-in.Mouse.ScrollDelta)

		if o == Vertical {
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

func doScrollbarv(out *widgetBuffer, scroll, scrollwheel_bounds Rect, offset float64, target float64, step float64, button_pixel_inc float64, style *StyleScrollbar, in *Input, font *Face) float64 {
	var cursor Rect
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
		var button Rect
		button.X = scroll.X
		button.W = scroll.W
		button.H = scroll.W

		scroll_h := float64(scroll.H - 2*button.H)
		scroll_step = minFloat(step, button_pixel_inc)

		/* decrement button */
		button.Y = scroll.Y

		if doButtonSymbol(out, button, style.DecSymbol, ButtonRepeater, &style.DecButton, in) {
			offset = offset - scroll_step
		}

		/* increment button */
		button.Y = scroll.Y + scroll.H - button.H

		if doButtonSymbol(out, button, style.IncSymbol, ButtonRepeater, &style.IncButton, in) {
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
	state := out.PrevState(scroll)
	scroll_offset = scrollbarBehavior(&state, in, scroll, cursor, scrollwheel_bounds, scroll_offset, target, scroll_step, Vertical)

	scroll_off = scroll_offset / target
	cursor.Y = scroll.Y + int(scroll_off*float64(scroll.H))

	out.Add(state, scroll, &drawableScrollbar{state, style, scroll, cursor})

	return scroll_offset
}

func doScrollbarh(out *widgetBuffer, scroll Rect, offset float64, target float64, step float64, button_pixel_inc float64, style *StyleScrollbar, in *Input, font *Face) float64 {
	var cursor Rect
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
		var button Rect
		button.Y = scroll.Y
		button.W = scroll.H
		button.H = scroll.H

		scroll_w = float64(scroll.W - 2*button.W)
		scroll_step = minFloat(step, button_pixel_inc)

		/* decrement button */
		button.X = scroll.X

		if doButtonSymbol(out, button, style.DecSymbol, ButtonRepeater, &style.DecButton, in) {
			offset = offset - scroll_step
		}

		/* increment button */
		button.X = scroll.X + scroll.W - button.W

		if doButtonSymbol(out, button, style.IncSymbol, ButtonRepeater, &style.IncButton, in) {
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
	state := out.PrevState(scroll)
	scroll_offset = scrollbarBehavior(&state, in, scroll, cursor, Rect{0, 0, 0, 0}, scroll_offset, target, scroll_step, Horizontal)

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

func toggleBehavior(in *Input, b Rect, state *WidgetStates, active bool) bool {
	//TODO: rewrite using basicWidgetStateControl
	if InputIsMouseHoveringRect(in, b) {
		*state = WidgetStateHovered
	} else {
		*state = WidgetStateInactive
	}
	if *state == WidgetStateHovered && InputMouseClicked(in, mouse.ButtonLeft, b) {
		*state = WidgetStateActive
		active = !active
	}

	return active
}

func doToggle(out *widgetBuffer, r Rect, active bool, str string, type_ toggleType, style *StyleToggle, in *Input, font *Face) bool {
	var bounds Rect
	var select_ Rect
	var cursor Rect
	var label Rect
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

func sliderBehavior(state *WidgetStates, cursor *Rect, in *Input, style *StyleSlider, bounds Rect, slider_min float64, slider_value float64, slider_max float64, slider_step float64, slider_steps int) float64 {
	exitstate := basicWidgetStateControl(state, in, bounds)

	if *state == WidgetStateActive {
		if !InputIsMouseDown(in, mouse.ButtonLeft) {
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

func doSlider(out *widgetBuffer, bounds Rect, minval float64, val float64, maxval float64, step float64, style *StyleSlider, in *Input, font *Face) float64 {
	var slider_range float64
	var cursor_offset float64
	var cursor Rect

	/* remove padding from slider bounds */
	bounds.X = bounds.X + style.Padding.X

	bounds.Y = bounds.Y + style.Padding.Y
	bounds.H = max(bounds.H, 2*style.Padding.Y)
	bounds.W = max(bounds.W, 1+bounds.H+2*style.Padding.X)
	bounds.H -= 2 * style.Padding.Y
	bounds.W -= 2 * style.Padding.Y

	/* optional buttons */
	if style.ShowButtons {
		var button Rect
		button.Y = bounds.Y
		button.W = bounds.H
		button.H = bounds.H

		/* decrement button */
		button.X = bounds.X

		if doButtonSymbol(out, button, style.DecSymbol, ButtonNormal, &style.DecButton, in) {
			val -= step
		}

		/* increment button */
		button.X = (bounds.X + bounds.W) - button.W

		if doButtonSymbol(out, button, style.IncSymbol, ButtonNormal, &style.IncButton, in) {
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
	*value = doSlider(&win.widgets, bounds, min_value, old_value, max_value, value_step, &style.Slider, in, style.Font)
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

func progressBehavior(state *WidgetStates, in *Input, r Rect, maxval int, value int, modifiable bool) int {
	if !modifiable {
		*state = WidgetStateInactive
		return value
	}

	exitstate := basicWidgetStateControl(state, in, r)

	if *state == WidgetStateActive {
		if !InputIsMouseDown(in, mouse.ButtonLeft) {
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

func doProgress(out *widgetBuffer, bounds Rect, value int, maxval int, modifiable bool, style *StyleProgress, in *Input) int {
	var prog_scale float64
	var cursor Rect

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

func (ed *TextEditor) propertyBehavior(ws *WidgetStates, in *Input, property Rect, label Rect, edit Rect, empty Rect) (drag bool, delta int) {
	if ed.propertyStatus == propertyDefault {
		if buttonBehaviorDo(ws, edit, in, ButtonNormal) {
			ed.propertyStatus = propertyEdit
		} else if InputIsMouseClickDownInRect(in, mouse.ButtonLeft, label, true) {
			ed.propertyStatus = propertyDrag
		} else if InputIsMouseClickDownInRect(in, mouse.ButtonLeft, empty, true) {
			ed.propertyStatus = propertyDrag
		}
	}

	if ed.propertyStatus == propertyDrag {
		if InputIsMouseReleased(in, mouse.ButtonLeft) {
			ed.propertyStatus = propertyDefault
		} else {
			delta = in.Mouse.Delta.X
			drag = true
		}
	}

	if ed.propertyStatus == propertyDefault {
		if InputIsMouseHoveringRect(in, property) {
			*ws = WidgetStateHovered
		} else {
			*ws = WidgetStateInactive
		}
	} else {
		*ws = WidgetStateActive
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

func (win *Window) doProperty(property Rect, name string, text string, filter FilterFunc, in *Input) (ret doPropertyRet, delta int, ed *TextEditor) {
	ret = doPropertyStay
	style := &win.ctx.Style.Property
	font := win.ctx.Style.Font

	// left decrement button
	var left Rect
	left.H = fontHeight(font) / 2
	left.W = left.H
	left.X = property.X + style.Border + style.Padding.X
	left.Y = property.Y + style.Border + property.H/2.0 - left.H/2

	// text label
	size := fontWidth(font, name)
	var label Rect
	label.X = left.X + left.W + style.Padding.X
	label.W = size + 2*style.Padding.X
	label.Y = property.Y + style.Border
	label.H = property.H - 2*style.Border

	/* right increment button */
	var right Rect
	right.Y = left.Y
	right.W = left.W
	right.H = left.H
	right.X = property.X + property.W - (right.W + style.Padding.X)

	ws := win.widgets.PrevState(property)
	oldws := ws
	if ws == WidgetStateActive {
		ed = win.editor
	} else {
		ed = &TextEditor{}
		ed.initString(win)
		ed.Buffer = []rune(text)
	}

	size = fontWidth(font, string(ed.Buffer)) + style.Edit.CursorSize

	/* edit */
	var edit Rect
	edit.W = size + 2*style.Padding.X
	edit.X = right.X - (edit.W + style.Padding.X)
	edit.Y = property.Y + style.Border + 1
	edit.H = property.H - (2*style.Border + 2)

	/* empty left space activator */
	var empty Rect
	empty.W = edit.X - (label.X + label.W)
	empty.X = label.X + label.W
	empty.Y = property.Y
	empty.H = property.H

	/* update property */
	old := ed.propertyStatus == propertyEdit

	drag, delta := ed.propertyBehavior(&ws, in, property, label, edit, empty)
	if drag {
		ret = doPropertyDrag
	}
	if ws == WidgetStateActive {
		ed.Active = true
		win.editor = ed
	} else if oldws == WidgetStateActive {
		win.editor = nil
	}
	ed.win.widgets.Add(ws, property, &drawableProperty{style, property, label, ws, name})

	/* execute right and left button  */
	if doButtonSymbol(&ed.win.widgets, left, style.SymLeft, ButtonNormal, &style.DecButton, in) {
		ret = doPropertyDec
	}
	if doButtonSymbol(&ed.win.widgets, right, style.SymRight, ButtonNormal, &style.IncButton, in) {
		ret = doPropertyInc
	}

	active := ed.propertyStatus == propertyEdit
	if !old && active {
		/* property has been activated so setup buffer */
		ed.Cursor = len(ed.Buffer)
		ed.Filter = filter
	}
	ed.Flags = EditAlwaysInsertMode | EditNoHorizontalScroll
	ed.doEdit(edit, &style.Edit, in)
	active = ed.Active

	if active && InputIsKeyPressed(in, key.CodeReturnEnter) {
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
		*val, _ = strconv.ParseFloat(ed.String(), 64)
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
		*val, _ = strconv.Atoi(ed.String())
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

func (ctx *context) nonblockOpen(flags WindowFlags, body Rect, header Rect, updateFn UpdateFn) {
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
func (win *MasterWindow) PopupOpen(title string, flags WindowFlags, rect Rect, scale bool, updateFn UpdateFn) {
	go func() {
		win.uilock.Lock()
		defer win.uilock.Unlock()
		win.ctx.popupOpen(title, flags, rect, scale, updateFn)
	}()
}

func (ctx *context) popupOpen(title string, flags WindowFlags, rect Rect, scale bool, updateFn UpdateFn) {
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
func (win *Window) ContextualOpen(flags WindowFlags, size image.Point, trigger_bounds Rect, updateFn UpdateFn) {
	size.X = win.ctx.scale(size.X)
	size.Y = win.ctx.scale(size.Y)
	if !InputMouseClicked(win.Input(), mouse.ButtonRight, trigger_bounds) {
		return
	}

	var body Rect
	body.X = win.ctx.Input.Mouse.Pos.X
	body.Y = win.ctx.Input.Mouse.Pos.Y
	body.W = size.X
	body.H = size.Y

	atomic.AddInt32(&win.ctx.changed, 1)
	win.ctx.nonblockOpen(flags|windowContextual|WindowNoScrollbar, body, Rect{}, updateFn)
	popup := win.ctx.Windows[len(win.ctx.Windows)-1]
	popup.triggerBounds = trigger_bounds
}

// ContextualItemText adds a text item to the contextual menu
// (see ContextualBegin).
func (win *Window) ContextualItemText(text string, alignment TextAlign) bool {
	style := &win.ctx.Style
	state, bounds := win.widgetFitting(style.ContextualButton.Padding)
	if state == 0 {
		return false
	}

	in := win.inputMaybe(state)
	if doButtonText(&win.widgets, bounds, text, alignment, ButtonNormal, &style.ContextualButton, in) {
		win.Close()
		return true
	}

	return false
}

// ContextualItemImageText adds an image+text item to the contextual
// menu (see ContextualBegin).
func (win *Window) ContextualItemImageText(img *image.RGBA, text string, align TextAlign) bool {
	style := &win.ctx.Style
	state, bounds := win.widgetFitting(style.ContextualButton.Padding)
	if state == 0 {
		return false
	}

	in := win.inputMaybe(state)
	if doButtonTextImage(&win.widgets, bounds, img, text, align, ButtonNormal, &style.ContextualButton, in) {
		win.Close()
		return true
	}

	return false
}

// ContextualItemSymbolText adds a symbol+text item to the contextual
// menu (see ContextualBegin).
func (win *Window) ContextualItemSymbolText(symbol SymbolType, text string, align TextAlign) bool {
	style := &win.ctx.Style
	state, bounds := win.widgetFitting(style.ContextualButton.Padding)
	if state == 0 {
		return false
	}

	in := win.inputMaybe(state)
	if doButtonTextSymbol(&win.widgets, bounds, symbol, text, align, ButtonNormal, &style.ContextualButton, style.Font, in) {
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

	var bounds Rect
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
	text_width := fontWidth(win.ctx.Style.Font, text)
	text_height := fontHeight(win.ctx.Style.Font)
	text_width += win.ctx.scale(2*padding.X) + win.ctx.scale(2*item_spacing.X)

	win.TooltipOpen(text_width, false, func(mw *MasterWindow, tw *Window) {
		tw.LayoutRowDynamicScaled(text_height, 1)
		tw.Label(text, TextLeft)
	})
}

///////////////////////////////////////////////////////////////////////////////////
// COMBO-BOX
///////////////////////////////////////////////////////////////////////////////////

func (ctx *context) comboOpen(height int, is_clicked bool, header Rect, updateFn UpdateFn) {
	height = ctx.scale(height)

	if !is_clicked {
		return
	}

	var body Rect
	body.X = header.X
	body.W = header.W
	body.Y = header.Y + header.H - 1
	body.H = height

	ctx.nonblockOpen(windowCombo, body, Rect{0, 0, 0, 0}, updateFn)
}

// Adds a drop-down list to win.
func (win *Window) ComboColor(color color.RGBA, height int, updateFn UpdateFn) {
	var is_active bool = false

	s, header := win.widget()
	if s == widgetInvalid {
		return
	}

	in := win.inputMaybe(s)
	state := win.widgets.PrevState(header)
	if buttonBehaviorDo(&state, header, in, ButtonNormal) {
		is_active = true
	}

	win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Color: color, Fn: drawableComboColor})

	win.ctx.comboOpen(height, is_active, header, updateFn)
}

// Adds a drop-down list to win.
func (win *Window) ComboSymbol(symbol SymbolType, height int, updateFn UpdateFn) {
	var is_active bool = false

	s, header := win.widget()
	if s == widgetInvalid {
		return
	}

	in := win.inputMaybe(s)
	state := win.widgets.PrevState(header)
	if buttonBehaviorDo(&state, header, in, ButtonNormal) {
		is_active = true
	}

	win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Symbol: symbol, Fn: drawableComboSymbol})

	win.ctx.comboOpen(height, is_active, header, updateFn)
}

// Adds a drop-down list to win.
func (win *Window) ComboSymbolText(selected string, symbol SymbolType, height int, updateFn UpdateFn) {
	var is_active bool = false

	s, header := win.widget()
	if s == 0 {
		return
	}

	in := win.inputMaybe(s)
	state := win.widgets.PrevState(header)
	if buttonBehaviorDo(&state, header, in, ButtonNormal) {
		is_active = true
	}

	win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Symbol: symbol, Selected: selected, Fn: drawableComboSymbolText})

	win.ctx.comboOpen(height, is_active, header, updateFn)
}

// Adds a drop-down list to win.
func (win *Window) ComboImage(img *image.RGBA, height int, updateFn UpdateFn) {
	var is_active bool = false

	s, header := win.widget()
	if s == widgetInvalid {
		return
	}

	in := win.inputMaybe(s)
	state := win.widgets.PrevState(header)
	if buttonBehaviorDo(&state, header, in, ButtonNormal) {
		is_active = true
	}

	win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Image: img, Fn: drawableComboImage})

	win.ctx.comboOpen(height, is_active, header, updateFn)
}

// Adds a drop-down list to win.
func (win *Window) ComboImageText(selected string, img *image.RGBA, height int, updateFn UpdateFn) {
	var is_active bool = false

	s, header := win.widget()
	if s == 0 {
		return
	}

	in := win.inputMaybe(s)
	state := win.widgets.PrevState(header)
	if buttonBehaviorDo(&state, header, in, ButtonNormal) {
		is_active = true
	}

	win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Image: img, Selected: selected, Fn: drawableComboImageText})

	win.ctx.comboOpen(height, is_active, header, updateFn)
}

// Adds a drop-down list to win.
func (win *Window) ComboText(selected string, height int, updateFn UpdateFn) {
	is_active := false

	if selected == "" {
		return
	}

	//style := &win.ctx.Style
	s, header := win.widget()
	if s == widgetInvalid {
		return
	}

	in := win.inputMaybe(s)
	state := win.widgets.PrevState(header)
	if buttonBehaviorDo(&state, header, in, ButtonNormal) {
		is_active = true
	}

	win.widgets.Add(state, header, &drawableCombo{State: state, Header: header, Active: is_active, Selected: selected, Fn: drawableComboText})

	win.ctx.comboOpen(height, is_active, header, updateFn)
}

// Adds a drop-down list to win. The contents are specified by items,
// with selected being the index of the selected item.
func (win *Window) Combo(items []string, selected *int, item_height int) {
	if len(items) == 0 {
		return
	}

	item_padding := win.ctx.Style.Combo.ButtonPadding.Y
	window_padding := win.style().Padding.Y
	max_height := (len(items)+1)*item_height + item_padding*3 + window_padding*2
	win.ComboText(items[*selected], max_height, func(mw *MasterWindow, w *Window) {
		w.LayoutRowDynamic(item_height, 1)
		for i := range items {
			if w.ContextualItemText(items[i], TextLeft) {
				*selected = i
			}
		}
	})
}

///////////////////////////////////////////////////////////////////////////////////
// MENU
///////////////////////////////////////////////////////////////////////////////////

func (win *Window) menuOpen(is_clicked bool, header Rect, width int, updateFn UpdateFn) {
	width = win.ctx.scale(width)

	if !is_clicked {
		return
	}

	var body Rect
	body.X = header.X
	body.W = width
	body.Y = header.Y + header.H
	body.H = (win.layout.Bounds.Y + win.layout.Bounds.H) - body.Y

	win.ctx.nonblockOpen(windowMenu|WindowNoScrollbar, body, header, updateFn)
}

// Adds a menu to win with a text label.
func (win *Window) MenuText(title string, align TextAlign, width int, updateFn UpdateFn) {
	state, header := win.widget()
	if state == 0 {
		return
	}
	var in *Input
	if !(state == widgetRom || !win.toplevel()) {
		in = &win.ctx.Input
	}
	is_clicked := doButtonText(&win.widgets, header, title, align, ButtonNormal, &win.ctx.Style.MenuButton, in)
	win.menuOpen(is_clicked, header, width, updateFn)
}

// Adds a menu to win with an image label.
func (win *Window) MenuImage(img *image.RGBA, width int, updateFn UpdateFn) {
	state, header := win.widget()
	if state == 0 {
		return
	}
	in := win.inputMaybe(state)
	is_clicked := doButtonImage(&win.widgets, header, img, ButtonNormal, &win.ctx.Style.MenuButton, in)
	win.menuOpen(is_clicked, header, width, updateFn)
}

// Adds a menu to win with a symbol label.
func (win *Window) MenuOpenSymbol(sym SymbolType, width int, updateFn UpdateFn) {
	state, header := win.widget()
	if state == 0 {
		return
	}
	in := win.inputMaybe(state)
	is_clicked := doButtonSymbol(&win.widgets, header, sym, ButtonNormal, &win.ctx.Style.MenuButton, in)
	win.menuOpen(is_clicked, header, width, updateFn)
}

// Adds a menu to win with an image and text label.
func (win *Window) MenuImageText(title string, align TextAlign, img *image.RGBA, width int, updateFn UpdateFn) {
	state, header := win.widget()
	if state == 0 {
		return
	}
	in := win.inputMaybe(state)
	is_clicked := doButtonTextImage(&win.widgets, header, img, title, align, ButtonNormal, &win.ctx.Style.MenuButton, in)
	win.menuOpen(is_clicked, header, width, updateFn)
}

// Adds a menu to win with a symbol and text label.
func (win *Window) MenuSymbolText(title string, size int, align TextAlign, sym SymbolType, width int, updateFn UpdateFn) {
	state, header := win.widget()
	if state == 0 {
		return
	}

	in := win.inputMaybe(state)
	is_clicked := doButtonTextSymbol(&win.widgets, header, sym, title, align, ButtonNormal, &win.ctx.Style.MenuButton, win.ctx.Style.Font, in)
	win.menuOpen(is_clicked, header, width, updateFn)
}

func (win *Window) MenuItemText(title string, align TextAlign) bool {
	return win.ContextualItemText(title, align)
}

func (win *Window) MenuItemImageText(img *image.RGBA, text string, align TextAlign) bool {
	return win.ContextualItemImageText(img, text, align)
}

func (win *Window) MenuItemSymbolText(sym SymbolType, text string, align TextAlign) bool {
	return win.ContextualItemSymbolText(sym, text, align)
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
	} else {
		sw.curNode = sw.rootNode
		sw.widgets.reset()
		sw.cmds.reset()
	}

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
func (sw *Window) GroupEnd() {
	panelEnd(sw.ctx, sw)
	sw.parent.widgets.prev = append(sw.parent.widgets.prev, sw.widgets.prev...)
	sw.parent.widgets.cur = append(sw.parent.widgets.cur, sw.widgets.cur...)

	// immediate drawing
	sw.parent.cmds.Commands = append(sw.parent.cmds.Commands, sw.cmds.Commands...)
}
