package gui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"github.com/ushineko/nmsbonker/internal/core"
)

/*
Talking to the core from a window.

internal/core is synchronous: an operation takes a request struct, does the work
on the calling goroutine, and returns a result. A cold build is three minutes of
that. So every call in this file runs on a goroutine of its own and hops back to
the UI thread with fyne.Do, and nothing here may be called from a widget handler
without the `go`.

Two things follow from that and are easy to get wrong. The busy indicator has to
be started before the goroutine can fail, or a failure leaves the strip spinning
for the life of the window -- hence the deferred done(). And the results a
section renders are fields on *ui, written only on the UI thread, so a load that
finishes after the user has navigated away updates state that the next rebuild
picks up rather than a widget that is no longer on screen.
*/

// request is the core request every operation takes, carrying the --config
// override so that this window and `nmsbonker --config …` read one document.
func (u *ui) request() core.Request {
	return core.Request{ConfigPath: u.configPath}
}

// report puts a failed operation on screen. Cancellation is not a failure: the
// user asked for it, and the step list already says so.
func (u *ui) report(what string, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	fyne.Do(func() { u.flash(what+": "+err.Error(), StatusBad) })
}

// ok reports a completed operation and treats what is on screen as stale.
//
// An operation that succeeded has usually changed something a section is
// showing — the mod list, the installed tools, the report — so invalidating is
// the safe default. Reloading costs one core call; showing a list that no
// longer matches the config is how someone removes the wrong mod.
func (u *ui) ok(msg string) {
	fyne.Do(func() {
		u.flash(msg, StatusGood)
		u.invalidate()
	})
}

// invalidate discards everything loaded from the core and rebuilds, which makes
// the sections fetch again. Called on the UI thread.
//
// The build log is deliberately not part of this: F5 while a build is running
// must not throw away the output it has produced so far.
func (u *ui) invalidate() {
	u.statusOK = false
	u.detectOK = false
	u.modsOK = false
	u.tweaksOK = false
	u.savesOK = false
	u.archiveOK = false
	u.toolsOK = false
	u.releasesOK = false
	u.cacheOK = false
	u.configOK = false
	u.lastReportOK = false
	u.freshAudit = nil
	if !u.onScreen() {
		// No window to redraw. Clearing the flags is the whole of the work:
		// whatever builds the sections next will fetch. Fetching here anyway
		// would send a headless test off to read the user's Steam install.
		return
	}
	// The status bar names the game, the compiler and the mod library from every
	// section, so both are fetched here rather than left to Overview. Left to a
	// section, the bar read "reading…" and "—" everywhere else.
	u.loadStatus()
	u.loadMods()
	u.rebuild()
}

// onScreen reports whether there is a window to draw into. False in a headless
// test, and in the window between a load finishing and the application exiting.
func (u *ui) onScreen() bool { return u.content != nil }

/*
perform runs one core operation off the UI thread with the busy indicator up.

The name is what the status bar shows, so it is a phrase in the present
participle rather than a command: "Installing MBINCompiler…", not "ensure".
Every core call from this window goes through here or through startBuild; a raw
`go func()` reaching into core would be a window that sits still with no
explanation, and the button that looks like it did nothing is the button that
gets clicked twice.
*/
func (u *ui) perform(what string, fn func(ctx context.Context) error) {
	if u.working() {
		u.flash("Something is already running. Wait for it to finish, or cancel it.", StatusWarn)
		return
	}
	if !u.onScreen() {
		// No window, so there is no render thread to keep free and the
		// goroutine buys nothing. A headless test gets a finished operation
		// when the button returns instead of one that lands "soon", which is
		// the difference between a test and a race: Fyne's test driver runs
		// fyne.Do inline on the calling goroutine rather than serialising onto
		// a main loop, so a worker refreshing a widget genuinely does race the
		// test driving it.
		if err := fn(context.Background()); err != nil {
			u.report(what, err)
		}
		return
	}
	go func() {
		done := u.busy(what)
		defer done()
		if err := fn(context.Background()); err != nil {
			u.report(what, err)
		}
	}()
}

// --- loaders ---------------------------------------------------------------

// loadStatus fills the install, tools and library facts, which Overview renders
// and the status bar summarises.
func (u *ui) loadStatus() {
	if u.statusOK {
		return
	}
	// Marked loaded before the call rather than after, so a section rebuilt
	// while the first load is still running does not start a second one. A
	// failed load leaves the flag set and the facts empty, which is the state
	// Overview explains; F5 tries again.
	u.statusOK = true
	go func() {
		done := u.busy("Reading the game install…")
		defer done()
		res, err := core.Status(context.Background(), core.StatusRequest{Request: u.request()})
		if err != nil {
			u.report("Read the install", err)
			return
		}
		fyne.Do(func() {
			u.status = res
			u.rebuild()
		})
	}()
}

// loadDetect explains a failed search. Only fetched when there is one to
// explain: on a machine with a game, it is a listing of Steam libraries nobody
// asked for.
func (u *ui) loadDetect() {
	if u.detectOK {
		return
	}
	u.detectOK = true
	go func() {
		done := u.busy("Looking for the game…")
		defer done()
		res, err := core.Detect(context.Background(), core.DetectRequest{Request: u.request()})
		if err != nil {
			u.report("Look for the game", err)
			return
		}
		fyne.Do(func() {
			u.detect = res
			u.refresh()
		})
	}()
}

// loadMods fills the build order. ListMods reconciles the config with the
// library and may write the config, which is why arriving at the section is
// enough to pick up a script someone dropped into the library from a file
// manager.
func (u *ui) loadMods() {
	if u.modsOK {
		return
	}
	u.modsOK = true
	go func() {
		done := u.busy("Reading the mod library…")
		defer done()
		res, err := core.ListMods(context.Background(), core.ListModsRequest{Request: u.request()})
		if err != nil {
			u.report("Read the mod library", err)
			return
		}
		// The script headers come with the listing rather than on a button.
		// Loading twenty-seven scripts through the embedded sandbox is a
		// hundredth of a second and reads no game files, and a table with an
		// Author column that fills in later reads as a table that is broken.
		checks, cerr := core.CheckMods(context.Background(), core.CheckModsRequest{
			Request: u.request(), All: true,
		})
		fyne.Do(func() {
			u.mods = res
			if cerr == nil {
				u.setChecks(checks)
			}
			u.rebuild()
			for _, n := range res.Notices {
				u.flash(n, StatusInfo)
			}
		})
	}()
}

/*
loadTweaks fills the built-in tweaks and their parameters.

It reads the last build report as well, to work out whether what is on screen
has been built yet, so it is a section load rather than something the status bar
needs.
*/
func (u *ui) loadTweaks() {
	if u.tweaksOK {
		return
	}
	u.tweaksOK = true
	go func() {
		done := u.busy("Reading the built-in tweaks…")
		defer done()
		res, err := core.ListTweaks(context.Background(), core.ListTweaksRequest{
			Request: u.request(),
		})
		if err != nil {
			u.report("Read the built-in tweaks", err)
			return
		}
		fyne.Do(func() {
			u.tweaks = res
			u.rebuild()
		})
	}()
}

/*
loadCompat runs the round-trip compatibility check, once per process (R3.4).

The Overview's Tools card is the only place a user is told whether the compiler
they installed can actually read this install's files, and spec 003 left that
line reading "unknown -- not checked yet" even after a build had measured it.
The check is the measurement: it decompiles two known game files, recompiles
them and compares the bytes. Two MBINCompiler processes and about two seconds,
so it runs in the background behind the busy strip and the card fills in.

Once per process, not once per section: the answer changes when the game or the
compiler changes, and both of those mean a restart or an explicit action that
clears this itself.
*/
func (u *ui) loadCompat() {
	if u.compatOK || u.compatError != "" {
		return
	}
	if !u.status.Install.Found || !u.status.Compiler.Installed {
		return // nothing to check against, or nothing to check with
	}
	u.compatOK = true
	go func() {
		done := u.busy("Checking compiler compatibility…")
		defer done()
		res, err := core.ToolCheck(context.Background(), core.ToolCheckRequest{
			Request: u.request(),
		})
		fyne.Do(func() {
			if err != nil {
				// Not a banner: a compatibility check that could not run is a
				// line on a card, not a failed operation the user asked for.
				u.compatError = err.Error()
			} else {
				u.compat = res
			}
			u.refresh()
		})
	}()
}

// loadTools fills the installed-compiler table.
func (u *ui) loadTools() {
	if u.toolsOK {
		return
	}
	u.toolsOK = true
	go func() {
		done := u.busy("Reading the installed compilers…")
		defer done()
		res, err := core.ListTools(context.Background(), core.ListToolsRequest{Request: u.request()})
		if err != nil {
			u.report("Read the installed compilers", err)
			return
		}
		fyne.Do(func() {
			u.tools = res
			u.refresh()
		})
	}()
}

// loadCache measures the caches. Not loaded on arrival at Tools: it walks the
// pristine tree, which is a hundred files and two directory levels, and the
// card says "not measured" until asked.
func (u *ui) loadCache() {
	u.cacheOK = true
	go func() {
		done := u.busy("Measuring the caches…")
		defer done()
		res, err := core.CacheInfo(context.Background(), core.CacheInfoRequest{Request: u.request()})
		if err != nil {
			u.report("Measure the caches", err)
			return
		}
		fyne.Do(func() {
			u.cache = res
			u.refresh()
		})
	}()
}

// loadReleases asks GitHub what exists. Explicit, never on arrival: a window
// that contacts the network because a section was opened is a window that
// contacts the network when someone is on a metered connection.
func (u *ui) loadReleases() {
	u.perform("Checking for MBINCompiler releases…", func(ctx context.Context) error {
		res, err := core.ListReleases(ctx, core.ListReleasesRequest{Request: u.request()})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			u.releases, u.releasesOK = res, true
			if res.Warning != "" {
				u.flash("GitHub could not be reached, so this listing came from the cache: "+
					res.Warning, StatusWarn)
			}
			u.refresh()
			u.showReleases()
		})
		return nil
	})
}

// loadReport reads the last build's report.json.
func (u *ui) loadReport() {
	if u.lastReportOK {
		return
	}
	u.lastReportOK = true
	go func() {
		done := u.busy("Reading the last build report…")
		defer done()
		res, err := core.Report(context.Background(), core.ReportRequest{Request: u.request()})
		if err != nil {
			// Never having built is the ordinary state of a new install, not a
			// failure worth a red banner. The section says so instead.
			fyne.Do(func() {
				u.lastReport = core.ReportResult{}
				u.lastReportErr = err.Error()
				u.refresh()
			})
			return
		}
		fyne.Do(func() {
			u.lastReport, u.lastReportErr = res, ""
			u.refresh()
		})
	}()
}

// --- the build -------------------------------------------------------------

// logPumpInterval is how often the log pane redraws while a build runs.
//
// Per line would be a hundred fyne.Do calls a second on a cold build and a
// window slower than the thing it is watching. A tenth of a second is faster
// than anyone reads and slow enough to cost nothing.
const logPumpInterval = 100 * time.Millisecond

/*
buildEvents bridges core.Events into the Build section (R4.1).

Every level reaches the log, debug included: the per-edit lines are the build's
product, not diagnostics, and dropping them would leave the pane showing four
lines for a three-minute run. Warnings are coloured in the pane and are
deliberately NOT flashed one by one -- a build with sixty skipped keys would
leave the banner slot flickering for the whole run, and one banner summarises
the result when it finishes.

Called from core's worker goroutines. The log model takes its own lock; the step
list is touched on the UI thread through fyne.Do, because it is read by the
widgets.
*/
func (u *ui) buildEvents() core.Events {
	return core.Events{
		Log: func(level core.Level, msg string) {
			u.run.log.append(level, msg)
		},
		Progress: func(p core.Progress) {
			step, ok := stepFor(p.What)
			if !ok {
				return
			}
			note := p.What
			if p.Total > 0 && p.Step > 0 && isItemProgress(p.What) {
				note = fmt.Sprintf("%s (%d of %d)", p.What, p.Step, p.Total)
			}
			fyne.Do(func() {
				u.run.advance(step, note)
				u.drawSteps()
			})
		},
	}
}

// isItemProgress says whether a progress message counts items rather than
// announcing a phase, which decides whether "3 of 100" means anything.
func isItemProgress(what string) bool {
	switch what {
	case "checking compiler compatibility", "loading mod scripts",
		"indexing the game archives", "preparing pristine game files", "merging and compiling":
		return false
	}
	return true
}

/*
startBuild runs core.Build with a cancellable context (R4.3).

The whole run is one goroutine and one context. Cancel closes the context, the
compiler runner kills the MBINCompiler processes it started, and build.Run
restores the previous output before returning — so a cancelled build leaves the
workspace holding the mod folder that was there before, not a half-built one.
*/
func (u *ui) startBuild(recache bool) {
	if u.working() {
		u.flash("A build is already running. Cancel it first.", StatusWarn)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.run.reset()
	u.run.cancel = cancel
	u.drawSteps()
	u.drawControls()
	u.redrawStatus()

	req := core.BuildRequest{
		Request: core.Request{ConfigPath: u.configPath, Events: u.buildEvents()},
		Recache: recache,
	}

	go func() {
		done := u.busy("Building…")
		defer done()
		defer cancel()

		stop := u.startLogPump(ctx)
		res, err := core.Build(ctx, req)
		stop()

		fyne.Do(func() { u.finishBuild(res, err) })
	}()
}

/*
startLogPump redraws the log pane on a timer until the build ends.

The returned function stops the pump, waits for it, and draws once more, so the
last lines of a build are on screen even if it finished between ticks.

The quit channel is what makes that wait terminate. Stopping the ticker does not
close its channel, so a pump woken only by the ticker and the build's context
would sit in its select for ever and the caller would block on it -- which it
did: the build finished, the goroutine waiting to report it never returned, and
the window sat there with the progress bar still going.
*/
func (u *ui) startLogPump(ctx context.Context) func() {
	ticker := time.NewTicker(logPumpInterval)
	quit := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		defer ticker.Stop()
		for {
			select {
			case <-quit:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !u.run.log.takeDirty() {
					continue
				}
				fyne.Do(u.drawLog)
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(quit)
			<-stopped
			fyne.Do(u.drawLog)
		})
	}
}

// finishBuild records the outcome and reports it in one banner (R4.1). Called
// on the UI thread.
func (u *ui) finishBuild(res core.BuildResult, err error) {
	u.run.cancel = nil
	switch {
	case u.run.cancelled || errors.Is(err, context.Canceled):
		u.run.cancelled = true
		u.run.stop(stepCancelled, "cancelled; the previous output was put back")
		u.run.summary = "The build was cancelled. Nothing was installed, and the mod folder from " +
			"the previous build is back in the workspace."
		u.run.summarySt = StatusWarn
	case err != nil:
		u.run.stop(stepFailed, err.Error())
		u.run.summary = "The build failed: " + err.Error()
		u.run.summarySt = StatusBad
	default:
		u.run.finish(stepReport, "written")
		u.run.running, u.run.finished = false, true
		u.run.summary, u.run.summarySt = buildSummary(res)
		u.lastReportOK = false
	}
	u.run.running = false
	u.flash(u.run.summary, u.run.summarySt)
	u.drawSteps()
	u.drawControls()
	u.drawLog()
	u.statusOK = false
	u.loadStatus()
	u.refresh()
}

// buildSummary is the one line a finished build gets in the banner slot.
//
// Bad when a target was dropped, because a dropped target is a game file this
// build could not ship and the mod that wanted it is not doing what it says.
// Warn when everything shipped but a mod came out PARTIAL or NOT BUILT.
func buildSummary(res core.BuildResult) (string, Status) {
	if res.Report == nil {
		return "The build finished, but wrote no report.", StatusWarn
	}
	r := res.Report
	partial := 0
	for _, m := range r.Mods {
		switch verdictStatus(m.Verdict) {
		case StatusBad, StatusWarn:
			partial++
		case StatusGood, StatusInfo:
		}
	}
	msg := fmt.Sprintf("Built %d file(s) from %d mod(s); %d edit(s) applied, %d skipped.",
		r.Built, len(r.Mods), r.Applied, r.Skipped)
	if res.Deployed != nil {
		msg += " Installed to " + res.Deployed.Dest + "."
	}
	switch {
	case r.Dropped > 0:
		return msg + fmt.Sprintf(" %d target(s) were dropped — see the report.", r.Dropped), StatusBad
	case partial > 0:
		return msg + fmt.Sprintf(" %d mod(s) need checking — see the report.", partial), StatusWarn
	}
	return msg, StatusGood
}

// cancelBuild asks the running build to stop.
func (u *ui) cancelBuild() {
	if !u.run.running || u.run.cancel == nil {
		return
	}
	u.run.cancelled = true
	u.run.cancel()
	u.flash("Cancelling. The compiler processes are being stopped and the previous "+
		"output put back; this takes a moment.", StatusInfo)
	u.drawControls()
}
