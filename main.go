package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/label"
	nstyle "github.com/aarzilli/nucular/style"

	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
)

var Repodir string

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func execCommand(cmdname string, args ...string) (string, error) {
	cmd := exec.Command(cmdname, args...)
	cmd.Dir = Repodir
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

func (ref *Ref) RemoteName() string {
	return ref.nice[len(ref.remote)+1:]
}

func getHead() (isref bool, reforid string, err error) {
	const refPrefix = "ref: "
	bs, err := ioutil.ReadFile(filepath.Join(filepath.Join(Repodir, ".git"), "HEAD"))
	if err != nil {
		return false, "", err
	}
	s := strings.TrimSpace(string(bs))

	if strings.HasPrefix(s, refPrefix) {
		return true, s[len(refPrefix):], nil
	}

	return false, s, nil
}

func allRefs() ([]Ref, error) {
	headisref, headref, err := getHead()
	if err != nil {
		return nil, err
	}
	if !headisref {
		headref = ""
	}

	s, err := execCommand("git", "show-ref", "-d")
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

func allRemotes() map[string]string {
	r := map[string]string{}
	s, err := execCommand("git", "remote", "-v")
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

func findRepository(arg []string) string {
	var path string
	if len(arg) > 0 {
		path = arg[0]
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

func LoadCommit(commitId string) (Commit, bool) {
	cmd := exec.Command("git", "show", "--pretty=raw", "--no-color", "--no-patch", commitId)
	cmd.Dir = Repodir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Commit{}, false
	}
	defer stdout.Close()
	err = cmd.Start()
	if err != nil {
		return Commit{}, false
	}
	go cmd.Wait()
	commit, ok, err := readCommit(stdout)
	if !ok || err != nil {
		return Commit{}, false
	}
	return commit, true
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
	Protected() bool
}

var tabs []Tab
var currentTab int

func openTab(tab Tab) {
	tabs = append(tabs, tab)
	currentTab = len(tabs) - 1
}

func closeTab(tab Tab) {
	if tab.Protected() {
		return
	}
	found := -1
	for i := range tabs {
		if tab == tabs[i] {
			if currentTab == i && currentTab > 0 {
				currentTab--
			}

			copy(tabs[i:], tabs[i+1:])
			tabs = tabs[:len(tabs)-1]
			found = i
			break
		}
	}
	if found >= 0 && currentTab > found {
		currentTab--
	} 
}

func tabIndex(tab Tab) int {
	for i := range tabs {
		if tab == tabs[i] {
			return i
		}
	}
	return -1
}

func guiUpdate(w *nucular.Window) {
	mw := w.Master()
	for _, e := range w.Input().Keyboard.Keys {
		switch {
		case (e.Modifiers == key.ModControl || e.Modifiers == key.ModControl|key.ModShift) && (e.Code == key.CodeEqualSign):
			conf.Scaling += 0.1
			mw.Style().Scale(conf.Scaling)
			saveConfiguration()

		case (e.Modifiers == key.ModControl || e.Modifiers == key.ModControl|key.ModShift) && (e.Code == key.CodeHyphenMinus):
			conf.Scaling -= 0.1
			mw.Style().Scale(conf.Scaling)
			saveConfiguration()

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
			if tabs[currentTab].Protected() {
				go mw.Close()
			} else {
				closeTab(tabs[currentTab])
			}

		case (e.Modifiers == key.ModControl) && (e.Code == key.CodeTab):
			currentTab = (currentTab + 1) % len(tabs)
		}
	}

	closetab := -1
	tabwidths := make([]int, len(tabs)+1)
	tabwidths[0] = 90
	for i := range tabs {
		tabwidths[i+1] = 0
	}
	w.Row(20).Static(tabwidths...)
	if w := w.Menu(label.TA("More...", "CC"), 200, nil); w != nil {
		w.Row(20).Dynamic(1)
		if w.MenuItem(label.TA("Remotes", "LC")) {
			newRemotesTab()
		}
		if w.MenuItem(label.TA("Refs", "LC")) {
			newRefsTab()
		}
		if githubStuff != nil {
			if w.MenuItem(label.TA("Github Issues", "LC")) {
				NewGithubIssuesWindow(githubStuff)
			}
			if w.MenuItem(label.TA("Github Pull Requests", "LC")) {
				NewGithubPullWindow(githubStuff)
			}
		}
	}
	for i := range tabs {
		selected := i == currentTab
		bounds := w.WidgetBounds()
		if w.Input().Mouse.Clicked(mouse.ButtonMiddle, bounds) {
			closetab = i
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
	rand.Seed(time.Now().Unix())

	blamefile, blamerev := "", ""

	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "help":
			fmt.Printf("Usage:\n")
			fmt.Printf("\tfkgit\n")
			fmt.Printf("\tfkgit blame [revision] file\n")
			fmt.Printf("\tfkgit help\n")
			fmt.Printf("\n")
			fmt.Printf("Call without arguments to open log/commit window\n")
			os.Exit(0)
		case "seqed", "comed":
			if os.Getenv("FKGIT_SEQUENCE_EDITOR_SOCKET") == "" {
				fmt.Fprintf(os.Stderr, "no sequence editor socket\n")
				os.Exit(1)
			}
			editmodeMain()
			return
		case "blame":
			switch len(os.Args) {
			case 3:
				blamefile = os.Args[2]
				Repodir = findRepository(nil)
			case 4:
				blamerev = os.Args[2]
				blamefile = os.Args[3]
				Repodir = findRepository(nil)
			case 5:
				blamerev = os.Args[2]
				blamefile = os.Args[3]
				Repodir = findRepository(os.Args[4:])
			}
			if len(os.Args) < 3 {
				fmt.Fprintf(os.Stderr, "blame needs an argument\n")
				os.Exit(1)
			}
		default:
			Repodir = findRepository(os.Args[1:])
		}
	} else {
		Repodir = findRepository(nil)
	}

	if Repodir == "" {
		fmt.Fprintf(os.Stderr, "could not find repository\n")
		os.Exit(1)
		return
	}

	loadConfiguration()

	wnd := nucular.NewMasterWindow(guiUpdate, nucular.WindowNoScrollbar)
	wnd.SetStyle(nstyle.FromTheme(nstyle.DarkTheme, conf.Scaling))
	fixStyle(wnd.Style())

	lw.edOutput.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditReadOnly | nucular.EditClipboard
	lw.searchEd.Flags = nucular.EditSelectable | nucular.EditFocusFollowsMouse | nucular.EditSigEnter | nucular.EditClipboard
	lw.searchMode = noSearch
	lw.needsMore = -1
	lw.split.Size = 250
	lw.split.MinSize = 20
	lw.split.Spacing = 5
	lw.mw = wnd

	openTab(&lw)

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

	blameTabIndex := -1
	if blamefile != "" {
		wd, _ := os.Getwd()
		if len(blamefile) <= 0 || blamefile[0] != '/' {
			blamefile, _ = filepath.Abs(filepath.Join(wd, blamefile))
		}
		blamefile, _ = filepath.Rel(Repodir, blamefile)
		NewBlameWindow(wnd, blamerev, blamefile)
		blameTabIndex = len(tabs) - 1
	}

	initGithubIntegration(wnd)

	status := gitStatus()
	switch {
	case blameTabIndex >= 0:
		currentTab = blameTabIndex
	case len(status.Lines) != 0:
		currentTab = indexTabIndex
	default:
		currentTab = graphTabIndex
	}

	wnd.Main()
}
