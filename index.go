package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aarzilli/nucular"
	ntypes "github.com/aarzilli/nucular/types"

	"golang.org/x/mobile/event/key"
)

type IndexManagerWindow struct {
	repodir  string
	selected int
	status   *GitStatus
	diff     Diff

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

func (idxmw *IndexManagerWindow) Update(mw *nucular.MasterWindow, w *nucular.Window) {
	idxmw.mu.Lock()
	defer idxmw.mu.Unlock()

	var diffbounds ntypes.Rect

	if idxmw.updating {
		w.LayoutRowDynamic(25, 1)
		w.Label("Updating...", "LC")
		return
	}

	style, scaling := mw.Style()

	w.LayoutRowRatio(0, 0.3, 0.7)

	if sw := w.GroupBegin("index-files", nucular.WindowBorder); sw != nil {
		cbw := min(int(25*scaling), style.Font.Size+style.Option.Padding.Y) + style.Option.Padding.X*2
		sw.LayoutRowStaticScaled(int(25*scaling), cbw, 0)

		for i, line := range idxmw.status.Lines {
			checked := line.Index != " " && line.WorkDir == " "

			if sw.CheckboxText("", &checked) {
				idxmw.addRemoveIndex(checked, i)
			}

			selected := idxmw.selected == i
			sw.SelectableLabel(idxmw.status.Lines[i].Path, "LC", &selected)
			if selected && idxmw.selected != i {
				idxmw.selected = i
				idxmw.loadDiff()
			}
		}
		sw.GroupEnd()
	}

	if sw := w.GroupBegin("index-right-column", nucular.WindowNoScrollbar); sw != nil {
		sw.LayoutRowStatic(25, 100, 100)
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

		sw.LayoutRowDynamic(100, 1)

		idxmw.ed.Edit(sw)

		sw.LayoutRowDynamic(5, 1)
		sw.LayoutRowStatic(25, 100, 50, 50, 100)
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
				newMessagePopup(mw, "Error", fmt.Sprintf("error: %v\n%s\n", err, bs))
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
		sw.LayoutRowDynamic(5, 1)

		sw.LayoutRowDynamic(0, 1)
		diffbounds = sw.WidgetBounds()
		if diffgroup := sw.GroupBegin("index-diff", nucular.WindowBorder); diffgroup != nil {
			if idxmw.selected >= 0 {
				showDiff(mw, diffgroup, idxmw.diff, &idxmw.diffwidth)
			}
			diffgroup.GroupEnd()
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
					idxmw.selected = 0
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
}
