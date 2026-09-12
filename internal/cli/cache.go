package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
)

func newCacheCmd() *cobra.Command {
	cmd := group("cache", "Show and clear the derived caches")
	cmd.AddCommand(newCacheShowCmd(), newCacheClearCmd())
	return cmd
}

func newCacheShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Report what the pristine cache, the pak index and the release listing hold",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.CacheInfo(cmd.Context(), core.CacheInfoRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w := cmd.OutOrStdout()
			fact(w, "cache dir", res.Dir)
			fact(w, "current buildid", orNone(res.CurrentBuildID))
			fact(w, "pristine cache", fmt.Sprintf("%d file(s), %d bytes", res.TotalFiles, res.TotalBytes))
			if len(res.Games) > 0 {
				var t table
				t.header("", "BUILDID", "FILES", "BYTES", "WRITTEN")
				for _, g := range res.Games {
					marker := ""
					if g.Current {
						marker = "*"
					}
					t.row(marker, g.BuildID, strconv.Itoa(g.Files),
						strconv.FormatInt(g.Bytes, 10), g.Modified.Format(time.RFC3339))
				}
				t.write(w)
			}
			say(w, "")
			idx := res.PakIndex
			switch {
			case !idx.Exists:
				fact(w, "pak index", "not built yet")
			case idx.Stale == 0 && idx.Missing == 0:
				fact(w, "pak index", fmt.Sprintf("current: %d paks, %d files, %d bytes, written %s",
					idx.Paks, idx.Files, res.PakIndexBytes, idx.Written.Format(time.RFC3339)))
			default:
				fact(w, "pak index", fmt.Sprintf("stale: %d pak(s) changed, %d gone, written %s",
					idx.Stale, idx.Missing, idx.Written.Format(time.RFC3339)))
			}
			fact(w, "release listing", fmt.Sprintf("%d bytes (%s)", res.ReleasesBytes, res.ReleasesPath))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

func newCacheClearCmd() *cobra.Command {
	var (
		all   bool
		index bool
	)
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Delete the cached pristine game files, which the next build rebuilds",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ClearCache(cmd.Context(), core.ClearCacheRequest{
				Request: request(), All: all, IncludeIndex: index,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(res.Removed) == 0 {
				say(w, "%s", "Nothing was cached, so nothing was removed.")
				return nil
			}
			for _, p := range res.Removed {
				fact(w, "removed", p)
			}
			fact(w, "freed", fmt.Sprintf("%d file(s), %d bytes", res.Files, res.Bytes))
			say(w, "%s", "The next build re-extracts and re-decompiles what it needs. "+
				"Your mod library, the build output and the game are untouched.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "clear every cached game build, not only the installed one")
	cmd.Flags().BoolVar(&index, "index", false, "also delete the pak index")
	return cmd
}

// orNone is the CLI's stand-in for an empty value, so a blank field never reads
// as a formatting fault.
func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
