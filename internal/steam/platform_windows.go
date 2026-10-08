//go:build windows

package steam

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

/*
platformRoots finds the Windows Steam client (spec 023 R2.1).

The registry is where Steam records itself: HKCU's SteamPath is written by the
client on every start (lower-case, forward slashes), HKLM's InstallPath by the
installer. The default install location is last, for a machine whose registry
has neither.
*/
func platformRoots() []string {
	var roots []string
	if v := registryString(registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath"); v != "" {
		roots = append(roots, filepath.FromSlash(v))
	}
	if v := registryString(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"); v != "" {
		roots = append(roots, v)
	}
	if pf := os.Getenv("ProgramFiles(x86)"); pf != "" {
		roots = append(roots, filepath.Join(pf, "Steam"))
	}
	for i, r := range roots {
		roots[i] = trueCase(r)
	}
	return roots
}

// trueCase spells an existing path the way the file system does. The
// registry's SteamPath is lower-case, and every path derived from it -- the
// game directory, the mod folder -- would otherwise be shown that way.
// EvalSymlinks normalises the case of each existing component on Windows.
func trueCase(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}

// registryString reads one string value, or "" when the key or value is absent.
func registryString(root registry.Key, path, name string) string {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	return v
}

// foldPath makes two spellings of one NTFS path compare equal: the file system
// ignores case, and the registry and the VDF disagree on it (spec 023 R2.2).
func foldPath(p string) string { return strings.ToLower(filepath.Clean(filepath.FromSlash(p))) }

// saveDir is where the Windows game keeps its saves: %APPDATA%\HelloGames\NMS.
// There is no Proton prefix, so compatData is unused.
func saveDir(string) string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, "HelloGames", "NMS")
}
