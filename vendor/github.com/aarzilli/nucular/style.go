package nucular

import (
	"image"
	"image/color"
)

type Style struct {
	Font             *Face
	Text             StyleText
	Button           StyleButton
	ContextualButton StyleButton
	MenuButton       StyleButton
	Option           StyleToggle
	Checkbox         StyleToggle
	Selectable       StyleSelectable
	Slider           StyleSlider
	Progress         StyleProgress
	Property         StyleProperty
	Edit             StyleEdit
	Scrollh          StyleScrollbar
	Scrollv          StyleScrollbar
	Tab              StyleTab
	Combo            StyleCombo
	NormalWindow     StyleWindow
	MenuWindow       StyleWindow
	TooltipWindow    StyleWindow
	ComboWindow      StyleWindow
	ContextualWindow StyleWindow
	GroupWindow      StyleWindow
}

type StyleColors int

const (
	ColorText StyleColors = iota
	ColorWindow
	ColorHeader
	ColorBorder
	ColorButton
	ColorButtonHover
	ColorButtonActive
	ColorToggle
	ColorToggleHover
	ColorToggleCursor
	ColorSelect
	ColorSelectActive
	ColorSlider
	ColorSliderCursor
	ColorSliderCursorHover
	ColorSliderCursorActive
	ColorProperty
	ColorEdit
	ColorEditCursor
	ColorCombo
	ColorChart
	ColorChartColor
	ColorChartColorHighlight
	ColorScrollbar
	ColorScrollbarCursor
	ColorScrollbarCursorHover
	ColorScrollbarCursorActive
	ColorTabHeader
	ColorCount
)

type StyleItemType int

const (
	StyleItemColor StyleItemType = iota
	StyleItemImage
)

type StyleItemData struct {
	Image *image.RGBA
	Color color.RGBA
}

type StyleItem struct {
	Type StyleItemType
	Data StyleItemData
}

type StyleText struct {
	Color   color.RGBA
	Padding image.Point
}

type StyleButton struct {
	Normal         StyleItem
	Hover          StyleItem
	Active         StyleItem
	BorderColor    color.RGBA
	TextBackground color.RGBA
	TextNormal     color.RGBA
	TextHover      color.RGBA
	TextActive     color.RGBA
	TextAlignment  TextAlign
	Border         int
	Rounding       uint16
	Padding        image.Point
	ImagePadding   image.Point
	TouchPadding   image.Point
	DrawBegin      func(*CommandBuffer)
	Draw           StyleCustomButtonDrawing
	DrawEnd        func(*CommandBuffer)
}

type StyleCustomButtonDrawing struct {
	ButtonText       func(*CommandBuffer, image.Rectangle, image.Rectangle, WidgetStates, *StyleButton, string, TextAlign, *Face)
	ButtonSymbol     func(*CommandBuffer, image.Rectangle, image.Rectangle, WidgetStates, *StyleButton, SymbolType, *Face)
	ButtonImage      func(*CommandBuffer, image.Rectangle, image.Rectangle, WidgetStates, *StyleButton, *image.RGBA)
	ButtonTextSymbol func(*CommandBuffer, image.Rectangle, image.Rectangle, image.Rectangle, WidgetStates, *StyleButton, string, SymbolType, *Face)
	ButtonTextImage  func(*CommandBuffer, image.Rectangle, image.Rectangle, image.Rectangle, WidgetStates, *StyleButton, string, *Face, *image.RGBA)
}

type StyleToggle struct {
	Normal         StyleItem
	Hover          StyleItem
	Active         StyleItem
	CursorNormal   StyleItem
	CursorHover    StyleItem
	TextNormal     color.RGBA
	TextHover      color.RGBA
	TextActive     color.RGBA
	TextBackground color.RGBA
	TextAlignment  uint32
	Padding        image.Point
	TouchPadding   image.Point
	DrawBegin      func(*CommandBuffer)
	Draw           StyleCustomToggleDrawing
	DrawEnd        func(*CommandBuffer)
}

type StyleCustomToggleDrawing struct {
	Radio    func(*CommandBuffer, WidgetStates, *StyleToggle, bool, image.Rectangle, image.Rectangle, image.Rectangle, string, *Face)
	Checkbox func(*CommandBuffer, WidgetStates, *StyleToggle, bool, image.Rectangle, image.Rectangle, image.Rectangle, string, *Face)
}

type StyleSelectable struct {
	Normal            StyleItem
	Hover             StyleItem
	Pressed           StyleItem
	NormalActive      StyleItem
	HoverActive       StyleItem
	PressedActive     StyleItem
	TextNormal        color.RGBA
	TextHover         color.RGBA
	TextPressed       color.RGBA
	TextNormalActive  color.RGBA
	TextHoverActive   color.RGBA
	TextPressedActive color.RGBA
	TextBackground    color.RGBA
	TextAlignment     uint32
	Rounding          uint16
	Padding           image.Point
	TouchPadding      image.Point
	DrawBegin         func(*CommandBuffer)
	Draw              func(*CommandBuffer, WidgetStates, *StyleSelectable, bool, image.Rectangle, string, TextAlign, *Face)
	DrawEnd           func(*CommandBuffer)
}

type StyleSlider struct {
	Normal       StyleItem
	Hover        StyleItem
	Active       StyleItem
	BorderColor  color.RGBA
	BarNormal    color.RGBA
	BarHover     color.RGBA
	BarActive    color.RGBA
	BarFilled    color.RGBA
	CursorNormal StyleItem
	CursorHover  StyleItem
	CursorActive StyleItem
	Border       int
	Rounding     uint16
	BarHeight    int
	Padding      image.Point
	Spacing      image.Point
	CursorSize   image.Point
	ShowButtons  bool
	IncButton    StyleButton
	DecButton    StyleButton
	IncSymbol    SymbolType
	DecSymbol    SymbolType
	DrawBegin    func(*CommandBuffer)
	Draw         func(*CommandBuffer, WidgetStates, *StyleSlider, image.Rectangle, image.Rectangle, float64, float64, float64)
	DrawEnd      func(*CommandBuffer)
}

type StyleProgress struct {
	Normal       StyleItem
	Hover        StyleItem
	Active       StyleItem
	CursorNormal StyleItem
	CursorHover  StyleItem
	CursorActive StyleItem
	Rounding     uint16
	Padding      image.Point
	DrawBegin    func(*CommandBuffer)
	Draw         func(*CommandBuffer, WidgetStates, *StyleProgress, image.Rectangle, image.Rectangle, int, int)
	DrawEnd      func(*CommandBuffer)
}

type StyleScrollbar struct {
	Normal       StyleItem
	Hover        StyleItem
	Active       StyleItem
	BorderColor  color.RGBA
	CursorNormal StyleItem
	CursorHover  StyleItem
	CursorActive StyleItem
	Border       int
	Rounding     uint16
	Padding      image.Point
	ShowButtons  bool
	IncButton    StyleButton
	DecButton    StyleButton
	IncSymbol    SymbolType
	DecSymbol    SymbolType
	DrawBegin    func(*CommandBuffer)
	Draw         func(*CommandBuffer, WidgetStates, *StyleScrollbar, image.Rectangle, image.Rectangle)
	DrawEnd      func(*CommandBuffer)
}

type StyleEdit struct {
	Normal             StyleItem
	Hover              StyleItem
	Active             StyleItem
	BorderColor        color.RGBA
	Scrollbar          StyleScrollbar
	CursorNormal       color.RGBA
	CursorHover        color.RGBA
	CursorTextNormal   color.RGBA
	CursorTextHover    color.RGBA
	TextNormal         color.RGBA
	TextHover          color.RGBA
	TextActive         color.RGBA
	SelectedNormal     color.RGBA
	SelectedHover      color.RGBA
	SelectedTextNormal color.RGBA
	SelectedTextHover  color.RGBA
	Border             int
	Rounding           uint16
	CursorSize         int
	ScrollbarSize      image.Point
	Padding            image.Point
	RowPadding         int
}

type StyleProperty struct {
	Normal      StyleItem
	Hover       StyleItem
	Active      StyleItem
	BorderColor color.RGBA
	LabelNormal color.RGBA
	LabelHover  color.RGBA
	LabelActive color.RGBA
	SymLeft     SymbolType
	SymRight    SymbolType
	Border      int
	Rounding    uint16
	Padding     image.Point
	Edit        StyleEdit
	IncButton   StyleButton
	DecButton   StyleButton
	DrawBegin   func(*CommandBuffer)
	Draw        func(*CommandBuffer, *StyleProperty, image.Rectangle, image.Rectangle, WidgetStates, string, *Face)
	DrawEnd     func(*CommandBuffer)
}

type StyleChart struct {
	Background    StyleItem
	BorderColor   color.RGBA
	SelectedColor color.RGBA
	Color         color.RGBA
	Border        int
	Rounding      uint16
	Padding       image.Point
}

type StyleCombo struct {
	Normal         StyleItem
	Hover          StyleItem
	Active         StyleItem
	BorderColor    color.RGBA
	LabelNormal    color.RGBA
	LabelHover     color.RGBA
	LabelActive    color.RGBA
	SymbolNormal   color.RGBA
	SymbolHover    color.RGBA
	SymbolActive   color.RGBA
	Button         StyleButton
	SymNormal      SymbolType
	SymHover       SymbolType
	SymActive      SymbolType
	Border         int
	Rounding       uint16
	ContentPadding image.Point
	ButtonPadding  image.Point
	Spacing        image.Point
}

type StyleTab struct {
	Background  StyleItem
	BorderColor color.RGBA
	Text        color.RGBA
	TabButton   StyleButton
	NodeButton  StyleButton
	SymMinimize SymbolType
	SymMaximize SymbolType
	Border      int
	Rounding    uint16
	Padding     image.Point
	Spacing     image.Point
}

type StyleHeaderAlign int

const (
	HeaderLeft StyleHeaderAlign = iota
	HeaderRight
)

type StyleWindowHeader struct {
	Normal         StyleItem
	Hover          StyleItem
	Active         StyleItem
	CloseButton    StyleButton
	MinimizeButton StyleButton
	CloseSymbol    SymbolType
	MinimizeSymbol SymbolType
	MaximizeSymbol SymbolType
	LabelNormal    color.RGBA
	LabelHover     color.RGBA
	LabelActive    color.RGBA
	Align          StyleHeaderAlign
	Padding        image.Point
	LabelPadding   image.Point
	Spacing        image.Point
}

type StyleWindow struct {
	Header          StyleWindowHeader
	FixedBackground StyleItem
	Background      color.RGBA
	BorderColor     color.RGBA
	Scaler          StyleItem
	FooterPadding   image.Point
	Border          int
	Rounding        uint16
	ScalerSize      image.Point
	Padding         image.Point
	Spacing         image.Point
	ScrollbarSize   image.Point
	MinSize         image.Point
}

var defaultColorStyle = []color.RGBA{color.RGBA{175, 175, 175, 255}, color.RGBA{45, 45, 45, 255}, color.RGBA{40, 40, 40, 255}, color.RGBA{65, 65, 65, 255}, color.RGBA{50, 50, 50, 255}, color.RGBA{40, 40, 40, 255}, color.RGBA{35, 35, 35, 255}, color.RGBA{100, 100, 100, 255}, color.RGBA{120, 120, 120, 255}, color.RGBA{45, 45, 45, 255}, color.RGBA{45, 45, 45, 255}, color.RGBA{35, 35, 35, 255}, color.RGBA{38, 38, 38, 255}, color.RGBA{100, 100, 100, 255}, color.RGBA{120, 120, 120, 255}, color.RGBA{150, 150, 150, 255}, color.RGBA{38, 38, 38, 255}, color.RGBA{38, 38, 38, 255}, color.RGBA{175, 175, 175, 255}, color.RGBA{45, 45, 45, 255}, color.RGBA{120, 120, 120, 255}, color.RGBA{45, 45, 45, 255}, color.RGBA{255, 0, 0, 255}, color.RGBA{40, 40, 40, 255}, color.RGBA{100, 100, 100, 255}, color.RGBA{120, 120, 120, 255}, color.RGBA{150, 150, 150, 255}, color.RGBA{40, 40, 40, 255}}

var nk_color_names = []string{"ColorText", "ColorWindow", "ColorHeader", "ColorBorder", "ColorButton", "ColorButtonHover", "ColorButtonActive", "ColorToggle", "ColorToggleHover", "ColorToggleCursor", "ColorSelect", "ColorSelectActive", "ColorSlider", "ColorSliderCursor", "ColorSliderCursorHover", "ColorSliderCursorActive", "ColorProperty", "ColorEdit", "ColorEditCursor", "ColorCombo", "ColorChart", "ColorChartColor", "ColorChartColorHighlight", "ColorScrollbar", "ColorScrollbarCursor", "ColorScrollbarCursorHover", "ColorScrollbarCursorActive", "ColorTabHeader"}

func StyleFromTable(table []color.RGBA) *Style {
	var text *StyleText
	var button *StyleButton
	var toggle *StyleToggle
	var select_ *StyleSelectable
	var slider *StyleSlider
	var prog *StyleProgress
	var scroll *StyleScrollbar
	var edit *StyleEdit
	var property *StyleProperty
	var combo *StyleCombo
	var tab *StyleTab
	var win *StyleWindow

	style := &Style{}
	if table == nil {
		table = defaultColorStyle[:]
	}

	/* default text */
	text = &style.Text

	text.Color = table[ColorText]
	text.Padding = image.Point{4, 4}

	/* default button */
	button = &style.Button

	*button = StyleButton{}
	button.Normal = MakeStyleItemColor(table[ColorButton])
	button.Hover = MakeStyleItemColor(table[ColorButtonHover])
	button.Active = MakeStyleItemColor(table[ColorButtonActive])
	button.BorderColor = table[ColorBorder]
	button.TextBackground = table[ColorButton]
	button.TextNormal = table[ColorText]
	button.TextHover = table[ColorText]
	button.TextActive = table[ColorText]
	button.Padding = image.Point{4.0, 4.0}
	button.ImagePadding = image.Point{0.0, 0.0}
	button.TouchPadding = image.Point{0.0, 0.0}
	button.TextAlignment = TextCentered
	button.Border = 1
	button.Rounding = 4
	button.DrawBegin = nil
	button.DrawEnd = nil

	/* contextual button */
	button = &style.ContextualButton

	*button = StyleButton{}
	button.Normal = MakeStyleItemColor(table[ColorWindow])
	button.Hover = MakeStyleItemColor(table[ColorButtonHover])
	button.Active = MakeStyleItemColor(table[ColorButtonActive])
	button.BorderColor = table[ColorWindow]
	button.TextBackground = table[ColorWindow]
	button.TextNormal = table[ColorText]
	button.TextHover = table[ColorText]
	button.TextActive = table[ColorText]
	button.Padding = image.Point{4.0, 4.0}
	button.TouchPadding = image.Point{0.0, 0.0}
	button.TextAlignment = TextCentered
	button.Border = 0
	button.Rounding = 0
	button.DrawBegin = nil
	button.DrawEnd = nil

	/* menu button */
	button = &style.MenuButton

	*button = StyleButton{}
	button.Normal = MakeStyleItemColor(table[ColorWindow])
	button.Hover = MakeStyleItemColor(table[ColorWindow])
	button.Active = MakeStyleItemColor(table[ColorWindow])
	button.BorderColor = table[ColorWindow]
	button.TextBackground = table[ColorWindow]
	button.TextNormal = table[ColorText]
	button.TextHover = table[ColorText]
	button.TextActive = table[ColorText]
	button.Padding = image.Point{4.0, 4.0}
	button.TouchPadding = image.Point{0.0, 0.0}
	button.TextAlignment = TextCentered
	button.Border = 0
	button.Rounding = 1
	button.DrawBegin = nil
	button.DrawEnd = nil

	/* checkbox toggle */
	toggle = &style.Checkbox

	*toggle = StyleToggle{}
	toggle.Normal = MakeStyleItemColor(table[ColorToggle])
	toggle.Hover = MakeStyleItemColor(table[ColorToggleHover])
	toggle.Active = MakeStyleItemColor(table[ColorToggleHover])
	toggle.CursorNormal = MakeStyleItemColor(table[ColorToggleCursor])
	toggle.CursorHover = MakeStyleItemColor(table[ColorToggleCursor])
	toggle.TextBackground = table[ColorWindow]
	toggle.TextNormal = table[ColorText]
	toggle.TextHover = table[ColorText]
	toggle.TextActive = table[ColorText]
	toggle.Padding = image.Point{4.0, 4.0}
	toggle.TouchPadding = image.Point{0, 0}

	/* option toggle */
	toggle = &style.Option

	*toggle = StyleToggle{}
	toggle.Normal = MakeStyleItemColor(table[ColorToggle])
	toggle.Hover = MakeStyleItemColor(table[ColorToggleHover])
	toggle.Active = MakeStyleItemColor(table[ColorToggleHover])
	toggle.CursorNormal = MakeStyleItemColor(table[ColorToggleCursor])
	toggle.CursorHover = MakeStyleItemColor(table[ColorToggleCursor])
	toggle.TextBackground = table[ColorWindow]
	toggle.TextNormal = table[ColorText]
	toggle.TextHover = table[ColorText]
	toggle.TextActive = table[ColorText]
	toggle.Padding = image.Point{4.0, 4.0}
	toggle.TouchPadding = image.Point{0, 0}

	/* selectable */
	select_ = &style.Selectable

	*select_ = StyleSelectable{}
	select_.Normal = MakeStyleItemColor(table[ColorSelect])
	select_.Hover = MakeStyleItemColor(table[ColorSelect])
	select_.Pressed = MakeStyleItemColor(table[ColorSelect])
	select_.NormalActive = MakeStyleItemColor(table[ColorSelectActive])
	select_.HoverActive = MakeStyleItemColor(table[ColorSelectActive])
	select_.PressedActive = MakeStyleItemColor(table[ColorSelectActive])
	select_.TextNormal = table[ColorText]
	select_.TextHover = table[ColorText]
	select_.TextPressed = table[ColorText]
	select_.TextNormalActive = table[ColorText]
	select_.TextHoverActive = table[ColorText]
	select_.TextPressedActive = table[ColorText]
	select_.Padding = image.Point{4.0, 4.0}
	select_.TouchPadding = image.Point{0, 0}
	select_.Rounding = 0.0
	select_.DrawBegin = nil
	select_.Draw = nil
	select_.DrawEnd = nil

	/* slider */
	slider = &style.Slider

	*slider = StyleSlider{}
	slider.Normal = StyleItemHide()
	slider.Hover = StyleItemHide()
	slider.Active = StyleItemHide()
	slider.BarNormal = table[ColorSlider]
	slider.BarHover = table[ColorSlider]
	slider.BarActive = table[ColorSlider]
	slider.BarFilled = table[ColorSliderCursor]
	slider.CursorNormal = MakeStyleItemColor(table[ColorSliderCursor])
	slider.CursorHover = MakeStyleItemColor(table[ColorSliderCursorHover])
	slider.CursorActive = MakeStyleItemColor(table[ColorSliderCursorActive])
	slider.IncSymbol = SymbolTriangleRight
	slider.DecSymbol = SymbolTriangleLeft
	slider.CursorSize = image.Point{16, 16}
	slider.Padding = image.Point{4, 4}
	slider.Spacing = image.Point{4, 4}
	slider.ShowButtons = false
	slider.BarHeight = 8
	slider.Rounding = 0
	slider.DrawBegin = nil
	slider.Draw = nil
	slider.DrawEnd = nil

	/* slider buttons */
	button = &style.Slider.IncButton

	button.Normal = MakeStyleItemColor(color.RGBA{40, 40, 40, 0xff})
	button.Hover = MakeStyleItemColor(color.RGBA{42, 42, 42, 0xff})
	button.Active = MakeStyleItemColor(color.RGBA{44, 44, 44, 0xff})
	button.BorderColor = color.RGBA{65, 65, 65, 0xff}
	button.TextBackground = color.RGBA{40, 40, 40, 0xff}
	button.TextNormal = color.RGBA{175, 175, 175, 0xff}
	button.TextHover = color.RGBA{175, 175, 175, 0xff}
	button.TextActive = color.RGBA{175, 175, 175, 0xff}
	button.Padding = image.Point{8.0, 8.0}
	button.TouchPadding = image.Point{0.0, 0.0}
	button.TextAlignment = TextCentered
	button.Border = 1.0
	button.Rounding = 0.0
	button.DrawBegin = nil
	button.DrawEnd = nil
	style.Slider.DecButton = style.Slider.IncButton

	/* progressbar */
	prog = &style.Progress

	*prog = StyleProgress{}
	prog.Normal = MakeStyleItemColor(table[ColorSlider])
	prog.Hover = MakeStyleItemColor(table[ColorSlider])
	prog.Active = MakeStyleItemColor(table[ColorSlider])
	prog.CursorNormal = MakeStyleItemColor(table[ColorSliderCursor])
	prog.CursorHover = MakeStyleItemColor(table[ColorSliderCursorHover])
	prog.CursorActive = MakeStyleItemColor(table[ColorSliderCursorActive])
	prog.Padding = image.Point{4, 4}
	prog.Rounding = 0
	prog.DrawBegin = nil
	prog.Draw = nil
	prog.DrawEnd = nil

	/* scrollbars */
	scroll = &style.Scrollh

	*scroll = StyleScrollbar{}
	scroll.Normal = MakeStyleItemColor(table[ColorScrollbar])
	scroll.Hover = MakeStyleItemColor(table[ColorScrollbar])
	scroll.Active = MakeStyleItemColor(table[ColorScrollbar])
	scroll.CursorNormal = MakeStyleItemColor(table[ColorScrollbarCursor])
	scroll.CursorHover = MakeStyleItemColor(table[ColorScrollbarCursorHover])
	scroll.CursorActive = MakeStyleItemColor(table[ColorScrollbarCursorActive])
	scroll.DecSymbol = SymbolCircleFilled
	scroll.IncSymbol = SymbolCircleFilled
	scroll.BorderColor = color.RGBA{65, 65, 65, 0xff}
	scroll.Padding = image.Point{4, 4}
	scroll.ShowButtons = false
	scroll.Border = 0
	scroll.Rounding = 0
	scroll.DrawBegin = nil
	scroll.Draw = nil
	scroll.DrawEnd = nil
	style.Scrollv = style.Scrollh

	/* scrollbars buttons */
	button = &style.Scrollh.IncButton

	button.Normal = MakeStyleItemColor(color.RGBA{40, 40, 40, 0xff})
	button.Hover = MakeStyleItemColor(color.RGBA{42, 42, 42, 0xff})
	button.Active = MakeStyleItemColor(color.RGBA{44, 44, 44, 0xff})
	button.BorderColor = color.RGBA{65, 65, 65, 0xff}
	button.TextBackground = color.RGBA{40, 40, 40, 0xff}
	button.TextNormal = color.RGBA{175, 175, 175, 0xff}
	button.TextHover = color.RGBA{175, 175, 175, 0xff}
	button.TextActive = color.RGBA{175, 175, 175, 0xff}
	button.Padding = image.Point{4.0, 4.0}
	button.TouchPadding = image.Point{0.0, 0.0}
	button.TextAlignment = TextCentered
	button.Border = 1.0
	button.Rounding = 0.0
	button.DrawBegin = nil
	button.DrawEnd = nil
	style.Scrollh.DecButton = style.Scrollh.IncButton
	style.Scrollv.IncButton = style.Scrollh.IncButton
	style.Scrollv.DecButton = style.Scrollh.IncButton

	/* edit */
	edit = &style.Edit

	*edit = StyleEdit{}
	edit.Normal = MakeStyleItemColor(table[ColorEdit])
	edit.Hover = MakeStyleItemColor(table[ColorEdit])
	edit.Active = MakeStyleItemColor(table[ColorEdit])
	edit.Scrollbar = *scroll
	edit.CursorNormal = table[ColorText]
	edit.CursorHover = table[ColorText]
	edit.CursorTextNormal = table[ColorEdit]
	edit.CursorTextHover = table[ColorEdit]
	edit.BorderColor = table[ColorBorder]
	edit.TextNormal = table[ColorText]
	edit.TextHover = table[ColorText]
	edit.TextActive = table[ColorText]
	edit.SelectedNormal = table[ColorText]
	edit.SelectedHover = table[ColorText]
	edit.SelectedTextNormal = table[ColorEdit]
	edit.SelectedTextHover = table[ColorEdit]
	edit.RowPadding = 2
	edit.Padding = image.Point{4, 4}
	edit.ScrollbarSize = image.Point{4, 4}
	edit.CursorSize = 4
	edit.Border = 1
	edit.Rounding = 0

	/* property */
	property = &style.Property

	*property = StyleProperty{}
	property.Normal = MakeStyleItemColor(table[ColorProperty])
	property.Hover = MakeStyleItemColor(table[ColorProperty])
	property.Active = MakeStyleItemColor(table[ColorProperty])
	property.BorderColor = table[ColorBorder]
	property.LabelNormal = table[ColorText]
	property.LabelHover = table[ColorText]
	property.LabelActive = table[ColorText]
	property.SymLeft = SymbolTriangleLeft
	property.SymRight = SymbolTriangleRight
	property.Padding = image.Point{4, 4}
	property.Border = 1
	property.Rounding = 10
	property.DrawBegin = nil
	property.Draw = nil
	property.DrawEnd = nil

	/* property buttons */
	button = &style.Property.DecButton

	*button = StyleButton{}
	button.Normal = MakeStyleItemColor(table[ColorProperty])
	button.Hover = MakeStyleItemColor(table[ColorProperty])
	button.Active = MakeStyleItemColor(table[ColorProperty])
	button.BorderColor = color.RGBA{0, 0, 0, 0}
	button.TextBackground = table[ColorProperty]
	button.TextNormal = table[ColorText]
	button.TextHover = table[ColorText]
	button.TextActive = table[ColorText]
	button.Padding = image.Point{0.0, 0.0}
	button.TouchPadding = image.Point{0.0, 0.0}
	button.TextAlignment = TextCentered
	button.Border = 0.0
	button.Rounding = 0.0
	button.DrawBegin = nil
	button.DrawEnd = nil
	style.Property.IncButton = style.Property.DecButton

	/* property edit */
	edit = &style.Property.Edit

	*edit = StyleEdit{}
	edit.Normal = MakeStyleItemColor(table[ColorProperty])
	edit.Hover = MakeStyleItemColor(table[ColorProperty])
	edit.Active = MakeStyleItemColor(table[ColorProperty])
	edit.BorderColor = color.RGBA{0, 0, 0, 0}
	edit.CursorNormal = table[ColorText]
	edit.CursorHover = table[ColorText]
	edit.CursorTextNormal = table[ColorEdit]
	edit.CursorTextHover = table[ColorEdit]
	edit.TextNormal = table[ColorText]
	edit.TextHover = table[ColorText]
	edit.TextActive = table[ColorText]
	edit.SelectedNormal = table[ColorText]
	edit.SelectedHover = table[ColorText]
	edit.SelectedTextNormal = table[ColorEdit]
	edit.SelectedTextHover = table[ColorEdit]
	edit.Padding = image.Point{0, 0}
	edit.CursorSize = 8
	edit.Border = 0
	edit.Rounding = 0

	/* combo */
	combo = &style.Combo

	combo.Normal = MakeStyleItemColor(table[ColorCombo])
	combo.Hover = MakeStyleItemColor(table[ColorCombo])
	combo.Active = MakeStyleItemColor(table[ColorCombo])
	combo.BorderColor = table[ColorBorder]
	combo.LabelNormal = table[ColorText]
	combo.LabelHover = table[ColorText]
	combo.LabelActive = table[ColorText]
	combo.SymNormal = SymbolTriangleDown
	combo.SymHover = SymbolTriangleDown
	combo.SymActive = SymbolTriangleDown
	combo.ContentPadding = image.Point{4, 4}
	combo.ButtonPadding = image.Point{0, 4}
	combo.Spacing = image.Point{4, 0}
	combo.Border = 1
	combo.Rounding = 0

	/* combo button */
	button = &style.Combo.Button

	*button = StyleButton{}
	button.Normal = MakeStyleItemColor(table[ColorCombo])
	button.Hover = MakeStyleItemColor(table[ColorCombo])
	button.Active = MakeStyleItemColor(table[ColorCombo])
	button.BorderColor = color.RGBA{0, 0, 0, 0}
	button.TextBackground = table[ColorCombo]
	button.TextNormal = table[ColorText]
	button.TextHover = table[ColorText]
	button.TextActive = table[ColorText]
	button.Padding = image.Point{2.0, 2.0}
	button.TouchPadding = image.Point{0.0, 0.0}
	button.TextAlignment = TextCentered
	button.Border = 0.0
	button.Rounding = 0.0
	button.DrawBegin = nil
	button.DrawEnd = nil

	/* tab */
	tab = &style.Tab

	tab.Background = MakeStyleItemColor(table[ColorTabHeader])
	tab.BorderColor = table[ColorBorder]
	tab.Text = table[ColorText]
	tab.SymMinimize = SymbolTriangleDown
	tab.SymMaximize = SymbolTriangleRight
	tab.Border = 1
	tab.Rounding = 0
	tab.Padding = image.Point{4, 4}
	tab.Spacing = image.Point{4, 4}

	/* tab button */
	button = &style.Tab.TabButton

	*button = StyleButton{}
	button.Normal = MakeStyleItemColor(table[ColorTabHeader])
	button.Hover = MakeStyleItemColor(table[ColorTabHeader])
	button.Active = MakeStyleItemColor(table[ColorTabHeader])
	button.BorderColor = color.RGBA{0, 0, 0, 0}
	button.TextBackground = table[ColorTabHeader]
	button.TextNormal = table[ColorText]
	button.TextHover = table[ColorText]
	button.TextActive = table[ColorText]
	button.Padding = image.Point{2.0, 2.0}
	button.TouchPadding = image.Point{0.0, 0.0}
	button.TextAlignment = TextCentered
	button.Border = 0.0
	button.Rounding = 0.0
	button.DrawBegin = nil
	button.DrawEnd = nil

	/* node button */
	button = &style.Tab.NodeButton

	*button = StyleButton{}
	button.Normal = MakeStyleItemColor(table[ColorWindow])
	button.Hover = MakeStyleItemColor(table[ColorWindow])
	button.Active = MakeStyleItemColor(table[ColorWindow])
	button.BorderColor = color.RGBA{0, 0, 0, 0}
	button.TextBackground = table[ColorTabHeader]
	button.TextNormal = table[ColorText]
	button.TextHover = table[ColorText]
	button.TextActive = table[ColorText]
	button.Padding = image.Point{2.0, 2.0}
	button.TouchPadding = image.Point{0.0, 0.0}
	button.TextAlignment = TextCentered
	button.Border = 0.0
	button.Rounding = 0.0
	button.DrawBegin = nil
	button.DrawEnd = nil

	/* window header */
	win = &style.NormalWindow

	win.Header.Align = HeaderRight
	win.Header.CloseSymbol = SymbolX
	win.Header.MinimizeSymbol = SymbolMinus
	win.Header.MaximizeSymbol = SymbolPlus
	win.Header.Normal = MakeStyleItemColor(table[ColorHeader])
	win.Header.Hover = MakeStyleItemColor(table[ColorHeader])
	win.Header.Active = MakeStyleItemColor(table[ColorHeader])
	win.Header.LabelNormal = table[ColorText]
	win.Header.LabelHover = table[ColorText]
	win.Header.LabelActive = table[ColorText]
	win.Header.LabelPadding = image.Point{4, 4}
	win.Header.Padding = image.Point{4, 4}
	win.Header.Spacing = image.Point{0, 0}

	/* window header close button */
	button = &style.NormalWindow.Header.CloseButton

	*button = StyleButton{}
	button.Normal = MakeStyleItemColor(table[ColorHeader])
	button.Hover = MakeStyleItemColor(table[ColorHeader])
	button.Active = MakeStyleItemColor(table[ColorHeader])
	button.BorderColor = color.RGBA{0, 0, 0, 0}
	button.TextBackground = table[ColorHeader]
	button.TextNormal = table[ColorText]
	button.TextHover = table[ColorText]
	button.TextActive = table[ColorText]
	button.Padding = image.Point{0.0, 0.0}
	button.TouchPadding = image.Point{0.0, 0.0}
	button.TextAlignment = TextCentered
	button.Border = 0.0
	button.Rounding = 0.0
	button.DrawBegin = nil
	button.DrawEnd = nil

	/* window header minimize button */
	button = &style.NormalWindow.Header.MinimizeButton

	*button = StyleButton{}
	button.Normal = MakeStyleItemColor(table[ColorHeader])
	button.Hover = MakeStyleItemColor(table[ColorHeader])
	button.Active = MakeStyleItemColor(table[ColorHeader])
	button.BorderColor = color.RGBA{0, 0, 0, 0}
	button.TextBackground = table[ColorHeader]
	button.TextNormal = table[ColorText]
	button.TextHover = table[ColorText]
	button.TextActive = table[ColorText]
	button.Padding = image.Point{0.0, 0.0}
	button.TouchPadding = image.Point{0.0, 0.0}
	button.TextAlignment = TextCentered
	button.Border = 0.0
	button.Rounding = 0.0
	button.DrawBegin = nil
	button.DrawEnd = nil

	/* window */
	win.Background = table[ColorWindow]
	win.FixedBackground = MakeStyleItemColor(table[ColorWindow])
	win.BorderColor = table[ColorBorder]
	win.Scaler = MakeStyleItemColor(table[ColorText])
	win.FooterPadding = image.Point{0, 0}
	win.Rounding = 0.0
	win.ScalerSize = image.Point{16, 16}
	win.Padding = image.Point{8, 8}
	win.Spacing = image.Point{4, 4}
	win.ScrollbarSize = image.Point{10, 10}
	win.MinSize = image.Point{64, 64}
	win.Border = 2.0

	style.MenuWindow = style.NormalWindow
	style.TooltipWindow = style.NormalWindow
	style.ComboWindow = style.NormalWindow
	style.ContextualWindow = style.NormalWindow
	style.GroupWindow = style.NormalWindow

	style.MenuWindow.BorderColor = table[ColorBorder]
	style.MenuWindow.Border = 1
	style.MenuWindow.Spacing = image.Point{2, 2}

	style.TooltipWindow.BorderColor = table[ColorBorder]
	style.TooltipWindow.Border = 1
	style.TooltipWindow.Padding = image.Point{2, 2}

	style.ComboWindow.BorderColor = table[ColorBorder]
	style.ComboWindow.Border = 1

	style.ContextualWindow.BorderColor = table[ColorBorder]
	style.ContextualWindow.Border = 1

	style.GroupWindow.BorderColor = table[ColorBorder]
	style.GroupWindow.Border = 1
	style.GroupWindow.Padding = image.Point{2, 2}
	style.GroupWindow.Spacing = image.Point{2, 2}

	return style
}

func MakeStyleItemImage(img *image.RGBA) StyleItem {
	var i StyleItem
	i.Type = StyleItemImage
	i.Data.Image = img
	return i
}

func MakeStyleItemColor(col color.RGBA) StyleItem {
	var i StyleItem
	i.Type = StyleItemColor
	i.Data.Color = col
	return i
}

func StyleItemHide() StyleItem {
	var i StyleItem
	i.Type = StyleItemColor
	i.Data.Color = color.RGBA{0, 0, 0, 0}
	return i
}

type Theme int

const (
	DefaultTheme Theme = iota
	WhiteTheme
	RedTheme
	DarkTheme
)

var whiteThemeTable = make([]color.RGBA, ColorCount)
var redThemeTable = make([]color.RGBA, ColorCount)
var darkThemeTable = make([]color.RGBA, ColorCount)

func init() {
	whiteThemeTable[ColorText] = color.RGBA{70, 70, 70, 255}
	whiteThemeTable[ColorWindow] = color.RGBA{175, 175, 175, 255}
	whiteThemeTable[ColorHeader] = color.RGBA{175, 175, 175, 255}
	whiteThemeTable[ColorBorder] = color.RGBA{0, 0, 0, 255}
	whiteThemeTable[ColorButton] = color.RGBA{185, 185, 185, 255}
	whiteThemeTable[ColorButtonHover] = color.RGBA{170, 170, 170, 255}
	whiteThemeTable[ColorButtonActive] = color.RGBA{160, 160, 160, 255}
	whiteThemeTable[ColorToggle] = color.RGBA{150, 150, 150, 255}
	whiteThemeTable[ColorToggleHover] = color.RGBA{120, 120, 120, 255}
	whiteThemeTable[ColorToggleCursor] = color.RGBA{175, 175, 175, 255}
	whiteThemeTable[ColorSelect] = color.RGBA{190, 190, 190, 255}
	whiteThemeTable[ColorSelectActive] = color.RGBA{175, 175, 175, 255}
	whiteThemeTable[ColorSlider] = color.RGBA{190, 190, 190, 255}
	whiteThemeTable[ColorSliderCursor] = color.RGBA{80, 80, 80, 255}
	whiteThemeTable[ColorSliderCursorHover] = color.RGBA{70, 70, 70, 255}
	whiteThemeTable[ColorSliderCursorActive] = color.RGBA{60, 60, 60, 255}
	whiteThemeTable[ColorProperty] = color.RGBA{175, 175, 175, 255}
	whiteThemeTable[ColorEdit] = color.RGBA{150, 150, 150, 255}
	whiteThemeTable[ColorEditCursor] = color.RGBA{0, 0, 0, 255}
	whiteThemeTable[ColorCombo] = color.RGBA{175, 175, 175, 255}
	whiteThemeTable[ColorChart] = color.RGBA{160, 160, 160, 255}
	whiteThemeTable[ColorChartColor] = color.RGBA{45, 45, 45, 255}
	whiteThemeTable[ColorChartColorHighlight] = color.RGBA{255, 0, 0, 255}
	whiteThemeTable[ColorScrollbar] = color.RGBA{180, 180, 180, 255}
	whiteThemeTable[ColorScrollbarCursor] = color.RGBA{140, 140, 140, 255}
	whiteThemeTable[ColorScrollbarCursorHover] = color.RGBA{150, 150, 150, 255}
	whiteThemeTable[ColorScrollbarCursorActive] = color.RGBA{160, 160, 160, 255}
	whiteThemeTable[ColorTabHeader] = color.RGBA{180, 180, 180, 255}

	redThemeTable[ColorText] = color.RGBA{190, 190, 190, 255}
	redThemeTable[ColorWindow] = color.RGBA{30, 33, 40, 215}
	redThemeTable[ColorHeader] = color.RGBA{181, 45, 69, 220}
	redThemeTable[ColorBorder] = color.RGBA{51, 55, 67, 255}
	redThemeTable[ColorButton] = color.RGBA{181, 45, 69, 255}
	redThemeTable[ColorButtonHover] = color.RGBA{190, 50, 70, 255}
	redThemeTable[ColorButtonActive] = color.RGBA{195, 55, 75, 255}
	redThemeTable[ColorToggle] = color.RGBA{51, 55, 67, 255}
	redThemeTable[ColorToggleHover] = color.RGBA{45, 60, 60, 255}
	redThemeTable[ColorToggleCursor] = color.RGBA{181, 45, 69, 255}
	redThemeTable[ColorSelect] = color.RGBA{51, 55, 67, 255}
	redThemeTable[ColorSelectActive] = color.RGBA{181, 45, 69, 255}
	redThemeTable[ColorSlider] = color.RGBA{51, 55, 67, 255}
	redThemeTable[ColorSliderCursor] = color.RGBA{181, 45, 69, 255}
	redThemeTable[ColorSliderCursorHover] = color.RGBA{186, 50, 74, 255}
	redThemeTable[ColorSliderCursorActive] = color.RGBA{191, 55, 79, 255}
	redThemeTable[ColorProperty] = color.RGBA{51, 55, 67, 255}
	redThemeTable[ColorEdit] = color.RGBA{51, 55, 67, 225}
	redThemeTable[ColorEditCursor] = color.RGBA{190, 190, 190, 255}
	redThemeTable[ColorCombo] = color.RGBA{51, 55, 67, 255}
	redThemeTable[ColorChart] = color.RGBA{51, 55, 67, 255}
	redThemeTable[ColorChartColor] = color.RGBA{170, 40, 60, 255}
	redThemeTable[ColorChartColorHighlight] = color.RGBA{255, 0, 0, 255}
	redThemeTable[ColorScrollbar] = color.RGBA{30, 33, 40, 255}
	redThemeTable[ColorScrollbarCursor] = color.RGBA{64, 84, 95, 255}
	redThemeTable[ColorScrollbarCursorHover] = color.RGBA{70, 90, 100, 255}
	redThemeTable[ColorScrollbarCursorActive] = color.RGBA{75, 95, 105, 255}
	redThemeTable[ColorTabHeader] = color.RGBA{181, 45, 69, 220}

	darkThemeTable[ColorText] = color.RGBA{210, 210, 210, 255}
	darkThemeTable[ColorWindow] = color.RGBA{57, 67, 71, 255}
	darkThemeTable[ColorHeader] = color.RGBA{51, 51, 56, 220}
	darkThemeTable[ColorBorder] = color.RGBA{46, 46, 46, 255}
	darkThemeTable[ColorButton] = color.RGBA{48, 83, 111, 255}
	darkThemeTable[ColorButtonHover] = color.RGBA{58, 93, 121, 255}
	darkThemeTable[ColorButtonActive] = color.RGBA{63, 98, 126, 255}
	darkThemeTable[ColorToggle] = color.RGBA{50, 58, 61, 255}
	darkThemeTable[ColorToggleHover] = color.RGBA{45, 53, 56, 255}
	darkThemeTable[ColorToggleCursor] = color.RGBA{48, 83, 111, 255}
	darkThemeTable[ColorSelect] = color.RGBA{57, 67, 61, 255}
	darkThemeTable[ColorSelectActive] = color.RGBA{48, 83, 111, 255}
	darkThemeTable[ColorSlider] = color.RGBA{50, 58, 61, 255}
	darkThemeTable[ColorSliderCursor] = color.RGBA{48, 83, 111, 245}
	darkThemeTable[ColorSliderCursorHover] = color.RGBA{53, 88, 116, 255}
	darkThemeTable[ColorSliderCursorActive] = color.RGBA{58, 93, 121, 255}
	darkThemeTable[ColorProperty] = color.RGBA{50, 58, 61, 255}
	darkThemeTable[ColorEdit] = color.RGBA{50, 58, 61, 225}
	darkThemeTable[ColorEditCursor] = color.RGBA{210, 210, 210, 255}
	darkThemeTable[ColorCombo] = color.RGBA{50, 58, 61, 255}
	darkThemeTable[ColorChart] = color.RGBA{50, 58, 61, 255}
	darkThemeTable[ColorChartColor] = color.RGBA{48, 83, 111, 255}
	darkThemeTable[ColorChartColorHighlight] = color.RGBA{255, 0, 0, 255}
	darkThemeTable[ColorScrollbar] = color.RGBA{50, 58, 61, 255}
	darkThemeTable[ColorScrollbarCursor] = color.RGBA{48, 83, 111, 255}
	darkThemeTable[ColorScrollbarCursorHover] = color.RGBA{53, 88, 116, 255}
	darkThemeTable[ColorScrollbarCursorActive] = color.RGBA{58, 93, 121, 255}
	darkThemeTable[ColorTabHeader] = color.RGBA{48, 83, 111, 255}
}

func StyleFromTheme(theme Theme) *Style {
	switch theme {
	case DefaultTheme:
		fallthrough
	default:
		return StyleFromTable(nil)
	case WhiteTheme:
		return StyleFromTable(whiteThemeTable)
	case RedTheme:
		return StyleFromTable(redThemeTable)
	case DarkTheme:
		return StyleFromTable(darkThemeTable)
	}
}

func (w *MasterWindow) Style() (style *Style, scaling float64) {
	return &w.ctx.Style, w.ctx.Scaling
}

func (mw *MasterWindow) SetStyle(style *Style, ff *Face, scaling float64) {
	mw.ctx.Style = *style

	if ff == nil {
		ff = DefaultFont(12, scaling)
	}
	mw.ctx.Style.Font = ff
	mw.ctx.Scaling = scaling

	scale := func(x *int) {
		*x = int(float64(*x) * scaling)
	}

	scaleu := func(x *uint16) {
		*x = uint16(float64(*x) * scaling)
	}

	scalept := func(p *image.Point) {
		if scaling == 1.0 {
			return
		}
		scale(&p.X)
		scale(&p.Y)
	}

	scalebtn := func(button *StyleButton) {
		scalept(&button.Padding)
		scalept(&button.ImagePadding)
		scalept(&button.TouchPadding)
		scale(&button.Border)
		scaleu(&button.Rounding)
	}

	z := &mw.ctx.Style

	scalept(&z.Text.Padding)

	scalebtn(&z.Button)
	scalebtn(&z.ContextualButton)
	scalebtn(&z.MenuButton)

	scalept(&z.Checkbox.Padding)
	scalept(&z.Checkbox.TouchPadding)

	scalept(&z.Option.Padding)
	scalept(&z.Option.TouchPadding)

	scalept(&z.Selectable.Padding)
	scalept(&z.Selectable.TouchPadding)
	scaleu(&z.Selectable.Rounding)

	scalept(&z.Slider.CursorSize)
	scalept(&z.Slider.Padding)
	scalept(&z.Slider.Spacing)
	scaleu(&z.Slider.Rounding)
	scale(&z.Slider.BarHeight)

	scalebtn(&z.Slider.IncButton)

	scalept(&z.Progress.Padding)
	scaleu(&z.Progress.Rounding)

	scalept(&z.Scrollh.Padding)
	scale(&z.Scrollh.Border)
	scaleu(&z.Scrollh.Rounding)

	scalebtn(&z.Scrollh.IncButton)
	scalebtn(&z.Scrollh.DecButton)
	scalebtn(&z.Scrollv.IncButton)
	scalebtn(&z.Scrollv.DecButton)

	scaleedit := func(edit *StyleEdit) {
		scale(&edit.RowPadding)
		scalept(&edit.Padding)
		scalept(&edit.ScrollbarSize)
		scale(&edit.CursorSize)
		scale(&edit.Border)
		scaleu(&edit.Rounding)
	}

	scaleedit(&z.Edit)

	scalept(&z.Property.Padding)
	scale(&z.Property.Border)
	scaleu(&z.Property.Rounding)

	scalebtn(&z.Property.IncButton)
	scalebtn(&z.Property.DecButton)

	scaleedit(&z.Property.Edit)

	scalept(&z.Combo.ContentPadding)
	scalept(&z.Combo.ButtonPadding)
	scalept(&z.Combo.Spacing)
	scale(&z.Combo.Border)
	scaleu(&z.Combo.Rounding)

	scalebtn(&z.Combo.Button)

	scale(&z.Tab.Border)
	scaleu(&z.Tab.Rounding)
	scalept(&z.Tab.Padding)
	scalept(&z.Tab.Spacing)

	scalebtn(&z.Tab.TabButton)
	scalebtn(&z.Tab.NodeButton)

	scalewin := func(win *StyleWindow) {
		scalept(&win.Header.Padding)
		scalept(&win.Header.Spacing)
		scalept(&win.Header.LabelPadding)
		scalebtn(&win.Header.CloseButton)
		scalebtn(&win.Header.MinimizeButton)
		scalept(&win.FooterPadding)
		scaleu(&win.Rounding)
		scalept(&win.ScalerSize)
		scalept(&win.Padding)
		scalept(&win.Spacing)
		scalept(&win.ScrollbarSize)
		scalept(&win.MinSize)
		scale(&win.Border)
	}

	scalewin(&z.NormalWindow)
	scalewin(&z.MenuWindow)
	scalewin(&z.TooltipWindow)
	scalewin(&z.ComboWindow)
	scalewin(&z.ContextualWindow)
	scalewin(&z.GroupWindow)
}
