package mbin_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/mbin"
)

/*
fakeScript stands in for MBINCompiler.

The real binary is 2 MB of .NET that this repository may not download during a
unit-test run, so the runner is tested against a shell script that reproduces
the behaviours the runner actually depends on: the two `version` forms, the
`-y -q -d <dir> <path>` conversion, the upper-cased output extension, a failing
exit with output on stderr, and a zero exit that produces nothing. Its mode is
read from a file beside it, not from the environment, because the runner
deliberately strips the environment (R5.4) and an env-driven fake could not tell
that apart from a bug.
*/
const fakeScript = `#!/bin/sh
here=$(dirname "$0")
mode=$(cat "$here/mode" 2>/dev/null || echo ok)
env > "$here/seen-env"

if [ "$1" = "version" ]; then
  if [ -n "$2" ]; then echo "Compiled with MBINCompiler v7.1.0.1"; else echo "MBINCompiler v7.01.0-pre1"; fi
  exit 0
fi

outdir=""; input=""
while [ $# -gt 0 ]; do
  case "$1" in
    -d) outdir="$2"; shift 2;;
    -y|-q|-Q) shift;;
    *) input="$1"; shift;;
  esac
done

case "$mode" in
  slow)
    d="$here/running"; mkdir -p "$d"; : > "$d/$$"
    ls "$d" | wc -l >> "$here/concurrency"
    sleep 0.4
    rm -f "$d/$$"
    ;;
  hang) sleep 30 ;;
  fail)
    echo "[ERROR]: Invalid file type." >&2
    exit 1
    ;;
  no-output) exit 0 ;;
esac

base=$(basename "$input"); stem=${base%.*}
case "$base" in
  *.MXML|*.mxml|*.EXML|*.exml) printf 'compiled' > "$outdir/$stem.MBIN" ;;
  *) printf '<?xml version="1.0"?>' > "$outdir/$stem.MXML" ;;
esac
exit 0
`

func fakeCompiler(t *testing.T, mode string) *mbin.Compiler {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "MBINCompiler-linux-dotnet10")
	require.NoError(t, os.WriteFile(bin, []byte(fakeScript), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mode"), []byte(mode), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "libMBIN-linux-dotnet10.so"), []byte("stub"), 0o600))
	mbin.SetMaxProcesses(4)
	return &mbin.Compiler{Bin: bin, Tag: "v7.01.0-pre1", Flavor: mbin.FlavorDotnet10}
}

// The two `version` forms answer different questions -- which compiler is this,
// and which libMBIN made this file -- and R6.1 reads the game data version out
// of the second.
func TestBothVersionFormsAreReadBack(t *testing.T) {
	c := fakeCompiler(t, "ok")
	out, err := c.Version(t.Context())
	require.NoError(t, err)
	require.Equal(t, "MBINCompiler v7.01.0-pre1", out)

	out, err = c.FileVersion(t.Context(), "/some/file.MBIN")
	require.NoError(t, err)
	require.Contains(t, out, "7.1.0.1")
}

// The compiler writes its output extension in upper case whatever the input's
// case was, and older releases wrote .EXML where current ones write .MXML.
// Assuming the extension rather than looking for it is how a build ends up
// reporting a missing file that is sitting right there. R5.4.
func TestConversionFindsTheOutputWhateverCaseTheExtensionIs(t *testing.T) {
	c := fakeCompiler(t, "ok")
	dir := t.TempDir()
	in := filepath.Join(dir, "gcgameplayglobals.global.mbin")
	require.NoError(t, os.WriteFile(in, []byte("mbin"), 0o600))
	out := filepath.Join(dir, "out")

	mxml, err := c.Decompile(t.Context(), in, out)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(out, "gcgameplayglobals.global.MXML"), mxml)
	require.FileExists(t, mxml)

	back := filepath.Join(dir, "back")
	mbinPath, err := c.Compile(t.Context(), mxml, back)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(back, "gcgameplayglobals.global.MBIN"), mbinPath)
}

// A conversion that exits zero and writes nothing is a real outcome -- a
// filtered input, a silently skipped file. Without its own error the build
// carries on and fails later on a missing file, a long way from the cause.
// R5.4.
func TestAZeroExitWithNoOutputIsItsOwnError(t *testing.T) {
	c := fakeCompiler(t, "no-output")
	dir := t.TempDir()
	in := filepath.Join(dir, "x.mbin")
	require.NoError(t, os.WriteFile(in, []byte("mbin"), 0o600))

	_, err := c.Decompile(t.Context(), in, filepath.Join(dir, "out"))
	require.ErrorIs(t, err, mbin.ErrNoOutput)
}

// The useful half of a conversion failure -- the libMBIN stack trace naming the
// field it choked on -- goes to <binary>.log, not to stderr. An error without
// it says only "exit status 1", which is what the legacy pipeline gave and what
// made structural-edit failures so slow to diagnose. R5.4.
func TestAFailedConversionCarriesStderrAndTheTailOfTheLog(t *testing.T) {
	c := fakeCompiler(t, "fail")
	var log strings.Builder
	for i := range 60 {
		log.WriteString("log line " + strconv.Itoa(i) + "\n")
	}
	require.NoError(t, os.WriteFile(c.Bin+".log", []byte(log.String()), 0o600))

	dir := t.TempDir()
	in := filepath.Join(dir, "x.mbin")
	require.NoError(t, os.WriteFile(in, []byte("mbin"), 0o600))

	_, err := c.Decompile(t.Context(), in, filepath.Join(dir, "out"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid file type", "stderr is in the error")
	require.Contains(t, err.Error(), "log line 59", "so is the end of the log")
	require.Contains(t, err.Error(), "log line 20", "the last 40 lines")
	require.NotContains(t, err.Error(), "log line 19", "and no more than that")
}

// The legacy pipeline had to strip WINEDEBUG and CLAUDECODE by hand before every
// subprocess, and forgetting one leaked an agent session's environment into a
// child process. Passing an allow-list makes that impossible rather than
// remembered. R5.4.
func TestTheChildProcessGetsAnAllowListEnvironmentAndNothingElse(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("WINEDEBUG", "-all")
	t.Setenv("DOTNET_ROOT", "/usr/share/dotnet")

	c := fakeCompiler(t, "ok")
	_, err := c.Version(t.Context())
	require.NoError(t, err)

	seen, err := os.ReadFile(filepath.Join(filepath.Dir(c.Bin), "seen-env"))
	require.NoError(t, err)
	env := string(seen)
	require.NotContains(t, env, "CLAUDECODE")
	require.NotContains(t, env, "WINEDEBUG")
	require.Contains(t, env, "DOTNET_ROOT=/usr/share/dotnet", "the .NET runtime still needs its own")
	require.Contains(t, env, "PATH=")
	require.Contains(t, env, "HOME=")
}

// A build the user stopped must not leave .NET runtimes chewing cores until
// they finish files nobody wants. R5.4.
func TestCancellingTheContextKillsTheProcess(t *testing.T) {
	c := fakeCompiler(t, "hang")
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	dir := t.TempDir()
	in := filepath.Join(dir, "x.mbin")
	require.NoError(t, os.WriteFile(in, []byte("mbin"), 0o600))

	started := time.Now()
	_, err := c.Decompile(ctx, in, filepath.Join(dir, "out"))
	require.Error(t, err)
	require.Less(t, time.Since(started), 5*time.Second, "the 30 s sleep was killed, not waited out")
}

// Each conversion is a .NET process with its own runtime and a copy of libMBIN,
// and a build converts a hundred files on a desktop the user is still using.
// Without the limit the build spawns as many processes as there are files.
// R5.5.
func TestConcurrentConversionsAreLimitedToTheConfiguredCount(t *testing.T) {
	c := fakeCompiler(t, "slow")
	mbin.SetMaxProcesses(2)
	t.Cleanup(func() { mbin.SetMaxProcesses(4) })

	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := range 6 {
		in := filepath.Join(dir, "f"+strconv.Itoa(i)+".mbin")
		require.NoError(t, os.WriteFile(in, []byte("mbin"), 0o600))
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Decompile(t.Context(), in, filepath.Join(dir, "out"))
		}()
	}
	wg.Wait()

	counts, err := os.ReadFile(filepath.Join(filepath.Dir(c.Bin), "concurrency"))
	require.NoError(t, err)
	lines := strings.Fields(string(counts))
	require.Len(t, lines, 6, "every conversion ran")
	for _, line := range lines {
		n, err := strconv.Atoi(line)
		require.NoError(t, err)
		require.LessOrEqual(t, n, 2, "more than two MBINCompiler processes existed at once")
	}
}
