package gui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/logpane"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/steam"
)

// --- Build (R2.3) ----------------------------------------------------------

// The pane geometry. Fixed, all of it: a build changes what these widgets say
// several times a second, and a pane that sized itself to its contents would
// resize the window for three minutes.
const (
	stepColumnWidth = 330
	logPaneHeight   = 420
	totalsHeight    = 74
)

/*
buildBuild is the build, while it runs.

The left column is the step list, the right is the log. Both are built once per
visit to the section and then written to in place — the step list and the pane
by the library, the totals by drawTotals below — because rebuilding the section
on every line is exactly the reflow the project forbids, and because a rebuild
would throw away the user's scroll position in the middle of the thing they
were reading.
*/
func (u *ui) buildBuild() fyne.CanvasObject {
	head := widgets.Heading("Build",
		"Merge every enabled mod into one folder, recompile each file, and write the report. "+
			"Nothing reaches the game until you press Deploy.")

	build := widget.NewButtonWithIcon("Build", theme.MediaPlayIcon(), func() {
		u.startBuild(false)
	})
	build.Importance = widget.HighImportance

	recache := widget.NewButtonWithIcon("Rebuild cache", theme.ViewRefreshIcon(), func() {
		u.startBuild(true)
	})

	cancel := widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() { u.cancelBuild() })
	cancel.Importance = widget.DangerImportance

	// Deploy is its own button, not a variant of Build. Building and installing
	// are the two halves of the loop, and a button that did both hid the moment
	// between them: the one where the report says four mods need checking.
	deploy := widget.NewButtonWithIcon("Deploy…", theme.DownloadIcon(), func() { u.deployLast() })
	deploy.Importance = widget.DangerImportance

	view := widget.NewButtonWithIcon("View report", theme.DocumentIcon(), func() {
		u.sh.Select("Report")
	})

	// R4.2: while a run is in flight the buttons that would start another are
	// disabled, and Cancel is the only one that is not. Disabled, never hidden:
	// a toolbar that changes width mid-build is a toolbar whose buttons move
	// under the pointer. The shell rebuilds this section when work starts and
	// stops, so the gating is decided here, from state, every time.
	u.sh.Gate(build, recache)
	// View report needs a report, not an idle window.
	if u.lastReport.Report == nil {
		view.Disable()
	}
	// Deploy needs that same report, since it is what gets installed, and a
	// game to install it into. The Report section's copy is gated the same way.
	if u.sh.Working() || !u.canDeployLast() {
		deploy.Disable()
	}
	if !u.run.running || u.run.cancelled {
		cancel.Disable()
	}
	u.run.cancelBtn = cancel

	toolbar := container.NewHBox(build, recache, cancel, deploy, view)

	totals := widget.NewLabel("")
	totals.Wrapping = fyne.TextWrapWord
	u.run.totals = totals

	steps := container.NewBorder(
		widget.NewLabelWithStyle("Steps", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widgets.FixedHeight(totals, totalsHeight), nil, nil, u.run.steps.Widget())

	u.drawTotals()
	return container.NewBorder(container.NewVBox(head, toolbar), nil, nil, nil,
		container.NewBorder(nil, nil, widgets.FixedWidth(steps, stepColumnWidth), nil, u.logPane()))
}

// logPane is the live output: a list of monospace lines that scrolls itself to
// the end unless the user has scrolled up to read something. The pane is the
// library's; Copy reports into this window's banner slot.
func (u *ui) logPane() fyne.CanvasObject {
	return u.run.pane.Widget(logpane.Options{
		Title:     "Output",
		Height:    logPaneHeight,
		Clipboard: u.sh.App.Clipboard(),
		Flash:     u.sh.Flash,
	})
}

// drawTotals writes the step column's footer. Called on the UI thread, several
// times a second while a build runs.
func (u *ui) drawTotals() {
	if u.run.totals != nil {
		u.run.totals.SetText(u.buildTotals())
	}
}

// buildTotals is what the step column says underneath: nothing before a build,
// the count while one runs, and the verdict afterwards.
func (u *ui) buildTotals() string {
	switch {
	case u.run.running:
		return fmt.Sprintf("Running. %d line(s) of output so far.", u.run.pane.Model().Len())
	case u.run.finished:
		return u.run.summary
	case u.lastReport.Report != nil:
		r := u.lastReport.Report
		return fmt.Sprintf("Last build %s: %d built, %d dropped, %d edit(s) applied, %d skipped.",
			widgets.HumanAgo(r.Generated), r.Built, r.Dropped, r.Applied, r.Skipped)
	}
	return "Nothing has been built yet in this session."
}

// --- deploy ----------------------------------------------------------------

/*
confirmDeploy is the one dialog in this window that guards a write into the
game directory.

It states three things, in this order: where the files go, what happens to
whatever is already there, and what is not touched. The last is not politeness —
a symlinked GAMEDATA/MODS points at a directory holding the user's whole mod
tree, and "will this delete my mods" is the question a reasonable person asks
before pressing this button.
*/
func (u *ui) confirmDeploy(title, lead string, do func(replaceSymlink bool)) {
	in := u.status.Install
	dest := "GAMEDATA/MODS/" + widgets.OrNone(u.status.ModName, "COSMOS COMBINE")
	if in.ModsDir != "" {
		dest = in.ModsDir + "/" + widgets.OrNone(u.status.ModName, "COSMOS COMBINE")
	}

	body := container.NewVBox(
		widgets.Wrapped(lead),
		widgets.PlainRow("Installs to", dest),
		widgets.PlainRow("Archive", u.status.Paths.Archive),
		widgets.Wrapped("A folder of that name already in the game is moved into the archive "+
			"directory under a timestamp before the new one is put in place, so the "+
			"build you had is recoverable."),
		widgets.Wrapped("Not touched: your saves, the game's own .pak archives, every other mod "+
			"folder in GAMEDATA/MODS, and the scripts in your library."),
	)

	replace := widget.NewCheck(
		"Replace the GAMEDATA/MODS symlink with a real directory (the link only, never its target)",
		nil)
	if in.ModsState == steam.ModsSymlink {
		body.Add(widget.NewSeparator())
		body.Add(widgets.Note("GAMEDATA/MODS is a symlink to "+in.ModsTarget+". Installing through it "+
			"would write into that directory instead of into the game. Ticking this removes "+
			"the link and creates a real directory; what the link pointed at is left exactly "+
			"as it is.", fd.StatusWarn))
		body.Add(replace)
	}
	if in.ModSettingsOK && in.DisableAllMods {
		body.Add(widgets.Note("The game currently has DisableAllMods=true, so nothing will load until "+
			"you accept the mod warning at the title screen. nmsbonker reports that switch "+
			"and does not write it.", fd.StatusWarn))
	}

	dialogs.ConfirmWithBody(u.sh.Window, title, container.NewVScroll(body), "Deploy",
		func() { do(replace.Checked) }).Show()
}

// canDeployLast says whether there is a build to install and a game to install
// it into. Every Deploy button in the window is enabled on exactly this.
func (u *ui) canDeployLast() bool {
	return u.lastReport.Report != nil && u.status.Install.Found
}

// deployLast installs the build already in the workspace, without rebuilding.
//
// Its log lines go to the build log, under the build they install, so the
// Output pane reads as one story: built, then deployed.
func (u *ui) deployLast() {
	u.confirmDeploy("Deploy the last build?",
		"The folder already built in the workspace is copied into the game. Nothing is "+
			"rebuilt, so this installs exactly what the report describes.",
		func(replaceSymlink bool) {
			u.sh.Perform("Installing under GAMEDATA/MODS…", func(ctx context.Context) error {
				req := u.request()
				req.Events = core.Events{Log: func(level core.Level, msg string) {
					u.run.pane.Model().Append(logLevel(level), msg)
				}}
				res, err := core.Deploy(ctx, core.DeployRequest{
					Request: req, ReplaceSymlink: replaceSymlink,
				})
				fyne.Do(u.run.pane.Draw)
				if err != nil {
					return err
				}
				msg := fmt.Sprintf("Installed %d file(s) to %s.", res.Files, res.Dest)
				if res.Archived != "" {
					msg += " The folder that was there is in " + res.Archived + "."
				}
				if res.ReplacedSymlink != "" {
					msg += " The GAMEDATA/MODS symlink to " + res.ReplacedSymlink +
						" was removed; its target was left alone."
				}
				if len(res.Warnings) > 0 {
					fyne.Do(func() {
						u.sh.Flash(msg+" "+res.Warnings[0], fd.StatusWarn)
						u.sh.Invalidate()
					})
					return nil
				}
				fyne.Do(func() { u.sh.OK(msg) })
				return nil
			})
		})
}
