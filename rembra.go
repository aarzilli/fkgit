package main

import (
	"image"
	"os/exec"
	"sort"
	"strings"

	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/label"
)

type remotesTab struct {
	repodir     string
	ed          nucular.TextEditor
	remotes     map[string]string
	remoteNames []string
}

func newRemotesTab(repodir string) {
	rt := &remotesTab{}
	rt.repodir = repodir
	rt.ed.Flags = nucular.EditSelectable | nucular.EditClipboard
	rt.loadRemotes()
	sort.Strings(rt.remoteNames)
	openTab(rt)
}

func (rt *remotesTab) loadRemotes() {
	rt.remotes = allRemotes(rt.repodir)
	rt.remoteNames = make([]string, 0, len(rt.remotes))
	for k := range rt.remotes {
		rt.remoteNames = append(rt.remoteNames, k)
	}
}

func (rt *remotesTab) Title() string {
	return "Remotes"
}

func (rt *remotesTab) Update(w *nucular.Window) {
	w.Row(25).Static(90, 0)
	w.Label("Filter:", "LC")
	rt.ed.Edit(w)

	needle := string(rt.ed.Buffer)
	w.Row(0).Dynamic(1)
	if w := w.GroupBegin("remotes", nucular.WindowNoHScrollbar); w != nil {
		w.Row(20).Static(180, 0, 100)
		for _, name := range rt.remoteNames {
			if strings.Index(name, needle) < 0 {
				continue
			}
			w.Label(name, "LC")
			w.Label(rt.remotes[name], "LC")
			if w.ButtonText("Delete") {
				execBackground(true, &lw, "git", "remote", "remove", name)
				go func() {
					lw.mu.Lock()
					lw.reload()
					lw.mu.Unlock()
				}()
				rt.loadRemotes()
			}
		}
		w.GroupEnd()
	}
}

type refsTab struct {
	repodir     string
	ed          nucular.TextEditor
	refs        []Ref
	commits     map[string]Commit
	selectedRef Ref
}

func newRefsTab(repodir string) {
	rt := &refsTab{}
	rt.repodir = repodir
	rt.ed.Flags = nucular.EditSelectable | nucular.EditClipboard
	rt.loadRefs()
	openTab(rt)
}

type refsByCommitDate struct {
	refs    []Ref
	commits map[string]Commit
}

func (v refsByCommitDate) Len() int      { return len(v.refs) }
func (v refsByCommitDate) Swap(i, j int) { temp := v.refs[i]; v.refs[i] = v.refs[j]; v.refs[j] = temp }
func (v refsByCommitDate) Less(i, j int) bool {
	a := v.commits[v.refs[i].CommitId]
	b := v.commits[v.refs[j].CommitId]
	if a.CommitterDate == b.CommitterDate {
		if len(v.refs[i].Name) == len(v.refs[j].Name) {
			return v.refs[i].Name < v.refs[j].Name
		} else {
			return len(v.refs[i].Name) < len(v.refs[j].Name)
		}
	} else {
		return !a.CommitterDate.Before(b.CommitterDate)
	}
}

func (rt *refsTab) loadRefs() {
	rt.refs, _ = allRefs(rt.repodir)
	if rt.commits == nil {
		rt.commits = map[string]Commit{}
	}

	for _, ref := range rt.refs {
		if _, ok := rt.commits[ref.CommitId]; !ok {
			rt.commits[ref.CommitId] = Commit{}
		}
	}

	rt.selectedRef = Ref{}

	for commitId := range rt.commits {
		if rt.commits[commitId].Id != "" {
			continue
		}
		cmd := exec.Command("git", "show", "--pretty=raw", "--no-color", commitId)
		cmd.Dir = rt.repodir
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			continue
		}
		err = cmd.Start()
		if err != nil {
			continue
		}
		rt.commits[commitId], _, _ = readCommit(stdout)
		stdout.Close()
	}
	sort.Sort(refsByCommitDate{rt.refs, rt.commits})
}

func (rt *refsTab) Title() string {
	return "Refs"
}

func (rt *refsTab) Update(w *nucular.Window) {
	w.Row(25).Static(90, 0)
	w.Label("Filter:", "LC")
	rt.ed.Edit(w)

	needle := string(rt.ed.Buffer)

	style := w.Master().Style()

	datesz := nucular.FontWidth(style.Font, "0000-00-00 00:000") + style.Text.Padding.X*2
	idsz := nucular.FontWidth(style.Font, "0000000") + style.Text.Padding.X*2

	w.Row(0).Dynamic(1)
	if w := w.GroupBegin("remotes", nucular.WindowNoHScrollbar); w != nil {
		w.Row(20).StaticScaled(idsz, 0, datesz)
		update := false
		for i := range rt.refs {
			ref := &rt.refs[i]
			name := ref.Nice()
			if strings.Index(name, needle) < 0 {
				continue
			}
			commit := rt.commits[ref.CommitId]
			selected := rt.selectedRef == *ref
			rowwidth := w.LayoutAvailableWidth()
			w.SelectableLabel(abbrev(ref.CommitId), "LC", &selected)
			rowbounds := w.LastWidgetBounds
			rowbounds.W = rowwidth
			w.SelectableLabel(name, "LC", &selected)
			w.SelectableLabel(commit.CommitterDate.Local().Format("2006-01-02 15:04"), "RC", &selected)
			if selected {
				rt.selectedRef = *ref
			}
			if w := w.ContextualOpen(0, image.Point{200, 500}, rowbounds, nil); w != nil {
				w.Row(20).Dynamic(1)
				rt.selectedRef = *ref

				if !ref.IsHEAD {
					if w.MenuItem(label.TA("checkout", "LC")) {
						checkoutAction(&lw, &rt.selectedRef, "")
						update = true
					}
				}
				if ref.Kind == RemoteRef {
					if w.MenuItem(label.TA("remove remote", "LC")) {
						execBackground(true, &lw, "git", "branch", "-rD", rt.selectedRef.nice)
						execBackground(true, &lw, "git", "push", rt.selectedRef.Remote(), "--delete", rt.selectedRef.nice[len(rt.selectedRef.Remote())+1:])
						update = true
					}
				} else {
					if w.MenuItem(label.TA("remove", "LC")) {
						execBackground(true, &lw, "git", "branch", "-D", rt.selectedRef.nice)
						update = true
					}
				}
			}
		}
		if update {
			rt.loadRefs()
			go func() {
				lw.mu.Lock()
				lw.reload()
				lw.mu.Unlock()
			}()
		}
		w.GroupEnd()
	}
}
