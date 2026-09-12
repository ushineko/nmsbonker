package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/hgpak/hgpaktest"
)

/*
stubCompilerScript is a compiler that converts by copying, with a mode deciding
how faithfully it comes back.

The compatibility check's whole question is "does what comes out equal what went
in", so a stub that can be told to be faithful, to differ in the header, or to
differ in the body exercises every verdict without the real 2 MB .NET binary.
*/
const stubCompilerScript = `#!/bin/sh
here=$(dirname "$0")
mode=$(cat "$here/mode" 2>/dev/null || echo ok)
outdir=""; input=""
while [ $# -gt 0 ]; do
  case "$1" in
    -d) outdir="$2"; shift 2;;
    -y|-q|-Q) shift;;
    version) shift;;
    *) input="$1"; shift;;
  esac
done
[ -z "$input" ] && { echo "MBINCompiler v9.99.0-stub"; exit 0; }
base=$(basename "$input"); stem=${base%.*}
case "$base" in
  *.MBIN|*.mbin) cat "$input" > "$outdir/$stem.MXML"; exit 0;;
esac
out="$outdir/$stem.MBIN"
cat "$input" > "$out"
[ "$mode" = "body" ] && printf 'X' | dd of="$out" bs=1 seek=300 conv=notrunc 2>/dev/null
exit 0
`

// stubTools installs the stub where mbin.Locate will find it.
func stubTools(t *testing.T, mode string) {
	t.Helper()
	dir := filepath.Join(config.Defaults().Paths().Tools, "mbincompiler", "v9.99.0-stub")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "MBINCompiler-linux-dotnet10"),
		[]byte(stubCompilerScript), 0o700)) //nolint:gosec // a test stub that has to be executable
	require.NoError(t, os.WriteFile(filepath.Join(dir, "libMBIN-linux-dotnet10.so"), []byte("stub"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mode"), []byte(mode), 0o600))
}

// probeGame builds a fake install whose single pak holds the two files the
// compatibility check round-trips, each large enough to have a body past the
// skipped header.
func probeGame(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "game")
	banks := filepath.Join(dir, "GAMEDATA", "PCBANKS")
	require.NoError(t, os.MkdirAll(banks, 0o750))

	body := make([]byte, 1024)
	for i := range body {
		body[i] = byte(i % 251)
	}
	require.NoError(t, hgpaktest.WriteFile(filepath.Join(banks, "SYNTH.pak"), []hgpaktest.Entry{
		{Name: "gcgameplayglobals.global.mbin", Data: body},
		{Name: "metadata/reality/tables/rewardtable.mbin", Data: body},
	}, true))
	return dir
}

// AC9/R3.4: a compiler that returns the file unchanged is compatible.
func TestToolCheckReportsCompatibleWhenTheFilesRoundTrip(t *testing.T) {
	root := bare(t)
	game := probeGame(t, root)
	stubTools(t, "ok")

	res, err := core.ToolCheck(t.Context(), core.ToolCheckRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.Equal(t, core.CompatOK, res.Status, res.Detail)
	require.Len(t, res.Files, 2)
	require.Equal(t, "MBINCompiler v9.99.0-stub", res.CompilerVersion)
	require.Empty(t, res.Advice)
}

/*
AC9/R3.4: a compiler whose output differs is a mismatch with something to do
about it.

Naming the file and the offset is the difference between a verdict the user can
act on and one they can only worry about, and the advice is the actual next
step: a newer release, or a pin to one that matches.
*/
func TestToolCheckReportsMismatchWithAnActionableMessage(t *testing.T) {
	root := bare(t)
	game := probeGame(t, root)
	stubTools(t, "body")

	res, err := core.ToolCheck(t.Context(), core.ToolCheckRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err, "a mismatch is a verdict, not a failure: the user can still build")
	require.Equal(t, core.CompatMismatch, res.Status)
	require.Contains(t, res.Detail, "round-tripped to different bytes, first at offset 0x12C")
	require.Contains(t, res.Advice, "tools ensure")
	require.Contains(t, res.Advice, "tools pin")
	for _, f := range res.Files {
		require.False(t, f.OK)
	}
}

// R3.4: with nothing installed there is nothing to check, and the answer says
// what to run.
func TestToolCheckWithNoCompilerSaysWhatToRun(t *testing.T) {
	root := bare(t)
	game := probeGame(t, root)

	res, err := core.ToolCheck(t.Context(), core.ToolCheckRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.Equal(t, core.CompatNoCompiler, res.Status)
	require.Contains(t, res.Advice, "tools ensure")
}

// A probe file the install does not hold is skipped and named, rather than
// being reported as a compatibility failure it is not. R3.4.
func TestAProbeFileTheInstallDoesNotHoldIsSkipped(t *testing.T) {
	root := bare(t)
	game := probeGame(t, root)
	stubTools(t, "ok")

	res, err := core.ToolCheck(t.Context(), core.ToolCheckRequest{
		Request: core.Request{GameDir: game},
		Files:   []string{"gcgameplayglobals.global.mbin", "metadata/nothing/here.mbin"},
	})
	require.NoError(t, err)
	require.Equal(t, core.CompatOK, res.Status)
	require.Len(t, res.Files, 1)
	require.Contains(t, res.Skipped, "metadata/nothing/here.mbin")
}

// installTag writes a complete-looking compiler install under a tag, so the
// listing and removal paths have more than one to choose between.
func installTag(t *testing.T, tag string) string {
	t.Helper()
	dir := filepath.Join(config.Defaults().Paths().Tools, "mbincompiler", tag)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "MBINCompiler-linux-dotnet10"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o700)) //nolint:gosec // a test stub that has to be executable
	require.NoError(t, os.WriteFile(filepath.Join(dir, "libMBIN-linux-dotnet10.so"),
		[]byte("stub"), 0o600))
	return dir
}

/*
Removing the compiler builds are using would break the next build in a way the
user did not ask for and would not connect to what they just did.

The natural moment to press Remove is right after installing a newer release --
which is also the moment the release you are about to remove might still be the
active one, if the newer one is pinned away or failed to verify. Refusing, and
naming what to do instead, is the whole of the protection.
*/
func TestRemovingTheCompilerInUseIsRefused(t *testing.T) {
	bare(t)
	stubTools(t, "clean")

	_, err := core.RemoveTool(t.Context(), core.RemoveToolRequest{Tag: "v9.99.0-stub"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pin")
	require.DirExists(t, filepath.Join(config.Defaults().Paths().Tools, "mbincompiler", "v9.99.0-stub"),
		"and nothing was removed while refusing")
}

// A release that is installed but not in use goes, and nothing else does.
func TestRemovingAnUnusedCompilerLeavesTheActiveOneAlone(t *testing.T) {
	bare(t)
	stubTools(t, "clean")
	old := installTag(t, "v1.0.0")

	res, err := core.RemoveTool(t.Context(), core.RemoveToolRequest{Tag: "v1.0.0"})
	require.NoError(t, err)
	require.Equal(t, old, res.Dir)
	require.Positive(t, res.Files)
	require.NoDirExists(t, old)

	list, err := core.ListTools(t.Context(), core.ListToolsRequest{})
	require.NoError(t, err)
	require.Len(t, list.Entries, 1)
	require.Equal(t, "v9.99.0-stub", list.Entries[0].Tag)
	require.True(t, list.Entries[0].Active)
}

// A tag that is not a release tag is a path segment refused before it reaches
// the filesystem: this deletes a directory named from it.
func TestRemovingRejectsATagThatIsNotOne(t *testing.T) {
	bare(t)
	for _, tag := range []string{"../../etc", "not-a-tag", ""} {
		_, err := core.RemoveTool(t.Context(), core.RemoveToolRequest{Tag: tag})
		require.Errorf(t, err, "%q was accepted as a release tag", tag)
	}
}

// Removing something that was never installed says so plainly rather than
// reporting success for a directory that was not there.
func TestRemovingAnAbsentReleaseSaysSo(t *testing.T) {
	bare(t)
	_, err := core.RemoveTool(t.Context(), core.RemoveToolRequest{Tag: "v1.2.3"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not installed")
}

// The release listing must be usable with no network: the cache is empty here
// and --no-network forbids the fetch, so this is the offline-with-nothing-cached
// case, which has to fail with an explanation rather than hang or panic.
func TestListReleasesOfflineWithNoCacheExplainsItself(t *testing.T) {
	bare(t)
	_, err := core.ListReleases(t.Context(), core.ListReleasesRequest{
		Request: core.Request{NoNetwork: true},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "network")
}
