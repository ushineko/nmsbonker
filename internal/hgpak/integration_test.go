package hgpak_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/hgpak"
)

/*
Integration tests against the machine's real No Man's Sky install.

They skip rather than fail when NMSBONKER_GAME_DIR is unset (R9.2, R1.4): the
repository is public and holds no game data, so a checkout on a machine without
the game must still go green. Nothing here writes to the game directory.
*/

func gameDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("NMSBONKER_GAME_DIR")
	if dir == "" {
		t.Skip("NMSBONKER_GAME_DIR is unset; skipping the tests that need a real game install")
	}
	return dir
}

func pcbanks(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(gameDir(t), "GAMEDATA", "PCBANKS")
	require.DirExists(t, dir, "NMSBONKER_GAME_DIR should point at the install root, not at PCBANKS")
	return dir
}

func realPaks(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(pcbanks(t))
	require.NoError(t, err)
	var paks []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".pak") {
			paks = append(paks, filepath.Join(pcbanks(t), e.Name()))
		}
	}
	sort.Strings(paks)
	require.NotEmpty(t, paks)
	return paks
}

// The synthetic fixtures prove the reader is self-consistent; only the real
// install proves it agrees with Hello Games' packer. Every pak in PCBANKS must
// open and produce a manifest whose length matches its index. R9.2.
func TestEveryPakInTheInstallOpensAndListsItsFiles(t *testing.T) {
	total := 0
	for _, path := range realPaks(t) {
		pak, err := hgpak.Open(path)
		require.NoError(t, err, path)
		names := pak.Names()
		require.NotEmpty(t, names, path)
		for _, name := range names {
			entry, ok := pak.Stat(name)
			require.True(t, ok, name)
			require.Equal(t, hgpak.HashPath(name), entry.Hash,
				"%s: the manifest name does not hash to the index entry beside it", name)
		}
		total += len(names)
		require.NoError(t, pak.Close())
	}
	t.Logf("opened %d paks holding %d files", len(realPaks(t)), total)
}

/*
AC5: the reader replaces hgpaktool.exe under Wine, so the bytes it produces must
be the bytes hgpaktool produced. A one-chunk-late read, or a mishandled final
partial chunk, would still look like a plausible MBIN.

The oracle is the reference build cache (cache/raw/fN/.../<name>.mbin), which
build_cache.py fills by running hgpaktool directly. Spec 001 AC5 names
extract/GLOBALS/gcgameplayglobals.global.MBIN instead; that file is not an
hgpaktool extraction. It differs from the pak's bytes in the MBIN header
(libMBIN stamps its own version at 0x0A and 0x18) and at offset 0x17A0, where
MaxNumSameGroupTech is 25 rather than the game's 3 -- it is a *modded recompile*
produced by MBINCompiler, so byte-comparing against it would assert that this
reader reproduces someone's mod. See the spec's Status notes.
*/
func TestExtractedGlobalsAreByteIdenticalToTheReferenceExtraction(t *testing.T) {
	reference := os.Getenv("NMSBONKER_REFERENCE_DIR")
	if reference == "" {
		t.Skip("NMSBONKER_REFERENCE_DIR is unset; skipping the byte-for-byte comparison with hgpaktool")
	}
	matches, err := filepath.Glob(
		filepath.Join(reference, "cache", "raw", "*", "*", "gcgameplayglobals.global.mbin"))
	require.NoError(t, err)
	if len(matches) == 0 {
		t.Skipf("no hgpaktool extraction under %s/cache/raw; run the reference "+
			"build_cache.py to produce one",
			reference)
	}
	want, err := os.ReadFile(matches[0])
	require.NoError(t, err)

	pak, err := hgpak.Open(filepath.Join(pcbanks(t), "NMSARC.globals.pak"))
	require.NoError(t, err)
	defer func() { require.NoError(t, pak.Close()) }()

	got, err := pak.ReadFile("gcgameplayglobals.global.mbin")
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.Equal(t, []byte{0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC}, got[:8],
		"an MBIN starts with the CC or DD magic")
}

// AC4's half of the pak layer: the file the reward-table mods edit has to be
// findable by glob and readable, and it is in a different pak from the globals.
func TestTheRewardTableIsFoundByGlobAndReadsAsAnMBIN(t *testing.T) {
	idx, _, err := hgpak.BuildIndex(t.Context(), filepath.Join(t.TempDir(), "pak-index.json"),
		realPaks(t), runtime.NumCPU(), nil)
	require.NoError(t, err)

	hits := idx.Find("*rewardtable*")
	require.NotEmpty(t, hits)
	var found bool
	for _, h := range hits {
		if h.Name == "metadata/reality/tables/rewardtable.mbin" {
			found = true
			require.Equal(t, "NMSARC.Precache.pak", filepath.Base(h.Pak))
		}
	}
	require.True(t, found, "metadata/reality/tables/rewardtable.mbin should be in the index")

	loc, byBasename, ok := idx.Resolve(`METADATA\REALITY\TABLES\REWARDTABLE.MBIN`)
	require.True(t, ok)
	require.False(t, byBasename, "the full path should resolve exactly, not by the basename fallback")

	pak, err := hgpak.Open(loc.Pak)
	require.NoError(t, err)
	defer func() { require.NoError(t, pak.Close()) }()
	b, err := pak.ReadFile(loc.Name)
	require.NoError(t, err)
	require.Greater(t, len(b), 8)
	require.Contains(t,
		[][]byte{{0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC}, {0xDD, 0xDD, 0xDD, 0xDD, 0xDD, 0xDD, 0xDD, 0xDD}},
		b[:8])
}

// AC6: the index is built on every status and every build, so a cold build over
// the real 97 paks has to stay under 5 s and a warm load under 200 ms. The
// numbers are logged because the spec asks for them to be recorded.
func TestBuildingThePakIndexIsFastColdAndFasterWarm(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "pak-index.json")
	paks := realPaks(t)

	coldStart := time.Now()
	idx, stats, err := hgpak.BuildIndex(context.Background(), cache, paks, runtime.NumCPU(), nil)
	require.NoError(t, err)
	cold := time.Since(coldStart)
	require.Equal(t, len(paks), stats.Reindexed, "a cold build reads every pak")
	require.True(t, stats.Saved)

	warmStart := time.Now()
	warmIdx, warmStats, err := hgpak.BuildIndex(context.Background(), cache, paks, runtime.NumCPU(), nil)
	require.NoError(t, err)
	warm := time.Since(warmStart)
	require.Zero(t, warmStats.Reindexed, "an unchanged install re-reads nothing")
	require.Equal(t, idx.Len(), warmIdx.Len())

	fi, err := os.Stat(cache)
	require.NoError(t, err)
	t.Logf("pak index: %d paks, %d files, cold %s, warm %s, cache %d bytes (budget factor %dx)",
		len(paks), idx.Len(), cold.Round(time.Millisecond), warm.Round(time.Millisecond), fi.Size(),
		budgetFactor)

	require.Less(t, cold, budgetFactor*5*time.Second, "AC6: cold index build")
	require.Less(t, warm, budgetFactor*200*time.Millisecond, "AC6: warm index load")
}
