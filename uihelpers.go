package main

import (
	"fmt"

	"github.com/aarzilli/nucular"
	ntypes "github.com/aarzilli/nucular/types"

	"golang.org/x/mobile/event/key"
)

const popupFlags = nucular.WindowMovable | nucular.WindowTitle | nucular.WindowDynamic | nucular.WindowNoScrollbar | nucular.WindowScalable

type messagePopup struct {
	Title string
	ed    nucular.TextEditor
}

func newMessagePopup(mw *nucular.MasterWindow, title, message string) {
	var mp messagePopup
	mp.Title = title
	mp.ed.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditReadOnly
	mp.ed.Buffer = []rune(message)
	mw.PopupOpen(mp.Title, popupFlags, ntypes.Rect{20, 100, 640, 500}, true, mp.Update)
}

func (mp *messagePopup) Update(mw *nucular.MasterWindow, w *nucular.Window) {
	w.LayoutRowDynamic(200, 1)
	ok, cancel := okCancelKeys(w)
	w.LayoutRowDynamic(25, 1)
	mp.ed.Edit(w)
	if w.ButtonText("OK") || ok || cancel {
		w.Close()
	}
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

func selectFromList(w *nucular.Window, name string, idx int, list []string) int {
	if sw := w.GroupBegin(name, nucular.WindowNoHScrollbar|nucular.WindowBorder); sw != nil {
		sw.LayoutRowDynamic(25, 1)
		for i := range list {
			selected := idx == i
			sw.SelectableLabel(list[i], "LC", &selected)
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
		switch {
		case (e.Modifiers == 0) && (e.Code == key.CodeReturnEnter):
			return true, false
		case (e.Modifiers == 0) && (e.Code == key.CodeEscape):
			return false, true
		}
	}
	return false, false
}

func selectFromListWindow(mw *nucular.MasterWindow, title, text string, list []string, onSelect func(idx int)) {
	idx := -1
	mw.PopupOpen(title, popupFlags, ntypes.Rect{20, 100, 640, 400}, true, func(mw *nucular.MasterWindow, w *nucular.Window) {
		w.LayoutRowDynamic(25, 1)
		w.Label(text, "LC")
		w.LayoutRowDynamic(150, 1)

		idx = selectFromList(w, title+"-listgroup", idx, list)

		w.LayoutRowDynamic(25, 2)

		ok, cancel := okCancelKeys(w)

		if w.ButtonText("OK") || ok {
			w.Close()
			onSelect(idx)
		}
		if w.ButtonText("Cancel") || cancel {
			w.Close()
		}

		w.LayoutRowDynamic(10, 1)
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
	mw.PopupOpen("New branch...", popupFlags, ntypes.Rect{20, 100, 640, 400}, true, np.Update)

}

func (np *newBranchPopup) Update(mw *nucular.MasterWindow, w *nucular.Window) {
	w.LayoutRowDynamic(25, 1)
	active := np.ed.Edit(w)
	w.LayoutRowDynamic(25, 2)
	var ok, cancel bool
	if !np.ed.Active {
		ok, cancel = okCancelKeys(w)
	}
	if w.ButtonText("OK") || (active&nucular.EditCommitted != 0) || ok {
		newbranchAction(&lw, string(np.ed.Buffer), np.CommitId)
		w.Close()
	}
	if w.ButtonText("Cancel") || cancel {
		w.Close()
	}
	w.LayoutRowDynamic(10, 1)
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
	mw.PopupOpen("Reset...", popupFlags, ntypes.Rect{20, 100, 640, 400}, true, rp.Update)
}

func (rp *resetPopup) Update(mw *nucular.MasterWindow, w *nucular.Window) {
	w.LayoutRowDynamic(25, 1)
	if w.OptionText("Hard: reset working tree and index", rp.ResetMode == resetHard) {
		rp.ResetMode = resetHard
	}
	if w.OptionText("Mixed: leave working tree untouched, reset index", rp.ResetMode == resetMixed) {
		rp.ResetMode = resetMixed
	}
	if w.OptionText("Soft: leave working tree and index untouched", rp.ResetMode == resetSoft) {
		rp.ResetMode = resetSoft
	}
	w.LayoutRowDynamic(25, 2)
	ok, cancel := okCancelKeys(w)
	if w.ButtonText("OK") || ok {
		resetAction(&lw, rp.CommitId, rp.ResetMode)
		w.Close()
	}
	if w.ButtonText("Cancel") || cancel {
		w.Close()
	}
	w.LayoutRowDynamic(10, 1)
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
	mw.PopupOpen("Diff...", popupFlags, ntypes.Rect{20, 100, 460, 400}, true, dp.Update)
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

func (dp *diffPopup) Update(mw *nucular.MasterWindow, w *nucular.Window) {
	w.LayoutRowDynamic(150, 2)
	dp.Idx1 = selectFromList(w, "DiffA", dp.Idx1, dp.names)
	dp.Idx2 = selectFromList(w, "DiffB", dp.Idx2, dp.names)

	w.LayoutRowDynamic(25, 2)

	ok, cancel := okCancelKeys(w)

	if w.ButtonText("OK") || ok {
		w.Close()

		niceNameA, commitOrRefA := dp.idxToCommitOrRef(dp.Idx1)
		niceNameB, commitOrRefB := dp.idxToCommitOrRef(dp.Idx2)

		diffAction(&lw, niceNameA, commitOrRefA, niceNameB, commitOrRefB)
	}
	if w.ButtonText("Cancel") || cancel {
		w.Close()
	}
	w.LayoutRowDynamic(10, 1)
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
	mw.PopupOpen("Push error", popupFlags, ntypes.Rect{20, 100, 640, 500}, true, fp.Update)
}

func (fp *ForcePushPopup) Update(mw *nucular.MasterWindow, w *nucular.Window) {
	w.LayoutRowDynamic(200, 1)
	fp.ed.Edit(w)
	_, cancel := okCancelKeys(w)
	w.LayoutRowDynamic(25, 2)
	if w.ButtonText(fmt.Sprintf("Force Push %s", fp.Repository)) {
		pushAction(&lw, true, fp.Repository)
		w.Close()
	}
	if w.ButtonText("Cancel") || cancel {
		w.Close()
	}
}
