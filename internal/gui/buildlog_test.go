package gui

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/core"
)

// --- the log model ---------------------------------------------------------

/*
A build's log is bounded, and the end of it is the part worth keeping.

A cold build of a large library emits thousands of lines and a pathological one
could emit far more. Without a cap the window's memory grows with the build; with
a cap that dropped from the wrong end, the pane would show the beginning of a
build and hide the verdicts, which are the reason anyone is reading it.
*/
func TestTheBuildLogKeepsTheTailWithinItsCap(t *testing.T) {
	m := newLogModel()
	const written = maxLogLines + 1500
	for i := range written {
		m.append(core.LevelDebug, fmt.Sprintf("line %d", i))
	}

	require.LessOrEqual(t, m.len(), maxLogLines, "the log must not grow without bound")
	require.Positive(t, m.droppedCount(), "and it must admit that it dropped something")
	require.Equal(t, fmt.Sprintf("line %d", written-1), m.at(m.len()-1).text,
		"the newest line must survive: it is the one being watched")
	require.Equal(t, written, m.len()+m.droppedCount(),
		"every line is either kept or counted as dropped")
}

// The copied log says so when it is not the whole log. A user pasting it into a
// bug report should not have to work out that the first half is missing.
func TestTheCopiedLogAdmitsWhatWasDropped(t *testing.T) {
	m := newLogModel()
	for i := range maxLogLines + 10 {
		m.append(core.LevelInfo, fmt.Sprintf("line %d", i))
	}
	require.Contains(t, m.text(), "earlier line(s) were dropped")

	m.reset()
	m.append(core.LevelInfo, "only line")
	require.Equal(t, "only line\n", m.text(), "a short log is copied verbatim")
}

// An index the widget asks for after a drop must not panic: the item count and
// the update callback are read at different moments, so the list can ask for a
// row that has just gone.
func TestTheLogToleratesAnIndexThatHasGone(t *testing.T) {
	m := newLogModel()
	m.append(core.LevelInfo, "one")
	require.NotPanics(t, func() {
		require.Empty(t, m.at(-1).text)
		require.Empty(t, m.at(99).text)
	})
}

// The pump redraws only when something arrived, so a build that is thinking
// costs nothing.
func TestTheLogReportsWhetherItChanged(t *testing.T) {
	m := newLogModel()
	require.False(t, m.takeDirty(), "an untouched log has nothing to redraw")
	m.append(core.LevelInfo, "x")
	require.True(t, m.takeDirty())
	require.False(t, m.takeDirty(), "and the flag is cleared by reading it")
}

// --- auto-scroll -----------------------------------------------------------

/*
The pane stops following the tail when the user scrolls up to read something.

Fyne's list has no scroll callback, so the only evidence is the offset: lower
than where the last automatic scroll left it means the user dragged it. A pane
that yanked itself back to the bottom half a second later would make the log
unreadable during exactly the three minutes it is worth reading.
*/
func TestTheLogStopsFollowingWhenTheUserScrollsUp(t *testing.T) {
	require.True(t, followTail(true, 500, 500), "sitting at the end keeps following")
	require.True(t, followTail(true, 498, 500),
		"a couple of pixels is the list re-measuring itself, not a user")
	require.False(t, followTail(true, 200, 500), "scrolling up stops the follow")
	require.False(t, followTail(false, 500, 500),
		"and it does not resume by itself: the checkbox is how it comes back")
}

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
// has started would otherwise un-tick two steps.
func TestTheStepListNeverGoesBackwards(t *testing.T) {
	var r buildRun
	r.init()
	r.reset(false)

	r.advance(stepCompile, "building")
	require.Equal(t, stepDone, r.steps[stepCache].state)

	r.advance(stepCache, "a straggler from the cache")
	require.Equal(t, stepDone, r.steps[stepCache].state,
		"a late message must not reopen a finished step")
	require.Equal(t, stepRunning, r.steps[stepCompile].state)
}

// A cancelled run marks the step that was running and leaves the ones that
// never started alone: painting them as failures blames them for something
// they had no part in.
func TestCancellingMarksOnlyTheRunningStep(t *testing.T) {
	var r buildRun
	r.init()
	r.reset(false)
	r.advance(stepCache, "preparing pristine game files")

	r.stop(stepCancelled, "cancelled")

	require.Equal(t, stepDone, r.steps[stepTools].state)
	require.Equal(t, stepCancelled, r.steps[stepCache].state)
	require.Equal(t, stepPending, r.steps[stepCompile].state)
	require.False(t, r.running)
	require.True(t, r.finished)
}

// Deploy is a step only when the run was asked to deploy. A row that is always
// there and usually never runs reads as something that failed.
func TestTheDeployStepIsOnlyThereWhenDeploying(t *testing.T) {
	require.Len(t, newSteps(false), 6)
	steps := newSteps(true)
	require.Len(t, steps, 7)
	require.Equal(t, "Deploy", steps[stepDeploy].name)
}

// --- the controls ----------------------------------------------------------

/*
R4.2: one operation at a time, and the buttons say so by being disabled rather
than by disappearing.

Cancel is the mirror image — the only control that is live while a build runs,
and dead the moment it has been pressed, because pressing it twice does nothing
and a button that looks live is a button that gets pressed again.
*/
func TestTheCancelButtonFollowsTheRunningState(t *testing.T) {
	u := testUI(t)
	build := widget.NewButton("Build", nil)
	cancel := widget.NewButton("Cancel", nil)
	u.run.controls = []*widget.Button{build}
	u.run.cancelBtn = cancel

	u.drawControls()
	require.False(t, build.Disabled(), "an idle window offers to build")
	require.True(t, cancel.Disabled(), "with nothing to cancel")

	u.run.running = true
	u.drawControls()
	require.True(t, build.Disabled(), "a second build must not be startable")
	require.False(t, cancel.Disabled())

	u.run.cancelled = true
	u.drawControls()
	require.True(t, cancel.Disabled(), "cancelling twice does nothing, so it stops offering")

	u.run.running, u.run.cancelled = false, false
	u.drawControls()
	require.False(t, build.Disabled())
	require.True(t, cancel.Disabled())
}

// View report needs a report to view. Enabled with none, it leads to a section
// whose only content is "No build yet".
func TestViewReportNeedsAReport(t *testing.T) {
	u := testUI(t)
	u.run.viewBtn = widget.NewButton("View report", nil)

	u.drawControls()
	require.True(t, u.run.viewBtn.Disabled())
}

// drawSteps and drawLog run several times a second from the log pump, including
// while the section is not on screen and its widgets are nil.
func TestDrawingWithNoWidgetsIsHarmless(t *testing.T) {
	u := testUI(t)
	u.run.reset(false)
	u.run.detach()
	require.NotPanics(t, func() {
		u.drawSteps()
		u.drawLog()
		u.drawControls()
	})
}
