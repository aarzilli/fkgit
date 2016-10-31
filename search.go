package main

import (
	"strings"
	"unicode"

	"github.com/aarzilli/nucular"

	"golang.org/x/mobile/event/key"
)

type Searcher struct {
	searching    bool
	searched     nucular.TextEditor
	searchNeedle string
	searchSel    SearchSel
}

type SearchSel struct {
	Pos        SearchPos
	Start, End int
}

type SearchPos interface {
	BeforeFirst() SearchPos
	Next() (SearchPos, bool)
	Text() string
}

func (s *Searcher) Events(w *nucular.Window, scrollToSearch *bool) {
	for _, e := range w.Input().Keyboard.Keys {
		switch {
		case (e.Modifiers == key.ModControl) && (e.Code == key.CodeF):
			s.searching = true
			s.searched.Buffer = s.searched.Buffer[:0]
			s.searchNeedle = "-DIFFERENT-"
			s.searched.Flags = nucular.EditSelectable | nucular.EditClipboard | nucular.EditSigEnter
			s.searchSel.Pos = s.searchSel.Pos.BeforeFirst()
			s.searchSel.Start, s.searchSel.End = 0, 0
			w.Master().ActivateEditor(&s.searched)
			*scrollToSearch = true
		case (e.Modifiers == key.ModControl) && (e.Code == key.CodeG):
			if !s.searching {
				w.Master().ActivateEditor(&s.searched)
			}
			s.searching = true
			s.searchSel = Lookfwd(s.searchSel, s.searchNeedle, true)
			*scrollToSearch = true
		case (e.Modifiers == 0) && (e.Code == key.CodeEscape):
			s.searching = false
		}
	}
}

func (s *Searcher) Update(w *nucular.Window, scrollToSearch *bool) {

	w.Row(20).Static(100, 0)
	w.Label("Search", "LC")
	active := s.searched.Edit(w)
	if active&nucular.EditCommitted != 0 {
		s.searching = false
		w.Master().Changed()
	}
	if needle := string(s.searched.Buffer); needle != s.searchNeedle {
		s.searchSel = Lookfwd(s.searchSel, needle, false)
		s.searchNeedle = needle
		*scrollToSearch = true
	}

}

func (s *Searcher) Sel() *SearchSel {
	if !s.searching {
		return nil
	}
	return &s.searchSel
}

func Lookfwd(sel SearchSel, needle string, advance bool) SearchSel {
	first := true
	idx := sel.Start
	if advance {
		idx = sel.End
	}
	insensitive := true
	for _, ch := range needle {
		if unicode.IsUpper(ch) {
			insensitive = false
			break
		}
	}
	for {
		if !first {
			var ok bool
			sel.Pos, ok = sel.Pos.Next()
			if !ok {
				return SearchSel{sel.Pos.BeforeFirst(), 0, 0}
			}
			idx = 0
		}
		first = false
		txt := sel.Pos.Text()
		if insensitive {
			//XXX this is wrong for languages where changing case changes the number of runes
			txt = strings.ToLower(txt)
		}
		if len(txt) < 0 || idx >= len(txt) {
			continue
		}
		if i := strings.Index(txt[idx:], needle); i >= 0 {
			sel.Start = i + idx
			sel.End = sel.Start + len(needle)
			return sel
		}
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
