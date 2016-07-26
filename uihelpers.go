package main

import (
	"fmt"

	"github.com/aarzilli/nucular"

	"golang.org/x/mobile/event/key"
)

const popupFlags = nucular.WindowMovable | nucular.WindowTitle | nucular.WindowDynamic | nucular.WindowNoScrollbar | nucular.WindowScalable

type MessagePopup struct {
	Title   string
	Message string
}

func (mp *MessagePopup) Update(mw *nucular.MasterWindow) bool {
	w := mw.Wnd
	style, _ := mw.Style()
	lnh := style.Font.Size

	if w.PopupBegin(nucular.PopupStatic, mp.Title, popupFlags, nucular.Rect{20, 100, 640, 500}, true) {
		defer w.PopupEnd()
		w.Popup.LayoutRowDynamicScaled(lnh, 1)
		showLines(w.Popup, mp.Message)
		ok, cancel := okCancelKeys(w.Popup)
		w.Popup.LayoutRowDynamic(25, 1)
		if w.Popup.ButtonText("OK", 0) || ok || cancel {
			w.PopupClose()
			return false
		}
		return true
	} else {
		return false
	}
}

func showLines(w *nucular.Window, s string) {
	start := 0
	for i := range s {
		if s[i] == '\n' {
			w.Label(s[start:i], nucular.TextLeft)
			start = i + 1
		}
	}

	if start < len(s) {
		w.Label(s[start:], nucular.TextLeft)
	}
}

func selectFromList(w *nucular.Window, name string, idx int, list []string) int {
	if sw := w.GroupBegin(name, nucular.WindowNoHScrollbar|nucular.WindowBorder); sw != nil {
		sw.LayoutRowDynamic(25, 1)
		for i := range list {
			selected := idx == i
			sw.SelectableLabel(list[i], nucular.TextLeft, &selected)
			if selected {
				idx = i
			}
		}
		sw.GroupEnd()
	}

	return idx
}

func okCancelKeys(w *nucular.Window) (ok, cancel bool) {
	for _, e := range w.Input().Keyboard.Keys {
		fmt.Printf("key: %v\n", e)
		switch {
		case (e.Modifiers == 0) && (e.Code == key.CodeReturnEnter):
			return true, false
		case (e.Modifiers == 0) && (e.Code == key.CodeEscape):
			return false, true
		}
	}
	return false, false
}

func selectFromListWindow(w *nucular.Window, title, text string, idx int, list []string) (int, bool) {
	if w.PopupBegin(nucular.PopupStatic, title, popupFlags, nucular.Rect{20, 100, 640, 400}, true) {
		defer w.PopupEnd()
		w.Popup.LayoutRowDynamic(25, 1)
		w.Popup.Label(text, nucular.TextLeft)
		w.Popup.LayoutRowDynamic(150, 1)

		idx = selectFromList(w.Popup, title+"-listgroup", idx, list)

		w.Popup.LayoutRowDynamic(25, 2)

		ok, cancel := okCancelKeys(w.Popup)

		if w.Popup.ButtonText("OK", 0) || ok {
			w.PopupClose()
			return idx, true
		}
		if w.Popup.ButtonText("Cancel", 0) || cancel {
			w.PopupClose()
			return -1, true
		}

		w.Popup.LayoutRowDynamic(10, 1)
	} else {
		return -1, true
	}

	return idx, false
}

type NewBranchPopup struct {
	CommitId string
	ed       nucular.TextEditor
	first    bool
}

func (np *NewBranchPopup) Update(mw *nucular.MasterWindow) bool {
	w := mw.Wnd
	if np.first {
		np.ed.Flags = nucular.EditSigEnter | nucular.EditSelectable
		np.ed.Active = true
		np.first = false
	}
	open := w.PopupBegin(nucular.PopupStatic, "New branch...", popupFlags, nucular.Rect{20, 100, 640, 400}, true)
	if open {
		defer w.PopupEnd()
		w.Popup.LayoutRowDynamic(25, 1)
		active := np.ed.Edit(w.Popup, 128, nucular.FilterDefault)
		w.Popup.LayoutRowDynamic(25, 2)
		var ok, cancel bool
		if !np.ed.Active {
			ok, cancel = okCancelKeys(w.Popup)
		}
		if w.Popup.ButtonText("OK", 0) || (active&nucular.EditCommitted != 0) || ok {
			newbranchAction(&lw, string(np.ed.Buffer), np.CommitId)
			w.PopupClose()
			return false
		}
		if w.Popup.ButtonText("Cancel", 0) || cancel {
			w.PopupClose()
			return false
		}
		w.Popup.LayoutRowDynamic(10, 1)
	}

	return open
}

type resetMode int

const (
	resetHard = iota
	resetMixed
	resetSoft
)

type ResetPopup struct {
	CommitId  string
	ResetMode resetMode
}

func (rp *ResetPopup) Update(mw *nucular.MasterWindow) bool {
	w := mw.Wnd
	open := w.PopupBegin(nucular.PopupStatic, "Reset...", popupFlags, nucular.Rect{20, 100, 640, 400}, true)
	if open {
		defer w.PopupEnd()
		w.Popup.LayoutRowDynamic(25, 1)
		if w.Popup.OptionText("Hard: reset working tree and index", rp.ResetMode == resetHard) {
			rp.ResetMode = resetHard
		}
		if w.Popup.OptionText("Mixed: leave working tree untouched, reset index", rp.ResetMode == resetMixed) {
			rp.ResetMode = resetMixed
		}
		if w.Popup.OptionText("Soft: leave working tree and index untouched", rp.ResetMode == resetSoft) {
			rp.ResetMode = resetSoft
		}
		w.Popup.LayoutRowDynamic(25, 2)
		ok, cancel := okCancelKeys(w.Popup)
		if w.Popup.ButtonText("OK", 0) || ok {
			resetAction(&lw, rp.CommitId, rp.ResetMode)
			w.PopupClose()
			return false
		}
		if w.Popup.ButtonText("Cancel", 0) || cancel {
			w.PopupClose()
			return false
		}
		w.Popup.LayoutRowDynamic(10, 1)
	}
	return open
}

type RemotesPopup struct {
	Action  string
	Remotes []string
	Idx     int
}

func (rp *RemotesPopup) Update(mw *nucular.MasterWindow) bool {
	var title, text string

	switch rp.Action {
	case "fetch":
		title = "Fetch..."
		text = "Pick a repository to fetch:"
	case "pull":
		title = "Pull..."
		text = "Pick a repository to pull from:"
	case "push":
		title = "Push..."
		text = "Pick a repository to push to:"
	}

	var done bool
	rp.Idx, done = selectFromListWindow(mw.Wnd, title, text, rp.Idx, rp.Remotes)
	if done {
		if rp.Idx >= 0 {
			remoteAction(&lw, rp.Action, rp.Remotes[rp.Idx])
		}
		return false
	}
	return true
}

type MergePopup struct {
	Refs     []Ref
	Idx      int
	refNames []string
}

func (mp *MergePopup) Update(mw *nucular.MasterWindow) bool {
	if mp.refNames == nil {
		mp.refNames = make([]string, len(mp.Refs))
		for i := range mp.Refs {
			mp.refNames[i] = mp.Refs[i].Nice()
		}
	}
	var done bool
	mp.Idx, done = selectFromListWindow(mw.Wnd, "Merge...", "Select a branch to merge:", mp.Idx, mp.refNames)
	if done {
		if mp.Idx >= 0 {
			mergeAction(&lw, &mp.Refs[mp.Idx])
		}
		return false
	}
	return true
}

type DiffPopup struct {
	Refs      []Ref
	Bookmarks []LanedCommit
	Lc        LanedCommit
	Idx1      int
	Idx2      int
	names     []string
}

func (dp *DiffPopup) idxToCommitOrRef(idx int) (name, id string) {
	if idx == 0 {
		return dp.Lc.NiceWithAbbrev(), dp.Lc.Id
	}

	idx--
	if idx < len(dp.Bookmarks) {
		return dp.Bookmarks[idx].NiceWithAbbrev(), dp.Bookmarks[idx].Id
	}

	idx -= len(dp.Bookmarks)

	return dp.Refs[idx].Nice(), dp.Refs[idx].CommitId
}

func (dp *DiffPopup) Update(mw *nucular.MasterWindow) bool {
	if dp.names == nil {
		dp.names = make([]string, 0, len(dp.Refs)+len(dp.Bookmarks)+1)
		dp.names = append(dp.names, dp.Lc.NiceWithAbbrev())
		for _, lc := range dp.Bookmarks {
			dp.names = append(dp.names, lc.NiceWithAbbrev())
		}
		for _, ref := range dp.Refs {
			dp.names = append(dp.names, ref.Nice())
		}
	}

	w := mw.Wnd
	open := w.PopupBegin(nucular.PopupStatic, "Diff...", popupFlags, nucular.Rect{20, 100, 460, 400}, true)
	if open {
		defer w.PopupEnd()
		w.Popup.LayoutRowDynamic(150, 2)
		dp.Idx1 = selectFromList(w.Popup, "DiffA", dp.Idx1, dp.names)
		dp.Idx2 = selectFromList(w.Popup, "DiffB", dp.Idx2, dp.names)

		w.Popup.LayoutRowDynamic(25, 2)

		ok, cancel := okCancelKeys(w.Popup)

		if w.Popup.ButtonText("OK", 0) || ok {
			w.PopupClose()

			niceNameA, commitOrRefA := dp.idxToCommitOrRef(dp.Idx1)
			niceNameB, commitOrRefB := dp.idxToCommitOrRef(dp.Idx2)

			diffAction(&lw, niceNameA, commitOrRefA, niceNameB, commitOrRefB)

			return false
		}
		if w.Popup.ButtonText("Cancel", 0) || cancel {
			w.PopupClose()
			return false
		}
		w.Popup.LayoutRowDynamic(10, 1)
	}
	return open
}

type CheckoutPopup struct {
	LocalRefs      []Ref
	LocalRefsNames []string
	Idx            int
}

func (cp *CheckoutPopup) Update(mw *nucular.MasterWindow) bool {
	var done bool
	cp.Idx, done = selectFromListWindow(mw.Wnd, "Checkout...", "Pick a branch to checkout:", cp.Idx, cp.LocalRefsNames)
	if done {
		if cp.Idx >= 0 {
			checkoutAction(&lw, &cp.LocalRefs[cp.Idx], "")
		}
		return false
	}
	return true
}

type ForcePushPopup struct {
	Repository string
	Message    string
}

func (fp *ForcePushPopup) Update(mw *nucular.MasterWindow) bool {
	w := mw.Wnd
	style, _ := mw.Style()
	lnh := style.Font.Size

	if w.PopupBegin(nucular.PopupStatic, "Push error", popupFlags, nucular.Rect{20, 100, 640, 500}, true) {
		defer w.PopupEnd()
		nlines := 1
		for i := range fp.Message {
			if fp.Message[i] == '\n' {
				nlines++
			}
		}
		w.Popup.LayoutRowDynamicScaled(lnh, 1)
		showLines(w.Popup, fp.Message)
		_, cancel := okCancelKeys(w.Popup)
		w.Popup.LayoutRowDynamic(25, 2)
		if w.Popup.ButtonText(fmt.Sprintf("Force Push %s", fp.Repository), 0) {
			pushAction(&lw, true, fp.Repository)
			w.PopupClose()
			return false
		}
		if w.Popup.ButtonText("Cancel", 0) || cancel {
			w.PopupClose()
			return false
		}
		return true
	} else {
		return false
	}
}
