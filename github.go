package main

import (
	"fmt"
	"image"
	"strings"
	"sync"

	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/label"
	"github.com/google/go-github/github"
)

var githubStuff *GithubStuff

func initGithubIntegration(mw *nucular.MasterWindow, repodir string) {
	const prefix = "git@github.com:"
	const suffix = ".git"
	remotes := allRemotes(repodir)
	origin := remotes["origin"]

	if !strings.HasPrefix(origin, prefix) || !strings.HasSuffix(origin, suffix) {
		return
	}

	v := strings.Split(origin[len(prefix):len(origin)-len(suffix)], "/")
	if len(v) != 2 {
		return
	}

	owner := v[0]
	repo := v[1]

	githubStuff = NewGithubStuff(mw, repodir, owner, repo)
}

type issue struct {
	number int
	title  string
	url    string
}

type pull struct {
	number int
	title  string
	url    string
	owner  string
	branch string
}

type GithubStuff struct {
	repodir string
	owner   string
	repo    string
	mw      *nucular.MasterWindow

	selectedIssue int
	iss           []issue
	selectedPull  int
	prs           []pull

	mu      sync.Mutex
	loaded  bool
	loaderr bool
}

type GithubIssuesWindow struct {
	gs *GithubStuff
}

type GithubPullWindow struct {
	gs *GithubStuff
}

func NewGithubStuff(mw *nucular.MasterWindow, repodir, owner, repo string) *GithubStuff {
	var gs GithubStuff
	gs.repodir = repodir
	gs.owner = owner
	gs.repo = repo
	gs.mw = mw

	go gs.reload()
	return &gs
}

func NewGithubPullWindow(gs *GithubStuff) {
	var pw GithubPullWindow
	pw.gs = gs
	openTab(&pw)
}

func NewGithubIssuesWindow(gs *GithubStuff) {
	var iw GithubIssuesWindow
	iw.gs = gs
	openTab(&iw)
}

func (gs *GithubStuff) reload() {
	client := github.NewClient(nil)
	iss, _, err := client.Issues.ListByRepo(gs.owner, gs.repo, &github.IssueListByRepoOptions{State: "open", ListOptions: github.ListOptions{PerPage: 100}})
	if err != nil {
		gs.mu.Lock()
		gs.loaded = true
		gs.loaderr = true
		gs.mu.Unlock()
		return
	}

	prs, _, err := client.PullRequests.List(gs.owner, gs.repo, &github.PullRequestListOptions{State: "open", ListOptions: github.ListOptions{PerPage: 100}})
	if err != nil {
		gs.mu.Lock()
		gs.loaded = true
		gs.loaderr = true
		gs.mu.Unlock()
		return
	}

	gs.mu.Lock()
	defer gs.mu.Unlock()

	gs.selectedIssue = -1
	gs.iss = gs.iss[:0]

	for _, is := range iss {
		if strings.Index(*is.HTMLURL, "/pull/") >= 0 {
			continue
		}
		gs.iss = append(gs.iss, issue{number: *is.Number, title: *is.Title, url: *is.HTMLURL})
	}

	gs.selectedPull = -1
	gs.prs = gs.prs[:0]

	for _, pr := range prs {
		branch := *pr.Head.Label
		label := strings.Split(branch, ":")
		if len(label) > 1 {
			branch = label[1]
		}
		gs.prs = append(gs.prs, pull{number: *pr.Number, url: *pr.HTMLURL, title: *pr.Title, owner: *pr.Head.Repo.Owner.Login, branch: branch})
	}

	gs.loaded = true
	gs.loaderr = false
	gs.mw.Changed()
}

func (iw *GithubIssuesWindow) Title() string {
	return "Github issues"
}

func (pw *GithubPullWindow) Title() string {
	return "Github pull requests"
}

func (gs *GithubStuff) update(issues bool, w *nucular.Window) {
	style, scaling := w.Master().Style()
	lnh := int(scaling * 20)

	gs.mu.Lock()
	defer gs.mu.Unlock()

	if !gs.loaded {
		w.Row(25).Dynamic(1)
		w.Label("Updating...", "LC")
		return
	}

	if gs.loaderr {
		w.Row(25).Dynamic(1)
		w.Label("Loading error", "LC")
	}

	moveselection := false

	w.Row(0).Dynamic(1)
	if sw := w.GroupBegin("issue-list", nucular.WindowNoHScrollbar); sw != nil {
		defer sw.GroupEnd()

		numsz := nucular.FontWidth(style.Font, "00000") + style.GroupWindow.Padding.X*2

		n := len(gs.iss)
		if !issues {
			n = len(gs.prs)
		}

		for i := 0; i < n; i++ {
			sw.RowScaled(lnh).StaticScaled(numsz, 0)
			selected := gs.selectedIssue == i
			if !issues {
				selected = gs.selectedPull == i
			}
			if moveselection && selected {
				above, below := sw.Invisible()
				if above || below {
					sw.Scrollbar.Y = sw.At().Y
				}
			}
			bounds := sw.WidgetBounds()
			bounds.W = sw.LayoutAvailableWidth()
			if issues {
				is := gs.iss[i]
				sw.SelectableLabel(fmt.Sprintf("%d", is.number), "RC", &selected)
				sw.SelectableLabel(is.title, "LC", &selected)
				if selected {
					gs.selectedIssue = i
				}
				im := &issueMenu{gs, i, is}
				w.ContextualOpen(0, image.Point{200, 500}, bounds, im.Update)
			} else {
				pr := gs.prs[i]
				sw.SelectableLabel(fmt.Sprintf("%d", pr.number), "RC", &selected)
				sw.SelectableLabel(pr.title, "LC", &selected)
				if selected {
					gs.selectedPull = i
				}
				pm := &pullMenu{gs, i, pr}
				w.ContextualOpen(0, image.Point{200, 500}, bounds, pm.Update)
			}
		}
	}
}

func (pw *GithubPullWindow) Update(w *nucular.Window) {
	pw.gs.update(false, w)
}

func (iw *GithubIssuesWindow) Update(w *nucular.Window) {
	iw.gs.update(true, w)
}

type issueMenu struct {
	gs *GithubStuff
	i  int
	is issue
}

func (im *issueMenu) Update(w *nucular.Window) {
	im.gs.selectedIssue = im.i
	w.Row(20).Dynamic(1)
	if w.MenuItem(label.TA("Open", "LC")) {
		openUrl(im.is.url)
	}
	if w.MenuItem(label.TA("Create Fix Branch", "LC")) {
		newbranchAction(&lw, fmt.Sprintf("fix-%d", im.is.number), "master")
	}
}

type pullMenu struct {
	gs *GithubStuff
	i  int
	pr pull
}

func (pm *pullMenu) Update(w *nucular.Window) {
	pm.gs.selectedPull = pm.i
	w.Row(20).Dynamic(1)
	if w.MenuItem(label.TA("Open", "LC")) {
		openUrl(pm.pr.url)
	}
	if w.MenuItem(label.TA("Import", "LC")) {
		importPR(&lw, pm.pr.owner, pm.gs.repo, pm.pr.branch)
	}
}
