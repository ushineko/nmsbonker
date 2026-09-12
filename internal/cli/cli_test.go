package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/cli"
)

// run executes the command tree with the given arguments and captures output.
func run(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()
	root := cli.Root()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), err
}

// R8.3: a bare `nmsbonker` prints help and exits 0. Printing usage to stderr
// and exiting non-zero is the common alternative and it makes the first thing a
// new user types look like a failure.
func TestBareInvocationPrintsHelpAndSucceeds(t *testing.T) {
	out, err := run(t)
	require.NoError(t, err)
	require.Contains(t, out, "Available Commands")
	for _, want := range []string{"status", "detect", "tools", "pak", "config", "version"} {
		require.Contains(t, out, want, "every command in R8.1 is reachable")
	}
}

// R8.2: a misuse of the command line exits 2, distinct from a command that ran
// and failed, so a script can tell them apart. main keys off UsageError.
func TestMisuseIsAUsageErrorSoItCanExitTwo(t *testing.T) {
	var usage *cli.UsageError

	_, err := run(t, "frobnicate")
	require.Error(t, err)
	require.ErrorAs(t, err, &usage, "an unknown command is a usage error")

	_, err = run(t, "status", "--nope")
	require.Error(t, err)
	require.ErrorAs(t, err, &usage, "an unknown flag is a usage error")

	_, err = run(t, "pak", "find")
	require.Error(t, err)
	require.ErrorAs(t, err, &usage, "a missing argument is a usage error")

	_, err = run(t, "config", "set", "only-one-arg")
	require.Error(t, err)
	require.ErrorAs(t, err, &usage)

	_, err = run(t, "tools", "frobnicate")
	require.Error(t, err)
	require.ErrorAs(t, err, &usage, "a group's unknown subcommand too")
}

// The version line is what a bug report quotes, so it has to carry both halves.
func TestVersionPrintsWhatTheBinaryWasBuiltFrom(t *testing.T) {
	out, err := run(t, "version")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(out, "nmsbonker "))
	require.Contains(t, out, "(", "the commit is in parentheses after the version")
}

// Output is plain text with no ANSI colour (R8.2): it is read in terminals, in
// pipes and in bug reports, and escape sequences make the last two worse.
func TestOutputCarriesNoANSIEscapes(t *testing.T) {
	out, err := run(t, "version")
	require.NoError(t, err)
	require.NotContains(t, out, "\x1b[")

	out, _ = run(t, "--help")
	require.NotContains(t, out, "\x1b[")
}
