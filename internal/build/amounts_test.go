package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/build/audit"
	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/mxml"
)

/*
The audit as the build runs it (spec 005 R1).

The audit package's own tests drive it with hand-made block lists. These drive
it through the real edit engine, which is where the two things that can only
break together live: the attribution's re-merge has to apply exactly the blocks
the merge applied, in the same order, and the flagged positions have to survive
that second pass. A test that fakes the merge cannot see either.
*/

// rewardTable is a reward table in miniature, in MBINCompiler's shape.
const rewardTable = `<?xml version="1.0" encoding="utf-8"?>
<Data template="cGcRewardTable">
	<Property name="GenericTable">
		<Property name="GenericTable" value="GcGenericRewardTableEntry" _id="R_CHEST">
			<Property name="Id" value="R_CHEST" />
			<Property name="List">
				<Property name="Reward" value="GcRewardSpecificProduct">
					<Property name="GcRewardSpecificProduct">
						<Property name="ID" value="BP_SALVAGE" />
						<Property name="AmountMin" value="2" />
						<Property name="AmountMax" value="4" />
					</Property>
				</Property>
			</Property>
			<Property name="List">
				<Property name="Reward" value="GcRewardMoney">
					<Property name="GcRewardMoney">
						<Property name="AmountMin" value="1000" />
						<Property name="AmountMax" value="5000" />
						<Property name="Currency" value="GcCurrency">
							<Property name="Currency" value="Units" />
						</Property>
					</Property>
				</Property>
			</Property>
		</Property>
	</Property>
</Data>`

// productMult is the shape ChestAndLootMaterials10x uses: multiply the amounts
// in every GcRewardSpecificProduct section.
func productMult(mult int64, capValue float64) *modscript.Block {
	return &modscript.Block{
		SpecialKeyWords: []string{"GcRewardSpecificProduct"}, HasSKW: true,
		MathOperation: "*", ReplaceType: "ALL", HasVCT: true,
		ValueChanges: []modscript.ValueChange{
			{Key: "AmountMin", Value: modscript.IntValue(mult)},
			{Key: "AmountMax", Value: modscript.IntValue(mult)},
		},
		Cap: capValue,
		Raw: map[string]any{"VALUE_CHANGE_TABLE": []any{}},
	}
}

// runAudit merges the items over the pristine text the way a target does, then
// audits the result.
func runAudit(t *testing.T, pristine string, items []Item, th audit.Thresholds) (*audit.Result, []mxml.Event) {
	t.Helper()
	r := &runner{opts: Options{Audit: th}}
	lines := strings.Split(pristine, "\n")
	merged, _ := merge(lines, items)
	return r.audit(&Target{Items: items}, lines, merged,
		"METADATA/REALITY/TABLES/REWARDTABLE.MBIN")
}

/*
R1.3: two mods multiplying the same reward are both named, in build order.

This is the case the whole spec exists for: each edit is correct, the report
says OK twice, and the result is a reward the game cannot hold. The contributor
list is the only place the compounding is visible.
*/
func TestTheAuditNamesEveryModThatMovedAFlaggedAmount(t *testing.T) {
	items := []Item{
		{Mod: "BetterRewards", Source: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`,
			Block: productMult(250, 0)},
		{Mod: "BetterRewards", Source: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`,
			Block: productMult(250, 0)},
		{Mod: "ChestAndLootMaterials10x", Source: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`,
			Block: productMult(10, 0)},
	}
	res, events := runAudit(t, rewardTable, items, audit.Defaults())

	require.NotNil(t, res)
	require.Len(t, res.Flags, 1, "the money reward is untouched and under its limit")
	f := res.Flags[0]
	require.Equal(t, "R_CHEST", f.EntryID)
	require.Equal(t, "BP_SALVAGE", f.Item)
	require.InDelta(t, 4.0, f.PristineMax, 0)
	require.InDelta(t, 2500000.0, f.MergedMax, 0)
	require.InDelta(t, 625000.0, f.Ratio, 0)
	require.Equal(t,
		"BetterRewards x250 -> 1000, BetterRewards x250 -> 250000, "+
			"ChestAndLootMaterials10x x10 -> 2500000", f.ContributorText())

	// R1.6: one warning per flagged block, named after the table and marked so
	// the per-mod tallies leave it alone.
	require.Len(t, events, 1)
	require.Equal(t, mxml.WARN, events[0].Kind)
	require.True(t, events[0].Audit)
	require.Equal(t, "REWARDTABLE", events[0].Mod)
	require.Equal(t,
		"AUDIT R_CHEST/BP_SALVAGE 2-4 -> 1250000-2500000 (x625000): "+
			"BetterRewards, BetterRewards, ChestAndLootMaterials10x", events[0].Detail)
}

// A build whose amounts stay inside the limits produces a result that says so,
// which is not the same as producing no result at all.
func TestACleanTableStillProducesAnAuditResult(t *testing.T) {
	items := []Item{{Mod: "ChestAndLootMaterials10x",
		Source: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`, Block: productMult(10, 0)}}
	res, events := runAudit(t, rewardTable, items, audit.Defaults())

	require.NotNil(t, res)
	require.Empty(t, res.Flags)
	require.Empty(t, events)
	require.Equal(t, 2, res.Blocks)
	require.Equal(t, []string{"REWARDTABLE"}, res.Tables)
}

// A file that is not one of the audited tables is not audited, and the caller
// can tell that apart from "audited and clean".
func TestAFileThatIsNotARewardTableIsNotAudited(t *testing.T) {
	r := &runner{opts: Options{Audit: audit.Defaults()}}
	res, events := r.audit(&Target{}, nil, nil, "METADATA/GAMESTATE/DIFFICULTYCONFIG.MBIN")
	require.Nil(t, res)
	require.Empty(t, events)
}

/*
R2: a cap stops the compounding, and the audit still says it happened.

Both halves are the point. The cap is what makes the built value playable; the
flag is what tells the user their library is multiplying a reward by 625,000 and
that the only reason it is not visible in game is a ceiling.
*/
func TestACapBoundsTheResultAndTheAuditStillFlagsTheRatio(t *testing.T) {
	items := []Item{
		{Mod: "BetterRewards", Source: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`,
			Block: productMult(250, 0)},
		{Mod: "BetterRewards", Source: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`,
			Block: productMult(250, 0)},
		{Mod: "ChestAndLootMaterials10x", Source: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`,
			Block: productMult(10, 50000)},
	}
	res, _ := runAudit(t, rewardTable, items, audit.Defaults())

	require.Len(t, res.Flags, 1)
	require.InDelta(t, 50000.0, res.Flags[0].MergedMax, 0, "the cap held")
	require.InDelta(t, 12500.0, res.Flags[0].Ratio, 0, "and the ratio still says what happened")
}

/*
R1.5: the same audit, re-run over the merge a build kept, gives the same answer.

Which is the whole justification for the command: changing a threshold and
asking again must not require a rebuild, and the only way that is trustworthy is
if the re-read of the kept file agrees with what the build itself computed.
*/
func TestReAuditingTheKeptMergeAgreesWithTheBuild(t *testing.T) {
	items := []Item{
		{Mod: "BetterRewards", Source: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`,
			Block: productMult(250, 0)},
		{Mod: "ChestAndLootMaterials10x", Source: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`,
			Block: productMult(10, 0)},
	}
	inBuild, _ := runAudit(t, rewardTable, items, audit.Defaults())

	root := t.TempDir()
	pristinePath := filepath.Join(root, "REWARDTABLE.MXML")
	require.NoError(t, os.WriteFile(pristinePath, []byte(rewardTable), 0o600))
	mergedDir := filepath.Join(root, "MOD.work", "mxml", "METADATA", "REALITY", "TABLES")
	require.NoError(t, os.MkdirAll(mergedDir, 0o750))
	merged, _ := merge(strings.Split(rewardTable, "\n"), items)
	require.NoError(t, os.WriteFile(filepath.Join(mergedDir, "REWARDTABLE.MXML"),
		[]byte(strings.Join(merged, "\n")), 0o600))

	target := &Target{Key: "METADATA/REALITY/TABLES/REWARDTABLE.MBIN", Items: items}
	run, err := AuditKeptMerge(AuditOptions{
		Plan: &Plan{Targets: []*Target{target}},
		Sources: map[string]Source{target.Key: {
			MXML: pristinePath, Internal: "METADATA/REALITY/TABLES/REWARDTABLE.MBIN",
		}},
		Workspace: root, ModName: "MOD", Thresholds: audit.Defaults(),
	})
	require.NoError(t, err)
	require.Equal(t, []string{"REWARDTABLE"}, run.Checked)
	require.Len(t, run.Result.Flags, len(inBuild.Flags))
	require.Equal(t, inBuild.Flags[0].ContributorText(), run.Result.Flags[0].ContributorText())
	require.InDelta(t, inBuild.Flags[0].MergedMax, run.Result.Flags[0].MergedMax, 0)
}

// A threshold change is visible without rebuilding, which is what makes the
// command worth having.
func TestReAuditingHonoursTheThresholdsItIsGiven(t *testing.T) {
	items := []Item{{Mod: "ChestAndLootMaterials10x",
		Source: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`, Block: productMult(10, 0)}}

	clean, _ := runAudit(t, rewardTable, items, audit.Defaults())
	require.Empty(t, clean.Flags)

	strict := audit.Defaults()
	strict.MaxRatio = 5
	flagged, _ := runAudit(t, rewardTable, items, strict)
	require.Len(t, flagged.Flags, 1)
	require.Equal(t, "BP_SALVAGE", flagged.Flags[0].Item)
}

// An audit warning is about a value, not about an edit, so it must not turn a
// mod whose every edit landed into one the report says skipped something.
func TestAnAuditWarningIsNotCountedAsASkippedEdit(t *testing.T) {
	stats := newTally()
	stats.record(mxml.Event{Kind: mxml.WARN, Mod: "REWARDTABLE", Audit: true})
	stats.record(mxml.Event{Kind: mxml.OK, Mod: "AMod", Capped: 3})

	require.Zero(t, stats.skipped)
	require.Equal(t, 1, stats.applied)
	require.Equal(t, 3, stats.capped, "the capped count is totalled across the build")
}
