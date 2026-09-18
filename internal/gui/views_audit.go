package gui

import (
	"context"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/table"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/nmsbonker/internal/build/audit"
	"github.com/ushineko/nmsbonker/internal/core"
)

// --- the amount audit (spec 005 R3.1) ---------------------------------------

/*
The Report section's amount-audit block.

It sits above the mod table because the mod table cannot say this. Every row of
that table can read WORKING and the build still have produced a reward the game
cannot hold in a stack, because each edit was correct and it is their product
that is not. So the block leads with a verdict on the values, and when there is
something to say it names the mods that made it -- which is the one piece of
information "12 applied, 0 skipped" can never contain.

Re-check is here rather than on a toolbar because the thresholds are a
judgement call: the useful gesture is "lower the ratio limit in Settings, come
back, look again", and it must not require a rebuild.
*/

// maxAuditRowsShown is how many flagged amounts the table draws. The rest are
// counted; a scrolling table of four hundred is not read, and the report file
// has all of them.
const maxAuditRowsShown = 50

// auditResult is the audit the Report section is showing: the last build's,
// unless a re-check has produced a fresher one against the current limits.
func (u *ui) auditResult() (*audit.Result, bool) {
	if u.freshAudit != nil {
		return u.freshAudit.Run.Result, true
	}
	if u.lastReport.Report == nil {
		return nil, false
	}
	return u.lastReport.Report.Audit, false
}

/*
auditBlock is the whole block: verdict, table, advice and the two buttons.

Fixed shape in every state. The verdict row, the marker and the button strip are
always there, so re-checking with a different limit does not move the mod table
out from under the pointer (angou's rule: nothing transient reflows).
*/
func (u *ui) auditBlock() fyne.CanvasObject {
	a, fresh := u.auditResult()
	rows := []fyne.CanvasObject{u.auditVerdict(a, fresh)}

	if a != nil && len(a.Flags) > 0 {
		t := table.New()
		t.Header("Entry", "Item", "Stock", "Built", "Ratio", "Contributors")
		t.SetWidths(180, 160, 90, 130, 90, 460)
		shown := a.Flags
		if len(shown) > maxAuditRowsShown {
			shown = shown[:maxAuditRowsShown]
		}
		for _, f := range shown {
			t.Row(auditRowStatus(f), auditCells(f)...)
		}
		rows = append(rows, widgets.FixedHeight(t.Widget(), 200))
		if len(a.Flags) > len(shown) {
			rows = append(rows, widgets.Note(fmt.Sprintf(
				"%d further flagged amount(s) are not shown; the report file has all of them.",
				len(a.Flags)-len(shown)), fd.StatusWarn))
		}
		rows = append(rows, widgets.Note(auditAdvice, fd.StatusWarn))
	}
	if a != nil && a.Unauditable > 0 {
		rows = append(rows, widgets.Note(fmt.Sprintf(
			"%d block(s) could not be checked because a mod's add or remove changed the "+
				"table's structure.", a.Unauditable), fd.StatusInfo))
	}
	rows = append(rows, u.auditActions())
	return widgets.Card("Amount audit", rows...)
}

/*
auditCells is one row of the flag table.

Split out of the table's build so a headless test can assert what a flagged
amount renders as: Fyne builds a table's cells lazily from its callbacks, so
there is nothing in the object tree to read until it is on a canvas, and the
thing worth pinning is the content rather than the widget.
*/
func auditCells(f audit.Flag) []string {
	return []string{
		widgets.OrNone(f.EntryID, "—"), widgets.OrNone(f.Item, "—"),
		audit.Range(f.PristineMin, f.PristineMax),
		audit.Range(f.MergedMin, f.MergedMax),
		auditRatioText(f.Ratio), f.ContributorText(),
	}
}

// auditAdvice is the paragraph that says what to do, worded as the report file
// words it. Three renderings of one finding must not suggest three fixes.
const auditAdvice = "Every one of these edits applied correctly; the amounts are large because " +
	"they compound. Each mod multiplies what the mod before it left, so two reasonable " +
	"multipliers make an unreasonable amount and nothing in the mod table below can see it. " +
	"Two ways out, and they combine: disable or re-tune the script named most often in " +
	"Contributors, which is the one doing most of the multiplying; or put a ceiling on it " +
	"with a built-in tweak's cap parameter in the Tweaks section, which clamps the result " +
	"whatever ran before it. The limits themselves are in Settings, and Re-check applies a " +
	"new limit to this build without rebuilding."

// auditVerdict is the one row that is there in every state.
func (u *ui) auditVerdict(a *audit.Result, fresh bool) fyne.CanvasObject {
	suffix := " (from the last build)"
	if fresh {
		suffix = " (re-checked against the current limits)"
	}
	switch {
	case a == nil:
		return widgets.FactRow("Reward amounts", "not audited — no reward table in this build",
			fd.StatusInfo)
	case len(a.Flags) == 0:
		return widgets.FactRow("Reward amounts",
			fmt.Sprintf("No reward amount exceeds the configured limits — %d block(s) checked%s",
				a.Blocks, suffix), fd.StatusGood)
	default:
		return widgets.FactRow("Reward amounts",
			fmt.Sprintf("%d of %d reward amount(s) are above the configured limits%s",
				len(a.Flags), a.Blocks, suffix), auditStatus(len(a.Flags)))
	}
}

// auditActions is Re-check and Copy audit. Copy exists because the useful next
// step for a flagged table is often to paste it somewhere and read it properly.
func (u *ui) auditActions() fyne.CanvasObject {
	recheck := widget.NewButtonWithIcon("Re-check amounts", theme.SearchIcon(),
		func() { u.recheckAmounts() })
	copyAudit := widget.NewButtonWithIcon("Copy audit", theme.ContentCopyIcon(), func() {
		a, _ := u.auditResult()
		u.app.Clipboard().SetContent(auditText(a))
		u.flash("The amount audit is on the clipboard.", fd.StatusGood)
	})
	if a, _ := u.auditResult(); a == nil {
		copyAudit.Disable()
	}
	u.gate(recheck, copyAudit)
	return container.NewHBox(recheck, copyAudit)
}

/*
recheckAmounts re-runs the audit over the merge the last build kept.

It does not rebuild, which is the point of the operation: the merged MXML and
the pristine cache are both still on disk, so a threshold changed in Settings
can be tried against this build in about a second.
*/
func (u *ui) recheckAmounts() {
	u.perform("Re-checking the reward amounts…", func(ctx context.Context) error {
		res, err := core.Audit(ctx, core.AuditRequest{Request: u.request()})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			u.freshAudit = &res
			u.flash("Amount audit: "+res.Run.Result.Summary()+".",
				auditStatus(len(res.Run.Result.Flags)))
			u.rebuild()
		})
		return nil
	})
}

// auditText is the clipboard form: the same columns as the table, tab
// separated, with the limits under it so a pasted copy says what it was judged
// against.
func auditText(a *audit.Result) string {
	if a == nil {
		return "No reward table was audited in this build.\n"
	}
	var b strings.Builder
	if len(a.Flags) == 0 {
		fmt.Fprintf(&b, "No reward amount exceeds the configured limits (%d block(s) checked).\n",
			a.Blocks)
	} else {
		b.WriteString("Table\tEntry\tItem\tStock\tBuilt\tRatio\tWhy\tContributors\n")
		for _, f := range a.Flags {
			fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				f.Table, f.EntryID, f.Item,
				audit.Range(f.PristineMin, f.PristineMax),
				audit.Range(f.MergedMin, f.MergedMax),
				auditRatioText(f.Ratio), strings.Join(f.Reasons, "; "), f.ContributorText())
		}
	}
	if a.Unauditable > 0 {
		fmt.Fprintf(&b, "%d block(s) not auditable (an add or remove changed the structure).\n",
			a.Unauditable)
	}
	fmt.Fprintf(&b, "Limits: product %s, substance %s, units %s, nanites %s, "+
		"quicksilver %s, ratio x%s\n",
		audit.Amount(a.Thresholds.MaxProduct), audit.Amount(a.Thresholds.MaxSubstance),
		audit.Amount(a.Thresholds.MaxUnits), audit.Amount(a.Thresholds.MaxNanites),
		audit.Amount(a.Thresholds.MaxSpecials), audit.Amount(a.Thresholds.MaxRatio))
	return b.String()
}

/*
auditStatus ranks a flag count.

Any flag at all is a warning rather than an error: the build succeeded and the
mod folder is installable. Enough of them at once is a different situation --
one flagged reward is a mod to re-tune, forty is a library that is multiplying
everything -- so the marker goes red and the Overview card follows it.
*/
func auditStatus(flags int) fd.Status {
	switch {
	case flags == 0:
		return fd.StatusGood
	case flags < manyAuditFlags:
		return fd.StatusWarn
	default:
		return fd.StatusBad
	}
}

// manyAuditFlags is where "a mod to re-tune" becomes "a library that is
// multiplying everything".
const manyAuditFlags = 20

// auditRowStatus colours one row by how far its amount moved, so the salvage
// stack at x625000 does not read the same as a reward at x120.
func auditRowStatus(f audit.Flag) fd.Status {
	if f.Ratio >= badRatio || f.MergedMax == audit.MaxInt32 {
		return fd.StatusBad
	}
	return fd.StatusWarn
}

// badRatio is where a multiplier stops being a tuning choice.
const badRatio = 1000

// auditRatioText renders the ratio column, and a dash where the stock value was
// zero and there is no ratio to state.
func auditRatioText(ratio float64) string {
	if ratio <= 0 {
		return "—"
	}
	return "x" + audit.Amount(ratio)
}
