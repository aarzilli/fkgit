package nucular

import (
	"image"
	"image/color"
)

func drawSymbol(out *CommandBuffer, type_ SymbolType, content Rect, background color.RGBA, foreground color.RGBA, border_width int, font *Face) {
	triangleSymbol := func(heading Heading) {
		points := triangleFromDirection(content, 0, 0, heading)
		out.FillTriangle(points[0], points[1], points[2], foreground)
	}
	switch type_ {
	case SymbolX,
		SymbolUnderscore,
		SymbolPlus,
		SymbolMinus:
		var X rune
		switch type_ {
		case SymbolX:
			X = 'x'
		case SymbolUnderscore:
			X = '_'
		case SymbolPlus:
			X = '+'
		case SymbolMinus:
			X = '-'
		}

		var text textWidget
		text.Padding = image.Point{0, 0}
		text.Background = background
		text.Text = foreground
		widgetText(out, content, string(X), &text, TextCentered, font)
	case SymbolRect, SymbolRectFilled:
		out.FillRect(content, 0, foreground)
		if type_ == SymbolRectFilled {
			out.FillRect(shrinkRect(content, border_width), 0, background)
		}
	case SymbolCircle, SymbolCircleFilled:
		out.FillCircle(content, foreground)
		if type_ == SymbolCircleFilled {
			out.FillCircle(shrinkRect(content, 1), background)
		}
	case SymbolTriangleUp:
		triangleSymbol(Up)
	case SymbolTriangleDown:
		triangleSymbol(Down)
	case SymbolTriangleLeft:
		triangleSymbol(Left)
	case SymbolTriangleRight:
		triangleSymbol(Right)
	default:
		fallthrough
	case SymbolNone:
		break
	}
}

///////////////////////////////////////////////////////////////////////////////////
// WINOWS
///////////////////////////////////////////////////////////////////////////////////

type drawableWindowHeader struct {
	Header  Rect
	Label   Rect
	Hovered bool
	Title   string

	Minimized     bool
	Dynamic       bool
	HeaderActive  bool
	Bounds        Rect
	RowHeight     int
	LayoutWidth   int
	LayoutHeaderH int
	Style         *StyleWindow
}

func (dwh *drawableWindowHeader) Draw(z *Style, out *CommandBuffer) {
	style := dwh.Style
	if dwh.Minimized {
		/* draw window background if minimized */
		out.FillRect(Rect{dwh.Bounds.X, dwh.Bounds.Y, dwh.Bounds.W, dwh.RowHeight}, 0, style.Background)
	} else if !dwh.Dynamic {
		/* draw fixed window body */
		body := dwh.Bounds
		if dwh.HeaderActive {
			body.Y += dwh.LayoutHeaderH - 1
			body.H -= dwh.LayoutHeaderH
		}

		if style.FixedBackground.Type == StyleItemImage {
			out.DrawImage(body, style.FixedBackground.Data.Image)
		} else {
			out.FillRect(body, 0, style.FixedBackground.Data.Color)
		}
	} else {
		/* draw dynamic window body */
		out.FillRect(Rect{dwh.Bounds.X, dwh.Bounds.Y, dwh.Bounds.W, dwh.RowHeight + style.Padding.Y}, 0, style.Background)
	}

	if dwh.HeaderActive {
		var background *StyleItem
		var text textWidget

		/* select correct header background and text color */
		if dwh.Hovered {
			background = &style.Header.Hover
			text.Text = style.Header.LabelHover
		} else {
			background = &style.Header.Active
			text.Text = style.Header.LabelActive
		}

		/* draw header background */
		if background.Type == StyleItemImage {
			text.Background = color.RGBA{0, 0, 0, 0}
			out.DrawImage(dwh.Header, background.Data.Image)
		} else {
			text.Background = background.Data.Color
			out.FillRect(dwh.Header, 0, background.Data.Color)
		}

		text.Padding = image.Point{0, 0}
		widgetText(out, dwh.Label, dwh.Title, &text, TextLeft, z.Font)
	}
	return
}

type drawableWindowBody struct {
	NoScrollbar bool
	Bounds      Rect
	LayoutWidth int
	Clip        Rect
	Style       *StyleWindow
}

func (dwb *drawableWindowBody) Draw(z *Style, out *CommandBuffer) {

	out.PushScissor(dwb.Clip)
	out.Clip.X = dwb.Bounds.X
	out.Clip.W = dwb.LayoutWidth
	if !dwb.NoScrollbar {
		out.Clip.W += dwb.Style.ScrollbarSize.X
	}
	return
}

type drawableScissor struct {
	R Rect
}

func (d *drawableScissor) Draw(style *Style, out *CommandBuffer) {
	out.PushScissor(d.R)
	return
}

type drawableFillRect struct {
	R Rect
	C color.RGBA
}

func (d *drawableFillRect) Draw(style *Style, out *CommandBuffer) {
	out.FillRect(d.R, 0, d.C)
	return
}

type drawableScalerAndBorders struct {
	DrawScaler bool
	ScalerRect Rect

	DrawHeaderBorder bool
	DrawBorders      bool
	Bounds           Rect
	Border           int
	HeaderH          int
	BorderColor      color.RGBA
	PaddingY         int
	Style            *StyleWindow
}

func (d *drawableScalerAndBorders) Draw(z *Style, out *CommandBuffer) {
	style := d.Style
	if d.DrawScaler {
		/* draw scaler */
		if style.Scaler.Type == StyleItemImage {
			out.DrawImage(d.ScalerRect, style.Scaler.Data.Image)
		} else {
			out.FillTriangle(image.Point{d.ScalerRect.X + d.ScalerRect.W, d.ScalerRect.Y}, d.ScalerRect.Max(), image.Point{d.ScalerRect.X, d.ScalerRect.Y + d.ScalerRect.H}, style.Scaler.Data.Color)
		}
	}

	if d.DrawHeaderBorder {
		out.StrokeLine(image.Point{d.Bounds.X + d.Border/2.0, d.Bounds.Y + d.HeaderH - d.Border}, image.Point{d.Bounds.X + d.Bounds.W - d.Border, d.Bounds.Y + d.HeaderH - d.Border}, d.Border, d.BorderColor)
	}

	if d.DrawBorders {
		/* draw border top */
		out.StrokeLine(image.Point{d.Bounds.X + d.Border/2.0, d.Bounds.Y + d.Border/2.0}, image.Point{d.Bounds.X + d.Bounds.W - d.Border, d.Bounds.Y + d.Border/2.0}, style.Border, d.BorderColor)

		/* draw bottom border */
		out.StrokeLine(image.Point{d.Bounds.X + d.Border/2.0, d.PaddingY - d.Border}, image.Point{d.Bounds.X + d.Bounds.W - d.Border, d.PaddingY - d.Border}, d.Border, d.BorderColor)

		/* draw left border */
		out.StrokeLine(image.Point{d.Bounds.X + d.Border/2.0, d.Bounds.Y + d.Border/2.0}, image.Point{d.Bounds.X + d.Border/2.0, d.PaddingY - d.Border}, d.Border, d.BorderColor)

		/* draw right border */
		out.StrokeLine(image.Point{d.Bounds.X + d.Bounds.W - d.Border, d.Bounds.Y + d.Border/2.0}, image.Point{d.Bounds.X + d.Bounds.W - d.Border, d.PaddingY - d.Border}, d.Border, d.BorderColor)
	}
	return
}

///////////////////////////////////////////////////////////////////////////////////
// TREE
///////////////////////////////////////////////////////////////////////////////////

type drawableTreeNode struct {
	Style  *StyleWindow
	Type   TreeType
	Header Rect
	Sym    Rect
	Title  string
}

func (d *drawableTreeNode) Draw(z *Style, out *CommandBuffer) {
	type_ := d.Type
	header := d.Header
	sym := d.Sym
	title := d.Title
	style := d.Style

	item_spacing := style.Spacing
	panel_padding := style.Padding

	var text textWidget

	if type_ == TreeTab {
		var background *StyleItem = &z.Tab.Background
		if background.Type == StyleItemImage {
			out.DrawImage(header, background.Data.Image)
			text.Background = color.RGBA{0, 0, 0, 0}
		} else {
			text.Background = background.Data.Color
			out.FillRect(header, 0, z.Tab.BorderColor)
			out.FillRect(shrinkRect(header, z.Tab.Border), z.Tab.Rounding, background.Data.Color)
		}
	} else {
		text.Background = style.Background
	}

	/* draw node label */
	var label Rect
	header.W = max(header.W, sym.W+item_spacing.Y+panel_padding.X)
	label.X = sym.X + sym.W + item_spacing.X + 2*z.Tab.Spacing.X
	label.Y = sym.Y
	label.W = header.W - (sym.W + 2*z.Tab.Spacing.X + item_spacing.Y + panel_padding.X)
	label.H = fontHeight(z.Font)

	text.Text = z.Tab.Text
	widgetText(out, label, title, &text, TextLeft, z.Font)
	return
}

///////////////////////////////////////////////////////////////////////////////////
// NON-INTERACTIVE WIDGETS
///////////////////////////////////////////////////////////////////////////////////

type drawableNonInteractive struct {
	Bounds    Rect
	Color     color.RGBA
	Wrap      bool
	Text      textWidget
	Alignment TextAlign
	Str       string
	Img       *image.RGBA
	Fn        func(Rect, *Style, *CommandBuffer)
}

func (d *drawableNonInteractive) Draw(z *Style, out *CommandBuffer) {
	if d.Fn != nil {
		oldClip := out.Clip
		clip := unify(oldClip, d.Bounds)
		out.PushScissor(clip)
		d.Fn(d.Bounds, z, out)
		out.PushScissor(oldClip)
		return
	}
	if d.Img != nil {
		out.DrawImage(d.Bounds, d.Img)
		return
	}
	if d.Wrap {
		widgetTextWrap(out, d.Bounds, []rune(d.Str), &d.Text, z.Font)
	} else {
		widgetText(out, d.Bounds, d.Str, &d.Text, d.Alignment, z.Font)
	}
	return
}

///////////////////////////////////////////////////////////////////////////////////
// BUTTON
///////////////////////////////////////////////////////////////////////////////////

func drawButton(out *CommandBuffer, bounds Rect, state WidgetStates, style *StyleButton) *StyleItem {
	var background *StyleItem
	if state&WidgetStateHovered != 0 {
		background = &style.Hover
	} else if state&WidgetStateActive != 0 {
		background = &style.Active
	} else {
		background = &style.Normal
	}

	if background.Type == StyleItemImage {
		out.DrawImage(bounds, background.Data.Image)
	} else {
		out.FillRect(bounds, style.Rounding, style.BorderColor)
		out.FillRect(shrinkRect(bounds, style.Border), style.Rounding, background.Data.Color)
	}

	return background
}

type drawableTextButton struct {
	Bounds        Rect
	Content       Rect
	State         WidgetStates
	Style         *StyleButton
	Txt           string
	TextAlignment TextAlign
}

func (d *drawableTextButton) Draw(z *Style, out *CommandBuffer) {
	style := d.Style
	bounds := d.Bounds
	content := d.Content
	state := d.State
	txt := d.Txt
	align := d.TextAlignment
	font := z.Font

	if style.DrawBegin != nil {
		style.DrawBegin(out)
	}
	if style.DrawEnd != nil {
		defer style.DrawEnd(out)
	}

	if style.Draw.ButtonText != nil {
		style.Draw.ButtonText(out, bounds.Rectangle(), content.Rectangle(), state, style, txt, align, font)
		return
	}

	background := drawButton(out, bounds, state, style)

	/* select correct colors/images */
	var text textWidget
	if background.Type == StyleItemColor {
		text.Background = background.Data.Color
	} else {
		text.Background = style.TextBackground
	}
	if state&WidgetStateHovered != 0 {
		text.Text = style.TextHover
	} else if state&WidgetStateActive != 0 {
		text.Text = style.TextActive
	} else {
		text.Text = style.TextNormal
	}

	widgetText(out, content, txt, &text, align, font)
	return
}

type drawableSymbolButton struct {
	Bounds  Rect
	Content Rect
	State   WidgetStates
	Style   *StyleButton
	Symbol  SymbolType
}

func (d *drawableSymbolButton) Draw(z *Style, out *CommandBuffer) {
	style := d.Style
	bounds := d.Bounds
	content := d.Content
	state := d.State
	symbol := d.Symbol
	font := z.Font

	if style.DrawBegin != nil {
		style.DrawBegin(out)
	}
	if style.DrawEnd != nil {
		defer style.DrawEnd(out)
	}
	if style.Draw.ButtonSymbol != nil {
		style.Draw.ButtonSymbol(out, bounds.Rectangle(), content.Rectangle(), state, style, symbol, font)
		return
	}

	background := drawButton(out, bounds, state, style)
	var bg color.RGBA
	if background.Type == StyleItemColor {
		bg = background.Data.Color
	} else {
		bg = style.TextBackground
	}

	var sym color.RGBA
	if state&WidgetStateHovered != 0 {
		sym = style.TextHover
	} else if state&WidgetStateActive != 0 {
		sym = style.TextActive
	} else {
		sym = style.TextNormal
	}
	drawSymbol(out, symbol, content, bg, sym, 1, font)
	return
}

type drawableImageButton struct {
	Bounds  Rect
	Content Rect
	State   WidgetStates
	Style   *StyleButton
	Img     *image.RGBA
}

func (d *drawableImageButton) Draw(z *Style, out *CommandBuffer) {
	bounds := d.Bounds
	content := d.Content
	state := d.State
	style := d.Style
	img := d.Img

	if style.DrawBegin != nil {
		style.DrawBegin(out)
	}
	if style.DrawEnd != nil {
		defer style.DrawEnd(out)
	}
	if style.Draw.ButtonImage != nil {
		style.Draw.ButtonImage(out, bounds.Rectangle(), content.Rectangle(), state, style, img)
		return
	}
	drawButton(out, bounds, state, style)
	out.DrawImage(content, img)
	return
}

type drawableTextSymbolButton struct {
	Bounds            Rect
	Label, SymbolRect Rect
	State             WidgetStates
	Style             *StyleButton
	Txt               string
	Symbol            SymbolType
}

func (d *drawableTextSymbolButton) Draw(z *Style, out *CommandBuffer) {
	bounds := d.Bounds
	label := d.Label
	symbolrect := d.SymbolRect
	state := d.State
	style := d.Style
	str := d.Txt
	symbol := d.Symbol
	font := z.Font

	if style.DrawBegin != nil {
		style.DrawBegin(out)
	}
	if style.DrawEnd != nil {
		defer style.DrawEnd(out)
	}
	if style.Draw.ButtonTextSymbol != nil {
		style.Draw.ButtonTextSymbol(out, bounds.Rectangle(), d.Label.Rectangle(), d.SymbolRect.Rectangle(), state, style, str, symbol, font)
		return
	}

	/* select correct background colors/images */
	background := drawButton(out, bounds, state, style)

	var text textWidget
	if background.Type == StyleItemColor {
		text.Background = background.Data.Color
	} else {
		text.Background = style.TextBackground
	}

	/* select correct text colors */
	var sym color.RGBA
	if state&WidgetStateHovered != 0 {
		sym = style.TextHover
		text.Text = style.TextHover
	} else if state&WidgetStateActive != 0 {
		sym = style.TextActive
		text.Text = style.TextActive
	} else {
		sym = style.TextNormal
		text.Text = style.TextNormal
	}

	text.Padding = image.Point{0, 0}
	drawSymbol(out, symbol, symbolrect, style.TextBackground, sym, 0, font)
	widgetText(out, label, str, &text, TextCentered, font)
	return
}

type drawableTextImageButton struct {
	Bounds         Rect
	Label, ImgRect Rect
	State          WidgetStates
	Style          *StyleButton
	Str            string
	Img            *image.RGBA
}

func (d *drawableTextImageButton) Draw(z *Style, out *CommandBuffer) {
	bounds := d.Bounds
	label := d.Label
	imgrect := d.ImgRect
	state := d.State
	style := d.Style
	str := d.Str
	img := d.Img
	font := z.Font

	if style.DrawBegin != nil {
		style.DrawBegin(out)
	}
	if style.DrawEnd != nil {
		defer style.DrawEnd(out)
	}
	if style.Draw.ButtonTextImage != nil {
		style.Draw.ButtonTextImage(out, bounds.Rectangle(), label.Rectangle(), imgrect.Rectangle(), state, style, str, font, img)
		return
	}

	background := drawButton(out, bounds, state, style)

	/* select correct colors */
	var text textWidget
	if background.Type == StyleItemColor {
		text.Background = background.Data.Color
	} else {
		text.Background = style.TextBackground
	}
	if state&WidgetStateHovered != 0 {
		text.Text = style.TextHover
	} else if state&WidgetStateActive != 0 {
		text.Text = style.TextActive
	} else {
		text.Text = style.TextNormal
	}

	text.Padding = image.Point{0, 0}
	widgetText(out, label, str, &text, TextCentered, font)
	out.DrawImage(imgrect, img)
	return
}

///////////////////////////////////////////////////////////////////////////////////
// SELECTABLE
///////////////////////////////////////////////////////////////////////////////////

type drawableSelectable struct {
	State  WidgetStates
	Style  *StyleSelectable
	Active bool
	Bounds Rect
	Str    string
	Align  TextAlign
}

func (d *drawableSelectable) Draw(z *Style, out *CommandBuffer) {
	state := d.State
	style := d.Style
	active := d.Active
	bounds := d.Bounds
	str := d.Str
	align := d.Align
	font := z.Font

	if style.DrawBegin != nil {
		style.DrawBegin(out)
	}
	if style.DrawEnd != nil {
		defer style.DrawEnd(out)
	}
	if style.Draw != nil {
		style.Draw(out, state, style, active, bounds.Rectangle(), str, align, font)
		return
	}

	var background *StyleItem
	var text textWidget
	text.Padding = style.Padding

	/* select correct colors/images */
	if !active {
		if state&WidgetStateActive != 0 {
			background = &style.Pressed
			text.Text = style.TextPressed
		} else if state&WidgetStateHovered != 0 {
			background = &style.Hover
			text.Text = style.TextHover
		} else {
			background = &style.Normal
			text.Text = style.TextNormal
		}
	} else {
		if state&WidgetStateActive != 0 {
			background = &style.PressedActive
			text.Text = style.TextPressedActive
		} else if state&WidgetStateHovered != 0 {
			background = &style.HoverActive
			text.Text = style.TextHoverActive
		} else {
			background = &style.NormalActive
			text.Text = style.TextNormalActive
		}
	}

	/* draw selectable background and text */
	if background.Type == StyleItemImage {
		out.DrawImage(bounds, background.Data.Image)
		text.Background = color.RGBA{0, 0, 0, 0}
	} else {
		out.FillRect(bounds, style.Rounding, background.Data.Color)
		text.Background = background.Data.Color
	}

	widgetText(out, bounds, str, &text, align, font)
	return
}

///////////////////////////////////////////////////////////////////////////////////
// SCROLLBARS
///////////////////////////////////////////////////////////////////////////////////

type drawableScrollbar struct {
	State  WidgetStates
	Style  *StyleScrollbar
	Bounds Rect
	Scroll Rect
}

func (d *drawableScrollbar) Draw(z *Style, out *CommandBuffer) {
	state := d.State
	style := d.Style
	bounds := d.Bounds
	scroll := d.Scroll

	if style.DrawBegin != nil {
		style.DrawBegin(out)
	}
	if style.DrawEnd != nil {
		defer style.DrawEnd(out)
	}
	if style.Draw != nil {
		style.Draw(out, state, style, bounds.Rectangle(), scroll.Rectangle())
		return
	}

	/* select correct colors/images to draw */
	var background *StyleItem
	var cursorstyle *StyleItem
	if state&WidgetStateActive != 0 {
		background = &style.Active
		cursorstyle = &style.CursorActive
	} else if state&WidgetStateHovered != 0 {
		background = &style.Hover
		cursorstyle = &style.CursorHover
	} else {
		background = &style.Normal
		cursorstyle = &style.CursorNormal
	}

	/* draw background */
	if background.Type == StyleItemColor {
		out.FillRect(bounds, style.Rounding, style.BorderColor)
		out.FillRect(shrinkRect(bounds, style.Border), style.Rounding, background.Data.Color)
	} else {
		out.DrawImage(bounds, background.Data.Image)
	}

	/* draw cursor */
	if cursorstyle.Type == StyleItemImage {
		out.DrawImage(scroll, cursorstyle.Data.Image)
	} else {
		out.FillRect(scroll, style.Rounding, cursorstyle.Data.Color)
	}
	return
}

///////////////////////////////////////////////////////////////////////////////////
// TOGGLE BOXES
///////////////////////////////////////////////////////////////////////////////////

type drawableTogglebox struct {
	Type                     toggleType
	State                    WidgetStates
	Style                    *StyleToggle
	Active                   bool
	Label, Selector, Cursors Rect
	Str                      string
}

func (d *drawableTogglebox) Draw(z *Style, out *CommandBuffer) {
	type_ := d.Type
	state := d.State
	style := d.Style
	active := d.Active
	label, select_, cursor := d.Label, d.Selector, d.Cursors
	str := d.Str
	font := z.Font

	if style.DrawBegin != nil {
		style.DrawBegin(out)
	}
	if style.DrawEnd != nil {
		defer style.DrawEnd(out)
	}
	switch type_ {
	case toggleCheck:
		if style.Draw.Checkbox != nil {
			style.Draw.Checkbox(out, state, style, active, label.Rectangle(), select_.Rectangle(), cursor.Rectangle(), str, font)
			return
		}
	default:
		if style.Draw.Radio != nil {
			style.Draw.Radio(out, state, style, active, label.Rectangle(), select_.Rectangle(), cursor.Rectangle(), str, font)
			return
		}
	}

	/* select correct colors/images */
	var background *StyleItem
	var cursorstyle *StyleItem
	var text textWidget
	if state&WidgetStateHovered != 0 {
		background = &style.Hover
		cursorstyle = &style.CursorHover
		text.Text = style.TextHover
	} else if state&WidgetStateActive != 0 {
		background = &style.Hover
		cursorstyle = &style.CursorHover
		text.Text = style.TextActive
	} else {
		background = &style.Normal
		cursorstyle = &style.CursorNormal
		text.Text = style.TextNormal
	}

	/* draw background and cursor */
	if background.Type == StyleItemImage {
		out.DrawImage(select_, background.Data.Image)
	} else {
		switch type_ {
		case toggleCheck:
			out.FillRect(select_, 0, background.Data.Color)
		default:
			out.FillCircle(select_, background.Data.Color)
		}
	}
	if active {
		if cursorstyle.Type == StyleItemImage {
			out.DrawImage(cursor, cursorstyle.Data.Image)
		} else {
			switch type_ {
			case toggleCheck:
				out.FillRect(cursor, 0, cursorstyle.Data.Color)
			default:
				out.FillCircle(cursor, cursorstyle.Data.Color)
			}
		}
	}

	text.Padding.X = 0
	text.Padding.Y = 0
	text.Background = style.TextBackground
	widgetText(out, label, str, &text, TextLeft, font)
	return
}

///////////////////////////////////////////////////////////////////////////////////
// PROGRESS BAR
///////////////////////////////////////////////////////////////////////////////////

type drawableProgress struct {
	State   WidgetStates
	Style   *StyleProgress
	Bounds  Rect
	Scursor Rect
	Value   int
	MaxVal  int
}

func (d *drawableProgress) Draw(z *Style, out *CommandBuffer) {
	state := d.State
	style := d.Style
	bounds := d.Bounds
	scursor := d.Scursor
	value := d.Value
	maxval := d.MaxVal

	if style.DrawBegin != nil {
		style.DrawBegin(out)
	}
	if style.DrawEnd != nil {
		defer style.DrawEnd(out)
	}

	if style.Draw != nil {
		style.Draw(out, state, style, bounds.Rectangle(), scursor.Rectangle(), value, maxval)
		return
	}

	var background *StyleItem
	var cursor *StyleItem

	/* select correct colors/images to draw */
	if state&WidgetStateActive != 0 {
		background = &style.Active
		cursor = &style.CursorActive
	} else if state&WidgetStateHovered != 0 {
		background = &style.Hover
		cursor = &style.CursorHover
	} else {
		background = &style.Normal
		cursor = &style.CursorNormal
	}

	/* draw background */
	if background.Type == StyleItemImage {
		out.DrawImage(bounds, background.Data.Image)
	} else {
		out.FillRect(bounds, style.Rounding, background.Data.Color)
	}

	/* draw cursor */
	if cursor.Type == StyleItemImage {
		out.DrawImage(scursor, cursor.Data.Image)
	} else {
		out.FillRect(scursor, style.Rounding, cursor.Data.Color)
	}
	return
}

///////////////////////////////////////////////////////////////////////////////////
// SLIDER
///////////////////////////////////////////////////////////////////////////////////

type drawableSlider struct {
	State               WidgetStates
	Style               *StyleSlider
	Bounds              Rect
	VirtualCursor       Rect
	MinVal, Val, MaxVal float64
}

func (d *drawableSlider) Draw(z *Style, out *CommandBuffer) {
	state := d.State
	style := d.Style
	bounds := d.Bounds
	virtual_cursor := d.VirtualCursor
	minval, value, maxval := d.MinVal, d.Val, d.MaxVal

	if style.DrawBegin != nil {
		style.DrawBegin(out)
	}
	if style.DrawEnd != nil {
		defer style.DrawEnd(out)
	}
	if style.Draw != nil {
		style.Draw(out, state, style, bounds.Rectangle(), virtual_cursor.Rectangle(), minval, value, maxval)
		return
	}

	var fill Rect
	var bar Rect
	var scursor Rect
	var background *StyleItem
	var bar_color color.RGBA
	var cursor *StyleItem

	/* select correct slider images/colors */
	if state&WidgetStateActive != 0 {
		background = &style.Active
		bar_color = style.BarActive
		cursor = &style.CursorActive
	} else if state&WidgetStateHovered != 0 {
		background = &style.Hover
		bar_color = style.BarHover
		cursor = &style.CursorHover
	} else {
		background = &style.Normal
		bar_color = style.BarNormal
		cursor = &style.CursorNormal
	}

	/* calculate slider background bar */
	bar.X = bounds.X
	bar.Y = (bounds.Y + virtual_cursor.H/2) - virtual_cursor.H/8
	bar.W = bounds.W
	bar.H = bounds.H / 6

	/* resize virtual cursor to given size */
	scursor.H = style.CursorSize.Y
	scursor.W = style.CursorSize.X
	scursor.Y = (bar.Y + bar.H/2.0) - scursor.H/2.0
	scursor.X = virtual_cursor.X - (virtual_cursor.W / 2)

	/* filled background bar style */
	fill.W = (scursor.X + (scursor.W / 2.0)) - bar.X

	fill.X = bar.X
	fill.Y = bar.Y
	fill.H = bar.H

	/* draw background */
	if background.Type == StyleItemImage {
		out.DrawImage(bounds, background.Data.Image)
	} else {
		out.FillRect(bounds, style.Rounding, style.BorderColor)
		out.FillRect(shrinkRect(bounds, style.Border), style.Rounding, background.Data.Color)
	}

	/* draw slider bar */
	out.FillRect(bar, style.Rounding, bar_color)

	out.FillRect(fill, style.Rounding, style.BarFilled)

	/* draw cursor */
	if cursor.Type == StyleItemImage {
		out.DrawImage(scursor, cursor.Data.Image)
	} else {
		out.FillCircle(scursor, cursor.Data.Color)
	}
	return
}

///////////////////////////////////////////////////////////////////////////////////
// PROPERTY
///////////////////////////////////////////////////////////////////////////////////

type drawableProperty struct {
	Style  *StyleProperty
	Bounds Rect
	Label  Rect
	state  WidgetStates
	Name   string
}

func (d *drawableProperty) Draw(z *Style, out *CommandBuffer) {
	style := d.Style
	bounds := d.Bounds
	label := d.Label
	ws := d.state
	name := d.Name
	font := z.Font

	if style.DrawBegin != nil {
		style.DrawBegin(out)
	}
	if style.DrawEnd != nil {
		defer style.DrawEnd(out)
	}
	if style.Draw != nil {
		style.Draw(out, style, bounds.Rectangle(), label.Rectangle(), ws, name, font)
		return
	}

	var text textWidget
	var background *StyleItem

	// select correct background and text color
	if ws&WidgetStateActive != 0 {
		background = &style.Active
		text.Text = style.LabelActive
	} else if ws&WidgetStateHovered != 0 {
		background = &style.Hover
		text.Text = style.LabelHover
	} else {
		background = &style.Normal
		text.Text = style.LabelNormal
	}

	// draw background
	if background.Type == StyleItemImage {
		out.DrawImage(bounds, background.Data.Image)
		text.Background = color.RGBA{0, 0, 0, 0}
	} else {
		text.Background = background.Data.Color
		out.FillRect(bounds, style.Rounding, style.BorderColor)
		out.FillRect(shrinkRect(bounds, style.Border), style.Rounding, background.Data.Color)
	}

	// draw label
	widgetText(out, label, name, &text, TextCentered, font)
	return
}

///////////////////////////////////////////////////////////////////////////////////
// COMBO-BOX
///////////////////////////////////////////////////////////////////////////////////

type drawableCombo struct {
	State    WidgetStates
	Header   Rect
	Active   bool
	Selected string
	Color    color.RGBA
	Symbol   SymbolType
	Image    *image.RGBA
	Fn       func(*drawableCombo, *Style, *CommandBuffer)
}

func (d *drawableCombo) Draw(style *Style, out *CommandBuffer) {
	d.Fn(d, style, out)
	return
}

func drawableComboColor(d *drawableCombo, style *Style, out *CommandBuffer) {
	state := d.State
	header := d.Header
	is_active := d.Active
	color := d.Color

	/* draw combo box header background and border */
	var background *StyleItem
	if state&WidgetStateActive != 0 {
		background = &style.Combo.Active
	} else if state&WidgetStateHovered != 0 {
		background = &style.Combo.Hover
	} else {
		background = &style.Combo.Normal
	}

	if background.Type == StyleItemImage {
		out.DrawImage(header, background.Data.Image)
	} else {
		out.FillRect(header, 0, style.Combo.BorderColor)
		out.FillRect(shrinkRect(header, 1), 0, background.Data.Color)
	}
	{
		var content Rect
		var button Rect
		var bounds Rect
		var sym SymbolType
		if state&WidgetStateHovered != 0 {
			sym = style.Combo.SymHover
		} else if is_active {
			sym = style.Combo.SymActive
		} else {
			sym = style.Combo.SymNormal
		}

		/* calculate button */
		button.W = header.H - 2*style.Combo.ButtonPadding.Y

		button.X = (header.X + header.W - header.H) - style.Combo.ButtonPadding.X
		button.Y = header.Y + style.Combo.ButtonPadding.Y
		button.H = button.W

		content.X = button.X + style.Combo.Button.Padding.X
		content.Y = button.Y + style.Combo.Button.Padding.Y
		content.W = button.W - 2*style.Combo.Button.Padding.X
		content.H = button.H - 2*style.Combo.Button.Padding.Y

		/* draw color */
		bounds.H = header.H - 4*style.Combo.ContentPadding.Y

		bounds.Y = header.Y + 2*style.Combo.ContentPadding.Y
		bounds.X = header.X + 2*style.Combo.ContentPadding.X
		bounds.W = (button.X - (style.Combo.ContentPadding.X + style.Combo.Spacing.X)) - bounds.X
		out.FillRect(bounds, 0, color)

		/* draw open/close button */
		dbs := &drawableSymbolButton{button, content, state, &style.Combo.Button, sym}
		dbs.Draw(style, out)
	}
}

func drawableComboSymbol(d *drawableCombo, style *Style, out *CommandBuffer) {
	state := d.State
	header := d.Header
	is_active := d.Active
	symbol := d.Symbol

	var background *StyleItem
	var sym_background color.RGBA
	var symbol_color color.RGBA

	/* draw combo box header background and border */
	if state&WidgetStateActive != 0 {
		background = &style.Combo.Active
		symbol_color = style.Combo.SymbolActive
	} else if state&WidgetStateHovered != 0 {
		background = &style.Combo.Hover
		symbol_color = style.Combo.SymbolHover
	} else {
		background = &style.Combo.Normal
		symbol_color = style.Combo.SymbolHover
	}

	if background.Type == StyleItemImage {
		sym_background = color.RGBA{0, 0, 0, 0}
		out.DrawImage(header, background.Data.Image)
	} else {
		sym_background = background.Data.Color
		out.FillRect(header, 0, style.Combo.BorderColor)
		out.FillRect(shrinkRect(header, 1), 0, background.Data.Color)
	}
	{
		var bounds = Rect{0, 0, 0, 0}
		var content Rect
		var button Rect
		var sym SymbolType
		if state&WidgetStateHovered != 0 {
			sym = style.Combo.SymHover
		} else if is_active {
			sym = style.Combo.SymActive
		} else {
			sym = style.Combo.SymNormal
		}

		/* calculate button */
		button.W = header.H - 2*style.Combo.ButtonPadding.Y

		button.X = (header.X + header.W - header.H) - style.Combo.ButtonPadding.Y
		button.Y = header.Y + style.Combo.ButtonPadding.Y
		button.H = button.W

		content.X = button.X + style.Combo.Button.Padding.X
		content.Y = button.Y + style.Combo.Button.Padding.Y
		content.W = button.W - 2*style.Combo.Button.Padding.X
		content.H = button.H - 2*style.Combo.Button.Padding.Y

		/* draw symbol */
		bounds.H = header.H - 2*style.Combo.ContentPadding.Y

		bounds.Y = header.Y + style.Combo.ContentPadding.Y
		bounds.X = header.X + style.Combo.ContentPadding.X
		bounds.W = (button.X - style.Combo.ContentPadding.Y) - bounds.X
		drawSymbol(out, symbol, bounds, sym_background, symbol_color, 1.0, style.Font)

		/* draw open/close button */
		dbs := &drawableSymbolButton{button, content, state, &style.Combo.Button, sym}
		dbs.Draw(style, out)
	}
}

func drawableComboSymbolText(d *drawableCombo, style *Style, out *CommandBuffer) {
	state := d.State
	header := d.Header
	is_active := d.Active
	symbol := d.Symbol
	selected := d.Selected

	var background *StyleItem
	var symbol_color color.RGBA
	var text textWidget

	/* draw combo box header background and border */
	if state&WidgetStateActive != 0 {
		background = &style.Combo.Active
		symbol_color = style.Combo.SymbolActive
		text.Text = style.Combo.LabelActive
	} else if state&WidgetStateHovered != 0 {
		background = &style.Combo.Hover
		symbol_color = style.Combo.SymbolHover
		text.Text = style.Combo.LabelHover
	} else {
		background = &style.Combo.Normal
		symbol_color = style.Combo.SymbolNormal
		text.Text = style.Combo.LabelNormal
	}

	if background.Type == StyleItemImage {
		text.Background = color.RGBA{0, 0, 0, 0}
		out.DrawImage(header, background.Data.Image)
	} else {
		text.Background = background.Data.Color
		out.FillRect(header, 0, style.Combo.BorderColor)
		out.FillRect(shrinkRect(header, 1), 0, background.Data.Color)
	}
	{
		var content Rect
		var button Rect
		var label Rect
		var imrect Rect
		var sym SymbolType
		if state&WidgetStateHovered != 0 {
			sym = style.Combo.SymHover
		} else if is_active {
			sym = style.Combo.SymActive
		} else {
			sym = style.Combo.SymNormal
		}

		/* calculate button */
		button.W = header.H - 2*style.Combo.ButtonPadding.Y

		button.X = (header.X + header.W - header.H) - style.Combo.ButtonPadding.X
		button.Y = header.Y + style.Combo.ButtonPadding.Y
		button.H = button.W

		content.X = button.X + style.Combo.Button.Padding.X
		content.Y = button.Y + style.Combo.Button.Padding.Y
		content.W = button.W - 2*style.Combo.Button.Padding.X
		content.H = button.H - 2*style.Combo.Button.Padding.Y
		dbs := &drawableSymbolButton{button, content, state, &style.Combo.Button, sym}
		dbs.Draw(style, out)

		/* draw symbol */
		imrect.X = header.X + style.Combo.ContentPadding.X

		imrect.Y = header.Y + style.Combo.ContentPadding.Y
		imrect.H = header.H - 2*style.Combo.ContentPadding.Y
		imrect.W = imrect.H
		drawSymbol(out, symbol, imrect, text.Background, symbol_color, 1.0, style.Font)

		/* draw label */
		text.Padding = image.Point{0, 0}

		label.X = imrect.X + imrect.W + style.Combo.Spacing.X + style.Combo.ContentPadding.X
		label.Y = header.Y + style.Combo.ContentPadding.Y
		label.W = (button.X - style.Combo.ContentPadding.X) - label.X
		label.H = header.H - 2*style.Combo.ContentPadding.Y
		widgetText(out, label, selected, &text, TextLeft, style.Font)
	}
}

func drawableComboImage(d *drawableCombo, style *Style, out *CommandBuffer) {
	state := d.State
	header := d.Header
	is_active := d.Active
	img := d.Image

	var background *StyleItem

	/* draw combo box header background and border */
	if state&WidgetStateActive != 0 {
		background = &style.Combo.Active
	} else if state&WidgetStateHovered != 0 {
		background = &style.Combo.Hover
	} else {
		background = &style.Combo.Normal
	}

	if background.Type == StyleItemImage {
		out.DrawImage(header, background.Data.Image)
	} else {
		out.FillRect(header, 0, style.Combo.BorderColor)
		out.FillRect(shrinkRect(header, 1), 0, background.Data.Color)
	}
	{
		var bounds = Rect{0, 0, 0, 0}
		var content Rect
		var button Rect
		var sym SymbolType
		if state&WidgetStateHovered != 0 {
			sym = style.Combo.SymHover
		} else if is_active {
			sym = style.Combo.SymActive
		} else {
			sym = style.Combo.SymNormal
		}

		/* calculate button */
		button.W = header.H - 2*style.Combo.ButtonPadding.Y
		button.X = (header.X + header.W - header.H) - style.Combo.ButtonPadding.Y
		button.Y = header.Y + style.Combo.ButtonPadding.Y
		button.H = button.W

		content.X = button.X + style.Combo.Button.Padding.X
		content.Y = button.Y + style.Combo.Button.Padding.Y
		content.W = button.W - 2*style.Combo.Button.Padding.X
		content.H = button.H - 2*style.Combo.Button.Padding.Y

		/* draw image */
		bounds.H = header.H - 2*style.Combo.ContentPadding.Y
		bounds.Y = header.Y + style.Combo.ContentPadding.Y
		bounds.X = header.X + style.Combo.ContentPadding.X
		bounds.W = (button.X - style.Combo.ContentPadding.Y) - bounds.X
		out.DrawImage(bounds, img)

		/* draw open/close button */
		dbs := &drawableSymbolButton{button, content, state, &style.Combo.Button, sym}
		dbs.Draw(style, out)
	}
}

func drawableComboImageText(d *drawableCombo, style *Style, out *CommandBuffer) {
	state := d.State
	header := d.Header
	is_active := d.Active
	selected := d.Selected
	img := d.Image

	var background *StyleItem
	var text textWidget

	/* draw combo box header background and border */
	if state&WidgetStateActive != 0 {
		background = &style.Combo.Active
		text.Text = style.Combo.LabelActive
	} else if state&WidgetStateHovered != 0 {
		background = &style.Combo.Hover
		text.Text = style.Combo.LabelHover
	} else {
		background = &style.Combo.Normal
		text.Text = style.Combo.LabelNormal
	}

	if background.Type == StyleItemImage {
		text.Background = color.RGBA{0, 0, 0, 0}
		out.DrawImage(header, background.Data.Image)
	} else {
		text.Background = background.Data.Color
		out.FillRect(header, 0, style.Combo.BorderColor)
		out.FillRect(shrinkRect(header, 1), 0, background.Data.Color)
	}
	{
		var content Rect
		var button Rect
		var label Rect
		var imrect Rect
		var sym SymbolType
		if state&WidgetStateHovered != 0 {
			sym = style.Combo.SymHover
		} else if is_active {
			sym = style.Combo.SymActive
		} else {
			sym = style.Combo.SymNormal
		}

		/* calculate button */
		button.W = header.H - 2*style.Combo.ButtonPadding.Y

		button.X = (header.X + header.W - header.H) - style.Combo.ButtonPadding.X
		button.Y = header.Y + style.Combo.ButtonPadding.Y
		button.H = button.W

		content.X = button.X + style.Combo.Button.Padding.X
		content.Y = button.Y + style.Combo.Button.Padding.Y
		content.W = button.W - 2*style.Combo.Button.Padding.X
		content.H = button.H - 2*style.Combo.Button.Padding.Y
		dbs := &drawableSymbolButton{button, content, state, &style.Combo.Button, sym}
		dbs.Draw(style, out)

		/* draw image */
		imrect.X = header.X + style.Combo.ContentPadding.X
		imrect.Y = header.Y + style.Combo.ContentPadding.Y
		imrect.H = header.H - 2*style.Combo.ContentPadding.Y
		imrect.W = imrect.H
		out.DrawImage(imrect, img)

		/* draw label */
		text.Padding = image.Point{0, 0}

		label.X = imrect.X + imrect.W + style.Combo.Spacing.X + style.Combo.ContentPadding.X
		label.Y = header.Y + style.Combo.ContentPadding.Y
		label.W = (button.X - style.Combo.ContentPadding.X) - label.X
		label.H = header.H - 2*style.Combo.ContentPadding.Y
		widgetText(out, label, selected, &text, TextLeft, style.Font)
	}
}

func drawableComboText(d *drawableCombo, style *Style, out *CommandBuffer) {
	state := d.State
	header := d.Header
	is_active := d.Active
	selected := d.Selected

	/* draw combo box header background and border */
	var background *StyleItem
	var text textWidget
	if state&WidgetStateActive != 0 {
		background = &style.Combo.Active
		text.Text = style.Combo.LabelActive
	} else if state&WidgetStateHovered != 0 {
		background = &style.Combo.Hover
		text.Text = style.Combo.LabelHover
	} else {
		background = &style.Combo.Normal
		text.Text = style.Combo.LabelNormal
	}

	if background.Type == StyleItemImage {
		text.Background = color.RGBA{0, 0, 0, 0}
		out.DrawImage(header, background.Data.Image)
	} else {
		text.Background = background.Data.Color
		out.FillRect(header, style.Combo.Rounding, style.Combo.BorderColor)
		out.FillRect(shrinkRect(header, 1), style.Combo.Rounding, background.Data.Color)
	}

	var label Rect
	var button Rect
	var content Rect
	/* print currently selected text item */

	var sym SymbolType
	if state&WidgetStateHovered != 0 {
		sym = style.Combo.SymHover
	} else if is_active {
		sym = style.Combo.SymActive
	} else {
		sym = style.Combo.SymNormal
	}

	/* calculate button */
	button.W = header.H - 2*style.Combo.ButtonPadding.Y

	button.X = (header.X + header.W - header.H) - style.Combo.ButtonPadding.X
	button.Y = header.Y + style.Combo.ButtonPadding.Y
	button.H = button.W

	content.X = button.X + style.Combo.Button.Padding.X
	content.Y = button.Y + style.Combo.Button.Padding.Y
	content.W = button.W - 2*style.Combo.Button.Padding.X
	content.H = button.H - 2*style.Combo.Button.Padding.Y

	/* draw selected label */
	text.Padding = image.Point{0, 0}

	label.X = header.X + style.Combo.ContentPadding.X
	label.Y = header.Y + style.Combo.ContentPadding.Y
	label.W = button.X - (style.Combo.ContentPadding.X + style.Combo.Spacing.X) - label.X
	label.H = header.H - 2*style.Combo.ContentPadding.Y
	widgetText(out, label, selected, &text, TextLeft, style.Font)

	/* draw open/close button */
	dbs := &drawableSymbolButton{button, content, state, &style.Combo.Button, sym}
	dbs.Draw(style, out)
}
