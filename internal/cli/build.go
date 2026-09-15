package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/core"
)

func newBuildCmd() *cobra.Command {
	var (
		recache  bool
		deploy   bool
		modName  string
		all      bool
		replaceL bool
	)
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Merge every enabled mod into one collision-free mod folder",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Build(cmd.Context(), core.BuildRequest{
				Request: request(), Recache: recache, Deploy: deploy,
				ModName: modName, All: all, ReplaceSymlink: replaceL,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			for _, l := range res.Report.Lines {
				say(w, "%s", l.Render())
			}
			say(w, "")
			printReport(w, res.Report)
			say(w, "")
			fact(w, "output", res.OutputDir)
			fact(w, "report", res.ReportPaths.Markdown)
			if res.Deployed != nil {
				printDeploy(w, *res.Deployed)
			}
			// R7: a dropped target means a game file this build could not ship,
			// which a script wrapping the build needs to notice. A NOT BUILT mod
			// is a warning, not a failure: the other mods still installed.
			if res.Report.Dropped > 0 {
				return fmt.Errorf("%d target file(s) were dropped; see %s",
					res.Report.Dropped, res.ReportPaths.Markdown)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&recache, "recache", false, "re-extract and re-decompile every game file first")
	f.BoolVar(&deploy, "deploy", false, "install the result under GAMEDATA/MODS when it succeeds")
	f.BoolVar(&replaceL, "replace-symlink", false, "with --deploy, replace a symlinked GAMEDATA/MODS")
	f.StringVar(&modName, "mod-name", "", "output folder name, overriding the mod_name setting")
	f.BoolVar(&all, "all", false, "build disabled mods too")
	return cmd
}

func newReportCmd() *cobra.Command {
	var (
		asJSON   bool
		markdown bool
	)
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Show the last build's report",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Report(cmd.Context(), core.ReportRequest{Request: request()})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			switch {
			case asJSON:
				return writeJSON(w, res.Report)
			case markdown:
				say(w, "%s", strings.TrimRight(res.Markdown, "\n"))
			default:
				printReport(w, res.Report)
				fact(w, "report", res.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the stored report.json")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "print BUILD_REPORT.md")
	return cmd
}

// printReport renders the verdict table and the header facts a user reads after
// a build.
func printReport(w io.Writer, r *report.Result) {
	var t table
	t.header("MOD", "STATUS", "EDITS", "SKIPPED", "NOTES")
	for _, m := range r.Mods {
		t.row(m.Name, m.Verdict, strconv.Itoa(m.Applied), strconv.Itoa(m.Skipped), modNote(m))
	}
	t.write(w)
	say(w, "")
	fact(w, "MBINs built", fmt.Sprintf("%d built, %d dropped", r.Built, r.Dropped))
	fact(w, "edits", fmt.Sprintf("%d applied, %d skipped", r.Applied, r.Skipped))
	fact(w, "compiler", r.CompilerVersion)
	compat := r.Compatibility
	if r.CompatibilityDetail != "" {
		compat += " (" + r.CompatibilityDetail + ")"
	}
	fact(w, "compatibility", compat)
	if len(r.UnsupportedKeys) > 0 {
		fact(w, "ignored script keys", join(r.UnsupportedKeys))
	}
	for _, m := range r.CacheMisses {
		fact(w, "missing game file", m)
	}
	if len(r.CompilerFailures) > 0 {
		fact(w, "COMPILER FAILURES", fmt.Sprintf("MBINCompiler %s could not handle %d file(s); "+
			"run `nmsbonker tools check` and pin a matching release", orDash(r.CompilerVersion), len(r.CompilerFailures)))
		for _, f := range r.CompilerFailures {
			fact(w, "  did not "+f.Stage, f.Internal+": "+report.FirstLine(f.Detail))
		}
	}
	fact(w, "timings", fmt.Sprintf(
		"%s wall clock (cache %s; merge %s, audit %s and compile %s summed over %d workers)",
		r.Timings.Total.Round(1e6), r.Timings.Cache.Round(1e6), r.Timings.Merge.Round(1e6),
		r.Timings.Audit.Round(1e6), r.Timings.Compile.Round(1e6), r.Workers))
	// R1.5: one line, always, and never a non-zero exit. A flagged amount is a
	// warning about a value; the build itself succeeded.
	fact(w, "amount audit", auditSummary(r))
	if r.Capped > 0 {
		fact(w, "capped values", r.Capped)
	}
}

// auditSummary is the `amount audit:` line (spec 005 R1.5). "not run" is a real
// answer: a build whose mods touch neither reward table has nothing to audit,
// and reporting that as "clean" would be a claim nobody checked.
func auditSummary(r *report.Result) string {
	if r.Audit == nil {
		return "not run (no audited table in this build)"
	}
	return r.Audit.Summary()
}

func modNote(m report.ModResult) string {
	switch m.Verdict {
	case report.Partial:
		return "structural add/remove skipped; value edits kept"
	case report.WorkingStructural:
		return "adds/removes entries — verify in game"
	case report.WorkingSkipped:
		seen := map[string]bool{}
		var keys []string
		for _, k := range m.NotFound {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			return ""
		}
		return "keys not found: " + join(keys)
	default:
		return ""
	}
}

// join renders a short list for one table cell, eliding a long one.
func join(items []string) string {
	const maxItems = 6
	if len(items) > maxItems {
		return strings.Join(items[:maxItems], ", ") + " …"
	}
	return strings.Join(items, ", ")
}
