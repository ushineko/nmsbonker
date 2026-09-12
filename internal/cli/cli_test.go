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
	for _, want := range []string{
		"status", "detect", "tools", "pak", "mods", "build", "deploy", "report", "config", "version",
	} {
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

/*
R7: every subcommand the spec names is reachable and takes the flags it names.

A command that exists but spells its flag differently from the spec is a command
scripts and documentation cannot use, and cobra will happily accept the
definition either way, so the names are asserted rather than assumed.
*/
func TestTheModPipelineCommandsAndTheirFlagsExist(t *testing.T) {
	for _, tc := range []struct {
		path  []string
		flags []string
	}{
		{path: []string{"mods", "list"}, flags: []string{"json"}},
		{path: []string{"mods", "add"}, flags: []string{"replace"}},
		{path: []string{"mods", "remove"}, flags: []string{"delete"}},
		{path: []string{"mods", "enable"}},
		{path: []string{"mods", "disable"}},
		{path: []string{"mods", "move"}},
		{path: []string{"mods", "import"}, flags: []string{"pattern", "replace"}},
		{path: []string{"mods", "check"}, flags: []string{"json"}},
		{path: []string{"build"}, flags: []string{"recache", "deploy", "mod-name"}},
		{path: []string{"deploy"}, flags: []string{"replace-symlink"}},
		{path: []string{"report"}, flags: []string{"json"}},
		{path: []string{"tools", "check"}, flags: []string{"json"}},
	} {
		name := strings.Join(tc.path, " ")
		cmd, _, err := cli.Root().Find(tc.path)
		require.NoError(t, err, name)
		require.Equal(t, tc.path[len(tc.path)-1], cmd.Name(), "%s exists", name)
		for _, flag := range tc.flags {
			require.NotNil(t, cmd.Flags().Lookup(flag), "%s --%s", name, flag)
		}
	}
}

// R8.2: the new commands report a wrong argument count as a usage error, so a
// script can tell a typo from a build that ran and failed.
func TestTheModCommandsRejectWrongArgumentCountsAsUsageErrors(t *testing.T) {
	var usage *cli.UsageError
	for _, args := range [][]string{
		{"mods", "remove"},
		{"mods", "move", "OnlyOne"},
		{"mods", "import"},
		{"mods", "frobnicate"},
		{"build", "extra-arg"},
	} {
		_, err := run(t, args...)
		require.Error(t, err, "%v", args)
		require.ErrorAs(t, err, &usage, "%v", args)
	}
}

// `mods move NAME POSITION` takes a number; a word is a usage error rather than
// a silent move to position zero. R7.
func TestMovingToANonNumericPositionIsAUsageError(t *testing.T) {
	var usage *cli.UsageError
	_, err := run(t, "mods", "move", "Something", "first")
	require.Error(t, err)
	require.ErrorAs(t, err, &usage)
}
