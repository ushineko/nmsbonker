package mxml_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/mxml"
)

/*
CAP, the ceiling on a multiplied value (spec 005 R2).

Two claims are being tested and the second is the important one. A block that
sets CAP clamps its arithmetic to it, which is the feature. A block that does
not set CAP produces exactly the bytes and exactly the report line it produced
before the key existed, which is what lets the golden suite -- captured from a
builder that had never heard of CAP -- keep passing unchanged. The second claim
is asserted here on a synthetic snippet because a golden failure says "some byte
somewhere moved" and this says which behaviour changed.
*/

// rewards is a reward table in miniature: a money block with a currency, a
// substance block, and the wrapper the standing tweaks multiply.
const rewards = `<?xml version="1.0" encoding="utf-8"?>
<Data template="cGcRewardTable">
	<Property name="Table">
		<Property name="GcRewardMoney">
			<Property name="AmountMin" value="100" />
			<Property name="AmountMax" value="500" />
			<Property name="Currency" value="GcCurrency">
				<Property name="Currency" value="Units" />
			</Property>
		</Property>
		<Property name="GcRewardStanding">
			<Property name="AmountMin" value="20" />
			<Property name="AmountMax" value="80" />
		</Property>
		<Property name="GcRewardSpecificSubstance">
			<Property name="ID" value="AF_METAL" />
			<Property name="AmountMin" value="100" />
			<Property name="AmountMax" value="300" />
		</Property>
	</Property>
</Data>`

// currencyBlock is a CURRENCY_MULT edit, with and without a ceiling.
func currencyBlock(mult, capValue float64) *modscript.Block {
	return &modscript.Block{
		CurrencyMult: &modscript.CurrencyMult{Currency: "Units", Mult: mult},
		Cap:          capValue, HasCap: capValue != 0,
	}
}

// wrapperBlock is a WRAPPER_MULT edit, with and without a ceiling.
func wrapperBlock(mult, capValue float64) *modscript.Block {
	return &modscript.Block{
		WrapperMult: &modscript.WrapperMult{
			Wrapper: "GcRewardStanding", Mult: mult, Keys: []string{"AmountMin", "AmountMax"},
		},
		Cap: capValue, HasCap: capValue != 0,
	}
}

// mathBlock is a VALUE_CHANGE_TABLE with a MATH_OPERATION, with and without a
// ceiling. REPLACE_TYPE ALL so it reaches every matching section, which is the
// shape the loot tweak uses.
func mathBlock(mult, capValue float64) *modscript.Block {
	return &modscript.Block{
		SpecialKeyWords: []string{"GcRewardSpecificSubstance"}, HasSKW: true,
		MathOperation: "*", ReplaceType: "ALL", HasVCT: true,
		ValueChanges: []modscript.ValueChange{
			{Key: "AmountMin", Value: modscript.IntValue(int64(mult))},
			{Key: "AmountMax", Value: modscript.IntValue(int64(mult))},
		},
		Cap: capValue, HasCap: capValue != 0,
	}
}

// AC4: a CAP clamps a CURRENCY_MULT, and says so.
func TestACapClampsACurrencyMult(t *testing.T) {
	got, report := apply(t, rewards, currencyBlock(100, 20000))

	require.Contains(t, got, `<Property name="AmountMin" value="10000" />`, "under the cap, untouched")
	require.Contains(t, got, `<Property name="AmountMax" value="20000" />`, "50000 clamped to 20000")
	require.Equal(t, []string{
		`   OK  TestMod: CURRENCY_MULT Units x100.0 cap 20000 across 1 GcRewardMoney blocks ` +
			`in METADATA\TABLES\F.MBIN (capped 1)`,
	}, report)
}

// AC4: a CAP clamps a WRAPPER_MULT.
func TestACapClampsAWrapperMult(t *testing.T) {
	got, report := apply(t, rewards, wrapperBlock(50, 500))

	require.Contains(t, got, `<Property name="AmountMin" value="500" />`, "1000 clamped")
	require.Contains(t, got, `<Property name="AmountMax" value="500" />`, "4000 clamped")
	require.Equal(t, []string{
		`   OK  TestMod: WRAPPER_MULT GcRewardStanding x50.0 cap 500 across 1 blocks in ` +
			`METADATA\TABLES\F.MBIN (capped 2)`,
	}, report)
}

// AC4: a CAP clamps a VALUE_CHANGE_TABLE multiply.
func TestACapClampsAValueChangeTableMultiply(t *testing.T) {
	got, report := apply(t, rewards, mathBlock(1000, 150000))

	require.Contains(t, got, `<Property name="AmountMin" value="100000" />`, "under the cap")
	require.Contains(t, got, `<Property name="AmountMax" value="150000" />`, "300000 clamped")
	require.Equal(t, []string{
		`   OK  TestMod: ['AmountMin', 'AmountMax'] x* cap 150000 across 1 ` +
			`'GcRewardSpecificSubstance' sections in METADATA\TABLES\F.MBIN (capped 1)`,
	}, report)
}

/*
AC4: with no CAP, the output and the report line are what they were.

The expectation is spelled out in full rather than compared against a second
run, because "the same as itself" is a test that passes when both sides are
wrong. These are the bytes and the wording the golden fixtures hold.
*/
func TestWithNoCapTheOutputIsUnchanged(t *testing.T) {
	got, report := apply(t, rewards, currencyBlock(100, 0))
	require.Contains(t, got, `<Property name="AmountMin" value="10000" />`)
	require.Contains(t, got, `<Property name="AmountMax" value="50000" />`)
	require.Equal(t, []string{
		`   OK  TestMod: CURRENCY_MULT Units x100.0 across 1 GcRewardMoney blocks in ` +
			`METADATA\TABLES\F.MBIN`,
	}, report)

	got, report = apply(t, rewards, wrapperBlock(50, 0))
	require.Contains(t, got, `<Property name="AmountMax" value="4000" />`)
	require.Equal(t, []string{
		`   OK  TestMod: WRAPPER_MULT GcRewardStanding x50.0 across 1 blocks in METADATA\TABLES\F.MBIN`,
	}, report)

	got, report = apply(t, rewards, mathBlock(1000, 0))
	require.Contains(t, got, `<Property name="AmountMax" value="300000" />`)
	for _, line := range report {
		require.NotContains(t, line, "cap", "a block with no CAP must not mention one")
	}
}

// A cap the arithmetic never reaches leaves both the value and the wording of
// the count alone: "capped 0" in every line would make the one line that
// matters impossible to find.
func TestACapThatNeverBitesIsSilentAboutIt(t *testing.T) {
	got, report := apply(t, rewards, wrapperBlock(2, 1000000))

	require.Contains(t, got, `<Property name="AmountMax" value="160" />`)
	require.Equal(t, []string{
		`   OK  TestMod: WRAPPER_MULT GcRewardStanding x2.0 cap 1000000 across 1 blocks in ` +
			`METADATA\TABLES\F.MBIN`,
	}, report)
}

/*
A CAP is a ceiling on the arithmetic, not on a value the script writes out.

A VALUE_CHANGE_TABLE with no MATH_OPERATION puts the script's own number into
the file verbatim, and clamping that would mean a script asking for a stack
limit of 999,999 and silently getting 50,000 because a cap in the same block
was meant for a different key.
*/
func TestACapDoesNotClampAVerbatimValue(t *testing.T) {
	blk := &modscript.Block{
		SpecialKeyWords: []string{"GcRewardSpecificSubstance"}, HasSKW: true,
		HasVCT:       true,
		ValueChanges: []modscript.ValueChange{{Key: "AmountMax", Value: modscript.StringValue("999999")}},
		Cap:          100, HasCap: true,
	}
	got, _ := apply(t, rewards, blk)
	require.Contains(t, got, `<Property name="AmountMax" value="999999" />`)
}

/*
The engine's int32 clamp still applies under a cap, and a cap below it wins.

The two limits do different jobs -- one stops the field overflowing, the other
stops the game seeing an amount it cannot hold in a stack -- and a build that
applied only the first is exactly the build spec 005 exists because of.
*/
func TestTheCapAndTheInt32ClampBothHold(t *testing.T) {
	capped, _ := apply(t, rewards, currencyBlock(1e9, 2000))
	require.Contains(t, capped, `<Property name="AmountMax" value="2000" />`)

	uncapped, _ := apply(t, rewards, currencyBlock(1e9, 0))
	require.Contains(t, uncapped, `<Property name="AmountMax" value="2147483647" />`)
}

/*
The report line's shape is part of the contract, so it is asserted rather than
described: the golden set compares 504 of these byte for byte, and the cap text
sits between the multiplier and the "across N" clause on every op that has one.
*/
func TestTheCapTextSitsAfterTheMultiplier(t *testing.T) {
	_, report := apply(t, rewards, wrapperBlock(5, 100))
	require.Len(t, report, 1)
	require.True(t, strings.Contains(report[0], "x5.0 cap 100 across"),
		"unexpected wording: %s", report[0])
}

// A block whose CAP is negative is treated as having none. A nonsense value
// should not clamp every amount in the game to a negative number.
func TestANegativeCapIsNoCap(t *testing.T) {
	got, report := apply(t, rewards, wrapperBlock(50, -1))
	require.Contains(t, got, `<Property name="AmountMax" value="4000" />`)
	require.NotContains(t, report[0], "cap")
}

// The engine reports how many values a cap held back, which is what the build
// report totals into "capped N values".
func TestTheEventCarriesTheCappedCount(t *testing.T) {
	_, events := mxml.Apply(lines(rewards), wrapperBlock(50, 500), actx())
	require.Len(t, events, 1)
	require.Equal(t, 2, events[0].Capped)
}
