// +build nucular_gio

package nucular

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/profile"
	"gioui.org/io/system"
	"gioui.org/op"
	gioclip "gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/aarzilli/nucular/command"
	"github.com/aarzilli/nucular/font"
	"github.com/aarzilli/nucular/rect"

	"golang.org/x/image/math/fixed"
	mkey "golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
)

type masterWindow struct {
	masterWindowCommon

	Title       string
	initialSize image.Point
	size        image.Point

	w   *app.Window
	ops op.Ops

	textbuffer bytes.Buffer

	closed bool
}

func NewMasterWindowSize(flags WindowFlags, title string, sz image.Point, updatefn UpdateFn) MasterWindow {
	ctx := &context{}
	wnd := &masterWindow{}

	wnd.masterWindowCommonInit(ctx, flags, updatefn, wnd)

	wnd.Title = title
	wnd.initialSize = sz

	return wnd
}

func (mw *masterWindow) Main() {
	go func() {
		mw.w = app.NewWindow(app.Title(mw.Title), app.Size(unit.Px(float32(mw.ctx.scale(mw.initialSize.X))), unit.Px(float32(mw.ctx.scale(mw.initialSize.Y)))))
		mw.main()
	}()
	go mw.updater()
	app.Main()
}

func (mw *masterWindow) Lock() {
	mw.uilock.Lock()
}

func (mw *masterWindow) Unlock() {
	mw.uilock.Unlock()
}

func (mw *masterWindow) Close() {
	os.Exit(0) // Bad...
}

func (mw *masterWindow) Closed() bool {
	mw.uilock.Lock()
	defer mw.uilock.Unlock()
	return mw.closed
}

func (mw *masterWindow) main() {
	for {
		e := <-mw.w.Events()
		switch e := e.(type) {
		case system.DestroyEvent:
			mw.uilock.Lock()
			mw.closed = true
			mw.uilock.Unlock()
			if e.Err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", e.Err)
			}
			return

		case system.FrameEvent:
			mw.size = e.Size
			mw.uilock.Lock()
			mw.prevCmds = mw.prevCmds[:0]
			mw.updateLocked()
			mw.uilock.Unlock()

			e.Frame(&mw.ops)
		}
	}
}

func (mw *masterWindow) processPointerEvent(e pointer.Event) {
	switch e.Type {
	case pointer.Release, pointer.Cancel:
		for i := range mw.ctx.Input.Mouse.Buttons {
			btn := &mw.ctx.Input.Mouse.Buttons[i]
			btn.Down = false
			btn.Clicked = true
		}

	case pointer.Press:
		var button mouse.Button

		switch {
		case e.Buttons.Contain(pointer.ButtonLeft):
			button = mouse.ButtonLeft
		case e.Buttons.Contain(pointer.ButtonRight):
			button = mouse.ButtonRight
		case e.Buttons.Contain(pointer.ButtonMiddle):
			button = mouse.ButtonMiddle
		}

		down := e.Type == pointer.Press
		btn := &mw.ctx.Input.Mouse.Buttons[button]
		if btn.Down == down {
			break
		}

		if down {
			btn.ClickedPos.X = int(e.Position.X)
			btn.ClickedPos.Y = int(e.Position.Y)
		}
		btn.Clicked = true
		btn.Down = down

	case pointer.Move:
		mw.ctx.Input.Mouse.Pos.X = int(e.Position.X)
		mw.ctx.Input.Mouse.Pos.Y = int(e.Position.Y)
		mw.ctx.Input.Mouse.Delta = mw.ctx.Input.Mouse.Pos.Sub(mw.ctx.Input.Mouse.Prev)

		if e.Scroll.Y < 0 {
			mw.ctx.Input.Mouse.ScrollDelta++
		} else if e.Scroll.Y > 0 {
			mw.ctx.Input.Mouse.ScrollDelta--
		}
	}
}

var runeToCode = map[string]mkey.Code{}

func init() {
	for i := byte('a'); i <= 'z'; i++ {
		c := mkey.Code((i - 'a') + 4)
		runeToCode[string([]byte{i})] = c
		runeToCode[string([]byte{i - 0x20})] = c
	}

	runeToCode["\t"] = mkey.CodeTab
	runeToCode[" "] = mkey.CodeSpacebar
	runeToCode["-"] = mkey.CodeHyphenMinus
	runeToCode["="] = mkey.CodeEqualSign
	runeToCode["["] = mkey.CodeLeftSquareBracket
	runeToCode["]"] = mkey.CodeRightSquareBracket
	runeToCode["\\"] = mkey.CodeBackslash
	runeToCode[";"] = mkey.CodeSemicolon
	runeToCode["\""] = mkey.CodeApostrophe
	runeToCode["`"] = mkey.CodeGraveAccent
	runeToCode[","] = mkey.CodeComma
	runeToCode["."] = mkey.CodeFullStop
	runeToCode["/"] = mkey.CodeSlash

	runeToCode[key.NameLeftArrow] = mkey.CodeLeftArrow
	runeToCode[key.NameRightArrow] = mkey.CodeRightArrow
	runeToCode[key.NameUpArrow] = mkey.CodeUpArrow
	runeToCode[key.NameDownArrow] = mkey.CodeDownArrow
	runeToCode[key.NameReturn] = mkey.CodeReturnEnter
	runeToCode[key.NameEnter] = mkey.CodeReturnEnter
	runeToCode[key.NameEscape] = mkey.CodeEscape
	runeToCode[key.NameHome] = mkey.CodeHome
	runeToCode[key.NameEnd] = mkey.CodeEnd
	runeToCode[key.NameDeleteBackward] = mkey.CodeDeleteBackspace
	runeToCode[key.NameDeleteForward] = mkey.CodeDeleteForward
	runeToCode[key.NamePageUp] = mkey.CodePageUp
	runeToCode[key.NamePageDown] = mkey.CodePageDown
}

func gio2mobileKey(e key.Event) mkey.Event {
	var mod mkey.Modifiers

	if e.Modifiers.Contain(key.ModCommand) {
		mod |= mkey.ModMeta
	}
	if e.Modifiers.Contain(key.ModCtrl) {
		mod |= mkey.ModControl
	}
	if e.Modifiers.Contain(key.ModAlt) {
		mod |= mkey.ModAlt
	}
	if e.Modifiers.Contain(key.ModSuper) {
		mod |= mkey.ModMeta
	}

	var name rune

	for _, ch := range e.Name {
		name = ch
		break
	}

	return mkey.Event{
		Rune:      name,
		Code:      runeToCode[e.Name],
		Modifiers: mod,
		Direction: mkey.DirRelease,
	}
}

func (w *masterWindow) updater() {
	var down bool
	for {
		if down {
			time.Sleep(10 * time.Millisecond)
		} else {
			time.Sleep(20 * time.Millisecond)
		}
		func() {
			w.uilock.Lock()
			defer w.uilock.Unlock()
			if w.closed {
				return
			}
			changed := atomic.LoadInt32(&w.ctx.changed)
			if changed > 0 {
				atomic.AddInt32(&w.ctx.changed, -1)
				w.w.Invalidate()
			}
		}()
	}
}

func (mw *masterWindow) updateLocked() {
	perfString := ""
	q := mw.w.Queue()
	for _, e := range q.Events(mw.ctx) {
		switch e := e.(type) {
		case profile.Event:
			perfString = e.Timings
		case pointer.Event:
			mw.processPointerEvent(e)
		case key.EditEvent:
			io.WriteString(&mw.textbuffer, e.Text)

		case key.Event:
			mw.ctx.Input.Keyboard.Keys = append(mw.ctx.Input.Keyboard.Keys, gio2mobileKey(e))
		}
	}

	mw.ctx.Windows[0].Bounds = rect.Rect{X: 0, Y: 0, W: mw.size.X, H: mw.size.Y}
	in := &mw.ctx.Input
	in.Mouse.clip = nk_null_rect
	in.Keyboard.Text = mw.textbuffer.String()
	mw.textbuffer.Reset()

	var t0, t1, te time.Time
	if perfUpdate || mw.Perf {
		t0 = time.Now()
	}

	if dumpFrame && !perfUpdate {
		panic("dumpFrame")
	}

	mw.ctx.Update()

	if perfUpdate || mw.Perf {
		t1 = time.Now()
	}
	nprimitives := mw.draw()
	if perfUpdate && nprimitives > 0 {
		te = time.Now()

		fps := 1.0 / te.Sub(t0).Seconds()

		fmt.Printf("Update %0.4f msec = %0.4f updatefn + %0.4f draw (%d primitives) [max fps %0.2f]\n", te.Sub(t0).Seconds()*1000, t1.Sub(t0).Seconds()*1000, te.Sub(t1).Seconds()*1000, nprimitives, fps)
	}
	if mw.Perf && nprimitives > 0 {
		te = time.Now()
		fps := 1.0 / te.Sub(t0).Seconds()

		s := fmt.Sprintf("%0.4fms + %0.4fms (%0.2f)\n%s", t1.Sub(t0).Seconds()*1000, te.Sub(t1).Seconds()*1000, fps, perfString)

		font := mw.Style().Font
		txt := fontFace2fontFace(&font).layout(s, -1)

		bounds := image.Point{X: maxLinesWidth(txt.Lines), Y: (txt.Lines[0].Ascent + txt.Lines[0].Descent).Ceil() * 2}

		pos := mw.size
		pos.Y -= bounds.Y
		pos.X -= bounds.X

		paintRect := f32.Rectangle{f32.Point{float32(pos.X), float32(pos.Y)}, f32.Point{float32(pos.X + bounds.X), float32(pos.Y + bounds.Y)}}

		var stack op.StackOp
		stack.Push(&mw.ops)
		paint.ColorOp{Color: color.RGBA{0xff, 0xff, 0xff, 0xff}}.Add(&mw.ops)
		paint.PaintOp{Rect: paintRect}.Add(&mw.ops)
		stack.Pop()

		drawText(&mw.ops, txt, font, color.RGBA{0x00, 0x00, 0x00, 0xff}, pos, bounds, paintRect)
	}
}

func (w *masterWindow) draw() int {
	if !w.drawChanged() {
		return 0
	}

	w.prevCmds = append(w.prevCmds[:0], w.ctx.cmds...)

	return w.ctx.Draw(&w.ops, w.size, w.Perf)
}

func (ctx *context) Draw(ops *op.Ops, size image.Point, perf bool) int {
	ops.Reset()

	if perf {
		profile.Op{ctx}.Add(ops)
	}
	pointer.InputOp{ctx, false}.Add(ops)
	key.InputOp{ctx, true}.Add(ops)

	var scissorStack op.StackOp
	scissorless := true

	for i := range ctx.cmds {
		icmd := &ctx.cmds[i]
		switch icmd.Kind {
		case command.ScissorCmd:
			if !scissorless {
				scissorStack.Pop()
			}
			scissorStack.Push(ops)
			gioclip.Rect{Rect: n2fRect(icmd.Rect)}.Op(ops).Add(ops)
			scissorless = false

		case command.LineCmd:
			cmd := icmd.Line

			var stack op.StackOp
			stack.Push(ops)
			paint.ColorOp{Color: cmd.Color}.Add(ops)

			h1 := int(cmd.LineThickness / 2)
			h2 := int(cmd.LineThickness) - h1

			if cmd.Begin.X == cmd.End.X {
				y0, y1 := cmd.Begin.Y, cmd.End.Y
				if y0 > y1 {
					y0, y1 = y1, y0
				}
				paint.PaintOp{Rect: f32.Rectangle{
					f32.Point{float32(cmd.Begin.X - h1), float32(y0)},
					f32.Point{float32(cmd.Begin.X + h2), float32(y1)}}}.Add(ops)
			} else if cmd.Begin.Y == cmd.End.Y {
				x0, x1 := cmd.Begin.X, cmd.End.X
				if x0 > x1 {
					x0, x1 = x1, x0
				}
				paint.PaintOp{Rect: f32.Rectangle{
					f32.Point{float32(x0), float32(cmd.Begin.Y - h1)},
					f32.Point{float32(x1), float32(cmd.Begin.Y + h2)}}}.Add(ops)
			} else {
				m := float32(cmd.Begin.Y-cmd.End.Y) / float32(cmd.Begin.X-cmd.End.X)
				invm := -1 / m

				xadv := float32(math.Sqrt(float64(cmd.LineThickness*cmd.LineThickness) / (4 * float64((invm*invm + 1)))))
				yadv := xadv * invm

				var p gioclip.Path
				p.Begin(ops)

				pa := f32.Point{float32(cmd.Begin.X) - xadv, float32(cmd.Begin.Y) - yadv}
				p.Move(pa)
				pb := f32.Point{2 * xadv, 2 * yadv}
				p.Line(pb)
				pc := f32.Point{float32(cmd.End.X - cmd.Begin.X), float32(cmd.End.Y - cmd.Begin.Y)}
				p.Line(pc)
				pd := f32.Point{-2 * xadv, -2 * yadv}
				p.Line(pd)
				p.Line(f32.Point{float32(cmd.Begin.X - cmd.End.X), float32(cmd.Begin.Y - cmd.End.Y)})

				p.End().Add(ops)

				pb = pb.Add(pa)
				pc = pc.Add(pb)
				pd = pd.Add(pc)

				minp := f32.Point{
					min4(pa.X, pb.X, pc.X, pd.X),
					min4(pa.Y, pb.Y, pc.Y, pd.Y)}
				maxp := f32.Point{
					max4(pa.X, pb.X, pc.X, pd.X),
					max4(pa.Y, pb.Y, pc.Y, pd.Y)}

				paint.PaintOp{Rect: f32.Rectangle{minp, maxp}}.Add(ops)
			}

			stack.Pop()

		case command.RectFilledCmd:
			cmd := icmd.RectFilled
			// rounding is true if rounding has been requested AND we can draw it
			rounding := cmd.Rounding > 0 && int(cmd.Rounding*2) < icmd.W && int(cmd.Rounding*2) < icmd.H

			var stack op.StackOp
			stack.Push(ops)
			paint.ColorOp{Color: cmd.Color}.Add(ops)

			if rounding {
				const c = 0.55228475 // 4*(sqrt(2)-1)/3

				x, y, w, h := float32(icmd.X), float32(icmd.Y), float32(icmd.W), float32(icmd.H)
				r := float32(cmd.Rounding)

				var b gioclip.Path
				b.Begin(ops)
				b.Move(f32.Point{X: x + w, Y: y + h - r})
				b.Cube(f32.Point{X: 0, Y: r * c}, f32.Point{X: -r + r*c, Y: r}, f32.Point{X: -r, Y: r}) // SE
				b.Line(f32.Point{X: r - w + r, Y: 0})
				b.Cube(f32.Point{X: -r * c, Y: 0}, f32.Point{X: -r, Y: -r + r*c}, f32.Point{X: -r, Y: -r}) // SW
				b.Line(f32.Point{X: 0, Y: r - h + r})
				b.Cube(f32.Point{X: 0, Y: -r * c}, f32.Point{X: r - r*c, Y: -r}, f32.Point{X: r, Y: -r}) // NW
				b.Line(f32.Point{X: w - r - r, Y: 0})
				b.Cube(f32.Point{X: r * c, Y: 0}, f32.Point{X: r, Y: r - r*c}, f32.Point{X: r, Y: r}) // NE
				b.End().Add(ops)
			}

			paint.PaintOp{Rect: n2fRect(icmd.Rect)}.Add(ops)
			stack.Pop()

		case command.TriangleFilledCmd:
			cmd := icmd.TriangleFilled

			var stack op.StackOp
			stack.Push(ops)

			paint.ColorOp{cmd.Color}.Add(ops)

			var p gioclip.Path
			p.Begin(ops)
			p.Move(f32.Point{float32(cmd.A.X), float32(cmd.A.Y)})
			p.Line(f32.Point{float32(cmd.B.X - cmd.A.X), float32(cmd.B.Y - cmd.A.Y)})
			p.Line(f32.Point{float32(cmd.C.X - cmd.B.X), float32(cmd.C.Y - cmd.B.Y)})
			p.Line(f32.Point{float32(cmd.A.X - cmd.C.X), float32(cmd.A.Y - cmd.C.Y)})
			p.End().Add(ops)

			pmin := f32.Point{
				min2(min2(float32(cmd.A.X), float32(cmd.B.X)), float32(cmd.C.X)),
				min2(min2(float32(cmd.A.Y), float32(cmd.B.Y)), float32(cmd.C.Y))}

			pmax := f32.Point{
				max2(max2(float32(cmd.A.X), float32(cmd.B.X)), float32(cmd.C.X)),
				max2(max2(float32(cmd.A.Y), float32(cmd.B.Y)), float32(cmd.C.Y))}

			paint.PaintOp{Rect: f32.Rectangle{pmin, pmax}}.Add(ops)

			stack.Pop()

		case command.CircleFilledCmd:
			var stack op.StackOp
			stack.Push(ops)

			paint.ColorOp{icmd.CircleFilled.Color}.Add(ops)

			r := min2(float32(icmd.W), float32(icmd.H)) / 2

			const c = 0.55228475 // 4*(sqrt(2)-1)/3
			var b gioclip.Path
			b.Begin(ops)
			b.Move(f32.Point{X: float32(icmd.X) + r*2, Y: float32(icmd.Y) + r})
			b.Cube(f32.Point{X: 0, Y: r * c}, f32.Point{X: -r + r*c, Y: r}, f32.Point{X: -r, Y: r})    // SE
			b.Cube(f32.Point{X: -r * c, Y: 0}, f32.Point{X: -r, Y: -r + r*c}, f32.Point{X: -r, Y: -r}) // SW
			b.Cube(f32.Point{X: 0, Y: -r * c}, f32.Point{X: r - r*c, Y: -r}, f32.Point{X: r, Y: -r})   // NW
			b.Cube(f32.Point{X: r * c, Y: 0}, f32.Point{X: r, Y: r - r*c}, f32.Point{X: r, Y: r})      // NE
			b.End().Add(ops)

			paint.PaintOp{Rect: n2fRect(icmd.Rect)}.Add(ops)

			stack.Pop()

		case command.ImageCmd:
			var stack op.StackOp
			stack.Push(ops)
			icmd.Image.Img.Add(ops)
			paint.PaintOp{n2fRect(icmd.Rect)}.Add(ops)
			stack.Pop()

		case command.TextCmd:
			txt := fontFace2fontFace(&icmd.Text.Face).layout(icmd.Text.String, -1)
			if len(txt.Lines) <= 0 {
				continue
			}

			bounds := image.Point{X: maxLinesWidth(txt.Lines), Y: (txt.Lines[0].Ascent + txt.Lines[0].Descent).Ceil()}
			if bounds.X > icmd.W {
				bounds.X = icmd.W
			}
			if bounds.Y > icmd.H {
				bounds.Y = icmd.H
			}

			drawText(ops, txt, icmd.Text.Face, icmd.Text.Foreground, image.Point{icmd.X, icmd.Y}, bounds, n2fRect(icmd.Rect))

		default:
			panic(UnknownCommandErr)
		}
	}

	return len(ctx.cmds)
}

func n2fRect(r rect.Rect) f32.Rectangle {
	return f32.Rectangle{
		Min: f32.Point{float32(r.X), float32(r.Y)},
		Max: f32.Point{float32(r.X + r.W), float32(r.Y + r.H)}}
}

func i2fRect(r image.Rectangle) f32.Rectangle {
	return f32.Rectangle{
		Min: f32.Point{X: float32(r.Min.X), Y: float32(r.Min.Y)},
		Max: f32.Point{X: float32(r.Max.X), Y: float32(r.Max.Y)}}
}

func min4(a, b, c, d float32) float32 {
	return min2(min2(a, b), min2(c, d))
}

func min2(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max4(a, b, c, d float32) float32 {
	return max2(max2(a, b), max2(c, d))
}

func max2(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func textPadding(lines []text.Line) (padding image.Rectangle) {
	if len(lines) == 0 {
		return
	}
	first := lines[0]
	if d := first.Ascent + first.Bounds.Min.Y; d < 0 {
		padding.Min.Y = d.Ceil()
	}
	last := lines[len(lines)-1]
	if d := last.Bounds.Max.Y - last.Descent; d > 0 {
		padding.Max.Y = d.Ceil()
	}
	if d := first.Bounds.Min.X; d < 0 {
		padding.Min.X = d.Ceil()
	}
	if d := first.Bounds.Max.X - first.Width; d > 0 {
		padding.Max.X = d.Ceil()
	}
	return
}

func clipLine(line text.Line, clip image.Rectangle) (text.String, f32.Point, bool) {
	off := fixed.Point26_6{X: fixed.I(0), Y: fixed.I(line.Ascent.Ceil())}
	str := line.Text
	for len(str.Advances) > 0 {
		adv := str.Advances[0]
		if (off.X + adv + line.Bounds.Max.X - line.Width).Ceil() >= clip.Min.X {
			break
		}
		off.X += adv
		_, s := utf8.DecodeRuneInString(str.String)
		str.String = str.String[s:]
		str.Advances = str.Advances[1:]
	}
	n := 0
	endx := off.X
	for i, adv := range str.Advances {
		if (endx + line.Bounds.Min.X).Floor() > clip.Max.X {
			str.String = str.String[:n]
			str.Advances = str.Advances[:i]
			break
		}
		_, s := utf8.DecodeRuneInString(str.String[n:])
		n += s
		endx += adv
	}
	offf := f32.Point{X: float32(off.X) / 64, Y: float32(off.Y) / 64}
	return str, offf, true
}

func maxLinesWidth(lines []text.Line) int {
	w := 0
	for _, line := range lines {
		if line.Width.Ceil() > w {
			w = line.Width.Ceil()
		}
	}
	return w
}

func drawText(ops *op.Ops, txt *text.Layout, face font.Face, fgcolor color.RGBA, pos, bounds image.Point, paintRect f32.Rectangle) {
	clip := textPadding(txt.Lines)
	clip.Max = clip.Max.Add(bounds)

	var stack op.StackOp
	stack.Push(ops)
	paint.ColorOp{fgcolor}.Add(ops)

	fc := fontFace2fontFace(&face)

	for i := range txt.Lines {
		txtstr, off, ok := clipLine(txt.Lines[i], clip)
		if !ok {
			continue
		}

		off.X += float32(pos.X)
		off.Y += float32(pos.Y) + float32(i*FontHeight(face))

		var stack op.StackOp
		stack.Push(ops)

		op.TransformOp{}.Offset(off).Add(ops)
		fc.shape(txtstr).Add(ops)

		paint.PaintOp{Rect: paintRect.Sub(off)}.Add(ops)

		stack.Pop()
	}

	stack.Pop()
}
