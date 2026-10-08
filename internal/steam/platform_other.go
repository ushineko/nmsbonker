//go:build !windows

package steam

import (
	"os"
	"path/filepath"
)

// platformRoots are the places a Linux Steam client lives: native, the
// ~/.steam links, Flatpak and Snap.
func platformRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".steam", "root"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
		filepath.Join(home, "snap", "steam", "common", ".local", "share", "Steam"),
	}
}

// foldPath is the identity: Linux paths are case-sensitive.
func foldPath(p string) string { return p }

// protonSaves is the path from the compatdata directory to the save folder.
//
//nolint:gochecknoglobals // a fixed path, read-only
var protonSaves = []string{
	"pfx", "drive_c", "users", "steamuser", "AppData", "Roaming", "HelloGames", "NMS",
}

// saveDir is the save folder inside the game's Proton prefix, or "" when the
// game has never run under Proton and there is no prefix.
func saveDir(compatData string) string {
	if compatData == "" {
		return ""
	}
	return filepath.Join(append([]string{compatData}, protonSaves...)...)
}
