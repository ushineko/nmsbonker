//go:build windows

package config

import (
	"os"
	"path/filepath"
	"strings"
)

/*
Windows base directories (spec 020 R1.1).

Settings roam with the profile (%APPDATA%); everything else is gigabytes of
machine-specific data and stays local (%LOCALAPPDATA%). The cache gets its own
subdirectory because it and the data directory would otherwise be the same
folder.
*/

func platformConfigDir() (string, bool) { return knownDir(os.UserConfigDir) }

func platformDataDir() (string, bool) { return knownDir(os.UserCacheDir) }

func platformCacheDir() (string, bool) {
	dir, ok := knownDir(os.UserCacheDir)
	if !ok {
		return "", false
	}
	return filepath.Join(dir, "cache"), true
}

// knownDir appends the program's name to one of the os package's base
// directories, or reports that Windows did not say where it is.
func knownDir(base func() (string, error)) (string, bool) {
	dir, err := base()
	if err != nil || dir == "" {
		return "", false
	}
	return filepath.Join(dir, appName), true
}

// hasBackslashTilde reports a leading "~\", the way a Windows user writes a
// path under their profile.
func hasBackslashTilde(p string) bool { return strings.HasPrefix(p, `~\`) }
