package cli

import (
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
		Args:  cobra.NoArgs,
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
	for _, warning := range res.Warnings {
		fact(w, "warning", warning)
	}
}
