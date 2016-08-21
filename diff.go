package main

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"github.com/aarzilli/nucular"
	nstyle "github.com/aarzilli/nucular/style"

	"golang.org/x/image/font"
	"golang.org/x/mobile/event/key"
)

const wordDiffDebug = false

type Diff []FileDiff

type FileDiff struct {
	Num      int
	Filename string
	Headers  []Chunk
	Lines    []LineDiff
}

type LineDiffOpts int

const (
	LineDiffOptsNone LineDiffOpts = iota
	Addline
	Delline
)

type LineDiff struct {
	Opts   LineDiffOpts
	Chunks []Chunk
}

type ChunkOpts int

const (
	ChunkNoOpts ChunkOpts = iota
	Hunkhdr
	Filehdr
	Delseg
	Addseg
)

type Chunk struct {
	Opts ChunkOpts
	Text string
}

func (ld *LineDiff) IsHunkHeader() bool {
	for _, chunk := range ld.Chunks {
		if chunk.Opts == Hunkhdr {
			return true
		}
	}
	return false
}

func parseDiff(bs []byte) Diff {
	rdr := bufio.NewScanner(bytes.NewReader(bs))

	var diff Diff
	var filediff *FileDiff
	filediffBody := false
	lines := []string{}

	start := true
	merge := false

	const delprefix = "--- "
	const delprefixExtra = "a/"
	const addprefix = "+++ "
	const addprefixExtra = "b/"

	for rdr.Scan() {
		text := rdr.Text()

		if filediff == nil {
			if !strings.HasPrefix(text, "diff ") {
				if start {
					if strings.HasPrefix(text, "Merge:") {
						merge = true
					}

					continue
				} else {
					break
				}
			}

			filediff = &FileDiff{}
			filediff.Headers = append(filediff.Headers, Chunk{Opts: Filehdr, Text: text})
		} else if filediff != nil && !filediffBody {
			filediff.Headers = append(filediff.Headers, Chunk{Opts: Filehdr, Text: text})
			if strings.HasPrefix(text, delprefix) {
				delfile := text[len(delprefix):]
				if strings.HasPrefix(delfile, delprefixExtra) {
					delfile = delfile[len(delprefixExtra):]
				}
				rdr.Scan()
				text = rdr.Text()
				filediff.Headers = append(filediff.Headers, Chunk{Opts: Filehdr, Text: text})
				addfile := text[len(addprefix):]
				if strings.HasPrefix(addfile, addprefixExtra) {
					addfile = addfile[len(addprefixExtra):]
				}
				if delfile != "/dev/null" {
					filediff.Filename = delfile
				} else {
					filediff.Filename = addfile
				}
				filediffBody = true
			}
		} else if filediff != nil && filediffBody {
			if strings.HasPrefix(text, "diff ") {
				if len(lines) > 0 {
					filediff.Lines = append(filediff.Lines, diffLines(lines, merge)...)
				}
				diff = append(diff, *filediff)
				filediffBody = false
				filediff = &FileDiff{}
				lines = []string{}
				filediff.Headers = append(filediff.Headers, Chunk{Opts: Filehdr, Text: text})
			} else if text[0] == '@' {
				if len(lines) > 0 {
					filediff.Lines = append(filediff.Lines, diffLines(lines, merge)...)
				}
				lines = []string{}
				filediff.Lines = append(filediff.Lines, parseHunkHeader(text))
			} else {
				lines = append(lines, text)
			}
		}
	}

	if filediff != nil {
		if len(lines) > 0 {
			filediff.Lines = append(filediff.Lines, diffLines(lines, merge)...)
		}
		diff = append(diff, *filediff)
	}

	for i := range diff {
		diff[i].Num = i
	}

	// convert spaces to tabs
	for _, filediff := range diff {
		for _, linediff := range filediff.Lines {
			for i := range linediff.Chunks {
				text := linediff.Chunks[i].Text
				if strings.Index(text, "\t") >= 0 {
					out := make([]byte, 0, len(text))
					for j := range text {
						if text[j] != '\t' {
							out = append(out, text[j])
						} else {
							out = append(out, []byte("        ")...)
						}
					}
					linediff.Chunks[i].Text = string(out)
				}
			}
		}
	}

	return diff
}

func parseHunkHeader(text string) LineDiff {
	r := make([]Chunk, 0, 2)
	state := 0
loop:
	for i := range text {
		switch state {
		case 0:
			if text[i] != '@' {
				state++
			}
		case 1:
			if text[i] == '@' {
				state++
			}
		case 2:
			if text[i] != '@' {
				r = append(r, Chunk{Opts: Hunkhdr, Text: text[:i]})
				r = append(r, Chunk{Text: text[i:]})
				state++
				break loop
			}
		}
	}

	if state <= 2 {
		r = append(r, Chunk{Opts: Hunkhdr, Text: text})
	}

	return LineDiff{Chunks: r}
}

func diffLines(lines []string, merge bool) []LineDiff {
	if merge {
		return diffLinesCombined(lines)
	}
	return diffLinesWord(lines)
}

func diffLinesCombined(lines []string) []LineDiff {
	r := make([]LineDiff, len(lines))
	for i, line := range lines {
		opts := LineDiffOptsNone
		if line[0] == '+' || line[1] == '+' {
			opts = Addline
		} else if line[0] == '-' || line[1] == '-' {
			opts = Delline
		}

		r[i] = LineDiff{Opts: opts, Chunks: []Chunk{{Text: line}}}
	}
	return r
}

type wdseg struct {
	hasdel, hasadd bool
	del, add, norm string
}

func diffLinesWord(lines []string) []LineDiff {
	r := make([]LineDiff, 0, len(lines))

	addlines := []string{}
	dellines := []string{}

	flushBlock := func() {
		if len(dellines) == 0 || len(addlines) == 0 {
			for i := range dellines {
				r = append(r, LineDiff{Opts: Delline, Chunks: []Chunk{{Text: dellines[i]}}})
			}
			for i := range addlines {
				r = append(r, LineDiff{Opts: Addline, Chunks: []Chunk{{Text: addlines[i]}}})
			}
			addlines = []string{}
			dellines = []string{}
			return
		}

		if len(addlines) == len(dellines) {
			_, difflines := wordDiff(addlines, dellines, false)
			r = append(r, difflines...)
		} else {
			scoretop, difflinestop := wordDiffMismatched(addlines, dellines, false)
			scorebot, difflinesbot := wordDiffMismatched(reverse(addlines), reverse(dellines), true)
			difflinesbot = reverseLineDiff(difflinesbot)

			if scoretop <= 0 && scorebot <= 0 {
				r = lineDiffAppend(r, Delline, dellines, 0, len(dellines))
				r = lineDiffAppend(r, Addline, addlines, 0, len(addlines))
			} else if scoretop >= scorebot {
				r = append(r, difflinestop...)
			} else {
				if wordDiffDebug {
					fmt.Printf("Using bottom alignment on group\n")
				}
				r = append(r, difflinesbot...)
			}
		}

		addlines = []string{}
		dellines = []string{}
	}

	for _, line := range lines {
		switch line[0] {
		case ' ':
			flushBlock()
			r = append(r, LineDiff{Chunks: []Chunk{{Text: line}}})
		case '+':
			addlines = append(addlines, line)
		case '-':
			dellines = append(dellines, line)
		}
	}

	flushBlock()
	return r
}

func reverse(a []string) []string {
	r := make([]string, 0, len(a))
	for i := len(a) - 1; i >= 0; i-- {
		r = append(r, a[i])
	}
	return r
}

func reverseLineDiff(a []LineDiff) []LineDiff {
	r := make([]LineDiff, 0, len(a))
	for i := len(a) - 1; i >= 0; i-- {
		r = append(r, a[i])
	}
	return r
}

func lineDiff(opts LineDiffOpts, line string) LineDiff {
	return LineDiff{Opts: opts, Chunks: []Chunk{{Text: line}}}
}

func lineDiffAppend(r []LineDiff, opts LineDiffOpts, lines []string, start, end int) []LineDiff {
	for i := start; i < end; i++ {
		r = append(r, lineDiff(opts, lines[i]))
	}
	return r
}

// line wordDiff but addlines and dellines can have different lengths, they will be aligned at the top
func wordDiffMismatched(addlines, dellines []string, swap bool) (int, []LineDiff) {
	min := min(len(addlines), len(dellines))

	score, difflines := wordDiff(addlines[:min], dellines[:min], swap)

	if swap {
		difflines = lineDiffAppend(difflines, Addline, addlines, min, len(addlines))
		difflines = lineDiffAppend(difflines, Delline, dellines, min, len(dellines))
	} else {
		difflines = lineDiffAppend(difflines, Delline, dellines, min, len(dellines))
		difflines = lineDiffAppend(difflines, Addline, addlines, min, len(addlines))
	}

	return score, difflines
}

// addlines and dellines must have the same length
func wordDiff(addlines, dellines []string, swap bool) (int, []LineDiff) {
	r := []LineDiff{}
	for i := range addlines {
		delline := wordSplit(dellines[i][1:])
		addline := wordSplit(addlines[i][1:])
		score, difflines := textAlign(delline, addline)
		if score < int(0.50*float64(len(delline))) || score < int(0.50*float64(len(addline))) {
			if swap {
				r = lineDiffAppend(r, Addline, addlines, i, len(addlines))
				r = lineDiffAppend(r, Delline, dellines, i, len(dellines))
			} else {
				r = lineDiffAppend(r, Delline, dellines, i, len(dellines))
				r = lineDiffAppend(r, Addline, addlines, i, len(addlines))
			}
			return i - 1, r
		}

		if swap {
			r = append(r, reverseLineDiff(difflines)...)
		} else {
			r = append(r, difflines...)
		}
	}

	return len(addlines), r
}

func textAlign(a, b []string) (int, []LineDiff) {
	if len(a) == 0 {
		return 0, []LineDiff{lineDiff(Addline, "+"+strings.Join(b, ""))}
	}
	if len(b) == 0 {
		return 0, []LineDiff{lineDiff(Delline, "-"+strings.Join(a, ""))}
	}

	script := textAlignIntl(a, b)
	score := 0

	for i := range script {
		if script[i].kind == matchOp {
			score++
		}
	}

	lineAppend := func(line []Chunk, opts ChunkOpts, word string) []Chunk {
		if len(line) > 0 {
			if line[len(line)-1].Opts == opts {
				line[len(line)-1].Text += word
				return line
			}
		}
		return append(line, Chunk{Opts: opts, Text: word})
	}

	delline := []Chunk{{Text: "-"}}
	addline := []Chunk{{Text: "+"}}
	for i := range script {
		switch script[i].kind {
		case matchOp:
			delline = lineAppend(delline, ChunkNoOpts, a[script[i].src])
			addline = lineAppend(addline, ChunkNoOpts, a[script[i].src])
		case delOp:
			delline = lineAppend(delline, Delseg, a[script[i].src])
		case insOp:
			addline = lineAppend(addline, Addseg, b[script[i].dst])
		}
	}

	return score, []LineDiff{
		{Opts: Delline, Chunks: delline},
		{Opts: Addline, Chunks: addline},
	}
}

type opKind int

const (
	matchOp = opKind(iota)
	insOp
	delOp
)

type op struct {
	kind     opKind
	src, dst int
}

func fmtscript(script []op, a, b []string) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "<")
	for i := range script {
		switch script[i].kind {
		case matchOp:
			fmt.Fprintf(&buf, "(%c)", a[script[i].src])
		case insOp:
			fmt.Fprintf(&buf, "(ins:%c)", b[script[i].dst])
		case delOp:
			fmt.Fprintf(&buf, "(del:%c)", a[script[i].src])
		}
	}
	fmt.Fprintf(&buf, ">")
	return buf.String()
}

func textAlignIntl(a, b []string) []op {
	h := len(a) + 1
	w := len(b) + 1
	mx := make([][]int, h)

	for i := 0; i < h; i++ {
		mx[i] = make([]int, w)
		mx[i][0] = i
	}

	for j := 1; j < w; j++ {
		mx[0][j] = j
	}

	for i := 1; i < h; i++ {
		for j := 1; j < w; j++ {
			delCost := mx[i-1][j] + 1
			insCost := mx[i][j-1] + 1
			minCost := min(delCost, insCost)
			if a[i-1] == b[j-1] {
				minCost = min(minCost, mx[i-1][j-1])
			}
			mx[i][j] = minCost
		}
	}

	return backtrace(mx, len(mx)-1, len(mx[0])-1)
}

func min(a int, b int) int {
	if b < a {
		return b
	}
	return a
}

func backtrace(mx [][]int, i, j int) []op {
	var eo op
	if i > 0 && mx[i-1][j]+1 == mx[i][j] {
		eo.kind = delOp
		eo.src = i - 1
		eo.dst = j
		return append(backtrace(mx, i-1, j), eo)
	}
	if j > 0 && mx[i][j-1]+1 == mx[i][j] {
		eo.kind = insOp
		eo.src = i
		eo.dst = j - 1
		return append(backtrace(mx, i, j-1), eo)
	}
	if i > 0 && j > 0 && mx[i-1][j-1] == mx[i][j] {
		eo.kind = matchOp
		eo.src = i - 1
		eo.dst = j - 1
		return append(backtrace(mx, i-1, j-1), eo)
	}
	return []op{}
}

type wordSplitKind int

const (
	spaceWsk = wordSplitKind(iota)
	textWsk  = wordSplitKind(iota)
	symbWsk  = wordSplitKind(iota)
)

/*split symbols on their own
func wordSplit(in string) []string {
	r := []string{}
	start := 0
	kind := spaceWsk
	for i, ch := range in {
		var curkind wordSplitKind
		switch {
		case unicode.IsSpace(ch):
			curkind = spaceWsk
		case unicode.IsLetter(ch) || unicode.IsNumber(ch):
			curkind = textWsk
		default:
			curkind = symbWsk
		}

		if curkind != kind || kind == symbWsk {
			if start != i {
				r = append(r, in[start:i])
				start = i
			}
			kind = curkind
		}
	}

	r = append(r, in[start:len(in)])
	return r
}
*/

func wordSplit(in string) []string {
	r := []string{}
	start := 0
	kind := spaceWsk
	for i, ch := range in {
		var curkind wordSplitKind
		switch {
		case unicode.IsSpace(ch):
			curkind = spaceWsk
		default:
			curkind = textWsk
		}

		if curkind != kind {
			if start != i {
				r = append(r, in[start:i])
				start = i
			}
			kind = curkind
		}
	}

	r = append(r, in[start:len(in)])
	return r
}

func showDiff(mw *nucular.MasterWindow, w *nucular.Window, diff Diff, width *int) {
	style, scaling := mw.Style()

	rounding := uint16(4 * scaling)

	lnh := nucular.FontHeight(style.Font)
	d := font.Drawer{Face: style.Font}

	for _, filediff := range diff {
		if w.TreePush(nucular.TreeTab, filediff.Filename, len(diff) == 1) {
			originalSpacing := style.NormalWindow.Spacing.Y
			style.NormalWindow.Spacing.Y = 0

			if *width > 0 {
				w.RowScaled(lnh).StaticScaled(*width)
			} else {
				w.RowScaled(lnh).Dynamic(1)
			}

			for _, hdr := range filediff.Headers[1:] {
				w.LabelColored(hdr.Text, "LC", hunkhdrColor)
			}

			w.Spacing(1)

			for _, linediff := range filediff.Lines {
				bounds, out := w.Custom(nstyle.WidgetStateInactive)
				if out == nil {
					continue
				}

				switch linediff.Opts {
				case Addline:
					out.FillRect(bounds, 0, addlineBg)
				case Delline:
					out.FillRect(bounds, 0, dellineBg)
				}

				dot := bounds
				for _, chunk := range linediff.Chunks {
					dot.W = d.MeasureString(chunk.Text).Ceil()

					if dot.W > *width {
						*width = dot.W
					}

					switch chunk.Opts {
					case Addseg:
						out.FillRect(dot, rounding, addsegBg)
					case Delseg:
						out.FillRect(dot, rounding, delsegBg)
					}

					out.DrawText(dot, chunk.Text, style.Font, style.Text.Color)
					dot.X += dot.W
				}
			}

			style.NormalWindow.Spacing.Y = originalSpacing

			w.TreePop()
		}
	}

	for _, e := range w.KeyboardOnHover(w.Bounds).Keys {
		switch {
		case (e.Modifiers == 0) && (e.Code == key.CodeHome):
			w.Scrollbar.X = 0
			w.Scrollbar.Y = 0
		case (e.Modifiers == 0) && (e.Code == key.CodeEnd):
			w.Scrollbar.X = 0
			w.Scrollbar.Y = w.At().Y
		case (e.Modifiers == 0) && (e.Code == key.CodeUpArrow):
			w.Scrollbar.Y -= lnh
		case (e.Modifiers == 0) && (e.Code == key.CodeDownArrow):
			w.Scrollbar.Y += lnh
		case (e.Modifiers == 0) && (e.Code == key.CodeLeftArrow):
			w.Scrollbar.X -= w.Bounds.W / 10
		case (e.Modifiers == 0) && (e.Code == key.CodeRightArrow):
			w.Scrollbar.X += w.Bounds.W / 10
		case (e.Modifiers == 0) && (e.Code == key.CodePageUp):
			w.Scrollbar.Y -= w.Bounds.H / 2
		case (e.Modifiers == 0) && (e.Code == key.CodePageDown):
			w.Scrollbar.Y += w.Bounds.H / 2
		}

		if w.Scrollbar.Y < 0 {
			w.Scrollbar.Y = 0
		}
		if w.Scrollbar.X < 0 {
			w.Scrollbar.X = 0
		}
	}
}
