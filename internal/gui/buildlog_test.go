package gui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/steps"

	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/core"
)

// The log model, the follow-tail rule and the pane's drawing are the
// library's and are tested there. What is pinned here is this program's own:
// the mapping from core's progress messages onto steps, the run's state, and
// the controls.

// --- the step list ---------------------------------------------------------

// Every progress message core emits during a build must land on a step. One
// that does not is a step list that sits at "Detect" through a three-minute
// compile.
func TestEveryBuildProgressMessageMapsToAStep(t *testing.T) {
	cases := map[string]int{
		"checking compiler compatibility": stepTools,
		"loading mod scripts":             stepCache,
		"indexing the game archives":      stepCache,
		"indexing NMSARC.Precache.pak":    stepCache,
		"preparing pristine game files":   stepCache,
		"decompiling rewardtable.mbin":    stepCache,
		"merging and compiling":           stepMerge,
		"building REWARDTABLE.MBIN":       stepCompile,
	}
	for what, want := range cases {
		got, ok := stepFor(what)
		require.Truef(t, ok, "%q maps to no step", what)
		require.Equalf(t, want, got, "%q went to the wrong step", what)
	}
	_, ok := stepFor("something core does not emit")
	require.False(t, ok, "an unknown message must be ignored, not misfiled")
}

// Progress arrives from several worker goroutines and is not ordered, so the
// step list must never walk backwards: a late "decompiling" after the compile
// has started would otherwise un-tick two steps. The library's Advance would
// reopen the step; the guard is this program's.
func TestTheStepListNeverGoesBackwards(t *testing.T) {
	var r buildRun
	r.init()
	r.reset()

	r.advance(stepCompile, "building")
	require.Equal(t, steps.Done, r.steps.Steps()[stepCache].State)

	r.advance(stepCache, "a straggler from the cache")
	require.Equal(t, steps.Done, r.steps.Steps()[stepCache].State,
		"a late message must not reopen a finished step")
	require.Equal(t, steps.Running, r.steps.Steps()[stepCompile].State)
}

// A cancelled run marks the step that was running and leaves the ones that
// never started alone: painting them as failures blames them for something
// they had no part in.
func TestCancellingMarksOnlyTheRunningStep(t *testing.T) {
	var r buildRun
	r.init()
	r.reset()
	r.advance(stepCache, "preparing pristine game files")

	r.stop(steps.Cancelled, "cancelled")

	all := r.steps.Steps()
	require.Equal(t, steps.Done, all[stepTools].State)
	require.Equal(t, steps.Cancelled, all[stepCache].State)
	require.Equal(t, steps.Pending, all[stepCompile].State)
	require.False(t, r.running)
	require.True(t, r.finished)
}

// A finished run marks every step through the report done, including the
// ones core never announced by name.
func TestFinishingMarksEveryStepThroughTheReportDone(t *testing.T) {
	var r buildRun
	r.init()
	r.reset()
	r.advance(stepCache, "indexing")

	r.finish(stepReport, "written")

	for i, s := range r.steps.Steps() {
		require.Equalf(t, steps.Done, s.State, "step %d (%s) is not done", i, s.Name)
	}
	require.Equal(t, "written", r.steps.Steps()[stepReport].Note)
}

// A new run starts from pending steps carrying their standing notes, with
// Detect already running: the list must not open blank.
func TestResetPutsTheStandingNotesBack(t *testing.T) {
	var r buildRun
	r.init()
	r.reset()
	r.advance(stepCompile, "building X")
	r.reset()

	all := r.steps.Steps()
	require.Equal(t, steps.Running, all[stepDetect].State)
	require.Equal(t, "the game install and the mod library", all[stepDetect].Note)
	require.Equal(t, steps.Pending, all[stepCompile].State)
	require.Equal(t, "recompile each merged file", all[stepCompile].Note)
	require.Zero(t, r.pane.Model().Len(), "the previous run's output is gone")
}

// Deploy is not a step. It is a separate button, pressed once the report has
// been read, so the step list ends where the build does.
func TestTheStepListEndsAtTheReport(t *testing.T) {
	all := buildSteps()
	require.Len(t, all, 6)
	require.Equal(t, "Report", all[len(all)-1].name)
	var r buildRun
	r.init()
	require.Len(t, r.steps.Steps(), len(all))
}

// --- the controls ----------------------------------------------------------

// toolbar builds the Build section headlessly and returns the button named.
func toolbar(t *testing.T, u *ui, label string) *widget.Button {
	t.Helper()
	b := fynetest.FindButton(u.buildBuild(), label)
	require.NotNilf(t, b, "no %q button in the Build section", label)
	return b
}

/*
R4.2: one operation at a time, and the buttons say so by being disabled rather
than by disappearing.

Cancel is the mirror image — the only control that is live while a build runs,
and dead the moment it has been pressed, because pressing it twice does nothing
and a button that looks live is a button that gets pressed again. The section
is rebuilt from state when work starts and stops, so the test builds it in
each state.
*/
func TestTheCancelButtonFollowsTheRunningState(t *testing.T) {
	u := testUI(t)

	require.False(t, toolbar(t, u, "Build").Disabled(), "an idle window offers to build")
	require.True(t, toolbar(t, u, "Cancel").Disabled(), "with nothing to cancel")

	u.run.running = true
	require.True(t, toolbar(t, u, "Build").Disabled(), "a second build must not be startable")
	require.True(t, toolbar(t, u, "Rebuild cache").Disabled())
	require.False(t, toolbar(t, u, "Cancel").Disabled())

	u.run.cancelled = true
	require.True(t, toolbar(t, u, "Cancel").Disabled(), "cancelling twice does nothing, so it stops offering")

	u.run.running, u.run.cancelled = false, false
	require.False(t, toolbar(t, u, "Build").Disabled())
	require.True(t, toolbar(t, u, "Cancel").Disabled())
}

// Pressing Cancel kills the button in place, without waiting for the rebuild
// the build's end brings.
func TestPressingCancelDisablesItAtOnce(t *testing.T) {
	u := testUI(t)
	u.run.running = true
	cancelled := false
	u.run.cancel = func() { cancelled = true }
	cancel := toolbar(t, u, "Cancel")
	require.False(t, cancel.Disabled())

	test.Tap(cancel)

	require.True(t, cancelled, "the run's context is cancelled")
	require.True(t, u.run.cancelled)
	require.True(t, cancel.Disabled())
}

// View report needs a report to view. Enabled with none, it leads to a section
// whose only content is "No build yet".
func TestViewReportNeedsAReport(t *testing.T) {
	u := testUI(t)
	require.True(t, toolbar(t, u, "View report").Disabled())

	u.lastReport = core.ReportResult{Report: &report.Result{}}
	require.False(t, toolbar(t, u, "View report").Disabled())
}

// Deploy installs the last build, so it needs one, and a game to put it in. It
// is also not startable while a build is running: the folder it would copy is
// the one being rewritten.
func TestDeployNeedsABuildAndAGame(t *testing.T) {
	u := testUI(t)
	require.True(t, toolbar(t, u, "Deploy…").Disabled(), "nothing has been built")

	u.lastReport = core.ReportResult{Report: &report.Result{}}
	require.True(t, toolbar(t, u, "Deploy…").Disabled(), "no game install to deploy into")

	u.status.Install.Found = true
	require.False(t, toolbar(t, u, "Deploy…").Disabled())

	u.run.running = true
	require.True(t, toolbar(t, u, "Deploy…").Disabled(), "not while the build is rewriting the folder")
}

// Progress events from core's workers land on the step list on the UI thread
// (spec 012 AC6): the step named runs, the ones before it are done.
func TestProgressEventsAdvanceTheStepList(t *testing.T) {
	u := testUI(t)
	u.run.reset()
	ev := u.buildEvents()

	ev.Progress(core.Progress{What: "checking compiler compatibility"})
	all := u.run.steps.Steps()
	require.Equal(t, steps.Done, all[stepDetect].State)
	require.Equal(t, steps.Running, all[stepTools].State)

	ev.Progress(core.Progress{What: "building REWARDTABLE.MBIN", Step: 3, Total: 100})
	all = u.run.steps.Steps()
	require.Equal(t, steps.Done, all[stepMerge].State)
	require.Equal(t, steps.Running, all[stepCompile].State)
	require.Equal(t, "building REWARDTABLE.MBIN (3 of 100)", all[stepCompile].Note)

	ev.Progress(core.Progress{What: "something core does not emit"})
	require.Equal(t, steps.Running, u.run.steps.Steps()[stepCompile].State, "an unknown message changes nothing")
}

// The totals and the pane are drawn several times a second from the log pump,
// including while the section is not on screen and its widgets are nil.
func TestDrawingWithNoWidgetsIsHarmless(t *testing.T) {
	u := testUI(t)
	u.run.reset()
	u.run.detach()
	require.NotPanics(t, func() {
		u.drawTotals()
		u.run.pane.Draw()
	})
}

// The build's log lines are coloured by core's level and nothing else is added
// to them: the line is the product, and a timestamp in front of every edit
// would push the file names off the right edge of the pane.
func TestBuildLinesKeepCoresWordingAndLevel(t *testing.T) {
	u := testUI(t)
	ev := u.buildEvents()
	ev.Log(core.LevelWarn, "no game file for X: compiler said\n[INFO]: one\n")
	ev.Log(core.LevelDebug, "   OK  Mod: detail")

	m := u.run.pane.Model()
	require.Equal(t, 3, m.Len(), "a message with newlines is one row per line")
	require.Equal(t, "no game file for X: compiler said", m.At(0).Text)
	require.Equal(t, "[INFO]: one", m.At(1).Text)
	require.Equal(t, "   OK  Mod: detail", m.At(2).Text)
	require.Equal(t, logLevelWarn(), m.At(0).Level)
	require.Equal(t, logLevelDebug(), m.At(2).Level)
}
