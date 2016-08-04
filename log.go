package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"io/ioutil"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/label"
	ntypes "github.com/aarzilli/nucular/types"

	"golang.org/x/mobile/event/key"
)

const LookaheadSize = 128
const MaxLanes = 10

type CommitFetcher struct {
	Out <-chan Commit
	Err error
	cmd *exec.Cmd
}

func (fetcher *CommitFetcher) Stop() {
	fetcher.cmd.Process.Kill()
}

type zeroDelimitedReader struct {
	In    io.Reader
	ateof bool
	buf   []byte
}

func (zdr *zeroDelimitedReader) reset() {
	zdr.ateof = false
}

func (zdr *zeroDelimitedReader) Read(p []byte) (n int, err error) {
	if zdr.ateof {
		return 0, io.EOF
	}
	if len(zdr.buf) > 0 {
		var i int
		for i = 0; i < len(zdr.buf); i++ {
			if i >= len(p) {
				break
			}
			if zdr.buf[i] == 0x00 {
				err = io.EOF
				zdr.ateof = true
				i++
				break
			}
			p[i] = zdr.buf[i]
			n++
		}

		if i < len(zdr.buf) {
			rem := zdr.buf[i:]
			copy(zdr.buf, rem)
			zdr.buf = zdr.buf[:len(rem)]
		} else {
			zdr.buf = nil
		}

		return
	}

	n, err = zdr.In.Read(p)
	read := p[:n]
	for i := range read {
		if read[i] == 0x00 {
			if i+1 < len(read) {
				zdr.buf = make([]byte, len(read[i+1:]))
				copy(zdr.buf, read[i+1:])
			}
			zdr.ateof = true
			n = i
			err = io.EOF
			return
		}
	}
	return
}

func parseAuthor(in string) (string, time.Time) {
	i := 0
	for i = 0; i < len(in); i++ {
		if in[i] == '<' {
			break
		}
	}
	for ; i < len(in); i++ {
		if in[i] == '>' {
			break
		}
	}
	i++
	endauthor := i
	for ; i < len(in); i++ {
		if in[i] != ' ' {
			break
		}
	}
	startdate := i
	for ; i < len(in); i++ {
		if in[i] == ' ' {
			break
		}
	}
	enddate := i
	starttz := i + 1

	ts, _ := strconv.Atoi(in[startdate:enddate])

	zonename := in[starttz:]
	offhrs, _ := strconv.Atoi(zonename[:3])
	offmins, _ := strconv.Atoi(zonename[3:])
	tzoffset := offhrs*60*60 + offmins*60
	loc := time.FixedZone(zonename, tzoffset)

	tutc := time.Unix(int64(ts+tzoffset), 0)

	return in[:endauthor], time.Date(tutc.Year(), tutc.Month(), tutc.Day(), tutc.Hour(), tutc.Minute(), tutc.Second(), 0, loc)
}

func (fetcher *CommitFetcher) readCommit(in io.Reader) (commit Commit, ok bool) {
	const (
		commitHeader    = "commit "
		parentHeader    = "parent "
		authorHeader    = "author "
		committerHeader = "committer "
	)

	scanner := bufio.NewScanner(in)

	// Commit Header
headerLoop:
	for scanner.Scan() {
		ln := scanner.Text()
		switch {
		case len(ln) <= 0:
			// end of header
			break headerLoop
		case ln[0] == ' ':
			// part of a multiline commit field, since we don't support those at all we just ignore it
			break
		case strings.HasPrefix(ln, commitHeader):
			commit.Id = ln[len(commitHeader):]
		case strings.HasPrefix(ln, parentHeader):
			commit.Parent = append(commit.Parent, ln[len(parentHeader):])
		case strings.HasPrefix(ln, authorHeader):
			commit.Author, commit.AuthorDate = parseAuthor(ln[len(authorHeader):])
		case strings.HasPrefix(ln, committerHeader):
			commit.Committer, commit.CommitterDate = parseAuthor(ln[len(committerHeader):])
		}
	}

	if commit.Id == "" {
		ok = false
		return
	}

	var bs bytes.Buffer

	for scanner.Scan() {
		fmt.Fprintln(&bs, strings.TrimSpace(scanner.Text()))
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("[exiting for error %v]\n", err)
		fetcher.Err = err
	} else {
		commit.Message = bs.String()
		ok = true
	}

	return
}

func allCommits(repodir string) *CommitFetcher {
	fetcher := &CommitFetcher{}
	outchan := make(chan Commit)
	fetcher.Out = outchan
	fetcher.cmd = exec.Command("git", "log", "--pretty=raw", "-z", "--all", "--no-color", "--date-order")
	fetcher.cmd.Dir = repodir
	stdout, err := fetcher.cmd.StdoutPipe()
	if err != nil {
		fetcher.Err = err
		close(outchan)
		return fetcher
	}
	stderr, err := fetcher.cmd.StderrPipe()
	if err != nil {
		fetcher.Err = err
		close(outchan)
		return fetcher
	}
	err = fetcher.cmd.Start()
	if err != nil {
		fetcher.Err = err
		close(outchan)
		return fetcher
	}

	errdone := make(chan struct{})
	var errout string

	// stderr reader
	go func() {
		defer close(errdone)
		bs, err := ioutil.ReadAll(stderr)
		if err != nil {
			return
		}
		errout = string(bs)
	}()

	go func() {
		defer close(outchan)
		zdr := zeroDelimitedReader{In: stdout}
		for {
			zdr.reset()
			commit, ok := fetcher.readCommit(&zdr)
			if !ok {
				break
			}
			outchan <- commit
		}
		err := fetcher.cmd.Wait()
		<-errdone
		if err != nil {
			fetcher.Err = fmt.Errorf("Error %v: %s\n", err, errout)
		}
	}()

	return fetcher
}

type commitLookaheadBuffer struct {
	commitchan <-chan Commit
	buffer     [LookaheadSize]Commit
	head       int
	sz         int
	done       bool
}

func (clb *commitLookaheadBuffer) init(commitchan <-chan Commit) {
	clb.commitchan = commitchan

	for i := 0; i < LookaheadSize; i++ {
		var ok bool
		clb.buffer[i], ok = <-clb.commitchan
		if !ok {
			clb.done = true
			break
		}
		clb.sz++
	}
}

func (clb *commitLookaheadBuffer) Get() (commit Commit, ok bool) {
	if clb.sz <= 0 {
		return
	}
	commit = clb.buffer[clb.head]
	ok = true
	clb.sz--

	if !clb.done {
		var subok bool
		clb.buffer[clb.head], subok = <-clb.commitchan
		if !subok {
			clb.done = true
		} else {
			clb.sz++
		}
	}

	clb.head = (clb.head + 1) % LookaheadSize
	return
}

func (clb *commitLookaheadBuffer) Lookup(id string) *Commit {
	i := clb.head

	for cnt := clb.sz; cnt > 0; cnt-- {
		if clb.buffer[i].Id == id {
			return &clb.buffer[i]
		}

		i = (i + 1) % LookaheadSize
	}

	return nil
}

var emergencycount int = 0

func laneCommits(headcommit string, refs []Ref, commitchan <-chan Commit, out chan<- LanedCommit) {
	defer close(out)

	var clb commitLookaheadBuffer
	clb.init(commitchan)

	var lanes [MaxLanes]string
	const emergencylane = MaxLanes - 1

	refmap := map[string][]Ref{}
	for _, ref := range refs {
		v, ok := refmap[ref.CommitId]
		if !ok {
			refmap[ref.CommitId] = []Ref{ref}
		} else {
			refmap[ref.CommitId] = append(v, ref)
		}
	}

	findLaneForCommit := func(commit string) int {
		for i := range lanes {
			if lanes[i] == commit {
				return i
			}
		}
		return -1
	}

	for {
		commit, ok := clb.Get()
		if !ok {
			return
		}

		var lc LanedCommit
		lc.Commit = commit

		if len(refmap) > 0 {
			if refs, ok := refmap[lc.Id]; ok {
				lc.Refs = refs
				delete(refmap, lc.Id)
			}
		}

		for _, ref := range lc.Refs {
			if ref.IsHEAD {
				lc.IsHEAD = true
				break
			}
		}

		if !lc.IsHEAD {
			if lc.Id == headcommit {
				lc.IsHEAD = true
			}
		}

		// look for a lane reserved for this commit
		lc.Lane = findLaneForCommit(lc.Id)

		// if there isn't any reserved lane find an empty lane
		// looking for an empty lane always succeeds because we keep the
		// emergency lane unreserved
		if lc.Lane < 0 {
			lc.Lane = findLaneForCommit("")
		} else {
			lanes[lc.Lane] = ""
		}

		if lc.Lane == emergencylane {
			emergencycount++
		}

		lc.ParentLane = make([]int, len(lc.Parent))

		// place parents in their allocated lanes
		for i := range lc.Parent {
			lc.ParentLane[i] = findLaneForCommit(lc.Parent[i])
		}

		parentCommits := make([]*Commit, len(lc.Parent))

		// if we aren't on emergencylane allocate this commit's
		// lane to the closes of the unallocated parents
		if lc.Lane != emergencylane {
			closeparent := ""
			closeparentidx := -1
			var parentdst time.Duration = (1 << 60)
			for i := range lc.Parent {
				if lc.ParentLane[i] >= 0 {
					continue
				}

				parentCommits[i] = clb.Lookup(lc.Parent[i])

				var d time.Duration = (1 << 60) - 1
				if parentCommits[i] != nil {
					d = lc.CommitterDate.Sub(parentCommits[i].CommitterDate)
				}
				if d < parentdst {
					closeparentidx = i
					closeparent = lc.Parent[i]
					parentdst = d
				}
			}

			lanes[lc.Lane] = closeparent
			if closeparentidx >= 0 {
				lc.ParentLane[closeparentidx] = lc.Lane
			}
		}

		// allocate lanes for parents that aren't allocated already
		for i := range lc.Parent {
			if lc.ParentLane[i] < 0 && parentCommits[i] != nil {
				lc.ParentLane[i] = findLaneForCommit("")
				if lc.ParentLane[i] == emergencylane {
					lc.ParentLane[i] = -1
				}
				if lc.ParentLane[i] >= 0 {
					lanes[lc.ParentLane[i]] = lc.Parent[i]
				}
			}
		}

		for i := range lanes {
			lc.LanesAfter[i] = lanes[i] != ""
		}

		// 		lc.Print()
		// 		for i := range lanes {
		// 			fmt.Printf("[%s] ", abbrev(lanes[i]))
		// 		}
		// 		fmt.Printf("\n")
		lc.ShiftLeftFrom = -1

		out <- lc
	}
}

type LogWindow struct {
	mu sync.Mutex

	commits     []LanedCommit
	maxOccupied int

	Headisref bool
	Head      *Ref
	status    *GitStatus

	needsMore int
	done      bool
	started   bool

	selectedId string

	repodir string
	allrefs []Ref
	mw      *nucular.MasterWindow

	showOutput         bool
	edCommit, edOutput nucular.TextEditor
}

func (lw *LogWindow) commitproc() {
	defer func() {
		lw.done = true
		lw.mw.Changed()
	}()

	var err error
	lw.allrefs, err = allRefs(lw.repodir)
	if err != nil {
		newMessagePopup(lw.mw, "Error", fmt.Sprintf("Error fetching references: %v\n", err))
		return
	}

	fetcher := allCommits(lw.repodir)
	commitchan := make(chan LanedCommit)
	var headcommit string
	lw.Headisref, headcommit, _ = getHead(lw.repodir)
	if lw.Headisref {
		lw.Head = &Ref{}
		lw.Head.Init(headcommit, "")
	}
	if lw.Headisref {
		headcommit = ""
	}
	go laneCommits(headcommit, lw.allrefs, fetcher.Out, commitchan)

	lw.maxOccupied = 1

	for commit := range commitchan {
		lw.mu.Lock()
		lw.commits = append(lw.commits, commit)
		if lw.needsMore >= 0 && lw.needsMore < len(lw.commits) {
			lw.needsMore = -1
			lw.mw.Changed()
		}
		if occupied := commit.Occupied(); occupied > lw.maxOccupied {
			lw.maxOccupied = occupied
		}
		lw.mu.Unlock()
	}

	lw.mu.Lock()
	if lw.needsMore >= 0 {
		lw.needsMore = -1
		lw.mw.Changed()
	}
	lw.mu.Unlock()

	if fetcher.Err != nil {
		newMessagePopup(lw.mw, "Error", fmt.Sprintf("Error fetching commits: %v\n", fetcher.Err))
	}
}

func nameInitials(s string) string {
	inspace := true
	out := make([]rune, 0, 3)
	for _, ch := range s {
		if ch == '<' {
			break
		}
		if inspace {
			if ch != ' ' {
				out = append(out, ch)
				inspace = false
			}
		} else {
			if ch == ' ' {
				inspace = true
			}
		}
	}
	return string(out)
}

const graphLineHeight = 20
const graphThick = 2

func shrinkRect(r ntypes.Rect, amount int) (res ntypes.Rect) {
	res.X = r.X + amount
	res.Y = r.Y + amount
	res.W = r.W - 2*amount
	res.H = r.H - 2*amount
	return res
}

func laneboundsOf(lnh int, bounds ntypes.Rect, lane int) (r ntypes.Rect) {
	r.X = bounds.X + lnh*lane
	r.W = lnh
	r.Y = bounds.Y
	r.H = lnh
	return
}

var graphColor = color.RGBA{213, 204, 255, 0xff}
var refsColor = color.RGBA{255, 182, 97, 0xff}
var refsHeadColor = color.RGBA{233, 255, 97, 0xff}

func (lw *LogWindow) UpdateGraph(mw *nucular.MasterWindow, w *nucular.Window) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	w.MenubarBegin()
	w.LayoutRowDynamic(25, 1)
	if lw.status == nil {
		lw.status = gitStatus(lw.repodir)
	}
	w.Label(lw.status.Summary(), "LC")
	w.MenubarEnd()

	style, scaling := mw.Style()

	oldspacing := style.GroupWindow.Spacing.Y
	style.GroupWindow.Spacing.Y = 0
	defer func() {
		style.GroupWindow.Spacing.Y = oldspacing
	}()

	lnh := int(graphLineHeight * scaling)
	thick := int(graphThick * scaling)

	for _, e := range w.KeyboardOnHover(w.Bounds).Keys {
		switch {
		case (e.Modifiers == 0) && (e.Code == key.CodeHome):
			w.Scrollbar.Y = 0
		case (e.Modifiers == 0) && (e.Code == key.CodeEnd):
			w.Scrollbar.Y = (lnh * len(lw.commits)) - w.LayoutAvailableHeight()
		case (e.Modifiers == 0) && (e.Code == key.CodeUpArrow):
			w.Scrollbar.Y -= lnh
		case (e.Modifiers == 0) && (e.Code == key.CodeDownArrow):
			w.Scrollbar.Y += lnh
		case (e.Modifiers == 0) && (e.Code == key.CodePageUp):
			w.Scrollbar.Y -= w.LayoutAvailableHeight() / 2
		case (e.Modifiers == 0) && (e.Code == key.CodePageDown):
			w.Scrollbar.Y += w.LayoutAvailableHeight() / 2
		}
		if w.Scrollbar.Y < 0 {
			w.Scrollbar.Y = 0
		}
	}

	// number of commits needed before we can show up anything
	n := int(math.Ceil(float64(w.LayoutAvailableHeight()+w.Scrollbar.Y) / float64(lnh)))

	updating := false

	if !lw.started {
		lw.started = true
		go lw.commitproc()
		updating = true
	} else if !lw.done {
		if n >= len(lw.commits) {
			lw.needsMore = n
			updating = true
		}
	}

	if updating {
		w.LayoutRowDynamic(graphLineHeight, 1)
		w.Label("Loading...", "LC")
		return
	}

	datesz := nucular.FontWidth(style.Font, "0000-00-00 00:000") + style.Text.Padding.X*2
	authorsz := nucular.FontWidth(style.Font, "MMMM") + style.Text.Padding.X*2

	availableWidth := w.LayoutAvailableWidth()

	spacing := style.GroupWindow.Spacing

	calcCommitsz := func(graphsz int, includeAuthor, includeDate bool) int {
		w := availableWidth - graphsz - spacing.X
		if includeAuthor {
			w -= (authorsz + spacing.X)
		}
		if includeDate {
			w -= (datesz + spacing.X)
		}

		return w
	}

	maxOccupied := 8
	if lw.done {
		maxOccupied = lw.maxOccupied
	}

	includeAuthor, includeDate := true, true
	commitsz := calcCommitsz(lnh*maxOccupied, includeAuthor, includeDate)
	if commitsz < authorsz+datesz {
		includeAuthor = false
		commitsz = calcCommitsz(lnh*maxOccupied, includeAuthor, includeDate)
		if commitsz < datesz {
			includeDate = false
			commitsz = calcCommitsz(lnh*maxOccupied, includeAuthor, includeDate)
		}
	}

	szs := make([]int, 0, 5)

	var prevLanes [MaxLanes]bool

	skip := w.Scrollbar.Y/(lnh+style.GroupWindow.Spacing.Y) - 2

	if skip < 0 {
		skip = 0
	}

	if maxskip := len(lw.commits) - 3; skip > maxskip {
		skip = maxskip
	}

	emptyCommitRows := func(n int) {
		if n > 0 {
			w.LayoutRowDynamic(n*graphLineHeight+(n-1)*style.GroupWindow.Spacing.Y, 1)
			// this will never be seen, I could use a Spacing but I want to be bugs noticeable
			w.Label("More...", "LC")
		}
	}

	emptyCommitRows(skip)

	for i, lc := range lw.commits[skip:] {
		if _, below := w.Invisible(); below {
			// fill the space that would be occupied by commits below the fold
			// with a big row
			emptyCommitRows(len(lw.commits[skip:]) - i)
			break
		}

		szs = szs[:0]

		szs = append(szs, lnh*lc.Occupied())
		commitsz := calcCommitsz(szs[0], includeAuthor, includeDate)

		refstr := ""
		ishead := false

		if len(lc.Refs) != 0 || lc.IsHEAD {
			var buf bytes.Buffer
			for i := range lc.Refs {
				io.WriteString(&buf, lc.Refs[i].Nice())
				if i != len(lc.Refs)-1 {
					io.WriteString(&buf, ", ")
				}
				ishead = ishead || lc.Refs[i].IsHEAD
			}
			refstr = buf.String()

			if len(lc.Refs) == 0 && lc.IsHEAD {
				refstr = "HEAD"
			}

			refsz := nucular.FontWidth(style.Font, refstr) + 2*style.Text.Padding.X
			origcommitsz := commitsz
			commitsz -= refsz + style.GroupWindow.Spacing.X

			if commitsz <= 0 {
				refsz = origcommitsz
				commitsz = 0
				szs = append(szs, refsz)
			} else {
				szs = append(szs, refsz)
				szs = append(szs, commitsz)
			}
		} else {
			szs = append(szs, commitsz)
		}

		if includeAuthor {
			szs = append(szs, authorsz)
		}

		if includeDate {
			szs = append(szs, datesz)
		}

		datestr := lc.CommitterDate.Local().Format("2006-01-02 15:04")
		authorstr := nameInitials(lc.Author)

		w.LayoutRowFixedScaled(lnh, szs...)

		rowwidth := w.LayoutAvailableWidth()

		bounds, out := w.Custom(ntypes.WidgetStateInactive)
		rowbounds := bounds
		rowbounds.W = rowwidth

		if len(lc.Refs) != 0 {
			if ishead {
				w.LabelColored(refstr, "LC", refsHeadColor)
			} else {
				w.LabelColored(refstr, "LC", refsColor)
			}
		}

		selected := lc.Id == lw.selectedId

		if commitsz > 0 {
			w.SelectableLabel(lc.ShortMessage(), "LC", &selected)
		}

		if includeAuthor {
			w.SelectableLabel(authorstr, "CC", &selected)
		}
		if includeDate {
			w.SelectableLabel(datestr, "RC", &selected)
		}

		if selected && lc.Id != lw.selectedId {
			lw.selectedId = lc.Id
			lw.showOutput = false
			var buf bytes.Buffer

			fmt.Fprintf(&buf, "commit %s ", abbrev(lc.Id))
			if len(lc.Parent) > 0 {
				if len(lc.Parent) == 1 {
					fmt.Fprintf(&buf, "parent ")
				} else {
					fmt.Fprintf(&buf, "parents ")
				}
				// TODO: make these clickable
				for i := range lc.Parent {
					if i == 0 {
						fmt.Fprintf(&buf, "%s", abbrev(lc.Parent[i]))
					} else {
						fmt.Fprintf(&buf, ", %s", abbrev(lc.Parent[i]))
					}
				}
				buf.Write([]byte{'\n'})
			}

			ad := lc.AuthorDate.Local().Format("2006-01-02 15:04")
			cd := lc.CommitterDate.Local().Format("2006-01-02 15:04")

			if lc.Author != lc.Committer {
				fmt.Fprintf(&buf, "author %s on %s\n", lc.Author, ad)
				fmt.Fprintf(&buf, "committer %s on %s\n", lc.Committer, cd)
			} else {
				if ad != cd {
					fmt.Fprintf(&buf, "author %s on %s (committed %s)\n", lc.Author, ad, cd)
				} else {
					fmt.Fprintf(&buf, "author %s on %s\n", lc.Author, ad)
				}
			}
			buf.WriteByte('\n')
			buf.Write([]byte(lc.Message))

			lw.edCommit.Buffer = []rune(buf.String())
			lw.edCommit.Cursor = 0
		}

		if out == nil {
			copy(prevLanes[:], lc.LanesAfter[:])
			continue
		}

		cm := commitMenu{lw, lc}
		w.ContextualOpen(0, image.Point{250, 300}, rowbounds, cm.Update)

		// draws graph proper

		lanebounds := laneboundsOf(lnh, bounds, lc.Lane)
		circle := shrinkRect(lanebounds, lnh/4)

		out.FillCircle(circle, graphColor)

		center := lanebounds.Min()
		center.X += lnh / 2
		center.Y += lnh / 2

		bottomleft := lanebounds.Min()
		bottomleft.Y += lanebounds.H

		bottomright := lanebounds.Max()

		minparentlane, maxparentlane := -1, -1

		for i := range lc.ParentLane {
			if lc.ParentLane[i] < 0 {
				continue
			}
			if minparentlane < 0 {
				minparentlane = lc.ParentLane[i]
			}
			if lc.ParentLane[i] < minparentlane {
				minparentlane = lc.ParentLane[i]
			}
			if lc.ParentLane[i] > maxparentlane {
				maxparentlane = lc.ParentLane[i]
			}
		}

		if minparentlane >= lc.Lane {
			minparentlane = -1
		}

		if maxparentlane <= lc.Lane {
			maxparentlane = -1
		}

		if minparentlane >= 0 {
			out.StrokeLine(center, bottomleft, thick, graphColor)
		}

		if maxparentlane >= 0 {
			out.StrokeLine(center, bottomright, thick, graphColor)
		}

		nextbounds := bounds
		nextbounds.Y += nextbounds.H + int(float64(style.GroupWindow.Spacing.Y)*scaling)

		for _, dst := range lc.ParentLane {
			if dst < 0 {
				continue
			}

			dstbounds := laneboundsOf(lnh, nextbounds, dst)
			dstcenter := dstbounds.Min()
			dstcenter.X += lnh / 2
			dstcenter.Y += lnh / 2

			dstboundsInline := laneboundsOf(lnh, bounds, dst)

			switch {
			case dst < lc.Lane:
				topright := dstboundsInline.Min()
				topright.Y += lnh
				topright.X += lnh

				out.StrokeLine(topright, dstcenter, thick, graphColor)

				if dst == minparentlane && dst != lc.Lane-1 {
					out.StrokeLine(bottomleft, topright, thick, graphColor)
				}

			case dst > lc.Lane:
				topleft := dstboundsInline.Min()
				topleft.Y += lnh

				out.StrokeLine(topleft, dstcenter, thick, graphColor)

				if dst == maxparentlane && dst != lc.Lane+1 {
					out.StrokeLine(bottomright, topleft, thick, graphColor)
				}

			case dst == lc.Lane:
				out.StrokeLine(center, dstcenter, thick, graphColor)
			}
		}

		for i := range lc.LanesAfter {
			if lc.LanesAfter[i] && prevLanes[i] {
				lanebounds := laneboundsOf(lnh, bounds, i)
				dstlanebounds := laneboundsOf(lnh, nextbounds, i)
				center := lanebounds.Min()
				center.X += lnh / 2
				center.Y += lnh / 2

				dstcenter := dstlanebounds.Min()
				dstcenter.X += lnh / 2
				dstcenter.Y += lnh / 2

				out.StrokeLine(center, dstcenter, thick, graphColor)
			}
		}

		copy(prevLanes[:], lc.LanesAfter[:])
	}
}

type commitMenu struct {
	lw *LogWindow
	lc LanedCommit
}

func (cm *commitMenu) Update(mw *nucular.MasterWindow, w *nucular.Window) {
	lw, lc := cm.lw, cm.lc
	lw.selectedId = lc.Id
	localRefs := []Ref{}
	remoteRefs := []Ref{}
	for _, ref := range lc.Refs {
		switch ref.Kind {
		case LocalRef:
			localRefs = append(localRefs, ref)
		case RemoteRef:
			remoteRefs = append(remoteRefs, ref)
		}
	}

	remotesMap := map[string]struct{}{}
	for _, ref := range lw.allrefs {
		if ref.Kind == RemoteRef {
			remotesMap[ref.Remote()] = struct{}{}
		}
	}

	remotes := make([]string, 0, len(remotesMap))
	for remote := range remotesMap {
		remotes = append(remotes, remote)
	}

	w.LayoutRowDynamic(25, 1)
	if _, bookmarked := bookmarks[lc.Id]; !bookmarked {
		if w.MenuItem(label.TA("Bookmark", "LC")) {
			bookmarks[lc.Id] = lc
		}
	}

	if w.MenuItem(label.TA("View", "LC")) {
		viewAction(lw, lc)
	}

	if !lc.IsHEAD {
		if w.MenuItem(label.TA("Checkout", "LC")) {
			switch len(localRefs) {
			case 0:
				checkoutAction(lw, nil, lc.Id)
			case 1:
				checkoutAction(lw, &localRefs[0], "")
			default:
				newCheckoutPopup(lw.mw, localRefs)
			}
		}
	}

	if w.MenuItem(label.TA("New branch", "LC")) {
		newNewBranchPopup(lw.mw, lc.Id)
	}

	if !lc.IsHEAD && lw.Headisref {
		if w.MenuItem(label.TA(fmt.Sprintf("Reset %s here", lw.Head.Nice()), "LC")) {
			newResetPopup(lw.mw, lc.Id, resetHard)
		}
	}

	if !lc.IsHEAD {
		if w.MenuItem(label.TA("Cherrypick", "LC")) {
			cherrypickAction(lw, lc.Id)
		}
	}

	if len(remoteRefs) > 0 {
		if w.MenuItem(label.TA("Fetch", "LC")) {
			if len(remotes) == 1 {
				remoteAction(lw, "fetch", remotes[0])
			} else {
				newRemotesPopup(lw.mw, "fetch", remotes)
			}
		}
	}

	if lc.IsHEAD && lw.Headisref && len(remoteRefs) > 0 {
		if w.MenuItem(label.TA("Pull", "LC")) {
			if len(remotes) == 1 {
				remoteAction(lw, "pull", remotes[0])
			} else {
				newRemotesPopup(lw.mw, "pull", remotes)
			}
		}
	}

	if lc.IsHEAD && lw.Headisref && len(remotes) > 0 {
		if w.MenuItem(label.TA("Push", "LC")) {
			if len(remotes) == 1 {
				pushAction(lw, false, remotes[0])
			} else {
				newRemotesPopup(lw.mw, "push", remotes)
			}
		}
	}

	if lc.IsHEAD && lw.Headisref {
		if w.MenuItem(label.TA("Merge", "LC")) {
			newMergePopup(lw.mw, lw.allrefs)
		}
	}

	if lw.Headisref {
		if w.MenuItem(label.TA(fmt.Sprintf("Rebase %s here", lw.Head.Nice()), "LC")) {
			if len(localRefs) > 0 {
				rebaseAction(lw, localRefs[0].Nice())
			} else {
				rebaseAction(lw, lc.Id)
			}
		}
	}

	if w.MenuItem(label.TA("Diff", "LC")) {
		newDiffPopup(lw.mw, lw.allrefs, bookmarksAsSlice(), lc)
	}
}

func (lw *LogWindow) UpdateExtra(mw *nucular.MasterWindow, sw *nucular.Window) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	style, _ := mw.Style()

	sw.LayoutAvailableHeight()
	sw.LayoutRowStatic(25, 120, 2)
	showDetails := !lw.showOutput
	sw.SelectableLabel("Commit Details", "LC", &showDetails)
	lw.showOutput = !showDetails
	sw.SelectableLabel("Output", "LC", &lw.showOutput)

	sw.LayoutRowDynamicScaled(sw.LayoutAvailableHeight()-style.NormalWindow.Spacing.Y, 1)
	if lw.showOutput {
		lw.edOutput.Edit(sw)
	} else {
		lw.edCommit.Edit(sw)
	}
}

func (lw *LogWindow) Title() string {
	return "Graph"
}

func (lw *LogWindow) Update(mw *nucular.MasterWindow, w *nucular.Window) {
	style, _ := mw.Style()

	h := w.LayoutAvailableHeight() - style.NormalWindow.Spacing.Y

	w.LayoutRowDynamicScaled(int(float64(h)*0.7), 1)
	if sw := w.GroupBegin("log-group-top", nucular.WindowBorder|nucular.WindowNoHScrollbar); sw != nil {
		lw.UpdateGraph(mw, sw)
		sw.GroupEnd()
	}
	w.LayoutRowDynamicScaled(int(float64(h)*0.3), 1)
	if sw := w.GroupBegin("log-group-bot", nucular.WindowBorder|nucular.WindowNoScrollbar); sw != nil {
		lw.UpdateExtra(mw, sw)
		sw.GroupEnd()
	}
}

func (lw *LogWindow) reload() {
	lw.commits = lw.commits[:0]
	lw.maxOccupied = 0

	lw.selectedId = ""

	lw.Headisref = false
	lw.Head = nil

	lw.needsMore = -1
	lw.done = false
	lw.started = false
	lw.status = nil
}
