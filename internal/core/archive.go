package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/modsettings"
)

/*
The archive: what deploy displaced, and how to put it back (spec 004 R4).

One directory per deploy, named `<MOD_NAME>-<UTC timestamp>`, holding whatever
that deploy replaced:

	archive/COSMOS COMBINE-20260911-204500Z/
	    mod/                  the mod folder that was installed, if there was one
	    GCMODSETTINGS.MXML    the game's mod switches as they were, if the file existed
	    archive.json          what this entry is and when it was made

An entry is written on every deploy, including the first, because the settings
file is edited on every deploy and "undo" has to include undoing that. An entry
with no `mod/` records that nothing was installed at that point, and rolling
back to it removes what is installed now -- which is the correct restoration of
that state, and the confirmation dialog says so.

Retention keeps the newest five (R4.1). Deleting is reported rather than
silent: the archive is the undo button, and a user should know how far back it
now reaches.
*/

// ArchiveRetention is how many deploy archives are kept (R4.1).
const ArchiveRetention = 5

// archiveMeta is archive.json.
type archiveMeta struct {
	ModName   string    `json:"modName"`
	Timestamp string    `json:"timestamp"`
	Created   time.Time `json:"created"`
	// Dest is where the mod folder came from, so a rollback puts it back in the
	// same place even if the game has moved.
	Dest string `json:"dest"`
	// HadMod records that a mod folder was displaced, as opposed to this being
	// the first deploy into an empty MODS directory.
	HadMod bool `json:"hadMod"`
	// SettingsPath is the settings file the backup was taken from.
	SettingsPath string `json:"settingsPath,omitempty"`
	// Files and Bytes describe the archived mod folder.
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

const (
	archiveModDir   = "mod"
	archiveMetaFile = "archive.json"
	archiveSettings = "GCMODSETTINGS.MXML"
	// archiveStamp is the timestamp format in an entry's directory name. UTC
	// and sortable, so listing the directory in name order is listing it in
	// time order whatever the machine's locale does.
	archiveStamp = "20060102-150405Z"
)

// ArchiveEntry is one archived deploy, as `archive list` reports it.
type ArchiveEntry struct {
	// Timestamp is the entry's identifier, which `rollback TS` takes.
	Timestamp string    `json:"timestamp"`
	ModName   string    `json:"modName"`
	Created   time.Time `json:"created"`
	Dir       string    `json:"dir"`
	// HasMod reports whether a mod folder is stored here.
	HasMod bool `json:"hasMod"`
	// HasSettings reports whether the game's mod switches are stored here.
	HasSettings bool  `json:"hasSettings"`
	Files       int   `json:"files"`
	Bytes       int64 `json:"bytes"`
}

// ListArchiveRequest asks what can be rolled back to (R4.2).
type ListArchiveRequest struct {
	Request
}

// ListArchiveResult is every archived deploy, newest first.
type ListArchiveResult struct {
	Dir       string         `json:"dir"`
	Entries   []ArchiveEntry `json:"entries"`
	Retention int            `json:"retention"`
}

// ListArchive lists the archived deploys, newest first.
func ListArchive(_ context.Context, req ListArchiveRequest) (ListArchiveResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ListArchiveResult{}, err
	}
	entries, err := readArchive(s.paths.Archive)
	if err != nil {
		return ListArchiveResult{}, err
	}
	return ListArchiveResult{Dir: s.paths.Archive, Entries: entries, Retention: ArchiveRetention}, nil
}

// readArchive reads every entry directory, newest first.
func readArchive(dir string) ([]ArchiveEntry, error) {
	items, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var out []ArchiveEntry
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		entry, ok := readArchiveEntry(filepath.Join(dir, item.Name()))
		if !ok {
			continue
		}
		out = append(out, entry)
	}
	// Newest first: the answer to "roll back" is almost always the last one,
	// and it should be the row the eye lands on.
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	return out, nil
}

/*
readArchiveEntry describes one entry directory.

An entry with no archive.json is still read: a directory named
`<mod>-<timestamp>` written by an earlier version of this tool held the mod
folder directly, and refusing to list it would hide the only copy the user has
of whatever it displaced.
*/
func readArchiveEntry(dir string) (ArchiveEntry, bool) {
	base := filepath.Base(dir)
	entry := ArchiveEntry{Dir: dir}

	var meta archiveMeta
	if b, err := os.ReadFile(filepath.Join(dir, archiveMetaFile)); err == nil {
		if err := json.Unmarshal(b, &meta); err == nil {
			entry.ModName, entry.Timestamp, entry.Created = meta.ModName, meta.Timestamp, meta.Created
			entry.Files, entry.Bytes = meta.Files, meta.Bytes
		}
	}
	if entry.Timestamp == "" {
		// `<mod name>-<timestamp>`: the mod name may hold dashes, the timestamp
		// may not, so the split is from the right.
		i := strings.LastIndex(base, "-2")
		if i < 0 {
			return entry, false
		}
		entry.ModName, entry.Timestamp = base[:i], base[i+1:]
		if t, err := time.Parse(archiveStamp, entry.Timestamp); err == nil {
			entry.Created = t
		}
	}
	if fi, err := os.Stat(filepath.Join(dir, archiveModDir)); err == nil && fi.IsDir() {
		entry.HasMod = true
		if entry.Files == 0 {
			entry.Files, entry.Bytes, _ = treeSize(filepath.Join(dir, archiveModDir))
		}
	}
	if _, err := os.Stat(filepath.Join(dir, archiveSettings)); err == nil {
		entry.HasSettings = true
	}
	return entry, true
}

/*
archiveDeploy records what a deploy is about to displace.

Called before the new folder is installed and after the staging copy has
succeeded, so a deploy that fails to copy has not already moved the user's
working mod out of the game.
*/
func (s *session) archiveDeploy(modName, dest, settingsPath string) (ArchiveEntry, error) {
	stamp, dir := s.freeArchiveDir(modName)
	if err := config.MkdirAll(dir); err != nil {
		return ArchiveEntry{}, err
	}
	meta := archiveMeta{
		ModName: modName, Timestamp: stamp, Created: time.Now().UTC(), Dest: dest,
	}

	if _, err := os.Lstat(dest); err == nil {
		files, bytes, err := treeSize(dest)
		if err != nil {
			return ArchiveEntry{}, err
		}
		if err := moveTree(dest, filepath.Join(dir, archiveModDir)); err != nil {
			return ArchiveEntry{}, err
		}
		meta.HadMod, meta.Files, meta.Bytes = true, files, bytes
	}

	if settingsPath != "" {
		if err := copyFile(settingsPath, filepath.Join(dir, archiveSettings)); err == nil {
			meta.SettingsPath = settingsPath
		} else if !errors.Is(err, os.ErrNotExist) {
			return ArchiveEntry{}, err
		}
	}

	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return ArchiveEntry{}, fmt.Errorf("encode %s: %w", archiveMetaFile, err)
	}
	if err := os.WriteFile(filepath.Join(dir, archiveMetaFile), append(b, '\n'), 0o600); err != nil {
		return ArchiveEntry{}, fmt.Errorf("write %s: %w", archiveMetaFile, err)
	}
	entry, _ := readArchiveEntry(dir)
	return entry, nil
}

/*
freeArchiveDir picks an entry directory that does not exist yet.

The timestamp has one-second resolution, which two deploys in the same second
would share -- and a second deploy that wrote into the first's directory would
merge two mod folders into one archive entry and lose the ability to roll back
to either. A counter suffix keeps the name sortable, since it sorts after the
bare timestamp it extends.
*/
func (s *session) freeArchiveDir(modName string) (stamp, dir string) {
	stamp = time.Now().UTC().Format(archiveStamp)
	dir = filepath.Join(s.paths.Archive, modName+"-"+stamp)
	for n := 2; n < 1000; n++ {
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			return stamp, dir
		}
		stamp = fmt.Sprintf("%s-%d", time.Now().UTC().Format(archiveStamp), n)
		dir = filepath.Join(s.paths.Archive, modName+"-"+stamp)
	}
	return stamp, dir
}

// pruneArchive deletes all but the newest ArchiveRetention entries and returns
// the timestamps it removed (R4.1).
func (s *session) pruneArchive() ([]string, error) {
	entries, err := readArchive(s.paths.Archive)
	if err != nil {
		return nil, err
	}
	if len(entries) <= ArchiveRetention {
		return nil, nil
	}
	var pruned []string
	for _, e := range entries[ArchiveRetention:] {
		if err := os.RemoveAll(e.Dir); err != nil {
			return pruned, fmt.Errorf("remove %s: %w", e.Dir, err)
		}
		pruned = append(pruned, e.Timestamp)
	}
	return pruned, nil
}

// RollbackRequest puts an archived deploy back (R4.2).
type RollbackRequest struct {
	Request
	// Timestamp names the entry; empty means the newest.
	Timestamp string
	// ModName overrides the configured folder name.
	ModName string
}

// RollbackResult says what was restored and what took its place in the archive.
type RollbackResult struct {
	ModName string `json:"modName"`
	// From is the entry that was restored.
	From ArchiveEntry `json:"from"`
	// Dest is the folder under GAMEDATA/MODS that now holds it.
	Dest string `json:"dest"`
	// Removed reports that the archived state was "nothing installed", so the
	// deployed folder was taken out rather than replaced.
	Removed bool `json:"removed,omitempty"`
	// Archived is where the folder that was installed has been put.
	Archived string `json:"archived,omitempty"`
	// SettingsRestored is the settings file that was put back, if any.
	SettingsRestored string   `json:"settingsRestored,omitempty"`
	Files            int      `json:"files"`
	Warnings         []string `json:"warnings,omitempty"`
}

// ErrNoArchive reports that there is nothing to roll back to.
var ErrNoArchive = errors.New("no archived deploy to roll back to")

/*
Rollback swaps the deployed mod folder with an archived one (R4.2).

The folder that is currently installed becomes a new archive entry rather than
being deleted, so a rollback is itself reversible: "the previous build broke my
save" and "no, it was the other thing" are both recoverable states. The
workspace build is not touched at all -- rolling back changes what is installed
in the game, not what the next deploy would install.
*/
func Rollback(_ context.Context, req RollbackRequest) (RollbackResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return RollbackResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return RollbackResult{}, err
	}
	modName := req.ModName
	if modName == "" {
		modName = s.cfg.ModName
	}
	out := RollbackResult{ModName: modName}

	entries, err := readArchive(s.paths.Archive)
	if err != nil {
		return out, err
	}
	entry, err := pickArchive(entries, req.Timestamp)
	if err != nil {
		return out, err
	}
	out.From = entry

	dest := filepath.Join(s.install.ModsDir, modName)
	out.Dest = dest

	// The folder in the game now goes into a new entry first, so the swap never
	// has both copies out of the archive at once.
	archived, err := s.archiveDeploy(modName, dest, s.install.ModSettingsPath)
	if err != nil {
		return out, err
	}
	out.Archived = archived.Dir

	if entry.HasMod {
		if err := os.MkdirAll(s.install.ModsDir, 0o755); err != nil { //nolint:gosec // read by the game
			return out, fmt.Errorf("create %s: %w", s.install.ModsDir, err)
		}
		if err := moveTree(filepath.Join(entry.Dir, archiveModDir), dest); err != nil {
			return out, err
		}
		out.Files = entry.Files
	} else {
		out.Removed = true
		out.Warnings = append(out.Warnings,
			"that archive records the state before anything was installed, so the mod folder "+
				"has been removed rather than replaced")
	}

	if entry.HasSettings {
		src := filepath.Join(entry.Dir, archiveSettings)
		if s.install.ModSettingsPath != "" {
			if err := copyFile(src, s.install.ModSettingsPath); err != nil {
				return out, err
			}
			out.SettingsRestored = s.install.ModSettingsPath
		}
	}

	// The entry has been consumed: its folder is back in the game and its
	// settings are restored. Leaving an empty directory behind would make
	// `archive list` offer a rollback that does nothing.
	if err := os.RemoveAll(entry.Dir); err != nil {
		return out, fmt.Errorf("remove %s: %w", entry.Dir, err)
	}
	return out, nil
}

// pickArchive chooses the entry a rollback names, defaulting to the newest.
func pickArchive(entries []ArchiveEntry, timestamp string) (ArchiveEntry, error) {
	if len(entries) == 0 {
		return ArchiveEntry{}, ErrNoArchive
	}
	if timestamp == "" {
		return entries[0], nil
	}
	for _, e := range entries {
		if e.Timestamp == timestamp {
			return e, nil
		}
	}
	var have []string
	for _, e := range entries {
		have = append(have, e.Timestamp)
	}
	return ArchiveEntry{}, fmt.Errorf("%w named %q (the archive holds %s)",
		ErrNoArchive, timestamp, strings.Join(have, ", "))
}

// UndeployRequest removes the installed mod folder (R4.3).
type UndeployRequest struct {
	Request
	ModName string
}

// UndeployResult says what was removed and where it went.
type UndeployResult struct {
	ModName  string `json:"modName"`
	Dest     string `json:"dest"`
	Archived string `json:"archived"`
	Files    int    `json:"files"`
	// Pruned lists archive entries retention deleted.
	Pruned []string `json:"pruned,omitempty"`
}

// ErrNotDeployed reports that there is no mod folder in the game to remove.
var ErrNotDeployed = errors.New("no mod folder is installed")

/*
Undeploy takes the mod folder out of the game, archiving it (R4.3).

The game's mod settings are left exactly as they are, deliberately. The entry
naming a folder that is no longer there is harmless -- the game ignores it --
and turning mods off because one mod was removed would be this tool deciding
something about the user's other mods.
*/
func Undeploy(_ context.Context, req UndeployRequest) (UndeployResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return UndeployResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return UndeployResult{}, err
	}
	modName := req.ModName
	if modName == "" {
		modName = s.cfg.ModName
	}
	out := UndeployResult{ModName: modName}

	dest := filepath.Join(s.install.ModsDir, modName)
	fi, err := os.Lstat(dest)
	if err != nil || !fi.IsDir() {
		return out, fmt.Errorf("%w at %s", ErrNotDeployed, dest)
	}
	out.Dest = dest

	entry, err := s.archiveDeploy(modName, dest, s.install.ModSettingsPath)
	if err != nil {
		return out, err
	}
	out.Archived, out.Files = entry.Dir, entry.Files

	pruned, err := s.pruneArchive()
	if err != nil {
		return out, err
	}
	out.Pruned = pruned
	return out, nil
}

// ModsToggleRequest flips the game's own kill switch (R3.3).
type ModsToggleRequest struct {
	Request
	// DisableAll is the value to write.
	DisableAll bool
}

// ModsToggleResult reports the switch's state.
type ModsToggleResult struct {
	Path       string `json:"path"`
	DisableAll bool   `json:"disableAllMods"`
	Changed    bool   `json:"changed"`
	// Mods lists the entries the file holds, so the CLI can say what the switch
	// is switching.
	Mods []string `json:"mods,omitempty"`
}

/*
ModsToggle flips DisableAllMods and nothing else (R3.3).

It is the non-destructive kill switch: everything stays installed and every
per-mod entry keeps its own state, so turning mods back on restores exactly what
was loading before. That is what makes it the right first thing to try when a
game stops starting -- the alternative, removing the mod folder, throws away the
information about what was in it.
*/
func ModsToggle(_ context.Context, req ModsToggleRequest) (ModsToggleResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ModsToggleResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return ModsToggleResult{}, err
	}
	out := ModsToggleResult{Path: s.install.ModSettingsPath, DisableAll: req.DisableAll}

	file, err := modsettings.Read(s.install.ModSettingsPath)
	if err != nil {
		return out, err
	}
	for _, e := range file.Entries() {
		out.Mods = append(out.Mods, e.Name)
	}
	out.Changed = file.SetDisableAllMods(req.DisableAll)
	if !out.Changed {
		return out, nil
	}
	if err := file.Write(s.install.ModSettingsPath); err != nil {
		return out, err
	}
	return out, nil
}
