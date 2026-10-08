//go:build !windows

package steam_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/steam"
)

// Under Proton the saves are inside the game's prefix, and a game that has
// never run has no prefix and so no save folder (spec 020 R4.1).
func TestTheSaveFolderIsInsideTheProtonPrefix(t *testing.T) {
	lib := t.TempDir()
	game := writeInstall(t, lib)
	in, err := steam.Locate(game)
	require.NoError(t, err)
	require.Empty(t, in.SaveDir, "no prefix, no save folder")

	compat := filepath.Join(lib, "steamapps", "compatdata", "275850")
	require.NoError(t, os.MkdirAll(compat, 0o750))
	in, err = steam.Locate(game)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(compat, "pfx", "drive_c", "users", "steamuser",
		"AppData", "Roaming", "HelloGames", "NMS"), in.SaveDir)
}
