package mxml_test

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/mxml"
)

/*
Every fixture here is hand-written. The repository is public and nothing
extracted from the game may be committed, so these snippets imitate
MBINCompiler's output -- tab indentation, one <Property> per line, self-closing
leaves -- without being any of it.
*/
const sample = `<?xml version="1.0" encoding="utf-8"?>
<Data template="cGcTest">
	<Property name="Table">
		<Property name="GcEntry.xml">
			<Property name="Id" value="ALPHA" />
			<Property name="AmountMin" value="10" />
			<Property name="AmountMax" value="20" />
		</Property>
		<Property name="GcEntry.xml">
			<Property name="Id" value="BETA" />
			<Property name="AmountMin" value="1.5" />
			<Property name="AmountMax" value="3" />
		</Property>
	</Property>
</Data>`

func lines(s string) []string { return strings.Split(s, "\n") }

func actx() mxml.ApplyContext {
	return mxml.ApplyContext{Mod: "TestMod", Source: `METADATA\TABLES\F.MBIN`}
}

// apply runs one block and returns the merged text plus the rendered report.
func apply(t *testing.T, src string, blk *modscript.Block) (string, []string) {
	t.Helper()
	out, events := mxml.Apply(lines(src), blk, actx())
	rendered := make([]string, 0, len(events))
	for _, e := range events {
		rendered = append(rendered, e.Line())
	}
	return strings.Join(out, "\n"), rendered
}

// vct is the common case: change these keys to these literal values.
func vct(pairs ...string) []modscript.ValueChange {
	out := make([]modscript.ValueChange, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, modscript.ValueChange{Key: pairs[i], Value: modscript.StringValue(pairs[i+1])})
	}
	return out
}

// R2.1: the plainest block -- no anchor, no maths -- edits the first matching
// property in the file and reports it in the reference wording.
func TestABlockWithNoAnchorEditsTheFirstMatchingProperty(t *testing.T) {
	got, report := apply(t, sample, &modscript.Block{ValueChanges: vct("AmountMin", "99"), HasVCT: true})

	require.Contains(t, got, `<Property name="AmountMin" value="99" />`)
	require.Contains(t, got, `<Property name="AmountMin" value="1.5" />`, "only the first")
	require.Equal(t, []string{`   OK  TestMod: AmountMin -> 99 (1x) in METADATA\TABLES\F.MBIN`}, report)
}

/*
R2.1: PRECEDING_KEY_WORDS moves the starting line, it does not select a scope.

Each keyword advances the cursor to the line after its first hit, so listing the
same keyword twice reaches the second occurrence. Three of the reference scripts
rely on exactly that to address the second and third engine block in a globals
file.
*/
func TestPrecedingKeyWordsAdvanceTheCursorOncePerKeyword(t *testing.T) {
	got, _ := apply(t, sample, &modscript.Block{
		PrecedingKeyWords: []string{"GcEntry.xml", "GcEntry.xml"},
		ValueChanges:      vct("AmountMin", "99"), HasVCT: true,
	})
	require.Contains(t, got, `<Property name="AmountMin" value="10" />`, "the first entry is untouched")
	require.Contains(t, got, `<Property name="AmountMin" value="99" />`, "the second is edited")
}

// An empty keyword is skipped rather than matching every line, which is what
// `["PRECEDING_KEY_WORDS"] = ""` in a script means. R2.1.
func TestAnEmptyPrecedingKeywordIsIgnored(t *testing.T) {
	got, _ := apply(t, sample, &modscript.Block{
		PrecedingKeyWords: []string{""},
		ValueChanges:      vct("AmountMin", "99"), HasVCT: true,
	})
	require.Contains(t, got, `<Property name="AmountMin" value="99" />`)
}

// R2.3: SECTION_UP climbs by tab depth, walking straight past self-closing
// lines, so an anchor on a leaf property can select the entry containing it.
func TestSectionUpClimbsByTabDepthPastSelfClosingLines(t *testing.T) {
	got, report := apply(t, sample, &modscript.Block{
		SpecialKeyWords: []string{"BETA"}, HasSKW: true, SectionUp: 1,
		ValueChanges: vct("AmountMin", "77"), HasVCT: true,
	})
	require.Contains(t, got, `<Property name="AmountMin" value="10" />`, "the first entry is out of scope")
	require.Contains(t, got, `<Property name="AmountMin" value="77" />`)
	require.Equal(t, []string{`   OK  TestMod: AmountMin -> 77 (1x) in METADATA\TABLES\F.MBIN`}, report)
}

/*
R2.2: a SECTION_UP walk that lands on a self-closing line scopes the rest of
the file.

close_index counts openers, and a self-closing line is not one, so its depth
never returns to zero and it falls through to the last line. That is a bug in
the reference engine, it is reachable from a badly indented MXML, and reproducing
it is the point: a merge that "fixes" it edits different lines than the golden
set records.
*/
func TestASectionUpWalkOntoASelfClosingLineScopesTheRestOfTheFile(t *testing.T) {
	src := "<Data>\n\t<Property name=\"A\" value=\"X\" />\n\t\t<Property name=\"Deep\" value=\"Y\" />\n\t<Property name=\"Late\" value=\"1\" />\n</Data>"

	got, _ := apply(t, src, &modscript.Block{
		SpecialKeyWords: []string{"Y"}, HasSKW: true, SectionUp: 1,
		ValueChanges: vct("Late", "9"), HasVCT: true,
	})
	require.Contains(t, got, `<Property name="Late" value="9" />`,
		"the scope reached a line that a correct close_index would have excluded")
}

// R2.3: REMOVE deletes the whole selected section and stops the block.
func TestRemoveDeletesTheSelectedSectionAndEndsTheBlock(t *testing.T) {
	got, report := apply(t, sample, &modscript.Block{
		SpecialKeyWords: []string{"BETA"}, HasSKW: true, SectionUp: 1,
		Remove: true, HasRemove: true,
	})
	require.NotContains(t, got, "BETA")
	require.Contains(t, got, "ALPHA")
	require.Equal(t, []string{`   OK  TestMod: REMOVE section ['BETA'] in METADATA\TABLES\F.MBIN`}, report)
}

// With no anchor at all, REMOVE empties the file: the scope is the whole
// document. The report says "None" where the keyword list would go, which is
// Python's repr of the absent group. R2.1.
func TestRemoveWithNoAnchorReportsANoneKeywordList(t *testing.T) {
	got, report := apply(t, sample, &modscript.Block{Remove: true, HasRemove: true})
	require.Empty(t, got)
	require.Equal(t, []string{`   OK  TestMod: REMOVE section None in METADATA\TABLES\F.MBIN`}, report)
}

// R2.3: ADD inserts its lines immediately after the selected section, and the
// report counts them.
func TestAddInsertsItsLinesAfterTheSelectedSection(t *testing.T) {
	got, report := apply(t, sample, &modscript.Block{
		SpecialKeyWords: []string{"ALPHA"}, HasSKW: true, SectionUp: 1,
		Add: "\t\t<Property name=\"New\" />\n\t\t<Property name=\"Also\" />", HasAdd: true,
	})
	out := lines(got)
	at := -1
	for i, l := range out {
		if strings.Contains(l, `name="New"`) {
			at = i
		}
	}
	require.Positive(t, at)
	require.Equal(t, "\t\t</Property>", out[at-1], "inserted right after the entry that held ALPHA closes")
	require.Contains(t, out[at+1], `name="Also"`)
	require.Contains(t, out[at+2], `name="GcEntry.xml"`, "and before the next entry")
	require.Equal(t, []string{`   OK  TestMod: ADD block in METADATA\TABLES\F.MBIN (2 lines)`}, report)
}

/*
R2.3: REPLACE_TYPE ALL inside one scope edits every match, not just the first.

Reached here through FOREACH_SKW_GROUP, because a bare SPECIAL_KEY_WORDS with
REPLACE_TYPE ALL takes the separate multi-section path instead.
*/
func TestReplaceTypeAllEditsEveryMatchInsideTheScope(t *testing.T) {
	got, report := apply(t, sample, &modscript.Block{
		ForEachSKWGroup: [][]string{{"Table"}},
		ReplaceType:     "ALL",
		ValueChanges:    vct("AmountMin", "5"), HasVCT: true,
	})
	require.Equal(t, 2, strings.Count(got, `name="AmountMin" value="5"`))
	require.Equal(t, []string{`   OK  TestMod: AmountMin -> 5 (2x) in METADATA\TABLES\F.MBIN`}, report)
}

// R2.1: a keyword group that matches nothing warns and moves on to the next
// group instead of aborting the block.
func TestAKeywordGroupThatMatchesNothingWarnsAndContinues(t *testing.T) {
	_, report := apply(t, sample, &modscript.Block{
		ForEachSKWGroup: [][]string{{"MISSING"}, {"BETA"}},
		SectionUp:       1,
		ValueChanges:    vct("AmountMax", "8"), HasVCT: true,
	})
	require.Equal(t, []string{
		`  WARN TestMod: SKW ['MISSING'] not found in METADATA\TABLES\F.MBIN`,
		`   OK  TestMod: AmountMax -> 8 (1x) in METADATA\TABLES\F.MBIN`,
	}, report)
}

/*
R2.1: a partly matching keyword group still anchors, on its last hit.

The reference loop breaks out on the first keyword it cannot find without clearing
the anchor it had already set, so ["GcEntry.xml", "NOPE"] behaves as
["GcEntry.xml"]. Mods in the golden set apply edits through this path; treating
it as "not found" would turn those into warnings and change the merge.
*/
func TestAPartlyMatchingKeywordGroupStillAnchorsOnItsLastHit(t *testing.T) {
	got, report := apply(t, sample, &modscript.Block{
		ForEachSKWGroup: [][]string{{"GcEntry.xml", "NOPE"}},
		ValueChanges:    vct("AmountMin", "42"), HasVCT: true,
	})
	require.Contains(t, got, `<Property name="AmountMin" value="42" />`)
	require.Equal(t, []string{`   OK  TestMod: AmountMin -> 42 (1x) in METADATA\TABLES\F.MBIN`}, report)
}

// A key the scope does not contain is a warning that names the key, and the
// key is recorded so the report can group them per mod. R2.1.
func TestAMissingKeyWarnsAndRecordsWhatWasNotFound(t *testing.T) {
	out, events := mxml.Apply(lines(sample), &modscript.Block{
		ValueChanges: vct("PulseRange", "1"), HasVCT: true,
	}, actx())
	require.Equal(t, sample, strings.Join(out, "\n"), "nothing changed")
	require.Len(t, events, 1)
	require.Equal(t, mxml.WARN, events[0].Kind)
	require.Equal(t, "PulseRange", events[0].NotFound)
	require.Equal(t, `  WARN TestMod: key 'PulseRange' not found in METADATA\TABLES\F.MBIN scope`, events[0].Line())
}

/*
R2.3: the REPLACE_TYPE ALL + SPECIAL_KEY_WORDS multi-section path.

One block edits every section the keywords match. The report names the keys as
a Python list, the operation, the match count and the last keyword -- the exact
shape 65 lines of the golden report take.
*/
func TestReplaceTypeAllWithKeywordsEditsEveryMatchingSection(t *testing.T) {
	got, report := apply(t, sample, &modscript.Block{
		SpecialKeyWords: []string{"GcEntry.xml"}, HasSKW: true,
		ReplaceType: "ALL", MathOperation: "*",
		ValueChanges: []modscript.ValueChange{
			{Key: "AmountMin", Value: modscript.IntValue(2)},
			{Key: "AmountMax", Value: modscript.IntValue(2)},
		}, HasVCT: true,
	})
	require.Contains(t, got, `name="AmountMin" value="20"`)
	require.Contains(t, got, `name="AmountMax" value="40"`)
	require.Contains(t, got, `name="AmountMin" value="3"`, "1.5 x 2, kept as a float spelling")
	require.Contains(t, got, `name="AmountMax" value="6"`)
	require.Equal(t, []string{
		`   OK  TestMod: ['AmountMin', 'AmountMax'] x* across 2 'GcEntry.xml' sections in METADATA\TABLES\F.MBIN`,
	}, report)
}

// The same path with no matches warns once, naming the section it looked for.
// R2.1.
func TestReplaceTypeAllWithNoMatchesWarnsOnce(t *testing.T) {
	_, report := apply(t, sample, &modscript.Block{
		SpecialKeyWords: []string{"GcNothing"}, HasSKW: true,
		ReplaceType: "ALL", ValueChanges: vct("AmountMin", "1"), HasVCT: true,
	})
	require.Equal(t, []string{
		`  WARN TestMod: no 'GcNothing' sections found in METADATA\TABLES\F.MBIN`,
	}, report)
}

/*
R2.3: SECTION_UP_TO selects the enclosing block by looking back for a marker,
and skips the match entirely when the marker is not within twelve lines.

Skipping rather than falling back is what stops a mod from editing whichever
section happens to be nearby when the structure it was written against is gone.
*/
func TestSectionUpToSelectsByMarkerAndSkipsWhenTheMarkerIsTooFarBack(t *testing.T) {
	found, report := apply(t, sample, &modscript.Block{
		SpecialKeyWords: []string{"BETA"}, HasSKW: true, SectionUpTo: `name="GcEntry.xml"`,
		ReplaceType: "ALL", ValueChanges: vct("AmountMax", "31"), HasVCT: true,
	})
	require.Contains(t, found, `name="AmountMax" value="31"`)
	require.Equal(t, []string{
		`   OK  TestMod: ['AmountMax'] x= across 1 'BETA' sections in METADATA\TABLES\F.MBIN`,
	}, report)

	_, missing := apply(t, sample, &modscript.Block{
		SpecialKeyWords: []string{"BETA"}, HasSKW: true, SectionUpTo: "NO-SUCH-MARKER",
		ReplaceType: "ALL", ValueChanges: vct("AmountMax", "31"), HasVCT: true,
	})
	require.Equal(t, []string{
		`  WARN TestMod: no 'BETA' sections found in METADATA\TABLES\F.MBIN`,
	}, missing)
}

/*
R2.1: CURRENCY_MULT multiplies the amounts in every GcRewardMoney block whose
currency matches, once each.

The count in the report is blocks visited, not lines edited, and the multiplier
prints as a Python float ("x5.0") because the reference code called float() on it.
*/
func TestCurrencyMultScalesOnlyTheMatchingCurrencyBlocks(t *testing.T) {
	src := `<Data>
	<Property name="GcRewardMoney">
		<Property name="Currency" value="Units" />
		<Property name="AmountMin" value="100" />
		<Property name="AmountMax" value="200" />
	</Property>
	<Property name="GcRewardMoney">
		<Property name="Currency" value="Nanites" />
		<Property name="AmountMin" value="10" />
	</Property>
</Data>`
	got, report := apply(t, src, &modscript.Block{
		CurrencyMult: &modscript.CurrencyMult{Currency: "Units", Mult: 5},
	})
	require.Contains(t, got, `name="AmountMin" value="500"`)
	require.Contains(t, got, `name="AmountMax" value="1000"`)
	require.Contains(t, got, `name="AmountMin" value="10"`, "the Nanites block is untouched")
	require.Equal(t, []string{
		`   OK  TestMod: CURRENCY_MULT Units x5.0 across 1 GcRewardMoney blocks in METADATA\TABLES\F.MBIN`,
	}, report)
}

// R2.1: WRAPPER_MULT scales the named keys inside every block with that
// wrapper, skipping the wrapper's own line.
func TestWrapperMultScalesTheNamedKeysInsideEveryWrapper(t *testing.T) {
	src := `<Data>
	<Property name="GcRewardStanding">
		<Property name="AmountMin" value="2" />
		<Property name="AmountMax" value="4" />
	</Property>
	<Property name="Other">
		<Property name="AmountMin" value="7" />
	</Property>
</Data>`
	got, report := apply(t, src, &modscript.Block{
		WrapperMult: &modscript.WrapperMult{
			Wrapper: "GcRewardStanding", Mult: 5, Keys: []string{"AmountMin", "AmountMax"},
		},
	})
	require.Contains(t, got, `name="AmountMin" value="10"`)
	require.Contains(t, got, `name="AmountMax" value="20"`)
	require.Contains(t, got, `name="AmountMin" value="7"`)
	require.Equal(t, []string{
		`   OK  TestMod: WRAPPER_MULT GcRewardStanding x5.0 across 1 blocks in METADATA\TABLES\F.MBIN`,
	}, report)
}

// A MULT the script wrote as a word is Python's ValueError, reported as the
// exception the reference builder printed rather than silently multiplying by
// zero. R2.1.
func TestANonNumericMultiplierIsReportedAsAnException(t *testing.T) {
	_, report := apply(t, sample, &modscript.Block{
		CurrencyMult: &modscript.CurrencyMult{
			Currency: "Units", MultErr: "could not convert string to float: 'lots'",
		},
	})
	require.Equal(t, []string{
		`  WARN TestMod: exception on METADATA\TABLES\F.MBIN: could not convert string to float: 'lots'`,
	}, report)
}

/*
R2.3: arithmetic on a value that is not a number writes the script's value
verbatim.

Python's float() raised, the except clause wrote str(val), and the merge went on.
Several reward tables carry string ids where a mod's block also names a numeric
key, so this path runs in a real build.
*/
func TestMathsOnANonNumericValueFallsBackToTheLiteralValue(t *testing.T) {
	got, report := apply(t, sample, &modscript.Block{
		MathOperation: "*",
		ValueChanges:  []modscript.ValueChange{{Key: "Id", Value: modscript.IntValue(5)}},
		HasVCT:        true,
	})
	require.Contains(t, got, `<Property name="Id" value="5" />`)
	require.Equal(t, []string{`   OK  TestMod: Id -> 5 (1x) in METADATA\TABLES\F.MBIN`}, report)
}

// Division by zero leaves the value alone rather than writing an infinity, and
// an operation the engine does not know falls back to the literal. R2.1.
func TestDivisionByZeroAndAnUnknownOperationAreBothSafe(t *testing.T) {
	got, _ := apply(t, sample, &modscript.Block{
		MathOperation: "/",
		ValueChanges:  []modscript.ValueChange{{Key: "AmountMin", Value: modscript.IntValue(0)}},
		HasVCT:        true,
	})
	require.Contains(t, got, `<Property name="AmountMin" value="10" />`)

	got, _ = apply(t, sample, &modscript.Block{
		MathOperation: "^",
		ValueChanges:  []modscript.ValueChange{{Key: "AmountMin", Value: modscript.IntValue(3)}},
		HasVCT:        true,
	})
	require.Contains(t, got, `<Property name="AmountMin" value="3" />`)
}

// Addition and subtraction exist too, and are the paths a "give me 50 more
// slots" mod takes. R2.1.
func TestAdditionAndSubtractionApply(t *testing.T) {
	got, _ := apply(t, sample, &modscript.Block{
		MathOperation: "+",
		ValueChanges:  []modscript.ValueChange{{Key: "AmountMin", Value: modscript.IntValue(5)}},
		HasVCT:        true,
	})
	require.Contains(t, got, `<Property name="AmountMin" value="15" />`)

	got, _ = apply(t, sample, &modscript.Block{
		MathOperation: "-",
		ValueChanges:  []modscript.ValueChange{{Key: "AmountMax", Value: modscript.IntValue(5)}},
		HasVCT:        true,
	})
	require.Contains(t, got, `<Property name="AmountMax" value="15" />`)
}

/*
R2.2/R2.3: FormatNum, the piece every numeric edit goes through.

Python's round() breaks ties to even, so 2.5 becomes 2 and 3.5 becomes 4. Two
reference mods multiply by values that land exactly on .5, and rounding half up
instead changes their merged output.
*/
func TestFormatNumRoundsTiesToEvenLikePython(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		old  string
		want string
	}{
		{2.5, "1", "2"}, {3.5, "1", "4"}, {-2.5, "1", "-2"}, {-3.5, "1", "-4"},
		{2.4, "1", "2"}, {2.6, "1", "3"},
	} {
		got, err := mxml.FormatNum(tc.in, tc.old, modscript.ITOFOff)
		require.NoError(t, err)
		require.Equal(t, tc.want, got, "round(%v)", tc.in)
	}
}

// An edit that overflows int32 clamps instead of wrapping: MBINCompiler writes
// the field without checking, and a wrapped negative reward is a crash in game.
// R2.2.
func TestFormatNumClampsToInt32(t *testing.T) {
	got, err := mxml.FormatNum(9e18, "1", modscript.ITOFOff)
	require.NoError(t, err)
	require.Equal(t, "2147483647", got)

	got, err = mxml.FormatNum(-9e18, "1", modscript.ITOFOff)
	require.NoError(t, err)
	require.Equal(t, "-2147483648", got)
}

/*
R2.2: a float result is rendered with "%f" and then stripped.

Six decimal places, trailing zeros removed, then a trailing dot removed. It is
lossy -- 1/3 comes out as 0.333333 -- and that is the rendering the reference
scripts were tuned against, so the shortest round-trip form is not a substitute.
*/
func TestFormatNumStripsTrailingZerosFromTheSixDecimalForm(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		old  string
		want string
	}{
		{1.5, "1.0", "1.5"}, {2.0, "1.0", "2"}, {0, "1.0", "0"},
		{1.0 / 3.0, "1.0", "0.333333"}, {1e-9, "1.0", "0"}, {-0.5, "1.0", "-0.5"},
	} {
		got, err := mxml.FormatNum(tc.in, tc.old, modscript.ITOFOff)
		require.NoError(t, err)
		require.Equal(t, tc.want, got, "%v against old %q", tc.in, tc.old)
	}
}

// R2.2: INTEGER_TO_FLOAT decides the kind when the old value's spelling would
// have decided differently.
func TestIntegerToFloatOverridesTheOldValuesKind(t *testing.T) {
	forced, err := mxml.FormatNum(3, "1", modscript.ITOFForce)
	require.NoError(t, err)
	require.Equal(t, "3", forced, `"%f" of 3.0 strips back to "3"`)

	forced, err = mxml.FormatNum(3.25, "1", modscript.ITOFForce)
	require.NoError(t, err)
	require.Equal(t, "3.25", forced, "an integer old value would have rounded this to 3")

	preserved, err := mxml.FormatNum(3.25, "1", modscript.ITOFPreserve)
	require.NoError(t, err)
	require.Equal(t, "3", preserved, "PRESERVE keeps the old value's integer spelling")

	preserved, err = mxml.FormatNum(3.25, "1.0", modscript.ITOFPreserve)
	require.NoError(t, err)
	require.Equal(t, "3.25", preserved)
}

// An infinite result is what Python's round() refused with an OverflowError,
// which sent the caller to its literal-value fallback. R2.2.
func TestAnInfiniteIntegerResultIsRefused(t *testing.T) {
	inf := math.Inf(1)

	_, err := mxml.FormatNum(inf, "1", modscript.ITOFOff)
	require.ErrorIs(t, err, mxml.ErrNotFinite)

	got, err := mxml.FormatNum(inf, "1.0", modscript.ITOFOff)
	require.NoError(t, err)
	require.Equal(t, "inf", got, "the float path prints Python's spelling")
}

// R2.2: the line helpers, which every branch above is built out of.
func TestLineHelpersReadAndWriteTheFirstValueAttribute(t *testing.T) {
	line := `			<Property name="A" value="10" extra="value=&quot;x&quot;" />`
	got, ok := mxml.GetVal(line)
	require.True(t, ok)
	require.Equal(t, "10", got)
	require.Contains(t, mxml.SetVal(line, "99"), `name="A" value="99"`)

	_, ok = mxml.GetVal(`	<Property name="A" />`)
	require.False(t, ok)
	require.Equal(t, `<x/>`, mxml.SetVal(`<x/>`, "9"), "a line with no value is returned unchanged")

	require.Equal(t, 3, mxml.NTabs("\t\t\t<Property"))
	require.Equal(t, 0, mxml.NTabs("<Property"))
}

// CloseIndex is the scope finder; its fallback to the last line when the
// document does not close is what the SECTION_UP quirk above rides on. R2.2.
func TestCloseIndexFindsTheMatchingCloseAndFallsBackToTheLastLine(t *testing.T) {
	l := lines(sample)
	require.Equal(t, 7, mxml.CloseIndex(l, 3), "the first GcEntry.xml closes on line 7")
	require.Equal(t, len(l)-1, mxml.CloseIndex(l, 4), "a self-closing line never balances")
}

// FindKW and FindProp are bounded searches; an end past the slice is clamped
// rather than panicking, which matters because `end` is captured before ADD
// grows the file. R2.2.
func TestTheSearchHelpersClampTheirBounds(t *testing.T) {
	l := lines(sample)
	require.Equal(t, 4, mxml.FindKW(l, `"ALPHA"`, 0, 9999))
	require.Equal(t, -1, mxml.FindKW(l, "ALPHA", 6, 9999))
	require.Equal(t, 5, mxml.FindProp(l, "AmountMin", -1, 9999))
	require.Equal(t, -1, mxml.FindProp(l, "AmountMin", 0, 0))
}

// R2.1: the three report line shapes, which the golden set compares byte for
// byte.
func TestEventLinesUseTheReferenceColumns(t *testing.T) {
	require.Equal(t, "   OK  M: d", mxml.Event{Kind: mxml.OK, Mod: "M", Detail: "d"}.Line())
	require.Equal(t, "  WARN M: d", mxml.Event{Kind: mxml.WARN, Mod: "M", Detail: "d"}.Line())
	require.Equal(t, "       d", mxml.Info("d").Line())
	require.Equal(t, "ok", mxml.OK.String())
	require.Equal(t, "warn", mxml.WARN.String())
	require.Equal(t, "info", mxml.INFO.String())
}

// R2.1: Python's list and string reprs, which appear inside those lines.
func TestPyListMatchesPythonsRepr(t *testing.T) {
	require.Equal(t, "['AmountMin', 'AmountMax']", mxml.PyList([]string{"AmountMin", "AmountMax"}))
	require.Equal(t, "[]", mxml.PyList([]string{}))
	require.Equal(t, "None", mxml.PyList(nil))
	require.Equal(t, `"it's"`, mxml.PyQuote("it's"))
	require.Equal(t, `'say "hi"'`, mxml.PyQuote(`say "hi"`))
	require.Equal(t, `'a\\b'`, mxml.PyQuote(`a\b`))
}

// Base is POSIX basename: a Windows-spelled source keeps its backslashes, which
// is how every report line in the golden set quotes it. R2.1.
func TestBaseSplitsOnForwardSlashesOnly(t *testing.T) {
	require.Equal(t, `METADATA\TABLES\F.MBIN`, mxml.Base(`METADATA\TABLES\F.MBIN`))
	require.Equal(t, "F.MBIN", mxml.Base("metadata/tables/F.MBIN"))
	require.Equal(t, "F.MBIN", mxml.Base("F.MBIN"))
}
