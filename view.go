package main

import (
	"fmt"
	"image/color"
	"os/exec"

	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/clipboard"

	"golang.org/x/mobile/event/key"
)

type ViewWindow struct {
	repodir         string
	lc              LanedCommit
	viewInTabButton bool

	isdiff               bool
	niceNameA, niceNameB string
	commitA, commitB     string

	tooLong bool
	diff    Diff
	width   int

	searching    bool
	searched     nucular.TextEditor
	searchNeedle string
	searchSel    DiffSel
}

func NewViewWindow(repodir string, lc LanedCommit, opentab bool) *ViewWindow {
	vw := &ViewWindow{}

	vw.repodir = repodir
	vw.lc = lc

	vw.parseDiff()

	if opentab {
		openTab(vw)
	} else {
		vw.viewInTabButton = true
	}

	return vw
}

func NewDiffWindow(repodir string, niceNameA, commitA, niceNameB, commitB string) {
	vw := &ViewWindow{}

	vw.isdiff = true
	vw.repodir = repodir
	vw.niceNameA = niceNameA
	vw.niceNameB = niceNameB
	vw.commitA = commitA
	vw.commitB = commitB

	vw.parseDiff()

	openTab(vw)
}

func (vw *ViewWindow) parseDiff() {
	var cmd *exec.Cmd
	if vw.isdiff {
		cmd = exec.Command("git", "diff", "--color=never", vw.commitA, vw.commitB)
	} else {
		cmd = exec.Command("git", "show", vw.lc.Id, "--decorate=full", "--no-abbrev", "--color=never")
	}
	cmd.Dir = vw.repodir

	bs, err := cmd.CombinedOutput()
	if err != nil {
		closeTab(vw)
		return
	}

	if len(bs) > 1*1024*1024 {
		vw.tooLong = true
		return
	}

	vw.diff = parseDiff(bs)
}

var (
	addlineBg    = color.RGBA{0x00, 0x41, 0x00, 0xff}
	dellineBg    = color.RGBA{0x4f, 0x00, 0x00, 0xff}
	addsegBg     = color.RGBA{0x2f, 0x76, 0x2f, 0xff}
	delsegBg     = color.RGBA{0xb8, 0x29, 0x29, 0xff}
	hunkhdrColor = color.RGBA{0x06, 0x98, 0x9a, 0xff}
)

func (vw *ViewWindow) Title() string {
	return vw.lc.NiceWithAbbrev()
}

func (vw *ViewWindow) Protected() bool {
	return false
}

func (vw *ViewWindow) Update(w *nucular.Window) {
	w.Row(0).Dynamic(1)
	if sw := w.GroupBegin("view-"+vw.lc.Id, 0); sw != nil {
		vw.updateView(sw)
		sw.GroupEnd()
	}
}

func (vw *ViewWindow) updateView(w *nucular.Window) {
	style := w.Master().Style()

	scrollToSearch := false

	if vw.isdiff {
		w.Row(20).Dynamic(1)
		w.Label("Diff", "LC")
		w.Label("    "+vw.niceNameA, "LC")
		w.Label("    "+vw.niceNameB, "LC")
	} else {
		if !vw.viewInTabButton {
			for _, e := range w.Input().Keyboard.Keys {
				switch {
				case (e.Modifiers == key.ModControl) && (e.Code == key.CodeF):
					vw.searching = true
					vw.searched.Buffer = vw.searched.Buffer[:0]
					vw.searchNeedle = "-DIFFERENT-"
					vw.searched.Flags = nucular.EditSelectable | nucular.EditClipboard | nucular.EditSigEnter
					vw.searchSel.Pos = vw.diff.BeforeFirst()
					vw.searchSel.Start, vw.searchSel.End = 0, 0
					w.Master().ActivateEditor(&vw.searched)
					scrollToSearch = true
				case (e.Modifiers == key.ModControl) && (e.Code == key.CodeG):
					if !vw.searching {
						w.Master().ActivateEditor(&vw.searched)
					}
					vw.searching = true
					vw.searchSel = vw.diff.Lookfwd(vw.searchSel, string(vw.searched.Buffer), true)
					scrollToSearch = true
				case (e.Modifiers == 0) && (e.Code == key.CodeEscape):
					vw.searching = false
				}
			}

			if vw.searching {
				w.MenubarBegin()
				w.Row(20).Static(100, 0)
				w.Label("Search", "LC")
				active := vw.searched.Edit(w)
				if active&nucular.EditCommitted != 0 {
					vw.searching = false
					w.Master().Changed()
				}
				if needle := string(vw.searched.Buffer); needle != vw.searchNeedle {
					vw.searchSel = vw.diff.Lookfwd(vw.searchSel, needle, false)
					vw.searchNeedle = needle
					scrollToSearch = true
				}
				w.MenubarEnd()
			}
		}
		showCommit(nucular.FontHeight(style.Font), w, vw.lc, vw.viewInTabButton)
		w.Label(" ", "LC")
	}

	sel := &vw.searchSel
	if !vw.searching {
		sel = nil
	}
	showDiff(w, vw.diff, &vw.width, sel, scrollToSearch)
}

func showCommit(lnh int, w *nucular.Window, lc LanedCommit, viewInTabButton bool) {
	style := w.Master().Style()
	commitWidth := nucular.FontWidth(style.Font, "0")*48 + style.Text.Padding.X*2
	btnWidth := int(80 * style.Scaling)
	if viewInTabButton {
		w.Row(20).StaticScaled(commitWidth, btnWidth, btnWidth)
	} else {
		w.Row(20).StaticScaled(commitWidth, btnWidth)
	}
	w.Label(fmt.Sprintf("commit %s\n", lc.Id), "LC")
	if w.ButtonText("Copy") {
		clipboard.Set(lc.Id)
	}
	if viewInTabButton {
		if w.ButtonText("View") {
			viewAction(&lw, lc)
		}
	}
	for i := range lc.Parent {
		w.Label(fmt.Sprintf("parent %s\n", lc.Parent[i]), "LC")
		if w.ButtonText("Copy") {
			clipboard.Set(lc.Parent[i])
		}
	}
	w.RowScaled(lnh).Dynamic(1)
	w.Label(fmt.Sprintf("author %s on %s\n", lc.Author, lc.AuthorDate.Local().Format("2006-01-02 15:04")), "LC")
	w.Label(fmt.Sprintf("committer %s on %s\n", lc.Committer, lc.CommitterDate.Local().Format("2006-01-02 15:04")), "LC")
	w.Spacing(1)
	showLines(w, lc.Message)
}
