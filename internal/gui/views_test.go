package gui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	fd "github.com/ushineko/fynedesygn"

	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/steam"
)

// --- the mod table ---------------------------------------------------------

/*
The three states a mod can be in must look different.

Enabled, disabled and "the config remembers this mod but its .lua is gone" are
one glyph and one colour apart, and the third is the one that matters: a missing
script contributes nothing to a build, and a row that renders identically to a
disabled one sends the user looking for a checkbox they already ticked.
*/
func TestTheModTableTellsEnabledDisabledAndMissingApart(t *testing.T) {
	rows := []modRow{
		{order: 1, info: core.ModInfo{Name: "Enabled", Enabled: true, Status: core.ModOK},
			author: "someone", verdict: report.Working},
		{order: 2, info: core.ModInfo{Name: "Disabled", Enabled: false, Status: core.ModOK}},
		{order: 3, info: core.ModInfo{Name: "Gone", Enabled: true, Status: core.ModMissing}},
	}

	on, onImportance := modCell(rows[0], modColOn)
	require.Equal(t, modOnGlyph, on)
	require.Equal(t, widget.SuccessImportance, onImportance)

	off, offImportance := modCell(rows[1], modColOn)
	require.Equal(t, modOffGlyph, off)
	require.NotEqual(t, onImportance, offImportance,
		"an enabled and a disabled mod must not be one glyph apart in the same colour")

	name, nameImportance := modCell(rows[2], modColName)
	require.Equal(t, "Gone", name)
	require.Equal(t, widget.WarningImportance, nameImportance,
		"a mod whose script is missing must be marked, not merely listed")
	files, _ := modCell(rows[2], modColTargets)
	require.Equal(t, "missing", files)

	// The verdict column carries the last build's answer, and dims when there
	// has not been one rather than leaving the cell blank.
	verdict, verdictImportance := modCell(rows[0], modColVerdict)
	require.Equal(t, report.Working, verdict)
	require.Equal(t, widget.SuccessImportance, verdictImportance)
	none, noneImportance := modCell(rows[1], modColVerdict)
	require.Equal(t, "—", none)
	require.Equal(t, widget.LowImportance, noneImportance)
}

// Every verdict the report can produce must be ranked. An unranked one renders
// in the ordinary colour, which says "this mod is fine" about a mod that is not.
func TestEveryReportVerdictIsRanked(t *testing.T) {
	require.Equal(t, fd.StatusGood, verdictStatus(report.Working))
	require.Equal(t, fd.StatusWarn, verdictStatus(report.WorkingSkipped))
	require.Equal(t, fd.StatusWarn, verdictStatus(report.WorkingStructural))
	require.Equal(t, fd.StatusBad, verdictStatus(report.Partial))
	require.Equal(t, fd.StatusBad, verdictStatus(report.NotBuilt))
	require.Equal(t, fd.StatusInfo, verdictStatus(""))
}

// modRows joins three sources by mod name. A verdict landing on the wrong row
// would tell the user a working mod is broken, and the join is the only place
// that can go wrong.
func TestModRowsJoinTheConfigTheScriptAndTheLastBuild(t *testing.T) {
	u := testUI(t)
	u.mods = core.ListModsResult{Mods: []core.ModInfo{
		{Name: "First", Enabled: true, Status: core.ModOK},
		{Name: "Second", Enabled: false, Status: core.ModOK},
	}}
	u.checks = map[string]core.ModCheck{
		"Second": {Name: "Second", Author: "an author", Targets: []string{"a.mbin", "b.mbin"}},
	}
	u.lastReport = core.ReportResult{Report: &report.Result{
		Mods: []report.ModResult{{Name: "Second", Verdict: report.Partial}},
	}}

	rows := u.modRows()
	require.Len(t, rows, 2)
	require.Equal(t, 1, rows[0].order)
	require.Empty(t, rows[0].verdict, "a mod the last build did not mention has no verdict")
	require.Equal(t, 2, rows[1].order)
	require.Equal(t, "an author", rows[1].author)
	require.Equal(t, 2, rows[1].targets)
	require.Equal(t, report.Partial, rows[1].verdict)
}

// --- the report ------------------------------------------------------------

// The Notes column must say the same thing the CLI and BUILD_REPORT.md say. One
// report explained three different ways is three different decisions about
// whether to re-download a mod.
func TestReportNotesMatchTheOtherRenderings(t *testing.T) {
	require.Equal(t, "structural add/remove skipped; value edits kept",
		modNote(report.ModResult{Verdict: report.Partial}))
	require.Contains(t, modNote(report.ModResult{
		Verdict: report.WorkingSkipped, NotFound: []string{"KeyA", "KeyA", "KeyB"},
	}), "keys not found: KeyA, KeyB", "a repeated key is named once")
	require.Empty(t, modNote(report.ModResult{Verdict: report.Working}),
		"a mod that worked needs no note")
}

// --- settings --------------------------------------------------------------

// settingsUI is a window pointed at a throwaway config file, so the Settings
// tests write nothing the developer will find later.
func settingsUI(t *testing.T) (*ui, string) {
	t.Helper()
	u := testUI(t)
	path := filepath.Join(t.TempDir(), "config.json")
	u.configPath = path

	res, err := core.ConfigShow(t.Context(), core.ConfigShowRequest{
		Request: core.Request{ConfigPath: path},
	})
	require.NoError(t, err)
	u.settings, u.configOK = res, true
	return u, path
}

/*
Save writes through core, so the window and `nmsbonker config set` cannot
disagree about what a valid value is or where it goes.

This drives the real button on the real form and then reads the file back with
core, which is the whole contract: the GUI holds no settings of its own. It is
a synchronous assertion because a window with no content pane runs its
operations on the calling goroutine -- see shell.Perform.
*/
func TestSavingTheSettingsFormWritesTheConfigFile(t *testing.T) {
	u, path := settingsUI(t)
	f := u.newSettingsForm()

	f.modName.SetText("TEST BONK")
	f.jobs.SetSelected("3")
	test.Tap(f.save)

	require.FileExists(t, path, "Save must write the settings file")
	res, err := core.ConfigShow(t.Context(), core.ConfigShowRequest{
		Request: core.Request{ConfigPath: path},
	})
	require.NoError(t, err)
	values := map[string]string{}
	for _, e := range res.Entries {
		values[e.Key] = e.Value
	}
	require.Equal(t, "TEST BONK", values["mod_name"])
	require.Equal(t, "3", values["parallel"])
}

// Revert puts the fields back to what the file says, there and then. Reverting
// by scheduling a reload leaves the typed values on screen until it lands,
// which is the opposite of what the button promises.
func TestRevertingTheSettingsFormRestoresTheLoadedValues(t *testing.T) {
	u, _ := settingsUI(t)
	f := u.newSettingsForm()
	original := f.modName.Text
	require.Equal(t, config.DefaultModName, original)

	f.modName.SetText("something else")
	f.flavor.SetSelected(config.FlavorSelfContained)
	test.Tap(f.revert)

	require.Equal(t, original, f.modName.Text)
	require.Equal(t, config.FlavorAuto, f.flavor.Selected)
}

// Saving with nothing changed must write nothing: rewriting every key on every
// Save would rewrite values this build does not understand.
func TestSavingAnUnchangedFormWritesNothing(t *testing.T) {
	u, path := settingsUI(t)
	f := u.newSettingsForm()

	test.Tap(f.save)

	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist,
		"an unchanged form must not create a settings file")
}

// The parallel Select round-trips through the stored value, where 0 means
// "decide from the CPU count" rather than "no workers".
func TestParallelAutoRoundTrips(t *testing.T) {
	require.Equal(t, parallelAuto, parallelValue("0"))
	require.Equal(t, parallelAuto, parallelValue(""))
	require.Equal(t, "4", parallelValue("4"))
	require.Equal(t, "0", parallelSetting(parallelAuto))
	require.Equal(t, "4", parallelSetting("4"))
	require.Contains(t, parallelOptions(), parallelAuto)
}

// --- status bar and cards --------------------------------------------------

// The status bar is drawn from every section and before anything has loaded.
// It must not panic on a zero-valued window, which is exactly the state it is
// in for the first few hundred milliseconds of every run.
func TestTheStatusBarDrawsBeforeAnythingHasLoaded(t *testing.T) {
	u := testUI(t)
	require.NotPanics(t, func() { u.statusSegments() })

	u.statusOK = true
	u.status = core.StatusResult{ModName: "COSMOS COMBINE"}
	require.NotPanics(t, func() { u.statusSegments() })
}

// A compatibility verdict core can return but the window cannot rank would be
// painted as an ordinary fact, which is the one thing it is not.
func TestEveryCompatibilityVerdictIsRanked(t *testing.T) {
	require.Equal(t, fd.StatusGood, compatStatus(core.CompatOK))
	require.Equal(t, fd.StatusWarn, compatStatus(core.CompatMismatch))
	require.Equal(t, fd.StatusBad, compatStatus(core.CompatNoCompiler))
	require.NotEmpty(t, compatText(core.CompatOK))
	require.NotEmpty(t, compatText(core.CompatMismatch))
	require.NotEmpty(t, compatText("unknown"))
}

// The symlinked GAMEDATA/MODS is the state where the obvious action does the
// wrong thing, so it must rank as a warning rather than as a fact.
func TestASymlinkedModsDirectoryIsAWarning(t *testing.T) {
	require.Equal(t, fd.StatusGood, modsStateStatus("dir"))
	require.Equal(t, fd.StatusWarn, modsStateStatus("symlink"))
	require.Equal(t, fd.StatusInfo, modsStateStatus("absent"))
}

// --- the way in to GAMEDATA/MODS --------------------------------------------

/*
The way in to the mod directory must be dead when there is no directory.

`absent` is the ordinary state of a game that has never had a mod installed. A
live button there hands a path that does not exist to the desktop, and what
comes back is a file manager's own "no such directory" with nothing to connect
it to the row it came from. It also has to switch itself back on once a deploy
has made the directory, which is why both states are asserted rather than only
the dead one.
*/
func TestTheOverviewOnlyOffersToOpenTheModsFolderWhenThereIsOne(t *testing.T) {
	u := testUI(t)
	u.statusOK = true
	u.status.Install = core.InstallSummary{
		Found:     true,
		Dir:       "/games/No Man's Sky",
		ModsDir:   "/games/No Man's Sky/GAMEDATA/MODS",
		ModsState: steam.ModsAbsent,
	}

	require.True(t, buttonNamed(t, u.installCard(), "Open").Disabled(),
		"an absent GAMEDATA/MODS is nothing to open")
	require.True(t, buttonNamed(t, u.gameActions(), "Open mods folder").Disabled())

	u.status.Install.ModsState = steam.ModsDir
	require.False(t, buttonNamed(t, u.installCard(), "Open").Disabled())
	require.False(t, buttonNamed(t, u.gameActions(), "Open mods folder").Disabled())

	// A symlinked MODS is still what the game reads its mods through, so the
	// way in stays open; the row beside it is what says it is a link.
	u.status.Install.ModsState = steam.ModsSymlink
	u.status.Install.ModsTarget = "/elsewhere/mods"
	require.False(t, buttonNamed(t, u.installCard(), "Open").Disabled())
	require.False(t, buttonNamed(t, u.gameActions(), "Open mods folder").Disabled())
}

/*
Report reaches the game's mod directory as well as the build's.

Report is where somebody stands when a mod did not load: the table says what the
build made of it, and the only way to tell whether that reached the game is to
look in GAMEDATA/MODS. The two folders are a deploy apart and the strip has to
offer both, so this pins the new button beside the existing one rather than in
place of it — and pins that it is disabled, not missing, when there is no
directory, because a strip that grows a button after the first deploy moves
every button beside it.
*/
func TestTheReportReachesTheGamesModsFolderAndDisablesItWhenAbsent(t *testing.T) {
	u := testUI(t)
	u.lastReport = core.ReportResult{Report: &report.Result{
		OutputDir: "/workspace/COSMOS COMBINE",
	}}
	u.status.Install = core.InstallSummary{
		Found:     true,
		ModsDir:   "/games/No Man's Sky/GAMEDATA/MODS",
		ModsState: steam.ModsAbsent,
	}

	actions := u.reportActions()
	require.False(t, buttonNamed(t, actions, "Open output folder").Disabled(),
		"the build's own output is there whatever the game looks like")
	require.True(t, buttonNamed(t, actions, "Open game mods folder").Disabled())

	u.status.Install.ModsState = steam.ModsDir
	require.False(t, buttonNamed(t, u.reportActions(), "Open game mods folder").Disabled())
}

// A build that dropped a target is a bad outcome, not a mixed one: a dropped
// target is a game file the build could not ship.
func TestABuildSummaryRanksDroppedTargetsWorstOfAll(t *testing.T) {
	_, st := buildSummary(core.BuildResult{Report: &report.Result{
		Built: 10, Dropped: 1, Mods: []report.ModResult{{Verdict: report.Working}},
	}})
	require.Equal(t, fd.StatusBad, st)

	_, st = buildSummary(core.BuildResult{Report: &report.Result{
		Built: 10, Mods: []report.ModResult{{Verdict: report.WorkingSkipped}},
	}})
	require.Equal(t, fd.StatusWarn, st, "a mod that needs checking is a warning")

	msg, st := buildSummary(core.BuildResult{Report: &report.Result{
		Built: 10, Applied: 40, Mods: []report.ModResult{{Verdict: report.Working}},
	}})
	require.Equal(t, fd.StatusGood, st)
	require.Contains(t, msg, "10 file(s)")
}
