package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
)

func newModsCmd() *cobra.Command {
	cmd := group("mods", "Manage the mod library and the build order")
	cmd.AddCommand(
		newModsShowCmd(), newModsWriteCmd(),
		newModsListCmd(), newModsAddCmd(), newModsRemoveCmd(),
		newModsEnableCmd(true), newModsEnableCmd(false),
		newModsMoveCmd(), newModsImportCmd(), newModsCheckCmd(),
	)
	return cmd
}

func newModsListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the mods in build order",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ListMods(cmd.Context(), core.ListModsRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w := cmd.OutOrStdout()
			fact(w, "library dir", res.LibraryDir)
			for _, n := range res.Notices {
				fact(w, "notice", n)
			}
			if len(res.Mods) == 0 {
				say(w, "%s", "No mods yet. Run `nmsbonker mods import DIR` or `nmsbonker mods add FILE.lua`.")
				return nil
			}
			var t table
			t.header("#", "MOD", "SOURCE", "ENABLED", "STATUS")
			for i, m := range res.Mods {
				status := m.Status
				if m.Shadowed {
					status += ", shadowing a library script"
				}
				t.row(strconv.Itoa(i+1), m.Name, m.Source, yesNo(m.Enabled), status)
			}
			t.write(w)
			// The order is the conflict rule, so it is worth saying once rather
			// than leaving the user to infer it from a numbered list.
			say(w, "%s", "")
			say(w, "%s", "Lower in the list is applied later and wins where two mods change the same value.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

func newModsAddCmd() *cobra.Command {
	var replace bool
	cmd := &cobra.Command{
		Use:   "add PATH...",
		Short: "Copy .lua mod scripts into the library and enable them",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.AddMod(cmd.Context(), core.AddModRequest{
				Request: request(), Paths: args, Replace: replace, Enabled: true,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			for _, n := range res.Added {
				say(w, "added %s", n)
			}
			for _, n := range res.Replaced {
				say(w, "replaced %s", n)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite a script of the same name")
	return cmd
}

func newModsRemoveCmd() *cobra.Command {
	var deleteFile bool
	cmd := &cobra.Command{
		Use:   "remove NAME",
		Short: "Take a mod out of the build order",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.RemoveMod(cmd.Context(), core.RemoveModRequest{
				Request: request(), Name: args[0], DeleteFile: deleteFile,
			})
			if err != nil {
				return err
			}
			say(cmd.OutOrStdout(), "removed %s", res.Name)
			if res.Deleted != "" {
				say(cmd.OutOrStdout(), "deleted %s", res.Deleted)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&deleteFile, "delete", false, "also delete the .lua from the library")
	return cmd
}

func newModsEnableCmd(enable bool) *cobra.Command {
	verb, past, short := "enable", "enabled", "Include mods in the next build"
	if !enable {
		verb, past, short = "disable", "disabled", "Leave mods out of the next build"
	}
	return &cobra.Command{
		Use:   verb + " NAME...",
		Short: short,
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.SetModEnabled(cmd.Context(), core.SetModEnabledRequest{
				Request: request(), Names: args, Enabled: enable,
			})
			if err != nil {
				return err
			}
			for _, n := range res.Changed {
				say(cmd.OutOrStdout(), "%s %s", past, n)
			}
			return nil
		},
	}
}

func newModsMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "move NAME POSITION",
		Short: "Move a mod to a position in the build order (1 is first)",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pos, err := strconv.Atoi(args[1])
			if err != nil {
				return usagef("POSITION must be a number, got %q", args[1])
			}
			res, err := core.MoveMod(cmd.Context(), core.MoveModRequest{
				Request: request(), Name: args[0], To: pos,
			})
			if err != nil {
				return err
			}
			say(cmd.OutOrStdout(), "moved %s from %d to %d", res.Name, res.From, res.To)
			return nil
		},
	}
}

func newModsImportCmd() *cobra.Command {
	var (
		pattern string
		replace bool
	)
	cmd := &cobra.Command{
		Use:   "import DIR",
		Short: "Copy every .lua in a directory into the library and enable them",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.ImportDir(cmd.Context(), core.ImportDirRequest{
				Request: request(), Dir: args[0], Pattern: pattern, Replace: replace, Enabled: true,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "library dir", res.LibraryDir)
			fact(w, "imported", len(res.Added))
			if len(res.Replaced) > 0 {
				fact(w, "replaced", len(res.Replaced))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&pattern, "pattern", "*.lua", "which files to import")
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite scripts of the same name")
	return cmd
}

func newModsCheckCmd() *cobra.Command {
	var (
		asJSON bool
		all    bool
	)
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Load every enabled mod script and report what it says",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.CheckMods(cmd.Context(), core.CheckModsRequest{
				Request: request(), All: all, IncludeDump: asJSON,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w := cmd.OutOrStdout()
			var t table
			t.header("MOD", "FILES", "EDITS", "LAST BUILD", "NOTE")
			for _, m := range res.Mods {
				t.row(m.Name, strconv.Itoa(len(m.Targets)), strconv.Itoa(m.Blocks),
					lastBuildCell(m), checkNote(m))
			}
			t.write(w)
			say(w, "")
			fact(w, "loaded", fmt.Sprintf("%d ok, %d failed", res.OK, res.Failed))
			if res.Failed > 0 {
				return fmt.Errorf("%d mod script(s) did not load", res.Failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result, including the decoded scripts, as JSON")
	cmd.Flags().BoolVar(&all, "all", false, "check disabled mods too")
	return cmd
}

/*
lastBuildCell is what the last build made of one script (spec 005 R4.1).

Verdict and counts in one cell rather than three columns: they are read
together -- "NOT BUILT 0/12" is one fact -- and three narrow columns of numbers
pushed the note that says what to do about it off the right of the terminal.
*/
func lastBuildCell(m core.ModCheck) string {
	if m.Verdict == "" {
		return "—"
	}
	return fmt.Sprintf("%s %d/%d", m.Verdict, m.Applied, m.Skipped)
}

/*
checkNote is the one note column, worst thing first.

A script that will not load has nothing else worth saying about it, so that
wins; after it come the effectiveness signals, then the keys a game update took
away, then the directives this engine ignores.
*/
func checkNote(m core.ModCheck) string {
	if !m.OK {
		return m.Error
	}
	var parts []string
	if m.Effect != "" {
		parts = append(parts, m.Effect)
	}
	if len(m.NotFound) > 0 {
		parts = append(parts, "keys not found: "+join(m.NotFound))
	}
	if len(m.Unsupported) > 0 {
		parts = append(parts, "ignored keys: "+join(m.Unsupported))
	}
	return strings.Join(parts, "; ")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// newModsShowCmd prints a script's text (spec 010 R3).
func newModsShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print a mod's script as it is on disk (or compiled in, for a built-in)",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.ReadModScript(cmd.Context(), core.ModScriptRequest{Request: request(), Name: args[0]})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			_, err = io.WriteString(cmd.OutOrStdout(), res.Text)
			return err //nolint:wrapcheck // the writer's own error
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

// newModsWriteCmd replaces a library script's text (spec 010 R3).
func newModsWriteCmd() *cobra.Command {
	var check, force bool
	cmd := &cobra.Command{
		Use:   "write <name> <file|->",
		Short: "Replace a library mod's script with a file (or stdin with -), keeping a .bak",
		Long: "The new text has to load through the sandbox, or it is not written; --force writes\n" +
			"it anyway (the build will then report the mod NOT BUILT). The previous text is kept\n" +
			"beside the script as <name>.lua.bak. Built-in tweaks cannot be written.",
		Args: exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var body []byte
			var err error
			if args[1] == "-" {
				body, err = io.ReadAll(cmd.InOrStdin())
			} else {
				body, err = os.ReadFile(args[1])
			}
			if err != nil {
				return fmt.Errorf("read the new text: %w", err)
			}
			res, err := core.WriteModScript(cmd.Context(), core.WriteModScriptRequest{
				Request: request(), Name: args[0], Text: string(body), Check: check, Force: force,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "mod", res.Name)
			fact(w, "script", res.Path)
			if res.Loads {
				fact(w, "loads", fmt.Sprintf("yes, %d change block(s)", res.Blocks))
			} else {
				fact(w, "loads", "no: "+res.LoadError)
			}
			switch {
			case check:
				say(w, "%s", "Check only: nothing was written.")
			case !res.Written:
				say(w, "%s", "The file already holds that text; nothing was written.")
			default:
				fact(w, "wrote", fmt.Sprintf("%d bytes", res.Bytes))
				fact(w, "previous text", res.Backup)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "load the text through the sandbox and write nothing")
	cmd.Flags().BoolVar(&force, "force", false, "write even if the text does not load")
	return cmd
}
