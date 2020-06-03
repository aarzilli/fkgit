package main

import (
	"testing"
)

func TestPRMessageBody(t *testing.T) {
	f1 := func(in, tgt string) {
		out := PRMessageBody([]Commit{Commit{Message: in}})
		t.Logf("%q -> %q\n", in, out)
		if string(out) != tgt {
			t.Errorf("Error: expected %q\n", tgt)
		}
	}

	fn := func(in []string, tgt string) {
		commits := make([]Commit, len(in))
		for i := range in {
			commits[i].Message = in[i]
		}
		out := PRMessageBody(commits)
		t.Logf("%q -> %q\n", in, out)
		if string(out) != tgt {
			t.Errorf("Error: expected %q\n", tgt)
		}
	}

	// remove title
	f1(`proc: something

blah blah.
`,
		`blah blah.
`)

	// autoadd markdown paragraph separator newline
	f1(`proc: something

blah blah
blah.
Blah blah blah.

Fixes #123
`,
		`blah blah
blah.

Blah blah blah.

Fixes #123
`)

	// do not autoadd markdown paragraph separator newline if it's already there
	f1(`proc: something

blah blah.

blah.`,
		`blah blah.

blah.
`)

	// do not autoadd markdown paragraph separator newline in bullet lists (and bullet list conversion)
	f1(`proc: something

- blah blah.
- blah.

asdas`,
		`* blah blah.
* blah.

asdas
`)

	// do not autoadd markdown paragraph separator newline in number lists (and bullet list conversion)
	f1(`proc: something

1. blah blah.
2. blah.

asdas`,
		`1. blah blah.
2. blah.

asdas
`)

	// do not autoadd markdown paragraph separator newline if the line starts with a space
	f1(`proc: something

blah blah
 test.
blah`,
		`blah blah
 test.
blah
`)

	// do not autoadd markdown paragraph separator newline if the line starts with a tab
	f1(`proc: something

blah blah
	test.
blah`,
		`blah blah
	test.
blah
`)

	fn([]string{
		`proc: something

first commit




`,

		`proc: something else

second commit

Fixes #123
`,
		`proc: something else still

third commit

Fixes #456
`},

		`### proc: something

first commit

### proc: something else

second commit

Fixes #123

### proc: something else still

third commit

Fixes #456
`)

}
