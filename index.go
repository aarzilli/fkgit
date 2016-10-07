package main

import (
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/label"
	"github.com/aarzilli/nucular/rect"

	"golang.org/x/mobile/event/key"
)

type IndexManagerWindow struct {
	repodir             string
	selected            int
	status              *GitStatus
	diff                Diff
	resetDiffViewScroll bool

	splitv nucular.ScalableSplit
	splith nucular.ScalableSplit

	mu       sync.Mutex
	mw       *nucular.MasterWindow
	updating bool

	diffwidth int
	fmtwidth  int

	amend bool
	ed    nucular.TextEditor
}

func (idxmw *IndexManagerWindow) Title() string {
	return "Commit"
}

func (idxmw *IndexManagerWindow) Update(w *nucular.Window) {
	idxmw.mu.Lock()
	defer idxmw.mu.Unlock()

	var diffbounds rect.Rect

	if idxmw.updating {
		w.Row(25).Dynamic(1)
		w.Label("Updating...", "LC")
		return
	}

	style, scaling := w.Master().Style()

	area := w.Row(0).SpaceBegin(0)
	leftbounds, rightbounds := idxmw.splitv.Vertical(w, area)
	viewbounds, commitbounds := idxmw.splith.Horizontal(w, rightbounds)

	w.LayoutSpacePushScaled(leftbounds)
	if sw := w.GroupBegin("index-files", nucular.WindowBorder); sw != nil {
		cbw := min(int(25*scaling), nucular.FontHeight(style.Font)+style.Option.Padding.Y) + style.Option.Padding.X*2
		sw.Row(25).StaticScaled(cbw, 0)

		for i, line := range idxmw.status.Lines {
			checked := line.Index != " " && line.WorkDir == " "

			if sw.CheckboxText("", &checked) {
				idxmw.addRemoveIndex(checked, i)
			}

			selected := idxmw.selected == i
			sw.SelectableLabel(idxmw.status.Lines[i].Path, "LC", &selected)

			if idxmw.status.Lines[i].Index == "?" && idxmw.status.Lines[i].WorkDir == "?" {
				if w := sw.ContextualOpen(0, image.Point{200, 500}, sw.LastWidgetBounds, nil); w != nil {
					w.Row(20).Dynamic(1)
					selected = true
					if w.MenuItem(label.TA("Ignore", "LC")) {
						idxmw.ignoreIndex(i)
					}
				}
			}

			if selected && idxmw.selected != i {
				idxmw.selected = i
				idxmw.loadDiff()
			}
		}
		sw.GroupEnd()
	}

	w.LayoutSpacePushScaled(viewbounds)
	if diffgroup := w.GroupBegin("index-diff", nucular.WindowBorder); diffgroup != nil {
		if idxmw.resetDiffViewScroll {
			idxmw.resetDiffViewScroll = false
			diffgroup.Scrollbar.X = 0
			diffgroup.Scrollbar.Y = 0
		}
		if idxmw.selected >= 0 {
			showDiff(diffgroup, idxmw.diff, &idxmw.diffwidth)
		}
		diffgroup.GroupEnd()
	}

	w.LayoutSpacePushScaled(commitbounds)
	if sw := w.GroupBegin("index-right-column", nucular.WindowNoScrollbar|nucular.WindowBorder); sw != nil {
		sw.Row(25).Static(100, 100)
		oldamend := idxmw.amend
		if sw.OptionText("New commit", idxmw.amend == false) {
			idxmw.amend = false
		}
		if sw.OptionText("Amend", idxmw.amend) {
			idxmw.amend = true
		}
		if idxmw.amend != oldamend {
			if !idxmw.amend {
				idxmw.ed.Buffer = []rune{}
				idxmw.ed.Cursor = 0
			}
			idxmw.loadCommitMsg()
		}

		sw.LayoutReserveRow(25, 1)

		sw.Row(0).Dynamic(1)
		idxmw.ed.Edit(sw)

		sw.Row(25).Static(100, 50, 0, 100)
		sw.PropertyInt("fmt:", 10, &idxmw.fmtwidth, 150, 1, 1)
		if sw.ButtonText("fmt") {
			idxmw.formatmsg()
		}
		sw.Spacing(1)
		if sw.ButtonText("Commit") {
			var cmd *exec.Cmd
			if idxmw.amend {
				cmd = exec.Command("git", "commit", "--amend", "-F", "-")
			} else {
				cmd = exec.Command("git", "commit", "-F", "-")
			}
			cmd.Dir = idxmw.repodir
			gitin, err := cmd.StdinPipe()
			must(err)
			go func() {
				io.WriteString(gitin, string(idxmw.ed.Buffer))
				gitin.Close()
			}()
			bs, err := cmd.CombinedOutput()
			if err != nil {
				newMessagePopup(w.Master(), "Error", fmt.Sprintf("error: %v\n%s\n", err, bs))
			} else {
				idxmw.amend = false
				idxmw.ed.Buffer = []rune{}
				idxmw.ed.Cursor = 0
			}
			idxmw.updating = true
			lw.mu.Lock()
			go lw.reload()
			lw.mu.Unlock()
			go idxmw.reload()
		}
		sw.GroupEnd()
	}

	in := w.Input()
	if !idxmw.ed.Active && !in.Mouse.HoveringRect(diffbounds) {
		for _, e := range in.Keyboard.Keys {
			switch {
			case (e.Modifiers == 0) && (e.Code == key.CodeUpArrow):
				idxmw.selected--
				if idxmw.selected < 0 {
					idxmw.selected = -1
				}
				idxmw.loadDiff()
			case (e.Modifiers == 0) && (e.Code == key.CodeDownArrow):
				idxmw.selected++
				if idxmw.selected >= len(idxmw.status.Lines) {
					if len(idxmw.status.Lines) == 0 {
						idxmw.selected = -1
					} else {
						idxmw.selected = 0
					}
				}
				idxmw.loadDiff()
			}
		}
		if in.Keyboard.Text == " " {
			if idxmw.selected >= 0 {
				idxmw.addRemoveIndex(true, idxmw.selected)
			}
			idxmw.selected++
			if idxmw.selected >= len(idxmw.status.Lines) {
				idxmw.selected = 0
			}
			idxmw.loadDiff()
		}
	}
}

func (idxmw *IndexManagerWindow) addRemoveIndex(add bool, i int) {
	if add {
		execCommand(idxmw.repodir, "git", "add", idxmw.status.Lines[i].Path)
	} else {
		execCommand(idxmw.repodir, "git", "reset", "-q", "--", idxmw.status.Lines[i].Path)
	}
	idxmw.updating = true
	go idxmw.reload()
}

func (idxmw *IndexManagerWindow) ignoreIndex(i int) {
	fh, err := os.OpenFile(filepath.Join(idxmw.repodir, ".git/info/exclude"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer fh.Close()
	fmt.Fprintf(fh, "\n%s\n", idxmw.status.Lines[i].Path)
	idxmw.updating = true
	go idxmw.reload()
}

func (idxmw *IndexManagerWindow) reload() {
	idxmw.mu.Lock()
	idxmw.updating = true
	idxmw.mu.Unlock()

	defer func() {
		idxmw.mu.Lock()
		idxmw.updating = false
		idxmw.mu.Unlock()
		idxmw.mw.Changed()
	}()

	oldselected := ""
	if idxmw.status != nil && idxmw.selected >= 0 {
		oldselected = idxmw.status.Lines[idxmw.selected].Path
	}
	idxmw.selected = -1

	idxmw.loadCommitMsg()

	idxmw.status = gitStatus(idxmw.repodir)

	for i, line := range idxmw.status.Lines {
		if line.Path == oldselected {
			idxmw.selected = i
			break

		}
	}

	idxmw.loadDiff()
}

func (idxmw *IndexManagerWindow) loadDiff() {
	if idxmw.selected < 0 {
		return
	}

	line := idxmw.status.Lines[idxmw.selected]

	var bs string
	var err error

	if line.Index != " " && line.WorkDir == " " {
		bs, err = execCommand(idxmw.repodir, "git", "diff", "--color=never", "--cached", "--", line.Path)
	} else {
		bs, err = execCommand(idxmw.repodir, "git", "diff", "--color=never", "--", line.Path)
	}
	must(err)
	idxmw.resetDiffViewScroll = true
	idxmw.diff = parseDiff([]byte(bs))
}

func (idxmw *IndexManagerWindow) loadCommitMsg() {
	if idxmw.amend {
		out, err := execCommand(idxmw.repodir, "git", "cat-file", "commit", "HEAD")
		must(err)
		msgv := strings.Split(out, "\n")
		for i := range msgv {
			if msgv[i] == "" {
				idxmw.ed.Cursor = 0
				idxmw.ed.Buffer = []rune(strings.Join(msgv[i+1:], "\n"))
				return
			}
		}
	} else {
		for _, name := range []string{"MERGE_MSG", "SQUASH_MSG"} {
			bs, err := ioutil.ReadFile(filepath.Join(filepath.Join(idxmw.repodir, ".git"), name))
			if err == nil {
				idxmw.ed.Cursor = 0
				idxmw.ed.Buffer = []rune(string(bs))
				return
			}
		}
	}
}

func (idxmw *IndexManagerWindow) formatmsg() {
	fmtstart := idxmw.ed.SelectStart
	fmtend := idxmw.ed.SelectEnd

	if fmtstart > fmtend {
		fmtstart, fmtend = fmtend, fmtstart
	}

	if fmtstart == fmtend {
		fmtstart = 0
		fmtend = len(idxmw.ed.Buffer)
	}

	msg := idxmw.ed.Buffer[fmtstart:fmtend]
	out := make([]rune, 0, len(msg)+10)

	start := 0
	lastspace := 0

	for i := range msg {
		switch msg[i] {
		case '\n':
			out = append(out, msg[start:i+1]...)
			start = i + 1
			lastspace = start

		case ' ':
			lastspace = i

		default:
			if lastspace != start && (i-start) > idxmw.fmtwidth {
				out = append(out, msg[start:lastspace+1]...)
				out = append(out, '\n')
				start = lastspace + 1
				lastspace = start
			}
		}
	}

	out = append(out, msg[start:]...)

	end := make([]rune, len(idxmw.ed.Buffer[fmtend:]))
	copy(end, idxmw.ed.Buffer[fmtend:])
	idxmw.ed.Buffer = idxmw.ed.Buffer[:fmtstart]
	idxmw.ed.Buffer = append(idxmw.ed.Buffer, out...)
	idxmw.ed.Buffer = append(idxmw.ed.Buffer, end...)
	idxmw.ed.CursorFollow = true
	go idxmw.mw.Changed()
}
