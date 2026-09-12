package tweaks_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/mxml"
	"github.com/ushineko/nmsbonker/internal/tweaks"
)

// R1.1: every embedded script is in the default order and vice versa. A script
// embedded but not listed ships in the binary and is reachable from nowhere;
// one listed but not embedded is a name the interface offers and cannot load.
func TestEveryEmbeddedScriptIsListedAndEveryListedScriptIsEmbedded(t *testing.T) {
	files := tweaks.Files()
	require.Len(t, files, 10, "the built-in set is ten scripts")

	listed := map[string]bool{}
	for _, n := range tweaks.Names() {
		listed[n] = true
		_, ok := tweaks.Source(n)
		require.Truef(t, ok, "%s is in the default order but not embedded", n)
	}
	for _, f := range files {
		require.Truef(t, listed[f], "%s is embedded but not in the default order", f)
	}
}

/*
R1.1/R1.2: each built-in declares a header and parameters that match its code.

The header is what the Tweaks section draws from and the parameters are what it
writes to, so a script whose @param names a global it does not assign would show
a slider that changes nothing. Checking it here rather than in the GUI is the
difference between a test and a bug report.
*/
func TestEveryBuiltInDeclaresAHeaderAndUsableParameters(t *testing.T) {
	groups := map[string]bool{tweaks.GroupOther: true}
	for _, g := range tweaks.Groups {
		groups[g] = true
	}

	for _, tw := range tweaks.All() {
		t.Run(tw.Name, func(t *testing.T) {
			require.NotEmpty(t, tw.Header.Name)
			require.NotEqual(t, tweaks.GroupOther, tw.Header.Group,
				"every built-in belongs to one of the declared groups")
			require.True(t, groups[tw.Header.Group])
			require.NotEmpty(t, tw.Header.Desc, "the card needs something to say")
			require.NotEmpty(t, tw.Params)

			src, ok := tweaks.Source(tw.Name)
			require.True(t, ok)
			require.Empty(t, modscript.DuplicateAssignments(src),
				"a parameter assigned twice would ignore the user's override")

			for _, p := range tw.Params {
				require.NotEmpty(t, p.Label)
				require.True(t, p.Bounded, "a built-in declares its bounds")
				require.Greater(t, p.Max, p.Min)
				require.GreaterOrEqual(t, p.Default, p.Min)
				require.LessOrEqual(t, p.Default, p.Max)
				require.Equal(t, p.Default, p.Current)

				// The override path has to find the assignment, or the value
				// the user sets is silently dropped.
				_, err := modscript.Override(src, p.Name, p.Default, p.Kind)
				require.NoErrorf(t, err, "no assignment to substitute for %s", p.Name)
			}
		})
	}
}

// R1.1: the headers are comments, so every built-in still loads as a script and
// still says who wrote it. A script that will not load is a built-in that turns
// into a red row in the mod table on a fresh install.
func TestEveryBuiltInLoadsThroughTheSandbox(t *testing.T) {
	for _, name := range tweaks.Names() {
		src, ok := tweaks.Source(name)
		require.True(t, ok)
		def, err := modscript.LoadSource(t.Context(), name+".lua", src)
		require.NoErrorf(t, err, "%s does not load", name)
		require.Equal(t, "nmsbonker", def.Author, "%s", name)
		require.NotEmpty(t, def.Targets(), "%s edits nothing", name)
	}
}

// Source hands out a copy. The caller rewrites parameter assignments in the
// bytes it gets back, and the embedded copy has to survive that for the next
// build.
func TestSourceIsACopy(t *testing.T) {
	first, ok := tweaks.Source("ItemValueBoost")
	require.True(t, ok)
	first[0] = 'X'
	second, ok := tweaks.Source("ItemValueBoost")
	require.True(t, ok)
	require.NotEqual(t, byte('X'), second[0])
}

/*
harvester is the antimatter harvester's maintenance block in miniature
(spec 005 R2.5).

Two GcMaintenanceElement blocks with a MaxCapacity each, which is the shape of
the real entity and the reason a bare "anchor on GcMaintenanceElement" is wrong:
the first is the fuel slot. Hand-written, like every fixture here.
*/
const harvester = `<?xml version="1.0" encoding="utf-8"?>
<Data template="cGcSceneNodeData">
	<Property name="Components" value="GcMaintenanceComponentData">
		<Property name="GcMaintenanceComponentData">
			<Property name="PreInstalledTech">
				<Property name="PreInstalledTech" value="GcMaintenanceElement" _id="MAINT_FUEL4">
					<Property name="Id" value="MAINT_FUEL4" />
					<Property name="MaxCapacity" value="-1" />
				</Property>
				<Property name="PreInstalledTech" value="GcMaintenanceElement" _id="ANTIMATTER">
					<Property name="Id" value="ANTIMATTER" />
					<Property name="MaxCapacity" value="-1" />
				</Property>
			</Property>
		</Property>
	</Property>
</Data>`

// harvesterBlocks are BigStacks's edits to the harvester entity, as the script
// declares them.
func harvesterBlocks(t *testing.T, src []byte) []*modscript.Block {
	t.Helper()
	def, err := modscript.LoadSource(t.Context(), "BigStacks.lua", src)
	require.NoError(t, err)
	for _, mod := range def.Modifications {
		for _, ch := range mod.Changes {
			for _, s := range ch.Sources {
				if strings.Contains(s, "ANTIMATTERHARVESTER") {
					return ch.Blocks
				}
			}
		}
	}
	return nil
}

/*
R2.5: the harvester cap moves exactly one MaxCapacity, and it is the right one.

The harvester holds two maintenance elements -- fuel and antimatter -- and the
library script this edit replaces picked the second with SECTION_ACTIVE, a key
this engine does not implement. Anchoring on GcMaintenanceElement alone would
therefore silently cap the fuel slot instead, which is a wrong edit that looks
exactly like a right one in the report.
*/
func TestTheAntimatterHarvesterCapMovesExactlyOneMaxCapacity(t *testing.T) {
	src, ok := tweaks.Source("BigStacks")
	require.True(t, ok)
	blocks := harvesterBlocks(t, src)
	require.Len(t, blocks, 1, "one edit to the harvester entity")

	lines := strings.Split(harvester, "\n")
	for _, blk := range blocks {
		lines, _ = mxml.Apply(lines, blk, mxml.ApplyContext{
			Mod: "BigStacks", Source: `MODELS\...\ANTIMATTERHARVESTER.ENTITY.MBIN`,
		})
	}
	got := strings.Join(lines, "\n")

	require.Equal(t, 1, strings.Count(got, `name="MaxCapacity" value="20"`),
		"exactly one MaxCapacity is capped")
	require.Equal(t, 1, strings.Count(got, `name="MaxCapacity" value="-1"`),
		"and the other one is left stock")
	// And it is the antimatter element's, not the fuel slot's.
	fuelAt := strings.Index(got, `_id="MAINT_FUEL4"`)
	antimatterAt := strings.Index(got, `_id="ANTIMATTER"`)
	require.Positive(t, fuelAt, "the fixture no longer holds a fuel element")
	require.Greater(t, antimatterAt, fuelAt, "the fixture no longer holds both, in order")
	require.Contains(t, got[antimatterAt:], `name="MaxCapacity" value="20"`)
	require.Contains(t, got[fuelAt:antimatterAt], `name="MaxCapacity" value="-1"`)
}

// R2.5: 0 leaves the game's own value alone, by not producing the edit at all.
// A cap of zero would otherwise mean "a harvester holds nothing".
func TestAZeroAntimatterCapProducesNoHarvesterEdit(t *testing.T) {
	src, ok := tweaks.Source("BigStacks")
	require.True(t, ok)
	off, err := modscript.Override(src, "ANTIMATTER_HARVESTER_CAP", 0, modscript.ParamInt)
	require.NoError(t, err)

	require.Nil(t, harvesterBlocks(t, off), "no edit to the harvester entity at all")

	def, err := modscript.LoadSource(t.Context(), "BigStacks.lua", off)
	require.NoError(t, err)
	require.Len(t, def.Targets(), 1, "and therefore one target file, not two")
}

/*
R2.3: every cap parameter is a ceiling the engine will honour.

A @param named CAP_SOMETHING that no block passes to the engine is a slider the
user can drag with no effect, and that is not visible from the outside: the
build reports the same edits either way. So each declared cap has to appear as a
CAP key on at least one block of the script that declares it.
*/
func TestEveryDeclaredCapReachesABlock(t *testing.T) {
	caps := map[string][]string{
		"ChestAndLootMaterials10x": {"LOOT_CAP"},
		"MaterialYield10x":         {"YIELD_CAP"},
		"MoneyAndNanites5x":        {"UNITS_CAP", "NANITES_CAP"},
		"NaniteRewardBuff":         {"NANITES_CAP"},
		"SpaceMiningBoost":         {"AST_CAP"},
		"ScanValue50x":             {"SCAN_CAP"},
		"MissionStandingBuff":      {"STANDING_CAP"},
		"LearnMoreWords":           {"WORDS_CAP"},
	}
	for name, params := range caps {
		t.Run(name, func(t *testing.T) {
			src, ok := tweaks.Source(name)
			require.True(t, ok)
			declared := map[string]float64{}
			for _, p := range modscript.Parameters(src) {
				declared[p.Name] = p.Default
			}
			def, err := modscript.LoadSource(t.Context(), name+".lua", src)
			require.NoError(t, err)

			seen := map[float64]bool{}
			for _, mod := range def.Modifications {
				for _, ch := range mod.Changes {
					for _, blk := range ch.Blocks {
						if blk.HasCap {
							seen[blk.Cap] = true
						}
					}
				}
			}
			for _, param := range params {
				value, ok := declared[param]
				require.Truef(t, ok, "%s declares no %s", name, param)
				require.Greaterf(t, value, 0.0, "%s defaults to no ceiling at all", param)
				require.Truef(t, seen[value],
					"%s is declared but no block carries CAP = %v", param, value)
			}
		})
	}
}
