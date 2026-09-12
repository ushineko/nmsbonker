package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/mbin"
	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/mxml"
)

// def builds a definition with one change table over the given sources.
func def(name string, blocks []*modscript.Block, sources ...string) *modscript.Definition {
	return &modscript.Definition{
		Name: name,
		Modifications: []modscript.Modification{{
			Changes: []modscript.MBINChange{{Sources: sources, Blocks: blocks}},
		}},
	}
}

func valueBlock(key, value string) *modscript.Block {
	return &modscript.Block{
		ValueChanges: []modscript.ValueChange{{Key: key, Value: modscript.StringValue(value)}},
		HasVCT:       true,
		Raw:          map[string]any{"VALUE_CHANGE_TABLE": []any{}},
	}
}

func addBlock(text string) *modscript.Block {
	return &modscript.Block{Add: text, HasAdd: true, Raw: map[string]any{"ADD": modscript.StringValue(text)}}
}

/*
R4.1: targets are grouped by the normalised source and kept in first-seen order.

Order is the whole point. It decides which mod's edit lands last and therefore
wins, and it decides the order of the report, which is the only thing a user can
diff between two builds. Two spellings of the same file -- one Windows-style,
one not -- have to land in one target, or both mods write the same output path
and one of them silently disappears.
*/
func TestPlanGroupsSpellingsOfOneFileAndKeepsFirstSeenOrder(t *testing.T) {
	plan := NewPlan([]Script{
		{Name: "First", Enabled: true, Def: def("First", []*modscript.Block{valueBlock("A", "1")},
			`METADATA\TABLES\REWARDTABLE.MBIN`)},
		{Name: "Second", Enabled: true, Def: def("Second", []*modscript.Block{valueBlock("B", "2")},
			"GCGAMEPLAYGLOBALS.GLOBAL.MBIN")},
		{Name: "Third", Enabled: true, Def: def("Third", []*modscript.Block{valueBlock("C", "3")},
			"metadata/tables/rewardtable.mbin")},
	})

	require.Len(t, plan.Targets, 2, "the two spellings are one target")
	require.Equal(t, "METADATA/TABLES/REWARDTABLE.MBIN", plan.Targets[0].Key)
	require.Equal(t, "GCGAMEPLAYGLOBALS.GLOBAL.MBIN", plan.Targets[1].Key)
	require.Equal(t, []string{"First", "Third"}, plan.Targets[0].Mods())
	require.Equal(t, `METADATA\TABLES\REWARDTABLE.MBIN`, plan.Targets[0].Source,
		"the first spelling decides which cached file is read")
	require.Equal(t, []string{"First", "Second", "Third"}, plan.Enabled)
	require.Equal(t, 3, plan.Blocks())
}

// A change table naming several sources produces the same edits against each of
// them, which is how a mod edits 66 entity files with one block. R4.1.
func TestOneBlockAgainstSeveralSourcesBecomesSeveralTargets(t *testing.T) {
	plan := NewPlan([]Script{{Name: "M", Enabled: true, Def: def("M",
		[]*modscript.Block{valueBlock("A", "1")}, "ONE.MBIN", "TWO.MBIN")}})

	require.Len(t, plan.Targets, 2)
	require.Equal(t, []string{"ONE.MBIN", "TWO.MBIN"}, plan.Sources)
}

// Disabled mods contribute nothing and do not appear in the report. R4.1.
func TestDisabledModsAreNotPlanned(t *testing.T) {
	plan := NewPlan([]Script{
		{Name: "Off", Enabled: false, Def: def("Off", []*modscript.Block{valueBlock("A", "1")}, "X.MBIN")},
	})
	require.Empty(t, plan.Targets)
	require.Empty(t, plan.Enabled)
}

/*
R4.1: a script that will not load keeps its place in the build order.

The legacy builder reported these as warnings in the same stream as the edits,
which is what makes a report readable top to bottom: "this mod's file is gone"
appears where the mod would have been, not in a separate list nobody reads.
*/
func TestLoadFailuresBecomeWarningsInBuildOrder(t *testing.T) {
	plan := NewPlan([]Script{
		{Name: "Gone", Enabled: true, Missing: true},
		{Name: "Broken", Enabled: true, Err: os.ErrInvalid},
	})
	require.Equal(t, []string{"Gone", "Broken"}, plan.Enabled)
	require.Len(t, plan.Issues, 2)
	require.Equal(t, "  WARN Gone: .lua not found", plan.Issues[0].Line())
	require.Contains(t, plan.Issues[1].Line(), "  WARN Broken: dump failed:")
}

// R4.1: a block carrying ADD or REMOVE marks its mod complex, which drives both
// the WORKING* verdict and the retry set when a file will not recompile.
func TestBlocksWithAddOrRemoveMarkTheirModComplex(t *testing.T) {
	plan := NewPlan([]Script{
		{Name: "Plain", Enabled: true, Def: def("Plain", []*modscript.Block{valueBlock("A", "1")}, "X.MBIN")},
		{Name: "Structural", Enabled: true, Def: def("Structural",
			[]*modscript.Block{addBlock("<x />")}, "X.MBIN")},
	})
	require.Equal(t, []string{"Structural"}, plan.Complex)
	require.True(t, plan.isComplex("Structural"))
	require.False(t, plan.isComplex("Plain"))
}

/*
R4.4: the verdict table.

Every branch, because the order of the tests decides what a mod that is in two
categories reports, and because a wrong verdict is the one thing in this report
a user acts on directly -- they go and re-download the mods that are not
WORKING.
*/
func TestVerdictsCoverEveryCombination(t *testing.T) {
	plan := &Plan{
		Enabled: []string{"Clean", "Missing", "Structural", "Degraded", "Dropped", "Nothing"},
		Complex: []string{"Structural", "Degraded"},
	}
	tally := newTally()
	tally.record(mxml.Event{Kind: mxml.OK, Mod: "Clean"})
	tally.record(mxml.Event{Kind: mxml.OK, Mod: "Missing"})
	tally.record(mxml.Event{Kind: mxml.WARN, Mod: "Missing", NotFound: "Gone"})
	tally.record(mxml.Event{Kind: mxml.OK, Mod: "Structural"})
	tally.record(mxml.Event{Kind: mxml.OK, Mod: "Degraded"})
	tally.record(mxml.Event{Kind: mxml.OK, Mod: "Dropped"})
	tally.record(mxml.Event{Kind: mxml.WARN, Mod: "Nothing", NotFound: "Absent"})
	tally.degrade([]string{"Degraded"})
	tally.drop([]string{"Dropped"})

	got := map[string]string{}
	for _, row := range tally.rows(plan) {
		got[row.Name] = row.Verdict
	}
	require.Equal(t, map[string]string{
		"Clean":      report.Working,
		"Missing":    report.WorkingSkipped,
		"Structural": report.WorkingStructural,
		"Degraded":   report.Partial, // PARTIAL beats WORKING*
		"Dropped":    report.Partial,
		"Nothing":    report.NotBuilt,
	}, got)
}

// R4.4: the rows sort by verdict then name, so a reader who stops after the
// first block has seen the mods that are fine.
func TestVerdictRowsSortByVerdictThenName(t *testing.T) {
	rows := []report.ModResult{
		{Name: "zeta", Verdict: report.Working},
		{Name: "Alpha", Verdict: report.NotBuilt},
		{Name: "beta", Verdict: report.Working},
	}
	report.SortMods(rows)
	require.Equal(t, []string{"beta", "zeta", "Alpha"}, []string{rows[0].Name, rows[1].Name, rows[2].Name})
}

/*
fakeCompiler stands in for MBINCompiler in the build tests.

It refuses any MXML containing the marker below and otherwise writes a stub
MBIN. That is exactly the behaviour the degrade-and-drop logic keys off, and it
runs in milliseconds where the real 2 MB .NET binary would not be available at
all in a unit-test run.
*/
const breakMarker = "BREAK-THE-COMPILER"

const fakeCompilerScript = `#!/bin/sh
outdir=""; input=""
while [ $# -gt 0 ]; do
  case "$1" in
    -d) outdir="$2"; shift 2;;
    -y|-q|-Q) shift;;
    version) shift;;
    *) input="$1"; shift;;
  esac
done
[ -z "$input" ] && { echo "MBINCompiler v0.0.0-fake"; exit 0; }
if grep -q 'BREAK-THE-COMPILER' "$input"; then
  echo "[ERROR]: unexpected element" >&2
  exit 1
fi
base=$(basename "$input"); stem=${base%.*}
printf 'MBIN' > "$outdir/$stem.MBIN"
exit 0
`

func fakeCompiler(t *testing.T) *mbin.Compiler {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "MBINCompiler-linux-dotnet10")
	require.NoError(t, os.WriteFile(bin, []byte(fakeCompilerScript), 0o700)) //nolint:gosec // a test stub
	mbin.SetMaxProcesses(4)
	return &mbin.Compiler{Bin: bin, Tag: "v0.0.0-fake", Flavor: mbin.FlavorDotnet10}
}

// pristine writes a source MXML and returns the Options entry pointing at it.
func pristine(t *testing.T, internal, body string) (string, Source) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filepath.Base(internal))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return internal, Source{MXML: path, Internal: internal}
}

const tinyMXML = `<Data>
	<Property name="Entry">
		<Property name="A" value="1" />
	</Property>
</Data>`

// R4.2: the happy path writes the MBIN where the game expects it and mirrors
// the globals.
func TestABuiltTargetLandsUnderTheModFolder(t *testing.T) {
	internal, src := pristine(t, "gcgameplayglobals.global.mbin", tinyMXML)
	plan := NewPlan([]Script{{Name: "M", Enabled: true,
		Def: def("M", []*modscript.Block{valueBlock("A", "9")}, internal)}})

	res, err := Run(t.Context(), plan, Options{
		Sources:  map[string]Source{"GCGAMEPLAYGLOBALS.GLOBAL.MBIN": src},
		Compiler: fakeCompiler(t), Workspace: t.TempDir(), ModName: "TEST MOD", Workers: 2,
	})
	require.NoError(t, err)
	require.Equal(t, 1, res.Built)
	require.Equal(t, 0, res.Dropped)
	require.FileExists(t, filepath.Join(res.OutputDir, "GCGAMEPLAYGLOBALS.GLOBAL.MBIN"))
	require.FileExists(t, filepath.Join(res.OutputDir, "GLOBALS", "GCGAMEPLAYGLOBALS.GLOBAL.MBIN"),
		"R4.2: globals are mirrored into GLOBALS/ as well as the root")
	require.Equal(t, report.Working, res.Mods[0].Verdict)
}

/*
R4.2: a file that will not recompile is retried without its structural edits.

The trade this encodes: one mod's new reward entry is worth less than every
other mod's value edits to the same table. The retry's own report lines are
appended to the first attempt's, so both are visible.
*/
func TestAFileThatWillNotCompileIsRetriedWithoutItsStructuralEdits(t *testing.T) {
	internal, src := pristine(t, "metadata/tables/rewardtable.mbin", tinyMXML)
	plan := NewPlan([]Script{
		{Name: "Values", Enabled: true, Def: def("Values",
			[]*modscript.Block{valueBlock("A", "9")}, internal)},
		{Name: "Adder", Enabled: true, Def: def("Adder",
			[]*modscript.Block{addBlock("\t<" + breakMarker + " />")}, internal)},
	})

	res, err := Run(t.Context(), plan, Options{
		Sources:  map[string]Source{"METADATA/TABLES/REWARDTABLE.MBIN": src},
		Compiler: fakeCompiler(t), Workspace: t.TempDir(), ModName: "TEST MOD", Workers: 1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, res.Built)
	require.Equal(t, 0, res.Dropped)
	require.Equal(t, report.OutcomeDegraded, res.Targets[0].Outcome)
	require.Equal(t, []string{"Adder"}, res.Targets[0].SkippedMods)
	require.FileExists(t, filepath.Join(res.OutputDir, "METADATA", "TABLES", "REWARDTABLE.MBIN"))

	verdicts := map[string]string{}
	for _, m := range res.Mods {
		verdicts[m.Name] = m.Verdict
	}
	require.Equal(t, report.Partial, verdicts["Adder"], "the mod whose edits were dropped")
	require.Equal(t, report.Working, verdicts["Values"],
		"the other contributors got everything they asked for and are not implicated")
	require.Len(t, res.Degraded, 1)
}

// R4.2: when the retry cannot help either, nothing ships and every contributing
// mod is told. Shipping a file the compiler rejected is the one thing this
// pipeline must never do.
func TestATargetThatNeverCompilesIsDroppedAndEveryContributorIsWarned(t *testing.T) {
	internal, src := pristine(t, "broken.mbin", tinyMXML+"\n<"+breakMarker+" />")
	plan := NewPlan([]Script{{Name: "M", Enabled: true,
		Def: def("M", []*modscript.Block{valueBlock("A", "9")}, internal)}})

	res, err := Run(t.Context(), plan, Options{
		Sources:  map[string]Source{"BROKEN.MBIN": src},
		Compiler: fakeCompiler(t), Workspace: t.TempDir(), ModName: "TEST MOD", Workers: 1,
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.Built)
	require.Equal(t, 1, res.Dropped)
	require.Equal(t, report.OutcomeDropped, res.Targets[0].Outcome)
	require.NoFileExists(t, filepath.Join(res.OutputDir, "BROKEN.MBIN"))

	var dropped bool
	for _, l := range res.Lines {
		if strings.Contains(l.Detail, "RECOMPILE FAILED for broken.mbin -> DROPPED") {
			dropped = true
		}
	}
	require.True(t, dropped, "the warning names the file, in the pak's own spelling")
}

// R3.3/R4.2: a source the cache could not produce warns and does not stop the
// build; a mod naming a file the game no longer ships must not cost the user
// every other mod.
func TestASourceWithNoCachedFileIsReportedAndTheBuildContinues(t *testing.T) {
	internal, src := pristine(t, "present.mbin", tinyMXML)
	plan := NewPlan([]Script{
		{Name: "Missing", Enabled: true, Def: def("Missing",
			[]*modscript.Block{valueBlock("A", "1")}, "absent.mbin")},
		{Name: "Present", Enabled: true, Def: def("Present",
			[]*modscript.Block{valueBlock("A", "2")}, internal)},
	})

	res, err := Run(t.Context(), plan, Options{
		Sources:  map[string]Source{"PRESENT.MBIN": src},
		Compiler: fakeCompiler(t), Workspace: t.TempDir(), ModName: "TEST MOD", Workers: 2,
	})
	require.NoError(t, err)
	require.Equal(t, 1, res.Built)
	require.Equal(t, report.OutcomeNoSource, res.Targets[0].Outcome)
	require.Equal(t, "  WARN Missing: no cached MXML for absent.mbin", res.Lines[0].Render())
}

/*
R4.2: report lines come out in plan order however the work was scheduled.

Targets run concurrently, so without the buffering this is testing, the report
would be in completion order -- different on every run, and impossible to diff
against the previous build. Running with more workers than targets is the case
most likely to expose it.
*/
func TestReportLinesFollowPlanOrderNotCompletionOrder(t *testing.T) {
	var scripts []Script
	sources := map[string]Source{}
	var want []string
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		internal, src := pristine(t, name+".mbin", tinyMXML)
		sources[strings.ToUpper(internal)] = src
		scripts = append(scripts, Script{Name: "mod-" + name, Enabled: true,
			Def: def("mod-"+name, []*modscript.Block{valueBlock("A", "9")}, internal)})
		want = append(want, "   OK  mod-"+name+": A -> 9 (1x) in "+name+".mbin")
	}
	plan := NewPlan(scripts)

	for range 5 {
		res, err := Run(t.Context(), plan, Options{
			Sources: sources, Compiler: fakeCompiler(t), Workspace: t.TempDir(),
			ModName: "TEST MOD", Workers: 6,
		})
		require.NoError(t, err)
		var got []string
		for _, l := range res.Lines {
			if l.Kind == "ok" {
				got = append(got, l.Render())
			}
		}
		require.Equal(t, want, got)
	}
}

/*
R4.3: the previous output is kept until this run has finished.

A build that fails halfway must not have destroyed the mod folder the user
currently has deployed, and the merged MXML of a rejected file must still be on
disk afterwards -- it is the only place the answer to "why did the compiler
refuse this" can be found.
*/
func TestThePreviousOutputIsPreservedUntilTheRunSucceeds(t *testing.T) {
	workspace := t.TempDir()
	stale := filepath.Join(workspace, "TEST MOD", "STALE.MBIN")
	require.NoError(t, os.MkdirAll(filepath.Dir(stale), 0o750))
	require.NoError(t, os.WriteFile(stale, []byte("old"), 0o600))

	internal, src := pristine(t, "present.mbin", tinyMXML)
	plan := NewPlan([]Script{{Name: "M", Enabled: true,
		Def: def("M", []*modscript.Block{valueBlock("A", "1")}, internal)}})

	res, err := Run(t.Context(), plan, Options{
		Sources:  map[string]Source{"PRESENT.MBIN": src},
		Compiler: fakeCompiler(t), Workspace: workspace, ModName: "TEST MOD", Workers: 1,
	})
	require.NoError(t, err)
	require.NoFileExists(t, stale, "the previous output is gone from the live folder")
	require.NoDirExists(t, filepath.Join(workspace, "TEST MOD.prev"), "and its backup is cleaned up on success")
	require.FileExists(t, filepath.Join(workspace, "TEST MOD.work", "mxml", "PRESENT.MXML"),
		"the merged document is kept for inspection")
	require.Equal(t, 1, res.Built)
}

// A build with no compiler ships nothing rather than copying unverified files
// into the game folder. R4.2.
func TestWithoutACompilerNothingIsShipped(t *testing.T) {
	internal, src := pristine(t, "present.mbin", tinyMXML)
	plan := NewPlan([]Script{{Name: "M", Enabled: true,
		Def: def("M", []*modscript.Block{valueBlock("A", "1")}, internal)}})

	res, err := Run(t.Context(), plan, Options{
		Sources:   map[string]Source{"PRESENT.MBIN": src},
		Workspace: t.TempDir(), ModName: "TEST MOD", Workers: 1,
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.Built)
	require.Equal(t, 1, res.Dropped)
}
