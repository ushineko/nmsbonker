package report_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/build/report"
)

// sample is a result shaped like a real build: one of each verdict, one
// degraded file, and the header facts the report exists to carry.
func sample() *report.Result {
	return &report.Result{
		ModName:   "TEST MOD",
		Generated: time.Date(2026, 9, 11, 18, 38, 0, 0, time.UTC),
		OutputDir: "/tmp/build/TEST MOD",
		Built:     100, Dropped: 0, Applied: 465, Skipped: 115,
		CompilerVersion: "MBINCompiler v7.02.0-pre1",
		GameBuildID:     "25233815",
		Compatibility:   "compatible",
		Workers:         8,
		UnsupportedKeys: []string{"LINE_OFFSET", "VALUE_MATCH"},
		Degraded: []report.DegradedFile{{
			Internal: "METADATA/REALITY/TABLES/REWARDTABLE.MBIN",
			Mods:     []string{"BetterRewards", "Crashed Freighter Loot"},
		}},
		Mods: []report.ModResult{
			{Name: "Clean", Verdict: report.Working, Applied: 7},
			{Name: "Renamed", Verdict: report.WorkingSkipped, Applied: 12, Skipped: 6,
				NotFound: []string{"PulseRange", "ChargeTime", "PulseRange"}},
			{Name: "Adds", Verdict: report.WorkingStructural, Applied: 3},
			{Name: "BetterRewards", Verdict: report.Partial, Applied: 148},
			{Name: "Nothing", Verdict: report.NotBuilt, Skipped: 39},
		},
		Timings: report.Timings{
			Cache: 1200 * time.Millisecond, Merge: 990 * time.Millisecond,
			Compile: 22551 * time.Millisecond, Total: 5959 * time.Millisecond,
		},
	}
}

/*
R4.4: the Markdown is what the user reads after a game update.

Pinned as a whole because its value is the wording, not the data: the legend,
the "how to fix" prose and the missing-key list are the accumulated answer to
"which of my mods do I need to re-download", and a refactor that quietly drops
a paragraph has removed the point of the file.
*/
func TestMarkdownRendersTheWholeReferenceStructure(t *testing.T) {
	md := report.Markdown(sample())

	for _, want := range []string{
		"# No Man's Sky combined-mod build report",
		"- Generated: 2026-09-11 18:38",
		"- Game: No Man's Sky, Steam buildid 25233815",
		"- Compiler: MBINCompiler v7.02.0-pre1",
		"- Compiler compatibility: COMPATIBLE",
		"- Output: `MODS/TEST MOD/` (100 merged MBIN files)",
		"- Totals: 100 MBINs built, 0 dropped, 465 edits applied, 115 skipped",
		"- Unsupported script keys ignored: LINE_OFFSET, VALUE_MATCH",
		"Legend: **WORKING** all edits applied;",
		"| Mod | Status | Edits | Skipped | Notes |",
		"| Clean | WORKING | 7 | 0 |  |",
		"| Renamed | WORKING~ | 12 | 6 | keys not found: ChargeTime, PulseRange |",
		"| Adds | WORKING* | 3 | 0 | adds/removes reward or text entries — confirm in game |",
		"| BetterRewards | PARTIAL | 148 | 0 | new-entry add/remove not applied; base value edits kept |",
		"| Nothing | NOT BUILT | 0 | 39 |  |",
		"## Files where structural edits were skipped",
		"- `METADATA/REALITY/TABLES/REWARDTABLE.MBIN` — from BetterRewards, Crashed Freighter Loot",
		"## How to fix a PARTIAL / WORKING(star) mod",
		"## NOT BUILT / limited",
	} {
		require.Contains(t, md, want)
	}
}

// The summed timings exceed the wall clock whenever more than one worker is
// running, so the line says which is which. Without that a reader concludes the
// numbers are wrong. R4.4.
func TestTheTimingsLineDistinguishesWallClockFromSummedWork(t *testing.T) {
	md := report.Markdown(sample())
	require.Contains(t, md, "- Timings: 5.959s wall clock; cache 1.2s, merge 990ms and compile 22.551s "+
		"summed across 8 worker(s)")
}

// A compatibility mismatch has to reach the header of the report, because it is
// the reason every MBIN under it might be wrong. R3.4/R4.4.
func TestACompatibilityMismatchIsInTheHeader(t *testing.T) {
	r := sample()
	r.Compatibility = "mismatch"
	r.CompatibilityDetail = "rewardtable.MBIN round-tripped to a different size (4 bytes, was 1221928)"
	require.Contains(t, report.Markdown(r),
		"- Compiler compatibility: MISMATCH (rewardtable.MBIN round-tripped to a different size")
}

// A long missing-key list is elided in the table cell rather than making the
// row unreadable; the full list stays in report.json. R4.4.
func TestALongMissingKeyListIsElided(t *testing.T) {
	r := sample()
	r.Mods = []report.ModResult{{
		Name: "Many", Verdict: report.WorkingSkipped, Applied: 1, Skipped: 9,
		NotFound: []string{"a", "b", "c", "d", "e", "f", "g", "h"},
	}}
	require.Contains(t, report.Markdown(r), "| Many | WORKING~ | 1 | 9 | keys not found: a, b, c, d, e, f … |")
}

// R4.4: Write leaves both a stable path and a timestamped copy, so the GUI has
// something to open and a user can compare this build with the one before a
// game update.
func TestWriteLeavesALatestCopyAndATimestampedOne(t *testing.T) {
	dir := t.TempDir()
	paths, err := report.Write(dir, sample())
	require.NoError(t, err)

	require.Equal(t, filepath.Join(dir, "latest", "BUILD_REPORT.md"), paths.Markdown)
	require.FileExists(t, paths.Markdown)
	require.FileExists(t, paths.JSON)
	require.FileExists(t, filepath.Join(dir, "20260911-183800", "BUILD_REPORT.md"))
	require.FileExists(t, filepath.Join(dir, "20260911-183800", "report.json"))

	loaded, err := report.Load(paths.JSON)
	require.NoError(t, err)
	require.Equal(t, 100, loaded.Built)
	require.Equal(t, "TEST MOD", loaded.ModName)
	require.Len(t, loaded.Mods, 5)
}

// report.json is what the GUI and `nmsbonker report --json` read, so it has to
// stay parseable and carry the verdicts. R4.4.
func TestJSONCarriesTheVerdicts(t *testing.T) {
	b, err := report.JSON(sample())
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "TEST MOD", doc["modName"])
	require.Len(t, doc["mods"], 5)
}

// Load on a missing file reports os.ErrNotExist so the caller can say "nothing
// has been built yet" rather than "read failed". R6.3.
func TestLoadingAMissingReportIsRecognisable(t *testing.T) {
	_, err := report.Load(filepath.Join(t.TempDir(), "report.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// The three line shapes round-trip through the stored form, which is what
// `report --json` replays. R4.4.
func TestStoredLinesRenderLikeTheConsoleDid(t *testing.T) {
	lines := []report.Line{
		{Kind: "ok", Mod: "M", Detail: "A -> 1 (1x) in X.MBIN"},
		{Kind: "warn", Mod: "M", Detail: "key 'B' not found in X.MBIN scope", NotFound: "B"},
		{Kind: "info", Detail: "built X.MBIN from 1 edit-block(s)"},
	}
	var got []string
	for _, l := range lines {
		got = append(got, l.Render())
	}
	require.Equal(t, []string{
		"   OK  M: A -> 1 (1x) in X.MBIN",
		"  WARN M: key 'B' not found in X.MBIN scope",
		"       built X.MBIN from 1 edit-block(s)",
	}, got)
	require.True(t, strings.HasPrefix(got[0], "   OK  "))
}
