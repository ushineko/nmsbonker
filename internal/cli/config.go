package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
)

func newConfigCmd() *cobra.Command {
	cmd := group("config", "Show and change nmsbonker's settings")
	cmd.AddCommand(newConfigShowCmd(), newConfigSetCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print every setting and the directories they resolve to",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ConfigShow(cmd.Context(), core.ConfigShowRequest{Request: request()})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "config file", res.Path)
			say(w, "")
			var t table
			t.header("KEY", "VALUE")
			for _, e := range res.Entries {
				t.row(e.Key, e.Value)
			}
			t.write(w)
			say(w, "")
			gameDir := res.GameDir
			if gameDir == "" {
				gameDir = "(discovered; run `nmsbonker detect`)"
			}
			fact(w, "resolved game dir", fmt.Sprintf("%s [%s]", gameDir, res.GameDirSource))
			fact(w, "resolved library", res.Paths.Library)
			fact(w, "resolved tools", res.Paths.Tools)
			fact(w, "resolved cache", res.Paths.Cache)
			fact(w, "resolved workspace", res.Paths.Workspace)
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set KEY VALUE",
		Short: "Change one setting",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.ConfigSet(cmd.Context(), core.ConfigSetRequest{
				Request: request(), Key: args[0], Value: args[1],
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if res.Unchanged {
				say(w, "%s is already %q", res.Key, res.New)
				return nil
			}
			say(w, "%s: %q -> %q", res.Key, res.Old, res.New)
			fact(w, "written to", res.Path)
			return nil
		},
	}
}
