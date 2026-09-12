/*
Package cli assembles the nmsbonker command tree (spec 001 R8).

Every command here does the same three things: build a core request from flags,
call one core operation, and print the result. No command reaches past core into
hgpak, steam or mbin, because the GUI (spec 003) cannot, and the two front ends
have to be able to disagree only about presentation.

Output is plain text with no ANSI colour: one fact per line, or a fixed-width
table for lists. A --json form exists where a script is the likely reader.
*/
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/buildinfo"
	"github.com/ushineko/nmsbonker/internal/core"
)

// UsageError marks a failure caused by how the command was invoked. main turns
// it into exit code 2 so a script can tell "you typed it wrong" from "the
// operation ran and failed" (R8.2).
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

func usagef(format string, args ...any) error {
	return &UsageError{Err: fmt.Errorf(format, args...)}
}

// globalFlags are the flags every command shares (R8.1).
type globalFlags struct {
	configPath string
	gameDir    string
	verbose    bool
	noNetwork  bool
}

// global holds the parsed values. A package-level variable because cobra's
// flag binding needs a stable address at command-construction time; it is
// written once, by flag parsing, before any command runs.
//
//nolint:gochecknoglobals // cobra's flag binding requires a stable address
var global globalFlags

// request builds the core request every operation takes.
func request() core.Request {
	return core.Request{
		ConfigPath: global.configPath,
		GameDir:    global.gameDir,
		NoNetwork:  global.noNetwork,
		Events:     events(),
	}
}

/*
events routes core's log lines to stderr and drops progress.

Progress is deliberately unrendered by the CLI. A progress bar on a command that
finishes in 60 ms is noise, and the two operations slow enough to want one
(indexing a cold install, downloading a compiler) already print what they did
when they finish. The GUI is where Progress earns its place.
*/
func events() core.Events {
	return core.Events{
		Log: func(level core.Level, msg string) {
			if level == core.LevelDebug && !global.verbose {
				return
			}
			say(os.Stderr, "nmsbonker: %s: %s", level, msg)
		},
	}
}

// Root builds the command tree.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "nmsbonker",
		Short: "Build No Man's Sky mods from AMUMSS Lua scripts, on Linux",
		Long: "nmsbonker rebuilds AMUMSS-format .lua mod scripts against the game files you\n" +
			"actually have installed, merges every enabled mod into one collision-free mod\n" +
			"folder, and deploys it. No Wine, no Windows VM, no Python.",
		Version: fmt.Sprintf("%s (%s)", buildinfo.Version, buildinfo.Commit),
		// Errors are reported once, by main, with the program name. Leaving
		// cobra's own reporting on prints every failure twice.
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		// R8.3: bare `nmsbonker` prints help and exits 0. An unknown command is
		// a usage error, which is exit 2.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help() //nolint:wrapcheck // cobra's own writer error, nothing to add
			}
			return usagef("unknown command %q for %q", args[0], cmd.CommandPath())
		},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &UsageError{Err: err} })

	pf := root.PersistentFlags()
	pf.StringVar(&global.configPath, "config", "", "settings file (default $XDG_CONFIG_HOME/nmsbonker/config.json)")
	pf.StringVar(&global.gameDir, "game-dir", "", "game directory, overriding the setting and $NMSBONKER_GAME_DIR")
	pf.BoolVarP(&global.verbose, "verbose", "v", false, "report what nmsbonker is doing on stderr")
	pf.BoolVar(&global.noNetwork, "no-network", false, "never contact GitHub; use the cached release listing")

	root.AddCommand(
		newStatusCmd(),
		newDetectCmd(),
		newToolsCmd(),
		newPakCmd(),
		newConfigCmd(),
		newVersionCmd(),
	)
	return root
}

// group builds a command that only holds subcommands, with the same
// unknown-subcommand behaviour as the root.
func group(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help() //nolint:wrapcheck // cobra's own writer error, nothing to add
			}
			return usagef("unknown command %q for %q", args[0], cmd.CommandPath())
		},
	}
}

// exactArgs is cobra.ExactArgs with the error marked as a usage error, so a
// wrong argument count exits 2 rather than 1 (R8.2).
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return usagef("%s takes exactly %d argument(s), got %d\n\nUsage:\n  %s",
				cmd.CommandPath(), n, len(args), cmd.UseLine())
		}
		return nil
	}
}

// maxArgs is cobra.MaximumNArgs with the same treatment.
func maxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return usagef("%s takes at most %d argument(s), got %d\n\nUsage:\n  %s",
				cmd.CommandPath(), n, len(args), cmd.UseLine())
		}
		return nil
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version and commit this binary was built from",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			say(cmd.OutOrStdout(), "nmsbonker %s (%s)", buildinfo.Version, buildinfo.Commit)
			return nil
		},
	}
}
