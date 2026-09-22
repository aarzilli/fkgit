package main

import (
	"github.com/aarzilli/nucular"

	"golang.org/x/mobile/event/key"
)

type Searcher struct {
	searching    bool
	searched     nucular.TextEditor
	searchNeedle string
	Reset        func()
	Look         func(needle string, advance bool)
}

func (s *Searcher) Events(w *nucular.Window) {
	for e := range w.Input().Keyboard.Events() {
		switch {
		case e.HandleKey(key.CodeF, key.ModControl):
			s.searching = true
			s.searched.Buffer = s.searched.Buffer[:0]
			s.searchNeedle = "-DIFFERENT-"
			s.searched.Flags = nucular.EditSelectable | nucular.EditClipboard | nucular.EditSigEnter
			w.Master().ActivateEditor(w, &s.searched)
			s.Reset()
		case e.HandleKey(key.CodeG, key.ModControl):
			if !s.searching {
				w.Master().ActivateEditor(w, &s.searched)
			}
			s.searching = true
			s.Look(s.searchNeedle, true)
		case e.HandleKey(key.CodeEscape, 0):
			s.searching = false
		}
	}
}

func (s *Searcher) Update(w *nucular.Window) {
	w.Row(20).Static(100, 0)
	w.Label("Search", "LC")
	active := s.searched.Edit(w)
	if active&nucular.EditCommitted != 0 {
		s.searching = false
		w.Master().Changed()
	}
	if needle := string(s.searched.Buffer); needle != s.searchNeedle {
		s.searchNeedle = needle
		s.Look(s.searchNeedle, false)
	}

}

// Not actually related to search, implements standard scrolling behavior
func scrollingKeys(w *nucular.Window, lnh int) {
	kbd := w.KeyboardOnHover(w.Bounds)
	for e := range kbd.Events() {
		switch {
		case e.HandleKey(key.CodeHome, 0):
			w.Scrollbar.X = 0
			w.Scrollbar.Y = 0
		case e.HandleKey(key.CodeEnd, 0):
			w.Scrollbar.X = 0
			w.Scrollbar.Y = w.At().Y
		case e.HandleKey(key.CodeUpArrow, 0):
			w.Scrollbar.Y -= lnh
		case e.HandleKey(key.CodeDownArrow, 0):
			w.Scrollbar.Y += lnh
		case e.HandleKey(key.CodeLeftArrow, 0):
			w.Scrollbar.X -= w.Bounds.W / 10
		case e.HandleKey(key.CodeRightArrow, 0):
			w.Scrollbar.X += w.Bounds.W / 10
		case e.HandleKey(key.CodePageUp, 0):
			w.Scrollbar.Y -= w.Bounds.H / 2
		case e.HandleKey(key.CodePageDown, 0):
			w.Scrollbar.Y += w.Bounds.H / 2
		}

		if w.Scrollbar.Y < 0 {
			w.Scrollbar.Y = 0
		}
		if w.Scrollbar.X < 0 {
			w.Scrollbar.X = 0
		}
	}
}
