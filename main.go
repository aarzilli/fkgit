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

	"github.com/aarzilli/nucular"
	nstyle "github.com/aarzilli/nucular/style"

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

func allRemotes(repodir string) map[string]string {
	r := map[string]string{}
	s, err := execCommand(repodir, "git", "remote", "-v")
	must(err)
	rlines := strings.Split(s, "\n")
	for _, line := range rlines {
		fields := strings.Split(line, "\t")
		if len(fields) != 2 {
			continue
		}
		name := fields[0]
		fields = strings.Split(fields[1], " ")
		url := fields[0]
		r[name] = url
	}
	return r
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

var lw LogWindow
var idxmw IndexManagerWindow

const (
	graphTabIndex = 0
	indexTabIndex = 1
)

type Tab interface {
	Title() string
	Update(w *nucular.Window)
}

var tabs []Tab
var currentTab int
var fixedTabsLimit int

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
			break
		}
	}
}

func guiUpdate(w *nucular.Window) {
	mw := w.Master()
	for _, e := range w.Input().Keyboard.Keys {
		switch {
		case (e.Modifiers == key.ModControl || e.Modifiers == key.ModControl|key.ModShift) && (e.Rune == '+') || (e.Rune == '='):
			conf.Scaling += 0.1
			mw.SetStyle(nstyle.FromTheme(nstyle.DarkTheme), nil, conf.Scaling)
			style, _ := mw.Style()
			fixStyle(style)
			saveConfiguration()

		case (e.Modifiers == key.ModControl || e.Modifiers == key.ModControl|key.ModShift) && (e.Rune == '-'):
			conf.Scaling -= 0.1
			mw.SetStyle(nstyle.FromTheme(nstyle.DarkTheme), nil, conf.Scaling)
			style, _ := mw.Style()
			fixStyle(style)
			saveConfiguration()

		case (e.Modifiers == 0) && ((e.Code == key.CodeEscape) || (e.Code == key.CodeQ)):
			if currentTab > fixedTabsLimit {
				closeTab(tabs[currentTab])
			}

		case (e.Modifiers == key.ModControl) && (e.Code == key.CodeR):
			switch currentTab {
			case graphTabIndex:
				lw.mu.Lock()
				lw.reload()
				lw.mu.Unlock()

			case indexTabIndex:
				go idxmw.reload()
			}

		case (e.Modifiers == key.ModControl) && (e.Code == key.CodeW):
			go mw.Close()

		case (e.Modifiers == key.ModControl) && (e.Code == key.CodeTab):
			currentTab = (currentTab + 1) % len(tabs)
		}
	}

	closetab := -1
	w.Row(20).Dynamic(len(tabs))
	for i := range tabs {
		selected := i == currentTab
		bounds := w.WidgetBounds()
		if w.Input().Mouse.Clicked(mouse.ButtonMiddle, bounds) {
			if i > fixedTabsLimit {
				closetab = i
			}
		}
		w.SelectableLabel(tabs[i].Title(), "LC", &selected)
		if selected {
			currentTab = i
		}
	}

	if closetab >= 0 {
		closeTab(tabs[closetab])
	}

	tabs[currentTab].Update(w)
}

func fixStyle(style *nstyle.Style) {
	style.Selectable.Normal.Data.Color = style.NormalWindow.Background
	//style.NormalWindow.Padding.Y = 0
	//style.GroupWindow.Padding.Y = 0
	//style.GroupWindow.FooterPadding.Y = 0
	style.MenuWindow.FooterPadding.Y = 0
	style.ContextualWindow.FooterPadding.Y = 0
}

func main() {
	repodir := findRepository()
	if repodir == "" {
		fmt.Fprintf(os.Stderr, "could not find repository\n")
		os.Exit(1)
		return
	}

	loadConfiguration()

	wnd := nucular.NewMasterWindow(guiUpdate, nucular.WindowNoScrollbar)
	wnd.SetStyle(nstyle.FromTheme(nstyle.DarkTheme), nil, conf.Scaling)
	style, _ := wnd.Style()
	fixStyle(style)

	lw.repodir = repodir
	lw.edOutput.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditReadOnly | nucular.EditClipboard
	lw.edCommit.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditReadOnly | nucular.EditClipboard
	lw.searchEd.Flags = nucular.EditSelectable | nucular.EditFocusFollowsMouse | nucular.EditSigEnter | nucular.EditClipboard
	lw.searchMode = noSearch
	lw.needsMore = -1
	lw.split.Size = 250
	lw.split.MinSize = 20
	lw.split.Spacing = 5
	lw.mw = wnd

	openTab(&lw)

	idxmw.repodir = repodir
	idxmw.selected = -1
	idxmw.splitv.MinSize = 80
	idxmw.splitv.Size = 120
	idxmw.splitv.Spacing = 5
	idxmw.splith.MinSize = 100
	idxmw.splith.Size = 300
	idxmw.splith.Spacing = 5
	idxmw.mw = wnd
	idxmw.fmtwidth = 70
	idxmw.ed.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditClipboard
	idxmw.reload()

	openTab(&idxmw)

	fixedTabsLimit = 1

	initGithubIntegration(wnd, repodir)

	status := gitStatus(repodir)
	if len(status.Lines) == 0 {
		currentTab = graphTabIndex
	} else {
		currentTab = indexTabIndex
	}

	wnd.Main()
}
