/*
Package config holds nmsbonker's on-disk settings and the directories derived
from them (spec 001 R2).

The settings live in one JSON file rather than in Fyne's preference store so
that the CLI and the GUI read the same document; the CLI has no Fyne app and
would otherwise need a second source of truth.
*/
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath resolves a leading "~" (R2.3).
//
// Only a leading one: a tilde in the middle of a path is an ordinary filename
// to every shell, and quietly rewriting it would move a directory the user
// actually named.
func ExpandPath(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// ErrTildeComponent reports a path with a component that is literally "~".
var ErrTildeComponent = errors.New("path component is a bare tilde")

// CheckCreatablePath refuses to create anything under a path component that is
// literally "~" (R2.3, angou's rule).
//
// A directory named "~" is a trap: from inside its parent, the obvious way to
// remove it is `rm -rf ~`, which the shell expands to the user's home directory
// before rm ever runs. ExpandPath has already resolved a *leading* tilde, so
// anything reaching here is a tilde deeper in the path, where expansion does not
// apply and never will.
//
// Only creation is refused. An existing directory at such a path still opens, or
// the fix for having made one would be to be unable to reach it.
func CheckCreatablePath(p string) error {
	for _, part := range strings.Split(filepath.Clean(p), string(filepath.Separator)) {
		if part != "~" {
			continue
		}
		return fmt.Errorf("%w: %s\n"+
			"Refusing to create anything under a directory named \"~\". From its parent, the "+
			"obvious way to remove it is `rm -rf ~`, which the shell expands to your home "+
			"directory before rm runs.\n"+
			"If you meant your home directory, write it as ~/ at the start of the path, or "+
			"give the full path", ErrTildeComponent, p)
	}
	return nil
}

// MkdirAll creates a directory the way every nmsbonker operation should: lazily,
// at the moment it is needed, and never under a bare-tilde component.
func MkdirAll(dir string) error {
	if err := CheckCreatablePath(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return nil
}

// xdgDir resolves an XDG base directory variable with its specified fallback.
func xdgDir(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return ExpandPath(v)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return fallback
	}
	return filepath.Join(home, fallback)
}

func configHome() string { return xdgDir("XDG_CONFIG_HOME", ".config") }
func dataHome() string   { return xdgDir("XDG_DATA_HOME", filepath.Join(".local", "share")) }
func cacheHome() string  { return xdgDir("XDG_CACHE_HOME", ".cache") }
