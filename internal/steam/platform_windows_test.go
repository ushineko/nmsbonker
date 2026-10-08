//go:build windows

package steam_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/steam"
)

// A junction is how a Windows user points GAMEDATA\MODS somewhere else, and Go
// reports it as irregular rather than as a symlink. Deploy must still see a
// link, or it writes through it (spec 020 R2.4).
func TestAJunctionAtModsIsALink(t *testing.T) {
	lib := t.TempDir()
	game := writeInstall(t, lib)
	target := filepath.Join(t.TempDir(), "elsewhere")
	require.NoError(t, os.MkdirAll(target, 0o750))
	mods := filepath.Join(game, "GAMEDATA", "MODS")
	//nolint:gosec // fixed arguments, test paths
	if out, err := exec.CommandContext(t.Context(), "cmd", "/c", "mklink", "/J", mods, target).CombinedOutput(); err != nil {
		t.Skipf("mklink /J: %v: %s", err, out)
	}

	in, err := steam.Locate(game)
	require.NoError(t, err)
	require.Equal(t, steam.ModsSymlink, in.ModsState)
	require.NotEmpty(t, in.ModsTarget)
}

// The registry stores the Steam root lower-case with forward slashes; the VDF
// and the environment spell it the Windows way. One root, examined once
// (spec 020 R2.2).
func TestRootsAreOneRootWhateverTheSpelling(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Steam")
	require.NoError(t, os.MkdirAll(root, 0o750))
	t.Setenv("STEAM_ROOT", root)
	t.Cleanup(steam.OverridePlatformRoots([]string{
		filepath.ToSlash(strings.ToLower(root)),
		strings.ToUpper(root),
	}))
	require.Equal(t, []string{root}, steam.Roots())
}

// Windows keeps the saves under the roaming AppData folder, whatever library
// the game is in (spec 020 R4.1).
func TestTheSaveFolderIsUnderAppData(t *testing.T) {
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)
	in, err := steam.Locate(writeInstall(t, t.TempDir()))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(appdata, "HelloGames", "NMS"), in.SaveDir)
	require.Empty(t, in.CompatDataDir, "there is no Proton prefix on Windows")
}
