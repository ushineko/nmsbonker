package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/core"
)

// seedCache writes a fake pristine tree and pak index under the scratch cache
// directory, so the measuring and clearing paths have something to find without
// a game install or a compiler.
func seedCache(t *testing.T, cacheDir string, buildIDs ...string) {
	t.Helper()
	for _, id := range buildIDs {
		dir := filepath.Join(cacheDir, "game", id, "raw")
		require.NoError(t, os.MkdirAll(dir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.mbin"), []byte("0123456789"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "b.mxml"), []byte("<x/>"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "pak-index.json"), []byte("{}"), 0o600))
}

// The cache card has to be able to say what is on disk before anyone can decide
// whether to throw it away, and it must do that on a machine with no game --
// which is exactly the machine where a leftover cache is confusing.
func TestCacheInfoMeasuresEveryGameBuildWithoutAnInstall(t *testing.T) {
	bare(t)
	show, err := core.ConfigShow(t.Context(), core.ConfigShowRequest{})
	require.NoError(t, err)
	seedCache(t, show.Paths.Cache, "111", "222")

	res, err := core.CacheInfo(t.Context(), core.CacheInfoRequest{})
	require.NoError(t, err)
	require.Len(t, res.Games, 2)
	require.Equal(t, 4, res.TotalFiles, "two files in each of two game builds")
	require.Equal(t, int64(28), res.TotalBytes)
	require.Positive(t, res.PakIndexBytes)
	for _, g := range res.Games {
		require.False(t, g.Current, "no game is installed, so none of them is the current one")
	}
}

/*
Clearing the cache must not need a game, but it must not guess either.

With no install there is no way to tell which of several cached game builds
belongs to the game that is not there. Refusing and naming --all is the honest
answer; picking one would delete the wrong tree on a machine that has been
through two game updates.
*/
func TestClearCacheRefusesToGuessWhichBuildWithoutAnInstall(t *testing.T) {
	bare(t)
	show, err := core.ConfigShow(t.Context(), core.ConfigShowRequest{})
	require.NoError(t, err)
	seedCache(t, show.Paths.Cache, "111")

	_, err = core.ClearCache(t.Context(), core.ClearCacheRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--all")
	require.DirExists(t, filepath.Join(show.Paths.Cache, "game", "111"),
		"and it removed nothing while refusing")
}

// --all clears every game build. The pak index is left alone unless asked for
// separately: it is cheap to keep and expensive to rebuild on a cold cache.
func TestClearCacheAllRemovesTheGameTreesAndKeepsTheIndex(t *testing.T) {
	bare(t)
	show, err := core.ConfigShow(t.Context(), core.ConfigShowRequest{})
	require.NoError(t, err)
	seedCache(t, show.Paths.Cache, "111", "222")

	res, err := core.ClearCache(t.Context(), core.ClearCacheRequest{All: true})
	require.NoError(t, err)
	require.Equal(t, 4, res.Files)
	require.NoDirExists(t, filepath.Join(show.Paths.Cache, "game"))
	require.FileExists(t, filepath.Join(show.Paths.Cache, "pak-index.json"),
		"the index survives unless --index was given")

	res, err = core.ClearCache(t.Context(), core.ClearCacheRequest{All: true, IncludeIndex: true})
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(show.Paths.Cache, "pak-index.json"))
}

// Clearing a cache that is not there is not a failure: it is the state a fresh
// install is in, and a command that errors on it cannot be run defensively.
func TestClearCacheOnAnEmptyCacheSaysSoRatherThanFailing(t *testing.T) {
	bare(t)
	res, err := core.ClearCache(t.Context(), core.ClearCacheRequest{All: true})
	require.NoError(t, err)
	require.Empty(t, res.Removed)
	require.Zero(t, res.Bytes)
}

// Rebuilding the index needs the archives, so without a game it must say that
// rather than writing an empty index that every later lookup would miss in.
func TestRebuildIndexWithoutAGameRefuses(t *testing.T) {
	bare(t)
	_, err := core.RebuildIndex(t.Context(), core.RebuildIndexRequest{})
	require.Error(t, err)
}
