package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
)

// newSavesCmd is the save-backup group (spec 004 R5.2).
func newSavesCmd() *cobra.Command {
	cmd := group("saves", "Copy the game's saves out of the Proton prefix")
	cmd.AddCommand(newSavesBackupCmd(), newSavesListCmd())
	return cmd
}

func newSavesBackupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "backup",
		Short: "Copy every save profile to a timestamped folder",
		Long: "Copies the st_* folders out of the game's Proton prefix. Copy only: nothing\n" +
			"is ever written back into the prefix, and restoring is a manual copy.",
		Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.BackupSaves(cmd.Context(), core.BackupSavesRequest{Request: request()})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if res.Skipped != "" {
				fact(w, "skipped", res.Skipped)
				return nil
			}
			fact(w, "source", res.Source)
			fact(w, "backup", res.Dir)
			fact(w, "profiles", res.Profiles)
			fact(w, "files", fmt.Sprintf("%d (%s)", res.Files, humanSize(res.Bytes)))
			if len(res.Pruned) > 0 {
				fact(w, "pruned", fmt.Sprintf("%d older backup(s): %s", len(res.Pruned), join(res.Pruned)))
			}
			return nil
		},
	}
}

func newSavesListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the save backups, newest first",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ListSaveBackups(cmd.Context(),
				core.ListSaveBackupsRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w := cmd.OutOrStdout()
			fact(w, "backup dir", res.Dir)
			fact(w, "retention", fmt.Sprintf("the newest %d are kept", res.Retention))
			fact(w, "before a deploy", yesNo(res.Enabled)+" (config save_backup)")
			if len(res.Backups) == 0 {
				say(w, "%s", "No save backups yet.")
				return nil
			}
			var t table
			t.header("TIMESTAMP", "PROFILES", "FILES", "SIZE")
			for _, b := range res.Backups {
				t.row(b.Timestamp, fmt.Sprintf("%d", b.Profiles),
					fmt.Sprintf("%d", b.Files), humanSize(b.Bytes))
			}
			t.write(w)
			if res.Source != "" {
				say(w, "")
				say(w, "%s", "To restore one, close the game and copy an st_* folder back to:")
				say(w, "  %s", res.Source)
				say(w, "%s", "nmsbonker never writes into the prefix itself.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}
