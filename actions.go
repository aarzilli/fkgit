package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func viewAction(lw *LogWindow, lc LanedCommit) {
	NewViewWindow(lw.repodir, lc)
}

func newbranchAction(lw *LogWindow, branchname, commitId string) {
	execBackground(lw, "git", "checkout", "-b", branchname, commitId)
}

func checkoutAction(lw *LogWindow, ref *Ref, commitId string) {
	if ref != nil {
		execBackground(lw, "git", "checkout", ref.Nice())
	} else {
		execBackground(lw, "git", "checkout", commitId)
	}
}

func cherrypickAction(lw *LogWindow, commitId string) {
	execBackground(lw, "git", "cherry-pick", commitId)
}

func pushAction(lw *LogWindow, force bool, repository string) {
	if force {
		execBackground(lw, "git", "push", "--force", repository)
	} else {
		go func() {
			out, err := execCommand(lw.repodir, "git", "push", repository)
			if err != nil {
				lw.mu.Lock()
				popupWindows = append(popupWindows, &ForcePushPopup{repository, out})
				lw.mu.Unlock()
			}
		}()
	}
}

func rebaseAction(lw *LogWindow, commitIdOrRef string) {
	if os.Getenv("EDITOR") != "" {
		execBackground(lw, "git", "rebase", "-i", commitIdOrRef)
	} else {
		execBackground(lw, "git", "rebase", commitIdOrRef)
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
	execBackground(lw, "git", "reset", flag, commitId)
}

func remoteAction(lw *LogWindow, action, remote string) {
	if action == "push" {
		pushAction(lw, false, remote)
		return
	}
	execBackground(lw, "git", action, remote)
}

func mergeAction(lw *LogWindow, ref *Ref) {
	execBackground(lw, "git", "merge", ref.Nice())
}

func diffAction(lw *LogWindow, niceNameA, commitOrRefA, niceNameB, commitOrRefB string) {
	fmt.Printf("diff\n")
}

func execBackground(lw *LogWindow, cmdname string, args ...string) {
	go func() {
		cmd := exec.Command(cmdname, args...)
		cmd.Dir = lw.repodir
		bs, err := cmd.CombinedOutput()

		lw.mu.Lock()
		defer lw.mu.Unlock()

		out := "$ " + cmdname + " " + strings.Join(args, " ") + "\n" + string(bs)

		lw.edOutput.Buffer = []rune(out)
		lw.edOutput.Cursor = 0
		lw.showOutput = true

		if err != nil {
			popupWindows = append(popupWindows, &MessagePopup{"Error", fmt.Sprintf("Error: %v\n%s\n", err, out)})
			return
		}

		lw.reload()
	}()
}
