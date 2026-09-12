package cache_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/build/cache"
	"github.com/ushineko/nmsbonker/internal/hgpak"
	"github.com/ushineko/nmsbonker/internal/hgpak/hgpaktest"
	"github.com/ushineko/nmsbonker/internal/mbin"
)

/*
decompilerScript is a compiler that turns anything into an MXML naming its
input, and back.

The cache does not care what the compiler produces, only that it produced
something and where it put it, so a stub is a complete stand-in. It records each
invocation so a test can prove the second Ensure did not run it again.
*/
const decompilerScript = `#!/bin/sh
here=$(dirname "$0")
outdir=""; input=""
while [ $# -gt 0 ]; do
  case "$1" in
    -d) outdir="$2"; shift 2;;
    -y|-q|-Q) shift;;
    version) shift;;
    *) input="$1"; shift;;
  esac
done
[ -z "$input" ] && { echo "MBINCompiler v0.0.0-fake"; exit 0; }
echo "$input" >> "$here/calls"
base=$(basename "$input"); stem=${base%.*}
if [ -f "$here/fail" ] && grep -qF "$base" "$here/fail"; then
  echo "[ERROR]: unknown template" >&2
  exit 1
fi
printf '<?xml version="1.0"?><!-- %s -->' "$base" > "$outdir/$stem.MXML"
exit 0
`

type stub struct {
	compiler *mbin.Compiler
	dir      string
}

func (s stub) calls(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.dir, "calls"))
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	n := 0
	for _, line := range splitLines(string(b)) {
		if line != "" {
			n++
		}
	}
	return n
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func newStub(t *testing.T, failing ...string) stub {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "MBINCompiler-linux-dotnet10")
	require.NoError(t, os.WriteFile(bin, []byte(decompilerScript), 0o700)) //nolint:gosec // a test stub
	if len(failing) > 0 {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "fail"),
			[]byte(joinLines(failing)), 0o600))
	}
	mbin.SetMaxProcesses(4)
	return stub{compiler: &mbin.Compiler{Bin: bin, Tag: "v1.0.0-fake"}, dir: dir}
}

func joinLines(items []string) string {
	out := ""
	for _, i := range items {
		out += i + "\n"
	}
	return out
}

// synthPak builds an archive holding the named files and indexes it.
func synthPak(t *testing.T, names ...string) (*hgpak.Index, string) {
	t.Helper()
	dir := t.TempDir()
	pak := filepath.Join(dir, "SYNTH.pak")
	entries := make([]hgpaktest.Entry, 0, len(names))
	for _, n := range names {
		entries = append(entries, hgpaktest.Entry{Name: n, Data: []byte("MBIN:" + n)})
	}
	require.NoError(t, hgpaktest.WriteFile(pak, entries, true))

	idx, _, err := hgpak.BuildIndex(context.Background(), filepath.Join(dir, "index.json"),
		[]string{pak}, 1, nil)
	require.NoError(t, err)
	return idx, pak
}

/*
R3.2: the second Ensure reuses what the first produced.

This is the whole reason the cache exists. A hundred files at a second or two of
.NET startup each is the difference between a build that takes ten seconds and
one that takes three minutes, and it happens on every build the user runs
between game updates.
*/
func TestASecondEnsureReusesTheCachedFilesWithoutRunningTheCompiler(t *testing.T) {
	idx, _ := synthPak(t, "metadata/tables/rewardtable.mbin", "gcgameplayglobals.global.mbin")
	sc := newStub(t)
	dir := t.TempDir()
	sources := []string{`METADATA\TABLES\REWARDTABLE.MBIN`, "GCGAMEPLAYGLOBALS.GLOBAL.MBIN"}

	first := cache.New(dir, idx, sc.compiler, "v1.0.0-fake", 2)
	res, err := first.Ensure(t.Context(), sources, false)
	require.NoError(t, err)
	require.Equal(t, 2, res.Extracted)
	require.Equal(t, 0, res.Reused)
	require.Empty(t, res.Misses)
	require.Equal(t, 2, sc.calls(t))

	entry, ok := res.Entries["METADATA/TABLES/REWARDTABLE.MBIN"]
	require.True(t, ok, "keyed by the normalised source, whichever way the script spelled it")
	require.Equal(t, "mxml/METADATA/TABLES/REWARDTABLE.MXML", entry.MXML)
	require.FileExists(t, first.MXMLPath(entry))

	second := cache.New(dir, idx, sc.compiler, "v1.0.0-fake", 2)
	res, err = second.Ensure(t.Context(), sources, false)
	require.NoError(t, err)
	require.Equal(t, 2, res.Reused)
	require.Equal(t, 0, res.Extracted)
	require.Equal(t, 2, sc.calls(t), "the compiler was not run again")
}

/*
R3.1: a different compiler invalidates every entry.

MBINCompiler's output changes between releases -- that is the whole reason
upgrading it is a thing a user does -- so a cache produced by the old one would
merge edits into a document the new one will not accept.
*/
func TestADifferentCompilerVersionInvalidatesTheCache(t *testing.T) {
	idx, _ := synthPak(t, "gcgameplayglobals.global.mbin")
	sc := newStub(t)
	dir := t.TempDir()

	_, err := cache.New(dir, idx, sc.compiler, "v1.0.0-fake", 1).
		Ensure(t.Context(), []string{"GCGAMEPLAYGLOBALS.GLOBAL.MBIN"}, false)
	require.NoError(t, err)

	res, err := cache.New(dir, idx, sc.compiler, "v2.0.0-fake", 1).
		Ensure(t.Context(), []string{"GCGAMEPLAYGLOBALS.GLOBAL.MBIN"}, false)
	require.NoError(t, err)
	require.Equal(t, 1, res.Extracted)
	require.Equal(t, 0, res.Reused)
}

// R3.2: force re-extracts even when everything is current, which is what
// `build --recache` is for when a cache is suspected of being stale.
func TestForceReExtractsEverything(t *testing.T) {
	idx, _ := synthPak(t, "gcgameplayglobals.global.mbin")
	sc := newStub(t)
	dir := t.TempDir()
	store := cache.New(dir, idx, sc.compiler, "v1.0.0-fake", 1)

	_, err := store.Ensure(t.Context(), []string{"GCGAMEPLAYGLOBALS.GLOBAL.MBIN"}, false)
	require.NoError(t, err)
	res, err := store.Ensure(t.Context(), []string{"GCGAMEPLAYGLOBALS.GLOBAL.MBIN"}, true)
	require.NoError(t, err)
	require.Equal(t, 1, res.Extracted)
}

/*
R3.3: a source no pak holds, and one the compiler refuses, are both misses.

Neither stops the build. A mod naming a file a game update removed is common,
and refusing to build anything because of it would cost the user every other
mod they have installed.
*/
func TestAMissingSourceAndAFailedDecompileAreBothReportedAndNotFatal(t *testing.T) {
	idx, _ := synthPak(t, "present.mbin", "broken.mbin")
	sc := newStub(t, "broken.mbin")
	store := cache.New(t.TempDir(), idx, sc.compiler, "v1.0.0-fake", 2)

	res, err := store.Ensure(t.Context(), []string{"PRESENT.MBIN", "BROKEN.MBIN", "ABSENT.MBIN"}, false)
	require.NoError(t, err)
	require.Len(t, res.Entries, 1)
	require.Contains(t, res.Entries, "PRESENT.MBIN")
	require.Len(t, res.Misses, 2)
	require.Equal(t, "ABSENT.MBIN", res.Misses[0].Source)
	require.Contains(t, res.Misses[0].Reason, "none of the indexed paks")
	require.Equal(t, "BROKEN.MBIN", res.Misses[1].Source)
	require.Contains(t, res.Misses[1].Reason, "unknown template")
}

/*
R3.2: a source that only resolves by basename is recorded as such.

Several scripts write "GLOBALS\GCGAMEPLAYGLOBALS.GLOBAL.MBIN" for a file that
actually sits at the pak root. The fallback makes them work; recording that it
was used is what lets the build report say the mod is working by luck.
*/
func TestASourceResolvedByBasenameIsRecorded(t *testing.T) {
	idx, _ := synthPak(t, "gcgameplayglobals.global.mbin")
	sc := newStub(t)
	store := cache.New(t.TempDir(), idx, sc.compiler, "v1.0.0-fake", 1)

	res, err := store.Ensure(t.Context(), []string{`GLOBALS\GCGAMEPLAYGLOBALS.GLOBAL.MBIN`}, false)
	require.NoError(t, err)
	entry := res.Entries[`GLOBALS/GCGAMEPLAYGLOBALS.GLOBAL.MBIN`]
	require.Equal(t, "basename", entry.ResolvedBy)
	require.Equal(t, "gcgameplayglobals.global.mbin", entry.Internal,
		"placement follows the pak index, not the spelling the script used")
}

// R3.1: the MXML path is the pak-internal path, upper-cased, with the extension
// swapped -- the same spelling the built MBIN takes under GAMEDATA/MODS.
func TestTheCachedPathMirrorsTheOutputPath(t *testing.T) {
	require.Equal(t, "mxml/METADATA/REALITY/TABLES/REWARDTABLE.MXML",
		cache.MXMLRel("metadata/reality/tables/rewardtable.mbin"))
	require.Equal(t, "mxml/GCGAMEPLAYGLOBALS.GLOBAL.MXML",
		cache.MXMLRel("gcgameplayglobals.global.mbin"))
	require.Equal(t, "mxml/NOEXT.MXML", cache.MXMLRel("noext"))
}

// R3.2: the same file named twice is produced once.
func TestADuplicatedSourceIsProducedOnce(t *testing.T) {
	idx, _ := synthPak(t, "gcgameplayglobals.global.mbin")
	sc := newStub(t)
	store := cache.New(t.TempDir(), idx, sc.compiler, "v1.0.0-fake", 2)

	res, err := store.Ensure(t.Context(),
		[]string{"GCGAMEPLAYGLOBALS.GLOBAL.MBIN", `gcgameplayglobals.global.mbin`}, false)
	require.NoError(t, err)
	require.Equal(t, 1, res.Extracted)
	require.Equal(t, 1, sc.calls(t))
}

// Key is what groups two spellings of one file into one target, so it is worth
// pinning directly. R3.2.
func TestKeyNormalisesSlashesAndCase(t *testing.T) {
	require.Equal(t, "METADATA/TABLES/X.MBIN", cache.Key(`METADATA\TABLES\X.MBIN`))
	require.Equal(t, "METADATA/TABLES/X.MBIN", cache.Key("metadata/tables/x.mbin"))
}
