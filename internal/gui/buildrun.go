package gui

import (
	"context"
	"strings"

	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/logpane"
	"github.com/ushineko/fynedesygn/steps"
)

/*
The Build section's model: a step list and a log.

A build takes 30 s to 3 minutes and emits hundreds of lines, one per edit
block, and those lines are the product: they are how a user finds out that a
mod's key was renamed by a game update. The step list and the log pane are
fynedesygn's; what is this program's is the mapping from core's progress
messages onto steps, the cap, and the state of the run.
*/

// maxLogLines is how much of a build's output is retained (R2.3).
//
// A cold build of 27 mods emits a few thousand lines, so this holds all of one
// in the ordinary case and keeps the tail of a pathological one. The cap is on
// the model rather than on the widget because the widget only ever draws the
// rows it can see; what costs memory is the slice.
const maxLogLines = 5000

// The steps, in the order they run. The names are the spec's (R2.3).
const (
	stepDetect = iota
	stepTools
	stepCache
	stepMerge
	stepCompile
	stepReport
)

// buildStep is one row of the step list before a run: its name and what the
// note says while it is pending.
type buildStep struct {
	name string
	note string
}

// buildSteps is the step list for a run. Deploy is not a step: it is its own
// button, pressed after the report has been read.
func buildSteps() []buildStep {
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

	// steps and pane hold the step list and the log across rebuilds of the
	// section; their widgets are built per visit and dropped by detach.
	steps *steps.List
	pane  *logpane.Pane

	// finished describes the last completed run, so the section can be rebuilt
	// after the fact and still say what happened.
	finished  bool
	summary   string
	summarySt fd.Status

	// Live widgets, non-nil only while the Build section is on screen. They are
	// updated in place: rebuilding the section on every log line is exactly the
	// reflow the project rule forbids.
	totals *widget.Label
	// cancelBtn is the one control that is live only while a run is in flight;
	// the builder gates the rest from state, and Cancel is disabled in place the
	// moment it is pressed.
	cancelBtn *widget.Button
}

func (r *buildRun) init() {
	r.pane = logpane.New(logpane.NewModel(maxLogLines))
	names := make([]string, 0, len(buildSteps()))
	for _, s := range buildSteps() {
		names = append(names, s.name)
	}
	r.steps = steps.New(names...)
	r.pendingNotes()
}

// pendingNotes writes each step's standing note, the one it shows before and
// until the run reaches it.
func (r *buildRun) pendingNotes() {
	for i, s := range buildSteps() {
		r.steps.Set(i, steps.Pending, s.note)
	}
}

// detach forgets the live widgets. Called when the content pane is replaced: a
// build streaming into widgets nothing is drawing wastes work, and holding the
// old ones would keep a whole section tree alive for the life of the window.
func (r *buildRun) detach() {
	r.steps.Detach()
	r.pane.Detach()
	r.totals, r.cancelBtn = nil, nil
}

// reset starts a new run's state. The log is emptied rather than appended to:
// two builds' output in one pane, with no divider a scroll bar can find, is
// worse than losing the previous one, which is on disk in the report anyway.
func (r *buildRun) reset() {
	r.running, r.cancelled, r.finished = true, false, false
	r.steps.Reset()
	r.pendingNotes()
	r.steps.Set(stepDetect, steps.Running, buildSteps()[stepDetect].note)
	r.summary, r.summarySt = "", fd.StatusInfo
	r.pane.Model().Reset()
}

// advance marks a step running and everything before it done. Progress arrives
// out of order — the cache reports per-file progress after the merge phase has
// been announced on a slow filesystem — so this never walks a step backwards:
// a step that is already done, failed or cancelled is left as it is.
func (r *buildRun) advance(step int, note string) {
	all := r.steps.Steps()
	if step < 0 || step >= len(all) {
		return
	}
	if st := all[step].State; st != steps.Pending && st != steps.Running {
		return
	}
	if note == "" {
		note = all[step].Note
	}
	r.steps.Advance(step, note)
}

// finish marks every step up to and including step as done, with the note on
// the last of them.
func (r *buildRun) finish(step int, note string) {
	if n := len(r.steps.Steps()); step >= n {
		step = n - 1
	}
	if step < 0 {
		return
	}
	r.steps.Advance(step, note)
	r.steps.Finish(step, note)
}

// stop records how the run ended: the step that was running takes the verdict,
// and the ones that never started stay pending rather than being painted as
// failures they had no part in.
func (r *buildRun) stop(state steps.State, note string) {
	r.steps.Stop(state, note)
	r.running = false
	r.finished = true
}
