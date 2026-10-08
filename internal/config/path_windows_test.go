//go:build windows

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/config"
)

// With no XDG variable set, Windows keeps settings in the roaming profile and
// everything else in the local one, with the cache in its own folder so it is
// not the data directory itself (spec 020 R1.1).
func TestWindowsDefaultsAreAppDataAndLocalAppData(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("APPDATA", filepath.Join(root, "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "Local"))
	t.Setenv(config.FileEnv, "")

	require.Equal(t, filepath.Join(root, "Roaming", "nmsbonker", "config.json"), config.FilePath())

	paths := config.Defaults().Paths()
	local := filepath.Join(root, "Local", "nmsbonker")
	require.Equal(t, filepath.Join(local, "library"), paths.Library)
	require.Equal(t, filepath.Join(local, "tools"), paths.Tools)
	require.Equal(t, filepath.Join(local, "build"), paths.Workspace)
	require.Equal(t, filepath.Join(local, "archive"), paths.Archive)
	require.Equal(t, filepath.Join(local, "save-backup"), paths.SaveBackup)
	require.Equal(t, filepath.Join(local, "cache"), paths.Cache)
}

// "~\" is how a path under the profile is written on Windows (R1.4).
func TestBackslashTildeExpandsOnWindows(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, "mods"), config.ExpandPath(`~\mods`))
	require.Equal(t, `a\~\b`, config.ExpandPath(`a\~\b`), "only a leading tilde")
}
