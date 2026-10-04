package audit_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/build/audit"
)

// Spec 017. Every number here is hand-picked; the shapes are the ones a real
// build produced (a x10 loot multiplier under a x15 mission-board one, and a
// x6 currency multiplier under a x100 units one).

func knob(mod, param string, current float64, cats ...string) audit.Knob {
	return audit.Knob{Mod: mod, Param: param, Current: current, Min: 1, Categories: cats}
}

func flag(category string, stock float64, chain ...audit.Contribution) audit.Flag {
	v := stock
	for i := range chain {
		v *= chain[i].Factor
		chain[i].Max = v
	}
	return audit.Flag{
		Category: category, PristineMax: stock, MergedMax: v,
		Ratio: v / stock, Contributors: chain,
	}
}

func changed(p audit.Plan) map[string]float64 {
	out := map[string]float64{}
	for _, c := range p.Changes {
		out[c.Mod] = c.New
	}
	return out
}

func by(mod string, factor float64) audit.Contribution {
	return audit.Contribution{Mod: mod, Factor: factor}
}

func TestAFlagOverTheRatioLimitLowersTheLastMultiplierInItsChain(t *testing.T) {
	flags := []audit.Flag{
		flag(audit.CategoryProduct, 2, by("Chest", 10), by("Mission", 15)),
		flag(audit.CategorySubstance, 40, by("Chest", 10), by("Mission", 15)),
	}
	knobs := []audit.Knob{
		knob("Chest", "LOOT", 10, audit.CategoryProduct, audit.CategorySubstance),
		knob("Mission", "ITEM", 15, audit.CategoryProduct, audit.CategorySubstance),
	}
	plan := audit.Recommend(flags, audit.Defaults(), knobs)

	require.Len(t, plan.Changes, 1, "the earlier multiplier is left alone")
	require.Equal(t, "Mission", plan.Changes[0].Mod)
	require.InDelta(t, 10, plan.Changes[0].New, 0, "x10 * x10 is the x100 limit, not over it")
	require.Equal(t, 2, plan.Changes[0].Flags)
	require.Equal(t, 2, plan.Fixed)
	require.Zero(t, plan.Remaining)
}

func TestAnAbsoluteLimitIsMetToo(t *testing.T) {
	// 750000 units x6 x100 = 450M over the 100M limit, and x600 over x100.
	flags := []audit.Flag{flag(audit.CategoryUnits, 750000, by("Money", 6), by("Mission", 100))}
	knobs := []audit.Knob{
		knob("Money", "CUR", 6, audit.CategoryUnits, audit.CategoryNanites),
		knob("Mission", "UNITS", 100, audit.CategoryUnits),
	}
	plan := audit.Recommend(flags, audit.Defaults(), knobs)

	require.Len(t, plan.Changes, 1)
	// 750000 * 6 * n <= 100M  =>  n <= 22.2; x6 * n <= x100  =>  n <= 16.6.
	require.InDelta(t, 16, plan.Changes[0].New, 0)
	require.Zero(t, plan.Remaining)
}

func TestAKnobAtItsMinimumHandsTheRestToTheOneBeforeIt(t *testing.T) {
	flags := []audit.Flag{flag(audit.CategoryProduct, 10, by("Chest", 50), by("Mission", 4))}
	knobs := []audit.Knob{
		knob("Chest", "LOOT", 50, audit.CategoryProduct),
		knob("Mission", "ITEM", 4, audit.CategoryProduct),
	}
	plan := audit.Recommend(flags, audit.Defaults(), knobs)

	require.Equal(t, map[string]float64{"Mission": 2}, changed(plan), "x50 * x2 is x100")
	require.Zero(t, plan.Remaining)

	// Needing more than the last knob can give: x200 * x4 must come under x100.
	flags = []audit.Flag{flag(audit.CategoryProduct, 10, by("Chest", 200), by("Mission", 4))}
	knobs[0].Current = 200
	plan = audit.Recommend(flags, audit.Defaults(), knobs)
	require.Equal(t, map[string]float64{"Mission": 1, "Chest": 100}, changed(plan),
		"the last one goes to its minimum and the one before it takes the rest")
	require.Zero(t, plan.Remaining)
}

func TestALibraryScriptIsNamedAndNeverChanged(t *testing.T) {
	flags := []audit.Flag{flag(audit.CategoryProduct, 2, by("BetterRewards", 250), by("Chest", 10))}
	knobs := []audit.Knob{knob("Chest", "LOOT", 10, audit.CategoryProduct)}
	plan := audit.Recommend(flags, audit.Defaults(), knobs)

	require.Equal(t, 1, plan.Remaining, "x250 alone is over x100; no built-in can undo it")
	require.Equal(t, []audit.Unfixed{{Mod: "BetterRewards", Flags: 1}}, plan.Unfixed[:1])
	for _, c := range plan.Changes {
		require.NotEqual(t, "BetterRewards", c.Mod)
	}
}

func TestAKnobOnlyMovesForTheCategoryItScales(t *testing.T) {
	flags := []audit.Flag{flag(audit.CategoryNanites, 100, by("Mission", 500))}
	knobs := []audit.Knob{knob("Mission", "ITEM", 500, audit.CategoryProduct)}
	plan := audit.Recommend(flags, audit.Defaults(), knobs)

	require.Empty(t, plan.Changes, "an item multiplier does not scale nanites")
	require.Equal(t, 1, plan.Remaining)
}

func TestACapHoldingAContributorIsPredictedAsStillHolding(t *testing.T) {
	// Stock 1000, a x100 knob capped at 50000 (applied factor x50), then a
	// x10 library: 500000, x500. Lowering the knob to 20 changes nothing (the
	// cap still holds at x50 > x20 -> x20, 20000 * 10 = x200), so it has to go
	// to 10: x10 * x10 = x100.
	flags := []audit.Flag{flag(audit.CategorySubstance, 1000, by("Chest", 50), by("Lib", 10))}
	knobs := []audit.Knob{knob("Chest", "LOOT", 100, audit.CategorySubstance)}
	plan := audit.Recommend(flags, audit.Defaults(), knobs)

	require.Len(t, plan.Changes, 1)
	require.InDelta(t, 10, plan.Changes[0].New, 0)
	require.Zero(t, plan.Remaining)
}

func TestNothingFlaggedIsNothingToChange(t *testing.T) {
	plan := audit.Recommend(nil, audit.Defaults(), []audit.Knob{knob("Chest", "LOOT", 10, "product")})
	require.Empty(t, plan.Changes)
	require.Zero(t, plan.Fixed)
	require.Zero(t, plan.Remaining)
}
