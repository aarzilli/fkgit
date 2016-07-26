package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aarzilli/fkgit/clipboard"
	"github.com/aarzilli/nucular"

	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func execCommand(repodir, cmdname string, args ...string) (string, error) {
	cmd := exec.Command(cmdname, args...)
	cmd.Dir = repodir
	bs, err := cmd.CombinedOutput()
	return string(bs), err
}

type RefKind int

const (
	LocalRef RefKind = iota
	RemoteRef
	TagRef
)

type Ref struct {
	IsHEAD   bool
	Name     string
	CommitId string
	Kind     RefKind
	nice     string
	remote   string
}

func (ref *Ref) Init(name, commitid string) {
	const headsPrefix = "refs/heads/"
	const remotesPrefix = "refs/remotes/"
	const tagsPrefix = "refs/tags/"

	ref.Name = name
	ref.CommitId = commitid

	for i, prefix := range []string{headsPrefix, remotesPrefix, tagsPrefix} {
		if strings.HasPrefix(ref.Name, prefix) {
			ref.nice = ref.Name[len(prefix):]
			switch i {
			case 0:
				ref.Kind = LocalRef
			case 1:
				ref.Kind = RemoteRef
				ref.remote = ref.nice
				if idx := strings.Index(ref.remote, "/"); idx >= 0 {
					ref.remote = ref.remote[:idx]
				}
			case 2:
				ref.Kind = TagRef
			}
			break
		}
	}

}

func (ref *Ref) Nice() string {
	if ref.IsHEAD {
		return "HEAD -> " + ref.nice
	}
	return ref.nice
}

func (ref *Ref) Remote() string {
	return ref.remote
}

func getHead(repodir string) (isref bool, reforid string, err error) {
	const refPrefix = "ref: "
	bs, err := ioutil.ReadFile(filepath.Join(filepath.Join(repodir, ".git"), "HEAD"))
	if err != nil {
		return false, "", err
	}
	s := strings.TrimSpace(string(bs))

	if strings.HasPrefix(s, refPrefix) {
		return true, s[len(refPrefix):], nil
	}

	return false, s, nil
}

func allRefs(repodir string) ([]Ref, error) {
	headisref, headref, err := getHead(repodir)
	if err != nil {
		return nil, err
	}
	if !headisref {
		headref = ""
	}

	s, err := execCommand(repodir, "git", "show-ref", "-d")
	if err != nil {
		return nil, err
	}
	v := strings.Split(s, "\n")

	const realTagSuffix = "^{}"

	r := make([]Ref, 0, len(v))
	for i := range v {
		if v[i] == "" {
			continue
		}
		commitidend := strings.Index(v[i], " ")
		var ref Ref
		ref.Init(v[i][commitidend+1:], v[i][:commitidend])
		if ref.Kind == TagRef {
			if strings.HasSuffix(ref.Name, realTagSuffix) {
				ref.Name = ref.Name[:len(ref.Name)-len(realTagSuffix)]
				ref.Init(ref.Name, ref.CommitId)
			} else {
				// skip this tag the ^{} version will be following it
				continue
			}
		}
		if ref.Name == headref {
			ref.IsHEAD = true
		}
		r = append(r, ref)
	}

	return r, nil
}

func findRepository() string {
	var path string
	if len(os.Args) > 1 {
		path = os.Args[1]
	} else {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return ""
		}
	}
	for {
		gitdir := filepath.Join(path, ".git")
		fi, err := os.Stat(gitdir)
		if err == nil && fi.IsDir() {
			return path
		}
		oldpath := path
		path = filepath.Dir(path)
		if oldpath == path {
			return ""
		}
	}
}

type Commit struct {
	Id            string
	Parent        []string
	Author        string
	AuthorDate    time.Time
	Committer     string
	CommitterDate time.Time
	Message       string
}

func (commit *Commit) LongPrint() {
	fmt.Printf("Commit %s\n", commit.Id)
	for i := range commit.Parent {
		fmt.Printf("Parent %s\n", commit.Parent[i])
	}
	fmt.Printf("Author %s at %s\n", commit.Author, commit.AuthorDate.Format(time.RFC3339))
	fmt.Printf("Committer %s at %s\n", commit.Committer, commit.CommitterDate.Format(time.RFC3339))
	fmt.Printf("\n%s", commit.Message)
	fmt.Printf("\n")
}

type LanedCommit struct {
	Commit
	IsHEAD        bool
	Refs          []Ref
	Lane          int
	LanesAfter    [MaxLanes]bool
	ParentLane    []int
	ShiftLeftFrom int
}

func (lc *LanedCommit) Occupied() int {
	occupied := lc.Lane + 1
	for i := lc.Lane; i < MaxLanes; i++ {
		if lc.LanesAfter[i] {
			occupied = i + 1
		}
	}
	return occupied
}

func (c *Commit) ShortMessage() string {
	for i := 0; i < len(c.Message); i++ {
		if c.Message[i] == '\n' {
			return c.Message[:i]
		}
	}
	return c.Message
}

func abbrev(s string) string {
	if len(s) > 6 {
		return s[:6]
	}
	return s
}

func (lc *Commit) NiceWithAbbrev() string {
	return fmt.Sprintf("%s - %s", abbrev(lc.Id), lc.ShortMessage())
}

func (lc *LanedCommit) Print() {
	occupied := lc.Occupied()
	for i := 0; i < occupied; i++ {
		if lc.Lane == i {
			if lc.Lane == MaxLanes-1 {
				fmt.Printf("E")
			} else {
				fmt.Printf("*")
			}
		} else {
			fmt.Printf(" ")
		}
	}
	fmt.Printf(" ")
	var refsout bytes.Buffer
	if len(lc.Refs) > 0 {
		fmt.Fprintf(&refsout, "[")
		for i, ref := range lc.Refs {
			io.WriteString(&refsout, ref.Nice())
			if i != len(lc.Refs)-1 {
				io.WriteString(&refsout, ", ")
			}
		}
		fmt.Fprintf(&refsout, "]")
	}
	fmt.Printf("%s %s %s %s\n", abbrev(lc.Id), refsout.String(), lc.ShortMessage(), lc.CommitterDate.Local().Format("2006-01-02 15:04"))
}

var bookmarks = map[string]LanedCommit{}

func bookmarksAsSlice() []LanedCommit {
	r := make([]LanedCommit, 0, len(bookmarks))
	for _, lc := range bookmarks {
		r = append(r, lc)
	}
	return r
}

type PopupWindow interface {
	Update(mw *nucular.MasterWindow) bool
}

var lw LogWindow
var idxmw IndexManagerWindow
var popupWindows []PopupWindow

const (
	graphTabIndex = 0
	indexTabIndex = 1
)

type Tab interface {
	Title() string
	Update(mw *nucular.MasterWindow)
}

var tabs []Tab
var currentTab int

func openTab(tab Tab) {
	tabs = append(tabs, tab)
	currentTab = len(tabs) - 1
}

func closeTab(tab Tab) {
	for i := range tabs {
		if tab == tabs[i] {
			if currentTab == i && currentTab > 0 {
				currentTab--
			}

			copy(tabs[i:], tabs[i+1:])
			tabs = tabs[:len(tabs)-1]
		}
	}
}

func guiUpdate(mw *nucular.MasterWindow) {
	w := mw.Wnd

	for _, e := range w.Input().Keyboard.Keys {
		switch {
		case (e.Modifiers == key.ModControl || e.Modifiers == key.ModControl|key.ModShift) && (e.Rune == '+') || (e.Rune == '='):
			conf.Scaling += 0.1
			mw.SetStyle(nucular.StyleFromTheme(nucular.DarkTheme), nil, conf.Scaling)
			style, _ := mw.Style()
			style.Selectable.Normal.Data.Color = style.NormalWindow.Background
			saveConfiguration()

		case (e.Modifiers == key.ModControl || e.Modifiers == key.ModControl|key.ModShift) && (e.Rune == '-'):
			conf.Scaling -= 0.1
			mw.SetStyle(nucular.StyleFromTheme(nucular.DarkTheme), nil, conf.Scaling)
			style, _ := mw.Style()
			style.Selectable.Normal.Data.Color = style.NormalWindow.Background
			saveConfiguration()

		case (e.Modifiers == 0) && ((e.Code == key.CodeEscape) || (e.Code == key.CodeQ)):
			if currentTab != graphTabIndex && currentTab != indexTabIndex {
				closeTab(tabs[currentTab])
			}

		case (e.Modifiers == key.ModControl) && (e.Code == key.CodeR):
			switch currentTab {
			case graphTabIndex:
				// (*LogWindow).reload calls Update which can not be called from inside the update function
				go func() {
					lw.mu.Lock()
					lw.reload()
					lw.mu.Unlock()
				}()

			case indexTabIndex:
				go idxmw.reload()
			}
		}
	}

	if len(popupWindows) > 0 {
		open := popupWindows[0].Update(mw)
		if !open {
			copy(popupWindows, popupWindows[1:])
			popupWindows = popupWindows[:len(popupWindows[1:])]
		}
	}

	closetab := -1
	w.LayoutRowDynamic(25, len(tabs))
	for i := range tabs {
		selected := i == currentTab
		bounds := w.WidgetBounds()
		if nucular.InputMouseClicked(mw.Wnd.Input(), mouse.ButtonMiddle, bounds) {
			if i >= 2 {
				closetab = i
			}
		}
		w.SelectableLabel(tabs[i].Title(), nucular.TextCentered, &selected)
		if selected {
			currentTab = i
		}
	}

	if closetab >= 0 {
		closeTab(tabs[closetab])
	}

	tabs[currentTab].Update(mw)
}

type Clipboard struct {
}

func (cb *Clipboard) Copy(text string) {
	clipboard.Set(text)
}

func (cb *Clipboard) Paste() string {
	return clipboard.Get()
}

func main() {
	repodir := findRepository()

	loadConfiguration()

	wnd := nucular.NewMasterWindow(guiUpdate, nucular.WindowNoScrollbar)
	wnd.SetClipboard(&Clipboard{})
	wnd.SetStyle(nucular.StyleFromTheme(nucular.DarkTheme), nil, conf.Scaling)
	style, _ := wnd.Style()
	style.Selectable.Normal.Data.Color = style.NormalWindow.Background

	lw.repodir = repodir
	lw.edOutput.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditReadOnly
	lw.edCommit.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditReadOnly
	lw.needsMore = -1
	lw.mw = wnd

	openTab(&lw)

	idxmw.repodir = repodir
	idxmw.selected = -1
	idxmw.mw = wnd
	idxmw.fmtwidth = 70
	idxmw.ed.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditClipboard
	idxmw.reload()

	openTab(&idxmw)

	status := gitStatus(repodir)
	if len(status.Lines) == 0 {
		currentTab = graphTabIndex
	} else {
		currentTab = indexTabIndex
	}

	clipboard.Start()

	wnd.Main()
}
