package mxml_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/modscript"
)

/*
Spec 006 R1: ENTRY confines WRAPPER_MULT and CURRENCY_MULT to one reward-table
entry.

Two entries of the same shape, so that "the other one is untouched" is a real
assertion rather than an absence of anything to touch. The _id attribute on the
entry line is what MBINCompiler writes; the fallback through the Id line is
tested separately with the attribute removed.
*/
const twoEntries = `<Data>
	<Property name="MissionBoardTable" value="GcGenericRewardTableEntry" _id="R_ONE">
		<Property name="Id" value="R_ONE" />
		<Property name="List">
			<Property name="GcRewardSpecificProduct">
				<Property name="AmountMin" value="1" />
				<Property name="AmountMax" value="2" />
			</Property>
			<Property name="GcRewardMoney">
				<Property name="AmountMin" value="100" />
				<Property name="AmountMax" value="200" />
				<Property name="Currency" value="Units" />
			</Property>
		</Property>
	</Property>
	<Property name="MissionBoardTable" value="GcGenericRewardTableEntry" _id="R_TWO">
		<Property name="Id" value="R_TWO" />
		<Property name="List">
			<Property name="GcRewardSpecificProduct">
				<Property name="AmountMin" value="3" />
				<Property name="AmountMax" value="4" />
			</Property>
			<Property name="GcRewardMoney">
				<Property name="AmountMin" value="300" />
				<Property name="AmountMax" value="400" />
				<Property name="Currency" value="Units" />
			</Property>
		</Property>
	</Property>
</Data>`

// AC1: a wrapper multiplier with ENTRY edits that entry's blocks and no other.
func TestAnEntryScopedWrapperMultLeavesTheOtherEntryAlone(t *testing.T) {
	got, report := apply(t, twoEntries, &modscript.Block{
		WrapperMult: &modscript.WrapperMult{
			Wrapper: "GcRewardSpecificProduct", Mult: 5, Keys: []string{"AmountMin", "AmountMax"},
			Entry: "R_TWO",
		},
	})
	require.Contains(t, got, `name="AmountMin" value="15"`)
	require.Contains(t, got, `name="AmountMax" value="20"`)
	require.Contains(t, got, `name="AmountMin" value="1"`, "R_ONE's item block is untouched")
	require.Contains(t, got, `name="AmountMax" value="2"`)
	require.Contains(t, got, `name="AmountMin" value="300"`, "money is not this op's business")
	require.Equal(t, []string{
		`   OK  TestMod: WRAPPER_MULT GcRewardSpecificProduct x5.0 in entry R_TWO across 1 blocks in METADATA\TABLES\F.MBIN`,
	}, report)
}

// AC2: a currency multiplier with ENTRY edits that entry's money and no other.
func TestAnEntryScopedCurrencyMultLeavesTheOtherEntryAlone(t *testing.T) {
	got, report := apply(t, twoEntries, &modscript.Block{
		CurrencyMult: &modscript.CurrencyMult{Currency: "Units", Mult: 5, Entry: "R_ONE"},
	})
	require.Contains(t, got, `name="AmountMin" value="500"`)
	require.Contains(t, got, `name="AmountMax" value="1000"`)
	require.Contains(t, got, `name="AmountMin" value="300"`, "R_TWO's money is untouched")
	require.Contains(t, got, `name="AmountMax" value="400"`)
	require.Contains(t, got, `name="AmountMin" value="1"`, "items are not this op's business")
	require.Equal(t, []string{
		`   OK  TestMod: CURRENCY_MULT Units x5.0 in entry R_ONE across 1 GcRewardMoney blocks in METADATA\TABLES\F.MBIN`,
	}, report)
}

// AC3: without the _id attribute the entry is found through its Id line and
// that line's parent, and the same blocks are edited.
func TestAnEntryResolvesThroughTheIdLineWhenThereIsNoIdAttribute(t *testing.T) {
	src := stripAttr(twoEntries)
	require.NotContains(t, src, `_id=`)
	got, _ := apply(t, src, &modscript.Block{
		WrapperMult: &modscript.WrapperMult{
			Wrapper: "GcRewardSpecificProduct", Mult: 5, Keys: []string{"AmountMin", "AmountMax"},
			Entry: "R_TWO",
		},
	})
	require.Contains(t, got, `name="AmountMin" value="15"`)
	require.Contains(t, got, `name="AmountMin" value="1"`, "R_ONE's item block is untouched")
}

// AC4: an entry the table does not have is one warning and no edits, so a
// renamed entry shows up in the report rather than vanishing.
func TestAnUnknownEntryIsAWarningAndNoEdit(t *testing.T) {
	got, report := apply(t, twoEntries, &modscript.Block{
		CurrencyMult: &modscript.CurrencyMult{Currency: "Units", Mult: 5, Entry: "R_GONE"},
	})
	require.Equal(t, twoEntries, got)
	require.Equal(t, []string{
		`  WARN TestMod: entry R_GONE not found in METADATA\TABLES\F.MBIN`,
	}, report)
}

// AC5: a block without ENTRY still runs over the whole file with the pre-006
// event text.
func TestNoEntryMeansTheWholeFileAsBefore(t *testing.T) {
	got, report := apply(t, twoEntries, &modscript.Block{
		CurrencyMult: &modscript.CurrencyMult{Currency: "Units", Mult: 5},
	})
	require.Contains(t, got, `name="AmountMin" value="500"`)
	require.Contains(t, got, `name="AmountMin" value="1500"`)
	require.Equal(t, []string{
		`   OK  TestMod: CURRENCY_MULT Units x5.0 across 2 GcRewardMoney blocks in METADATA\TABLES\F.MBIN`,
	}, report)
}

// stripAttr removes the _id attributes MBINCompiler writes on entry lines.
func stripAttr(src string) string {
	out := ""
	for _, line := range lines(src) {
		if i := indexOf(line, ` _id="`); i >= 0 {
			line = line[:i] + ">"
		}
		out += line + "\n"
	}
	return out[:len(out)-1]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
