package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/table"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/nmsbonker/internal/build/audit"
	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/steam"
)

// --- Overview (R2.1) -------------------------------------------------------

/*
buildOverview is the answer to "can this machine build a mod, and what would it
build".

Three cards, in the order the pipeline needs them: the game, the tools, the
library. Each is the same facts `nmsbonker status` prints, ranked — a fact with
a verdict attached is why this is a window rather than a terminal. The two
states a new user is in, no game and no compiler, explain themselves here rather
than turning into an empty table and a failed build.
*/
func (u *ui) buildOverview() fyne.CanvasObject {
	u.loadStatus()
	u.loadMods()
	u.loadReport()
	u.loadSaves()
	// The compatibility line is a measurement, not a lookup, so it is taken
	// here rather than left saying "not checked yet" (R3.4).
	u.loadCompat()

	body := container.NewVBox(
		widgets.Heading("Overview", "What this machine has, and what a build would produce."),
		u.installCard(),
		widget.NewSeparator(),
		u.toolsCard(),
		widget.NewSeparator(),
		u.libraryCard(),
	)
	return container.NewBorder(nil, u.overviewActions(), nil, nil, container.NewVScroll(body))
}

// installCard is the game side: where it is, which build, and the state of the
// directory a deploy would write into.
func (u *ui) installCard() fyne.CanvasObject {
	in := u.status.Install
	if !u.statusOK {
		return widgets.Card("Game", widget.NewLabel("Reading the install…"))
	}
	if !in.Found {
		return widgets.Card("Game", u.noGameBlock(in.Error))
	}

	mods := in.ModsState
	if in.ModsTarget != "" {
		// "->" rather than an arrow glyph: the font Fyne bundles has no U+2192
		// and draws a replacement box for it, which in the one row that warns
		// about a symlinked GAMEDATA/MODS reads as a rendering fault. The CLI
		// spells it the same way.
		mods += " -> " + in.ModsTarget
	}
	// The row that reports the deploy target carries the way in to it. Low
	// importance because it is a convenience beside a fact, not one of the
	// operations the strip at the bottom of the section is for.
	openMods := widget.NewButtonWithIcon("Open", theme.FolderOpenIcon(),
		func() { u.openModsDir() })
	openMods.Importance = widget.LowImportance
	u.gate(openMods)
	if !modsDirOpenable(in) {
		openMods.Disable()
	}

	rows := []fyne.CanvasObject{
		widgets.PlainRow("Directory", in.Dir),
		widgets.PlainRow("Found by", in.Source),
		widgets.FactRow("Steam buildid", widgets.OrNone(in.BuildID, "no appmanifest"), buildIDStatus(in.BuildID)),
		widgets.PlainRow("Archives", fmt.Sprintf("%d .pak in %s", in.PakCount, in.PCBanksDir)),
		widgets.RowWithAction(widgets.FactRow("GAMEDATA/MODS", mods, modsStateStatus(in.ModsState)), openMods),
	}
	if in.ModsState == steam.ModsSymlink {
		// R6.2: the fact, the consequence, and the action, in that order. The
		// text describes the symlink and nothing else -- whatever put it there
		// is not this tool's business and naming a guess would be worse than
		// saying nothing.
		block, replace := widgets.Action("GAMEDATA/MODS is a symlink",
			"It points at "+in.ModsTarget+". The game reads mods through the link, so "+
				"installing here would write into that directory rather than into the game. "+
				"Replacing the link removes the link only — never what it points at — and "+
				"creates a real directory in its place.",
			"Replace symlink and deploy…", true, func() { u.replaceSymlinkAndDeploy() })
		u.gate(replace)
		rows = append(rows, block)
	}
	if in.ModSettingsOK {
		rows = append(rows, widgets.FactRow("DisableAllMods", fmt.Sprintf("%t", in.DisableAllMods),
			disableAllStatus(in.DisableAllMods)))
		if in.DisableAllMods {
			rows = append(rows, widgets.Note(
				"The game's own switch is off, so nothing under GAMEDATA/MODS will load until you "+
					"accept the mod warning at the title screen. nmsbonker reports this switch and "+
					"does not write it.", fd.StatusWarn))
		}
	} else {
		rows = append(rows, widgets.PlainRow("Mod settings", "absent ("+in.ModSettingsPath+")"))
	}
	return widgets.Card("Game", rows...)
}

// noGameBlock is the first-run state: no install, and what to do about it.
func (u *ui) noGameBlock(reason string) fyne.CanvasObject {
	u.loadDetect()

	body := container.NewVBox(
		widgets.FactRow("Game", "not found", fd.StatusBad),
		widgets.Note(reason+". Steam's own manifests were read; the game may be on a drive Steam no "+
			"longer lists, or installed outside Steam. Set the directory below and it is "+
			"remembered in config.json, where the command line reads it too.", fd.StatusInfo),
	)

	dir := widget.NewEntry()
	dir.SetPlaceHolder("the No Man's Sky directory, the one holding GAMEDATA")
	save := widget.NewButtonWithIcon("Use this directory", theme.ConfirmIcon(), func() {
		u.setConfig("game_dir", dir.Text, "Game directory")
	})
	body.Add(container.NewBorder(nil, nil, nil, save, dialogs.WithBrowse(u.win, dir, true)))

	if u.detectOK && len(u.detect.Candidates) > 0 {
		t := table.New()
		t.Header("Steam library", "Game directory", "Verdict")
		for _, c := range u.detect.Candidates {
			lib := c.LibraryDir
			if lib == "" {
				lib = c.Root
			}
			if lib == "" {
				lib = "(explicit game directory)"
			}
			verdict, st := "ok", fd.StatusGood
			if c.Reason != "" {
				verdict, st = "rejected: "+c.Reason, fd.StatusWarn
			}
			t.Row(st, lib, c.GameDir, verdict)
		}
		body.Add(widget.NewSeparator())
		body.Add(widget.NewLabelWithStyle("Where nmsbonker looked",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		body.Add(widgets.FixedHeight(t.Widget(), 160))
	}
	return body
}

// toolsCard is the compiler side: what is installed, whether it agrees with
// this install's files, and how fresh the index is.
func (u *ui) toolsCard() fyne.CanvasObject {
	c := u.status.Compiler
	if !u.statusOK {
		return widgets.Card("Tools", widget.NewLabel("Reading the installed tools…"))
	}
	if !c.Installed {
		block, install := widgets.Action("Install MBINCompiler",
			"Nothing can be built without it. It is downloaded from the MBINCompiler releases "+
				"on GitHub into the tools directory; the game is not touched, and nothing is "+
				"installed system-wide.",
			"Install", false, func() { u.ensureTools() })
		install.Importance = widget.HighImportance
		u.gate(install)
		return widgets.Card("Tools", widgets.FactRow("MBINCompiler", "not installed", fd.StatusBad), block)
	}

	idx := u.status.PakIndex
	indexText, indexStatus := "not built yet", fd.StatusInfo
	switch {
	case !idx.Exists:
	case idx.Stale == 0 && idx.Missing == 0:
		indexText = fmt.Sprintf("current: %d paks, %d files, %s",
			idx.Paks, idx.Files, widgets.HumanAgo(idx.Written))
		indexStatus = fd.StatusGood
	default:
		indexText = fmt.Sprintf("stale: %d pak(s) changed, %d gone, %s",
			idx.Stale, idx.Missing, widgets.HumanAgo(idx.Written))
		indexStatus = fd.StatusWarn
	}

	compat, compatSt := u.compatLine()
	return widgets.Card("Tools",
		widgets.FactRow("MBINCompiler", c.Tag+" ("+c.Flavor+")", fd.StatusGood),
		widgets.PlainRow("Reports", c.Version),
		widgets.FactRow("Compatibility", compat, compatSt),
		widgets.FactRow(".NET 10 runtime", dotnetText(c.Dotnet10, c.Flavor), dotnetStatus(c.Dotnet10, c.Flavor)),
		widgets.FactRow("Pak index", indexText, indexStatus),
	)
}

/*
compatLine is the Compatibility row, from the round-trip check (R3.4).

Spec 001 derived this from version strings and could not answer the question:
the game's own MBINs carry no libMBIN version. Spec 002 replaced it with a
measurement -- decompile two known game files, recompile them, compare the bytes
-- and this row now reports that measurement rather than the guess. While it is
running the row says so, because "checking" and "unknown" are different states
and a card that says "unknown" for two seconds and then changes its mind reads
as a card that was wrong.
*/
func (u *ui) compatLine() (string, fd.Status) {
	switch {
	case u.compatError != "":
		return "could not be checked: " + u.compatError, fd.StatusInfo
	case u.compat.Status == "":
		return "checking…", fd.StatusInfo
	}
	text := compatText(u.compat.Status)
	if u.compat.Detail != "" {
		text += " (" + u.compat.Detail + ")"
	}
	return text, compatStatus(u.compat.Status)
}

// libraryCard is the mod side: how much is enabled, and what the last build
// made of it.
func (u *ui) libraryCard() fyne.CanvasObject {
	enabled, missing := 0, 0
	for _, m := range u.mods.Mods {
		if m.Enabled {
			enabled++
		}
		if m.Status == core.ModMissing {
			missing++
		}
	}
	rows := []fyne.CanvasObject{
		widgets.PlainRow("Library", u.status.Paths.Library),
		widgets.FactRow("Enabled", fmt.Sprintf("%d of %d mod(s)", enabled, len(u.mods.Mods)),
			enabledStatus(enabled, len(u.mods.Mods))),
	}
	if missing > 0 {
		rows = append(rows, widgets.FactRow("Missing scripts", fmt.Sprintf("%d", missing), fd.StatusWarn))
	}
	rows = append(rows, widgets.PlainRow("Output folder", widgets.OrNone(u.status.ModName, "COSMOS COMBINE")))
	rows = append(rows, widgets.PlainRow("Save backup", u.saveBackupText()))

	switch {
	case !u.lastReportOK:
		rows = append(rows, widgets.PlainRow("Last build", "reading…"))
	case u.lastReport.Report == nil:
		rows = append(rows, widgets.FactRow("Last build", "never", fd.StatusInfo))
		rows = append(rows, widgets.Note("Nothing has been built yet. Build merges every enabled mod "+
			"into one folder in the workspace; it does not touch the game until you deploy.",
			fd.StatusInfo))
	default:
		r := u.lastReport.Report
		rows = append(rows,
			widgets.PlainRow("Last build", widgets.HumanAgo(r.Generated)+" ("+r.Generated.Format("2006-01-02 15:04")+")"),
			widgets.FactRow("Result", fmt.Sprintf("%d built, %d dropped, %d edits applied, %d skipped",
				r.Built, r.Dropped, r.Applied, r.Skipped), builtStatus(r.Dropped)),
		)
		if n := len(r.CompilerFailures); n > 0 {
			rows = append(rows, widgets.FactRow("Compiler failures",
				fmt.Sprintf("%d file(s) MBINCompiler could not handle — see the Report", n), fd.StatusBad))
		}
		rows = append(rows,
			widgets.PlainRow("Verdicts", verdictCounts(r.Mods)),
			amountFlagsRow(r.Audit),
		)
	}
	return widgets.Card("Mod library", rows...)
}

/*
amountFlagsRow is the last build's reward-amount verdict, on Overview (R3.1).

One line, because Overview is the "can this machine build, and what did it
build" summary and the detail lives in the Report section. It is worth a line at
all because the verdict counts above it cannot say it: every mod can be WORKING
and the reward table still be wrong.
*/
func amountFlagsRow(a *audit.Result) fyne.CanvasObject {
	switch {
	case a == nil:
		return widgets.PlainRow("Amount audit", "no reward table in that build")
	case len(a.Flags) == 0:
		return widgets.FactRow("Amount audit", "no amount flags", fd.StatusGood)
	default:
		return widgets.FactRow("Amount audit", fmt.Sprintf("%d amount flag(s) — see Report", len(a.Flags)),
			auditStatus(len(a.Flags)))
	}
}

// verdictCounts summarises the report table in one line, in verdict order.
func verdictCounts(mods []report.ModResult) string {
	order := []string{report.Working, report.WorkingSkipped, report.WorkingStructural,
		report.Partial, report.NotBuilt}
	counts := map[string]int{}
	for _, m := range mods {
		counts[m.Verdict]++
	}
	out := ""
	for _, v := range order {
		if counts[v] == 0 {
			continue
		}
		if out != "" {
			out += " · "
		}
		out += fmt.Sprintf("%d %s", counts[v], v)
	}
	if out == "" {
		return "none"
	}
	return out
}

// overviewActions is the bottom strip: the two things this window is for
// (build, then install what was built) and a reload.
func (u *ui) overviewActions() fyne.CanvasObject {
	build := widget.NewButtonWithIcon("Build", theme.MediaPlayIcon(), func() {
		u.selectSection("Build")
		u.startBuild(false)
	})
	build.Importance = widget.HighImportance

	deploy := widget.NewButtonWithIcon("Deploy…", theme.DownloadIcon(), func() { u.deployLast() })
	deploy.Importance = widget.DangerImportance

	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { u.invalidate() })
	u.gate(build, deploy, refresh)
	if !u.status.Install.Found || !u.status.Compiler.Installed {
		// Nothing to build against, or nothing to build with. Disabled rather
		// than hidden, so the window has the same shape once it is fixed.
		build.Disable()
	}
	if !u.canDeployLast() {
		deploy.Disable()
	}
	return container.NewVBox(widget.NewSeparator(),
		container.NewHBox(build, deploy, refresh),
		u.gameActions())
}

// --- small helpers shared by the cards --------------------------------------

// gate disables the buttons that start work while something is running (R4.2).
func (u *ui) gate(buttons ...*widget.Button) {
	if !u.working() {
		return
	}
	for _, b := range buttons {
		b.Disable()
	}
}

func buildIDStatus(id string) fd.Status {
	if id == "" {
		return fd.StatusWarn
	}
	return fd.StatusGood
}

func disableAllStatus(disabled bool) fd.Status {
	if disabled {
		return fd.StatusWarn
	}
	return fd.StatusGood
}

func enabledStatus(enabled, total int) fd.Status {
	switch {
	case total == 0:
		return fd.StatusWarn
	case enabled == 0:
		return fd.StatusWarn
	}
	return fd.StatusGood
}

func builtStatus(dropped int) fd.Status {
	if dropped > 0 {
		return fd.StatusBad
	}
	return fd.StatusGood
}

// compatText spells out the verdict the round-trip check produced. The bare
// word is what the CLI prints; here there is room to say what it means.
func compatText(v string) string {
	switch v {
	case core.CompatOK:
		return "compatible — game files round-trip through this compiler unchanged"
	case core.CompatMismatch:
		return "mismatch — a game file did not survive a round trip; output is suspect"
	case core.CompatNoCompiler:
		return "no compiler installed"
	}
	return "unknown — not checked yet"
}

func dotnetText(present bool, flavor string) string {
	switch {
	case present:
		return "present"
	case flavor == "self-contained":
		return "absent, and not needed: this build of MBINCompiler carries its own runtime"
	}
	return "absent — the dotnet10 flavor will not start without it"
}

func dotnetStatus(present bool, flavor string) fd.Status {
	switch {
	case present:
		return fd.StatusGood
	case flavor == "self-contained":
		return fd.StatusInfo
	}
	return fd.StatusBad
}
