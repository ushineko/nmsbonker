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
The Saves section (spec 007 R8).

Three cards, top to bottom: the slots the game has, the editor for the one
selected, and the backups. The editor writes into the game's save folder, which
nothing else in this window does, so every write goes through a confirmation
that names the backup it will take first and refuses while the game is running
unless the box that says "unsafe" is ticked.
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

	body := container.NewVBox(
		heading("Saves", "The game's save slots, an editor for currencies and exosuit slots, and the backups."),
		u.slotsCard(),
		widget.NewSeparator(),
		u.saveEditorCard(),
		widget.NewSeparator(),
		u.backupsCard(),
	)
	return container.NewVScroll(body)
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
		rows = append(rows, fixedHeight(table, 220))
	}
	rows = append(rows,
		plainRow("Profile", orNone(u.slots.Profile, "—")),
		factRow("Key mapping", mappingStatusText(u.slots.Mapping), mappingStatus(u.slots.Mapping)),
		factRow("Game", gameRunningText(u.slots.GameRunning), gameRunningStatus(u.slots.GameRunning)),
		container.NewHBox(refresh, openProfile),
	)
	return card("Slots", rows...)
}

// selectSlot reads one save for the editor card.
func (u *ui) selectSlot(sel core.SlotSelector) {
	u.perform(fmt.Sprintf("Reading slot %d %s…", sel.Slot, sel.Kind), func(ctx context.Context) error {
		res, err := core.InspectSave(ctx, core.InspectSaveRequest{Request: u.request(), Slot: sel})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			u.inspect = &res
			u.draft = nil // a different save, a fresh form
			u.refresh()
		})
		return nil
	})
}

// --- the editor (R8.2) ------------------------------------------------------

// editField is one row of the form: what it edits and what the save holds.
type editField struct {
	label, hint string
	current     string
	entry       *widget.Entry
	set         func(cs *save.ChangeSet, text string) error
}

func (u *ui) saveEditorCard() fyne.CanvasObject {
	if u.inspect == nil {
		return card("Editor", note("Select a save in the table above to inspect and edit it.", StatusInfo))
	}
	in := u.inspect
	s := in.Summary

	fields := []*editField{
		{label: "Units", hint: "0 … 4294967295", current: strconv.FormatInt(s.Units, 10),
			set: func(cs *save.ChangeSet, t string) error { return parseUint(t, &cs.Units) }},
		{label: "Nanites", hint: "0 … 4294967295", current: strconv.FormatInt(s.Nanites, 10),
			set: func(cs *save.ChangeSet, t string) error { return parseUint(t, &cs.Nanites) }},
		{label: "Quicksilver", hint: "0 … 4294967295", current: strconv.FormatInt(s.Quicksilver, 10),
			set: func(cs *save.ChangeSet, t string) error { return parseUint(t, &cs.Quicksilver) }},
		{label: "Health", hint: "the game clamps its maximum", current: strconv.FormatInt(s.Health, 10),
			set: func(cs *save.ChangeSet, t string) error { return parseInt(t, &cs.Health) }},
		{label: "Shield", hint: "the game clamps its maximum", current: strconv.FormatInt(s.Shield, 10),
			set: func(cs *save.ChangeSet, t string) error { return parseInt(t, &cs.Shield) }},
		{label: "Suit item slots", hint: fmt.Sprintf("1 … %d (%d×%d grid, %d occupied)", s.SuitItems.Max(), s.SuitItems.Width, s.SuitItems.Height, s.SuitItems.Occupied),
			current: strconv.Itoa(s.SuitItems.Valid),
			set:     func(cs *save.ChangeSet, t string) error { return parseCount(t, &cs.SuitItemSlots) }},
		{label: "Suit technology slots", hint: fmt.Sprintf("1 … %d (%d×%d grid, %d occupied)", s.SuitTech.Max(), s.SuitTech.Width, s.SuitTech.Height, s.SuitTech.Occupied),
			current: strconv.Itoa(s.SuitTech.Valid),
			set:     func(cs *save.ChangeSet, t string) error { return parseCount(t, &cs.SuitTechSlots) }},
	}
	form := []fyne.CanvasObject{}
	for _, f := range fields {
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
		form = append(form, container.NewBorder(nil, nil,
			fixedWidth(lowLabel(f.label), 200),
			fixedWidth(dim(f.hint), 300),
			fixedWidth(f.entry, 200)))
	}

	preview := widget.NewButtonWithIcon("Preview changes", theme.SearchIcon(), func() { u.previewEdit(fields) })
	apply := widget.NewButtonWithIcon("Apply…", theme.DocumentSaveIcon(), func() { u.confirmEdit(fields) })
	apply.Importance = widget.DangerImportance
	export := widget.NewButtonWithIcon("Export JSON…", theme.UploadIcon(), func() { u.exportDialog() })
	importBtn := widget.NewButtonWithIcon("Import JSON…", theme.DownloadIcon(), func() { u.importDialog() })
	u.gate(preview, apply, export, importBtn)

	title := fmt.Sprintf("Editor — slot %d %s", in.Ref.Slot, in.Ref.Kind)
	rows := []fyne.CanvasObject{
		plainRow("File", in.File),
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
	rows = append(rows, widget.NewSeparator())
	rows = append(rows, form...)
	rows = append(rows,
		note("Slot counts unlock cells of the grid the save already has and never grow the grid. "+
			"Every write copies the whole profile to the backup directory first.", StatusInfo),
		container.NewHBox(preview, apply, export, importBtn),
	)
	return card(title, rows...)
}

// changeSet reads the form into a change set, taking only fields that differ
// from what the save holds.
func changeSet(fields []*editField) (save.ChangeSet, error) {
	var cs save.ChangeSet
	for _, f := range fields {
		text := strings.TrimSpace(f.entry.Text)
		if text == f.current {
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
			u.showDetail("Planned changes (nothing written)", changesTable(res.Changes), 760, 360)
		})
		return nil
	})
}

// changesTable lists changes the way the CLI does.
func changesTable(changes []save.Change) fyne.CanvasObject {
	var t detailTable
	t.header("Field", "Old", "New", "Path")
	t.setWidths(180, 120, 120, 320)
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
		u.refresh()
		u.selectSlot(sel)
	})
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

// --- the backups (spec 004 R5.2, moved here from the Overview dialog) -------

func (u *ui) backupsCard() fyne.CanvasObject {
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
			"the save folder. The editor above is the only part of nmsbonker that writes into "+
			"that folder, and it takes one of these backups before every write."),
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
