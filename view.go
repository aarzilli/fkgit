package main

import (
	"fmt"
	"image/color"
	"os/exec"

	"github.com/aarzilli/nucular"

	"golang.org/x/mobile/event/key"
)

type ViewWindow struct {
	repodir string
	lc      LanedCommit

	tooLong bool
	diff    Diff
	width   int
}

func NewViewWindow(repodir string, lc LanedCommit) {
	vw := &ViewWindow{}

	vw.repodir = repodir
	vw.lc = lc

	vw.parseDiff()

	openTab(vw)
}

func (vw *ViewWindow) parseDiff() {
	cmd := exec.Command("git", "show", vw.lc.Id, "--decorate=full", "--no-abbrev", "--color=never")
	cmd.Dir = vw.repodir

	bs, err := cmd.CombinedOutput()
	must(err)

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

func (vw *ViewWindow) Update(mw *nucular.MasterWindow) {
	mw.Wnd.LayoutRowDynamicScaled(mw.Wnd.LayoutAvailableHeight(), 1)
	if sw := mw.Wnd.GroupBegin("view-"+vw.lc.Id, 0); sw != nil {
		vw.updateView(mw, sw)
		sw.GroupEnd()
	}
}

func (vw *ViewWindow) updateView(mw *nucular.MasterWindow, w *nucular.Window) {
	style, _ := mw.Style()

	showCommit(style.Font.Size, w, vw.lc)
	w.Label(" ", nucular.TextLeft)

	scrollend := false

	for _, e := range w.KeyboardOnHover(w.Bounds).Keys {
		switch {
		case (e.Modifiers == 0) && (e.Code == key.CodeHome):
			w.Scrollbar.X = 0
			w.Scrollbar.Y = 0
		case (e.Modifiers == 0) && (e.Code == key.CodeEnd):
			w.Scrollbar.X = 0
			scrollend = true
		case (e.Modifiers == 0) && (e.Code == key.CodeUpArrow):
			w.Scrollbar.Y -= style.Font.Size
		case (e.Modifiers == 0) && (e.Code == key.CodeDownArrow):
			w.Scrollbar.Y += style.Font.Size
		case (e.Modifiers == 0) && (e.Code == key.CodeLeftArrow):
			w.Scrollbar.X -= w.Bounds.W / 10
		case (e.Modifiers == 0) && (e.Code == key.CodeRightArrow):
			w.Scrollbar.X += w.Bounds.W / 10
		case (e.Modifiers == 0) && (e.Code == key.CodePageUp):
			w.Scrollbar.Y -= w.Bounds.H / 2
		case (e.Modifiers == 0) && (e.Code == key.CodePageDown):
			w.Scrollbar.Y += w.Bounds.H / 2
		case (e.Modifiers == 0) && ((e.Code == key.CodeEscape) || (e.Code == key.CodeQ)):
			closeTab(vw)
		}

		if w.Scrollbar.Y < 0 {
			w.Scrollbar.Y = 0
		}
		if w.Scrollbar.X < 0 {
			w.Scrollbar.X = 0
		}
	}

	w.LayoutRowDynamic(25, 1)
	w.Label("Changed Files:", nucular.TextLeft)

	clickedfile := -1

	for i := range vw.diff {
		if w.ButtonText(vw.diff[i].Filename, 0) {
			clickedfile = i
		}
	}

	w.LayoutRowDynamic(15, 1)
	w.Spacing(1)

	scrollto := showDiff(mw, w, vw.diff, clickedfile, &vw.width)

	if scrollend {
		scrollto = w.At().Y
	}

	if scrollend || clickedfile >= 0 {
		w.Scrollbar.Y = scrollto
	}
}

func showCommit(lnh int, w *nucular.Window, lc LanedCommit) {
	w.LayoutRowDynamicScaled(lnh, 1)
	w.Label(fmt.Sprintf("commit %s\n", lc.Id), nucular.TextLeft)
	for i := range lc.Parent {
		w.Label(fmt.Sprintf("parent %s\n", lc.Parent[i]), nucular.TextLeft)
	}
	w.Label(fmt.Sprintf("author %s on %s\n", lc.Author, lc.AuthorDate.Local().Format("2006-01-02 15:04")), nucular.TextLeft)
	w.Label(fmt.Sprintf("committer %s on %s\n", lc.Committer, lc.CommitterDate.Local().Format("2006-01-02 15:04")), nucular.TextLeft)
	w.Spacing(1)
	showLines(w, lc.Message)
}
