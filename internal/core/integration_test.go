package core_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/mbin"
)

/*
Integration tests against the real install and the real tools directory.

They skip when NMSBONKER_GAME_DIR is unset (R9.2), so a checkout on a machine
without the game still goes green. Caches and workspaces are redirected into
t.TempDir(), but the tools directory is not: downloading a 2 MB compiler per
test run is the kind of thing that makes a suite too slow to run, so these use
whatever `nmsbonker tools ensure` has already installed and skip when nothing
has been.
*/
func withRealGame(t *testing.T) {
	t.Helper()
	if os.Getenv("NMSBONKER_GAME_DIR") == "" {
		t.Skip("NMSBONKER_GAME_DIR is unset; skipping the tests that need a real game install")
	}
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv(config.FileEnv, "")
}

// realTools points the session at the tools directory the user's own
// `tools ensure` populated, and skips when it is empty.
func realTools(t *testing.T) string {
	t.Helper()
	cfg := config.Defaults()
	dir := cfg.Paths().Tools
	if len(mbin.Installed(dir)) == 0 {
		t.Skipf("no MBINCompiler installed in %s; run `nmsbonker tools ensure`", dir)
	}
	return dir
}

// AC2: what `status` says about this machine. The assertions are the facts the
// spec names, not the whole output, so a cosmetic change to the report does not
// fail the test.
func TestStatusFindsTheRealInstall(t *testing.T) {
	withRealGame(t)

	res, err := core.Status(t.Context(), core.StatusRequest{})
	require.NoError(t, err)
	require.True(t, res.Install.Found)
	require.Contains(t, res.Install.Dir, "No Man's Sky")
	require.NotEmpty(t, res.Install.BuildID)
	require.Equal(t, 97, res.Install.PakCount, "the COSMOS install ships 97 paks")
	require.NotEmpty(t, res.Install.ModsState)
	require.True(t, res.Install.ModSettingsOK)
	require.False(t, res.Install.DisableAllMods)
	t.Logf("game dir %s, buildid %s, MODS=%s -> %s, DisableAllMods=%t",
		res.Install.Dir, res.Install.BuildID, res.Install.ModsState, res.Install.ModsTarget,
		res.Install.DisableAllMods)
}

/*
AC4/R9.2: the whole path a build depends on, end to end -- index, resolve,
extract, decompile -- against files the game actually ships.

The MXML assertion is `template="cGcGameplayGlobals"`. Spec 001 R9.2 asks for
`template="GcGameplayGlobals"`; MBINCompiler writes the libMBIN class name,
which carries the leading "c", so the spec's literal string never appears. See
the spec's Status notes.
*/
func TestExtractingAndDecompilingTheGlobalsWorksEndToEnd(t *testing.T) {
	withRealGame(t)
	tools := realTools(t)

	out := t.TempDir()
	res, err := core.PakExtract(t.Context(), core.PakExtractRequest{
		Name: core.GlobalsFile, OutDir: out,
	})
	require.NoError(t, err)
	require.Equal(t, core.GlobalsPak, res.Pak)
	require.Equal(t, filepath.Join(out, core.GlobalsFile), res.Path)

	data, err := os.ReadFile(res.Path)
	require.NoError(t, err)
	require.Equal(t, []byte{0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC}, data[:8])

	compiler, err := mbin.Locate(tools, "")
	require.NoError(t, err)
	mbin.SetMaxProcesses(2)

	// The extension has to be .MBIN: MBINCompiler dispatches on it.
	probe := filepath.Join(out, "gcgameplayglobals.global.MBIN")
	require.NoError(t, os.WriteFile(probe, data, 0o600))

	mxmlPath, err := compiler.Decompile(t.Context(), probe, filepath.Join(out, "mxml"))
	require.NoError(t, err)
	mxml, err := os.ReadFile(mxmlPath)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(mxml), "<?xml"))
	require.Contains(t, string(mxml), `template="cGcGameplayGlobals"`)
}

// AC4's other half: the reward table is in a different pak, is a megabyte
// rather than eight kilobytes, and is what the reward mods edit.
func TestTheRewardTableExtractsAndDecompiles(t *testing.T) {
	withRealGame(t)
	tools := realTools(t)

	out := t.TempDir()
	res, err := core.PakExtract(t.Context(), core.PakExtractRequest{
		Name: "metadata/reality/tables/rewardtable.mbin", OutDir: out,
	})
	require.NoError(t, err)
	require.Equal(t, "NMSARC.Precache.pak", res.Pak)
	require.False(t, res.ByBasename)

	data, err := os.ReadFile(res.Path)
	require.NoError(t, err)
	require.Equal(t, []byte{0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC}, data[:8])

	compiler, err := mbin.Locate(tools, "")
	require.NoError(t, err)
	mbin.SetMaxProcesses(2)

	probe := filepath.Join(out, "rewardtable.MBIN")
	require.NoError(t, os.WriteFile(probe, data, 0o600))
	mxmlPath, err := compiler.Decompile(t.Context(), probe, filepath.Join(out, "mxml"))
	require.NoError(t, err)
	mxml, err := os.ReadFile(mxmlPath)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(mxml), "<?xml"))
	require.Contains(t, string(mxml), `template="cGcRewardTable"`)
}

/*
R6.1 measured against the real install.

The mechanism does not work on pristine game MBINs -- they are not produced by
MBINCompiler -- so the expected answer here is "unknown with a reason", not a
version. This test pins that so a future libMBIN that *can* read the game's
field is noticed rather than silently changing behaviour: it will start
returning Known, and this assertion will fail and be updated.
*/
func TestTheGameDataVersionIsUnknownBecauseGameMBINsCarryNoStamp(t *testing.T) {
	withRealGame(t)
	realTools(t)

	res, err := core.GameDataVersion(t.Context(), core.GameDataVersionRequest{})
	require.NoError(t, err, "an unreadable version is an answer, never a failure")
	t.Logf("MBINCompiler said %q; nmsbonker reports known=%t reason=%q", res.Raw, res.Known, res.Reason)
	require.False(t, res.Known)
	require.NotEmpty(t, res.Reason)
}

// `pak find` over the real index, which is what AC4 runs on the command line.
func TestPakFindLocatesTheRewardTableInThePrecachePak(t *testing.T) {
	withRealGame(t)

	res, err := core.PakFind(t.Context(), core.PakFindRequest{Glob: "*rewardtable*"})
	require.NoError(t, err)
	require.Equal(t, 97, res.IndexedPaks)
	var found bool
	for _, m := range res.Matches {
		if m.Name == "metadata/reality/tables/rewardtable.mbin" {
			found = true
			require.Equal(t, "NMSARC.Precache.pak", m.Pak)
		}
	}
	require.True(t, found)
}
