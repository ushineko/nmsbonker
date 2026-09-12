package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
)

// writeJSON prints a result as indented JSON for the --json forms (R8.1).
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode result as JSON: %w", err)
	}
	return nil
}

func newStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report the game, the tools and the caches this machine has",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Status(cmd.Context(), core.StatusRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			printStatus(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

func printStatus(w io.Writer, res core.StatusResult) {
	fact(w, "nmsbonker", fmt.Sprintf("%s (%s)", res.Version, res.Commit))
	fact(w, "config", res.ConfigPath)
	fact(w, "library dir", fmt.Sprintf("%s (%d .lua)", res.Paths.Library, res.LibraryMods))
	fact(w, "tools dir", res.Paths.Tools)
	fact(w, "cache dir", res.Paths.Cache)
	fact(w, "workspace dir", res.Paths.Workspace)
	fact(w, "output folder", res.ModName)
	say(w, "")

	in := res.Install
	if !in.Found {
		fact(w, "game", "not found ("+in.Error+")")
		say(w, "%s", "Run `nmsbonker detect` to see every place that was examined.")
	} else {
		fact(w, "game dir", in.Dir)
		fact(w, "game dir from", in.Source)
		fact(w, "steam library", in.LibraryDir)
		fact(w, "name", in.Name)
		fact(w, "buildid", in.BuildID)
		fact(w, "paks", fmt.Sprintf("%d in %s", in.PakCount, in.PCBanksDir))
		mods := in.ModsState
		if in.ModsTarget != "" {
			mods += " -> " + in.ModsTarget
		}
		fact(w, "GAMEDATA/MODS", mods)
		if in.CompatDataDir != "" {
			fact(w, "proton prefix", in.CompatDataDir)
		}
		if in.ModSettingsOK {
			fact(w, "mod settings", in.ModSettingsPath)
			fact(w, "DisableAllMods", in.DisableAllMods)
			for _, m := range in.Mods {
				fact(w, "  mod", fmt.Sprintf("%s (enabled=%t, priority=%d)", m.Name, m.Enabled, m.ModPriority))
			}
		} else {
			fact(w, "mod settings", "absent ("+in.ModSettingsPath+")")
		}
	}
	say(w, "")

	c := res.Compiler
	if c.Installed {
		fact(w, "MBINCompiler", fmt.Sprintf("%s (%s)", c.Tag, c.Flavor))
		fact(w, "  binary", c.Bin)
		fact(w, "  reports", c.Version)
	} else {
		fact(w, "MBINCompiler", "not installed (run `nmsbonker tools ensure`)")
	}
	fact(w, "dotnet 10 runtime", yesNo(c.Dotnet10))
	if len(c.Others) > 0 {
		fact(w, "  also installed", fmt.Sprint(c.Others))
	}
	fact(w, "game data version", res.GameDataVersion)
	fact(w, "compatibility", res.Compatibility)
	say(w, "")

	idx := res.PakIndex
	switch {
	case !idx.Exists:
		fact(w, "pak index", "not built yet")
	case idx.Stale == 0 && idx.Missing == 0:
		fact(w, "pak index", fmt.Sprintf("current: %d paks, %d files, written %s",
			idx.Paks, idx.Files, idx.Written.Format(time.RFC3339)))
	default:
		fact(w, "pak index", fmt.Sprintf("stale: %d pak(s) changed, %d gone, written %s",
			idx.Stale, idx.Missing, idx.Written.Format(time.RFC3339)))
	}
}

func newDetectCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Show every place the game was looked for, and why each was rejected",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Detect(cmd.Context(), core.DetectRequest{Request: request()})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(w, res)
			}
			fact(w, "resolution", res.Source)
			if res.Found {
				fact(w, "game dir", res.GameDir)
			} else {
				fact(w, "result", "not found ("+res.Error+")")
			}
			say(w, "")
			say(w, "%s", "Steam roots examined, in order:")
			for _, root := range res.Roots {
				say(w, "%s", "  "+root)
			}
			say(w, "")

			var t table
			t.header("LIBRARY", "GAME DIR", "VERDICT")
			for _, c := range res.Candidates {
				verdict := "ok"
				if c.Reason != "" {
					verdict = "rejected: " + c.Reason
				}
				lib := c.LibraryDir
				if lib == "" {
					lib = c.Root
				}
				if lib == "" {
					lib = "(explicit --game-dir)"
				}
				t.row(lib, c.GameDir, verdict)
			}
			t.write(w)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}
