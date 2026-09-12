package core_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/tweaks"
)

// R2.1: the listing carries everything a card needs, grouped, in build order.
func TestListTweaksDescribesEveryBuiltIn(t *testing.T) {
	bare(t)

	res, err := core.ListTweaks(t.Context(), core.ListTweaksRequest{})
	require.NoError(t, err)
	require.Len(t, res.Tweaks, len(tweaks.Names()))
	require.NotEmpty(t, res.Groups)
	require.Contains(t, res.Groups, "Mining")

	for i, tw := range res.Tweaks {
		require.NotEmpty(t, tw.Title)
		require.NotEmpty(t, tw.Desc)
		require.NotEmpty(t, tw.Group)
		require.Equal(t, i+1, tw.Order, "the listing is the build order")
		require.False(t, tw.Enabled)
		require.NotEmpty(t, tw.Params)
		for _, p := range tw.Params {
			require.False(t, p.Overridden)
			require.Equal(t, p.Default, p.Current)
		}
	}
	require.False(t, res.Unbuilt, "nothing has been changed, so nothing is unbuilt")
}

/*
R1.3/R2.3: setting a parameter records an override and the script is untouched.

The second half is the one worth pinning. The value lives in config.json and the
substitution happens in memory at build time, so a user who changes a multiplier
and then upgrades nmsbonker gets the new script with their number in it, rather
than an edited copy of the old one.
*/
func TestSettingAParameterRecordsAnOverrideWithoutEditingTheScript(t *testing.T) {
	bare(t)

	before, ok := tweaks.Source("MaterialYield10x")
	require.True(t, ok)

	res, err := core.SetTweakParam(t.Context(), core.SetTweakParamRequest{
		Name: "MaterialYield10x", Param: "MATERIAL_MULTIPLIER", Value: 20,
	})
	require.NoError(t, err)
	require.Equal(t, 10.0, res.Old)
	require.Equal(t, 20.0, res.New)
	require.Equal(t, 10.0, res.Default)
	require.False(t, res.Clamped)

	after, ok := tweaks.Source("MaterialYield10x")
	require.True(t, ok)
	require.Equal(t, before, after, "the embedded script is not modified")

	list, err := core.ListTweaks(t.Context(), core.ListTweaksRequest{})
	require.NoError(t, err)
	p := findParam(t, list, "MaterialYield10x", "MATERIAL_MULTIPLIER")
	require.Equal(t, 20.0, p.Current)
	require.True(t, p.Overridden)
	require.True(t, list.Unbuilt, "nothing has been built with this value")
}

/*
AC2: the override reaches the change table the engine acts on.

Asserted on the decoded container `mods check --json` prints, because that is
what the build's own loader produced: CheckMods and Build share scriptSource, so
a dump carrying 20 is a build carrying 20. MaterialYield10x edits AmountMin and
AmountMax on every mineable entity, so a stray unreplaced 10 would show up here.
*/
func TestAnOverrideReachesTheEditsTheBuildWillApply(t *testing.T) {
	bare(t)
	_, err := core.SetTweakParam(t.Context(), core.SetTweakParamRequest{
		Name: "MaterialYield10x", Param: "MATERIAL_MULTIPLIER", Value: 20,
	})
	require.NoError(t, err)
	_, err = core.SetModEnabled(t.Context(), core.SetModEnabledRequest{
		Names: []string{"MaterialYield10x"}, Enabled: true,
	})
	require.NoError(t, err)

	res, err := core.CheckMods(t.Context(), core.CheckModsRequest{IncludeDump: true})
	require.NoError(t, err)
	require.Len(t, res.Mods, 1, "only the enabled tweak was checked")
	require.Equal(t, core.SourceBuiltin, res.Mods[0].Source)
	require.True(t, res.Mods[0].OK)
	require.Greater(t, len(res.Mods[0].Targets), 50,
		"MaterialYield10x edits every mineable entity")

	dump := string(res.Mods[0].Dump)
	require.Contains(t, dump, `["AmountMin",20]`)
	require.Contains(t, dump, `["AmountMax",20]`)
	require.NotContains(t, dump, `["AmountMin",10]`, "the script's own value is gone")
	require.NotContains(t, dump, `["AmountMax",10]`)

	// The parameter the front ends draw agrees with what was decoded.
	require.Len(t, res.Mods[0].Params, 1)
	require.Equal(t, 20.0, res.Mods[0].Params[0].Current)
	require.Equal(t, 10.0, res.Mods[0].Params[0].Default)
}

// R2.3: an out-of-range value is clamped to the declared bounds and says so,
// rather than being refused. A slider cannot produce one; typing can.
func TestAnOutOfRangeValueIsClampedAndSaysSo(t *testing.T) {
	bare(t)

	res, err := core.SetTweakParam(t.Context(), core.SetTweakParamRequest{
		Name: "LearnMoreWords", Param: "WORD_MULT", Value: 5000,
	})
	require.NoError(t, err)
	require.True(t, res.Clamped)
	require.Equal(t, 50.0, res.New)

	// An integer parameter takes whole numbers only: the engine's integer and
	// float values produce differently-shaped MXML.
	res, err = core.SetTweakParam(t.Context(), core.SetTweakParamRequest{
		Name: "LearnMoreWords", Param: "WORD_MULT", Value: 3.7,
	})
	require.NoError(t, err)
	require.Equal(t, 4.0, res.New)
}

// A float parameter keeps its fraction, and is written into the change table as
// a float: the engine's integer/float split decides the shape of the MXML.
func TestAFloatParameterKeepsItsFraction(t *testing.T) {
	bare(t)

	res, err := core.SetTweakParam(t.Context(), core.SetTweakParamRequest{
		Name: "SpaceMiningBoost", Param: "VOXEL_CHANCE", Value: 0.35,
	})
	require.NoError(t, err)
	require.Equal(t, 0.35, res.New)

	check, err := core.CheckMods(t.Context(), core.CheckModsRequest{All: true, IncludeDump: true})
	require.NoError(t, err)
	for _, m := range check.Mods {
		if m.Name != "SpaceMiningBoost" {
			continue
		}
		require.True(t, m.OK, m.Error)
		require.Contains(t, string(m.Dump), `["VoxelAsteroidResourceChance",0.35]`)
		return
	}
	t.Fatal("SpaceMiningBoost was not checked")
}

// R2.3: reset drops the override, so the script's own number applies again.
func TestResetRestoresTheScriptsOwnValues(t *testing.T) {
	bare(t)
	for _, p := range []string{"SUBSTANCE", "PRODUCT"} {
		_, err := core.SetTweakParam(t.Context(), core.SetTweakParamRequest{
			Name: "BigStacks", Param: p, Value: 42,
		})
		require.NoError(t, err)
	}

	res, err := core.ResetTweak(t.Context(), core.ResetTweakRequest{Name: "BigStacks", Param: "SUBSTANCE"})
	require.NoError(t, err)
	require.Equal(t, []string{"SUBSTANCE"}, res.Reset)

	res, err = core.ResetTweak(t.Context(), core.ResetTweakRequest{Name: "BigStacks"})
	require.NoError(t, err)
	require.Equal(t, []string{"PRODUCT"}, res.Reset)

	list, err := core.ListTweaks(t.Context(), core.ListTweaksRequest{})
	require.NoError(t, err)
	for _, p := range findTweak(t, list, "BigStacks").Params {
		require.False(t, p.Overridden)
		require.Equal(t, p.Default, p.Current)
	}
}

// A parameter the script does not declare is named, along with the ones it does.
func TestSettingAnUnknownParameterSaysWhatThereIs(t *testing.T) {
	bare(t)

	_, err := core.SetTweakParam(t.Context(), core.SetTweakParamRequest{
		Name: "BigStacks", Param: "NOPE", Value: 1,
	})
	require.ErrorIs(t, err, core.ErrNoSuchParam)
	require.ErrorContains(t, err, "SUBSTANCE, PRODUCT")

	_, err = core.SetTweakParam(t.Context(), core.SetTweakParamRequest{
		Name: "NoSuchMod", Param: "X", Value: 1,
	})
	require.ErrorContains(t, err, `no mod named "NoSuchMod"`)
}

// --- helpers ---------------------------------------------------------------

func findTweak(t *testing.T, list core.ListTweaksResult, name string) core.TweakInfo {
	t.Helper()
	for _, tw := range list.Tweaks {
		if tw.Name == name {
			return tw
		}
	}
	t.Fatalf("no tweak named %q", name)
	return core.TweakInfo{}
}

func findParam(t *testing.T, list core.ListTweaksResult, mod, param string) core.TweakParam {
	t.Helper()
	for _, p := range findTweak(t, list, mod).Params {
		if p.Name == param {
			return p
		}
	}
	t.Fatalf("%s has no parameter %q", mod, param)
	return core.TweakParam{}
}
