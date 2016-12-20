package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/label"
	nstyle "github.com/aarzilli/nucular/style"

	"golang.org/x/image/font"
)

type BlameTab struct {
	filename string
	revision string

	mu sync.Mutex
	mw nucular.MasterWindow

	CommitColor  map[string]color.RGBA
	Commits      map[string]*Commit
	Lines        []BlameLine
	CoveredLines int
	loadDone     bool

	title    string
	maxwidth int

	err error

	searcher Searcher
}

var colors = []color.RGBA{
	color.RGBA{0x4b, 0x58, 0x5d, 0xff},
	color.RGBA{0x67, 0x79, 0x80, 0xff},
	color.RGBA{0x2f, 0x24, 0x47, 0xff},
	color.RGBA{0x55, 0x41, 0x7e, 0xff},
	color.RGBA{0x91, 0xaa, 0xb4, 0xff},
	color.RGBA{0x5a, 0x6a, 0x70, 0xff},
	color.RGBA{0x6d, 0x4e, 0x83, 0xff},
	color.RGBA{0x96, 0x6b, 0xb3, 0xff},
	color.RGBA{0x46, 0x75, 0x53, 0xff},
	color.RGBA{0x68, 0x5d, 0x3e, 0xff},
}

type BlameLine struct {
	Commit *Commit
	Text   string
}

func NewBlameWindow(mw nucular.MasterWindow, revision, filename string) {
	tab := &BlameTab{filename: filename, revision: revision, mw: mw, Commits: map[string]*Commit{}, CommitColor: map[string]color.RGBA{}}

	tab.loadFile()

	tab.searcher.searchSel = SearchSel{BlamePos{tab, -1}, 0, 0}

	go tab.parseBlameOut()
	openTab(tab)
}

func (tab *BlameTab) loadFile() {
	if tab.revision == "" {
		fh, err := os.Open(filepath.Join(Repodir, tab.filename))
		if err != nil {
			tab.err = err
			return
		}
		defer fh.Close()
		tab.loadReader(fh)
		return
	}
	cmd := exec.Command("git", "show", tab.revision+":"+tab.filename)
	cmd.Dir = Repodir
	stdout, _ := cmd.StdoutPipe()
	defer stdout.Close()
	if err := cmd.Start(); err != nil {
		tab.err = err
		return
	}
	tab.loadReader(stdout)
	cmd.Wait()
}

func (tab *BlameTab) loadReader(in io.Reader) {
	s := bufio.NewScanner(in)
	for s.Scan() {
		tab.Lines = append(tab.Lines, BlameLine{Text: expandtabs(s.Text())})
	}
}

func (tab *BlameTab) parseBlameOut() {
	defer func() { tab.loadDone = true }()
	args := []string{"blame", "--incremental", "-w"}
	if tab.revision != "" {
		args = append(args, tab.revision)
	}
	args = append(args, "--", tab.filename)
	cmd := exec.Command("git", args...)

	stdout, _ := cmd.StdoutPipe()
	defer stdout.Close()
	s := bufio.NewScanner(stdout)

	cmd.Dir = Repodir
	if err := cmd.Start(); err != nil {
		tab.mu.Lock()
		tab.err = err
		tab.mw.Changed()
		tab.mu.Unlock()
	}
	colorseq := 0

	for s.Scan() {
		line := strings.TrimSpace(s.Text())

		// header
		fields := strings.Split(line, " ")
		sha1 := fields[0]
		dstline, _ := strconv.Atoi(fields[2])
		numlines, _ := strconv.Atoi(fields[3])

		commit, ok := tab.Commits[sha1]
		if !ok {
			commit = &Commit{Id: sha1}
			tab.Commits[sha1] = commit
			tab.CommitColor[sha1] = colors[colorseq]
			colorseq = (colorseq + 1) % len(colors)
		}

		const (
			authorField        = "author "
			authorTimeField    = "author-time "
			committerField     = "committer "
			committerTimeField = "committer-time "
			summaryField       = "summary "
			filenameField      = "filename "
		)

	commitBodyLoop:
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			switch {
			case strings.HasPrefix(line, authorField):
				commit.Author = line[len(authorField):]
			case strings.HasPrefix(line, authorTimeField):
				t, _ := strconv.Atoi(line[len(authorTimeField):])
				commit.AuthorDate = time.Unix(int64(t), 0)
			case strings.HasPrefix(line, committerField):
				commit.Committer = line[len(committerField):]
			case strings.HasPrefix(line, committerTimeField):
				t, _ := strconv.Atoi(line[len(committerTimeField):])
				commit.CommitterDate = time.Unix(int64(t), 0)
			case strings.HasPrefix(line, summaryField):
				commit.Message = line[len(summaryField):]
			case strings.HasPrefix(line, filenameField):
				break commitBodyLoop
			}
		}

		tab.mu.Lock()
		for i := 0; i < numlines; i++ {
			tab.Lines[dstline+i-1].Commit = commit
		}
		tab.CoveredLines += numlines
		tab.mu.Unlock()
		tab.mw.Changed()
	}
}

func (tab *BlameTab) Protected() bool {
	return false
}

func (tab *BlameTab) Title() string {
	if tab.title == "" {
		if tab.revision != "" {
			if len(tab.revision) >= 40 {
				tab.title = fmt.Sprintf("Blame %s %s", abbrev(tab.revision), tab.filename)
			} else {
				tab.title = fmt.Sprintf("Blame %s %s", tab.revision, tab.filename)
			}
		} else {
			tab.title = fmt.Sprintf("Blame %s", tab.filename)
		}
	}
	return tab.title
}

var blameCommitMenuSize = image.Point{150, 300}

func (tab *BlameTab) Update(w *nucular.Window) {
	tab.mu.Lock()
	defer tab.mu.Unlock()

	scrollToSearch := false

	tab.searcher.Events(w, &scrollToSearch)

	switch {
	case !tab.loadDone:
		w.Row(20).Static(80, 80)
		w.Label("Loading:", "LC")
		w.Progress(&tab.CoveredLines, len(tab.Lines), false)
	case tab.searcher.searching:
		tab.searcher.Update(w, &scrollToSearch)
	}

	style := w.Master().Style()

	lnh := nucular.FontHeight(style.Font)
	abbrevsz := nucular.FontWidth(style.Font, "0")*6 + style.Text.Padding.X*2
	authorsz := nucular.FontWidth(style.Font, "MMMM") + style.Text.Padding.X*2
	thick := int(style.Scaling)
	tooltipsz := nucular.FontWidth(style.Font, "0")*80 + style.TooltipWindow.Padding.X*2

	w.Row(0).Dynamic(1)
	if tab.err != nil {
		w.Label(fmt.Sprintf("Error: %v", tab.err), "LT")
		return
	}

	// 	colorTest(w)
	// 	_, _, _, _ = lnh, abbrevsz, authorsz, thick
	//	return

	var curCommit *Commit = nil

	maxwidth := 0
	if w := w.GroupBegin("blame", 0); w != nil {
		originalSpacing := style.GroupWindow.Spacing.Y
		style.GroupWindow.Spacing.Y = 0

		for lineidx, line := range tab.Lines {
			w.RowScaled(lnh).StaticScaled(abbrevsz, authorsz, thick, tab.maxwidth)

			if line.Commit != curCommit {
				if line.Commit != nil {
					w.Label(abbrev(line.Commit.Id), "CC")
					if w := w.ContextualOpen(0, blameCommitMenuSize, w.LastWidgetBounds, nil); w != nil {
						tab.blameCommitMenu(w, line.Commit)
					}
					w.Label(nameInitials(line.Commit.Author), "CC")
				} else {
					w.Label("", "LC")
					w.Label("", "LC")
				}
				curCommit = line.Commit
			} else {
				w.Label("", "LC")
				w.Label("", "LC")
			}

			if bounds, out := w.Custom(nstyle.WidgetStateInactive); out != nil {
				top := image.Pt(bounds.X+bounds.W/2, bounds.Y)
				bot := image.Pt(bounds.X+bounds.W/2, bounds.Y+bounds.H)
				out.StrokeLine(top, bot, thick, style.Text.Color)
			}

			issearchline := tab.searcher.searching && lineidx == tab.searcher.searchSel.Pos.(BlamePos).index

			if bounds, out := w.Custom(nstyle.WidgetStateInactive); out != nil {
				if line.Commit != nil {
					if c, ok := tab.CommitColor[line.Commit.Id]; ok {
						out.FillRect(bounds, 0, c)
					}
				}
				out.DrawText(bounds, line.Text, style.Font, style.Text.Color)

				if issearchline {
					d := font.Drawer{Face: style.Font}
					startX := bounds.X + d.MeasureString(line.Text[:tab.searcher.searchSel.Start]).Ceil()
					endX := bounds.X + d.MeasureString(line.Text[:tab.searcher.searchSel.End]).Ceil()
					maxy := bounds.Y + bounds.H
					out.StrokeLine(image.Pt(startX, maxy), image.Pt(endX, maxy), int(math.Ceil(style.Scaling)), style.Text.Color)
				}

				if w.Input().Mouse.HoveringRect(bounds) && line.Commit != nil {
					w.TooltipOpen(tooltipsz, false, BlameCommitFn(line.Commit))
				}
			}
			if issearchline && scrollToSearch {
				if above, below := w.Invisible(); above || below {
					w.Scrollbar.Y = w.At().Y
					w.Master().Changed()
					scrollToSearch = false
				}

			}
			if line.Commit != nil {
				if w := w.ContextualOpen(0, blameCommitMenuSize, w.LastWidgetBounds, nil); w != nil {
					tab.blameCommitMenu(w, line.Commit)
				}
			}

			if tab.maxwidth == 0 {
				if width := nucular.FontWidth(style.Font, line.Text); width > maxwidth {
					maxwidth = width
				}
			}
		}

		style.GroupWindow.Spacing.Y = originalSpacing
		scrollingKeys(w, lnh)
		w.GroupEnd()
	}

	if tab.maxwidth == 0 {
		tab.maxwidth = maxwidth
	}
}

func colorTest(w *nucular.Window) {
	bounds, out := w.Custom(nstyle.WidgetStateInactive)
	if out == nil {
		return
	}

	b := bounds
	b.W = 20
	b.H = 20

	for _, c := range colors {
		out.FillRect(b, 0, c)
		b.X += b.W
	}
}

func BlameCommitFn(commit *Commit) func(*nucular.Window) {
	commitline := fmt.Sprintf("commit %s", commit.Id)
	authorline := fmt.Sprintf("author %s %s", commit.Author, commit.AuthorDate.Local().Format("2006-01-02 15:04"))
	committerline := fmt.Sprintf("committer %s %s", commit.Committer, commit.CommitterDate.Local().Format("2006-01-02 15:04"))
	return func(w *nucular.Window) {
		style := w.Master().Style()
		lnh := nucular.FontHeight(style.Font)
		w.RowScaled(lnh).Dynamic(1)
		w.Label(commitline, "LC")
		w.Label(authorline, "LC")
		w.Label(committerline, "LC")
		w.Label(commit.Message, "LC")
	}
}

func (tab *BlameTab) blameCommitMenu(w *nucular.Window, commit *Commit) {
	w.Row(20).Dynamic(1)
	a := abbrev(commit.Id)
	if w.MenuItem(label.TA(fmt.Sprintf("View %s", a), "LC")) {
		// commit must be reloaded because it only contains the summary in Message
		fullCommit, _ := LoadCommit(commit.Id)
		NewViewWindow(fullCommit, true)
	}
	if w.MenuItem(label.TA(fmt.Sprintf("Blame at %s", a), "LC")) {
		NewBlameWindow(w.Master(), commit.Id, tab.filename)
	}
}

type BlamePos struct {
	tab   *BlameTab
	index int
}

func (pos BlamePos) BeforeFirst() SearchPos {
	return BlamePos{pos.tab, -1}
}

func (pos BlamePos) Next() (SearchPos, bool) {
	pos.index++
	if pos.index < 0 || pos.index >= len(pos.tab.Lines) {
		return pos, false
	}
	return pos, true
}

func (pos BlamePos) Text() string {
	if pos.index < 0 || pos.index >= len(pos.tab.Lines) {
		return ""
	}
	return pos.tab.Lines[pos.index].Text
}
