package gui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/table"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/modscript"
)

// --- Mods (R2.2) -----------------------------------------------------------

// The columns, by index, because three closures index into them.
const (
	modColOrder = iota
	modColOn
	modColName
	modColSource
	modColAuthor
	modColTargets
	modColVerdict
)

// modRow is one line of the build order: what the config says, what the script
// says about itself, and what the last build made of it.
type modRow struct {
	order   int
	info    core.ModInfo
	author  string
	targets int
	blocks  int
	verdict string
}

/*
buildMods is the build order, which is also the conflict-resolution rule.

Order is the only control a user has over two mods that change the same value,
so it is the first column and the footer says what it means. The table sorts by
other columns for reading, and says so when it does: a listing sorted by name
under a footer claiming the rows are the build order would be a lie told by the
interface.
*/
func (u *ui) buildMods() fyne.CanvasObject {
	u.loadMods()
	u.loadReport()

	rows := u.modRows()
	selected := -1
	sortCol, sortAsc := modColOrder, true

	cols := []struct {
		title string
		width float32
	}{
		{"#", 50}, {"On", 50}, {"Name", 260}, {"Source", 110}, {"Author", 170},
		{"Files", 70}, {"Last verdict", 160},
	}

	// resort orders the rows for reading. Stable, so re-sorting by a column
	// with ties leaves the previous order inside each tie rather than shuffling
	// rows the user was not asking about.
	resort := func() {
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := rows[i], rows[j]
			less := false
			switch sortCol {
			case modColOrder:
				less = a.order < b.order
			case modColOn:
				less = boolLess(a.info.Enabled, b.info.Enabled)
			case modColName:
				less = strings.ToLower(a.info.Name) < strings.ToLower(b.info.Name)
			case modColSource:
				less = a.info.Source < b.info.Source
			case modColAuthor:
				less = strings.ToLower(a.author) < strings.ToLower(b.author)
			case modColTargets:
				less = a.targets < b.targets
			case modColVerdict:
				less = verdictStatus(a.verdict) < verdictStatus(b.verdict)
			}
			if sortAsc {
				return less
			}
			return !less
		})
	}
	resort()

	table := widget.NewTable(
		func() (int, int) { return len(rows), len(cols) },
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		func(id widget.TableCellID, o fyne.CanvasObject) {
			l := o.(*widget.Label)
			if id.Row < 0 || id.Row >= len(rows) {
				l.SetText("")
				l.Importance = widget.MediumImportance
				return
			}
			text, importance := modCell(rows[id.Row], id.Col)
			// Importance first, text second. SetText refreshes the label, and
			// a refresh is when the importance is turned into a colour -- so
			// assigning it afterwards paints this cell in the colour of
			// whatever row the recycled widget held last. In a scrolling table
			// that shows up as verdict colours landing on file counts.
			l.Importance = importance
			l.SetText(text)
		},
	)
	for i, c := range cols {
		table.SetColumnWidth(i, c.width)
	}

	// Row actions are disabled until a row is selected: a destructive action
	// must not be the thing a stray Enter key reaches.
	enable := widget.NewButtonWithIcon("Enable", theme.ConfirmIcon(), nil)
	disable := widget.NewButtonWithIcon("Disable", theme.CancelIcon(), nil)
	up := widget.NewButtonWithIcon("Move up", theme.MoveUpIcon(), nil)
	down := widget.NewButtonWithIcon("Move down", theme.MoveDownIcon(), nil)
	details := widget.NewButtonWithIcon("Details…", theme.InfoIcon(), nil)
	open := widget.NewButtonWithIcon("Open script", theme.DocumentIcon(), nil)
	edit := widget.NewButtonWithIcon("Edit script…", theme.DocumentCreateIcon(), nil)
	remove := widget.NewButtonWithIcon("Remove…", theme.DeleteIcon(), nil)
	remove.Importance = widget.DangerImportance
	rowActions := []*widget.Button{enable, disable, up, down, details, open, edit, remove}
	for _, b := range rowActions {
		b.Disable()
	}

	// Move only makes sense while the table is showing the build order. Sorted
	// by name, "up" would move a mod to a position the user cannot see.
	inBuildOrder := func() bool { return sortCol == modColOrder && sortAsc }

	armRow := func(i int) {
		selected = i
		for _, b := range rowActions {
			b.Enable()
		}
		if !inBuildOrder() {
			up.Disable()
			down.Disable()
		}
		if rows[i].info.Status == core.ModMissing {
			open.Disable()
			edit.Disable()
			details.Disable()
		}
		if rows[i].info.Source == core.SourceBuiltin && !rows[i].info.Shadowed {
			// A built-in has no file in the library to open, and no entry to
			// remove: it is compiled in. Its parameters live in Tweaks.
			open.Disable()
			remove.Disable()
		}
		u.gate(rowActions...)
	}
	clearRow := func() {
		selected = -1
		table.UnselectAll()
		for _, b := range rowActions {
			b.Disable()
		}
	}

	table.ShowHeaderRow = true
	// Headers are buttons so a click can sort. A plain label cannot receive the
	// tap, and the arrow has to live in the header text because Fyne gives a
	// header cell one object.
	table.CreateHeader = func() fyne.CanvasObject {
		b := widget.NewButton("", nil)
		b.Importance = widget.LowImportance
		b.Alignment = widget.ButtonAlignLeading
		return b
	}
	table.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		b := o.(*widget.Button)
		if id.Col < 0 || id.Col >= len(cols) {
			b.SetText("")
			b.OnTapped = nil
			return
		}
		title := cols[id.Col].title
		if id.Col == sortCol {
			if sortAsc {
				title += " ↑"
			} else {
				title += " ↓"
			}
		}
		b.SetText(title)
		col := id.Col
		b.OnTapped = func() {
			if sortCol == col {
				sortAsc = !sortAsc
			} else {
				sortCol, sortAsc = col, true
			}
			resort()
			// The selection indexed into the old order, so it no longer names
			// the row the user picked. Clearing it is the honest response:
			// carrying it over would leave Remove armed against a different
			// mod than the one highlighted.
			clearRow()
			table.Refresh()
			u.refresh()
		}
	}

	table.OnSelected = func(id widget.TableCellID) {
		if id.Row < 0 || id.Row >= len(rows) {
			return
		}
		// Tapping the On cell toggles the row there and then. The column is two
		// characters wide and does nothing else, so there is no other thing a
		// tap in it could have meant.
		if id.Col == modColOn {
			u.setModEnabled([]string{rows[id.Row].info.Name}, !rows[id.Row].info.Enabled)
			return
		}
		armRow(id.Row)
	}

	enable.OnTapped = func() { u.setModEnabled([]string{rows[selected].info.Name}, true) }
	disable.OnTapped = func() { u.setModEnabled([]string{rows[selected].info.Name}, false) }
	up.OnTapped = func() { u.moveMod(rows[selected], -1) }
	down.OnTapped = func() { u.moveMod(rows[selected], +1) }
	details.OnTapped = func() { u.showModDetails(rows[selected]) }
	open.OnTapped = func() { u.openPath(rows[selected].info.Path) }
	edit.OnTapped = func() { u.editScriptDialog(rows[selected].info) }
	remove.OnTapped = func() { u.removeModDialog(rows[selected].info) }

	add := widget.NewButtonWithIcon("Add…", theme.ContentAddIcon(), func() { u.addModDialog() })
	imp := widget.NewButtonWithIcon("Import folder…", theme.FolderOpenIcon(), func() { u.importModsDialog() })
	check := widget.NewButtonWithIcon("Check", theme.SearchIcon(), func() { u.checkMods() })
	openLib := widget.NewButtonWithIcon("Open library folder", theme.FolderIcon(),
		func() { u.openPath(u.mods.LibraryDir) })
	u.gate(add, imp, check, openLib)

	toolbar := container.NewHBox(add, imp, check, openLib)

	head := widgets.Heading("Mods", "Every script in the library, in the order they are applied.")
	if u.modsOK && len(rows) == 0 {
		head = widgets.Heading("Mods",
			"The library is empty. Add a .lua mod script, or import a folder of them; "+
				"they are copied into the library and start out enabled.")
	}

	top := container.NewVBox(head, toolbar)
	bottom := container.NewVBox(
		u.modsFooter(rows, inBuildOrder()),
		widget.NewSeparator(),
		container.NewHBox(enable, disable, up, down, details, open, remove),
	)
	return container.NewBorder(top, bottom, nil, nil, widgets.FixedHeight(table, 380))
}

// modsFooter states what the order means, and stops claiming it when the table
// is sorted by something else.
func (u *ui) modsFooter(rows []modRow, buildOrder bool) fyne.CanvasObject {
	enabled := 0
	for _, r := range rows {
		if r.info.Enabled {
			enabled++
		}
	}
	text := fmt.Sprintf("%d of %d enabled · build order is table order; lower rows apply "+
		"later and win on conflicts.", enabled, len(rows))
	if !buildOrder {
		text = fmt.Sprintf("%d of %d enabled · sorted for reading, so these rows are not the "+
			"build order — the # column is. Sort by # to move mods.", enabled, len(rows))
	}
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	return l
}

// modRows joins what the three sources know about each mod: the config's order
// and enabled flag, the script's own header, and the last build's verdict.
func (u *ui) modRows() []modRow {
	verdicts := map[string]string{}
	if u.lastReport.Report != nil {
		for _, m := range u.lastReport.Report.Mods {
			verdicts[m.Name] = m.Verdict
		}
	}
	out := make([]modRow, 0, len(u.mods.Mods))
	for i, m := range u.mods.Mods {
		r := modRow{order: i + 1, info: m, verdict: verdicts[m.Name]}
		if c, ok := u.checks[m.Name]; ok {
			r.author, r.targets, r.blocks = c.Author, len(c.Targets), c.Blocks
		}
		out = append(out, r)
	}
	return out
}

/*
modCell is one cell of the mod table: its text and how to rank it.

Split out of the table's update callback so a headless test can assert what an
enabled, a disabled and a missing mod actually render as. That is worth pinning:
the three states differ by one glyph and one colour, and a mod whose .lua has
gone missing looking identical to one that is merely switched off is the kind of
bug that is only found by building against it.
*/
func modCell(r modRow, col int) (string, widget.Importance) {
	switch col {
	case modColOrder:
		return strconv.Itoa(r.order), widget.LowImportance
	case modColOn:
		// A glyph rather than a checkbox: a per-cell widget in a Fyne table is
		// a widget rebuilt on every scroll, and tapping the cell already
		// toggles the row.
		if r.info.Enabled {
			return modOnGlyph, widget.SuccessImportance
		}
		return modOffGlyph, widget.LowImportance
	case modColName:
		if r.info.Status == core.ModMissing {
			return r.info.Name, widget.WarningImportance
		}
		return r.info.Name, widget.MediumImportance
	case modColSource:
		if r.info.Shadowed {
			// The library holds a script of this name and the built-in is what
			// builds. Saying "built-in" alone would leave the user looking at a
			// file in their library that has no effect and no explanation.
			return modShadowedText, widget.WarningImportance
		}
		return r.info.Source, widget.LowImportance
	case modColAuthor:
		return widgets.OrNone(r.author, "—"), widget.LowImportance
	case modColTargets:
		if r.info.Status == core.ModMissing {
			return "missing", widget.WarningImportance
		}
		return targetsText(r), widget.LowImportance
	case modColVerdict:
		if r.verdict == "" {
			return "—", widget.LowImportance
		}
		return r.verdict, widgets.ImportanceFor(verdictStatus(r.verdict))
	}
	return "", widget.MediumImportance
}

// The two glyphs the On column is made of, named so a test asserts against the
// same characters the table draws.
const (
	modOnGlyph  = "✓"
	modOffGlyph = "–"
)

// modShadowedText is the Source cell for a built-in whose name a library script
// also carries.
const modShadowedText = "built-in *"

// targetsText is the Files column: how many game files the script edits, and
// how many edit blocks it carries.
func targetsText(r modRow) string {
	if r.info.Status == core.ModMissing {
		return "missing"
	}
	if r.targets == 0 && r.blocks == 0 {
		return "—"
	}
	return strconv.Itoa(r.targets)
}

func boolLess(a, b bool) bool { return !a && b }

// --- mod operations --------------------------------------------------------

// luaFilter limits the file chooser to mod scripts. A library that has
// collected a stray .txt is a mod that will not load, reported an hour later by
// a build; refusing it at the chooser is cheaper for everyone.
func luaFilter() storage.FileFilter {
	return storage.NewExtensionFileFilter([]string{".lua"})
}

// addModDialog copies one script into the library.
//
// One at a time because Fyne 2.8.1 has no multi-select file chooser; core.AddMod
// takes a list, which is what makes `mods add a.lua b.lua` and this the same
// operation, and Import folder… is the answer for a directory of them.
func (u *ui) addModDialog() {
	dialogs.ChooseFile(u.win, "", luaFilter(), func(path string) {
		u.perform("Adding "+path+"…", func(ctx context.Context) error {
			res, err := core.AddMod(ctx, core.AddModRequest{
				Request: u.request(), Paths: []string{path}, Enabled: true,
			})
			if err != nil {
				return err
			}
			u.ok(fmt.Sprintf("Added %s to the library, enabled and last in the build order. "+
				"The file you chose is untouched.", strings.Join(res.Added, ", ")))
			return nil
		})
	})
}

// importModsDialog copies a directory of scripts in, in name order.
func (u *ui) importModsDialog() {
	dialogs.ChooseFolder(u.win, "", func(dir string) {
		u.perform("Importing "+dir+"…", func(ctx context.Context) error {
			res, err := core.ImportDir(ctx, core.ImportDirRequest{
				Request: u.request(), Dir: dir, Enabled: true,
			})
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("Imported %d script(s) from %s, enabled, in name order. "+
				"The originals are untouched.", len(res.Added), dir)
			if len(res.Replaced) > 0 {
				msg += fmt.Sprintf(" %d already in the library were left as they were.",
					len(res.Replaced))
			}
			u.ok(msg)
			return nil
		})
	})
}

// removeModDialog takes a mod out of the build order, and optionally deletes it.
func (u *ui) removeModDialog(m core.ModInfo) {
	alsoDelete := widget.NewCheck("Also delete "+m.Name+".lua from the library", nil)
	body := container.NewVBox(
		widgets.Wrapped("Removing "+m.Name+" takes it out of the build order. The other mods keep "+
			"their positions, and the mod folder already installed in the game is not "+
			"touched until the next deploy."),
		alsoDelete,
		widgets.Wrapped("Deleting the file cannot be undone from here. If it came from Nexus, "+
			"downloading it again is the only way back."),
	)
	d := dialogs.ConfirmWithBody(u.win, "Remove "+m.Name+"?", body, "Remove", func() {
		del := alsoDelete.Checked
		u.perform("Removing "+m.Name+"…", func(ctx context.Context) error {
			res, err := core.RemoveMod(ctx, core.RemoveModRequest{
				Request: u.request(), Name: m.Name, DeleteFile: del,
			})
			if err != nil {
				return err
			}
			msg := "Removed " + res.Name + " from the build order."
			if res.Deleted != "" {
				msg += " Deleted " + res.Deleted + "."
			}
			u.ok(msg)
			return nil
		})
	})
	d.Show()
}

// setModEnabled flips one or more mods.
func (u *ui) setModEnabled(names []string, enabled bool) {
	verb := "Enabling"
	if !enabled {
		verb = "Disabling"
	}
	u.perform(verb+" "+strings.Join(names, ", ")+"…", func(ctx context.Context) error {
		res, err := core.SetModEnabled(ctx, core.SetModEnabledRequest{
			Request: u.request(), Names: names, Enabled: enabled,
		})
		if err != nil {
			return err
		}
		state := "enabled"
		if !enabled {
			state = "disabled"
		}
		u.ok(strings.Join(res.Changed, ", ") + " " + state + ".")
		return nil
	})
}

// moveMod shifts a mod one place in the build order.
func (u *ui) moveMod(r modRow, delta int) {
	to := r.order + delta
	if to < 1 || to > len(u.mods.Mods) {
		u.flash(r.info.Name+" is already at the "+edgeName(delta)+" of the build order.", fd.StatusWarn)
		return
	}
	u.perform("Moving "+r.info.Name+"…", func(ctx context.Context) error {
		res, err := core.MoveMod(ctx, core.MoveModRequest{
			Request: u.request(), Name: r.info.Name, To: to,
		})
		if err != nil {
			return err
		}
		u.ok(fmt.Sprintf("Moved %s from %d to %d. Lower rows apply later and win on conflicts.",
			res.Name, res.From, res.To))
		return nil
	})
}

func edgeName(delta int) string {
	if delta < 0 {
		return "top"
	}
	return "bottom"
}

/*
checkMods loads every script through the sandbox and reports what it found.

This is the fastest way to find out that a mod written for an older game version
no longer parses, and it is where a script that tries to reach outside the
sandbox is rejected by name. It compiles nothing and reads no game files, so it
costs a hundredth of a second on a library of twenty-seven.
*/
func (u *ui) checkMods() {
	u.perform("Checking the mod scripts…", func(ctx context.Context) error {
		res, err := core.CheckMods(ctx, core.CheckModsRequest{Request: u.request(), All: true})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			u.setChecks(res)
			u.refresh()
			u.showCheckResults(res)
		})
		return nil
	})
}

// setChecks records the per-script facts the table's Author and Files columns
// come from. Called on the UI thread.
func (u *ui) setChecks(res core.CheckModsResult) {
	u.checks = make(map[string]core.ModCheck, len(res.Mods))
	for _, m := range res.Mods {
		u.checks[m.Name] = m
	}
	u.checksOK = true
}

// showCheckResults puts the per-script problems on screen (R2.2).
func (u *ui) showCheckResults(res core.CheckModsResult) {
	t := table.New()
	t.Header("Mod", "Files", "Edits", "Note")
	t.SetWidths(280, 60, 60, 460)
	for _, m := range res.Mods {
		st, note := fd.StatusGood, ""
		switch {
		case !m.OK:
			st, note = fd.StatusBad, m.Error
		case len(m.Unsupported) > 0:
			st = fd.StatusWarn
			note = "ignored keys: " + strings.Join(m.Unsupported, ", ")
		}
		t.Row(st, m.Name, strconv.Itoa(len(m.Targets)), strconv.Itoa(m.Blocks), note)
	}

	summary := fmt.Sprintf("%d loaded, %d failed.", res.OK, res.Failed)
	st := fd.StatusGood
	if res.Failed > 0 {
		st = fd.StatusBad
		summary += " A mod that does not load contributes nothing to a build; the others still do."
	}
	body := container.NewBorder(
		container.NewVBox(
			container.NewHBox(widgets.Marker(st), widgets.StatusText(summary, st)),
			widgets.Wrapped("Ignored keys are script directives this engine does not implement. The "+
				"mod still builds; the edits those keys asked for do not happen."),
		), nil, nil, nil, widgets.FixedHeight(t.Widget(), 360))
	dialogs.ShowDetail(u.win, "Mod scripts", body, 900, 560)
}

/*
showModDetails is what one script says about itself, and its parameters (R2.2).

The parameters are the reason this dialog exists. A library script downloaded
from Nexus keeps its tuning constants at the top of the file the same way the
built-ins do, so they can be offered here too -- but undeclared, which means no
label beyond the global's own name and no range, because guessing a maximum for
somebody else's constant would be inventing a fact. A numeric field, therefore,
and no slider.
*/
func (u *ui) showModDetails(r modRow) {
	check, haveCheck := u.checks[r.info.Name]

	facts := container.NewVBox(
		widgets.PlainRow("Source", modSourceText(r.info)),
		widgets.PlainRow("Author", widgets.OrNone(check.Author, "—")),
		widgets.PlainRow("Pak name", widgets.OrNone(check.ModFilename, "—")),
		widgets.PlainRow("Written for", widgets.OrNone(check.NMSVersion, "—")),
		widgets.PlainRow("Position", fmt.Sprintf("%d in the build order", r.order)),
		widgets.FactRow("Enabled", yesNoText(r.info.Enabled), enabledRowStatus(r.info.Enabled)),
	)
	if r.info.Path != "" {
		facts.Add(widgets.PlainRow("File", r.info.Path))
	}
	// What the last build made of it, and the one-line reading of that (R4.1).
	if check.Verdict != "" {
		facts.Add(widgets.FactRow("Last build", fmt.Sprintf("%s — %d edit(s) applied, %d skipped",
			check.Verdict, check.Applied, check.Skipped), verdictStatus(check.Verdict)))
	}
	if check.Effect != "" {
		facts.Add(widgets.Note(effectBlurb(check), effectStatus(check)))
	}
	if len(check.NotFound) > 0 {
		facts.Add(widgets.Note("Keys the last build could not find, usually because a game update "+
			"renamed or removed them: "+strings.Join(check.NotFound, ", ")+".", fd.StatusWarn))
	}
	if r.info.Shadowed {
		facts.Add(widgets.Note("A library script of this name is present and is ignored: the built-in "+
			"is what builds. Remove takes the library copy out; the built-in stays.",
			fd.StatusWarn))
	}
	if haveCheck && !check.OK {
		facts.Add(widgets.Note("This script does not load: "+check.Error+
			". It contributes nothing to a build until that is fixed.", fd.StatusBad))
	}
	if len(check.Unsupported) > 0 {
		facts.Add(widgets.Note("Directives this engine ignores, so the edits they ask for do not "+
			"happen: "+strings.Join(check.Unsupported, ", ")+".", fd.StatusWarn))
	}
	if len(check.Duplicates) > 0 {
		facts.Add(widgets.Note("These parameters are assigned more than once at the top of the "+
			"script, so a value set here would be overwritten by the script itself and "+
			"quietly ignored: "+strings.Join(check.Duplicates, ", ")+".", fd.StatusWarn))
	}

	body := container.NewVBox(facts)
	if len(check.Targets) > 0 {
		body.Add(widget.NewSeparator())
		body.Add(widget.NewLabelWithStyle(
			fmt.Sprintf("Edits %d game file(s), %d edit block(s)", len(check.Targets), check.Blocks),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		files := widget.NewLabel(strings.Join(check.Targets, "\n"))
		files.Importance = widget.LowImportance
		body.Add(widgets.FixedHeight(container.NewVScroll(files), 120))
	}
	if len(check.Params) > 0 {
		body.Add(widget.NewSeparator())
		body.Add(widget.NewLabelWithStyle("Parameters",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		if r.info.Source == core.SourceBuiltin {
			body.Add(widgets.Note("These are also in the Tweaks section, with a slider each.",
				fd.StatusInfo))
		} else {
			body.Add(widgets.Note("Numbers this script declares at the top of the file. It says "+
				"nothing about what they may be set to, so there is no range here — the "+
				"script itself, and whoever wrote it, is the documentation.", fd.StatusInfo))
		}
		for _, p := range check.Params {
			body.Add(u.libraryParamRow(r.info.Name, p))
		}
	}

	dialogs.ShowDetail(u.win, r.info.Name, container.NewVScroll(body), 820, 620)
}

/*
effectBlurb turns the mechanical effectiveness note into a sentence (R4.1).

The note itself is a phrase -- "no effective edits", "overlaps built-in X" --
because it has to fit a CLI table cell. Here there is room to say what follows
from it, and what follows is the part a user can act on.
*/
func effectBlurb(c core.ModCheck) string {
	out := "From the last build: " + c.Effect + "."
	switch {
	case strings.Contains(c.Effect, core.EffectNoEdits):
		out += " Nothing this script asks for reached the merged files, so it changes nothing " +
			"in game. Usually it was written for an older version of the game."
	case strings.Contains(c.Effect, core.EffectMostlyFailing):
		out += " More of its edits failed to find their target than landed, which is what a " +
			"script written for an older version of the game looks like."
	}
	if len(c.Overlaps) > 0 {
		out += " It edits the same values as " + strings.Join(c.Overlaps, ", ") +
			", so the two multiply in build order — which is how a reward ends up far " +
			"larger than either one asked for. This is information, not a fault: check " +
			"the Report section's amount audit to see whether it matters here."
	}
	return out
}

// effectStatus ranks the note. A script that applied nothing is a real problem;
// an overlap on its own is a fact about the library.
func effectStatus(c core.ModCheck) fd.Status {
	if strings.Contains(c.Effect, core.EffectNoEdits) {
		return fd.StatusBad
	}
	return fd.StatusWarn
}

// libraryParamRow is one undeclared parameter: a field and a Reset, no slider.
func (u *ui) libraryParamRow(mod string, p modscript.Param) fyne.CanvasObject {
	label := widget.NewLabel(p.Name)
	label.Importance = widget.LowImportance

	entry := widget.NewEntry()
	entry.SetText(modscript.FormatValue(p.Current, p.Kind))
	entry.OnSubmitted = func(text string) {
		v, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			u.flash(p.Name+": "+strconv.Quote(text)+" is not a number.", fd.StatusWarn)
			entry.SetText(modscript.FormatValue(p.Current, p.Kind))
			return
		}
		u.perform("Setting "+mod+" "+p.Name+"…", func(ctx context.Context) error {
			res, err := core.SetTweakParam(ctx, core.SetTweakParamRequest{
				Request: u.request(), Name: mod, Param: p.Name, Value: v,
			})
			if err != nil {
				return err
			}
			fyne.Do(func() {
				entry.SetText(modscript.FormatValue(res.New, p.Kind))
				u.flash(fmt.Sprintf("%s %s is %s for the next build. The script on disk is "+
					"unchanged.", mod, p.Name, modscript.FormatValue(res.New, p.Kind)),
					fd.StatusGood)
			})
			return nil
		})
	}

	reset := widget.NewButtonWithIcon("Reset", theme.ContentUndoIcon(), func() {
		u.perform("Resetting "+mod+" "+p.Name+"…", func(ctx context.Context) error {
			if _, err := core.ResetTweak(ctx, core.ResetTweakRequest{
				Request: u.request(), Name: mod, Param: p.Name,
			}); err != nil {
				return err
			}
			fyne.Do(func() { entry.SetText(modscript.FormatValue(p.Default, p.Kind)) })
			return nil
		})
	})

	return container.NewBorder(nil, nil, widgets.FixedWidth(label, 220),
		container.NewHBox(widgets.FixedWidth(widgets.Dim("script value "+
			modscript.FormatValue(p.Default, p.Kind)), 190), reset), entry)
}

// modSourceText spells out where a mod's script comes from.
func modSourceText(m core.ModInfo) string {
	if m.Source == core.SourceBuiltin {
		if m.Shadowed {
			return "built in to nmsbonker (a library script of this name is ignored)"
		}
		return "built in to nmsbonker"
	}
	return "your library"
}

func yesNoText(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func enabledRowStatus(enabled bool) fd.Status {
	if enabled {
		return fd.StatusGood
	}
	return fd.StatusInfo
}

// --- editing a script in place (spec 010) ------------------------------------

/*
editScriptDialog is a text editor over one library script.

Check loads the text through the sandbox and says whether it would build; Save
writes it, keeping the previous text as a .bak beside the file, and refuses text
that does not load unless the box is ticked. A built-in tweak opens read-only:
it is compiled in, and its numbers are parameters in Tweaks.
*/
func (u *ui) editScriptDialog(m core.ModInfo) {
	u.perform("Reading "+m.Name+"…", func(ctx context.Context) error {
		res, err := core.ReadModScript(ctx, core.ModScriptRequest{Request: u.request(), Name: m.Name})
		if err != nil {
			return err
		}
		fyne.Do(func() { u.showScriptEditor(res) })
		return nil
	})
}

func (u *ui) showScriptEditor(script core.ModScriptResult) {
	text := widget.NewMultiLineEntry()
	text.TextStyle = fyne.TextStyle{Monospace: true}
	text.Wrapping = fyne.TextWrapOff
	text.SetText(script.Text)

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	force := widget.NewCheck("Save even if it does not load (the build will report the mod NOT BUILT)", nil)

	var d *dialog.CustomDialog
	revert := widget.NewButtonWithIcon("Revert", theme.ContentUndoIcon(), func() {
		text.SetText(script.Text)
		status.SetText("")
	})
	check := widget.NewButtonWithIcon("Check", theme.SearchIcon(), func() {
		u.perform("Checking "+script.Name+"…", func(ctx context.Context) error {
			res, err := core.WriteModScript(ctx, core.WriteModScriptRequest{
				Request: u.request(), Name: script.Name, Text: text.Text, Check: true,
			})
			if err != nil {
				return err
			}
			fyne.Do(func() { status.SetText(loadsText(res)) })
			return nil
		})
	})
	save := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		u.perform("Writing "+script.Name+"…", func(ctx context.Context) error {
			res, err := core.WriteModScript(ctx, core.WriteModScriptRequest{
				Request: u.request(), Name: script.Name, Text: text.Text, Force: force.Checked,
			})
			if err != nil {
				// Shown in the dialog rather than as a banner behind it: the
				// refusal names the line, and the editor is where to fix it.
				fyne.Do(func() { status.SetText(err.Error()) })
				return nil //nolint:nilerr // reported in the dialog, deliberately
			}
			fyne.Do(func() {
				if !res.Written {
					status.SetText("Nothing changed; the file already holds this text.")
					return
				}
				d.Hide()
				u.checksOK = false
				u.flash(fmt.Sprintf("Wrote %s (%s). The previous text is kept as %s.",
					res.Path, loadsText(res), filepath.Base(res.Backup)), fd.StatusGood)
				u.invalidate()
			})
			return nil
		})
	})
	save.Importance = widget.HighImportance
	closeBtn := widget.NewButton("Close", func() { d.Hide() })

	head := container.NewVBox(widgets.PlainRow("Script", widgets.OrNone(script.Path, "compiled into nmsbonker")))
	buttons := []fyne.CanvasObject{closeBtn, revert, check}
	if script.ReadOnly {
		head.Add(widgets.Note("A built-in tweak is compiled in and cannot be edited here; its numbers are "+
			"parameters in Tweaks. This is the text as shipped.", fd.StatusInfo))
		text.Disable()
	} else {
		head.Add(widgets.Wrapped("Check loads the text through the sandbox the build uses. Save keeps the previous " +
			"text beside the file as .bak and takes effect on the next build."))
		buttons = append(buttons, save)
	}
	body := container.NewBorder(head, container.NewVBox(status, force), nil, nil, text)
	if script.ReadOnly {
		body = container.NewBorder(head, status, nil, nil, text)
	}
	d = dialog.NewCustomWithoutButtons("Edit "+script.Name, body, u.win)
	d.SetButtons(buttons)
	d.Resize(fyne.NewSize(1000, 700))
	d.Show()
}

// loadsText is the sandbox's verdict in a sentence.
func loadsText(res core.WriteModScriptResult) string {
	if res.Loads {
		return fmt.Sprintf("loads: %d change block(s)", res.Blocks)
	}
	return "does not load: " + res.LoadError
}
