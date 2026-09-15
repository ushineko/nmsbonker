package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/mbin"
	"github.com/ushineko/nmsbonker/internal/save"
)

/*
The save editor (spec 007).

This file is the second exception to "the game directory is read-only except
during deploy": writeSave is the one function in the program that writes into
the game's save directory. Everything above it reads. Everything it does is
bounded: the whole profile is copied first, every time (R6.1); a running game
refuses the write (R6.2); each file is written to a temporary name and renamed
into place, data before manifest (R6.3); and nothing but the addressed pair is
ever opened for writing (R6.5).

The package internal/save knows the formats and nothing about paths. This file
knows the paths and delegates every byte-level question to it.
*/

// SteamCloudNote is what every write result carries (R6.4). Steam syncs the
// save directory; the edit is uploaded on the next launch, sometimes after a
// conflict prompt.
const SteamCloudNote = "If Steam shows a cloud sync conflict when the game next starts, choose the " +
	"local file: it is the edited one. Choosing the cloud copy discards the edit."

// ErrGameRunning reports a write refused because the game is open (R6.2).
var ErrGameRunning = errors.New("the game is running; close it before editing a save")

// ErrNoProfile reports a save directory with no st_* profile in it.
var ErrNoProfile = errors.New("no save profile (st_*) found")

// ErrNoSave reports a slot with no file behind it.
var ErrNoSave = errors.New("no save in that slot")

// procRoot is where running processes are looked up. A variable so a test can
// point it at a fixture (AC6).
//
//nolint:gochecknoglobals // test seam
var procRoot = "/proc"

// gameProcess is the executable name the game runs as under Proton.
const gameProcess = "NMS.exe"

// SlotSelector names a save. Kind "" means whichever half of the slot the game
// would load: the more recently written one.
type SlotSelector struct {
	Slot int       `json:"slot"`
	Kind save.Kind `json:"kind,omitempty"`
}

// MappingStatus says whether the save-key mapping is available (R4.2).
type MappingStatus struct {
	Path           string `json:"path,omitempty"`
	Present        bool   `json:"present"`
	LibMBINVersion string `json:"libmbin_version,omitempty"`
	Entries        int    `json:"entries,omitempty"`
	// Warning is why it is absent, when it is.
	Warning string `json:"warning,omitempty"`
}

// SaveSlotInfo is one save file as the slot screen would describe it (R5.1).
type SaveSlotInfo struct {
	Slot     int       `json:"slot"`
	Kind     save.Kind `json:"kind"`
	File     string    `json:"file"`
	Bytes    int64     `json:"bytes"`
	Modified time.Time `json:"modified"`
	// Newest marks the file the game would load: the most recently written of
	// all of them.
	Newest bool `json:"newest"`
	// The manifest's account of the save. MetaError explains an empty one.
	Name        string `json:"name,omitempty"`
	Summary     string `json:"summary,omitempty"`
	PlayTime    uint64 `json:"play_time_seconds"`
	BaseVersion uint32 `json:"base_version"`
	GameMode    string `json:"game_mode,omitempty"`
	Difficulty  uint32 `json:"difficulty"`
	Timestamp   int64  `json:"timestamp,omitempty"`
	MetaError   string `json:"meta_error,omitempty"`
}

// ListSaveSlotsRequest asks what saves the profile holds.
type ListSaveSlotsRequest struct {
	Request
}

// ListSaveSlotsResult is the profile's saves, most recently played first (R5.1).
type ListSaveSlotsResult struct {
	// Dir is the game's save folder; Profile the st_* directory read.
	Dir     string `json:"dir"`
	Profile string `json:"profile"`
	// Profiles lists every st_* directory found, for the case of several.
	Profiles    []string       `json:"profiles"`
	Slots       []SaveSlotInfo `json:"slots"`
	Mapping     MappingStatus  `json:"mapping"`
	GameRunning bool           `json:"game_running"`
}

// ListSaveSlots reads every save's manifest and says which is newest.
func ListSaveSlots(_ context.Context, req ListSaveSlotsRequest) (ListSaveSlotsResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ListSaveSlotsResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return ListSaveSlotsResult{}, err
	}
	out := ListSaveSlotsResult{Dir: s.savesDir(), GameRunning: gameRunning()}
	_, out.Mapping = s.saveMapping()
	out.Profiles, err = saveProfiles(out.Dir)
	if err != nil {
		return out, err
	}
	if len(out.Profiles) == 0 {
		return out, fmt.Errorf("%w in %s", ErrNoProfile, out.Dir)
	}
	out.Profile = out.Profiles[0]
	out.Slots, err = listSlots(out.Profile)
	return out, err
}

// listSlots describes every save file in a profile.
func listSlots(profile string) ([]SaveSlotInfo, error) {
	entries, err := os.ReadDir(profile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", profile, err)
	}
	var slots []SaveSlotInfo
	newest := -1
	for _, e := range entries {
		ref, ok := save.ParseFileName(e.Name())
		if !ok || e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", e.Name(), err)
		}
		info := SaveSlotInfo{
			Slot: ref.Slot, Kind: ref.Kind, File: filepath.Join(profile, e.Name()),
			Bytes: fi.Size(), Modified: fi.ModTime(),
		}
		describeSlot(&info, profile, ref)
		slots = append(slots, info)
		if newest < 0 || slotWritten(info).After(slotWritten(slots[newest])) {
			newest = len(slots) - 1
		}
	}
	if newest >= 0 {
		slots[newest].Newest = true
	}
	// Most recently played first, the way the game's slot screen orders
	// them: slots by their newer half, and within a slot the newer half first.
	latest := map[int]time.Time{}
	for _, sl := range slots {
		if w := slotWritten(sl); w.After(latest[sl.Slot]) {
			latest[sl.Slot] = w
		}
	}
	sort.SliceStable(slots, func(i, j int) bool {
		a, b := slots[i], slots[j]
		if a.Slot != b.Slot {
			if !latest[a.Slot].Equal(latest[b.Slot]) {
				return latest[a.Slot].After(latest[b.Slot])
			}
			return a.Slot < b.Slot
		}
		return slotWritten(a).After(slotWritten(b))
	})
	return slots, nil
}

// slotWritten is when a save was written: the manifest's own timestamp when
// it has one, the file's mtime otherwise. A Steam Cloud download gives a file
// the download's mtime, so the manifest is trusted first.
func slotWritten(info SaveSlotInfo) time.Time {
	if info.Timestamp > 0 {
		return time.Unix(info.Timestamp, 0)
	}
	return info.Modified
}

// describeSlot fills the manifest half of a SaveSlotInfo.
func describeSlot(info *SaveSlotInfo, profile string, ref save.SlotRef) {
	meta, err := readMeta(profile, ref)
	if err != nil {
		info.MetaError = err.Error()
		return
	}
	info.Name = meta.SaveName()
	info.Summary = meta.SaveSummary()
	info.PlayTime = meta.PlayTime()
	info.BaseVersion = meta.BaseVersion()
	info.GameMode = save.GameModeName(int64(meta.GameMode()))
	info.Difficulty = meta.Difficulty()
	info.Timestamp = int64(meta.Timestamp())
}

// readMeta decrypts a slot's manifest.
func readMeta(profile string, ref save.SlotRef) (*save.Meta, error) {
	raw, err := os.ReadFile(filepath.Join(profile, ref.MetaFile()))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	slot, err := save.SlotIndex(ref.DataFile())
	if err != nil {
		return nil, err
	}
	return save.DecodeMeta(raw, slot)
}

// saveProfiles lists the st_* directories, most recently written first.
func saveProfiles(dir string) ([]string, error) {
	if dir == "" {
		return nil, errors.New("no Proton prefix for this game; the game has not been run yet")
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no save folder in the Proton prefix (%s)", dir)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	type prof struct {
		path string
		when time.Time
	}
	var profs []prof
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "st_") {
			continue
		}
		p := prof{path: filepath.Join(dir, e.Name())}
		if files, err := os.ReadDir(p.path); err == nil {
			for _, f := range files {
				if fi, err := f.Info(); err == nil && fi.ModTime().After(p.when) {
					p.when = fi.ModTime()
				}
			}
		}
		profs = append(profs, p)
	}
	sort.Slice(profs, func(i, j int) bool { return profs[i].when.After(profs[j].when) })
	out := make([]string, 0, len(profs))
	for _, p := range profs {
		out = append(out, p.path)
	}
	return out, nil
}

// saveProfile is the profile operations act on: the most recently written one.
func (s *session) saveProfile() (string, error) {
	if err := s.requireInstall(); err != nil {
		return "", err
	}
	profiles, err := saveProfiles(s.savesDir())
	if err != nil {
		return "", err
	}
	if len(profiles) == 0 {
		return "", fmt.Errorf("%w in %s", ErrNoProfile, s.savesDir())
	}
	return profiles[0], nil
}

// saveMapping loads the key mapping installed beside the active compiler
// (R4.1). A missing mapping is reported in the status, not as an error, so a
// listing still works; the operations that need it check Present.
func (s *session) saveMapping() (*save.Mapping, MappingStatus) {
	compiler, err := mbin.Locate(s.paths.Tools, s.cfg.MBINCompiler.Pin)
	if err != nil {
		return nil, MappingStatus{Warning: "no MBINCompiler is installed; run `nmsbonker tools ensure`"}
	}
	path := mbin.MappingPath(compiler.Bin)
	st := MappingStatus{Path: path}
	m, err := save.LoadMapping(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			st.Warning = "no save-key mapping beside " + compiler.Tag + "; run `nmsbonker tools ensure` to fetch it"
		} else {
			st.Warning = err.Error()
		}
		return nil, st
	}
	st.Present = true
	st.LibMBINVersion = m.LibMBINVersion
	st.Entries = m.Entries
	return m, st
}

// resolveSlot turns a selector into the file to act on. With no kind, the
// more recently written half of the slot is chosen and the choice is returned,
// so the front ends can show it.
func resolveSlot(profile string, sel SlotSelector) (save.SlotRef, error) {
	if sel.Slot < 1 || sel.Slot > save.MaxSlots {
		return save.SlotRef{}, fmt.Errorf("%w: slot %d (slots are 1..%d)", save.ErrSlot, sel.Slot, save.MaxSlots)
	}
	if sel.Kind != "" {
		ref := save.SlotRef{Slot: sel.Slot, Kind: sel.Kind}
		if !ref.Valid() {
			return save.SlotRef{}, fmt.Errorf("%w: kind %q", save.ErrSlot, sel.Kind)
		}
		if _, err := os.Stat(filepath.Join(profile, ref.DataFile())); err != nil {
			return save.SlotRef{}, fmt.Errorf("%w: %s (%s)", ErrNoSave, ref, ref.DataFile())
		}
		return ref, nil
	}
	var best save.SlotRef
	var bestWhen time.Time
	for _, kind := range []save.Kind{save.KindAuto, save.KindManual} {
		ref := save.SlotRef{Slot: sel.Slot, Kind: kind}
		fi, err := os.Stat(filepath.Join(profile, ref.DataFile()))
		if err != nil {
			continue
		}
		info := SaveSlotInfo{Modified: fi.ModTime()}
		describeSlot(&info, profile, ref)
		if when := slotWritten(info); best.Slot == 0 || when.After(bestWhen) {
			best, bestWhen = ref, when
		}
	}
	if best.Slot == 0 {
		return save.SlotRef{}, fmt.Errorf("%w: slot %d", ErrNoSave, sel.Slot)
	}
	return best, nil
}

// loaded is one save read from disk and parsed.
type loaded struct {
	ref     save.SlotRef
	profile string
	path    string
	raw     []byte
	payload []byte
	root    *save.Node
	meta    *save.Meta
}

// loadSave reads, decodes and parses a save and its manifest.
func (s *session) loadSave(sel SlotSelector) (*loaded, error) {
	profile, err := s.saveProfile()
	if err != nil {
		return nil, err
	}
	ref, err := resolveSlot(profile, sel)
	if err != nil {
		return nil, err
	}
	l := &loaded{ref: ref, profile: profile, path: filepath.Join(profile, ref.DataFile())}
	if l.raw, err = os.ReadFile(l.path); err != nil {
		return nil, fmt.Errorf("read %s: %w", l.path, err)
	}
	if l.payload, err = save.Decode(l.raw); err != nil {
		return nil, fmt.Errorf("%s: %w", ref.DataFile(), err)
	}
	if l.root, err = save.Parse(l.payload); err != nil {
		return nil, fmt.Errorf("%s: %w", ref.DataFile(), err)
	}
	if l.meta, err = readMeta(profile, ref); err != nil {
		return nil, fmt.Errorf("%s: %w", ref.MetaFile(), err)
	}
	return l, nil
}

// InspectSaveRequest asks about one save.
type InspectSaveRequest struct {
	Request
	Slot SlotSelector
}

// InspectSaveResult is the save as the editor sees it (R5.2).
type InspectSaveResult struct {
	Ref     save.SlotRef  `json:"ref"`
	File    string        `json:"file"`
	Meta    SaveSlotInfo  `json:"meta"`
	Summary save.Summary  `json:"summary"`
	Mapping MappingStatus `json:"mapping"`
}

// InspectSave reads the values the editor shows. It writes nothing.
func InspectSave(_ context.Context, req InspectSaveRequest) (InspectSaveResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return InspectSaveResult{}, err
	}
	m, st := s.saveMapping()
	if m == nil {
		return InspectSaveResult{Mapping: st}, errors.New(st.Warning)
	}
	l, err := s.loadSave(req.Slot)
	if err != nil {
		return InspectSaveResult{Mapping: st}, err
	}
	out := InspectSaveResult{Ref: l.ref, File: l.path, Mapping: st}
	out.Meta = SaveSlotInfo{Slot: l.ref.Slot, Kind: l.ref.Kind, File: l.path, Bytes: int64(len(l.raw))}
	if fi, err := os.Stat(l.path); err == nil {
		out.Meta.Modified = fi.ModTime()
	}
	describeSlot(&out.Meta, l.profile, l.ref)
	out.Summary, err = save.Summarize(l.root, m)
	if err != nil {
		return out, fmt.Errorf("%s: %w", l.ref.DataFile(), err)
	}
	return out, nil
}

// ExportSaveRequest writes a save's JSON somewhere outside the prefix (R5.3).
type ExportSaveRequest struct {
	Request
	Slot SlotSelector
	// Out is the file to write; "" chooses one under the workspace.
	Out string
	// Pretty indents the JSON; Names replaces obfuscated keys with their names.
	Pretty bool
	Names  bool
}

// ExportSaveResult says what was written.
type ExportSaveResult struct {
	Ref   save.SlotRef `json:"ref"`
	File  string       `json:"file"`
	Out   string       `json:"out"`
	Bytes int64        `json:"bytes"`
	// Unnamed is how many keys stayed obfuscated in a named export.
	Unnamed int `json:"unnamed,omitempty"`
}

// ExportSave decodes a save to a JSON file.
func ExportSave(_ context.Context, req ExportSaveRequest) (ExportSaveResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ExportSaveResult{}, err
	}
	l, err := s.loadSave(req.Slot)
	if err != nil {
		return ExportSaveResult{}, err
	}
	out := ExportSaveResult{Ref: l.ref, File: l.path, Out: config.ExpandPath(req.Out)}
	if out.Out == "" {
		out.Out = filepath.Join(s.paths.Workspace, "saves",
			filepath.Base(l.profile)+"-"+strings.TrimSuffix(l.ref.DataFile(), ".hg")+".json")
	}
	if abs, err := filepath.Abs(out.Out); err == nil {
		out.Out = abs
	}
	if root, inside := s.insideTheGame(out.Out); inside {
		return out, fmt.Errorf("refusing to export into the game's own directories (%s is under %s)", out.Out, root)
	}
	m, st := s.saveMapping()
	if req.Names {
		if m == nil {
			return out, errors.New(st.Warning)
		}
		out.Unnamed = m.Deobfuscate(l.root)
	}
	body := l.root.Bytes()
	if req.Pretty {
		body = l.root.Pretty()
	}
	if err := config.MkdirAll(filepath.Dir(out.Out)); err != nil {
		return out, err
	}
	if err := os.WriteFile(out.Out, body, 0o600); err != nil {
		return out, fmt.Errorf("write %s: %w", out.Out, err)
	}
	out.Bytes = int64(len(body))
	return out, nil
}

/*
insideTheGame says whether a path lies under the Proton prefix or the game
directory, and which (R5.3, R6.5).

Both sides are resolved through symlinks first, as far as they exist, so a
link out of a scratch directory into the save folder, a relative spelling, or
a `..` in the middle cannot slip a write past the check. The whole compatdata
tree is refused rather than only the save folder: nothing this program exports
belongs anywhere in the prefix.
*/
func (s *session) insideTheGame(path string) (string, bool) {
	if s.install == nil {
		return "", false
	}
	target := resolveExisting(path)
	for _, root := range []string{s.install.CompatDataDir, s.install.Dir} {
		if root == "" {
			continue
		}
		if within(target, resolveExisting(root)) {
			return root, true
		}
	}
	return "", false
}

// resolveExisting makes a path absolute and follows symlinks through its
// longest existing prefix, re-attaching the part that does not exist yet.
func resolveExisting(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	var tail []string
	for cur := abs; ; cur = filepath.Dir(cur) {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		tail = append(tail, filepath.Base(cur))
	}
}

// within says whether a path lies under a directory, both already resolved.
func within(path, dir string) bool {
	if dir == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// SaveWriteResult is what a write into the profile did (R6.4).
type SaveWriteResult struct {
	// Backup is the directory the whole profile was copied to first.
	Backup   string `json:"backup"`
	DataFile string `json:"data_file"`
	MetaFile string `json:"meta_file"`
	// Bytes is the new data file's size; Decompressed the payload's, NUL included.
	Bytes        int64     `json:"bytes"`
	Decompressed int       `json:"decompressed"`
	Timestamp    time.Time `json:"timestamp"`
	// Forced records that the game-running check was overridden.
	Forced bool `json:"forced,omitempty"`
	// Pruned lists backups retention deleted to make room.
	Pruned []string `json:"pruned,omitempty"`
	Note   string   `json:"note"`
}

// ImportSaveRequest writes a JSON file back as a save (R5.3).
type ImportSaveRequest struct {
	Request
	Slot SlotSelector
	In   string
	// Force writes even when the game is running (R6.2).
	Force bool
}

// ImportSaveResult says what was written.
type ImportSaveResult struct {
	Ref   save.SlotRef    `json:"ref"`
	In    string          `json:"in"`
	Write SaveWriteResult `json:"write"`
	// Obfuscated is how many keys a named export had to be turned back.
	Obfuscated int `json:"obfuscated,omitempty"`
}

// ImportSave replaces a save's payload with a JSON file, re-obfuscating a
// named export, through the guarded write path.
func ImportSave(_ context.Context, req ImportSaveRequest) (ImportSaveResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ImportSaveResult{}, err
	}
	m, st := s.saveMapping()
	if m == nil {
		return ImportSaveResult{}, errors.New(st.Warning)
	}
	in := config.ExpandPath(req.In)
	body, err := os.ReadFile(in)
	if err != nil {
		return ImportSaveResult{}, fmt.Errorf("read %s: %w", in, err)
	}
	root, err := save.Parse(body)
	if err != nil {
		return ImportSaveResult{}, fmt.Errorf("%s: %w", in, err)
	}
	l, err := s.loadSave(req.Slot)
	if err != nil {
		return ImportSaveResult{}, err
	}
	out := ImportSaveResult{Ref: l.ref, In: in}
	out.Obfuscated = m.Obfuscate(root)
	// The same gate the typed edits pass through (R5.5): a payload without a
	// player state, or from before Waypoint, is not written.
	if _, _, err := save.PlayerState(root, m); err != nil {
		return out, fmt.Errorf("%s is not a save this editor writes: %w", in, err)
	}
	out.Write, err = s.writeSave(req.Events, l, root.Bytes(), req.Force)
	return out, err
}

// EditSaveRequest applies typed changes to a save (R5.4).
type EditSaveRequest struct {
	Request
	Slot    SlotSelector
	Changes save.ChangeSet
	// DryRun reports the changes and writes nothing.
	DryRun bool
	// Force writes even when the game is running (R6.2).
	Force bool
}

// EditSaveResult is what changed, and what was written if anything.
type EditSaveResult struct {
	Ref     save.SlotRef  `json:"ref"`
	File    string        `json:"file"`
	Changes []save.Change `json:"changes"`
	DryRun  bool          `json:"dry_run"`
	// Write is nil for a dry run and for a change set that changed nothing.
	Write *SaveWriteResult `json:"write,omitempty"`
}

// EditSave applies the change set through the guarded write path.
func EditSave(_ context.Context, req EditSaveRequest) (EditSaveResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return EditSaveResult{}, err
	}
	m, st := s.saveMapping()
	if m == nil {
		return EditSaveResult{}, errors.New(st.Warning)
	}
	l, err := s.loadSave(req.Slot)
	if err != nil {
		return EditSaveResult{}, err
	}
	out := EditSaveResult{Ref: l.ref, File: l.path, DryRun: req.DryRun}
	out.Changes, err = save.Apply(l.root, m, req.Changes)
	if err != nil {
		return out, fmt.Errorf("%s: %w", l.ref.DataFile(), err)
	}
	if req.DryRun || len(out.Changes) == 0 {
		return out, nil
	}
	w, err := s.writeSave(req.Events, l, l.root.Bytes(), req.Force)
	if w.Backup != "" {
		// Kept even on failure: once the backup exists the result is the
		// user's map to it (R6.4), and a write that failed halfway is exactly
		// when they need it.
		out.Write = &w
	}
	return out, err
}

/*
writeSave is the one function that writes into the save directory (R6).

Order matters. The backup is first and unconditional, so the file being
replaced is on disk somewhere else before anything is renamed. The game check
is before the backup only in the sense that a refusal costs nothing; a forced
write still takes the copy. The data file lands before the manifest, so a
crash between the two leaves a manifest describing the old sizes of a new file
-- which the game reads past -- rather than a manifest describing a file that
is not there.
*/
func (s *session) writeSave(ev Events, l *loaded, payload []byte, force bool) (SaveWriteResult, error) {
	out := SaveWriteResult{
		DataFile: l.path, MetaFile: filepath.Join(l.profile, l.ref.MetaFile()), Note: SteamCloudNote,
	}
	if gameRunning() {
		if !force {
			return out, ErrGameRunning
		}
		out.Forced = true
		ev.logf(LevelWarn, "the game is running and the write was forced; it may overwrite this save on its next autosave")
	}

	ev.logf(LevelInfo, "backing up the save profile before writing")
	backup, err := s.backupSaves()
	if err != nil {
		return out, fmt.Errorf("back up the saves first: %w", err)
	}
	if backup.Skipped != "" {
		return out, fmt.Errorf("back up the saves first: %s", backup.Skipped)
	}
	out.Backup = backup.Dir
	out.Pruned = backup.Pruned

	// From here on every failure names the backup: the profile may already be
	// half rewritten, and the copy is how to put it back.
	withBackup := func(err error) error {
		return fmt.Errorf("%w; the profile was copied to %s before the write, and can be copied back", err, out.Backup)
	}
	encoded, err := save.Encode(payload)
	if err != nil {
		return out, withBackup(err)
	}
	slot, err := save.SlotIndex(l.ref.DataFile())
	if err != nil {
		return out, withBackup(err)
	}
	now := time.Now().Truncate(time.Second)
	l.meta.SetSizes(uint32(len(payload)+1), uint32(len(encoded))) //nolint:gosec // a save is megabytes
	l.meta.SetTimestamp(uint32(now.Unix()))                       //nolint:gosec // until 2106
	metaBytes := l.meta.Encode(slot)

	if err := replaceFile(out.DataFile, encoded); err != nil {
		return out, withBackup(err)
	}
	if err := replaceFile(out.MetaFile, metaBytes); err != nil {
		return out, withBackup(err)
	}
	for _, p := range []string{out.DataFile, out.MetaFile} {
		if err := os.Chtimes(p, now, now); err != nil {
			ev.logf(LevelWarn, "set the time on %s: %v", filepath.Base(p), err)
		}
	}
	out.Bytes = int64(len(encoded))
	out.Decompressed = len(payload) + 1
	out.Timestamp = now
	ev.logf(LevelInfo, "wrote %s (%d bytes) and %s", l.ref.DataFile(), len(encoded), l.ref.MetaFile())
	return out, nil
}

// replaceFile writes data to a temporary file beside the target and renames it
// into place, keeping the target's mode (R6.3).
func replaceFile(path string, data []byte) error {
	mode := os.FileMode(0o644) //nolint:gosec // the game reads it; the original's mode wins below
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temporary file beside %s: %w", path, err)
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close %s: %w", name, err)
	}
	if err := os.Chmod(name, mode); err != nil {
		cleanup()
		return fmt.Errorf("chmod %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

/*
gameRunning looks for the game's process (R6.2).

/proc is read directly rather than through a process library: the check is one
directory listing and a short file per process, and the name Proton gives the
game is stable. `comm` is truncated to fifteen characters, which NMS.exe fits
inside; `cmdline` is checked too for the case of a launcher whose comm differs.
*/
func gameRunning() bool {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		dir := filepath.Join(procRoot, e.Name())
		if comm, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil &&
			strings.TrimSpace(string(comm)) == gameProcess {
			return true
		}
		if cmd, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			for _, arg := range strings.Split(string(cmd), "\x00") {
				if filepath.Base(strings.ReplaceAll(arg, `\`, "/")) == gameProcess {
					return true
				}
			}
		}
	}
	return false
}

// ParseSlotSelector reads a slot the way the command line spells it: "9",
// "9:auto", "9:manual". Without a kind the newest half of the slot is meant.
func ParseSlotSelector(s string) (SlotSelector, error) {
	ref, given, err := save.ParseSlotRef(s)
	if err != nil {
		return SlotSelector{}, err
	}
	sel := SlotSelector{Slot: ref.Slot}
	if given {
		sel.Kind = ref.Kind
	}
	return sel, nil
}

// --- the raw JSON browser (spec 009) ------------------------------------------

// MaxInlineJSON is the largest node the front ends put in an editor box. A
// whole save is two megabytes on one line; a text widget holding that is not an
// editor anyone can use, so the browser descends until a node fits.
const MaxInlineJSON = 256 * 1024

// SaveNodeRequest asks for one node of a save's JSON by path.
type SaveNodeRequest struct {
	Request
	Slot SlotSelector
	// Path is slash-separated names or keys, with a number for an array
	// element; "" is the whole save.
	Path string
	// Raw keeps the game's obfuscated keys in the JSON instead of naming them.
	Raw bool
}

// SaveNodeChild is one member or element of the node, for descending.
type SaveNodeChild struct {
	// Name is the readable name (or the key when unnamed); Key the key as the
	// save spells it; for an array element both are the index.
	Name  string `json:"name"`
	Key   string `json:"key"`
	Type  string `json:"type"`
	Bytes int    `json:"bytes"`
	// Len is the child's member or element count, for containers.
	Len int `json:"len"`
}

// SaveNodeResult is the node, pretty-printed when it is small enough.
type SaveNodeResult struct {
	Ref   save.SlotRef `json:"ref"`
	File  string       `json:"file"`
	Path  string       `json:"path"`
	Type  string       `json:"type"`
	Bytes int          `json:"bytes"`
	// JSON is the node indented, with keys named unless Raw; "" when TooLarge.
	JSON     string          `json:"json,omitempty"`
	TooLarge bool            `json:"too_large,omitempty"`
	Children []SaveNodeChild `json:"children,omitempty"`
}

// GetSaveNode reads one node of a save (spec 009 R1). It writes nothing.
func GetSaveNode(_ context.Context, req SaveNodeRequest) (SaveNodeResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return SaveNodeResult{}, err
	}
	m, st := s.saveMapping()
	if m == nil {
		return SaveNodeResult{}, errors.New(st.Warning)
	}
	l, err := s.loadSave(req.Slot)
	if err != nil {
		return SaveNodeResult{}, err
	}
	out := SaveNodeResult{Ref: l.ref, File: l.path, Path: strings.Join(save.SplitPath(req.Path), "/")}
	node := m.Lookup(l.root, req.Path)
	if node == nil {
		return out, fmt.Errorf("nothing at %q in %s", req.Path, l.ref.DataFile())
	}
	out.Type = node.TypeName()
	out.Bytes = node.Size()
	for i, mem := range node.Members() {
		_ = i
		out.Children = append(out.Children, SaveNodeChild{
			Name: m.Name(mem.Name), Key: mem.Name, Type: mem.Value.TypeName(),
			Bytes: mem.Value.Size(), Len: mem.Value.Len(),
		})
	}
	if node.Type() == save.TypeArray {
		for i := range node.Len() {
			el := node.Index(i)
			idx := strconv.Itoa(i)
			out.Children = append(out.Children, SaveNodeChild{
				Name: idx, Key: idx, Type: el.TypeName(), Bytes: el.Size(), Len: el.Len(),
			})
		}
	}
	if out.Bytes > MaxInlineJSON {
		out.TooLarge = true
		return out, nil
	}
	if !req.Raw {
		m.Deobfuscate(node)
	}
	out.JSON = string(node.Pretty())
	return out, nil
}

// SetSaveNodeRequest replaces one node of a save with JSON text (spec 009 R2).
type SetSaveNodeRequest struct {
	Request
	Slot SlotSelector
	Path string
	// JSON is the new value, keys named or obfuscated.
	JSON string
	// DryRun reports whether the node would change and writes nothing.
	DryRun bool
	// Force writes even when the game is running (R6.2).
	Force bool
}

// SetSaveNodeResult says what happened.
type SetSaveNodeResult struct {
	Ref  save.SlotRef `json:"ref"`
	Path string       `json:"path"`
	// Changed is false when the new value serialises to what was there.
	Changed bool `json:"changed"`
	// Obfuscated is how many keys the text spelled by name.
	Obfuscated int              `json:"obfuscated,omitempty"`
	DryRun     bool             `json:"dry_run"`
	Write      *SaveWriteResult `json:"write,omitempty"`
}

// SetSaveNode parses the text, turns named keys back, puts the value at the
// path and writes the save through the guarded path.
func SetSaveNode(_ context.Context, req SetSaveNodeRequest) (SetSaveNodeResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return SetSaveNodeResult{}, err
	}
	m, st := s.saveMapping()
	if m == nil {
		return SetSaveNodeResult{}, errors.New(st.Warning)
	}
	value, err := save.Parse([]byte(req.JSON))
	if err != nil {
		return SetSaveNodeResult{}, fmt.Errorf("the new value: %w", err)
	}
	l, err := s.loadSave(req.Slot)
	if err != nil {
		return SetSaveNodeResult{}, err
	}
	out := SetSaveNodeResult{Ref: l.ref, Path: strings.Join(save.SplitPath(req.Path), "/"), DryRun: req.DryRun}
	if len(save.SplitPath(req.Path)) == 0 {
		return out, errors.New("a path is needed; to replace the whole save use `saves import`")
	}
	old := m.Lookup(l.root, req.Path)
	if old == nil {
		return out, fmt.Errorf("nothing at %q in %s; this replaces a value, it does not add one", req.Path, l.ref.DataFile())
	}
	out.Obfuscated = m.Obfuscate(value)
	if string(value.Bytes()) == string(old.Bytes()) {
		return out, nil
	}
	if !m.Replace(l.root, req.Path, value) {
		return out, fmt.Errorf("could not replace %q", req.Path)
	}
	out.Changed = true
	// The same gate every write passes (R5.5): the result still has to be a
	// save this editor recognises.
	if _, _, err := save.PlayerState(l.root, m); err != nil {
		return out, fmt.Errorf("after the change the save is not one this editor writes: %w", err)
	}
	if req.DryRun {
		return out, nil
	}
	w, err := s.writeSave(req.Events, l, l.root.Bytes(), req.Force)
	if w.Backup != "" {
		out.Write = &w
	}
	return out, err
}
