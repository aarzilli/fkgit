package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/aarzilli/nucular"
)

var rebaseServerMutex sync.Mutex

func rebaseAction(lw *LogWindow, commitIdOrRef string) {
	var socname string
	var soc net.Listener
	errcount := 0
	for {
		socname = "/tmp/fkgitseqpipe." + strconv.Itoa(rand.Int())
		var err error
		soc, err = net.Listen("unixpacket", socname)
		if err == nil {
			break
		}
		addrerr, ok := err.(*net.AddrError)
		if !ok || !addrerr.Temporary() {
			newMessagePopup(lw.mw, "Error", fmt.Sprintf("Could not bind socket for rebase: %v\n", err))
			return
		}
		errcount++
		if errcount > 5 {
			newMessagePopup(lw.mw, "Error", fmt.Sprintf("Could not bind socket for rebase (repeated temporary errors): %v\n", err))
			return
		}
	}

	os.Setenv("FKGIT_SEQUENCE_EDITOR_SOCKET", socname)

	tab := &rebaseTab{soc: soc, mw: lw.mw}
	tab.newcommand("git", "rebase", "-i", commitIdOrRef)
	openTab(tab)

	go rebaseServer(soc, socname, tab)
	go tab.runcommand()
}

type rebaseTab struct {
	soc net.Listener
	mu  sync.Mutex
	mw  nucular.MasterWindow

	ed nucular.TextEditor

	// when in edit mode editing and editfile are set
	// editing will be closed when the edit is done
	// editfile must contain the file to edit
	editing  chan struct{}
	editfile string
	editload bool

	// when waiting for a background command to complete cmd is not nil
	cmd *exec.Cmd

	// when the rebase process is finished done is true
	done bool
}

func rebaseServer(soc net.Listener, socname string, tab *rebaseTab) {
	defer os.Remove(socname)
	for {
		conn, err := soc.Accept()
		if err != nil {
			return
		}
		b := bufio.NewReader(conn)
		bs, err := b.ReadBytes('\x00')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading rebase socket message: %v\n", err)
			continue
		}
		typ := bs[0]
		switch typ {
		case 's':
			tab.mu.Lock()
			tab.editfile = string(bs[1 : len(bs)-1])
			tab.editing = make(chan struct{})
			tab.editload = true
			tab.mu.Unlock()
			<-tab.editing
		case 'c':
			idxmw.rebasedit = string(bs[1 : len(bs)-1])
			idxmw.rebasechan = make(chan struct{})
			idxmw.rebaseTab = tab
			if i := tabIndex(&idxmw); i >= 0 {
				currentTab = i
			}
			<-idxmw.rebasechan
		}

		conn.Write([]byte{'\x00'})
		conn.Close()
	}
}

func (rt *rebaseTab) Title() string {
	return "Rebase"
}

func (rt *rebaseTab) Protected() bool {
	return !rt.done
}

func (rt *rebaseTab) Update(w *nucular.Window) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	w.LayoutReserveRow(25, 1)

	w.Row(0).Dynamic(1)

	switch {
	case rt.editing != nil: // editing mode
		if rt.editload {
			rt.editload = false
			bs, err := ioutil.ReadFile(rt.editfile)
			if err != nil {

				close(rt.editing)
				rt.editing = nil
				rt.ed.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditReadOnly | nucular.EditClipboard
				rt.done = true
				rt.ed.Buffer = rt.ed.Buffer[:0]
				rt.ed.Buffer = append(rt.ed.Buffer, []rune(fmt.Sprintf("Error loading %s: %v\n", rt.editfile, err))...)
				break
			}
			rt.ed.Buffer = []rune(string(bs))
			rt.ed.Cursor = 0
		}

		rt.ed.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditClipboard
		rt.ed.Edit(w)
		w.Row(25).Static(0, 100, 100)
		w.Spacing(1)
		if w.ButtonText("Ok") {
			ioutil.WriteFile(rt.editfile, []byte(string(rt.ed.Buffer)), 0666)
			rt.ed.Buffer = rt.ed.Buffer[:0]
			close(rt.editing)
			rt.editing = nil
		}
		if w.ButtonText("Cancel") {
			close(rt.editing)
			rt.editing = nil
		}

	case rt.cmd != nil: // a command is being executed in background and we are waiting for it to finish
		rt.ed.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditClipboard | nucular.EditReadOnly
		rt.ed.Edit(w)
		w.Row(25).Static(0)
		w.Spacing(1)

	default: // rebase is temorarily stopped waiting for user to correct something
		rt.ed.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditClipboard | nucular.EditReadOnly
		rt.ed.Edit(w)
		w.Row(25).Static(0, 100, 100, 100, 100)

		c := func(printcmd bool, cmd string, args ...string) {
			rt.ed.Buffer = rt.ed.Buffer[:0]
			if printcmd {
				rt.ed.Buffer = append(rt.ed.Buffer, []rune("$ "+cmd+strings.Join(args, " ")+"\n")...)
			}
			rt.newcommand(cmd, args...)
			go rt.runcommand()
		}

		w.Spacing(1)
		if w.ButtonText("Edit Todo") {
			c(false, "git", "rebase", "--edit-todo")
		}
		if w.ButtonText("Skip") {
			c(true, "git", "rebase", "--skip")
		}
		if w.ButtonText("Abort") {
			c(true, "git", "rebase", "--abort")
		}
		if w.ButtonText("Continue") {
			c(true, "git", "rebase", "--continue")
		}

	case rt.done: // rebase is finished
		rt.ed.Flags = nucular.EditSelectable | nucular.EditMultiline | nucular.EditFocusFollowsMouse | nucular.EditClipboard | nucular.EditReadOnly
		rt.ed.Edit(w)
		w.Row(25).Static(0, 100)
		w.Spacing(1)
		if w.ButtonText("Done") {
			closeTab(rt)
		}
	}
}

func (rt *rebaseTab) runcommand() {
	stdout, _ := rt.cmd.StdoutPipe()
	stderr, _ := rt.cmd.StderrPipe()
	var wg sync.WaitGroup

	filereader := func(fh io.ReadCloser) {
		defer fh.Close()
		defer wg.Done()
		bsr := make([]byte, 4096)
		for {
			n, err := fh.Read(bsr)
			if n > 0 {
				rt.mu.Lock()
				rt.ed.Buffer = append(rt.ed.Buffer, []rune(string(bsr[:n]))...)
				rt.mw.Changed()
				rt.mu.Unlock()
			}
			if err != nil {
				break
			}
		}
	}

	err := rt.cmd.Start()
	if err == nil {
		wg.Add(2)
		go filereader(stdout)
		go filereader(stderr)
		wg.Wait()
		err = rt.cmd.Wait()
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if err != nil {
		rt.ed.Buffer = append(rt.ed.Buffer, []rune(fmt.Sprintf("\n%v\n", err.Error()))...)
	}
	rt.cmd = nil

	f1, _ := os.Stat(filepath.Join(Repodir, ".git", "rebase-merge"))
	f2, _ := os.Stat(filepath.Join(Repodir, ".git", "rebase-apply"))
	rt.done = f1 == nil && f2 == nil
	if rt.done {
		rt.soc.Close()
	}
}

func (rt *rebaseTab) newcommand(cmd string, args ...string) {
	rt.cmd = exec.Command(cmd, args...)
	rt.cmd.Env = append(os.Environ(), "GIT_SEQUENCE_EDITOR=fkgit seqed", "GIT_EDITOR=fkgit comed")
	rt.cmd.Dir = Repodir
}

func editmodeMain() {
	soc, err := net.Dial("unixpacket", os.Getenv("FKGIT_SEQUENCE_EDITOR_SOCKET"))
	must(err)
	switch os.Args[1] {
	case "seqed":
		soc.Write([]byte("s" + os.Args[2] + "\x00"))
	case "comed":
		soc.Write([]byte("c" + os.Args[2] + "\x00"))
	}
	bs := []byte{0}
	soc.Read(bs)
	soc.Close()
}
