package nucular

import (
	"image"

	"golang.org/x/image/font"
	"golang.org/x/mobile/event/mouse"

	"github.com/golang/freetype/truetype"
)

type SymbolType int

const (
	SymbolNone SymbolType = iota
	SymbolX
	SymbolUnderscore
	SymbolCircle
	SymbolCircleFilled
	SymbolRect
	SymbolRectFilled
	SymbolTriangleUp
	SymbolTriangleDown
	SymbolTriangleLeft
	SymbolTriangleRight
	SymbolPlus
	SymbolMinus
)

type Heading int

const (
	Up Heading = iota
	Right
	Down
	Left
)

type Clipboard interface {
	Paste() string
	Copy(string)
}

type Face struct {
	Face   font.Face
	ttfont *truetype.Font
	Size   int
}

func fontWidth(f *Face, string string) int {
	d := font.Drawer{Face: f.Face}
	return d.MeasureString(string).Ceil()
}

func FontWidth(f *Face, string string) int {
	return fontWidth(f, string)
}

func fontHeight(f *Face) int {
	return f.Size
}

type Rect struct {
	X int
	Y int
	W int
	H int
}

var nk_null_rect = Rect{-8192.0, -8192.0, 16384.0, 16384.0}

func shrinkRect(r Rect, amount int) Rect {
	var res Rect
	r.W = max(r.W, 2*amount)
	r.H = max(r.H, 2*amount)
	res.X = r.X + amount
	res.Y = r.Y + amount
	res.W = r.W - 2*amount
	res.H = r.H - 2*amount
	return res
}

func padRect(r Rect, pad image.Point) Rect {
	r.W = max(r.W, 2*pad.X)
	r.H = max(r.H, 2*pad.Y)
	r.X += pad.X
	r.Y += pad.Y
	r.W -= 2 * pad.X
	r.H -= 2 * pad.Y
	return r
}

func between(x int, a int, b int) bool {
	return a <= x && x <= b
}

func (rect Rect) Contains(p image.Point) bool {
	return between(p.X, rect.X, rect.X+rect.W) && between(p.Y, rect.Y, rect.Y+rect.H)
}

func (r0 *Rect) Intersect(r1 *Rect) bool {
	return r1.X <= (r0.X+r0.W) && (r1.X+r1.W) >= r0.X && r1.Y <= (r0.Y+r0.H) && (r1.Y+r1.H) >= r0.Y
}

func (rect *Rect) Min() image.Point {
	return image.Point{rect.X, rect.Y}
}

func (rect *Rect) Max() image.Point {
	return image.Point{rect.X + rect.W, rect.Y + rect.H}
}

func (rect Rect) Scaled(scaling float64) Rect {
	if scaling == 1.0 {
		return rect
	}
	return Rect{int(float64(rect.X) * scaling), int(float64(rect.Y) * scaling), int(float64(rect.W) * scaling), int(float64(rect.H) * scaling)}
}

func unify(a Rect, b Rect) (clip Rect) {
	clip.X = max(a.X, b.X)
	clip.Y = max(a.Y, b.Y)
	clip.W = min(a.X+a.W, b.X+b.W) - clip.X
	clip.H = min(a.Y+a.H, b.Y+b.H) - clip.Y
	clip.W = max(0.0, clip.W)
	clip.H = max(0.0, clip.H)
	return
}

func FromRectangle(r image.Rectangle) Rect {
	return Rect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}
}

func (r *Rect) Rectangle() image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func assert(cond bool) {
	if !cond {
		panic("assert!")
	}
}

func assert2(cond bool, reason string) {
	if !cond {
		panic(reason)
	}
}

func triangleFromDirection(r Rect, pad_x, pad_y int, direction Heading) (result []image.Point) {
	result = make([]image.Point, 3)
	var w_half int
	var h_half int

	r.W = max(2*pad_x, r.W)
	r.H = max(2*pad_y, r.H)
	r.W = r.W - 2*pad_x
	r.H = r.H - 2*pad_y

	r.X = r.X + pad_x
	r.Y = r.Y + pad_y

	w_half = r.W / 2.0
	h_half = r.H / 2.0

	if direction == Up {
		result[0] = image.Point{r.X + w_half, r.Y}
		result[1] = image.Point{r.X + r.W, r.Y + r.H}
		result[2] = image.Point{r.X, r.Y + r.H}
	} else if direction == Right {
		result[0] = image.Point{r.X, r.Y}
		result[1] = image.Point{r.X + r.W, r.Y + h_half}
		result[2] = image.Point{r.X, r.Y + r.H}
	} else if direction == Down {
		result[0] = image.Point{r.X, r.Y}
		result[1] = image.Point{r.X + r.W, r.Y}
		result[2] = image.Point{r.X + w_half, r.Y + r.H}
	} else {
		result[0] = image.Point{r.X, r.Y + h_half}
		result[1] = image.Point{r.X + r.W, r.Y}
		result[2] = image.Point{r.X + r.W, r.Y + r.H}
	}
	return
}

func minFloat(x, y float64) float64 {
	if x < y {
		return x
	}
	return y
}

func maxFloat(x, y float64) float64 {
	if x > y {
		return x
	}
	return y
}

func clampFloat(i, v, x float64) float64 {
	if v < i {
		v = i
	}
	if v > x {
		v = x
	}
	return v
}

func clampInt(i, v, x int) int {
	if v < i {
		v = i
	}
	if v > x {
		v = x
	}
	return v
}

func saturateFloat(x float64) float64 {
	return maxFloat(0.0, minFloat(1.0, x))
}

func basicWidgetStateControl(state *WidgetStates, in *Input, bounds Rect) WidgetStates {
	if in == nil {
		*state = WidgetStateInactive
		return WidgetStateInactive
	}

	hovering := InputIsMouseHoveringRect(in, bounds)

	if *state == WidgetStateInactive && hovering {
		*state = WidgetStateHovered
	}

	if *state == WidgetStateHovered && !hovering {
		*state = WidgetStateInactive
	}

	if *state == WidgetStateHovered && InputHasMouseClickInRect(in, mouse.ButtonLeft, bounds) {
		*state = WidgetStateActive
	}

	if hovering {
		return WidgetStateHovered
	} else {
		return WidgetStateInactive
	}
}
