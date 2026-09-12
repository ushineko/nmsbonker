package mbin_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/mbin"
)

/*
roundTripScript is a compiler that converts by copying, with a mode that decides
how faithfully.

The real check is "does what comes back equal what went in", so a fake that can
be told to come back identical, to differ only in the header, to differ in the
body, to change length, or to fail outright covers every branch of the verdict
without needing the 2 MB .NET binary or a copy of the game.
*/
const roundTripScript = `#!/bin/sh
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
[ -z "$input" ] && { echo "MBINCompiler v0.0.0-fake"; exit 0; }

base=$(basename "$input"); stem=${base%.*}
case "$base" in
  *.MBIN|*.mbin)
    [ "$mode" = "faildecompile" ] && { echo "[ERROR]: unknown template" >&2; exit 1; }
    cat "$input" > "$outdir/$stem.MXML"
    exit 0
    ;;
esac

[ "$mode" = "failcompile" ] && { echo "[ERROR]: unexpected element" >&2; exit 1; }
out="$outdir/$stem.MBIN"
case "$mode" in
  short) head -c 200 "$input" > "$out" ;;
  *)     cat "$input" > "$out" ;;
esac
case "$mode" in
  header) printf 'X' | dd of="$out" bs=1 seek=24 conv=notrunc 2>/dev/null ;;
  body)   printf 'X' | dd of="$out" bs=1 seek=200 conv=notrunc 2>/dev/null ;;
esac
exit 0
`

func roundTripCompiler(t *testing.T, mode string) *mbin.Compiler {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "MBINCompiler-linux-dotnet10")
	require.NoError(t, os.WriteFile(bin, []byte(roundTripScript), 0o700)) //nolint:gosec // a test stub
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mode"), []byte(mode), 0o600))
	mbin.SetMaxProcesses(2)
	return &mbin.Compiler{Bin: bin, Tag: "v0.0.0-fake", Flavor: mbin.FlavorDotnet10}
}

// probe writes a file big enough to have a body past the skipped header.
func probe(t *testing.T) string {
	t.Helper()
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 251)
	}
	path := filepath.Join(t.TempDir(), "probe.MBIN")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

// R3.4: a compiler that reproduces the file is compatible.
func TestARoundTripThatReturnsTheSameBytesIsCompatible(t *testing.T) {
	rt, err := roundTripCompiler(t, "ok").CheckRoundTrip(t.Context(), probe(t), t.TempDir())
	require.NoError(t, err)
	require.True(t, rt.OK, rt.Reason)
	require.Equal(t, 1024, rt.OriginalSize)
	require.Equal(t, 1024, rt.RebuiltSize)
	require.Equal(t, -1, rt.FirstDiff)
}

/*
R3.4: a difference inside the header is expected and ignored.

MBINCompiler stamps its own format id and version there, so every recompiled
file differs from the pristine one at those offsets. Comparing whole files would
report every install as a mismatch and make the check useless.
*/
func TestADifferenceInsideTheHeaderIsNotAMismatch(t *testing.T) {
	rt, err := roundTripCompiler(t, "header").CheckRoundTrip(t.Context(), probe(t), t.TempDir())
	require.NoError(t, err)
	require.True(t, rt.OK, rt.Reason)
}

// A difference past the header is a real one, and the message says where. R3.4.
func TestADifferenceInTheBodyIsAMismatchAndNamesTheOffset(t *testing.T) {
	rt, err := roundTripCompiler(t, "body").CheckRoundTrip(t.Context(), probe(t), t.TempDir())
	require.NoError(t, err)
	require.False(t, rt.OK)
	require.Equal(t, 200, rt.FirstDiff)
	require.Contains(t, rt.Reason, "probe.MBIN round-tripped to different bytes, first at offset 0xC8")
}

/*
R3.4: a length change is a mismatch even when the skipped header could hide it.

The length check is what makes the 0x60 skip safe: a compiler that dropped a
field would otherwise be able to produce a shorter file whose first difference
fell inside the ignored range.
*/
func TestALengthChangeIsAMismatch(t *testing.T) {
	rt, err := roundTripCompiler(t, "short").CheckRoundTrip(t.Context(), probe(t), t.TempDir())
	require.NoError(t, err)
	require.False(t, rt.OK)
	require.Contains(t, rt.Reason, "round-tripped to a different size (200 bytes, was 1024)")
}

// AC9: a compiler that cannot read or cannot write the file is a mismatch with
// the compiler's own message attached, because "mismatch" alone tells the user
// nothing to act on.
func TestAConversionFailureIsAMismatchThatQuotesTheCompiler(t *testing.T) {
	rt, err := roundTripCompiler(t, "faildecompile").CheckRoundTrip(t.Context(), probe(t), t.TempDir())
	require.NoError(t, err)
	require.False(t, rt.OK)
	require.Contains(t, rt.Reason, "probe.MBIN did not decompile")
	require.Contains(t, rt.Reason, "unknown template")

	rt, err = roundTripCompiler(t, "failcompile").CheckRoundTrip(t.Context(), probe(t), t.TempDir())
	require.NoError(t, err)
	require.False(t, rt.OK)
	require.Contains(t, rt.Reason, "probe.MBIN did not recompile")
	require.Contains(t, rt.Reason, "unexpected element")
}

// A file that is nothing but header has only its length to check; the
// comparison must not index past it. R3.4.
func TestAFileSmallerThanTheSkippedHeaderIsHandled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiny.MBIN")
	require.NoError(t, os.WriteFile(path, make([]byte, 8), 0o600))

	rt, err := roundTripCompiler(t, "ok").CheckRoundTrip(t.Context(), path, t.TempDir())
	require.NoError(t, err)
	require.True(t, rt.OK, rt.Reason)
	require.Equal(t, mbin.HeaderSkip, 0x60)
}
