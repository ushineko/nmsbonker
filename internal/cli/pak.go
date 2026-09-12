package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/core"
)

func newPakCmd() *cobra.Command {
	cmd := group("pak", "Read the game's .pak archives")
	cmd.AddCommand(newPakListCmd(), newPakFindCmd(), newPakExtractCmd(), newPakReindexCmd())
	return cmd
}

func newPakListCmd() *cobra.Command {
	var pak string
	cmd := &cobra.Command{
		Use:   "list [GLOB]",
		Short: "List the archives, or the files inside one of them",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := core.PakListRequest{Request: request(), Pak: pak}
			if len(args) == 1 {
				req.Glob = args[0]
			}
			res, err := core.PakList(cmd.Context(), req)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			var t table
			if pak == "" {
				t.header("PAK", "FILES", "BYTES")
				for _, p := range res.Paks {
					t.row(p.Name, strconv.Itoa(p.Files), strconv.FormatInt(p.Size, 10))
				}
			} else {
				t.header("FILE", "BYTES")
				for _, e := range res.Entries {
					t.row(e.Name, strconv.FormatUint(e.Size, 10))
				}
			}
			t.write(w)
			return nil
		},
	}
	cmd.Flags().StringVar(&pak, "pak", "", "list the contents of this archive instead of the archives themselves")
	return cmd
}

func newPakFindCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "find GLOB",
		Short: "Find a file across every archive",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.PakFind(cmd.Context(), core.PakFindRequest{Request: request(), Glob: args[0]})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(w, res)
			}
			if len(res.Matches) == 0 {
				say(w, "no match for %q in %d files across %d paks",
					res.Glob, res.IndexedFiles, res.IndexedPaks)
				return nil
			}
			var t table
			t.header("FILE", "PAK")
			for _, m := range res.Matches {
				t.row(m.Name, m.Pak)
			}
			t.write(w)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

func newPakExtractCmd() *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:   "extract INTERNAL_PATH",
		Short: "Write one file out of the archives, keeping its internal path",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.PakExtract(cmd.Context(), core.PakExtractRequest{
				Request: request(), Name: args[0], OutDir: outDir,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "file", res.Name)
			fact(w, "from", res.Pak)
			fact(w, "wrote", res.Path)
			fact(w, "bytes", res.Size)
			if res.ByBasename {
				fact(w, "note", "matched by basename; the exact path is not in any pak")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&outDir, "out", "o", ".", "directory to write into")
	return cmd
}

/*
newPakReindexCmd forces the index to be rebuilt (spec 003 R2.5).

The index maintains itself by size and mtime, which is right almost always. This
is for when it is not: an archive replaced with one the same size, or an index
left half-written by an interrupted run. It is the answer to "why does it say
that file is not in any pak".
*/
func newPakReindexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reindex",
		Short: "Discard the pak index and read every archive again",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.RebuildIndex(cmd.Context(), core.RebuildIndexRequest{Request: request()})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "index", res.Path)
			fact(w, "indexed", fmt.Sprintf("%d pak(s), %d file(s)", res.Paks, res.Files))
			fact(w, "bytes", res.Bytes)
			fact(w, "took", res.Duration.Round(time.Millisecond))
			return nil
		},
	}
}
