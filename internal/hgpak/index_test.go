package hgpak_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/hgpak"
	"github.com/ushineko/nmsbonker/internal/hgpak/hgpaktest"
)

// twoPaks writes a pair of archives whose contents overlap by basename, which
// is what makes the fallback interesting.
func twoPaks(t *testing.T) (dir string, paks []string) {
	t.Helper()
	dir = t.TempDir()
	a := filepath.Join(dir, "NMSARC.globals.pak")
	b := filepath.Join(dir, "NMSARC.Precache.pak")
	require.NoError(t, hgpaktest.WriteFile(a, []hgpaktest.Entry{
		{Name: "GCGAMEPLAYGLOBALS.GLOBAL.MBIN", Data: []byte("globals")},
		{Name: "GCAISPACESHIPGLOBALS.GLOBAL.MBIN", Data: []byte("ships")},
	}, true))
	require.NoError(t, hgpaktest.WriteFile(b, []hgpaktest.Entry{
		{Name: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`, Data: []byte("rewards")},
		{Name: `METADATA\REALITY\TABLES\NMS_REALITY_GCPRODUCTTABLE.MBIN`, Data: []byte("products")},
	}, true))
	return dir, []string{a, b}
}

// The exact path is what a correct script uses; the basename fallback exists
// because real scripts write "GLOBALS\GCGAMEPLAYGLOBALS.GLOBAL.MBIN" for a file
// that lives at the pak root, and the legacy builder resolved those. Losing the
// fallback would silently stop applying those mods. R4.5.
func TestTheIndexResolvesExactPathsAndFallsBackToTheBasename(t *testing.T) {
	dir, paks := twoPaks(t)
	idx, stats, err := hgpak.BuildIndex(t.Context(), filepath.Join(dir, "pak-index.json"), paks, 2, nil)
	require.NoError(t, err)
	require.Equal(t, 2, stats.Reindexed)
	require.Equal(t, 4, idx.Len())

	loc, byBasename, ok := idx.Resolve("metadata/reality/tables/rewardtable.mbin")
	require.True(t, ok)
	require.False(t, byBasename)
	require.Equal(t, "NMSARC.Precache.pak", filepath.Base(loc.Pak))

	loc, byBasename, ok = idx.Resolve(`GLOBALS\GCGAMEPLAYGLOBALS.GLOBAL.MBIN`)
	require.True(t, ok, "a script's invented GLOBALS/ prefix must still resolve")
	require.True(t, byBasename, "and the caller must be told it only resolved by basename")
	require.Equal(t, "gcgameplayglobals.global.mbin", loc.Name)

	_, _, ok = idx.Resolve("nothing/at/all.mbin")
	require.False(t, ok)
}

// `pak find '*rewardtable*'` is the command AC4 exercises. filepath.Match will
// not let '*' cross a '/', so matching only the full path would return nothing
// for every file that is not at the pak root.
func TestFindMatchesGlobsAgainstTheBasenameAsWellAsTheFullPath(t *testing.T) {
	dir, paks := twoPaks(t)
	idx, _, err := hgpak.BuildIndex(t.Context(), filepath.Join(dir, "pak-index.json"), paks, 2, nil)
	require.NoError(t, err)

	hits := idx.Find("*rewardtable*")
	require.Len(t, hits, 1)
	require.Equal(t, "metadata/reality/tables/rewardtable.mbin", hits[0].Name)

	require.Len(t, idx.Find("metadata/reality/tables/*.mbin"), 2, "a full-path glob still works")
	require.Len(t, idx.Find("REWARDTABLE"), 1, "a bare word is a case-insensitive substring search")
	require.Empty(t, idx.Find("*.exml"))
}

// A game update rewrites a handful of paks out of 97. Re-reading all of them
// would turn a 60 ms warm start into a multi-second one on every patch day, so
// the cache is keyed per pak by (size, mtime). R4.5.
func TestOnlyChangedPaksAreReindexed(t *testing.T) {
	dir, paks := twoPaks(t)
	cache := filepath.Join(dir, "pak-index.json")

	_, stats, err := hgpak.BuildIndex(t.Context(), cache, paks, 2, nil)
	require.NoError(t, err)
	require.Equal(t, 2, stats.Reindexed)
	require.False(t, stats.FromCache)
	require.True(t, stats.Saved)

	_, stats, err = hgpak.BuildIndex(t.Context(), cache, paks, 2, nil)
	require.NoError(t, err)
	require.Zero(t, stats.Reindexed, "nothing changed, so nothing is re-read")
	require.Equal(t, 2, stats.Reused)
	require.True(t, stats.FromCache)
	require.False(t, stats.Saved, "an unchanged install must not rewrite a 15 MB cache file")

	// Rewrite one pak with different contents and a newer mtime.
	require.NoError(t, hgpaktest.WriteFile(paks[0], []hgpaktest.Entry{
		{Name: "GCGAMEPLAYGLOBALS.GLOBAL.MBIN", Data: []byte("globals, patched")},
		{Name: "GCNEWGLOBALS.GLOBAL.MBIN", Data: []byte("new in this patch")},
	}, true))
	require.NoError(t, os.Chtimes(paks[0], time.Now(), time.Now().Add(time.Second)))

	idx, stats, err := hgpak.BuildIndex(t.Context(), cache, paks, 2, nil)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Reindexed)
	require.Equal(t, 1, stats.Reused)
	_, _, ok := idx.Resolve("gcnewglobals.global.mbin")
	require.True(t, ok)
	_, _, ok = idx.Resolve("gcaispaceshipglobals.global.mbin")
	require.False(t, ok, "a file the patch removed must leave the index")
}

// A cache is an optimisation. Refusing to run because one is truncated or was
// written by a different build would make it a dependency, and the failure
// would arrive as "nmsbonker is broken" rather than "this took a moment".
func TestACorruptOrOldCacheIsRebuiltRatherThanFatal(t *testing.T) {
	dir, paks := twoPaks(t)
	cache := filepath.Join(dir, "pak-index.json")

	require.NoError(t, os.WriteFile(cache, []byte(`{"version":1,"paks":[`), 0o600))
	_, stats, err := hgpak.BuildIndex(t.Context(), cache, paks, 2, nil)
	require.NoError(t, err)
	require.Equal(t, 2, stats.Reindexed)

	require.NoError(t, os.WriteFile(cache, []byte(`{"version":999,"paks":[]}`), 0o600))
	_, stats, err = hgpak.BuildIndex(t.Context(), cache, paks, 2, nil)
	require.NoError(t, err)
	require.Equal(t, 2, stats.Reindexed)
}

// PCBANKS is not only paks: the game ships BankSignatures.bin and
// filenames.json in the same directory, and a user may drop anything there.
// One unreadable file must not fail the index for the other 96.
func TestANonPakInThePakListIsSkippedRatherThanFatal(t *testing.T) {
	dir, paks := twoPaks(t)
	stray := filepath.Join(dir, "BankSignatures.pak")
	require.NoError(t, os.WriteFile(stray, []byte("not a pak"), 0o600))

	idx, _, err := hgpak.BuildIndex(t.Context(), filepath.Join(dir, "pak-index.json"),
		append(paks, stray), 2, nil)
	require.NoError(t, err)
	require.Equal(t, 4, idx.Len())
}
