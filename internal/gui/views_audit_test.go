package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/build/audit"
	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/core"
)

// --- the amount audit (spec 005 R3.1) ---------------------------------------

// auditUI is a window holding one report, with the audit result under test.
func auditUI(t *testing.T, a *audit.Result) *ui {
	t.Helper()
	u := testUI(t)
	u.lastReport = core.ReportResult{Report: &report.Result{Audit: a}}
	u.lastReportOK = true
	return u
}

/*
The block says which of three things happened, and looks different in each.

Green when the amounts are inside the limits, ranked when they are not, and
neither when there was no reward table to audit. Getting the third wrong is the
one that matters: a green marker on a build that audited nothing is a claim
nobody checked, and it is exactly the claim the old report made by omission.
*/
func TestTheAuditBlockDistinguishesCleanFromNotAudited(t *testing.T) {
	notAudited := cardText(auditUI(t, nil).auditBlock())
	require.Contains(t, notAudited, "not audited — no reward table in this build")
	require.NotContains(t, notAudited, "No reward amount exceeds")

	clean := cardText(auditUI(t, &audit.Result{Blocks: 2431}).auditBlock())
	require.Contains(t, clean, "No reward amount exceeds the configured limits — 2431 block(s) "+
		"checked (from the last build)")

	flagged := cardText(auditUI(t, flaggedAudit()).auditBlock())
	require.Contains(t, flagged, "1 of 2431 reward amount(s) are above the configured limits")
}

// flaggedAudit is one compounded reward, as the build reports it.
func flaggedAudit() *audit.Result {
	return &audit.Result{
		Blocks: 2431, Thresholds: audit.Defaults(),
		Flags: []audit.Flag{{
			Table: "REWARDTABLE", EntryID: "R_CHEST", Item: "BP_SALVAGE",
			Category: audit.CategoryProduct, PristineMin: 2, PristineMax: 4,
			MergedMin: 1250000, MergedMax: 2500000, Ratio: 625000,
			Reasons: []string{"product amount 2500000 over the 99999 limit"},
			Contributors: []audit.Contribution{
				{Mod: "BetterRewards", Factor: 250, Max: 1000},
				{Mod: "BetterRewards", Factor: 250, Max: 250000},
				{Mod: "ChestAndLootMaterials10x", Factor: 10, Max: 2500000},
			},
		}},
	}
}

/*
A flagged block carries what the user needs to act, and what to do about it.

The contributors are the point. Everything else in the Report section can be
read as "your mods worked"; this is the only place that says which of them
multiplied the same reward three times over.
*/
func TestAFlaggedAmountNamesItsContributorsAndTheWayOut(t *testing.T) {
	require.Equal(t, []string{
		"R_CHEST", "BP_SALVAGE", "2-4", "1250000-2500000", "x625000",
		"BetterRewards x250 -> 1000, BetterRewards x250 -> 250000, " +
			"ChestAndLootMaterials10x x10 -> 2500000",
	}, auditCells(flaggedAudit().Flags[0]))

	text := cardText(auditUI(t, flaggedAudit()).auditBlock())
	require.Contains(t, text, "they compound", "the block says why, not only what")
	require.Contains(t, text, "cap parameter", "and how to stop it")
	require.Contains(t, text, "Re-check applies a new limit to this build without rebuilding")
}

// Copy audit puts the same finding on the clipboard, with the limits it was
// judged against: a pasted table that does not say what "too large" meant is
// a table nobody can act on a week later.
func TestCopyAuditPutsTheFindingAndTheLimitsOnTheClipboard(t *testing.T) {
	u := auditUI(t, flaggedAudit())
	tapButton(t, u.auditBlock(), "Copy audit")

	got := u.app.Clipboard().Content()
	require.Contains(t, got, "R_CHEST\tBP_SALVAGE\t2-4\t1250000-2500000\tx625000")
	require.Contains(t, got, "Limits: product 99999, substance 999999, units 100000000, "+
		"nanites 1000000, quicksilver 100000, ratio x100")
}

// Nothing to copy is not a crash: the button is there in every state and is
// disabled when there is no audit behind it.
func TestCopyAuditIsDisabledWithNothingToCopy(t *testing.T) {
	block := auditUI(t, nil).auditBlock()
	require.True(t, buttonNamed(t, block, "Copy audit").Disabled())
	require.False(t, buttonNamed(t, block, "Re-check amounts").Disabled())
}

/*
The marker ranks the count, so a library that is multiplying everything does not
read like one mod that needs a nudge.

Any flag is a warning -- the build succeeded and the folder is installable --
and enough of them at once is a different situation.
*/
func TestTheAuditMarkerRanksHowManyAmountsAreFlagged(t *testing.T) {
	require.Equal(t, StatusGood, auditStatus(0))
	require.Equal(t, StatusWarn, auditStatus(1))
	require.Equal(t, StatusWarn, auditStatus(manyAuditFlags-1))
	require.Equal(t, StatusBad, auditStatus(manyAuditFlags))
}

// A row is coloured by how far its amount moved, not by the fact that it moved.
func TestAnAuditRowIsRankedByItsRatio(t *testing.T) {
	require.Equal(t, StatusWarn, auditRowStatus(audit.Flag{Ratio: 120}))
	require.Equal(t, StatusBad, auditRowStatus(audit.Flag{Ratio: badRatio}))
	require.Equal(t, StatusBad, auditRowStatus(audit.Flag{Ratio: 2, MergedMax: audit.MaxInt32}),
		"a saturated value is bad however small the ratio looks")
	require.Equal(t, "—", auditRatioText(0), "no stock value means no ratio to state")
}

// Overview says the same thing in one line, and says the not-audited case as
// such rather than as "no flags".
func TestOverviewReportsTheAmountFlags(t *testing.T) {
	require.Contains(t, cardText(amountFlagsRow(nil)), "no reward table in that build")
	require.Contains(t, cardText(amountFlagsRow(&audit.Result{})), "no amount flags")
	require.Contains(t, cardText(amountFlagsRow(flaggedAudit())), "1 amount flag(s) — see Report")
}

// The Report facts card says whether a tweak's cap changed anything, in both
// states, so the row does not appear and disappear between builds.
func TestTheReportSaysWhetherACapBit(t *testing.T) {
	require.Equal(t, "none — no tweak cap changed a value", cappedText(0))
	require.Equal(t, "206 value(s) held back by a tweak cap", cappedText(206))
}

// --- audit limits in Settings (R3.3) ----------------------------------------

// The six limits are in the settings form and go through core like every other
// key, so the window and `nmsbonker config set` cannot disagree about them.
func TestTheAuditLimitsSaveThroughTheSettingsForm(t *testing.T) {
	u, path := settingsUI(t)
	f := u.newSettingsForm()

	require.Equal(t, "100", f.limits["audit.max_ratio"].Text, "the default is in the field")
	f.limits["audit.max_ratio"].SetText("5")
	test.Tap(f.save)

	require.FileExists(t, path)
	res, err := core.ConfigShow(t.Context(), core.ConfigShowRequest{
		Request: core.Request{ConfigPath: path},
	})
	require.NoError(t, err)
	values := map[string]string{}
	for _, e := range res.Entries {
		values[e.Key] = e.Value
	}
	require.Equal(t, "5", values["audit.max_ratio"])
	require.Equal(t, "99999", values["audit.max_product"], "the others are untouched")
}

// Reset puts the defaults back in the fields and writes nothing, like every
// other control on this form.
func TestResettingTheAuditLimitsChangesTheFormAndNotTheFile(t *testing.T) {
	u, path := settingsUI(t)
	f := u.newSettingsForm()
	f.limits["audit.max_ratio"].SetText("5")

	tapButton(t, u.auditGroup(f), "Reset limits to defaults")

	require.Equal(t, "100", f.limits["audit.max_ratio"].Text)
	require.NoFileExists(t, path, "Reset writes nothing; Save does")
}

// The form's own idea of the defaults must be the audit package's, or Reset
// restores numbers the build never used.
func TestTheFormsDefaultLimitsAreTheAuditPackagesDefaults(t *testing.T) {
	values := defaultAuditValues()
	d := audit.Defaults()
	require.Equal(t, audit.Amount(d.MaxProduct), values["audit.max_product"])
	require.Equal(t, audit.Amount(d.MaxSubstance), values["audit.max_substance"])
	require.Equal(t, audit.Amount(d.MaxUnits), values["audit.max_units"])
	require.Equal(t, audit.Amount(d.MaxNanites), values["audit.max_nanites"])
	require.Equal(t, audit.Amount(d.MaxSpecials), values["audit.max_specials"])
	require.Equal(t, audit.Amount(d.MaxRatio), values["audit.max_ratio"])
	require.Len(t, values, len(auditKeys))
}

// --- library hygiene in the Mods dialog (R4.1) ------------------------------

/*
The detail dialog explains the mechanical note rather than repeating it.

"no effective edits" fits a CLI table cell; here there is room to say what
follows from it, and what follows is the part somebody can act on.
*/
func TestTheModDetailExplainsTheEffectivenessNote(t *testing.T) {
	dead := core.ModCheck{Effect: core.EffectNoEdits}
	require.Contains(t, effectBlurb(dead), "Nothing this script asks for reached the merged files")
	require.Equal(t, StatusBad, effectStatus(dead))

	failing := core.ModCheck{Effect: core.EffectMostlyFailing}
	require.Contains(t, effectBlurb(failing), "written for an older version of the game")
	require.Equal(t, StatusWarn, effectStatus(failing))

	overlap := core.ModCheck{
		Effect:   "overlaps built-in ChestAndLootMaterials10x",
		Overlaps: []string{"ChestAndLootMaterials10x"},
	}
	blurb := effectBlurb(overlap)
	require.Contains(t, blurb, "edits the same values as ChestAndLootMaterials10x")
	require.Contains(t, blurb, "multiply in build order")
	require.Contains(t, blurb, "information, not a fault")
	require.Equal(t, StatusWarn, effectStatus(overlap))
}

// The re-check button is a real operation, and it is named in Actions() so the
// parity guard can see the CLI's `audit` has a GUI affordance.
func TestTheAuditOperationIsClaimedInActions(t *testing.T) {
	require.Contains(t, Actions(), "audit")
	block := auditUI(t, flaggedAudit()).auditBlock()
	require.NotNil(t, buttonNamed(t, block, "Re-check amounts"))
}

// buttonNamed finds a button by its label, so a test can drive the one it means
// without knowing the layout.
func buttonNamed(t *testing.T, o fyne.CanvasObject, label string) *widget.Button {
	t.Helper()
	var found *widget.Button
	walk(o, func(obj fyne.CanvasObject) bool {
		if b, ok := obj.(*widget.Button); ok && b.Text == label {
			found = b
			return true
		}
		return false
	})
	require.NotNilf(t, found, "no button labelled %q; the block holds:\n%s",
		label, strings.Join(strings.Split(cardText(o), "\n"), " | "))
	return found
}

func tapButton(t *testing.T, o fyne.CanvasObject, label string) {
	t.Helper()
	test.Tap(buttonNamed(t, o, label))
}
