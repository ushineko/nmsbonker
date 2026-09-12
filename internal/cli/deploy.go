package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
)

func newDeployCmd() *cobra.Command {
	var (
		replaceSymlink bool
		modName        string
	)
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Install the last build under GAMEDATA/MODS",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Deploy(cmd.Context(), core.DeployRequest{
				Request: request(), ReplaceSymlink: replaceSymlink, ModName: modName,
			})
			if err != nil {
				return err
			}
			printDeploy(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().BoolVar(&replaceSymlink, "replace-symlink", false,
		"replace a symlinked GAMEDATA/MODS with a real directory (the link only, never its target)")
	cmd.Flags().StringVar(&modName, "mod-name", "", "folder name, overriding the mod_name setting")
	return cmd
}

func printDeploy(w io.Writer, res core.DeployResult) {
	fact(w, "installed", res.Dest)
	fact(w, "files", res.Files)
	if res.ReplacedSymlink != "" {
		fact(w, "removed symlink", "was -> "+res.ReplacedSymlink+" (the target was left alone)")
	}
	if res.Archived != "" {
		fact(w, "archived", res.Archived)
	}
	if res.SettingsPath != "" {
		state := "already correct"
		switch {
		case res.SettingsAdded:
			state = "entry added, enabled"
		case res.SettingsChanged:
			state = "updated"
		}
		fact(w, "mod settings", state+" ("+res.SettingsPath+")")
		fact(w, "DisableAllMods", res.DisableAllMods)
	}
	if b := res.SaveBackup; b != nil {
		switch {
		case b.Error != "":
			fact(w, "save backup", "failed: "+b.Error)
		case b.Skipped != "":
			fact(w, "save backup", "skipped: "+b.Skipped)
		default:
			fact(w, "save backup", fmt.Sprintf("%d profile(s), %d file(s) in %s",
				b.Profiles, b.Files, b.Dir))
		}
	}
	if len(res.Pruned) > 0 {
		fact(w, "pruned", fmt.Sprintf("%d older archive(s): %s", len(res.Pruned), join(res.Pruned)))
	}
	for _, warning := range res.Warnings {
		fact(w, "warning", warning)
	}
}
