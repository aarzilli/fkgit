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
	"github.com/aarzilli/nucular/rect"
	nstyle "github.com/aarzilli/nucular/style"

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

		lc.ShiftLeftFrom = -1

		out <- lc
	}
}

type LogWindow struct {
	mu sync.Mutex

	split nucular.ScalableSplit

	commits     []LanedCommit
	maxOccupied int

	Headisref bool
	Head      *Ref
	status    *GitStatus

	needsMore int
	done      bool
	started   bool

	selectedId   string
	selectedView *ViewWindow

	repodir string
	allrefs []Ref
	mw      *nucular.MasterWindow

	searchCmd     *exec.Cmd
	searchMode    searchMode
	searchDone    bool
	searchEd      nucular.TextEditor
	searchIdx     int
	searchResults []string

	showOutput bool
	edOutput   nucular.TextEditor
}

type searchMode int

const (
	noSearch searchMode = iota
	pathSearchSetup
	grepSearchSetup
	searchRunning
	searchRestartMove
	searchMove
	searchAbort
)

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

func shrinkRect(r rect.Rect, amount int) (res rect.Rect) {
	res.X = r.X + amount
	res.Y = r.Y + amount
	res.W = r.W - 2*amount
	res.H = r.H - 2*amount
	return res
}

func laneboundsOf(lnh int, bounds rect.Rect, lane int) (r rect.Rect) {
	r.X = bounds.X + lnh*lane
	r.W = lnh
	r.Y = bounds.Y
	r.H = lnh
	return
}

func (lw *LogWindow) selectCommit(lc *LanedCommit) {
	if lc == nil {
		lw.selectedId = ""
		return
	}
	lw.selectedId = lc.Id
	lw.showOutput = false
	lw.selectedView = NewViewWindow(lw.repodir, *lc, false)
}

var graphColor = color.RGBA{213, 204, 255, 0xff}
var refsColor = color.RGBA{255, 182, 97, 0xff}
var refsHeadColor = color.RGBA{233, 255, 97, 0xff}

func (lw *LogWindow) UpdateGraph(w *nucular.Window) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	moveToSelected := false

	w.MenubarBegin()
	switch lw.searchMode {
	case noSearch:
		w.Row(25).Static(0, 120)
	case pathSearchSetup, grepSearchSetup:
		w.Row(25).Static(0, 100, 300)
	case searchRunning, searchAbort:
		w.Row(25).Static(0, 200)
	case searchMove, searchRestartMove:
		if lw.searchDone && len(lw.searchResults) == 0 {
			w.Row(25).Static(0, 100, 100)
		} else {
			w.Row(25).Static(0, 100, 100, 100, 100)
		}
	}
	if lw.status == nil {
		lw.status = gitStatus(lw.repodir)
	}
	w.Label(lw.status.Summary(), "LC")

	switch lw.searchMode {
	case noSearch:
		w.Menu(label.TA("Search", "RC"), 120, func(w *nucular.Window) {
			lw.mu.Lock()
			defer lw.mu.Unlock()
			w.Row(20).Dynamic(1)
			if w.MenuItem(label.T("Path...")) {
				lw.searchEd.Buffer = []rune{}
				lw.searchMode = pathSearchSetup
			}
			if w.MenuItem(label.T("Grep...")) {
				lw.searchEd.Buffer = []rune{}
				lw.searchMode = grepSearchSetup
			}
		})
	case pathSearchSetup:
		w.Label("Path:", "RC")
		active := lw.searchEd.Edit(w)
		if active&nucular.EditCommitted != 0 {
			if len(lw.searchEd.Buffer) != 0 {
				lw.pathSearch(string(lw.searchEd.Buffer))
			} else {
				lw.searchMode = noSearch
			}
		}
	case grepSearchSetup:
		w.Label("Grep:", "RC")
		active := lw.searchEd.Edit(w)
		if active&nucular.EditCommitted != 0 {
			if len(lw.searchEd.Buffer) != 0 {
				lw.grepSearch(string(lw.searchEd.Buffer))
			} else {
				lw.searchMode = noSearch
			}
		}
	case searchRunning:
		w.Label("Searching...", "RC")
	case searchRestartMove:
		if lw.searchIdx >= len(lw.searchResults) && len(lw.searchResults) > 0 {
			lw.searchIdx = len(lw.searchResults) - 1
		}
		// it could still be that we got no results back, hence this check
		if lw.searchIdx < len(lw.searchResults) {
			lw.selectedId = lw.searchResults[lw.searchIdx]
			lw.searchMode = searchMove
			moveToSelected = true
		}
		fallthrough
	case searchMove:
		if len(lw.searchResults) == 0 {
			w.Label("No results", "RC")
			if w.ButtonText("Exit") {
				lw.searchMode = noSearch
			}
		} else {
			w.Label(fmt.Sprintf("%d/%d", lw.searchIdx, len(lw.searchResults)), "RC")
			if w.ButtonText("Next") {
				lw.searchIdx++
				if lw.searchIdx >= len(lw.searchResults) {
					if lw.searchDone {
						lw.searchIdx = len(lw.searchResults) - 1
						lw.selectedId = lw.searchResults[lw.searchIdx]
						moveToSelected = true
					} else {
						lw.searchMode = searchRunning
					}
				} else {
					lw.selectedId = lw.searchResults[lw.searchIdx]
					moveToSelected = true
				}
			}
			if w.ButtonText("Prev") {
				lw.searchIdx--
				if lw.searchIdx < 0 {
					lw.searchIdx = 0
				}
				lw.selectedId = lw.searchResults[lw.searchIdx]
				moveToSelected = true
			}
			if w.ButtonText("Exit") {
				if lw.searchDone {
					lw.searchMode = noSearch
				} else {
					lw.searchMode = searchAbort
				}
			}
		}
	case searchAbort:
		w.Label("Stopping search...", "RC")
	}

	w.MenubarEnd()

	style, scaling := w.Master().Style()

	oldspacing := style.GroupWindow.Spacing.Y
	style.GroupWindow.Spacing.Y = 0
	defer func() {
		style.GroupWindow.Spacing.Y = oldspacing
	}()

	lnh := int(graphLineHeight * scaling)
	thick := int(graphThick * scaling)

	kbd := w.KeyboardOnHover(w.Bounds)
	for _, e := range kbd.Keys {
		switch {
		case (e.Modifiers == 0) && (e.Code == key.CodeHome):
			w.Scrollbar.Y = 0
		case (e.Modifiers == 0) && (e.Code == key.CodeEnd):
			w.Scrollbar.Y = (lnh * len(lw.commits)) - w.Bounds.H
		case (e.Modifiers == 0) && (e.Code == key.CodeUpArrow):
			w.Scrollbar.Y -= lnh
		case (e.Modifiers == 0) && (e.Code == key.CodeDownArrow):
			w.Scrollbar.Y += lnh
		case (e.Modifiers == 0) && (e.Code == key.CodePageUp):
			w.Scrollbar.Y -= w.Bounds.H / 2
		case (e.Modifiers == 0) && (e.Code == key.CodePageDown):
			w.Scrollbar.Y += w.Bounds.H / 2
		}
		if w.Scrollbar.Y < 0 {
			w.Scrollbar.Y = 0
		}
	}
	switch kbd.Text {
	case "h":
		for _, lc := range lw.commits {
			if lc.IsHEAD {
				lw.selectedId = lc.Id
				moveToSelected = true
				break
			}
		}
	case " ":
		moveToSelected = true
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
		w.Row(graphLineHeight).Dynamic(1)
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

	var prevLanes [MaxLanes]bool

	skip := w.Scrollbar.Y/(lnh+style.GroupWindow.Spacing.Y) - 2

	if maxskip := len(lw.commits) - 3; skip > maxskip {
		skip = maxskip
	}

	if skip < 0 || moveToSelected {
		skip = 0
	}

	emptyCommitRows := func(n int) {
		if n > 0 {
			w.Row(n*graphLineHeight + (n-1)*style.GroupWindow.Spacing.Y).Dynamic(1)
			// this will never be seen, I could use a Spacing but I want to be bugs noticeable
			w.Label("More...", "LC")
		}
	}

	emptyCommitRows(skip)

	for i, lc := range lw.commits[skip:] {
		if !moveToSelected {
			if _, below := w.Invisible(); below {
				// fill the space that would be occupied by commits below the fold
				// with a big row
				emptyCommitRows(len(lw.commits[skip:]) - i)
				break
			}
		}

		w.RowScaled(lnh).Static()

		rowwidth := w.LayoutAvailableWidth()

		w.LayoutSetWidthScaled(lnh * lc.Occupied())
		bounds, out := w.Custom(nstyle.WidgetStateInactive)

		commitsz := calcCommitsz(bounds.W, includeAuthor, includeDate)

		refstr := ""
		ishead := false

		selected := lc.Id == lw.selectedId

		if selected && moveToSelected {
			lw.selectCommit(&lc)
			if above, below := w.Invisible(); above || below {
				w.Scrollbar.Y = w.At().Y - w.Bounds.H/2
				if w.Scrollbar.Y < 0 {
					w.Scrollbar.Y = 0
				}
				moveToSelected = false
			}
		}

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
			}

			if refsz > 0 {
				w.LayoutSetWidthScaled(refsz)
				if ishead {
					w.LabelColored(refstr, "LC", refsHeadColor)
				} else {
					w.LabelColored(refstr, "LC", refsColor)
				}
			}
		}

		if commitsz > 0 {
			w.LayoutSetWidthScaled(commitsz)
			w.SelectableLabel(lc.ShortMessage(), "LC", &selected)
		}

		if includeAuthor {
			w.LayoutSetWidthScaled(authorsz)
			w.SelectableLabel(nameInitials(lc.Author), "CC", &selected)
		}

		if includeDate {
			w.LayoutSetWidthScaled(datesz)
			w.SelectableLabel(lc.CommitterDate.Local().Format("2006-01-02 15:04"), "RC", &selected)
		}

		if selected && lc.Id != lw.selectedId {
			lw.selectCommit(&lc)
		}

		rowbounds := bounds
		rowbounds.W = rowwidth
		cm := &commitMenu{lw, lc}
		w.ContextualOpen(0, image.Point{200, 500}, rowbounds, cm.Update)

		if out == nil {
			copy(prevLanes[:], lc.LanesAfter[:])
			continue
		}

		// draws graph proper

		lanebounds := laneboundsOf(lnh, bounds, lc.Lane)
		circle := shrinkRect(lanebounds, int(float64(lnh) * 0.28))

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

func (cm *commitMenu) Update(w *nucular.Window) {
	lw, lc := cm.lw, cm.lc
	if lw.selectedId != lc.Id {
		lw.selectCommit(&lc)
	}
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

	w.Row(20).Dynamic(1)
	if _, bookmarked := bookmarks[lc.Id]; !bookmarked {
		if w.MenuItem(label.TA("Bookmark", "LC")) {
			bookmarks[lc.Id] = lc
		}
	}

	if w.MenuItem(label.TA("View", "LC")) {
		viewAction(lw, lc)
	}

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

	if w.MenuItem(label.TA("New branch", "LC")) {
		newNewBranchPopup(lw.mw, lc.Id)
	}

	if lw.Headisref {
		if w.MenuItem(label.TA(fmt.Sprintf("Reset %s here", lw.Head.Nice()), "LC")) {
			newResetPopup(lw.mw, lc.Id, resetHard)
		}
	}

	if !lc.IsHEAD {
		if w.MenuItem(label.TA("Cherrypick", "LC")) {
			cherrypickAction(lw, lc.Id)
		}
		if w.MenuItem(label.TA("Revert", "LC")) {
			revertAction(lw, lc.Id)
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

	if lw.Headisref {
		if w.MenuItem(label.TA("Merge", "LC")) {
			//TODO: show this commit as the default merge destination
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

func (lw *LogWindow) UpdateExtra(sw *nucular.Window) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	sw.Row(25).Static(120, 120, 0)
	showDetails := !lw.showOutput
	sw.SelectableLabel("Commit Details", "LC", &showDetails)
	lw.showOutput = !showDetails
	sw.SelectableLabel("Output", "LC", &lw.showOutput)
	sw.Label("space: center on selection, h: center on HEAD", "RC")

	if lw.showOutput {
		sw.Row(0).Dynamic(1)
		lw.edOutput.Edit(sw)
	} else {
		if lw.selectedView != nil {
			lw.selectedView.Update(sw)
		}
	}
}

func (lw *LogWindow) Title() string {
	return "Graph"
}

func (lw *LogWindow) Update(w *nucular.Window) {
	area := w.Row(0).SpaceBegin(0)
	b0, b1 := lw.split.Horizontal(w, area)

	w.LayoutSpacePushScaled(b0)
	if sw := w.GroupBegin("log-group-top", nucular.WindowBorder|nucular.WindowNoHScrollbar); sw != nil {
		lw.UpdateGraph(sw)
		sw.GroupEnd()
	}
	w.LayoutSpacePushScaled(b1)
	if sw := w.GroupBegin("log-group-bot", nucular.WindowBorder|nucular.WindowNoScrollbar); sw != nil {
		lw.UpdateExtra(sw)
		sw.GroupEnd()
	}
}

func (lw *LogWindow) reload() {
	lw.commits = lw.commits[:0]
	lw.maxOccupied = 0

	lw.selectCommit(nil)

	lw.Headisref = false
	lw.Head = nil

	lw.needsMore = -1
	lw.done = false
	lw.started = false
	lw.status = nil
}

func (lw *LogWindow) pathSearch(path string) {
	lw.searchMode = searchRunning
	lw.searchIdx = 0
	lw.searchDone = false
	lw.searchResults = lw.searchResults[:0]
	lw.searchCmd = exec.Command("git", "log", "--format=%H", "--color=never", "--", path)
	lw.searchCmd.Dir = lw.repodir
	lw.anySearch()
}

func (lw *LogWindow) grepSearch(pattern string) {
	lw.searchCmd = exec.Command("git", "log", "--format=%H", "--color=never", "--grep="+pattern)
	lw.searchCmd.Dir = lw.repodir
	lw.anySearch()
}

func (lw *LogWindow) anySearch() {
	lw.searchMode = searchRunning
	lw.searchIdx = 0
	lw.searchDone = false
	lw.searchResults = lw.searchResults[:0]
	go func() {
		stdout, err := lw.searchCmd.StdoutPipe()
		if err != nil {
			return
		}
		lw.searchCmd.Start()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			commitId := scanner.Text()
			lw.mu.Lock()
			if lw.searchMode == searchAbort {
				lw.mu.Unlock()
				stdout.Close()
				lw.searchCmd.Wait()
				lw.mu.Lock()
				lw.searchMode = noSearch
				lw.mu.Unlock()
				return
			}
			lw.searchResults = append(lw.searchResults, commitId)
			if lw.searchMode != searchMove {
				lw.searchMode = searchRestartMove
				lw.mw.Changed()
			}
			lw.mu.Unlock()
		}

		lw.searchCmd.Wait()

		lw.mu.Lock()
		if lw.searchMode == searchAbort {
			lw.searchMode = noSearch
		} else {
			lw.searchMode = searchRestartMove
			lw.searchDone = true
		}
		lw.mw.Changed()
		lw.mu.Unlock()
	}()
}
