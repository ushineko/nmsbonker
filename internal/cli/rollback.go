package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
)

/*
Undoing a deploy (spec 004 R3.3, R4).

Four commands, and the split between them is the point. `rollback` puts an
earlier deployment back; `undeploy` takes the current one out; `mods-off` leaves
everything where it is and tells the game to load none of it. When a game stops
starting, the cheapest thing to try is the last of those, because it changes
nothing that has to be rebuilt afterwards.
*/

func newRollbackCmd() *cobra.Command {
	var modName string
	cmd := &cobra.Command{
		Use:   "rollback [TIMESTAMP]",
		Short: "Put an archived deployment back into the game",
		Long: "Swaps the installed mod folder for one out of the archive, newest by\n" +
			"default. What is installed now becomes a new archive entry, so a rollback\n" +
			"can itself be rolled back. The workspace build is not touched.",
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var stamp string
			if len(args) == 1 {
				stamp = args[0]
			}
			res, err := core.Rollback(cmd.Context(), core.RollbackRequest{
				Request: request(), Timestamp: stamp, ModName: modName,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "restored", res.From.Timestamp)
			if res.Removed {
				fact(w, "removed", res.Dest)
			} else {
				fact(w, "installed", res.Dest)
				fact(w, "files", res.Files)
			}
			if res.SettingsRestored != "" {
				fact(w, "settings restored", res.SettingsRestored)
			}
			fact(w, "archived", res.Archived)
			for _, warning := range res.Warnings {
				fact(w, "warning", warning)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&modName, "mod-name", "", "folder name, overriding the mod_name setting")
	return cmd
}

func newUndeployCmd() *cobra.Command {
	var modName string
	cmd := &cobra.Command{
		Use:   "undeploy",
		Short: "Remove the installed mod folder, keeping a copy in the archive",
		Long: "Takes GAMEDATA/MODS/<mod name> out of the game and archives it. The game's\n" +
			"own mod settings are left alone: an entry naming a folder that is no longer\n" +
			"there is harmless, and turning mods off is `nmsbonker mods-off`.",
		Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Undeploy(cmd.Context(), core.UndeployRequest{
				Request: request(), ModName: modName,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "removed", res.Dest)
			fact(w, "archived", res.Archived)
			fact(w, "files", res.Files)
			if len(res.Pruned) > 0 {
				fact(w, "pruned", fmt.Sprintf("%d older archive(s): %s", len(res.Pruned), join(res.Pruned)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&modName, "mod-name", "", "folder name, overriding the mod_name setting")
	return cmd
}

func newArchiveCmd() *cobra.Command {
	cmd := group("archive", "What deploy has displaced, and can put back")
	cmd.AddCommand(newArchiveListCmd())
	return cmd
}

func newArchiveListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the archived deployments, newest first",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ListArchive(cmd.Context(), core.ListArchiveRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w := cmd.OutOrStdout()
			fact(w, "archive dir", res.Dir)
			fact(w, "retention", fmt.Sprintf("the newest %d are kept", res.Retention))
			if len(res.Entries) == 0 {
				say(w, "%s", "Nothing has been deployed yet, so there is nothing to roll back to.")
				return nil
			}
			var t table
			t.header("TIMESTAMP", "MOD", "HOLDS", "FILES", "SIZE")
			for _, e := range res.Entries {
				t.row(e.Timestamp, e.ModName, holdsText(e),
					fmt.Sprintf("%d", e.Files), humanSize(e.Bytes))
			}
			t.write(w)
			say(w, "")
			say(w, "%s", "`nmsbonker rollback [TIMESTAMP]` puts one back; the newest is the default.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

// holdsText says what an archive entry can restore.
func holdsText(e core.ArchiveEntry) string {
	switch {
	case e.HasMod && e.HasSettings:
		return "mod folder + mod settings"
	case e.HasMod:
		return "mod folder"
	case e.HasSettings:
		return "mod settings (nothing was installed)"
	}
	return "nothing"
}

func newModsToggleCmd(off bool) *cobra.Command {
	use, short := "mods-on", "Let the game load mods again (DisableAllMods=false)"
	if off {
		use, short = "mods-off", "Stop the game loading any mod, without removing anything"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long: "Flips DisableAllMods in the game's own GCMODSETTINGS.MXML and changes nothing\n" +
			"else. Every mod stays installed and every per-mod switch keeps its state, so\n" +
			"turning mods back on restores exactly what was loading before.",
		Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ModsToggle(cmd.Context(), core.ModsToggleRequest{
				Request: request(), DisableAll: off,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "settings", res.Path)
			fact(w, "DisableAllMods", res.DisableAll)
			if !res.Changed {
				fact(w, "note", "it was already set that way; nothing was written")
			}
			if len(res.Mods) > 0 {
				fact(w, "mods listed", join(res.Mods))
			}
			return nil
		},
	}
}
