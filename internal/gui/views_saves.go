package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/save"
)

/*
The Saves section (spec 007 R8, spec 008 R4).

The slot list sits on top and everything else is a tab under it: the Editor
for the selected save, Raw JSON for export and import, Backups for what has
been copied. Tabs rather than a column of cards so that a later inventory or
ship view has a place to go without the section growing downward. The editor
writes into the game's save folder, which nothing else in this window does, so
every write goes through a confirmation that names the backup it will take
first and refuses while the game is running unless the box that says "unsafe"
is ticked.
*/

// loadSlots fills the slot listing.
func (u *ui) loadSlots() {
	if u.slotsOK {
		return
	}
	u.slotsOK = true
	go func() {
		done := u.busy("Reading the save slots…")
		defer done()
		res, err := core.ListSaveSlots(context.Background(),
			core.ListSaveSlotsRequest{Request: u.request()})
		fyne.Do(func() {
			u.slots = res
			u.slotsErr = ""
			if err != nil {
				u.slotsErr = err.Error()
			}
			u.refresh()
		})
	}()
}

// buildSaves is the section.
func (u *ui) buildSaves() fyne.CanvasObject {
	u.loadStatus()
	u.loadSlots()
	u.loadSaves()
	if sel := u.reinspect; sel != nil && !u.working() {
		u.reinspect = nil
		u.selectSlot(*sel)
	}

	// The section is its tabs; the slot list is the first of them rather than
	// a card above them, so each tab has the whole height. Each tab scrolls on
	// its own: stacked in a VBox instead, the tabs got the height of whichever
	// was built first and the others were clipped.
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Slots", theme.ListIcon(), container.NewVScroll(u.slotsCard())),
		container.NewTabItemWithIcon("Editor", theme.DocumentCreateIcon(), container.NewVScroll(u.saveEditorTab())),
		container.NewTabItemWithIcon("Raw JSON", theme.FileTextIcon(), container.NewVScroll(u.rawJSONTab())),
		container.NewTabItemWithIcon("Backups", theme.ContentCopyIcon(), container.NewVScroll(u.backupsTab())),
	)
	tabs.SetTabLocation(container.TabLocationTop)
	if u.savesTab >= 0 && u.savesTab < len(tabs.Items) {
		tabs.SelectIndex(u.savesTab)
	}
	tabs.OnSelected = func(item *container.TabItem) {
		for i, it := range tabs.Items {
			if it == item {
				u.savesTab = i
			}
		}
	}

	top := heading("Saves", "The game's save slots, and an editor for the one you pick.")
	// Not wrapped in a scroll of its own: the content pane already scrolls,
	// and gives this at least its own height, which the tabs then fill.
	return container.NewBorder(top, nil, nil, nil, tabs)
}

// The tab indices, for the code that switches tabs.
const (
	savesTabSlots = iota
	savesTabEditor
	savesTabRaw
)

// selectedSaveRow is the one-line header the other tabs carry, naming the
// save the slot list picked.
func (u *ui) selectedSaveRow() fyne.CanvasObject {
	if u.inspect == nil {
		return note("Pick a save in the Slots tab first.", StatusInfo)
	}
	in := u.inspect
	back := widget.NewButtonWithIcon("Slots", theme.ListIcon(), func() {
		u.savesTab = savesTabSlots
		u.refresh()
	})
	return container.NewBorder(nil, nil, nil, back,
		plainRow("Save", fmt.Sprintf("slot %d %s — %s", in.Ref.Slot, in.Ref.Kind, in.File)))
}

// --- the slots (R8.1) -------------------------------------------------------

func (u *ui) slotsCard() fyne.CanvasObject {
	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		u.slotsOK = false
		u.inspect = nil
		u.refresh()
	})
	openProfile := widget.NewButtonWithIcon("Open save folder", theme.FolderOpenIcon(),
		func() { u.openPath(u.slots.Profile) })
	u.gate(refresh, openProfile)
	if u.slots.Profile == "" {
		openProfile.Disable()
	}

	rows := []fyne.CanvasObject{}
	switch {
	case u.slotsErr != "":
		rows = append(rows, note(u.slotsErr, StatusWarn))
	case len(u.slots.Slots) == 0:
		rows = append(rows, note("No saves were found in the profile.", StatusInfo))
	default:
		var t detailTable
		t.header("", "Slot", "Kind", "Name", "Summary", "Played", "Version", "Written")
		t.setWidths(30, 60, 80, 160, 300, 90, 120, 150)
		for _, sl := range u.slots.Slots {
			marker, st := "", StatusInfo
			if sl.Newest {
				marker, st = "*", StatusGood
			}
			summary := sl.Summary
			if sl.MetaError != "" {
				summary, st = sl.MetaError, StatusWarn
			}
			t.row(st, marker, strconv.Itoa(sl.Slot), string(sl.Kind), orNone(sl.Name, "—"),
				orNone(summary, "—"), playTimeText(sl.PlayTime), slotVersionText(sl), slotWrittenText(sl))
		}
		table := t.widget()
		slots := u.slots.Slots
		table.OnSelected = func(id widget.TableCellID) {
			if id.Row < 0 || id.Row >= len(slots) {
				return
			}
			u.selectSlot(core.SlotSelector{Slot: slots[id.Row].Slot, Kind: slots[id.Row].Kind})
		}
		rows = append(rows, fixedHeight(table, 180))
	}
	rows = append(rows,
		plainRow("Profile", orNone(u.slots.Profile, "—")),
		factRow("Key mapping", mappingStatusText(u.slots.Mapping), mappingStatus(u.slots.Mapping)),
		factRow("Game", gameRunningText(u.slots.GameRunning), gameRunningStatus(u.slots.GameRunning)),
		container.NewHBox(refresh, openProfile),
	)
	return card("Slots", rows...)
}

// selectSlot reads one save for the editor.
func (u *ui) selectSlot(sel core.SlotSelector) {
	u.perform(fmt.Sprintf("Reading slot %d %s…", sel.Slot, sel.Kind), func(ctx context.Context) error {
		res, err := core.InspectSave(ctx, core.InspectSaveRequest{Request: u.request(), Slot: sel})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			u.inspect = &res
			u.draft = nil // a different save, a fresh form
			u.rawNode, u.rawText = nil, ""
			u.shipIndex = -1
			if u.savesTab == savesTabSlots {
				u.savesTab = savesTabEditor
			}
			u.refresh()
		})
		return nil
	})
}

// --- the editor tab (R8.2) --------------------------------------------------

// editField is one row of the form: what it edits and what the save holds.
type editField struct {
	label   string
	current string
	// now is shown beside the field: what the save holds, when the number in
	// the field does not say it all.
	now   string
	entry *widget.Entry
	set   func(cs *save.ChangeSet, text string) error
	// widget, when set, replaces the entry: a row that is a control rather
	// than a typed value (the ship picker, the type drop-downs). value, when
	// set with it, reads the control for the change set.
	widget fyne.CanvasObject
	value  func() string
}

// editGroup is a handful of fields with one line saying what they are.
type editGroup struct {
	title, blurb string
	fields       []*editField
}

func (u *ui) saveEditorTab() fyne.CanvasObject {
	if u.inspect == nil {
		return card("Editor", u.selectedSaveRow())
	}
	in := u.inspect
	s := in.Summary

	groups := []editGroup{
		{"Currencies", "Set directly. The game's counters hold up to 4,294,967,295.", []*editField{
			{label: "Units", current: strconv.FormatInt(s.Units, 10),
				set: func(cs *save.ChangeSet, t string) error { return parseUint(t, &cs.Units) }},
			{label: "Nanites", current: strconv.FormatInt(s.Nanites, 10),
				set: func(cs *save.ChangeSet, t string) error { return parseUint(t, &cs.Nanites) }},
			{label: "Quicksilver", current: strconv.FormatInt(s.Quicksilver, 10),
				set: func(cs *save.ChangeSet, t string) error { return parseUint(t, &cs.Quicksilver) }},
		}},
		{"Vitals", "The game clamps anything above its own maximum on load.", []*editField{
			{label: "Health", current: strconv.FormatInt(s.Health, 10),
				set: func(cs *save.ChangeSet, t string) error { return parseInt(t, &cs.Health) }},
			{label: "Shield", current: strconv.FormatInt(s.Shield, 10),
				set: func(cs *save.ChangeSet, t string) error { return parseInt(t, &cs.Shield) }},
		}},
		{"Exosuit slots", fmt.Sprintf("Unlocked slots, up to the game's own ceiling of %d items and %d technology. "+
			"The grid (%d×%d and %d×%d now) grows a row at a time, as it does when you buy slots.",
			save.CeilingItems.Max(), save.CeilingTech.Max(), s.SuitItems.Width, s.SuitItems.Height, s.SuitTech.Width, s.SuitTech.Height),
			[]*editField{
				{label: "Item slots", current: strconv.Itoa(s.SuitItems.Valid),
					now: fmt.Sprintf("of %d, %d occupied", save.CeilingItems.Max(), s.SuitItems.Occupied),
					set: func(cs *save.ChangeSet, t string) error { return parseCount(t, &cs.SuitItemSlots) }},
				{label: "Technology slots", current: strconv.Itoa(s.SuitTech.Valid),
					now: fmt.Sprintf("of %d, %d occupied", save.CeilingTech.Max(), s.SuitTech.Occupied),
					set: func(cs *save.ChangeSet, t string) error { return parseCount(t, &cs.SuitTechSlots) }},
			}},
		{"Standing", fmt.Sprintf("Levels 1 to %d, as the reputation screen shows them. Lowering one puts you back "+
			"under a mission's rank gate; the value written is the lowest that shows as that level.", save.MaxLevel),
			standingFields(s.Standings)},
	}
	groups = append(groups, u.shipGroup(s)...)
	groups = append(groups, freighterGroup(s)...)

	rows := []fyne.CanvasObject{
		u.selectedSaveRow(),
		plainRow("Name", orNone(orNone(in.Meta.Name, s.SaveName), "—")),
		plainRow("Summary", orNone(in.Meta.Summary, "—")),
		plainRow("Version", fmt.Sprintf("%d = base %d, %s · %s", s.Version, s.Base, save.GameModeName(s.GameMode), s.Platform)),
		plainRow("Played", playTimeText(uint64(max(s.PlayTime, 0)))), //nolint:gosec // clamped
		plainRow("Ships / multitools", fmt.Sprintf("%d / %d", s.Ships, s.Multitools)),
	}
	if len(s.Unmapped) > 0 {
		rows = append(rows, factRow("Unmapped keys",
			fmt.Sprintf("%d key(s) the mapping does not name; the game is newer than the mapping", len(s.Unmapped)),
			StatusWarn))
	}

	var fields []*editField
	for _, g := range groups {
		rows = append(rows, widget.NewSeparator(), groupHeading(g.title, g.blurb))
		for _, f := range g.fields {
			fields = append(fields, f)
			rows = append(rows, u.fieldRow(f))
		}
	}

	preview := widget.NewButtonWithIcon("Preview changes", theme.SearchIcon(), func() { u.previewEdit(fields) })
	apply := widget.NewButtonWithIcon("Apply…", theme.DocumentSaveIcon(), func() { u.confirmEdit(fields) })
	apply.Importance = widget.DangerImportance
	u.gate(preview, apply)
	rows = append(rows, widget.NewSeparator(),
		wrapped("Every write copies the whole profile to the backup directory first."),
		container.NewHBox(preview, apply))

	return card(fmt.Sprintf("Editor — slot %d %s", in.Ref.Slot, in.Ref.Kind), rows...)
}

/*
shipGroup is the starship fields: which ship, its class, and its two slot
counts. The ship is a drop-down over ShipOwnership; the chosen index is kept
on the ui so the rebuild an operation causes does not reset it.
*/
func (u *ui) shipGroup(s save.Summary) []editGroup {
	if len(s.ShipList) == 0 {
		return nil
	}
	idx := u.shipIndex
	var ship *save.ShipSummary
	for i := range s.ShipList {
		if s.ShipList[i].Index == idx || (idx < 0 && s.ShipList[i].Primary) {
			ship = &s.ShipList[i]
		}
	}
	if ship == nil {
		ship = &s.ShipList[0]
	}
	chosen := ship.Index
	labels := make([]string, 0, len(s.ShipList))
	for _, sh := range s.ShipList {
		labels = append(labels, sh.Label())
	}
	pick := widget.NewSelect(labels, func(label string) {
		for _, sh := range s.ShipList {
			if sh.Label() == label && sh.Index != u.shipIndex {
				u.shipIndex = sh.Index
				u.refresh()
			}
		}
	})
	pick.SetSelected(ship.Label())
	return []editGroup{{
		title: "Starship",
		blurb: fmt.Sprintf("Unlocked slots, up to the game's ceiling of %d items and %d technology; the grid (%d×%d and %d×%d now) "+
			"grows as it does when you buy slots. The class is the badge and the upgrade gate; it does not change the "+
			"grid, and the stat bonuses rolled with the ship stay as they are. A type change keeps the seed and "+
			"regenerates the ship as that kind.",
			save.CeilingItems.Max(), save.CeilingTech.Max(), ship.Items.Width, ship.Items.Height, ship.Tech.Width, ship.Tech.Height),
		fields: []*editField{
			{label: "Ship", widget: pick},
			typeField("Type", save.ShipTypes, ship.Type, func(cs *save.ChangeSet, key string) {
				cs.Ship = chosen
				cs.ShipType = &key
			}),
			{label: "Class", current: ship.Class, now: "C, B, A or S",
				set: func(cs *save.ChangeSet, t string) error { return parseClass(t, &cs.ShipClass, &cs.Ship, chosen) }},
			{label: "Item slots", current: strconv.Itoa(ship.Items.Valid),
				now: fmt.Sprintf("of %d, %d occupied", save.CeilingItems.Max(), ship.Items.Occupied),
				set: func(cs *save.ChangeSet, t string) error { cs.Ship = chosen; return parseCount(t, &cs.ShipItemSlots) }},
			{label: "Technology slots", current: strconv.Itoa(ship.Tech.Valid),
				now: fmt.Sprintf("of %d, %d occupied", save.CeilingTech.Max(), ship.Tech.Occupied),
				set: func(cs *save.ChangeSet, t string) error { cs.Ship = chosen; return parseCount(t, &cs.ShipTechSlots) }},
		},
	}}
}

// freighterGroup is the freighter's class and slot counts, when one is owned.
func freighterGroup(s save.Summary) []editGroup {
	f := s.Freighter
	if !f.Present {
		return nil
	}
	return []editGroup{{
		title: "Freighter",
		blurb: fmt.Sprintf("Unlocked slots, up to the game's ceiling of %d items and %d technology; the grid (%d×%d and %d×%d now) "+
			"grows as it does when you buy slots.",
			save.CeilingItems.Max(), save.CeilingTech.Max(), f.Items.Width, f.Items.Height, f.Tech.Width, f.Tech.Height),
		fields: []*editField{
			typeField("Type", save.FreighterTypes, f.Type, func(cs *save.ChangeSet, key string) { cs.FreighterType = &key }),
			{label: "Class", current: f.Class, now: "C, B, A or S",
				set: func(cs *save.ChangeSet, t string) error { return parseClass(t, &cs.FreighterClass, nil, 0) }},
			{label: "Item slots", current: strconv.Itoa(f.Items.Valid),
				now: fmt.Sprintf("of %d, %d occupied", save.CeilingItems.Max(), f.Items.Occupied),
				set: func(cs *save.ChangeSet, t string) error { return parseCount(t, &cs.FreighterItemSlots) }},
			{label: "Technology slots", current: strconv.Itoa(f.Tech.Valid),
				now: fmt.Sprintf("of %d, %d occupied", save.CeilingTech.Max(), f.Tech.Occupied),
				set: func(cs *save.ChangeSet, t string) error { return parseCount(t, &cs.FreighterTechSlots) }},
		},
	}}
}

/*
typeField is a drop-down over a type table. The current type is preselected;
a model the table does not know (a corvette, a newer generator) shows as its
own scene name and is left alone unless another entry is picked. The seed is
kept, so a type change regenerates the ship's look from the same seed.
*/
func typeField(label string, table []save.ShipType, current string, apply func(cs *save.ChangeSet, key string)) *editField {
	labels := make([]string, 0, len(table)+1)
	byLabel := map[string]string{}
	for _, t := range table {
		labels = append(labels, t.Label)
		byLabel[t.Label] = t.Key
	}
	currentLabel := current
	for _, t := range table {
		if t.Key == current {
			currentLabel = t.Label
		}
	}
	if current == "" {
		currentLabel = "(not a procedural type)"
		labels = append([]string{currentLabel}, labels...)
	}
	pick := widget.NewSelect(labels, nil)
	pick.SetSelected(currentLabel)
	return &editField{
		label: label, current: current, widget: pick,
		value: func() string { return byLabel[pick.Selected] },
		set: func(cs *save.ChangeSet, key string) error {
			if key == "" {
				return nil
			}
			apply(cs, key)
			return nil
		},
	}
}

func parseClass(text string, into **string, ship *int, index int) error {
	c := strings.ToUpper(strings.TrimSpace(text))
	if !save.ValidClass(c) {
		return fmt.Errorf("%q is not one of C, B, A, S", text)
	}
	*into = &c
	if ship != nil {
		*ship = index
	}
	return nil
}

// standingFields is one row per faction, edited as the level the game shows.
func standingFields(standings []save.StandingValue) []*editField {
	out := make([]*editField, 0, len(standings))
	for _, sv := range standings {
		key := sv.Key
		f := &editField{label: sv.Label, current: strconv.Itoa(sv.Level), now: fmt.Sprintf("value %d", sv.Value)}
		if !sv.Present {
			f.current, f.now = "", "not met yet; the game creates the entry on first contact"
		}
		f.set = func(cs *save.ChangeSet, t string) error {
			v, err := strconv.ParseInt(t, 10, 64)
			if err != nil {
				return fmt.Errorf("%q is not a whole number", t)
			}
			if cs.Standings == nil {
				cs.Standings = map[string]int64{}
			}
			cs.Standings[key] = v
			return nil
		}
		out = append(out, f)
	}
	return out
}

// fieldRow lays out label, entry and the note beside it, with the draft
// preserved across the rebuilds an operation causes.
func (u *ui) fieldRow(f *editField) fyne.CanvasObject {
	if f.widget != nil {
		return container.NewBorder(nil, nil, fixedWidth(lowLabel(f.label), 200), nil, f.widget)
	}
	f.entry = widget.NewEntry()
	f.entry.SetText(f.current)
	if d, ok := u.draft[f.label]; ok {
		f.entry.SetText(d)
	}
	label := f.label
	f.entry.OnChanged = func(text string) {
		if u.draft == nil {
			u.draft = map[string]string{}
		}
		u.draft[label] = text
	}
	return container.NewBorder(nil, nil,
		fixedWidth(lowLabel(f.label), 200),
		fixedWidth(dim(f.now), 320),
		fixedWidth(f.entry, 200))
}

// groupHeading is a group's title with its one line of explanation.
func groupHeading(title, blurb string) fyne.CanvasObject {
	t := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return container.NewVBox(t, wrapped(blurb))
}

// changeSet reads the form into a change set, taking only fields that differ
// from what the save holds.
func changeSet(fields []*editField) (save.ChangeSet, error) {
	cs := save.ChangeSet{Ship: -1}
	for _, f := range fields {
		if f.entry == nil && f.value == nil {
			continue
		}
		var text string
		if f.entry != nil {
			text = strings.TrimSpace(f.entry.Text)
		} else {
			text = f.value()
		}
		if text == f.current || text == "" {
			continue
		}
		if err := f.set(&cs, text); err != nil {
			return cs, fmt.Errorf("%s: %w", f.label, err)
		}
	}
	return cs, nil
}

func parseUint(text string, into **uint64) error {
	v, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return fmt.Errorf("%q is not a whole number", text)
	}
	*into = &v
	return nil
}

func parseInt(text string, into **int64) error {
	v, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("%q is not a whole number", text)
	}
	*into = &v
	return nil
}

func parseCount(text string, into **int) error {
	v, err := strconv.Atoi(text)
	if err != nil {
		return fmt.Errorf("%q is not a whole number", text)
	}
	*into = &v
	return nil
}

// previewEdit runs the change set as a dry run and shows what would change.
func (u *ui) previewEdit(fields []*editField) {
	cs, err := changeSet(fields)
	if err != nil {
		u.flash(err.Error(), StatusWarn)
		return
	}
	if cs.Empty() {
		u.flash("Nothing differs from what the save holds.", StatusInfo)
		return
	}
	sel := core.SlotSelector{Slot: u.inspect.Ref.Slot, Kind: u.inspect.Ref.Kind}
	u.perform("Previewing the edit…", func(ctx context.Context) error {
		res, err := core.EditSave(ctx, core.EditSaveRequest{
			Request: u.request(), Slot: sel, Changes: cs, DryRun: true,
		})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			if len(res.Changes) == 0 {
				u.flash("Every value is already what the save holds.", StatusInfo)
				return
			}
			u.showDetail("Planned changes (nothing written)", changesTable(res.Changes), 800, 360)
		})
		return nil
	})
}

// changesTable lists changes the way the CLI does.
func changesTable(changes []save.Change) fyne.CanvasObject {
	var t detailTable
	t.header("Field", "Old", "New", "Path")
	t.setWidths(180, 140, 140, 320)
	for _, c := range changes {
		t.row(StatusInfo, c.Field, c.Old, c.New, c.Path)
	}
	return fixedHeight(t.widget(), 260)
}

/*
confirmEdit is the confirmation in front of the one write this window makes
into the game's save folder.

It says what will be written, that the whole profile is copied first and where,
and what Steam Cloud will do about it. When the game is running the write is
refused unless the box is ticked; the box says why that is unsafe.
*/
func (u *ui) confirmEdit(fields []*editField) {
	cs, err := changeSet(fields)
	if err != nil {
		u.flash(err.Error(), StatusWarn)
		return
	}
	if cs.Empty() {
		u.flash("Nothing differs from what the save holds.", StatusInfo)
		return
	}
	in := u.inspect
	sel := core.SlotSelector{Slot: in.Ref.Slot, Kind: in.Ref.Kind}
	force := widget.NewCheck("Write even though the game is running (unsafe: its next autosave may overwrite the edit)", nil)
	body := container.NewVBox(
		wrapped(fmt.Sprintf("This rewrites %s and its manifest.", in.File)),
		wrapped("The whole save profile is copied to "+u.status.Paths.SaveBackup+" first, every time. "+
			"To undo, close the game and copy the st_* folder from that backup back into the save folder."),
		wrapped(core.SteamCloudNote),
	)
	if u.slots.GameRunning {
		body.Add(force)
	}
	u.confirmWithBody("Write the edited save?", body, "Write", func() {
		u.perform("Writing the save…", func(ctx context.Context) error {
			res, err := core.EditSave(ctx, core.EditSaveRequest{
				Request: u.request(), Slot: sel, Changes: cs, Force: force.Checked,
			})
			if err != nil {
				return err
			}
			u.wrote(res.Write, len(res.Changes), sel)
			return nil
		})
	}).Show()
}

// wrote reports a finished write and reloads the slots and the editor.
func (u *ui) wrote(w *core.SaveWriteResult, changes int, sel core.SlotSelector) {
	fyne.Do(func() {
		u.slotsOK = false
		u.savesOK = false
		u.draft = nil // written, so the form starts from what the save now holds
		if w == nil {
			u.flash("Every value was already what the save holds; nothing was written.", StatusInfo)
			u.refresh()
			return
		}
		st := StatusGood
		msg := fmt.Sprintf("Wrote %d change(s) to %s. The profile was backed up to %s first. %s",
			changes, filepath.Base(w.DataFile), w.Backup, w.Note)
		if w.Forced {
			st = StatusWarn
			msg += " The game was running: its next autosave may overwrite this."
		}
		u.flash(msg, st)
		// Re-read the save once this operation has let go of the busy
		// indicator; the rebuild that follows picks reinspect up.
		u.reinspect = &sel
	})
}

// --- the raw JSON tab (R5.3, spec 009) ---------------------------------------

/*
rawJSONTab is a browser over the save's JSON with an editor for the node in
view.

A whole save is two megabytes on one line, so the tab never puts all of it in a
text box: a path names a node, its children are listed to descend into, and the
node itself is shown indented with readable keys once it is small enough to
edit. Apply replaces exactly that node, through the same backup-first write as
the editor. Export and Import remain for the whole-file case.
*/
func (u *ui) rawJSONTab() fyne.CanvasObject {
	export := widget.NewButtonWithIcon("Export JSON…", theme.UploadIcon(), func() { u.exportDialog() })
	importBtn := widget.NewButtonWithIcon("Import JSON…", theme.DownloadIcon(), func() { u.importDialog() })
	u.gate(export, importBtn)
	if u.inspect == nil {
		export.Disable()
		importBtn.Disable()
		return card("Raw JSON", u.selectedSaveRow(), container.NewHBox(export, importBtn))
	}

	path := widget.NewEntry()
	path.SetText(u.rawPath)
	path.SetPlaceHolder("BaseContext/PlayerStateData/DifficultyState — empty is the whole save")
	load := widget.NewButtonWithIcon("Load", theme.NavigateNextIcon(), func() { u.loadRawNode(path.Text) })
	path.OnSubmitted = func(p string) { u.loadRawNode(p) }
	up := widget.NewButtonWithIcon("Up", theme.NavigateBackIcon(), func() {
		segs := save.SplitPath(u.rawPath)
		if len(segs) > 0 {
			u.loadRawNode(strings.Join(segs[:len(segs)-1], "/"))
		}
	})
	u.gate(load, up)
	if len(save.SplitPath(u.rawPath)) == 0 {
		up.Disable()
	}

	rows := []fyne.CanvasObject{
		u.selectedSaveRow(),
		wrapped("Type a path and Load, or pick a child to descend. Names or the game's keys both work; " +
			"a number picks an array element. Keys are shown by name; Apply turns them back."),
		container.NewBorder(nil, nil, fixedWidth(lowLabel("Path"), 60), container.NewHBox(up, load), path),
	}

	node := u.rawNode
	if node == nil {
		rows = append(rows, note("Load a path to browse.", StatusInfo))
	} else {
		rows = append(rows, plainRow("Node", fmt.Sprintf("%s · %s · %s", orNone(node.Path, "(whole save)"), node.Type, humanSize(int64(node.Bytes)))))
		if len(node.Children) > 0 {
			var t detailTable
			t.header("Child", "Type", "Size", "Items")
			t.setWidths(320, 90, 100, 80)
			for _, c := range node.Children {
				t.row(StatusInfo, c.Name, c.Type, humanSize(int64(c.Bytes)), strconv.Itoa(c.Len))
			}
			table := t.widget()
			children := node.Children
			base := node.Path
			table.OnSelected = func(id widget.TableCellID) {
				if id.Row < 0 || id.Row >= len(children) {
					return
				}
				next := children[id.Row].Name
				if base != "" {
					next = base + "/" + next
				}
				u.loadRawNode(next)
			}
			rows = append(rows, fixedHeight(table, 240))
		}
		if node.TooLarge {
			rows = append(rows, note(fmt.Sprintf("This node is %s, above the %s the editor box takes; pick a child.",
				humanSize(int64(node.Bytes)), humanSize(core.MaxInlineJSON)), StatusWarn))
		} else {
			rows = append(rows, u.rawEditor())
		}
	}
	rows = append(rows, widget.NewSeparator(),
		wrapped("Export writes the whole save as JSON outside the game folder; Import writes such a file back."),
		container.NewHBox(export, importBtn))
	return card("Raw JSON", rows...)
}

// rawEditor is the text box for the node in view and the buttons that act on it.
func (u *ui) rawEditor() fyne.CanvasObject {
	text := widget.NewMultiLineEntry()
	text.TextStyle = fyne.TextStyle{Monospace: true}
	text.Wrapping = fyne.TextWrapOff
	text.SetText(u.rawText)
	text.OnChanged = func(s string) { u.rawText = s }

	revert := widget.NewButtonWithIcon("Revert", theme.ContentUndoIcon(), func() {
		u.rawText = u.rawNode.JSON
		text.SetText(u.rawText)
	})
	check := widget.NewButtonWithIcon("Check", theme.SearchIcon(), func() { u.applyRawNode(true) })
	apply := widget.NewButtonWithIcon("Apply…", theme.DocumentSaveIcon(), func() { u.confirmRawNode() })
	apply.Importance = widget.DangerImportance
	u.gate(revert, check, apply)
	if len(save.SplitPath(u.rawPath)) == 0 {
		// The whole save is never edited in the box: that is what Import is for.
		apply.Disable()
		check.Disable()
	}
	return container.NewVBox(
		fixedHeight(text, 360),
		container.NewHBox(revert, check, apply),
	)
}

// loadRawNode reads one node for the browser.
func (u *ui) loadRawNode(path string) {
	in := u.inspect
	if in == nil {
		return
	}
	sel := core.SlotSelector{Slot: in.Ref.Slot, Kind: in.Ref.Kind}
	u.perform("Reading "+orNone(path, "the whole save")+"…", func(ctx context.Context) error {
		res, err := core.GetSaveNode(ctx, core.SaveNodeRequest{Request: u.request(), Slot: sel, Path: path})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			u.rawPath = res.Path
			u.rawNode = &res
			u.rawText = res.JSON
			u.savesTab = savesTabRaw
			u.refresh()
		})
		return nil
	})
}

// applyRawNode checks or writes the edited text at the path in view.
func (u *ui) applyRawNode(dryRun bool) {
	in := u.inspect
	if in == nil || u.rawNode == nil {
		return
	}
	sel := core.SlotSelector{Slot: in.Ref.Slot, Kind: in.Ref.Kind}
	path, text := u.rawPath, u.rawText
	what := "Writing "
	if dryRun {
		what = "Checking "
	}
	u.perform(what+path+"…", func(ctx context.Context) error {
		res, err := core.SetSaveNode(ctx, core.SetSaveNodeRequest{
			Request: u.request(), Slot: sel, Path: path, JSON: text, DryRun: dryRun,
		})
		if err != nil {
			return err
		}
		if dryRun {
			fyne.Do(func() {
				if !res.Changed {
					u.flash("Valid JSON, and the same value the save already holds.", StatusInfo)
					return
				}
				u.flash(fmt.Sprintf("Valid: %s would change (%d key(s) turned back). Apply writes it.", res.Path, res.Obfuscated), StatusGood)
			})
			return nil
		}
		u.wrote(res.Write, 1, sel)
		return nil
	})
}

// confirmRawNode is the write confirmation for the browser's editor.
func (u *ui) confirmRawNode() {
	in := u.inspect
	if in == nil || u.rawNode == nil {
		return
	}
	body := container.NewVBox(
		wrapped(fmt.Sprintf("This replaces %s in %s and rewrites the manifest.", u.rawPath, in.File)),
		wrapped("The whole save profile is copied to "+u.status.Paths.SaveBackup+" first, every time."),
		wrapped(core.SteamCloudNote),
	)
	u.confirmWithBody("Write this node into the save?", body, "Write", func() { u.applyRawNode(false) }).Show()
}

// exportDialog writes the save's JSON outside the game folder (R5.3).
func (u *ui) exportDialog() {
	in := u.inspect
	sel := core.SlotSelector{Slot: in.Ref.Slot, Kind: in.Ref.Kind}
	path := widget.NewEntry()
	path.SetText(filepath.Join(u.status.Paths.Workspace, "saves",
		filepath.Base(filepath.Dir(in.File))+"-"+strings.TrimSuffix(filepath.Base(in.File), ".hg")+".json"))
	names := widget.NewCheck("Readable key names (import turns them back)", nil)
	names.Checked = true
	pretty := widget.NewCheck("Indent for reading", nil)
	pretty.Checked = true
	body := container.NewVBox(
		wrapped("Writes the decoded save as JSON. Nothing in the game folder changes."),
		u.withBrowse(path, true),
		names, pretty,
	)
	u.pathDialog("Export save JSON", "Export", body, func() {
		out := strings.TrimSpace(path.Text)
		if fi, err := os.Stat(out); err == nil && fi.IsDir() {
			out = filepath.Join(out, filepath.Base(filepath.Dir(in.File))+"-"+
				strings.TrimSuffix(filepath.Base(in.File), ".hg")+".json")
		}
		u.perform("Exporting the save…", func(ctx context.Context) error {
			res, err := core.ExportSave(ctx, core.ExportSaveRequest{
				Request: u.request(), Slot: sel, Out: out, Pretty: pretty.Checked, Names: names.Checked,
			})
			if err != nil {
				return err
			}
			fyne.Do(func() {
				u.flash(fmt.Sprintf("Exported slot %d %s to %s (%s).", res.Ref.Slot, res.Ref.Kind, res.Out, humanSize(res.Bytes)), StatusGood)
			})
			return nil
		})
	})
}

// importDialog writes a JSON file back as the save (R5.3), through the same
// confirmation as an edit.
func (u *ui) importDialog() {
	in := u.inspect
	sel := core.SlotSelector{Slot: in.Ref.Slot, Kind: in.Ref.Kind}
	u.chooseFile(storage.NewExtensionFileFilter([]string{".json"}), func(file string) {
		force := widget.NewCheck("Write even though the game is running (unsafe: its next autosave may overwrite the edit)", nil)
		body := container.NewVBox(
			wrapped(fmt.Sprintf("This replaces the contents of %s with %s and rewrites the manifest to match.", in.File, file)),
			wrapped("The whole save profile is copied to "+u.status.Paths.SaveBackup+" first, every time."),
			wrapped(core.SteamCloudNote),
		)
		if u.slots.GameRunning {
			body.Add(force)
		}
		u.confirmWithBody("Import into this save?", body, "Import", func() {
			u.perform("Importing the save…", func(ctx context.Context) error {
				res, err := core.ImportSave(ctx, core.ImportSaveRequest{
					Request: u.request(), Slot: sel, In: file, Force: force.Checked,
				})
				if err != nil {
					return err
				}
				w := res.Write
				u.wrote(&w, 1, sel)
				return nil
			})
		}).Show()
	})
}

// --- the backups tab (spec 004 R5.2) ----------------------------------------

func (u *ui) backupsTab() fyne.CanvasObject {
	open := widget.NewButtonWithIcon("Open backup folder", theme.FolderOpenIcon(),
		func() { u.openPath(u.saves.Dir) })
	take := widget.NewButtonWithIcon("Back up now", theme.ContentCopyIcon(),
		func() { u.backupSaves() })
	take.Importance = widget.HighImportance
	u.gate(open, take)
	if !u.status.Install.Found {
		take.Disable()
	}

	rows := []fyne.CanvasObject{}
	if u.savesOK && len(u.saves.Backups) > 0 {
		var t detailTable
		t.header("Taken", "Profiles", "Files", "Size")
		t.setWidths(240, 100, 90, 120)
		for _, b := range u.saves.Backups {
			t.row(StatusInfo, b.Created.Format("2006-01-02 15:04")+" · "+humanAgo(b.Created),
				strconv.Itoa(b.Profiles), strconv.Itoa(b.Files), humanSize(b.Bytes))
		}
		rows = append(rows, fixedHeight(t.widget(), 160))
	} else {
		rows = append(rows, note("No save backups yet. A deploy takes one, and so does every write from the editor.", StatusInfo))
	}
	rows = append(rows,
		plainRow("Backups", u.saves.Dir),
		plainRow("Kept", fmt.Sprintf("the newest %d", core.SaveRetention)),
		wrapped("To restore one: close the game, then copy an st_* folder from a backup back into "+
			"the save folder. The editor is the only part of nmsbonker that writes into that "+
			"folder, and it takes one of these backups before every write."),
		container.NewHBox(take, open),
	)
	return card("Backups", rows...)
}

// --- rendering helpers ------------------------------------------------------

func lowLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Importance = widget.LowImportance
	return l
}

func playTimeText(seconds uint64) string {
	if seconds == 0 {
		return "—"
	}
	return fmt.Sprintf("%dh%02dm", seconds/3600, (seconds%3600)/60)
}

func slotVersionText(sl core.SaveSlotInfo) string {
	if sl.BaseVersion == 0 {
		return "—"
	}
	return fmt.Sprintf("%d %s", sl.BaseVersion, sl.GameMode)
}

func slotWrittenText(sl core.SaveSlotInfo) string {
	when := sl.Modified
	if sl.Timestamp > 0 {
		when = time.Unix(sl.Timestamp, 0)
	}
	return when.Local().Format("2006-01-02 15:04")
}

func mappingStatusText(st core.MappingStatus) string {
	if !st.Present {
		return "absent — " + st.Warning
	}
	return fmt.Sprintf("libMBIN %s, %d keys", st.LibMBINVersion, st.Entries)
}

func mappingStatus(st core.MappingStatus) Status {
	if st.Present {
		return StatusGood
	}
	return StatusWarn
}

func gameRunningText(running bool) string {
	if running {
		return "running — close it before writing a save"
	}
	return "not running"
}

func gameRunningStatus(running bool) Status {
	if running {
		return StatusWarn
	}
	return StatusGood
}
