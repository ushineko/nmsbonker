package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/mbin"
)

// bare points every directory at a scratch tree and hides any real Steam
// install, so the "nothing is set up yet" paths are what actually runs.
func bare(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("STEAM_ROOT", filepath.Join(root, "no-steam-here"))
	t.Setenv(config.FileEnv, "")
	t.Setenv(config.GameDirEnv, "")
	return root
}

// `status` is the command a user runs to find out what is wrong. A missing
// game, a missing compiler and an empty cache are the three states a new
// install is in, and failing on any of them would make the diagnostic command
// the one that cannot run. R7.2.
func TestStatusOnAMachineWithNothingSetUpStillAnswers(t *testing.T) {
	bare(t)

	res, err := core.Status(t.Context(), core.StatusRequest{})
	require.NoError(t, err)
	require.False(t, res.Install.Found)
	require.NotEmpty(t, res.Install.Error, "and says why")
	require.False(t, res.Compiler.Installed)
	require.Equal(t, mbin.VersionUnknown, res.GameDataVersion)
	require.Equal(t, mbin.CompatUnknown, res.Compatibility)
	require.False(t, res.PakIndex.Exists)
	require.NotEmpty(t, res.Paths.Library)

	// R2.4: nothing has been created by asking.
	require.NoDirExists(t, res.Paths.Library)
	require.NoDirExists(t, res.Paths.Tools)
}

// "installation not found" is useless on its own: libraries move to a second
// drive and a partly uninstalled game leaves a manifest with no PCBANKS.
// Listing what was examined is what makes it actionable. R7.2.
func TestDetectListsWhereItLookedEvenWhenItFindsNothing(t *testing.T) {
	bare(t)

	res, err := core.Detect(t.Context(), core.DetectRequest{})
	require.NoError(t, err, "detect reports a failure, it is not one")
	require.False(t, res.Found)
	require.NotEmpty(t, res.Roots)
	require.NotEmpty(t, res.Candidates)
	for _, c := range res.Candidates {
		require.NotEmpty(t, c.Reason)
	}
}

// Every field of Events may be nil, so that core.Status(ctx,
// core.StatusRequest{}) works. An interface with a no-op implementation would
// make the zero value a panic. R7.1.
func TestOperationsRunWithAZeroValueRequest(t *testing.T) {
	bare(t)
	require.NotPanics(t, func() {
		_, err := core.Status(t.Context(), core.StatusRequest{})
		require.NoError(t, err)
		_, err = core.ConfigShow(t.Context(), core.ConfigShowRequest{})
		require.NoError(t, err)
		_, err = core.ListTools(t.Context(), core.ListToolsRequest{})
		require.NoError(t, err)
	})
}

// The GUI's settings panel and `config set` are the same code path, so a value
// rejected in one is rejected in both, and a change written by either is
// visible to the other immediately. R7.2.
func TestConfigSetWritesAndConfigShowReadsItBack(t *testing.T) {
	bare(t)

	set, err := core.ConfigSet(t.Context(), core.ConfigSetRequest{Key: "mod_name", Value: "TEST COMBINE"})
	require.NoError(t, err)
	require.Equal(t, config.DefaultModName, set.Old)
	require.Equal(t, "TEST COMBINE", set.New)
	require.FileExists(t, set.Path)

	show, err := core.ConfigShow(t.Context(), core.ConfigShowRequest{})
	require.NoError(t, err)
	require.Contains(t, show.Entries, core.ConfigEntry{Key: "mod_name", Value: "TEST COMBINE"})

	_, err = core.ConfigSet(t.Context(), core.ConfigSetRequest{Key: "mbincompiler.flavor", Value: "wine"})
	require.Error(t, err)

	again, err := core.ConfigSet(t.Context(), core.ConfigSetRequest{Key: "mod_name", Value: "TEST COMBINE"})
	require.NoError(t, err)
	require.True(t, again.Unchanged)
}

// A pin is a decision the user made about which compiler this game version
// needs. Refusing to record it until the release is installed would force them
// to install a version they have already decided against. R7.2.
func TestPinningAReleaseThatIsNotInstalledIsAllowedAndSaidSo(t *testing.T) {
	bare(t)

	res, err := core.PinTool(t.Context(), core.PinToolRequest{Tag: "v7.01.0-pre1"})
	require.NoError(t, err)
	require.Equal(t, "v7.01.0-pre1", res.Pin)
	require.False(t, res.Installed)

	_, err = core.PinTool(t.Context(), core.PinToolRequest{Tag: "latest"})
	require.Error(t, err, "a tag that is not a version is a typo, not a pin")

	res, err = core.PinTool(t.Context(), core.PinToolRequest{})
	require.NoError(t, err)
	require.Empty(t, res.Pin)
}

// Reading a pak needs the game; saying "not found" here rather than panicking
// on a nil install is the difference between a message and a crash. R7.2.
func TestPakOperationsFailClearlyWithNoGameInstalled(t *testing.T) {
	bare(t)

	_, err := core.PakFind(t.Context(), core.PakFindRequest{Glob: "*"})
	require.Error(t, err)

	_, err = core.PakExtract(t.Context(), core.PakExtractRequest{Name: "x.mbin", OutDir: t.TempDir()})
	require.Error(t, err)
}

// The game directory the operation uses must be the one the caller asked for,
// whatever a stale config says. R2.2, R7.1.
func TestAnExplicitGameDirOnTheRequestOutranksEverything(t *testing.T) {
	root := bare(t)
	game := filepath.Join(root, "elsewhere", "No Man's Sky")
	require.NoError(t, os.MkdirAll(filepath.Join(game, "GAMEDATA", "PCBANKS"), 0o750))

	res, err := core.Status(t.Context(), core.StatusRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.True(t, res.Install.Found)
	require.Equal(t, game, res.Install.Dir)
	require.Equal(t, "--game-dir", res.Install.Source)
}
