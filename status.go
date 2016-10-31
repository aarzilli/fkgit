package main

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

type StatusLine struct {
	Index, WorkDir string
	Path           string
	Path2          string
}

type GitStatus struct {
	Branch string
	Lines  []StatusLine
}

func gitStatus() *GitStatus {
	r := GitStatus{}

	const arrow = " -> "

	bs, err := execCommand("git", "status", "--porcelain", "-b")
	must(err)
	rdr := bufio.NewScanner(bytes.NewReader([]byte(bs)))
	for rdr.Scan() {
		text := rdr.Text()
		if text[0] == '#' && text[1] == '#' {
			if text[3:] != "HEAD (no branch)" {
				r.Branch = text[3:]
			}
		} else {
			line := StatusLine{Index: string(text[0]), WorkDir: string(text[1]), Path: text[3:]}

			if idx := strings.Index(line.Path, arrow); idx >= 0 {
				line.Path2 = line.Path[idx+len(arrow):]
				line.Path = line.Path[:idx]
			}

			r.Lines = append(r.Lines, line)
		}
	}
	must(rdr.Err())
	return &r
}

func (status *GitStatus) Summary() string {
	index := false
	workdir := false
	for _, line := range status.Lines {
		if line.Index != " '" {
			index = true
		}
		if line.WorkDir != " " {
			workdir = true
		}
		if index && workdir {
			break
		}
	}

	if index && workdir {
		return fmt.Sprintf("On branch %s: Unmerged changes, dirty working directory", status.Branch)
	} else if !index && workdir {
		return fmt.Sprintf("On branch %s: Dirty working directory", status.Branch)
	} else if index && !workdir {
		return fmt.Sprintf("On branch %s: Unmerged changes", status.Branch)
	} else {
		return fmt.Sprintf("On branch %s", status.Branch)
	}
}
