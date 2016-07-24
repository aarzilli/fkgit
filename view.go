package main

import (
	"fmt"
	"image/color"
	"os/exec"
	"strings"

	"github.com/aarzilli/nucular"

	"golang.org/x/image/font"
)

func NewViewWindow(repodir string, lc LanedCommit) {
	vw := &ViewWindow{}

	vw.repodir = repodir
	vw.lc = lc

	vw.parseDiff()

	// convert spaces to tabs
	for _, filediff := range vw.diff {
		for _, linediff := range filediff.Lines {
			for i := range linediff.Chunks {
				text := linediff.Chunks[i].Text
				if strings.Index(text, "\t") >= 0 {
					out := make([]byte, 0, len(text))
					for j := range text {
						if text[j] != '\t' {
							out = append(out, text[j])
						} else {
							out = append(out, []byte("        ")...)
						}
					}
					linediff.Chunks[i].Text = string(out)
				}
			}
		}
	}

	wnd := nucular.NewMasterWindow(vw.viewUpdate, 0)
	wnd.SetStyle(nucular.StyleFromTheme(nucular.DarkTheme), nil, conf.Scaling)
	go wnd.Main()
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

func (vw *ViewWindow) viewUpdate(mw *nucular.MasterWindow) {
	w := mw.Wnd
	style, scaling := mw.Style()

	hdrrounding := uint16(6 * scaling)
	rounding := uint16(4 * scaling)
	showCommit(style.Font.Size, w, vw.lc)
	w.Label(" ", nucular.TextLeft)

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

	d := font.Drawer{Face: style.Font.Face}

	for filediffIdx, filediff := range vw.diff {
		if filediffIdx == clickedfile {
			//TODO: scroll here
		}
		w.LayoutRowDynamic(25, 1)
		bounds, out := w.Custom(nucular.WidgetStateInactive)
		if out != nil {
			out.FillRect(bounds, hdrrounding, hunkhdrColor)
			pos := bounds
			pos.Y += pos.H/2 - style.Font.Size/2
			width := d.MeasureString(filediff.Filename).Ceil()
			pos.X += pos.W/2 - width/2
			out.DrawText(pos, filediff.Filename, style.Font, color.RGBA{0x00, 0x00, 0x00, 0xff}, style.NormalWindow.Background)
		}

		originalSpacing := style.NormalWindow.Spacing.Y
		style.NormalWindow.Spacing.Y = 0

		w.LayoutRowDynamicScaled(style.Font.Size, 1)

		for _, hdr := range filediff.Headers[1:] {
			w.LabelColored(hdr.Text, nucular.TextLeft, hunkhdrColor)
		}

		w.Spacing(1)

		for _, linediff := range filediff.Lines {
			bounds, out := w.Custom(nucular.WidgetStateInactive)
			if out == nil {
				continue
			}

			switch linediff.Opts {
			case Addline:
				out.FillRect(bounds, 0, addlineBg)
			case Delline:
				out.FillRect(bounds, 0, dellineBg)
			}

			dot := bounds
			for _, chunk := range linediff.Chunks {
				dot.W = d.MeasureString(chunk.Text).Ceil()
				switch chunk.Opts {
				case Addseg:
					out.FillRect(dot, rounding, addsegBg)
				case Delseg:
					out.FillRect(dot, rounding, delsegBg)
				}

				out.DrawText(dot, chunk.Text, style.Font, color.RGBA{0x00, 0x00, 0x00, 0xff}, style.Text.Color)
				dot.X += dot.W
			}
		}

		style.NormalWindow.Spacing.Y = originalSpacing

		w.Spacing(1)
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
