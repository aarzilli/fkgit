package main

import (
	"sort"
	"strings"

	"github.com/aarzilli/nucular"
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
	repodir string
	ed      nucular.TextEditor
	refs    []Ref
}

func newRefsTab(repodir string) {
	rt := &refsTab{}
	rt.repodir = repodir
	rt.ed.Flags = nucular.EditSelectable | nucular.EditClipboard
	rt.loadRefs()
	openTab(rt)
}

type refsByName []Ref

func (v refsByName) Len() int           { return len(v) }
func (v refsByName) Swap(i, j int)      { temp := v[i]; v[i] = v[j]; v[j] = temp }
func (v refsByName) Less(i, j int) bool { return v[i].nice < v[j].nice }

func (rt *refsTab) loadRefs() {
	rt.refs, _ = allRefs(rt.repodir)
	//TODO:
	// - decorate with associated commit id and title
	// - sort by commit date
	sort.Sort(refsByName(rt.refs))
}

func (rt *refsTab) Title() string {
	return "Refs"
}

func (rt *refsTab) Update(w *nucular.Window) {
	w.Row(25).Static(90, 0)
	w.Label("Filter:", "LC")
	rt.ed.Edit(w)

	needle := string(rt.ed.Buffer)
	w.Row(0).Dynamic(1)
	if w := w.GroupBegin("remotes", nucular.WindowNoHScrollbar); w != nil {
		w.Row(20).Dynamic(1)
		for i := range rt.refs {
			name := rt.refs[i].Nice()
			if strings.Index(name, needle) < 0 {
				continue
			}
			w.Label(name, "LC")
			// TODO:
			// - show commit id
			// - show commit text and date
			// - contextual menu with checkout, remove and remove-remote (if remote ref)
		}
		w.GroupEnd()
	}
}
