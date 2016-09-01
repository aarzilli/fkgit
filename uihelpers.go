package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/rect"

	"golang.org/x/mobile/event/key"
)

const popupFlags = nucular.WindowMovable | nucular.WindowTitle | nucular.WindowNoScrollbar | nucular.WindowDynamic

type messagePopup struct {
	Title string
	ed    nucular.TextEditor
}

func newMessagePopup(mw *nucular.MasterWindow, title, message string) {
	var mp messagePopup
	mp.Title = title
	mp.ed.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse
	mp.ed.Buffer = []rune(message)
	mw.PopupOpen(mp.Title, popupFlags, rect.Rect{20, 100, 480, 500}, true, mp.Update)
}

func (mp *messagePopup) Update(w *nucular.Window) {
	w.Row(200).Dynamic(1)
	mp.ed.Edit(w)
	okCancelButtons(w, true, "OK", false)
}

func showLines(w *nucular.Window, s string) {
	start := 0
	for i := range s {
		if s[i] == '\n' {
			w.Label(s[start:i], "LC")
			start = i + 1
		}
	}

	if start < len(s) {
		w.Label(s[start:], "LC")
	}
}

type selectFromListSearchState struct {
	name string
	last time.Time
	text string
}

var selectFromListSearch selectFromListSearchState

func selectFromList(w *nucular.Window, name string, idx int, list []string) int {
	if sw := w.GroupBegin(name, nucular.WindowNoHScrollbar|nucular.WindowBorder); sw != nil {
		moveselection := false
		kbd := sw.KeyboardOnHover(sw.Bounds)
		for _, k := range kbd.Keys {
			switch {
			case (k.Modifiers == 0) && (k.Code == key.CodeDownArrow):
				idx++
				moveselection = true
			case (k.Modifiers == 0) && (k.Code == key.CodeUpArrow):
				idx--
				moveselection = true
			}
			if idx < 0 {
				idx = 0
			}
			if idx >= len(list) {
				idx = len(list) - 1
			}
		}
		if kbd.Text != "" {
			now := time.Now()
			if selectFromListSearch.name != name || now.Sub(selectFromListSearch.last) > 500*time.Millisecond {
				selectFromListSearch.name = name
				selectFromListSearch.text = ""
			}

			selectFromListSearch.last = now
			selectFromListSearch.text += kbd.Text

			for i := range list {
				if strings.HasPrefix(list[i], selectFromListSearch.text) {
					moveselection = idx != i
					idx = i
					break
				}
			}
		}

		sw.Row(25).Dynamic(1)
		for i := range list {
			selected := idx == i

			if moveselection && selected {
				above, below := sw.Invisible()
				if above || below {
					// recenter around selection
					sw.Scrollbar.Y = sw.At().Y
				}
			}

			sw.SelectableLabel(list[i], "LC", &selected)
			if selected {
				idx = i
			}
		}
		sw.GroupEnd()
	}

	return idx
}

func okCancelButtons(w *nucular.Window, dokeys bool, oktext string, showcancel bool) (ok, cancel bool) {
	if dokeys {
		for _, e := range w.Input().Keyboard.Keys {
			switch {
			case (e.Modifiers == 0) && (e.Code == key.CodeReturnEnter):
				ok, cancel = true, false
			case (e.Modifiers == 0) && (e.Code == key.CodeEscape):
				ok, cancel = false, true
			}
		}
	}

	w.Row(25).Static(0, 100, 100)
	w.Spacing(1)
	if oktext != "" {
		if !showcancel {
			w.Spacing(1)
		}
		if w.ButtonText(oktext) || ok {
			ok = true
			w.Close()
		}
	} else {
		w.Spacing(1)
	}
	if showcancel {
		if w.ButtonText("Cancel") || cancel {
			cancel = true
			w.Close()
		}
	}
	return
}

func selectFromListWindow(mw *nucular.MasterWindow, title, text string, list []string, onSelect func(idx int)) {
	idx := -1
	mw.PopupOpen(title, popupFlags, rect.Rect{20, 100, 480, 400}, true, func(w *nucular.Window) {
		w.Row(25).Dynamic(1)
		w.Label(text, "LC")

		w.Row(150).Dynamic(1)

		idx = selectFromList(w, title+"-listgroup", idx, list)

		ok, _ := okCancelButtons(w, true, "OK", true)
		if ok {
			onSelect(idx)
		}
	})
}

type newBranchPopup struct {
	CommitId string
	ed       nucular.TextEditor
}

func newNewBranchPopup(mw *nucular.MasterWindow, id string) {
	np := newBranchPopup{CommitId: id}
	np.ed.Flags = nucular.EditSigEnter | nucular.EditSelectable
	np.ed.Active = true
	np.ed.Maxlen = 128
	mw.PopupOpen("New branch...", popupFlags, rect.Rect{20, 100, 480, 400}, true, np.Update)

}

func (np *newBranchPopup) Update(w *nucular.Window) {
	w.Row(25).Dynamic(1)
	np.ed.Edit(w)
	ok, _ := okCancelButtons(w, !np.ed.Active, "OK", true)
	if ok {
		newbranchAction(&lw, string(np.ed.Buffer), np.CommitId)
	}
}

type resetMode int

const (
	resetHard = iota
	resetMixed
	resetSoft
)

type resetPopup struct {
	CommitId  string
	ResetMode resetMode
}

func newResetPopup(mw *nucular.MasterWindow, id string, mode resetMode) {
	rp := resetPopup{CommitId: id, ResetMode: mode}
	mw.PopupOpen("Reset...", popupFlags, rect.Rect{20, 100, 480, 400}, true, rp.Update)
}

func (rp *resetPopup) Update(w *nucular.Window) {
	w.Row(25).Dynamic(1)
	if w.OptionText("Hard: reset working tree and index", rp.ResetMode == resetHard) {
		rp.ResetMode = resetHard
	}
	if w.OptionText("Mixed: leave working tree untouched, reset index", rp.ResetMode == resetMixed) {
		rp.ResetMode = resetMixed
	}
	if w.OptionText("Soft: leave working tree and index untouched", rp.ResetMode == resetSoft) {
		rp.ResetMode = resetSoft
	}
	ok, _ := okCancelButtons(w, true, "OK", true)
	if ok {
		resetAction(&lw, rp.CommitId, rp.ResetMode)
	}
}

func newRemotesPopup(mw *nucular.MasterWindow, action string, remotes []string) {
	var title, text string

	switch action {
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

	selectFromListWindow(mw, title, text, remotes, func(idx int) {
		if idx >= 0 {
			remoteAction(&lw, action, remotes[idx])
		}
	})
}

func newMergePopup(mw *nucular.MasterWindow, allrefs []Ref) {
	refnames := make([]string, len(allrefs))
	for i := range allrefs {
		refnames[i] = allrefs[i].Nice()
	}

	selectFromListWindow(mw, "Merge...", "Select a branch to merge:", refnames, func(idx int) {
		if idx >= 0 {
			mergeAction(&lw, &allrefs[idx])
		}
	})

}

type diffPopup struct {
	Refs      []Ref
	Bookmarks []LanedCommit
	Lc        LanedCommit
	Idx1      int
	Idx2      int
	names     []string
}

func newDiffPopup(mw *nucular.MasterWindow, refs []Ref, bookmarks []LanedCommit, lc LanedCommit) {
	dp := diffPopup{refs, bookmarks, lc, -1, -1, nil}
	dp.names = make([]string, 0, len(dp.Refs)+len(dp.Bookmarks)+1)
	dp.names = append(dp.names, dp.Lc.NiceWithAbbrev())
	for _, lc := range dp.Bookmarks {
		dp.names = append(dp.names, lc.NiceWithAbbrev())
	}
	for _, ref := range dp.Refs {
		dp.names = append(dp.names, ref.Nice())
	}
	mw.PopupOpen("Diff...", popupFlags, rect.Rect{20, 100, 480, 400}, true, dp.Update)
}

func (dp *diffPopup) idxToCommitOrRef(idx int) (name, id string) {
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

func (dp *diffPopup) Update(w *nucular.Window) {
	w.Row(150).Dynamic(2)
	dp.Idx1 = selectFromList(w, "DiffA", dp.Idx1, dp.names)
	dp.Idx2 = selectFromList(w, "DiffB", dp.Idx2, dp.names)

	ok, _ := okCancelButtons(w, true, "OK", true)

	if ok {
		if dp.Idx1 < 0 || dp.Idx2 < 0 {
			return
		}
		niceNameA, commitOrRefA := dp.idxToCommitOrRef(dp.Idx1)
		niceNameB, commitOrRefB := dp.idxToCommitOrRef(dp.Idx2)

		diffAction(&lw, niceNameA, commitOrRefA, niceNameB, commitOrRefB)
	}
}

func newCheckoutPopup(mw *nucular.MasterWindow, localRefs []Ref) {
	localRefsNames := make([]string, len(localRefs))
	for i := range localRefs {
		localRefsNames[i] = localRefs[i].Nice()
	}
	selectFromListWindow(mw, "Checkout...", "Pick a branch to checkout:", localRefsNames, func(idx int) {
		if idx >= 0 {
			checkoutAction(&lw, &localRefs[idx], "")
		}
	})
}

type ForcePushPopup struct {
	Repository string
	ed         nucular.TextEditor
}

func newForcePushPopup(mw *nucular.MasterWindow, repository string, buffer []rune) {
	var fp ForcePushPopup
	fp.Repository = repository
	fp.ed.Buffer = buffer
	fp.ed.Flags = nucular.EditMultiline | nucular.EditReadOnly
	mw.PopupOpen("Push error", popupFlags, rect.Rect{20, 100, 480, 500}, true, fp.Update)
}

func (fp *ForcePushPopup) Update(w *nucular.Window) {
	w.Row(200).Dynamic(1)
	fp.ed.Edit(w)
	ok, _ := okCancelButtons(w, false, fmt.Sprintf("Force Push %s", fp.Repository), true)
	if ok {
		pushAction(&lw, true, fp.Repository)
	}
}
