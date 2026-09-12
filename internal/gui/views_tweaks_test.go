package gui

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/modscript"
)

/*
tweaksUI is a window pointed at a throwaway settings file and a throwaway
library, with the built-ins loaded through core exactly as the section loads
them.

The XDG directories are redirected as well as the config path. Without that the
library the section reconciles against is whatever this machine happens to
have, and a test whose result depends on the developer's mod collection is not
a test.
*/
func tweaksUI(t *testing.T) (*ui, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("STEAM_ROOT", filepath.Join(root, "no-steam-here"))

	u := testUI(t)
	path := filepath.Join(root, "config.json")
	u.configPath = path

	res, err := core.ListTweaks(t.Context(), core.ListTweaksRequest{
		Request: core.Request{ConfigPath: path},
	})
	require.NoError(t, err)
	u.tweaks, u.tweaksOK = res, true
	return u, path
}

/*
Dragging a slider writes the value through core, and the script is untouched.

The whole point of the section: a control on screen and a number in config.json,
with the .lua the number belongs to never modified. Driven synchronously because
a window with no content pane runs its operations on the calling goroutine --
see perform.
*/
func TestReleasingASliderWritesTheParameter(t *testing.T) {
	u, path := tweaksUI(t)
	tw := findTweak(t, u, "MaterialYield10x")
	p := tw.Params[0]

	row := u.paramRow(tw.Name, p, testLabel())
	require.NotNil(t, row)

	// The row's slider is the control the value is committed from.
	slider := findSlider(t, row)
	require.NotNil(t, slider)
	require.Equal(t, p.Min, slider.Min)
	require.Equal(t, p.Max, slider.Max)
	require.Equal(t, p.Current, slider.Value)

	slider.OnChangeEnded(20)

	res, err := core.ListTweaks(t.Context(), core.ListTweaksRequest{
		Request: core.Request{ConfigPath: path},
	})
	require.NoError(t, err)
	after := findParamIn(t, res, "MaterialYield10x", p.Name)
	require.Equal(t, 20.0, after.Current)
	require.True(t, after.Overridden)

	// And the section's own model agrees, so a later rebuild draws 20 rather
	// than the value it was loaded with.
	require.Equal(t, 20.0, findParam(t, u, "MaterialYield10x", p.Name).Current)
	require.True(t, u.tweaks.Unbuilt)
}

// Typing a value that is not a number must not write anything, and must put the
// field back rather than leaving the rejected text where it looks accepted.
func TestTypingRubbishIntoAParameterIsRefused(t *testing.T) {
	u, path := tweaksUI(t)
	tw := findTweak(t, u, "LearnMoreWords")
	p := tw.Params[0]

	row := u.paramRow(tw.Name, p, testLabel())
	entry := findEntry(t, row)
	require.NotNil(t, entry)

	entry.OnSubmitted("banana")
	require.Equal(t, modscript.FormatValue(p.Current, p.Kind), entry.Text)

	res, err := core.ListTweaks(t.Context(), core.ListTweaksRequest{
		Request: core.Request{ConfigPath: path},
	})
	require.NoError(t, err)
	require.False(t, findParamIn(t, res, "LearnMoreWords", p.Name).Overridden)
}

// A value outside the declared range is brought inside it and the field shows
// what was actually stored, rather than the number that was typed.
func TestTypingAnOutOfRangeValueShowsWhatWasStored(t *testing.T) {
	u, _ := tweaksUI(t)
	tw := findTweak(t, u, "LearnMoreWords")
	p := tw.Params[0]

	row := u.paramRow(tw.Name, p, testLabel())
	entry := findEntry(t, row)
	entry.OnSubmitted("5000")

	require.Equal(t, "50", entry.Text, "the field shows the value that was stored")
}

// A library script's parameters are offered with no slider: nothing declares a
// range for them, and a slider from nowhere to nowhere is a guess with a handle.
func TestALibraryScriptsParametersHaveNoSlider(t *testing.T) {
	u, _ := tweaksUI(t)
	row := u.libraryParamRow("SomeMod", modscript.Param{
		Name: "PULSE_SPEED", Label: "PULSE_SPEED", Default: 4, Current: 4,
		Kind: modscript.ParamInt, Bounded: false,
	})
	require.Nil(t, findSliderMaybe(row))
	require.NotNil(t, findEntry(t, row))
}

// The section builds headless, with a card per group and no group empty.
func TestTheTweaksSectionBuildsEveryGroup(t *testing.T) {
	u, _ := tweaksUI(t)
	require.NotEmpty(t, u.tweaks.Groups)
	require.NotPanics(t, func() { u.buildTweaks() })

	seen := map[string]int{}
	for _, tw := range u.tweaks.Tweaks {
		seen[tw.Group]++
	}
	for _, g := range u.tweaks.Groups {
		require.Positivef(t, seen[g], "the section lists group %q with nothing in it", g)
	}
}

/*
The compatibility row reports the round-trip check, not a version guess.

Spec 003 left this reading "unknown -- not checked yet" on a machine where the
build had already measured the answer. The three states it can be in are
distinct on purpose: a card that says "unknown" and then changes its mind two
seconds later reads as a card that was wrong.
*/
func TestTheCompatibilityRowReportsTheRoundTripCheck(t *testing.T) {
	u := testUI(t)

	text, st := u.compatLine()
	require.Contains(t, text, "checking")
	require.Equal(t, StatusInfo, st)

	u.compat = core.ToolCheckResult{Status: core.CompatOK, Detail: "2 file(s) round-tripped"}
	text, st = u.compatLine()
	require.Equal(t, StatusGood, st)
	require.Contains(t, text, "2 file(s) round-tripped")

	u.compat = core.ToolCheckResult{}
	u.compatError = "the compiler did not run"
	text, st = u.compatLine()
	require.Contains(t, text, "the compiler did not run")
	require.Equal(t, StatusInfo, st, "a check that could not run is not a bad verdict")
}

// The Overview no longer claims a game data version. Spec 002 R3.4 retired the
// concept: the game's MBINs carry no libMBIN version, so the row was reporting
// a parse of a filename as though it were a fact about the install.
func TestTheOverviewDoesNotReportAGameDataVersion(t *testing.T) {
	u := testUI(t)
	u.statusOK = true
	u.status = core.StatusResult{
		Install: core.InstallSummary{Found: true, Dir: "/somewhere", ModsState: "dir"},
	}
	require.NotContains(t, cardText(u.installCard()), "Game data version")
}

// --- helpers ---------------------------------------------------------------

func findTweak(t *testing.T, u *ui, name string) core.TweakInfo {
	t.Helper()
	for _, tw := range u.tweaks.Tweaks {
		if tw.Name == name {
			return tw
		}
	}
	t.Fatalf("no tweak named %q", name)
	return core.TweakInfo{}
}

func findParam(t *testing.T, u *ui, mod, param string) core.TweakParam {
	t.Helper()
	for _, p := range findTweak(t, u, mod).Params {
		if p.Name == param {
			return p
		}
	}
	t.Fatalf("%s has no parameter %q", mod, param)
	return core.TweakParam{}
}

func findParamIn(t *testing.T, res core.ListTweaksResult, mod, param string) core.TweakParam {
	t.Helper()
	for _, tw := range res.Tweaks {
		if tw.Name != mod {
			continue
		}
		for _, p := range tw.Params {
			if p.Name == param {
				return p
			}
		}
	}
	t.Fatalf("%s has no parameter %q", mod, param)
	return core.TweakParam{}
}
