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
)

type IndexManagerWindow struct {
	repodir  string
	selected int
	status   *GitStatus
	diffs    []Diff

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

func (idxmw *IndexManagerWindow) Update(mw *nucular.MasterWindow) {
	idxmw.mu.Lock()
	defer idxmw.mu.Unlock()

	w := mw.Wnd

	if idxmw.updating {
		w.LayoutRowDynamic(25, 1)
		w.Label("Updating...", nucular.TextLeft)
		return
	}

	style, scaling := mw.Style()

	w.LayoutRowRatioScaled(w.LayoutAvailableHeight(), 0.3, 0.7)

	if sw := w.GroupBegin("index-files", nucular.WindowBorder); sw != nil {
		w := sw.LayoutAvailableWidth()
		cbw := min(int(25*scaling), style.Font.Size+style.Option.Padding.Y) + style.Option.Padding.X*2
		sw.LayoutRowFixedScaled(int(25*scaling), cbw, w-cbw)

		for i, line := range idxmw.status.Lines {
			checked := line.Index != " " && line.WorkDir == " "

			if sw.CheckboxText("", &checked) {
				if checked {
					execCommand(idxmw.repodir, "git", "add", line.Path)
				} else {
					execCommand(idxmw.repodir, "git", "reset", "-q", "--", line.Path)
				}
				idxmw.updating = true
				go idxmw.reload()
			}

			selected := idxmw.selected == i
			sw.SelectableLabel(idxmw.status.Lines[i].Path, nucular.TextLeft, &selected)
			if selected && idxmw.selected != i {
				idxmw.selected = i
			}
		}
		sw.GroupEnd()
	}

	if sw := w.GroupBegin("index-right-column", nucular.WindowNoScrollbar); sw != nil {
		sw.LayoutRowStatic(25, 100, 2)
		oldamend := idxmw.amend
		if sw.OptionText("New commit", idxmw.amend == false) {
			idxmw.amend = false
		}
		if sw.OptionText("Amend", idxmw.amend) {
			idxmw.amend = true
		}
		if idxmw.amend != oldamend {
			idxmw.loadCommitMsg()
		}

		sw.LayoutRowDynamic(100, 1)

		idxmw.ed.Edit(sw, -1, nucular.FilterDefault)

		sw.LayoutRowFixed(25, 100, 50, 50, 100)
		sw.PropertyInt("fmt:", 10, &idxmw.fmtwidth, 150, 1, 1)
		sw.ButtonText("fmt", 0)
		sw.Spacing(1)
		if sw.ButtonText("Commit", 0) {
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
				popupWindows = append(popupWindows, &MessagePopup{"Error", fmt.Sprintf("error: %v\n%s\n", err, bs)})
			} else {
				idxmw.ed.Buffer = []rune{}
				idxmw.ed.Cursor = 0
			}
			idxmw.updating = true
			go idxmw.reload()
		}

		sw.LayoutRowDynamicScaled(sw.LayoutAvailableHeight(), 1)
		if diffgroup := sw.GroupBegin("index-diff", nucular.WindowBorder); diffgroup != nil {
			if idxmw.selected >= 0 {
				showDiff(mw, diffgroup, idxmw.diffs[idxmw.selected], -1, &idxmw.diffwidth)
			}
			diffgroup.GroupEnd()
		}
		sw.GroupEnd()
	}
}

func (idxmw *IndexManagerWindow) reload() {
	idxmw.mu.Lock()
	idxmw.updating = true
	idxmw.mu.Unlock()

	defer func() {
		idxmw.mu.Lock()
		idxmw.updating = false
		idxmw.mu.Unlock()
		idxmw.mw.Update()
	}()
	
	oldselected := ""
	if idxmw.status != nil {
		oldselected = idxmw.status.Lines[idxmw.selected].Path
	}
	idxmw.selected = -1

	idxmw.loadCommitMsg()

	idxmw.status = gitStatus(idxmw.repodir)

	idxmw.diffs = make([]Diff, len(idxmw.status.Lines))

	for i, line := range idxmw.status.Lines {
		var bs string
		var err error
		if line.Index != " " && line.WorkDir == " " {
			bs, err = execCommand(idxmw.repodir, "git", "diff", "--color=never", "--cached", "--", line.Path)
		} else {
			bs, err = execCommand(idxmw.repodir, "git", "diff", "--color=never", "--", line.Path)
		}
		must(err)
		idxmw.diffs[i] = parseDiff([]byte(bs))
		
		if line.Path == oldselected {
			idxmw.selected = i
		}
	}
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
