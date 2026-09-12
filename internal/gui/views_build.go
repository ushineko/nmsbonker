package gui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

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
visit to the section and then written to in place — drawSteps, drawLog and
drawControls below — because rebuilding the section on every line is exactly the
reflow the project forbids, and because a rebuild would throw away the user's
scroll position in the middle of the thing they were reading.
*/
func (u *ui) buildBuild() fyne.CanvasObject {
	head := heading("Build",
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
		u.selectSection("Report")
	})

	u.run.controls = []*widget.Button{build, recache}
	u.run.cancelBtn = cancel
	u.run.deployBtn = deploy
	u.run.viewBtn = view

	toolbar := container.NewHBox(build, recache, cancel, deploy, view)

	left := container.NewVBox()
	u.run.rows = nil
	for range u.run.steps {
		icon := widget.NewIcon(theme.RadioButtonIcon())
		name := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		note := widget.NewLabel("")
		note.Truncation = fyne.TextTruncateEllipsis
		u.run.rows = append(u.run.rows, stepRowWidgets{icon: icon, name: name, note: note})
		left.Add(container.NewBorder(nil, nil, container.NewHBox(icon, fixedWidth(name, 90)), nil, note))
	}

	totals := widget.NewLabel("")
	totals.Wrapping = fyne.TextWrapWord
	u.run.totals = totals

	steps := container.NewBorder(
		widget.NewLabelWithStyle("Steps", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		fixedHeight(totals, totalsHeight), nil, nil, left)

	return container.NewBorder(container.NewVBox(head, toolbar), nil, nil, nil,
		container.NewBorder(nil, nil, fixedWidth(steps, stepColumnWidth), nil, u.logPane()))
}

// logPane is the live output: a list of monospace lines that scrolls itself to
// the end unless the user has scrolled up to read something.
func (u *ui) logPane() fyne.CanvasObject {
	log := u.run.log
	list := widget.NewList(
		func() int { return log.len() },
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.TextStyle = fyne.TextStyle{Monospace: true}
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			line := log.at(i)
			l := o.(*widget.Label)
			// Importance first: SetText refreshes, and the refresh is what
			// applies it. See the note in views_table.go.
			l.Importance = importanceFor(levelStatus(line.level))
			l.SetText(line.text)
		},
	)
	u.run.list = list

	counter := widget.NewLabel("")
	counter.Importance = widget.LowImportance
	u.run.counter = counter

	follow := widget.NewCheck("Follow the tail", func(b bool) {
		u.run.follow = b
		if b {
			u.drawLog()
		}
	})
	follow.SetChecked(u.run.follow)
	u.run.followBox = follow

	copyLog := widget.NewButtonWithIcon("Copy log", theme.ContentCopyIcon(), func() {
		u.app.Clipboard().SetContent(log.text())
		u.flash(fmt.Sprintf("Copied %d line(s) to the clipboard.", log.len()), StatusGood)
	})

	bar := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Output", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(counter, follow, copyLog), widget.NewLabel(""))

	u.drawSteps()
	u.drawLog()
	u.drawControls()
	return container.NewBorder(bar, nil, nil, nil, fixedHeight(list, logPaneHeight))
}

// drawSteps writes the step list's current state into the widgets. Called on
// the UI thread, several times a second while a build runs.
func (u *ui) drawSteps() {
	if len(u.run.rows) != len(u.run.steps) {
		return // the section is not on screen, or is mid-rebuild
	}
	for i, s := range u.run.steps {
		row := u.run.rows[i]
		row.icon.SetResource(stepIcon(s.state))
		row.name.SetText(s.name)
		row.note.SetText(s.note)
		row.note.Importance = importanceFor(stepStatus(s.state))
	}
	if u.run.totals != nil {
		u.run.totals.SetText(u.buildTotals())
	}
}

// buildTotals is what the step column says underneath: nothing before a build,
// the count while one runs, and the verdict afterwards.
func (u *ui) buildTotals() string {
	switch {
	case u.run.running:
		return fmt.Sprintf("Running. %d line(s) of output so far.", u.run.log.len())
	case u.run.finished:
		return u.run.summary
	case u.lastReport.Report != nil:
		r := u.lastReport.Report
		return fmt.Sprintf("Last build %s: %d built, %d dropped, %d edit(s) applied, %d skipped.",
			humanAgo(r.Generated), r.Built, r.Dropped, r.Applied, r.Skipped)
	}
	return "Nothing has been built yet in this session."
}

/*
drawLog refreshes the log pane and decides whether to stay at the end.

The offset dance is the whole of the auto-scroll rule. Fyne's list has no
"the user scrolled" callback, so the only way to tell a user dragging the pane
up from the list re-measuring itself is to remember where the last automatic
scroll left it. See followTail.
*/
func (u *ui) drawLog() {
	if u.run.list == nil {
		return
	}
	if u.run.counter != nil {
		text := fmt.Sprintf("%d line(s)", u.run.log.len())
		if d := u.run.log.droppedCount(); d > 0 {
			text = fmt.Sprintf("%d line(s), %d older dropped", u.run.log.len(), d)
		}
		u.run.counter.SetText(text)
	}
	u.run.follow = followTail(u.run.follow, u.run.list.GetScrollOffset(), u.run.wantOffset)
	// The box is the state, so it has to say what the state is. Left alone it
	// stays ticked while the pane has quietly stopped following, and the way
	// back — untick, retick — is not one anybody would guess at.
	if u.run.followBox != nil && u.run.followBox.Checked != u.run.follow {
		u.run.followBox.SetChecked(u.run.follow)
	}
	u.run.list.Refresh()
	if !u.run.follow {
		return
	}
	u.run.list.ScrollToBottom()
	u.run.wantOffset = u.run.list.GetScrollOffset()
}

// drawControls applies R4.2: while a run is in flight the buttons that would
// start another are disabled, and Cancel is the only one that is not. Disabled,
// never hidden — a toolbar that changes width mid-build is a toolbar whose
// buttons move under the pointer.
func (u *ui) drawControls() {
	for _, b := range u.run.controls {
		if b == nil {
			continue
		}
		if u.working() {
			b.Disable()
		} else {
			b.Enable()
		}
	}
	// View report needs a report, not an idle window.
	if u.run.viewBtn != nil {
		if u.lastReport.Report == nil {
			u.run.viewBtn.Disable()
		} else {
			u.run.viewBtn.Enable()
		}
	}
	// Deploy needs that same report, since it is what gets installed, and a
	// game to install it into. The Report section's copy is gated the same way.
	if u.run.deployBtn != nil {
		if u.working() || !u.canDeployLast() {
			u.run.deployBtn.Disable()
		} else {
			u.run.deployBtn.Enable()
		}
	}
	if u.run.cancelBtn != nil {
		if u.run.running && !u.run.cancelled {
			u.run.cancelBtn.Enable()
		} else {
			u.run.cancelBtn.Disable()
		}
	}
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
	dest := "GAMEDATA/MODS/" + orNone(u.status.ModName, "COSMOS COMBINE")
	if in.ModsDir != "" {
		dest = in.ModsDir + "/" + orNone(u.status.ModName, "COSMOS COMBINE")
	}

	body := container.NewVBox(
		wrapped(lead),
		plainRow("Installs to", dest),
		plainRow("Archive", u.status.Paths.Archive),
		wrapped("A folder of that name already in the game is moved into the archive "+
			"directory under a timestamp before the new one is put in place, so the "+
			"build you had is recoverable."),
		wrapped("Not touched: your saves, the game's own .pak archives, every other mod "+
			"folder in GAMEDATA/MODS, and the scripts in your library."),
	)

	replace := widget.NewCheck(
		"Replace the GAMEDATA/MODS symlink with a real directory (the link only, never its target)",
		nil)
	if in.ModsState == steam.ModsSymlink {
		body.Add(widget.NewSeparator())
		body.Add(note("GAMEDATA/MODS is a symlink to "+in.ModsTarget+". Installing through it "+
			"would write into that directory instead of into the game. Ticking this removes "+
			"the link and creates a real directory; what the link pointed at is left exactly "+
			"as it is.", StatusWarn))
		body.Add(replace)
	}
	if in.ModSettingsOK && in.DisableAllMods {
		body.Add(note("The game currently has DisableAllMods=true, so nothing will load until "+
			"you accept the mod warning at the title screen. nmsbonker reports that switch "+
			"and does not write it.", StatusWarn))
	}

	u.confirmWithBody(title, container.NewVScroll(body), "Deploy",
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
			u.perform("Installing under GAMEDATA/MODS…", func(ctx context.Context) error {
				req := u.request()
				req.Events = core.Events{Log: func(level core.Level, msg string) {
					u.run.log.append(level, msg)
				}}
				res, err := core.Deploy(ctx, core.DeployRequest{
					Request: req, ReplaceSymlink: replaceSymlink,
				})
				fyne.Do(u.drawLog)
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
						u.flash(msg+" "+res.Warnings[0], StatusWarn)
						u.invalidate()
					})
					return nil
				}
				u.ok(msg)
				return nil
			})
		})
}
