package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
)

func newModsCmd() *cobra.Command {
	cmd := group("mods", "Manage the mod library and the build order")
	cmd.AddCommand(
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
			t.header("#", "MOD", "ENABLED", "STATUS")
			for i, m := range res.Mods {
				t.row(strconv.Itoa(i+1), m.Name, yesNo(m.Enabled), m.Status)
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
	verb, past := "enable", "enabled"
	if !enable {
		verb, past = "disable", "disabled"
	}
	return &cobra.Command{
		Use:   verb + " NAME...",
		Short: "Include mods in the next build",
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
			t.header("MOD", "FILES", "EDITS", "NOTE")
			for _, m := range res.Mods {
				note := ""
				switch {
				case !m.OK:
					note = m.Error
				case len(m.Unsupported) > 0:
					note = "ignored keys: " + join(m.Unsupported)
				}
				t.row(m.Name, strconv.Itoa(len(m.Targets)), strconv.Itoa(m.Blocks), note)
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

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
