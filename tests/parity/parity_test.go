//go:build parity

/*
Package parity holds the CLI/GUI feature-parity guard (spec 003 R5.1).

It is a package of its own, behind a build tag, for two reasons. It imports both
front ends, which no other test does and no binary does — the CLI must not link
the GUI at all — and `make test` passes the tag deliberately, so the guard runs
on every test run rather than when someone remembers to ask for it. It needs no
display: nothing here constructs a window.

If this fails, the fix is to implement the missing side, not to edit the
allow-list.
*/
package parity

import (
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/cli"
	"github.com/ushineko/nmsbonker/internal/gui"
)

/*
notInTheGUI are commands with no GUI affordance, each with the reason.

An entry here is a decision someone made and can be argued with, which is the
difference between an exception and an omission.
*/
var notInTheGUI = map[string]string{
	// Not an operation. The window title carries the version, the About section
	// shows it with the commit, and there is nothing to run.
	"version": "the window title and About already show what it prints",
	// cobra's own, generated rather than written here.
	"completion": "generates shell completion scripts, which a window cannot use",
	"help":       "cobra's help command",
}

/*
TestTheCLIAndTheGUIExposeTheSameOperations walks the cobra tree and gui.Actions()
and fails when either holds an operation the other does not.

Leaf commands, not top-level groups: `tools` is a heading, `tools pin` is an
operation, and comparing at the group level would let a whole subcommand land
with no GUI surface as long as its siblings had one.
*/
func TestTheCLIAndTheGUIExposeTheSameOperations(t *testing.T) {
	inCLI := map[string]bool{}
	for _, name := range leaves(cli.Root(), "") {
		if _, skip := notInTheGUI[name]; skip {
			continue
		}
		inCLI[name] = true
	}
	require.NotEmpty(t, inCLI, "the command tree came back empty, so this proves nothing")

	inGUI := map[string]bool{}
	for _, a := range gui.Actions() {
		inGUI[a] = true
	}

	var missingFromGUI, missingFromCLI []string
	for name := range inCLI {
		if !inGUI[name] {
			missingFromGUI = append(missingFromGUI, name)
		}
	}
	for name := range inGUI {
		if !inCLI[name] {
			missingFromCLI = append(missingFromCLI, name)
		}
	}
	sort.Strings(missingFromGUI)
	sort.Strings(missingFromCLI)

	require.Emptyf(t, missingFromGUI,
		"these commands have no GUI affordance: %v\n"+
			"Implement them in internal/gui and add them to gui.Actions(), or declare the "+
			"exception in notInTheGUI with a reason.", missingFromGUI)
	require.Emptyf(t, missingFromCLI,
		"the GUI claims operations the CLI does not have: %v\n"+
			"Either the name is wrong, or a feature landed in the GUI first, which the "+
			"project rule forbids.", missingFromCLI)
}

// The allow-list is the interesting half of this file, so it is held to the
// same rule as the rest: an entry with no reason beside it is an omission
// somebody wrote down rather than a decision anybody made.
func TestEveryAllowListEntryCarriesAReason(t *testing.T) {
	for name, reason := range notInTheGUI {
		require.NotEmptyf(t, strings.TrimSpace(reason),
			"%q is allow-listed with no reason given", name)
	}
}

// leaves returns the full path of every command that actually does something —
// a group with subcommands is a heading, not an operation.
func leaves(cmd *cobra.Command, prefix string) []string {
	var out []string
	for _, c := range cmd.Commands() {
		name := strings.Fields(c.Use)[0]
		path := strings.TrimSpace(prefix + " " + name)
		if len(c.Commands()) == 0 {
			out = append(out, path)
			continue
		}
		out = append(out, leaves(c, path)...)
	}
	return out
}
