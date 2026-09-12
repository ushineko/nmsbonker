package gui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/nmsbonker/internal/build/report"
)

// --- Report (R2.4) ---------------------------------------------------------

/*
buildReport is the last build's report.json, rendered as the table the Markdown
file carries.

It is the same data `nmsbonker report` prints, from the same file, and the
legend is the legend the report file uses rather than a second wording of the
five verdicts. That matters more than it sounds: the report is what a user reads
after a game update to decide which mods to re-download, and two different
explanations of WORKING~ would be two different decisions.
*/
func (u *ui) buildReport() fyne.CanvasObject {
	u.loadReport()
	u.loadArchive()

	if !u.lastReportOK {
		return container.NewVScroll(container.NewVBox(
			heading("Report", "Reading the last build report…")))
	}
	r := u.lastReport.Report
	if r == nil {
		return container.NewVScroll(container.NewVBox(
			heading("Report", "No build yet."),
			note("Nothing has been built, so there is nothing to report. A build merges every "+
				"enabled mod into one folder in the workspace and writes the report beside it; "+
				"the game is not touched until you deploy. "+
				trimReason(u.lastReportErr), StatusInfo),
		))
	}

	compat := r.Compatibility
	if r.CompatibilityDetail != "" {
		compat += " (" + r.CompatibilityDetail + ")"
	}
	facts := card("Last build",
		plainRow("Generated", r.Generated.Format("2006-01-02 15:04")+" · "+humanAgo(r.Generated)),
		plainRow("Output folder", r.ModName),
		plainRow("Output directory", r.OutputDir),
		plainRow("Game buildid", orNone(r.GameBuildID, "unknown")),
		plainRow("Compiler", orNone(r.CompilerVersion, "unknown")),
		factRow("Compatibility", compat, compatStatus(r.Compatibility)),
		factRow("MBINs", fmt.Sprintf("%d built, %d dropped", r.Built, r.Dropped),
			builtStatus(r.Dropped)),
		plainRow("Edits", fmt.Sprintf("%d applied, %d skipped", r.Applied, r.Skipped)),
		plainRow("Timings", fmt.Sprintf(
			"%s wall clock; cache %s, merge %s and compile %s summed across %d worker(s)",
			r.Timings.Total.Round(1e6), r.Timings.Cache.Round(1e6),
			r.Timings.Merge.Round(1e6), r.Timings.Compile.Round(1e6), r.Workers)),
	)

	var t detailTable
	t.header("Mod", "Status", "Edits", "Skipped", "Notes")
	t.setWidths(280, 110, 70, 80, 420)
	for _, m := range r.Mods {
		t.row(verdictStatus(m.Verdict), m.Name, m.Verdict,
			strconv.Itoa(m.Applied), strconv.Itoa(m.Skipped), modNote(m))
	}

	// The table first, the facts under it. The table is what the section is
	// for -- it is the list of mods to re-download after a game update -- and
	// with the header facts above it a default-sized window opened on two rows
	// of it and a page of numbers.
	body := container.NewVBox(
		heading("Report", "What the last build made of each mod."),
		fixedHeight(t.widget(), 320),
		note(reportLegend, StatusInfo),
		widget.NewSeparator(),
		facts,
	)

	if len(r.UnsupportedKeys) > 0 {
		body.Add(widget.NewSeparator())
		body.Add(widget.NewLabelWithStyle("Ignored script keys",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		body.Add(note("These directives appear in the scripts and this engine does not "+
			"implement them, so the edits they asked for did not happen: "+
			strings.Join(r.UnsupportedKeys, ", ")+".", StatusWarn))
	}
	if len(r.CacheMisses) > 0 {
		body.Add(widget.NewSeparator())
		body.Add(widget.NewLabelWithStyle("Game files not found",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		body.Add(note("A mod asked to edit a file that is in none of this install's archives, "+
			"usually because a game update moved or removed it: "+
			strings.Join(r.CacheMisses, ", ")+".", StatusWarn))
	}
	if len(r.Degraded) > 0 {
		body.Add(widget.NewSeparator())
		body.Add(widget.NewLabelWithStyle("Files that shipped without their structural edits",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		var d detailTable
		d.header("File", "Mods whose add/remove was skipped")
		d.setWidths(460, 400)
		for _, f := range r.Degraded {
			d.row(StatusWarn, f.Internal, strings.Join(f.Mods, ", "))
		}
		body.Add(fixedHeight(d.widget(), 140))
		body.Add(note("The merged file would not recompile with those entries, so it was built "+
			"again without them. Every value edit is in it; the added or removed entries are "+
			"not.", StatusWarn))
	}

	return container.NewBorder(nil, u.reportActions(), nil, nil, container.NewVScroll(body))
}

// reportActions is the bottom strip: two ways out to a file manager, and the
// deploy that installs exactly what this report describes.
func (u *ui) reportActions() fyne.CanvasObject {
	r := u.lastReport.Report
	openReport := widget.NewButtonWithIcon("Open report folder", theme.FolderOpenIcon(), func() {
		u.openPath(filepath.Dir(u.lastReport.Path))
	})
	openOutput := widget.NewButtonWithIcon("Open output folder", theme.FolderIcon(), func() {
		if r == nil {
			return
		}
		u.openPath(r.OutputDir)
	})
	deploy := widget.NewButtonWithIcon("Deploy…", theme.DownloadIcon(), func() { u.deployLast() })
	deploy.Importance = widget.DangerImportance

	// Roll back sits beside Deploy because it is the same question asked the
	// other way round: this report describes what Deploy would install, and the
	// archive holds what installing it would displace.
	rollBack := widget.NewButtonWithIcon("Roll back…", theme.HistoryIcon(),
		func() { u.showRollback() })
	rollBack.Importance = widget.DangerImportance

	if r == nil {
		openReport.Disable()
		openOutput.Disable()
		deploy.Disable()
	}
	if !u.status.Install.Found {
		deploy.Disable()
		rollBack.Disable()
	}
	if !u.archiveOK || len(u.archive.Entries) == 0 {
		rollBack.Disable()
	}
	u.gate(openReport, openOutput, deploy, rollBack)
	return container.NewVBox(widget.NewSeparator(),
		container.NewHBox(openReport, openOutput, deploy, rollBack))
}

// modNote is the Notes column, worded as the CLI and the Markdown report word
// it. Three renderings of one report must not describe a verdict differently.
func modNote(m report.ModResult) string {
	switch m.Verdict {
	case report.Partial:
		return "structural add/remove skipped; value edits kept"
	case report.WorkingStructural:
		return "adds or removes entries — verify in game"
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
		const maxKeys = 6
		if len(keys) > maxKeys {
			return "keys not found: " + strings.Join(keys[:maxKeys], ", ") + " …"
		}
		return "keys not found: " + strings.Join(keys, ", ")
	case report.NotBuilt:
		return "nothing this mod asked for applied"
	}
	return ""
}

// trimReason turns core's "no build report yet; run `nmsbonker build`" into
// something that does not tell a window user to run a command. The empty state
// has a Build button two clicks away.
func trimReason(reason string) string {
	if reason == "" {
		return ""
	}
	if i := strings.Index(reason, ";"); i > 0 {
		return ""
	}
	return reason
}
