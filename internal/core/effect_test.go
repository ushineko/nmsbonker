package core_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/core"
)

/*
Library hygiene signals (spec 005 R4).

These are read out of the last build's report rather than recomputed, which is
the property worth testing: the note describes the build whose output is
actually on disk, so a user who has changed the mod list since sees the verdict
of the build they installed rather than a guess about the one they have not run.
*/

// overlapping is a library script that multiplies the same reward key in the
// same file as the ChestAndLootMaterials10x built-in.
const overlapping = `NMS_MOD_DEFINITION_CONTAINER = {
["MOD_FILENAME"] = "overlap.pak",
["MODIFICATIONS"] = { { ["MBIN_CHANGE_TABLE"] = { {
  ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\REWARDTABLE.MBIN",
  ["EXML_CHANGE_TABLE"] = { {
    ["SPECIAL_KEY_WORDS"] = {"GcRewardSpecificProduct"},
    ["MATH_OPERATION"] = "*", ["REPLACE_TYPE"] = "ALL",
    ["VALUE_CHANGE_TABLE"] = { {"AmountMin", 250}, {"AmountMax", 250} }
  } }
} } } }
}`

// writeReport puts a stored build report where core reads it back from.
func writeReport(t *testing.T, root string, r *report.Result) {
	t.Helper()
	dir := filepath.Join(root, "data", "nmsbonker", "build", "reports", "latest")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	r.Generated = time.Now()
	raw, err := json.Marshal(r)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report.json"), raw, 0o600))
}

/*
R4.1: a script none of whose edits applied says so, in as many words.

"NOT BUILT, 0 applied, 39 skipped" is already in the report table, and it is
still the row nobody reads: a table of forty rows hides the one that contributes
nothing. The note is the same fact stated where it cannot be scrolled past.
*/
func TestAScriptWithNoAppliedEditsSaysSo(t *testing.T) {
	root := bare(t)
	src := writeScript(t, filepath.Join(root, "downloads"), "Inert", minimalScript)
	_, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Enabled: true})
	require.NoError(t, err)

	writeReport(t, root, &report.Result{Mods: []report.ModResult{
		{Name: "Inert", Verdict: report.NotBuilt, Applied: 0, Skipped: 39,
			NotFound: []string{"A", "A", "B"}},
	}})

	res, err := core.CheckMods(t.Context(), core.CheckModsRequest{})
	require.NoError(t, err)
	require.Len(t, res.Mods, 1)
	require.Equal(t, report.NotBuilt, res.Mods[0].Verdict)
	require.Equal(t, 0, res.Mods[0].Applied)
	require.Equal(t, 39, res.Mods[0].Skipped)
	require.Equal(t, []string{"A", "B"}, res.Mods[0].NotFound, "one entry per key, not per lookup")
	require.Equal(t, core.EffectNoEdits, res.Mods[0].Effect)
}

// A script that skipped at least as much as it applied is a script written for
// an older game version, which is a different problem from one that applied
// nothing at all.
func TestAScriptThatSkipsMoreThanItAppliesIsCalledMostlyFailing(t *testing.T) {
	root := bare(t)
	src := writeScript(t, filepath.Join(root, "downloads"), "Ageing", minimalScript)
	_, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Enabled: true})
	require.NoError(t, err)

	writeReport(t, root, &report.Result{Mods: []report.ModResult{
		{Name: "Ageing", Verdict: report.WorkingSkipped, Applied: 4, Skipped: 30},
	}})

	res, err := core.CheckMods(t.Context(), core.CheckModsRequest{})
	require.NoError(t, err)
	require.Equal(t, core.EffectMostlyFailing, res.Mods[0].Effect)
}

// A mod whose structural edits were dropped to make the file recompile is told
// so here as well as in the report's degraded-files list.
func TestAModWhoseStructuralEditsWereDroppedIsToldSo(t *testing.T) {
	root := bare(t)
	src := writeScript(t, filepath.Join(root, "downloads"), "Adds", minimalScript)
	_, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Enabled: true})
	require.NoError(t, err)

	writeReport(t, root, &report.Result{
		Mods:     []report.ModResult{{Name: "Adds", Verdict: report.Partial, Applied: 148}},
		Degraded: []report.DegradedFile{{Internal: "X.MBIN", Mods: []string{"Adds"}}},
	})

	res, err := core.CheckMods(t.Context(), core.CheckModsRequest{})
	require.NoError(t, err)
	require.Equal(t, core.EffectStructuralSkipped, res.Mods[0].Effect)
}

/*
R4.1: a library script editing the same file and keys as an enabled built-in is
named, and only while that built-in is enabled.

This is the shape of the bug spec 005 exists for -- two mods multiplying one
reward -- so the signal has to be about what will actually be built. A built-in
that is switched off multiplies nothing and reporting an overlap with it would
be noise.
*/
func TestOverlappingAnEnabledBuiltInIsReported(t *testing.T) {
	root := bare(t)
	src := writeScript(t, filepath.Join(root, "downloads"), "Compounder", overlapping)
	_, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Enabled: true})
	require.NoError(t, err)

	res, err := core.CheckMods(t.Context(), core.CheckModsRequest{})
	require.NoError(t, err)
	require.Len(t, res.Mods, 1, "the built-ins are all disabled on a fresh configuration")
	require.Empty(t, res.Mods[0].Overlaps, "nothing it overlaps with is going to be built")

	// ListMods is what writes the reconciled build order, built-ins included,
	// back to the settings file; enabling one before that has happened is
	// enabling a name the file has never heard of.
	_, err = core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	_, err = core.SetModEnabled(t.Context(), core.SetModEnabledRequest{
		Names: []string{"ChestAndLootMaterials10x"}, Enabled: true,
	})
	require.NoError(t, err)

	res, err = core.CheckMods(t.Context(), core.CheckModsRequest{})
	require.NoError(t, err)
	byName := map[string]core.ModCheck{}
	for _, m := range res.Mods {
		byName[m.Name] = m
	}
	require.Equal(t, []string{"ChestAndLootMaterials10x"}, byName["Compounder"].Overlaps)
	require.Equal(t, "overlaps built-in ChestAndLootMaterials10x", byName["Compounder"].Effect)
	require.Empty(t, byName["ChestAndLootMaterials10x"].Overlaps,
		"a built-in is not reported as overlapping a library script; the signal is "+
			"about what the library is doing on top of the tweaks")
}

// Before the first build there is no verdict to report, and the note says
// nothing rather than guessing.
func TestWithNoBuildYetThereIsNoEffectivenessNote(t *testing.T) {
	root := bare(t)
	src := writeScript(t, filepath.Join(root, "downloads"), "Fresh", minimalScript)
	_, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Enabled: true})
	require.NoError(t, err)

	res, err := core.CheckMods(t.Context(), core.CheckModsRequest{})
	require.NoError(t, err)
	require.Empty(t, res.Mods[0].Verdict)
	require.Empty(t, res.Mods[0].Effect)
}
