package audit_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/build/audit"
)

/*
Every fixture here is hand-written (project rule: nothing extracted from the
game is committed). They imitate MBINCompiler's output -- tab indentation, one
<Property> per line, self-closing leaves -- closely enough that the depth
arithmetic the parser does is the arithmetic it will do in anger, which is the
only property of the real file that matters to it.
*/
const table = `<?xml version="1.0" encoding="utf-8"?>
<Data template="cGcRewardTable">
	<Property name="GenericTable">
		<Property name="GenericTable" value="GcGenericRewardTableEntry" _id="R_CHEST">
			<Property name="Id" value="R_CHEST" />
			<Property name="List" value="GcRewardTableItemList">
				<Property name="List">
					<Property name="List" value="GcRewardTableItem" _index="0">
						<Property name="Reward" value="GcRewardSpecificProduct">
							<Property name="GcRewardSpecificProduct">
								<Property name="ID" value="BP_SALVAGE" />
								<Property name="AmountMin" value="2" />
								<Property name="AmountMax" value="4" />
								<Property name="HideAmountInMessage" value="false" />
							</Property>
						</Property>
					</Property>
					<Property name="List" value="GcRewardTableItem" _index="1">
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
		</Property>
		<Property name="GenericTable" value="GcGenericRewardTableEntry" _id="R_CAVE">
			<Property name="Id" value="R_CAVE" />
			<Property name="List">
				<Property name="Reward" value="GcRewardSpecificSubstance">
					<Property name="GcRewardSpecificSubstance">
						<Property name="ID" value="AF_METAL" />
						<Property name="AmountMin" value="100" />
						<Property name="AmountMax" value="300" />
					</Property>
				</Property>
			</Property>
			<Property name="List">
				<Property name="Reward" value="GcRewardMultiSpecificItems">
					<Property name="GcRewardMultiSpecificItems">
						<Property name="Items">
							<Property name="Items" value="GcMultiSpecificItemEntry" _id="BP_SALVAGE">
								<Property name="MultiItemRewardType" value="Product" />
								<Property name="Id" value="BP_SALVAGE" />
								<Property name="Amount" value="3" />
							</Property>
							<Property name="Items" value="GcMultiSpecificItemEntry" _id="LAND1">
								<Property name="MultiItemRewardType" value="Substance" />
								<Property name="Id" value="LAND1" />
								<Property name="Amount" value="50" />
							</Property>
						</Property>
					</Property>
				</Property>
			</Property>
		</Property>
	</Property>
</Data>`

func lines(s string) []string { return strings.Split(s, "\n") }

/*
R1.1: the parser finds every reward payload, its enclosing entry and its item.

The entry Id is the part worth pinning. It is the nearest preceding Id at a
shallower depth, which is not the same as "the nearest preceding Id": a
multi-item reward carries an Id per item, one level deeper, and reading that as
the entry would label every flagged amount with the product's own name and lose
the entry a user can look up.
*/
func TestParseFindsEveryRewardBlockAndItsEntry(t *testing.T) {
	blocks := audit.Parse(lines(table))
	require.Len(t, blocks, 5)

	require.Equal(t, audit.KindSpecificProduct, blocks[0].Kind)
	require.Equal(t, "R_CHEST", blocks[0].EntryID)
	require.Equal(t, "BP_SALVAGE", blocks[0].Item)
	require.Equal(t, audit.CategoryProduct, blocks[0].Category)
	require.InDelta(t, 2.0, blocks[0].Min, 0)
	require.InDelta(t, 4.0, blocks[0].Max, 0)

	require.Equal(t, audit.KindMoney, blocks[1].Kind)
	require.Equal(t, "Units", blocks[1].Item, "the inner Currency, not the GcCurrency wrapper")
	require.Equal(t, audit.CategoryUnits, blocks[1].Category)
	require.InDelta(t, 5000.0, blocks[1].Max, 0)

	require.Equal(t, audit.KindSpecificSubstance, blocks[2].Kind)
	require.Equal(t, "R_CAVE", blocks[2].EntryID)
	require.Equal(t, audit.CategorySubstance, blocks[2].Category)

	// One block per item of a multi-item reward, each judged in its own
	// category, and both belonging to the entry rather than to themselves.
	require.Equal(t, audit.KindMultiItems, blocks[3].Kind)
	require.Equal(t, "R_CAVE", blocks[3].EntryID)
	require.Equal(t, "BP_SALVAGE", blocks[3].Item)
	require.Equal(t, audit.CategoryProduct, blocks[3].Category)
	require.InDelta(t, 3.0, blocks[3].Max, 0)
	require.Equal(t, "LAND1", blocks[4].Item)
	require.Equal(t, audit.CategorySubstance, blocks[4].Category)
}

// The outer `<Property name="Reward" value="Gc…">` introduces the payload and
// the inner one opens it. Counting both would double every reward in the table.
func TestTheRewardIntroducerIsNotCountedAsAPayload(t *testing.T) {
	for _, b := range audit.Parse(lines(table)) {
		require.NotEmpty(t, b.Category, "%s/%s has no category", b.EntryID, b.Item)
	}
	require.Len(t, audit.Parse(lines(table)), 5)
}

// multiplied returns the table with every AmountMin/AmountMax scaled, which is
// what a merge does to it.
func multiplied(src string, factor int) string {
	out := make([]string, 0, 64)
	for _, line := range lines(src) {
		out = append(out, scale(line, factor))
	}
	return strings.Join(out, "\n")
}

func scale(line string, factor int) string {
	for _, key := range []string{"AmountMin", "AmountMax", "Amount"} {
		prefix := `<Property name="` + key + `" value="`
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		rest := trimmed[len(prefix):]
		v := rest[:strings.IndexByte(rest, '"')]
		n := 0
		for _, c := range v {
			n = n*10 + int(c-'0')
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, "\t"))]
		return indent + prefix + itoa(n*factor) + `" />`
	}
	return line
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

/*
R1.2: a merged amount over its category's limit is flagged, and so is a ratio.

Both tests matter and they catch different things. The absolute limit catches
"this reward cannot fit in a stack"; the ratio catches "this is a thousand times
what the game shipped", which is the signal that says something compounded even
when the result is still technically holdable.
*/
func TestAnAmountOverItsLimitIsFlagged(t *testing.T) {
	stock := audit.Parse(lines(table))
	merged := audit.Parse(lines(multiplied(table, 100000)))
	res := audit.Compare("REWARDTABLE", stock, merged, audit.Defaults())

	require.Zero(t, res.Unauditable)
	byItem := map[string]audit.Flag{}
	for _, f := range res.Flags {
		byItem[f.EntryID+"/"+f.Item] = f
	}
	salvage, ok := byItem["R_CHEST/BP_SALVAGE"]
	require.True(t, ok, "a product at 400000 is over the 99999 product limit")
	require.InDelta(t, 100000.0, salvage.Ratio, 0)
	require.Contains(t, strings.Join(salvage.Reasons, "; "), "product amount 400000 over the 99999 limit")
	require.Contains(t, strings.Join(salvage.Reasons, "; "), "x100000 over the x100 limit")
}

// A ratio limit low enough catches the ordinary x10 loot multiplier, which is
// what makes the threshold worth having as a setting rather than a constant.
func TestALowerRatioLimitFlagsAnOrdinaryMultiplier(t *testing.T) {
	stock := audit.Parse(lines(table))
	merged := audit.Parse(lines(multiplied(table, 10)))

	clean := audit.Compare("REWARDTABLE", stock, merged, audit.Defaults())
	require.Empty(t, clean.Flags, "x10 is under every default limit")

	th := audit.Defaults()
	th.MaxRatio = 5
	strict := audit.Compare("REWARDTABLE", stock, merged, th)
	require.Len(t, strict.Flags, 5, "every amount moved by x10")
	for _, f := range strict.Flags {
		require.Contains(t, strings.Join(f.Reasons, "; "), "x10 over the x5 limit")
	}
}

/*
R1.2: an int32-saturated value is always flagged, whatever the thresholds say.

The engine clamps rather than overflowing, so 2147483647 is never a number a
script asked for -- it is the mark left by arithmetic that ran off the end of
the field. A threshold set high enough to allow it would be allowing a value the
game will read as "the biggest integer there is".
*/
func TestInt32SaturationIsAlwaysFlagged(t *testing.T) {
	stock := audit.Parse(lines(table))
	merged := audit.Parse(lines(table))
	merged[1].Max = audit.MaxInt32

	th := audit.Thresholds{
		MaxProduct: 1e12, MaxSubstance: 1e12, MaxUnits: 1e12,
		MaxNanites: 1e12, MaxSpecials: 1e12, MaxRatio: 1e12,
	}
	res := audit.Compare("REWARDTABLE", stock, merged, th)
	require.Len(t, res.Flags, 1)
	require.Equal(t, []string{"int32 saturation"}, res.Flags[0].Reasons)
}

/*
R1.1: when a structural edit changed the block count, pairing falls back to the
key and the remainder is reported rather than compared.

Comparing by position after an ADD would judge every block after the insertion
against the stock value of a different reward, which produces a page of
confident nonsense. Counting them instead is the honest answer, and it is what
the report's "not auditable" line says.
*/
func TestAStructuralChangeIsPairedByKeyAndTheRemainderCounted(t *testing.T) {
	stock := audit.Parse(lines(table))
	merged := audit.Parse(lines(multiplied(table, 100000)))
	// A mod added a reward the stock table has no counterpart for.
	merged = append(merged, audit.Block{
		Kind: audit.KindSpecificProduct, EntryID: "R_NEW", Item: "NEW_ITEM",
		Category: audit.CategoryProduct, Min: 1, Max: 1,
	})

	res := audit.Compare("REWARDTABLE", stock, merged, audit.Defaults())
	require.Equal(t, 1, res.Unauditable, "the added block has no stock value to compare against")
	require.NotEmpty(t, res.Flags, "the blocks that did pair are still judged")
	for _, f := range res.Flags {
		require.NotEqual(t, "R_NEW", f.EntryID)
	}
}

/*
R1.3: the tracker names each mod that moved a flagged amount, in order.

A mod appears once per edit that changed the value, not once per mod, which is
what makes a script that multiplies the same reward twice legible: "x250 then
x250" is the fact, and reporting it as one x62500 pass hides that a single
script did it to itself.
*/
func TestTheTrackerAttributesEachChangeInOrder(t *testing.T) {
	stock := audit.Parse(lines(table))
	final := multiplied(multiplied(multiplied(table, 250), 250), 10)
	merged := audit.Parse(lines(final))
	res := audit.Compare("REWARDTABLE", stock, merged, audit.Defaults())
	require.NotEmpty(t, res.Flags)

	tracker := audit.NewTracker(res.Flags, stock, len(lines(table)), len(merged))
	running := table
	for _, pass := range []struct {
		mod    string
		factor int
	}{{"BetterRewards", 250}, {"BetterRewards", 250}, {"ChestAndLootMaterials10x", 10}} {
		running = multiplied(running, pass.factor)
		tracker.Observe(pass.mod, lines(running))
	}

	var salvage audit.Flag
	for _, f := range tracker.Flags() {
		if f.EntryID == "R_CHEST" && f.Kind == audit.KindSpecificProduct {
			salvage = f
		}
	}
	require.Len(t, salvage.Contributors, 3)
	require.Equal(t, "BetterRewards x250 -> 1000", salvage.Contributors[0].String())
	require.Equal(t, "BetterRewards x250 -> 250000", salvage.Contributors[1].String())
	require.Equal(t, "ChestAndLootMaterials10x x10 -> 2500000", salvage.Contributors[2].String())
	require.Equal(t,
		"BetterRewards x250 -> 1000, BetterRewards x250 -> 250000, "+
			"ChestAndLootMaterials10x x10 -> 2500000", salvage.ContributorText())
}

// An edit that changed none of the flagged amounts contributes nothing, so the
// contributor list names the mods that moved the number rather than every mod
// that touched the file.
func TestAPassThatChangesNothingIsNotAContributor(t *testing.T) {
	stock := audit.Parse(lines(table))
	merged := audit.Parse(lines(multiplied(table, 100000)))
	res := audit.Compare("REWARDTABLE", stock, merged, audit.Defaults())

	tracker := audit.NewTracker(res.Flags, stock, len(lines(table)), len(merged))
	tracker.Observe("DoesNothing", lines(table))
	tracker.Observe("TheMultiplier", lines(multiplied(table, 100000)))

	for _, f := range tracker.Flags() {
		require.Len(t, f.Contributors, 1)
		require.Equal(t, "TheMultiplier", f.Contributors[0].Mod)
	}
}

// Only the two reward tables are audited, and the match is on the basename
// because the same file is spelled six ways between a script and the cache.
func TestOnlyTheRewardTablesAreAudited(t *testing.T) {
	for _, spelling := range []string{
		`METADATA\REALITY\TABLES\REWARDTABLE.MBIN`,
		"metadata/reality/tables/rewardtable.mbin",
		"METADATA/REALITY/TABLES/REWARDTABLE.MXML",
	} {
		name, ok := audit.TableName(spelling)
		require.Truef(t, ok, "%s should be audited", spelling)
		require.Equal(t, "REWARDTABLE", name)
	}
	name, ok := audit.TableName("METADATA/REALITY/TABLES/EXPEDITIONREWARDTABLE.MBIN")
	require.True(t, ok)
	require.Equal(t, "EXPEDITIONREWARDTABLE", name)

	_, ok = audit.TableName(`METADATA\GAMESTATE\DIFFICULTYCONFIG.MBIN`)
	require.False(t, ok, "a file whose amounts cannot compound is not audited")
}

// The summary line is what the CLI prints after a build, so its three states
// are pinned: nothing to audit, audited and clean, and audited and not.
func TestTheSummaryLineSaysWhichOfThreeThingsHappened(t *testing.T) {
	var absent *audit.Result
	require.Equal(t, "not run", absent.Summary())
	require.True(t, absent.Clean())

	clean := &audit.Result{}
	require.Equal(t, "clean", clean.Summary())
	require.True(t, clean.Clean())

	partial := &audit.Result{Unauditable: 3}
	require.Equal(t, "clean (3 block(s) not auditable)", partial.Summary())

	flagged := &audit.Result{Flags: []audit.Flag{{}, {}}}
	require.Equal(t, "2 flagged; see report", flagged.Summary())
	require.False(t, flagged.Clean())
}

// Worst first, by how far the value moved rather than by how large it is: a
// large stock payout multiplied by two is fine and a stock 4 at 2,500,000 is not.
func TestFlagsSortByRatio(t *testing.T) {
	res := &audit.Result{Flags: []audit.Flag{
		{EntryID: "small", Ratio: 5, MergedMax: 10000000},
		{EntryID: "huge", Ratio: 625000, MergedMax: 2500000},
		{EntryID: "middling", Ratio: 250, MergedMax: 1000},
	}}
	res.Sort()
	require.Equal(t, []string{"huge", "middling", "small"},
		[]string{res.Flags[0].EntryID, res.Flags[1].EntryID, res.Flags[2].EntryID})
}
