package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

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
		heading("Overview", "What this machine has, and what a build would produce."),
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
		return card("Game", widget.NewLabel("Reading the install…"))
	}
	if !in.Found {
		return card("Game", u.noGameBlock(in.Error))
	}

	mods := in.ModsState
	if in.ModsTarget != "" {
		// "->" rather than an arrow glyph: the font Fyne bundles has no U+2192
		// and draws a replacement box for it, which in the one row that warns
		// about a symlinked GAMEDATA/MODS reads as a rendering fault. The CLI
		// spells it the same way.
		mods += " -> " + in.ModsTarget
	}
	rows := []fyne.CanvasObject{
		plainRow("Directory", in.Dir),
		plainRow("Found by", in.Source),
		factRow("Steam buildid", orNone(in.BuildID, "no appmanifest"), buildIDStatus(in.BuildID)),
		plainRow("Archives", fmt.Sprintf("%d .pak in %s", in.PakCount, in.PCBanksDir)),
		factRow("GAMEDATA/MODS", mods, modsStateStatus(in.ModsState)),
	}
	if in.ModsState == steam.ModsSymlink {
		// R6.2: the fact, the consequence, and the action, in that order. The
		// text describes the symlink and nothing else -- whatever put it there
		// is not this tool's business and naming a guess would be worse than
		// saying nothing.
		block, replace := action("GAMEDATA/MODS is a symlink",
			"It points at "+in.ModsTarget+". The game reads mods through the link, so "+
				"installing here would write into that directory rather than into the game. "+
				"Replacing the link removes the link only — never what it points at — and "+
				"creates a real directory in its place.",
			"Replace symlink and deploy…", true, func() { u.replaceSymlinkAndDeploy() })
		u.gate(replace)
		rows = append(rows, block)
	}
	if in.ModSettingsOK {
		rows = append(rows, factRow("DisableAllMods", fmt.Sprintf("%t", in.DisableAllMods),
			disableAllStatus(in.DisableAllMods)))
		if in.DisableAllMods {
			rows = append(rows, note(
				"The game's own switch is off, so nothing under GAMEDATA/MODS will load until you "+
					"accept the mod warning at the title screen. nmsbonker reports this switch and "+
					"does not write it.", StatusWarn))
		}
	} else {
		rows = append(rows, plainRow("Mod settings", "absent ("+in.ModSettingsPath+")"))
	}
	return card("Game", rows...)
}

// noGameBlock is the first-run state: no install, and what to do about it.
func (u *ui) noGameBlock(reason string) fyne.CanvasObject {
	u.loadDetect()

	body := container.NewVBox(
		factRow("Game", "not found", StatusBad),
		note(reason+". Steam's own manifests were read; the game may be on a drive Steam no "+
			"longer lists, or installed outside Steam. Set the directory below and it is "+
			"remembered in config.json, where the command line reads it too.", StatusInfo),
	)

	dir := widget.NewEntry()
	dir.SetPlaceHolder("the No Man's Sky directory, the one holding GAMEDATA")
	save := widget.NewButtonWithIcon("Use this directory", theme.ConfirmIcon(), func() {
		u.setConfig("game_dir", dir.Text, "Game directory")
	})
	body.Add(container.NewBorder(nil, nil, nil, save, u.withBrowse(dir, true)))

	if u.detectOK && len(u.detect.Candidates) > 0 {
		var t detailTable
		t.header("Steam library", "Game directory", "Verdict")
		for _, c := range u.detect.Candidates {
			lib := c.LibraryDir
			if lib == "" {
				lib = c.Root
			}
			if lib == "" {
				lib = "(explicit game directory)"
			}
			verdict, st := "ok", StatusGood
			if c.Reason != "" {
				verdict, st = "rejected: "+c.Reason, StatusWarn
			}
			t.row(st, lib, c.GameDir, verdict)
		}
		body.Add(widget.NewSeparator())
		body.Add(widget.NewLabelWithStyle("Where nmsbonker looked",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		body.Add(fixedHeight(t.widget(), 160))
	}
	return body
}

// toolsCard is the compiler side: what is installed, whether it agrees with
// this install's files, and how fresh the index is.
func (u *ui) toolsCard() fyne.CanvasObject {
	c := u.status.Compiler
	if !u.statusOK {
		return card("Tools", widget.NewLabel("Reading the installed tools…"))
	}
	if !c.Installed {
		block, install := action("Install MBINCompiler",
			"Nothing can be built without it. It is downloaded from the MBINCompiler releases "+
				"on GitHub into the tools directory; the game is not touched, and nothing is "+
				"installed system-wide.",
			"Install", false, func() { u.ensureTools() })
		install.Importance = widget.HighImportance
		u.gate(install)
		return card("Tools", factRow("MBINCompiler", "not installed", StatusBad), block)
	}

	idx := u.status.PakIndex
	indexText, indexStatus := "not built yet", StatusInfo
	switch {
	case !idx.Exists:
	case idx.Stale == 0 && idx.Missing == 0:
		indexText = fmt.Sprintf("current: %d paks, %d files, %s",
			idx.Paks, idx.Files, humanAgo(idx.Written))
		indexStatus = StatusGood
	default:
		indexText = fmt.Sprintf("stale: %d pak(s) changed, %d gone, %s",
			idx.Stale, idx.Missing, humanAgo(idx.Written))
		indexStatus = StatusWarn
	}

	compat, compatSt := u.compatLine()
	return card("Tools",
		factRow("MBINCompiler", c.Tag+" ("+c.Flavor+")", StatusGood),
		plainRow("Reports", c.Version),
		factRow("Compatibility", compat, compatSt),
		factRow(".NET 10 runtime", dotnetText(c.Dotnet10, c.Flavor), dotnetStatus(c.Dotnet10, c.Flavor)),
		factRow("Pak index", indexText, indexStatus),
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
func (u *ui) compatLine() (string, Status) {
	switch {
	case u.compatError != "":
		return "could not be checked: " + u.compatError, StatusInfo
	case u.compat.Status == "":
		return "checking…", StatusInfo
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
		plainRow("Library", u.status.Paths.Library),
		factRow("Enabled", fmt.Sprintf("%d of %d mod(s)", enabled, len(u.mods.Mods)),
			enabledStatus(enabled, len(u.mods.Mods))),
	}
	if missing > 0 {
		rows = append(rows, factRow("Missing scripts", fmt.Sprintf("%d", missing), StatusWarn))
	}
	rows = append(rows, plainRow("Output folder", orNone(u.status.ModName, "COSMOS COMBINE")))
	rows = append(rows, plainRow("Save backup", u.saveBackupText()))

	switch {
	case !u.lastReportOK:
		rows = append(rows, plainRow("Last build", "reading…"))
	case u.lastReport.Report == nil:
		rows = append(rows, factRow("Last build", "never", StatusInfo))
		rows = append(rows, note("Nothing has been built yet. Build merges every enabled mod "+
			"into one folder in the workspace; it does not touch the game until you deploy.",
			StatusInfo))
	default:
		r := u.lastReport.Report
		rows = append(rows,
			plainRow("Last build", humanAgo(r.Generated)+" ("+r.Generated.Format("2006-01-02 15:04")+")"),
			factRow("Result", fmt.Sprintf("%d built, %d dropped, %d edits applied, %d skipped",
				r.Built, r.Dropped, r.Applied, r.Skipped), builtStatus(r.Dropped)),
			plainRow("Verdicts", verdictCounts(r.Mods)),
			amountFlagsRow(r.Audit),
		)
	}
	return card("Mod library", rows...)
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
		return plainRow("Amount audit", "no reward table in that build")
	case len(a.Flags) == 0:
		return factRow("Amount audit", "no amount flags", StatusGood)
	default:
		return factRow("Amount audit", fmt.Sprintf("%d amount flag(s) — see Report", len(a.Flags)),
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

// overviewActions is the bottom strip: the two things this window is for, and
// a reload.
func (u *ui) overviewActions() fyne.CanvasObject {
	build := widget.NewButtonWithIcon("Build", theme.MediaPlayIcon(), func() {
		u.selectSection("Build")
		u.startBuild(false, false, false)
	})
	build.Importance = widget.HighImportance

	deploy := widget.NewButtonWithIcon("Build and deploy…", theme.DownloadIcon(), func() {
		u.confirmDeploy("Build and deploy?",
			"Every enabled mod is merged into one folder and then installed into the game.",
			func(replaceSymlink bool) {
				u.selectSection("Build")
				u.startBuild(true, false, replaceSymlink)
			})
	})
	deploy.Importance = widget.DangerImportance

	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { u.invalidate() })
	u.gate(build, deploy, refresh)
	if !u.status.Install.Found || !u.status.Compiler.Installed {
		// Nothing to build against, or nothing to build with. Disabled rather
		// than hidden, so the window has the same shape once it is fixed.
		build.Disable()
		deploy.Disable()
	}
	return container.NewVBox(widget.NewSeparator(),
		container.NewHBox(build, deploy, refresh),
		u.gameActions())
}

// --- small helpers shared by the cards --------------------------------------

// note is a wrapped paragraph under a row, for the things a value cannot say on
// its own.
func note(text string, st Status) fyne.CanvasObject {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	switch st {
	case StatusWarn:
		l.Importance = widget.WarningImportance
	case StatusBad:
		l.Importance = widget.DangerImportance
	case StatusGood, StatusInfo:
		l.Importance = widget.LowImportance
	}
	return container.NewBorder(nil, nil, fixedWidth(widget.NewLabel(""), 190), nil, l)
}

// gate disables the buttons that start work while something is running (R4.2).
func (u *ui) gate(buttons ...*widget.Button) {
	if !u.working() {
		return
	}
	for _, b := range buttons {
		b.Disable()
	}
}

func buildIDStatus(id string) Status {
	if id == "" {
		return StatusWarn
	}
	return StatusGood
}

func disableAllStatus(disabled bool) Status {
	if disabled {
		return StatusWarn
	}
	return StatusGood
}

func enabledStatus(enabled, total int) Status {
	switch {
	case total == 0:
		return StatusWarn
	case enabled == 0:
		return StatusWarn
	}
	return StatusGood
}

func builtStatus(dropped int) Status {
	if dropped > 0 {
		return StatusBad
	}
	return StatusGood
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

func dotnetStatus(present bool, flavor string) Status {
	switch {
	case present:
		return StatusGood
	case flavor == "self-contained":
		return StatusInfo
	}
	return StatusBad
}
