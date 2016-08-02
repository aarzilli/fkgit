package nucular

import (
	"image"
	"image/color"
)

// CommandBuffer is a list of drawing directives.
type CommandBuffer struct {
	UseClipping bool
	Clip        Rect
	Commands    []Command
}

func (buffer *CommandBuffer) reset() {
	buffer.UseClipping = true
	buffer.Clip = nk_null_rect
	buffer.Commands = []Command{}
}

// Represents one drawing directive.
type Command interface {
	command()
}

type CommandScissor struct {
	Rect
}

func (c *CommandScissor) command() {}

type CommandLine struct {
	LineThickness uint16
	Begin         image.Point
	End           image.Point
	Color         color.RGBA
}

func (c *CommandLine) command() {}

type CommandRectFilled struct {
	Rect
	Rounding uint16
	Color    color.RGBA
}

func (c *CommandRectFilled) command() {}

type CommandTriangleFilled struct {
	A     image.Point
	B     image.Point
	C     image.Point
	Color color.RGBA
}

func (c *CommandTriangleFilled) command() {}

type CommandCircleFilled struct {
	Rect
	Color color.RGBA
}

func (c *CommandCircleFilled) command() {}

type CommandImage struct {
	Rect
	Img *image.RGBA
}

func (c *CommandImage) command() {}

type CommandText struct {
	Rect
	Font       *Face
	Background color.RGBA
	Foreground color.RGBA
	String     string
}

func (c *CommandText) command() {}

func (b *CommandBuffer) PushScissor(r Rect) {
	cmd := &CommandScissor{}

	b.Clip = r

	b.Commands = append(b.Commands, cmd)

	cmd.Rect = r
}

func (b *CommandBuffer) StrokeLine(p0, p1 image.Point, line_thickness int, c color.RGBA) {
	cmd := &CommandLine{}
	b.Commands = append(b.Commands, cmd)
	cmd.LineThickness = uint16(line_thickness)
	cmd.Begin = p0
	cmd.End = p1
	cmd.Color = c
}

func (b *CommandBuffer) FillRect(rect Rect, rounding uint16, c color.RGBA) {
	cmd := &CommandRectFilled{}
	if c.A == 0 {
		return
	}
	if b.UseClipping {
		if !rect.Intersect(&b.Clip) {
			return
		}
	}

	b.Commands = append(b.Commands, cmd)
	cmd.Rounding = rounding
	cmd.Rect = rect
	cmd.Color = c
}

func (b *CommandBuffer) FillCircle(r Rect, c color.RGBA) {
	cmd := &CommandCircleFilled{}
	if c.A == 0 {
		return
	}
	if b.UseClipping {
		if !r.Intersect(&b.Clip) {
			return
		}
	}

	b.Commands = append(b.Commands, cmd)
	cmd.Rect = r
	cmd.Color = c
}

func (b *CommandBuffer) FillTriangle(p0, p1, p2 image.Point, c color.RGBA) {
	cmd := &CommandTriangleFilled{}
	if c.A == 0 {
		return
	}
	if b.UseClipping {
		if !b.Clip.Contains(p0) || !b.Clip.Contains(p1) || !b.Clip.Contains(p2) {
			return
		}
	}

	b.Commands = append(b.Commands, cmd)
	cmd.A = p0
	cmd.B = p1
	cmd.C = p2
	cmd.Color = c
}

func (b *CommandBuffer) DrawImage(r Rect, img *image.RGBA) {
	cmd := &CommandImage{}
	if b.UseClipping {
		if !r.Intersect(&b.Clip) {
			return
		}
	}

	b.Commands = append(b.Commands, cmd)
	cmd.Rect = r
	cmd.Img = img
}

func (b *CommandBuffer) DrawText(r Rect, str string, font *Face, bg color.RGBA, fg color.RGBA) {
	cmd := &CommandText{}

	if len(str) == 0 || (bg.A == 0 && fg.A == 0) {
		return
	}
	if b.UseClipping {
		if !r.Intersect(&b.Clip) {
			return
		}
	}

	// make sure text fits inside bounds
	text_width := fontWidth(font, str)

	if text_width > r.W {
		str = string(textClamp(font, []rune(str), r.W))
	}

	if len(str) == 0 {
		return
	}
	b.Commands = append(b.Commands, cmd)
	cmd.Rect = r
	cmd.Background = bg
	cmd.Foreground = fg
	cmd.Font = font
	cmd.String = str
}

type drawParams interface {
	Draw(style *Style, out *CommandBuffer)
}

type frozenWidget struct {
	ws     WidgetStates
	bounds Rect
	//drawParams drawParams
}

type widgetBuffer struct {
	win         *Window
	Clip        Rect
	UseClipping bool
	cur         []frozenWidget
	prev        []frozenWidget
}

func (wbuf *widgetBuffer) PrevState(bounds Rect) WidgetStates {
	for i := range wbuf.prev {
		if wbuf.prev[i].bounds == bounds {
			return wbuf.prev[i].ws
		}
	}
	return WidgetStateInactive
}

func (wbuf *widgetBuffer) Add(ws WidgetStates, bounds Rect, drawParams drawParams) {
	if drawParams != nil {
		drawParams.Draw(&wbuf.win.ctx.Style, &wbuf.win.cmds)
	}
	wbuf.cur = append(wbuf.cur, frozenWidget{ws, bounds})
}

// func (wbuf *widgetBuffer) Run(style *Style, out *CommandBuffer) int {
// 	for i := range wbuf.cur {
// 		wbuf.cur[i].drawParams.Draw(style, out)
// 	}
// 	return len(wbuf.cur)
// }

func (wbuf *widgetBuffer) reset() {
	wbuf.Clip = nk_null_rect
	wbuf.prev = wbuf.cur
	wbuf.cur = []frozenWidget{}
}
