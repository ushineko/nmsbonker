package cli

import (
	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
)

func newToolsCmd() *cobra.Command {
	cmd := group("tools", "Acquire and inspect MBINCompiler")
	cmd.AddCommand(newToolsEnsureCmd(), newToolsListCmd(), newToolsCheckCmd(),
		newToolsPinCmd(), newToolsUnpinCmd())
	return cmd
}

func newToolsEnsureCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ensure",
		Short: "Install the MBINCompiler release this game needs, if it is not already installed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.EnsureTools(cmd.Context(), core.EnsureToolsRequest{Request: request()})
			w := cmd.OutOrStdout()
			// The attempts are printed even on failure: "dotnet10 does not run
			// here, self-contained was not published for this release" is the
			// whole diagnosis, and it is lost if only the error is shown.
			for _, a := range res.Attempts {
				say(w, "%s", "  "+a)
			}
			if err != nil {
				return err
			}
			fact(w, "release listing", res.ListingSource)
			if res.Warning != "" {
				fact(w, "warning", res.Warning)
			}
			fact(w, "game data version", res.GameDataVersion)
			fact(w, "selected", res.Tag)
			fact(w, "reason", res.Reason)
			fact(w, "flavor", res.Flavor)
			if res.AlreadyPresent {
				fact(w, "result", "already installed")
			} else {
				fact(w, "result", "installed")
			}
			fact(w, "binary", res.Bin)
			return nil
		},
	}
}

func newToolsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the installed MBINCompiler releases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ListTools(cmd.Context(), core.ListToolsRequest{Request: request()})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "tools dir", res.ToolsDir)
			if res.Pin != "" {
				fact(w, "pinned", res.Pin)
			}
			if len(res.Entries) == 0 {
				say(w, "%s", "Nothing installed. Run `nmsbonker tools ensure`.")
				return nil
			}
			var t table
			t.header("", "TAG", "FLAVOR", "REPORTS")
			for _, e := range res.Entries {
				marker := ""
				if e.Active {
					marker = "*"
				}
				t.row(marker, e.Tag, e.Flavor, e.Version)
			}
			t.write(w)
			return nil
		},
	}
}

func newToolsPinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pin TAG",
		Short: "Always use this MBINCompiler release",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.PinTool(cmd.Context(), core.PinToolRequest{Request: request(), Tag: args[0]})
			if err != nil {
				return err
			}
			fact(cmd.OutOrStdout(), "pinned", res.Pin)
			if !res.Installed {
				say(cmd.OutOrStdout(), "%s", "It is not installed yet; `nmsbonker tools ensure` will fetch it.")
			}
			return nil
		},
	}
}

func newToolsUnpinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpin",
		Short: "Go back to choosing an MBINCompiler release automatically",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := core.PinTool(cmd.Context(), core.PinToolRequest{Request: request()}); err != nil {
				return err
			}
			fact(cmd.OutOrStdout(), "pinned", "(none)")
			return nil
		},
	}
}

/*
newToolsCheckCmd is the compatibility check (spec 002 R3.4).

It answers the question a version string could not: whether the installed
MBINCompiler can read this game install's files and write them back unchanged.
A mismatch is reported with the file that proved it, and does not stop a build
-- but it does mean the build's output is worth doubting.
*/
func newToolsCheckCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Round-trip game files through the installed compiler to prove it matches",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ToolCheck(cmd.Context(), core.ToolCheckRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w := cmd.OutOrStdout()
			fact(w, "compiler", res.CompilerVersion)
			fact(w, "result", res.Status)
			for _, f := range res.Files {
				state := "round-trips"
				if !f.OK {
					state = f.Reason
				}
				fact(w, "  "+f.Name, state)
			}
			for name, why := range res.Skipped {
				fact(w, "  "+name, "skipped: "+why)
			}
			if res.Detail != "" {
				fact(w, "detail", res.Detail)
			}
			if res.Advice != "" {
				fact(w, "advice", res.Advice)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}
