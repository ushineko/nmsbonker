//go:build !windows

package config

// Away from Windows the XDG fallbacks under the home directory are the
// platform defaults (spec 001 R2.1), so these report nothing of their own.

func platformConfigDir() (string, bool) { return "", false }
func platformDataDir() (string, bool)   { return "", false }
func platformCacheDir() (string, bool)  { return "", false }

func hasBackslashTilde(string) bool { return false }
