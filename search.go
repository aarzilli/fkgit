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
	for _, e := range w.Input().Keyboard.Keys {
		switch {
		case (e.Modifiers == key.ModControl) && (e.Code == key.CodeF):
			s.searching = true
			s.searched.Buffer = s.searched.Buffer[:0]
			s.searchNeedle = "-DIFFERENT-"
			s.searched.Flags = nucular.EditSelectable | nucular.EditClipboard | nucular.EditSigEnter
			w.Master().ActivateEditor(&s.searched)
			s.Reset()
		case (e.Modifiers == key.ModControl) && (e.Code == key.CodeG):
			if !s.searching {
				w.Master().ActivateEditor(&s.searched)
			}
			s.searching = true
			s.Look(s.searchNeedle, true)
		case (e.Modifiers == 0) && (e.Code == key.CodeEscape):
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
	for _, e := range w.KeyboardOnHover(w.Bounds).Keys {
		switch {
		case (e.Modifiers == 0) && (e.Code == key.CodeHome):
			w.Scrollbar.X = 0
			w.Scrollbar.Y = 0
		case (e.Modifiers == 0) && (e.Code == key.CodeEnd):
			w.Scrollbar.X = 0
			w.Scrollbar.Y = w.At().Y
		case (e.Modifiers == 0) && (e.Code == key.CodeUpArrow):
			w.Scrollbar.Y -= lnh
		case (e.Modifiers == 0) && (e.Code == key.CodeDownArrow):
			w.Scrollbar.Y += lnh
		case (e.Modifiers == 0) && (e.Code == key.CodeLeftArrow):
			w.Scrollbar.X -= w.Bounds.W / 10
		case (e.Modifiers == 0) && (e.Code == key.CodeRightArrow):
			w.Scrollbar.X += w.Bounds.W / 10
		case (e.Modifiers == 0) && (e.Code == key.CodePageUp):
			w.Scrollbar.Y -= w.Bounds.H / 2
		case (e.Modifiers == 0) && (e.Code == key.CodePageDown):
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
