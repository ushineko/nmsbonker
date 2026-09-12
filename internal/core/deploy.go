package core

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ushineko/nmsbonker/internal/modsettings"
	"github.com/ushineko/nmsbonker/internal/steam"
)

// DeployRequest installs a built mod folder under GAMEDATA/MODS (spec 002 R5).
type DeployRequest struct {
	Request
	// ReplaceSymlink turns a symlinked GAMEDATA/MODS into a real directory. It
	// removes the link only, never what the link pointed at.
	ReplaceSymlink bool
	// ModName overrides the configured folder name.
	ModName string
}

// DeployResult says what was installed and what it displaced (R5.2).
type DeployResult struct {
	ModName string `json:"modName"`
	Source  string `json:"source"`
	Dest    string `json:"dest"`
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
	// Archived is where the folder this replaced was moved to, or "" when
	// nothing was there.
	Archived string `json:"archived,omitempty"`
	// ReplacedSymlink records that GAMEDATA/MODS was a symlink and is now a
	// real directory, and what the link used to point at.
	ReplacedSymlink string `json:"replacedSymlink,omitempty"`
	// DisableAllMods is the game's own switch as it stands after the deploy
	// (spec 004 R3.2). Deploy turns it off when it was on, and warns.
	DisableAllMods bool `json:"disableAllMods"`
	// SettingsPath is the game's mod settings file, "" when it does not exist.
	SettingsPath string `json:"settingsPath,omitempty"`
	// SettingsChanged reports that the file was written.
	SettingsChanged bool `json:"settingsChanged,omitempty"`
	// SettingsAdded reports that this mod had no entry and one was created.
	SettingsAdded bool `json:"settingsAdded,omitempty"`
	// Pruned lists the archive entries retention deleted (R4.1).
	Pruned []string `json:"pruned,omitempty"`
	// SaveBackup is the automatic pre-deploy save backup, when one was taken
	// (R5.2).
	SaveBackup *BackupSavesResult `json:"saveBackup,omitempty"`
	Warnings   []string           `json:"warnings,omitempty"`
}

// ErrNoBuild reports a deploy with nothing to install.
var ErrNoBuild = errors.New("no build output to deploy; run `nmsbonker build` first")

// ErrModsSymlink reports that GAMEDATA/MODS is a symlink rather than a directory.
var ErrModsSymlink = errors.New("GAMEDATA/MODS is a symlink")

// Deploy installs the last build into the game directory (R5).
func Deploy(ctx context.Context, req DeployRequest) (DeployResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return DeployResult{}, err
	}
	return deploy(ctx, s, req)
}

/*
deploy copies the built folder in, archiving whatever it replaces (R5.2).

Three properties, in the order they matter. It never removes a symlink's target:
a symlinked GAMEDATA/MODS points at a directory outside the install, and deleting
that would take the user's whole mod tree with it. It stages into a
temp directory inside GAMEDATA/MODS and renames, so a deploy interrupted halfway
leaves the previous mod intact rather than a half-copied one. And it archives
what it replaces under a timestamp, because the undo for "the new build broke my
save" has to exist before the build is installed, not after.
*/
func deploy(_ context.Context, s *session, req DeployRequest) (DeployResult, error) {
	if err := s.requireInstall(); err != nil {
		return DeployResult{}, err
	}
	modName := req.ModName
	if modName == "" {
		modName = s.cfg.ModName
	}
	out := DeployResult{ModName: modName}

	src := filepath.Join(s.paths.Workspace, modName)
	files, bytes, err := treeSize(src)
	if err != nil || files == 0 {
		return out, fmt.Errorf("%w: %s holds no built MBINs", ErrNoBuild, src)
	}
	out.Source, out.Files, out.Bytes = src, files, bytes

	modsDir := s.install.ModsDir
	switch s.install.ModsState {
	case steam.ModsSymlink:
		if !req.ReplaceSymlink {
			return out, fmt.Errorf(
				"%w pointing at %s. The game reads mods through the link, so installing here "+
					"would write into that directory instead of the game. Pass --replace-symlink "+
					"to remove the link (never its target) and create a real GAMEDATA/MODS",
				ErrModsSymlink, s.install.ModsTarget)
		}
		// Remove removes the link itself, not what it points at.
		if err := os.Remove(modsDir); err != nil {
			return out, fmt.Errorf("remove the GAMEDATA/MODS symlink: %w", err)
		}
		out.ReplacedSymlink = s.install.ModsTarget
		fallthrough
	case steam.ModsAbsent:
		if err := os.MkdirAll(modsDir, 0o755); err != nil { //nolint:gosec // the game must be able to read it
			return out, fmt.Errorf("create %s: %w", modsDir, err)
		}
	case steam.ModsDir:
	}

	dest := filepath.Join(modsDir, modName)
	staging := filepath.Join(modsDir, fmt.Sprintf("%s.tmp-%d", modName, os.Getpid()))
	if err := os.RemoveAll(staging); err != nil {
		return out, fmt.Errorf("remove %s: %w", staging, err)
	}
	if err := copyTree(src, staging); err != nil {
		_ = os.RemoveAll(staging)
		return out, err
	}

	// The saves are copied before anything in the game changes, and only after
	// the staging copy has proved it can be done (R5.2).
	if backup := s.autoBackupSaves(req.Events); backup != nil {
		out.SaveBackup = backup
		if backup.Error != "" {
			out.Warnings = append(out.Warnings, "the save backup did not run: "+backup.Error)
		}
	}

	// Archived before the rename, so a deploy that cannot install has not
	// already moved the working mod out of the game.
	entry, err := s.archiveDeploy(modName, dest, s.install.ModSettingsPath)
	if err != nil {
		_ = os.RemoveAll(staging)
		return out, err
	}
	if entry.HasMod {
		out.Archived = entry.Dir
	}
	if err := os.Rename(staging, dest); err != nil {
		return out, fmt.Errorf("install %s: %w", dest, err)
	}
	out.Dest = dest

	if err := s.writeModSettings(modName, &out); err != nil {
		return out, err
	}

	pruned, err := s.pruneArchive()
	if err != nil {
		return out, err
	}
	out.Pruned = pruned
	return out, nil
}

/*
writeModSettings makes the game load what was just installed (R3.2).

Three facts, in the order they cost the user time. A loose-file mod with no
entry in GCMODSETTINGS.MXML does not load, so an entry is created if there is
none. An entry that exists but is switched off does not load either, so it is
switched on. And the game sets DisableAllMods=true after a crash, which silently
turns every mod off; that is turned back off, with a warning, because a user who
has just deployed a mod did not mean to leave mods disabled -- but they should
be told the game had disabled them, since the usual reason is a crash.

If the file does not exist there is nothing to do and the result says so: the
game writes it the first time it sees a mod folder.
*/
func (s *session) writeModSettings(modName string, out *DeployResult) error {
	file, err := modsettings.Read(s.install.ModSettingsPath)
	if errors.Is(err, modsettings.ErrNoFile) {
		out.Warnings = append(out.Warnings,
			"the game has not written GCMODSETTINGS.MXML yet, so there is nothing to enable in "+
				"it; the game creates it the first time it sees a mod folder and will prompt at "+
				"the title screen")
		return nil
	}
	if err != nil {
		return err
	}
	out.SettingsPath = s.install.ModSettingsPath

	wasDisabled := file.DisableAllMods()
	changed := file.SetDisableAllMods(false)
	enabled, added, err := file.EnableMod(modName)
	switch {
	case errors.Is(err, modsettings.ErrNoModList):
		// The mod folder is already installed at this point, so a settings file
		// this package cannot find a mod list in is a warning rather than a
		// failed deploy. The game rewrites the file when it next starts.
		out.Warnings = append(out.Warnings,
			"GCMODSETTINGS.MXML has no mod list to add an entry to, so "+modName+
				" could not be enabled there; the game adds an entry the first time it sees "+
				"the folder and prompts at the title screen")
	case err != nil:
		return err
	}
	changed = changed || enabled
	out.SettingsAdded = added

	if changed {
		if err := file.Write(s.install.ModSettingsPath); err != nil {
			return err
		}
		out.SettingsChanged = true
	}
	out.DisableAllMods = file.DisableAllMods()
	if wasDisabled {
		out.Warnings = append(out.Warnings,
			"the game had DisableAllMods=true in GCMODSETTINGS.MXML, which turns every mod off; "+
				"the game sets that after a crash. It has been set back to false, and the "+
				"previous file is in the archive alongside this deploy")
	}
	return nil
}

// treeSize counts the files and bytes under a directory.
func treeSize(dir string) (files int, bytes int64, err error) {
	err = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", d.Name(), err)
		}
		files++
		bytes += info.Size()
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("walk %s: %w", dir, err)
	}
	return files, bytes, nil
}

// copyTree copies a directory recursively.
//
// 0755/0644: the game reads these files, and a mod folder the user cannot list
// from a file manager is a support question waiting to happen.
func copyTree(src, dest string) error {
	if err := walkCopy(src, dest); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dest, err)
	}
	return nil
}

func walkCopy(src, dest string) error {
	//nolint:gosec,wrapcheck // both trees are this program's own -- its workspace
	// output and the mod folder it is installing -- and the callback's errors
	// are already wrapped with the path they concern.
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil { //nolint:gosec // read by the game
				return fmt.Errorf("create %s: %w", target, err)
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil { //nolint:gosec // read by the game
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}

// moveTree renames a directory, falling back to copy-and-remove when the
// archive lives on a different filesystem from the game (a common Steam layout:
// game on a data drive, XDG data dir on the root filesystem).
func moveTree(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	if err := copyTree(src, dest); err != nil {
		return err
	}
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("remove %s: %w", src, err)
	}
	return nil
}
