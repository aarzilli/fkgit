package main

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"strings"
	"sync"

	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/label"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var githubStuff *GithubStuff

func parseGithubRemote(remote string) (owner, repo string) {
	const prefix = "git@github.com:"
	const suffix = ".git"

	if !strings.HasPrefix(remote, prefix) || !strings.HasSuffix(remote, suffix) {
		return
	}

	v := strings.Split(remote[len(prefix):len(remote)-len(suffix)], "/")
	if len(v) != 2 {
		return
	}

	owner = v[0]
	repo = v[1]
	return
}

func initGithubIntegration(mw nucular.MasterWindow) {
	remotes := allRemotes()
	owner, repo := parseGithubRemote(remotes["origin"])
	if repo == "" {
		return
	}
	githubStuff = NewGithubStuff(mw, owner, repo)
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
	owner string
	repo  string
	mw    nucular.MasterWindow

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

func NewGithubStuff(mw nucular.MasterWindow, owner, repo string) *GithubStuff {
	var gs GithubStuff
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
		login := ""
		if pr.Head != nil && pr.Head.Repo != nil && pr.Head.Repo.Owner != nil {
			login = *pr.Head.Repo.Owner.Login
		}
		gs.prs = append(gs.prs, pull{number: *pr.Number, url: *pr.HTMLURL, title: *pr.Title, owner: login, branch: branch})
	}

	gs.loaded = true
	gs.loaderr = false
	gs.mw.Changed()
}

func githubRemoteRef(refs []Ref, allRemotes map[string]string) *Ref {
	for i := range refs {
		ref := &refs[i]
		_, repo := parseGithubRemote(allRemotes[ref.Remote()])
		if repo != "" {
			return ref
		}
	}
	return nil
}

func (gs *GithubStuff) pullRequest(lw *LogWindow, title string, ref *Ref, lc LanedCommit) {
	lw.showOutput = true
	lw.edOutput.Buffer = lw.edOutput.Buffer[:0]

	remotes := allRemotes()

	owner, repo := parseGithubRemote(remotes[ref.Remote()])

	commits, err := allCommits("origin^1..HEAD").ReadAll()
	if err != nil {
		lw.edOutput.Buffer = append(lw.edOutput.Buffer, []rune(fmt.Sprintf("Can not create Pull Request: %v\n", err))...)
		return
	}
	originCommit := commits[len(commits)-1]
	commits = commits[:len(commits)-1]

	var originRef *Ref
	var originOwner, originRepo string
	for i := range lw.allrefs {
		ref := &lw.allrefs[i]
		if ref.CommitId == originCommit.Id {
			originOwner, originRepo = parseGithubRemote(remotes[ref.Remote()])
			if originRepo != "" && originOwner == gs.owner && originRepo == gs.repo && ref.Nice() != "origin/HEAD" {
				originRef = ref
				break
			}
		}
	}
	if originRef == nil {
		lw.edOutput.Buffer = append(lw.edOutput.Buffer, []rune(fmt.Sprintf("Can not create Pull Request: could not find origin ref\n"))...)
		return
	}

	if originRepo != repo {
		lw.edOutput.Buffer = append(lw.edOutput.Buffer, []rune(fmt.Sprintf("repo mismatch %s %s\n", originRepo, repo))...)
		return
	}

	if len(commits) > 10 {
		lw.edOutput.Buffer = append(lw.edOutput.Buffer, []rune(fmt.Sprintf("Too many commits %d\n", len(commits)))...)
		return
	}

	base := originRef.Nice()
	if i := strings.Index(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	head := ref.Nice()
	if i := strings.Index(head, "/"); i >= 0 {
		head = owner + ":" + head[i+1:]
	}

	lw.edOutput.Buffer = append(lw.edOutput.Buffer, []rune(fmt.Sprintf("Generating Pull Request for %s (repository: %s/%s) with %d commits from %s\n", base, originOwner, repo, len(commits), head))...)

	if title == "" {
		title = commits[0].ShortMessage()
	}

	body := PRMessageBody(commits)

	bodystr := string(body)

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB")},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	client := github.NewClient(tc)
	pr, _, err := client.PullRequests.Create(originOwner, repo, &github.NewPullRequest{Title: &title, Head: &head, Base: &base, Body: &bodystr})
	if err != nil {
		lw.edOutput.Buffer = append(lw.edOutput.Buffer, []rune(fmt.Sprintf("Error creating pull request: %v\n", err))...)
		return
	}
	openUrl(*pr.HTMLURL)
}

func (iw *GithubIssuesWindow) Title() string {
	return "Github issues"
}

func (pw *GithubPullWindow) Title() string {
	return "Github pull requests"
}

func (gs *GithubStuff) update(issues bool, w *nucular.Window) {
	style := w.Master().Style()
	lnh := int(style.Scaling * 20)

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
				above, below := sw.Invisible(10)
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
				if w := w.ContextualOpen(0, image.Point{200, 500}, bounds, nil); w != nil {
					gs.selectedIssue = i
					w.Row(20).Dynamic(1)
					if w.MenuItem(label.TA("Open", "LC")) {
						openUrl(is.url)
					}
					if w.MenuItem(label.TA("Create Fix Branch", "LC")) {
						newbranchAction(&lw, fmt.Sprintf("fix-%d", is.number), "master")
					}
				}
			} else {
				pr := gs.prs[i]
				sw.SelectableLabel(fmt.Sprintf("%d", pr.number), "RC", &selected)
				sw.SelectableLabel(pr.title, "LC", &selected)
				if selected {
					gs.selectedPull = i
				}
				if w := w.ContextualOpen(0, image.Point{200, 500}, bounds, nil); w != nil {
					gs.selectedPull = i
					w.Row(20).Dynamic(1)
					if w.MenuItem(label.TA("Open", "LC")) {
						openUrl(pr.url)
					}
					if w.MenuItem(label.TA("Import", "LC")) {
						importPR(&lw, pr.owner, gs.repo, pr.branch)
					}
				}
			}
		}
	}
}

func (pw *GithubPullWindow) Update(w *nucular.Window) {
	pw.gs.update(false, w)
}

func (pw *GithubPullWindow) Protected() bool {
	return false
}

func (iw *GithubIssuesWindow) Update(w *nucular.Window) {
	iw.gs.update(true, w)
}

func (pw *GithubIssuesWindow) Protected() bool {
	return false
}

func PRMessageBody(commits []Commit) []byte {
	body := new(bytes.Buffer)
	for i := range commits {
		if i > 0 {
			body.WriteByte('\n')
		}
		prMessageBody1(body, &commits[i], len(commits) != 1)
	}
	return body.Bytes()
}

func prMessageBody1(body *bytes.Buffer, commit *Commit, addtitle bool) {
	title := commit.ShortMessage()
	if addtitle {
		fmt.Fprintf(body, "### %s\n\n", title)
	}
	msg := strings.Trim(commit.Message[len(title):], "\n") + "\n"

	lines := strings.Split(msg, "\n")

	for i := range lines {
		line := &lines[i]
		if len(*line) <= 0 {
			continue
		}
		if strings.HasPrefix(*line, "- ") {
			*line = "* " + (*line)[2:]
		}

		if (*line)[len(*line)-1] == '.' {
			isList := strings.HasPrefix(*line, "* ")
			for i := 0; i < len(*line); i++ {
				if (*line)[i] < '0' || (*line)[i] > '9' {
					if (*line)[i] == '.' && i != 0 && i+2 < len(*line) && (*line)[i+1] == ' ' {
						isList = true
					}
					break
				}
			}
			isVerbatim := strings.HasPrefix(*line, " ") || strings.HasPrefix(*line, "\t")
			nextIsNewline := i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == ""
			if !isList && !isVerbatim && !nextIsNewline {
				*line = *line + "\n"
			}
		}
	}

	body.Write([]byte(strings.Join(lines, "\n")))
}
