package main

import (
	"fmt"
	"image/color"
	"os/exec"

	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/clipboard"
)

type ViewWindow struct {
	repodir         string
	commit          Commit
	viewInTabButton bool

	isdiff               bool
	niceNameA, niceNameB string
	commitA, commitB     string

	tooLong bool
	diff    Diff
	width   int

	searcher Searcher
}

func NewViewWindow(repodir string, commit Commit, opentab bool) *ViewWindow {
	vw := &ViewWindow{}

	vw.repodir = repodir
	vw.commit = commit

	vw.parseDiff()

	vw.searcher.searchSel = SearchSel{vw.diff.BeforeFirst(), 0, 0}

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
		cmd = exec.Command("git", "show", vw.commit.Id, "--decorate=full", "--no-abbrev", "--color=never")
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
	return vw.commit.NiceWithAbbrev()
}

func (vw *ViewWindow) Protected() bool {
	return false
}

func (vw *ViewWindow) Update(w *nucular.Window) {
	w.Row(0).Dynamic(1)
	if sw := w.GroupBegin("view-"+vw.commit.Id, 0); sw != nil {
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
			vw.searcher.Events(w, &scrollToSearch)

			if vw.searcher.searching {
				w.MenubarBegin()
				vw.searcher.Update(w, &scrollToSearch)
				w.MenubarEnd()
			}
		}
		showCommit(nucular.FontHeight(style.Font), w, vw.commit, vw.viewInTabButton)
		w.Label(" ", "LC")
	}

	showDiff(w, vw.diff, vw.repodir, &vw.width, vw.searcher.Sel(), scrollToSearch)
}

func showCommit(lnh int, w *nucular.Window, lc Commit, viewInTabButton bool) {
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
