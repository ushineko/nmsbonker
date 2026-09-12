package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ushineko/nmsbonker/internal/config"
)

/*
Save backup (spec 004 R5).

No Man's Sky under Proton keeps its saves in the prefix, at

	<library>/steamapps/compatdata/275850/pfx/drive_c/users/steamuser/
	    AppData/Roaming/HelloGames/NMS/st_<id>/

and Steam Cloud syncs them, which means a save corrupted by a bad mod can be
synced everywhere before anyone notices. Copying the folder before a deploy
costs a second and a few megabytes and is the only thing standing between "that
mod broke my save" and losing it.

Copy only, in one direction. Nothing here ever writes into the prefix, and
restoring is a documented manual copy: a tool that can put files back into a
save directory is a tool that can destroy a save by getting one path wrong, and
the whole value of this feature is that it cannot.
*/

// SaveRetention is how many save backups are kept (R5.1).
const SaveRetention = 10

// saveStamp is the timestamp in a backup directory's name: UTC and sortable.
const saveStamp = "20060102-150405Z"

// savesRelative is the path from the compatdata directory to the save folder.
//
//nolint:gochecknoglobals // a fixed path, read-only
var savesRelative = []string{
	"pfx", "drive_c", "users", "steamuser", "AppData", "Roaming", "HelloGames", "NMS",
}

// BackupSavesRequest copies the game's saves (R5.1).
type BackupSavesRequest struct {
	Request
}

// SaveBackupInfo is one stored backup.
type SaveBackupInfo struct {
	Timestamp string    `json:"timestamp"`
	Dir       string    `json:"dir"`
	Created   time.Time `json:"created"`
	// Profiles is how many st_* folders it holds.
	Profiles int   `json:"profiles"`
	Files    int   `json:"files"`
	Bytes    int64 `json:"bytes"`
}

// BackupSavesResult reports what was copied (R5.1).
type BackupSavesResult struct {
	// Source is the save folder in the Proton prefix, "" when there is none.
	Source string `json:"source,omitempty"`
	Dir    string `json:"dir,omitempty"`
	// Skipped says why nothing was copied, when nothing was.
	Skipped string `json:"skipped,omitempty"`
	// Error is a copy that was attempted and failed. A failed backup does not
	// fail the deploy that asked for it; it warns.
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Profiles  int    `json:"profiles"`
	Files     int    `json:"files"`
	Bytes     int64  `json:"bytes"`
	// Pruned lists backups retention deleted.
	Pruned []string `json:"pruned,omitempty"`
}

// BackupSaves copies every save profile out of the Proton prefix (R5.1).
func BackupSaves(_ context.Context, req BackupSavesRequest) (BackupSavesResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return BackupSavesResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return BackupSavesResult{}, err
	}
	return s.backupSaves()
}

// savesDir is where the game keeps its saves, or "" when the prefix is absent.
func (s *session) savesDir() string {
	if s.install == nil || s.install.CompatDataDir == "" {
		return ""
	}
	return filepath.Join(append([]string{s.install.CompatDataDir}, savesRelative...)...)
}

/*
backupSaves copies the st_* profiles to a timestamped directory.

A missing prefix is reported, not an error (R5.1): a game that has never been
started under Proton has no save folder, and a first deploy on a fresh install
is exactly when that is true.
*/
func (s *session) backupSaves() (BackupSavesResult, error) {
	out := BackupSavesResult{Source: s.savesDir()}
	if out.Source == "" {
		out.Skipped = "no Proton prefix for this game; the game has not been run yet"
		return out, nil
	}
	entries, err := os.ReadDir(out.Source)
	if errors.Is(err, os.ErrNotExist) {
		out.Skipped = "no save folder in the Proton prefix (" + out.Source + ")"
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read %s: %w", out.Source, err)
	}

	var profiles []string
	for _, e := range entries {
		if e.IsDir() && len(e.Name()) > 3 && e.Name()[:3] == "st_" {
			profiles = append(profiles, e.Name())
		}
	}
	if len(profiles) == 0 {
		out.Skipped = "no st_* save profiles in " + out.Source
		return out, nil
	}
	sort.Strings(profiles)

	out.Timestamp = time.Now().UTC().Format(saveStamp)
	out.Dir = filepath.Join(s.paths.SaveBackup, out.Timestamp)
	if err := config.MkdirAll(out.Dir); err != nil {
		return out, err
	}
	for _, name := range profiles {
		if err := copyTree(filepath.Join(out.Source, name), filepath.Join(out.Dir, name)); err != nil {
			return out, err
		}
	}
	out.Profiles = len(profiles)
	out.Files, out.Bytes, err = treeSize(out.Dir)
	if err != nil {
		return out, err
	}

	pruned, err := pruneSaveBackups(s.paths.SaveBackup)
	if err != nil {
		return out, err
	}
	out.Pruned = pruned
	return out, nil
}

// ListSaveBackupsRequest asks what has been backed up (R5.2).
type ListSaveBackupsRequest struct {
	Request
}

// ListSaveBackupsResult lists the backups, newest first.
type ListSaveBackupsResult struct {
	Dir string `json:"dir"`
	// Source is where a restore would copy back to, for the documentation the
	// front ends print beside the listing.
	Source    string           `json:"source,omitempty"`
	Backups   []SaveBackupInfo `json:"backups"`
	Retention int              `json:"retention"`
	// Enabled is the save_backup setting: whether a deploy takes one.
	Enabled bool `json:"enabled"`
}

// ListSaveBackups lists the stored save backups, newest first.
func ListSaveBackups(_ context.Context, req ListSaveBackupsRequest) (ListSaveBackupsResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ListSaveBackupsResult{}, err
	}
	out := ListSaveBackupsResult{
		Dir: s.paths.SaveBackup, Retention: SaveRetention, Enabled: s.cfg.SaveBackup,
	}
	if s.installErr == nil {
		out.Source = s.savesDir()
	}
	out.Backups, err = readSaveBackups(s.paths.SaveBackup)
	if err != nil {
		return out, err
	}
	return out, nil
}

// readSaveBackups describes every backup directory, newest first.
func readSaveBackups(dir string) ([]SaveBackupInfo, error) {
	items, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var out []SaveBackupInfo
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		created, err := time.Parse(saveStamp, item.Name())
		if err != nil {
			continue
		}
		info := SaveBackupInfo{
			Timestamp: item.Name(), Dir: filepath.Join(dir, item.Name()), Created: created,
		}
		if profiles, err := os.ReadDir(info.Dir); err == nil {
			info.Profiles = len(profiles)
		}
		info.Files, info.Bytes, _ = treeSize(info.Dir)
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	return out, nil
}

// pruneSaveBackups keeps the newest SaveRetention backups.
func pruneSaveBackups(dir string) ([]string, error) {
	backups, err := readSaveBackups(dir)
	if err != nil {
		return nil, err
	}
	if len(backups) <= SaveRetention {
		return nil, nil
	}
	var pruned []string
	for _, b := range backups[SaveRetention:] {
		if err := os.RemoveAll(b.Dir); err != nil {
			return pruned, fmt.Errorf("remove %s: %w", b.Dir, err)
		}
		pruned = append(pruned, b.Timestamp)
	}
	return pruned, nil
}

/*
backedUp remembers which game builds this process has already backed up (R5.2).

Once per process, and again whenever the game's buildid changes. The point is to
cover the deploy that follows a game update -- the one where the mods were built
against different data and the save is most at risk -- without copying a
gigabyte of saves every time someone presses Deploy twice in an afternoon.

Process-wide rather than per session because a session is created per operation;
the state being tracked is "has this run of the program done it".
*/
//
//nolint:gochecknoglobals // process-wide by design, guarded by its own mutex
var backedUp = struct {
	sync.Mutex
	builds map[string]bool
}{builds: map[string]bool{}}

// autoBackupSaves runs the pre-deploy backup if it is due (R5.2). It returns
// nil when no backup was called for, and never fails a deploy: a backup that
// could not be taken is a warning on the deploy that wanted it.
func (s *session) autoBackupSaves(ev Events) *BackupSavesResult {
	if !s.cfg.SaveBackup {
		return nil
	}
	key := s.install.BuildID
	if key == "" {
		key = s.install.Dir
	}
	backedUp.Lock()
	done := backedUp.builds[key]
	backedUp.builds[key] = true
	backedUp.Unlock()
	if done {
		return nil
	}

	ev.logf(LevelInfo, "backing up the game's saves before deploying")
	res, err := s.backupSaves()
	if err != nil {
		res.Error = err.Error()
	}
	switch {
	case res.Error != "":
		ev.logf(LevelWarn, "save backup: %s", res.Error)
	case res.Skipped != "":
		ev.logf(LevelInfo, "save backup skipped: %s", res.Skipped)
	default:
		ev.logf(LevelInfo, "saved %d profile(s), %d file(s) to %s",
			res.Profiles, res.Files, res.Dir)
	}
	return &res
}
