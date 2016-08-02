package nucular

import (
	"bytes"
	"image"

	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
)

type mouseButton struct {
	Down       bool
	Clicked    bool
	ClickedPos image.Point
}

type MouseInput struct {
	Buttons     [4]mouseButton
	Pos         image.Point
	Prev        image.Point
	Delta       image.Point
	ScrollDelta int
}

type KeyboardInput struct {
	Keys []key.Event
	Text *bytes.Buffer
}

type Input struct {
	Keyboard KeyboardInput
	Mouse    MouseInput
}

func (w *Window) Input() *Input {
	if !w.toplevel() {
		return &Input{Keyboard: KeyboardInput{Keys: []key.Event{}}}
	}
	return &w.ctx.Input
}

func (w *Window) KeyboardOnHover(bounds Rect) KeyboardInput {
	if !w.toplevel() || !InputIsMouseHoveringRect(&w.ctx.Input, bounds) {
		return KeyboardInput{Keys: []key.Event{}}
	}
	return w.ctx.Input.Keyboard
}

func InputHasMouseClickInRect(i *Input, id mouse.Button, b Rect) bool {
	if i == nil {
		return false
	}
	btn := &i.Mouse.Buttons[id]
	if !b.Contains(btn.ClickedPos) {
		return false
	}
	return true
}

func InputHasMouseClickDownInRect(i *Input, id mouse.Button, b Rect, down bool) bool {
	if i == nil {
		return false
	}
	btn := &i.Mouse.Buttons[id]
	return InputHasMouseClickInRect(i, id, b) && (btn.Down == down)
}

func InputIsMouseClickInRect(i *Input, id mouse.Button, b Rect) bool {
	if i == nil {
		return false
	}
	btn := &i.Mouse.Buttons[id]
	return InputHasMouseClickDownInRect(i, id, b, false) && btn.Clicked
}

func InputIsMouseClickDownInRect(i *Input, id mouse.Button, b Rect, down bool) bool {
	if i == nil {
		return false
	}
	btn := &i.Mouse.Buttons[id]
	return InputHasMouseClickDownInRect(i, id, b, down) && btn.Clicked
}

func InputAnyMouseClickInRect(in *Input, b Rect) bool {
	if in == nil {
		return false
	}
	return InputIsMouseClickInRect(in, mouse.ButtonLeft, b) || InputIsMouseClickInRect(in, mouse.ButtonMiddle, b) || InputIsMouseClickInRect(in, mouse.ButtonRight, b)
}

func InputIsMouseHoveringRect(i *Input, rect Rect) bool {
	if i == nil {
		return false
	}
	return rect.Contains(i.Mouse.Pos)
}

func InputIsMousePrevHoveringRect(i *Input, rect Rect) bool {
	if i == nil {
		return false
	}
	return rect.Contains(i.Mouse.Prev)
}

func InputMouseClicked(i *Input, id mouse.Button, rect Rect) bool {
	if i == nil {
		return false
	}
	if !InputIsMouseHoveringRect(i, rect) {
		return false
	}
	return InputIsMouseClickInRect(i, id, rect)
}

func InputIsMouseDown(i *Input, id mouse.Button) bool {
	if i == nil {
		return false
	}
	return i.Mouse.Buttons[id].Down
}

func InputIsMousePressed(i *Input, id mouse.Button) bool {
	if i == nil {
		return false
	}
	b := &i.Mouse.Buttons[id]
	return b.Down && b.Clicked
}

func InputIsMouseReleased(i *Input, id mouse.Button) bool {
	if i == nil {
		return false
	}
	return !(i.Mouse.Buttons[id].Down) && i.Mouse.Buttons[id].Clicked
}

func InputIsKeyPressed(i *Input, key key.Code) bool {
	if i == nil {
		return false
	}
	for _, k := range i.Keyboard.Keys {
		if k.Code == key {
			return true
		}
	}
	return false
}

func (win *Window) inputMaybe(state widgetLayoutStates) *Input {
	if state != widgetRom && win.toplevel() {
		return &win.ctx.Input
	}
	return nil
}

func (win *Window) toplevel() bool {
	return win.idx == len(win.ctx.Windows)-1
}
