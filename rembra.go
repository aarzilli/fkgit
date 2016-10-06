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
	w.Row(20).Static(180, 0, 100)
	for _, name := range rt.remoteNames {
		if strings.Index(name, needle) < 0 {
			continue
		}
		w.Label(name, "LC")
		w.Label(rt.remotes[name], "LC")
		if w.ButtonText("Delete") {
			execBackground(true, &lw, "git", "remote", "remove", name)
			rt.loadRemotes()
		}
	}
}

func newBranchesTab(repodir string) {
	//TODO: implement
}
