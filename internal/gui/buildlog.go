package gui

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/nmsbonker/internal/core"
)

/*
The Build section's model: a step list and a log.

This is what spec 003 has that angou did not. A build takes 30 s to 3 minutes
and emits hundreds of lines, one per edit block, and those lines are the
product: they are how a user finds out that a mod's key was renamed by a game
update. angou's "results go in a banner, verbose output goes nowhere" stance is
right for a store operation and wrong for this, so the lines get a pane.

Two rules shape the code below. The log is written from core's worker goroutines
and read on the UI thread, so it is behind a mutex and the widget is refreshed
on a timer rather than per line — a hundred fyne.Do calls a second would make
the window slower than the build. And the pane never grows: it is a fixed-height
list inside a section that keeps its shape for the whole run, because nothing
transient may reflow the interface.
*/

// maxLogLines is how much of a build's output is retained (R2.3).
//
// A cold build of 27 mods emits a few thousand lines, so this holds all of one
// in the ordinary case and keeps the tail of a pathological one. The cap is on
// the model rather than on the widget because the widget only ever draws the
// rows it can see; what costs memory is the slice.
const maxLogLines = 5000

// logDropChunk is how many of the oldest lines go at once when the cap is hit.
// Dropping one line per append would copy the whole slice on every line for the
// rest of the build; dropping a chunk amortises that to nothing.
const logDropChunk = 512

// logLine is one line of build output and how to paint it.
type logLine struct {
	level core.Level
	text  string
}

// logModel holds the build log. Safe to append to from any goroutine.
type logModel struct {
	mu      sync.Mutex
	lines   []logLine
	dropped int
	// dirty says whether anything has been appended since the widget last drew.
	// The pump reads and clears it, so a quiet build costs one comparison per
	// tick rather than a redraw.
	dirty bool
}

func newLogModel() *logModel { return &logModel{} }

// append adds a line, dropping the oldest chunk when the cap is reached.
func (m *logModel) append(level core.Level, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.lines) >= maxLogLines {
		drop := min(logDropChunk, len(m.lines))
		m.lines = append(m.lines[:0], m.lines[drop:]...)
		m.dropped += drop
	}
	m.lines = append(m.lines, logLine{level: level, text: text})
	m.dirty = true
}

// reset empties the log for a new run.
func (m *logModel) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lines, m.dropped, m.dirty = nil, 0, true
}

func (m *logModel) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.lines)
}

// droppedCount is how many lines fell off the front, which the pane says out
// loud rather than quietly showing a log that starts mid-sentence.
func (m *logModel) droppedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dropped
}

// at returns one line, or the zero line for an index that has since been
// dropped. The widget's item count and its update callback are read at
// different moments, so an out-of-range index is expected rather than a bug.
func (m *logModel) at(i int) logLine {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.lines) {
		return logLine{}
	}
	return m.lines[i]
}

// takeDirty reports whether anything changed since the last call, and clears
// the flag.
func (m *logModel) takeDirty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	was := m.dirty
	m.dirty = false
	return was
}

// text renders the whole log for the clipboard.
func (m *logModel) text() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	if m.dropped > 0 {
		b.WriteString("… ")
		b.WriteString(strconv.Itoa(m.dropped))
		b.WriteString(" earlier line(s) were dropped; the log keeps the last ")
		b.WriteString(strconv.Itoa(maxLogLines))
		b.WriteString(".\n")
	}
	for _, l := range m.lines {
		b.WriteString(l.text)
		b.WriteString("\n")
	}
	return b.String()
}

// --- the step list ---------------------------------------------------------

// stepState is how far one step of the build has got.
type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepDone
	stepFailed
	stepCancelled
)

// The steps, in the order they run. The names are the spec's (R2.3).
const (
	stepDetect = iota
	stepTools
	stepCache
	stepMerge
	stepCompile
	stepReport
)

// buildStep is one row of the step list.
type buildStep struct {
	name  string
	state stepState
	note  string
}

// newSteps is the step list for a run. Deploy is not a step: it is its own
// button, pressed after the report has been read.
func newSteps() []buildStep {
	return []buildStep{
		{name: "Detect", note: "the game install and the mod library"},
		{name: "Tools", note: "MBINCompiler compatibility"},
		{name: "Cache", note: "pak index and pristine game files"},
		{name: "Merge", note: "apply every mod's edits"},
		{name: "Compile", note: "recompile each merged file"},
		{name: "Report", note: "verdicts and timings"},
	}
}

/*
stepFor maps one of core's progress messages onto a step.

core reports five phases plus two per-item streams, and this is the whole of the
translation. Two things are worth knowing about it. The pristine cache, the pak
index and script loading are one step here ("Cache") because from the outside
they are one wait: nothing is on screen between them. And merge and compile are
reported by core as a single per-target phase — a target is merged and then
compiled by the same worker — so the two rows are split at the first completed
target rather than at a phase boundary. Merge going green means at least one
file has been merged, not that all of them have.
*/
func stepFor(what string) (int, bool) {
	switch what {
	case "checking compiler compatibility":
		return stepTools, true
	case "loading mod scripts", "indexing the game archives", "preparing pristine game files":
		return stepCache, true
	case "merging and compiling":
		return stepMerge, true
	}
	for _, p := range []struct {
		prefix string
		step   int
	}{
		{"indexing ", stepCache},
		{"decompiling ", stepCache},
		{"building ", stepCompile},
	} {
		if strings.HasPrefix(what, p.prefix) {
			return p.step, true
		}
	}
	return 0, false
}

// --- the run ---------------------------------------------------------------

// buildRun is everything about the build in progress, or the last one.
//
// It lives on *ui rather than inside the Build section because a build outlives
// the section: the user can walk off to Mods and come back, and the log has to
// still be there when they do.
type buildRun struct {
	running   bool
	cancelled bool
	cancel    context.CancelFunc

	steps []buildStep
	log   *logModel

	// follow is the auto-scroll state: true until the user scrolls up.
	follow bool
	// wantOffset is the scroll offset left behind by the last auto-scroll, so
	// the next tick can tell "the user dragged the pane up" from "nothing
	// moved". Fyne's list has no scroll callback, only an offset to read.
	wantOffset float32

	// finished describes the last completed run, so the section can be rebuilt
	// after the fact and still say what happened.
	finished  bool
	summary   string
	summarySt Status

	// Live widgets, non-nil only while the Build section is on screen. They are
	// updated in place: rebuilding the section on every log line is exactly the
	// reflow the project rule forbids.
	list      *widget.List
	rows      []stepRowWidgets
	counter   *widget.Label
	totals    *widget.Label
	followBox *widget.Check
	// controls are the buttons that start work, disabled while a run is in
	// flight; cancelBtn is the one that is enabled only then, and deployBtn and
	// viewBtn need a report rather than an idle window.
	controls  []*widget.Button
	cancelBtn *widget.Button
	deployBtn *widget.Button
	viewBtn   *widget.Button
}

// stepRowWidgets are the three objects one step row is made of.
type stepRowWidgets struct {
	icon *widget.Icon
	name *widget.Label
	note *widget.Label
}

func (r *buildRun) init() {
	r.log = newLogModel()
	r.follow = true
	r.steps = newSteps()
}

// detach forgets the live widgets. Called when the content pane is replaced: a
// build streaming into widgets nothing is drawing wastes work, and holding the
// old ones would keep a whole section tree alive for the life of the window.
func (r *buildRun) detach() {
	r.list, r.rows, r.counter, r.totals, r.followBox = nil, nil, nil, nil, nil
	r.controls, r.cancelBtn, r.deployBtn, r.viewBtn = nil, nil, nil, nil
	// The next list starts at the top, so the offset the last automatic scroll
	// left behind describes a widget that no longer exists. Carried over, it
	// makes the fresh pane look as though the user had scrolled up in it, and
	// the pane stops following the tail on the first tick after a rebuild.
	r.wantOffset = 0
}

// reset starts a new run's state. The log is emptied rather than appended to:
// two builds' output in one pane, with no divider a scroll bar can find, is
// worse than losing the previous one, which is on disk in the report anyway.
func (r *buildRun) reset() {
	r.running, r.cancelled, r.finished = true, false, false
	r.steps = newSteps()
	r.steps[stepDetect].state = stepRunning
	r.summary, r.summarySt = "", StatusInfo
	r.follow = true
	r.wantOffset = 0
	r.log.reset()
}

// advance marks a step running and everything before it done. Progress arrives
// out of order — the cache reports per-file progress after the merge phase has
// been announced on a slow filesystem — so this never walks a step backwards.
func (r *buildRun) advance(step int, note string) {
	if step >= len(r.steps) {
		return
	}
	for i := range step {
		if r.steps[i].state == stepPending || r.steps[i].state == stepRunning {
			r.steps[i].state = stepDone
		}
	}
	if r.steps[step].state == stepPending || r.steps[step].state == stepRunning {
		r.steps[step].state = stepRunning
		if note != "" {
			r.steps[step].note = note
		}
	}
}

// finish marks every step up to and including step as done.
func (r *buildRun) finish(step int, note string) {
	if step >= len(r.steps) {
		step = len(r.steps) - 1
	}
	for i := 0; i <= step; i++ {
		if r.steps[i].state != stepFailed && r.steps[i].state != stepCancelled {
			r.steps[i].state = stepDone
		}
	}
	if note != "" && step >= 0 {
		r.steps[step].note = note
	}
}

// stop records how the run ended: the step that was running takes the verdict,
// and the ones that never started stay pending rather than being painted as
// failures they had no part in.
func (r *buildRun) stop(state stepState, note string) {
	for i := range r.steps {
		if r.steps[i].state == stepRunning {
			r.steps[i].state = state
			if note != "" {
				r.steps[i].note = note
			}
			break
		}
	}
	r.running = false
	r.finished = true
}

/*
followTail decides whether the log pane should keep scrolling to the end.

Fyne's list has no "the user scrolled" callback, so this compares the offset the
last auto-scroll left behind with the offset the list is at now. Lower means the
user dragged the pane up to read something, and a pane that yanks itself back to
the bottom half a second later is unusable. Following resumes when they scroll
back down, or when they tick the box.

The tolerance is there because ScrollToBottom lands on a fractional offset and
the list re-measures as rows arrive; without it the pane stops following itself.
*/
func followTail(following bool, current, wanted float32) bool {
	const tolerance = 4
	if !following {
		return false
	}
	return current >= wanted-tolerance
}

// stepIcon is the marker for a step's state.
func stepIcon(s stepState) fyne.Resource {
	switch s {
	case stepRunning:
		return theme.MediaPlayIcon()
	case stepDone:
		return theme.ConfirmIcon()
	case stepFailed:
		return theme.ErrorIcon()
	case stepCancelled:
		return theme.CancelIcon()
	}
	return theme.RadioButtonIcon()
}

// stepStatus ranks a step's state for the note's colour.
func stepStatus(s stepState) Status {
	switch s {
	case stepDone:
		return StatusGood
	case stepFailed:
		return StatusBad
	case stepCancelled:
		return StatusWarn
	}
	return StatusInfo
}
