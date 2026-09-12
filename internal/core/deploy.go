package core

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ushineko/nmsbonker/internal/config"
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
	// DisableAllMods is the game's own switch, reported not written (R5.2).
	DisableAllMods bool     `json:"disableAllMods"`
	Warnings       []string `json:"warnings,omitempty"`
}

// ErrNoBuild reports a deploy with nothing to install.
var ErrNoBuild = errors.New("no build output to deploy; run `nmsbonker build` first")

// ErrModsSymlink reports the legacy setup: GAMEDATA/MODS is a symlink.
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
the legacy pipeline points GAMEDATA/MODS at a directory outside the install, and
deleting that would take the user's whole mod tree with it. It stages into a
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
				"%w pointing at %s. That is the layout the legacy AMUMSS-on-Linux setup used: "+
					"the game reads mods through the link, so installing here would write into that "+
					"directory instead of the game. Pass --replace-symlink to remove the link "+
					"(never its target) and create a real GAMEDATA/MODS, or wait for "+
					"`nmsbonker migrate` in a later release",
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

	if _, err := os.Lstat(dest); err == nil {
		archive := filepath.Join(s.paths.Archive,
			fmt.Sprintf("%s-%s", modName, time.Now().UTC().Format("20060102-150405Z")))
		if err := config.MkdirAll(filepath.Dir(archive)); err != nil {
			_ = os.RemoveAll(staging)
			return out, err
		}
		if err := moveTree(dest, archive); err != nil {
			_ = os.RemoveAll(staging)
			return out, err
		}
		out.Archived = archive
	}
	if err := os.Rename(staging, dest); err != nil {
		return out, fmt.Errorf("install %s: %w", dest, err)
	}
	out.Dest = dest

	// The game's own mod switch is reported, never written: turning mods on
	// behind the user's back is not this tool's decision, and spec 004 owns
	// writing that file.
	if settings, err := steam.ReadModSettings(s.install.ModSettingsPath); err == nil && settings != nil {
		out.DisableAllMods = settings.DisableAllMods
		if settings.DisableAllMods {
			out.Warnings = append(out.Warnings,
				"the game has DisableAllMods=true in GCMODSETTINGS.MXML; nothing will load until "+
					"you enable mods at the title-screen warning")
		}
	}
	return out, nil
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
