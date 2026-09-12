package cli

import (
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ushineko/nmsbonker/internal/build/audit"
	"github.com/ushineko/nmsbonker/internal/core"
)

/*
`nmsbonker audit` re-checks the reward amounts of the last build (spec 005 R1.5).

It exists because the thresholds are a judgement call. Answering "is x100 the
right ratio limit for my library" means trying three numbers, and if each try
costs a build nobody tries any. This reads the merge the build already left in
the workspace, so `config set audit.max_ratio 5 && nmsbonker audit` answers in
about a second.

It exits 0 whether or not anything is flagged. A flagged amount is a warning
about a value, not a failure of the build, and a script that wraps this should
not have to distinguish "the audit ran" from "the audit found something": the
--json form carries the count.
*/
func newAuditCmd() *cobra.Command {
	var (
		asJSON  bool
		modName string
	)
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Re-check the last build's reward amounts against the configured limits",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Audit(cmd.Context(), core.AuditRequest{
				Request: request(), ModName: modName,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			printAudit(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	cmd.Flags().StringVar(&modName, "mod-name", "",
		"output folder name whose kept merge to audit, overriding the mod_name setting")
	return cmd
}

// printAudit renders the flags as a table, then the facts a reader needs to act
// on them: which limits were applied, and where the files it read live.
func printAudit(w io.Writer, res core.AuditResult) {
	a := res.Run.Result
	if a != nil && len(a.Flags) > 0 {
		var t table
		t.header("TABLE", "ENTRY", "ITEM", "STOCK", "BUILT", "RATIO", "WHY", "CONTRIBUTORS")
		for _, f := range a.Flags {
			t.row(f.Table, f.EntryID, f.Item,
				audit.Range(f.PristineMin, f.PristineMax),
				audit.Range(f.MergedMin, f.MergedMax),
				"x"+audit.Amount(f.Ratio),
				strings.Join(f.Reasons, "; "), f.ContributorText())
		}
		t.write(w)
		say(w, "")
	}

	fact(w, "amount audit", a.Summary())
	fact(w, "tables checked", orNone(strings.Join(res.Run.Checked, ", ")))
	if len(res.Run.Missing) > 0 {
		fact(w, "no kept merge for", strings.Join(res.Run.Missing, ", "))
	}
	if a != nil {
		fact(w, "blocks compared", a.Blocks)
		if a.Unauditable > 0 {
			fact(w, "not auditable", a.Unauditable)
		}
	}
	fact(w, "limits", thresholdText(res.Thresholds))
	fact(w, "merged MXML", res.MergeDir)
	fact(w, "pristine cache", res.CacheDir)
	fact(w, "took", res.Run.Duration.Round(1e6))
	if a != nil && len(a.Flags) > 0 {
		say(w, "")
		say(w, "%s", "Every edit applied; the amounts compound. Disable or re-tune the script "+
			"named most often above, or cap the built-in that multiplies it "+
			"(`nmsbonker tweaks set ChestAndLootMaterials10x LOOT_CAP 50000`).")
	}
}

// thresholdText names the six limits in one line, in the order `config show`
// lists them, so the two readings of the same numbers agree.
func thresholdText(t audit.Thresholds) string {
	return "product " + audit.Amount(t.MaxProduct) +
		", substance " + audit.Amount(t.MaxSubstance) +
		", units " + audit.Amount(t.MaxUnits) +
		", nanites " + audit.Amount(t.MaxNanites) +
		", quicksilver " + audit.Amount(t.MaxSpecials) +
		", ratio x" + audit.Amount(t.MaxRatio)
}
