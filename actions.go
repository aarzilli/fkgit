package main

import (
	"fmt"
)

func viewAction(lc LanedCommit) {
	fmt.Printf("view %#v\n", lc)
}

func newbranchAction(branchname, commitId string) {
	fmt.Printf("newbranch\n")
}

func checkoutAction(refOrCommitId string) {
	fmt.Printf("checkout of %s\n", refOrCommitId)
}

func cherrypickAction(commitId string) {
	fmt.Printf("cherrypick %s\n", commitId)
}

func fetchAction(repository string) {
	fmt.Printf("fetch %s\n", repository)
}

func pullAction(repository string) {
	fmt.Printf("pull %s\n", repository)
}

func rebaseAction(commitId string) {
	fmt.Printf("rebase %s\n", commitId)
}

func pushAction(repository string) {
	fmt.Printf("push %s\n", repository)
	// if push fails ask user if he wants to try forcing
}

func resetAction(ref *Ref, commitId string, resetMode resetMode) {
	fmt.Printf("reset\n")
}

func remoteAction(action, remote string) {
	fmt.Printf("remote %s\n", action)
}

func mergeAction(ref *Ref) {
	fmt.Printf("merging %s\n", ref.Nice())
}

func diffAction(niceNameA, commitOrRefA, niceNameB, commitOrRefB string) {
	fmt.Printf("diff\n")
}
