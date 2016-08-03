package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

func viewAction(lw *LogWindow, lc LanedCommit) {
	NewViewWindow(lw.repodir, lc)
}

func newbranchAction(lw *LogWindow, branchname, commitId string) {
	execBackground(false, lw, "git", "checkout", "-b", branchname, commitId)
}

func checkoutAction(lw *LogWindow, ref *Ref, commitId string) {
	if ref != nil {
		execBackground(false, lw, "git", "checkout", ref.Nice())
	} else {
		execBackground(false, lw, "git", "checkout", commitId)
	}
}

func cherrypickAction(lw *LogWindow, commitId string) {
	execBackground(false, lw, "git", "cherry-pick", commitId)
}

func pushAction(lw *LogWindow, force bool, repository string) {
	if force {
		execBackground(false, lw, "git", "push", "--force", repository)
	} else {
		go func() {
			err := execBackground(true, lw, "git", "push", repository)
			if err != nil {
				lw.mu.Lock()
				newForcePushPopup(lw.mw, repository, lw.edOutput.Buffer)
				lw.mu.Unlock()
			}
		}()
	}
}

func rebaseAction(lw *LogWindow, commitIdOrRef string) {
	if os.Getenv("EDITOR") != "" {
		execBackground(false, lw, "git", "rebase", "-i", commitIdOrRef)
	} else {
		execBackground(false, lw, "git", "rebase", commitIdOrRef)
	}
}

func resetAction(lw *LogWindow, commitId string, resetMode resetMode) {
	flag := ""
	switch resetMode {
	case resetHard:
		flag = "--hard"
	case resetMixed:
		flag = "--mixed"
	case resetSoft:
		flag = "--soft"
	}
	execBackground(false, lw, "git", "reset", flag, commitId)
}

func remoteAction(lw *LogWindow, action, remote string) {
	if action == "push" {
		pushAction(lw, false, remote)
		return
	}
	execBackground(false, lw, "git", action, remote)
}

func mergeAction(lw *LogWindow, ref *Ref) {
	execBackground(false, lw, "git", "merge", ref.Nice())
}

func diffAction(lw *LogWindow, niceNameA, commitOrRefA, niceNameB, commitOrRefB string) {
	NewDiffWindow(lw.repodir, niceNameA, commitOrRefA, niceNameB, commitOrRefB)
}

func execBackground(wait bool, lw *LogWindow, cmdname string, args ...string) error {
	var done chan struct{}
	if wait {
		done = make(chan struct{})
	}
	var returnerror error
	go func() {
		if wait {
			defer close(done)
		}

		cmd := exec.Command(cmdname, args...)
		cmd.Dir = lw.repodir

		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		err := cmd.Start()

		if err == nil {
			var wg sync.WaitGroup
			filereader := func(fh io.ReadCloser) {
				defer fh.Close()
				defer wg.Done()
				bsr := make([]byte, 4096)
				for {
					n, err := fh.Read(bsr)
					if n > 0 {
						lw.mu.Lock()
						lw.edOutput.Buffer = append(lw.edOutput.Buffer, []rune(string(bsr[:n]))...)
						lw.mu.Unlock()
					}
					if err != nil {
						break
					}
				}
			}
			wg.Add(2)
			go filereader(stdout)
			go filereader(stderr)
			wg.Wait()
			err = cmd.Wait()
		}

		if err != nil {
			returnerror = err
			newMessagePopup(lw.mw, "Error", fmt.Sprintf("Error: %v\n", err))
			return
		}

		lw.reload()
	}()

	if wait {
		<-done
		return returnerror
	}

	return nil
}
