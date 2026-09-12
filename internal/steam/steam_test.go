package steam_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/steam"
)

const libraryFolders = `"libraryfolders"
{
	"0"
	{
		"path"		"/home/player/.local/share/Steam"
		"label"		""
		"apps"
		{
			"228980"		"191330831"
			"275850"		"33461606729"
		}
	}
	"1"
	{
		"path"		"D:\\SteamLibrary"
		"label"		"games"
		"apps"
		{
			"400"		"4333976147"
		}
	}
}
`

const appManifest = `"AppState"
{
	"appid"		"275850"
	"name"		"No Man's Sky"
	"StateFlags"		"4"
	"installdir"		"No Man's Sky"
	"LastUpdated"		"1789080539"
	"buildid"		"25233815"
	"InstalledDepots"
	{
		"275851"
		{
			"manifest"		"3636525443356302129"
			"size"		"33461606729"
		}
	}
}
`

// A Windows library path arrives as "D:\\SteamLibrary". Read literally it names
// a directory with a backslash in it, and the library silently disappears --
// which on a dual-boot machine is the whole game. R3.1.
func TestLibraryPathsSurviveWindowsStyleEscaping(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "steamapps"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"),
		[]byte(libraryFolders), 0o600))

	libs := steam.Libraries(root)
	require.Equal(t, root, libs[0], "the root is always a library, listed or not")
	require.Contains(t, libs, `D:\SteamLibrary`)
	require.Contains(t, libs, "/home/player/.local/share/Steam")
}

// The manifest is the only place the install directory name is written down;
// nested blocks like InstalledDepots hold keys ("manifest", "size") that must
// not be mistaken for AppState's own. R3.2.
func TestTheAppManifestYieldsTheInstallDirAndBuildID(t *testing.T) {
	lib := t.TempDir()
	writeInstall(t, lib)

	in, err := steam.Locate(filepath.Join(lib, "steamapps", "common", "No Man's Sky"))
	require.NoError(t, err)
	require.DirExists(t, in.PCBanksDir)

	// Locate with an override does not read the manifest (there may not be one),
	// so the manifest fields are checked through discovery instead.
	t.Setenv("STEAM_ROOT", lib)
	t.Setenv("HOME", t.TempDir()) // keep discovery off the developer's real Steam
	found, err := steam.Locate("")
	require.NoError(t, err)
	require.Equal(t, "25233815", found.BuildID)
	require.Equal(t, "No Man's Sky", found.Name)
	require.Equal(t, "4", found.StateFlags)
	require.Equal(t, filepath.Join(lib, "steamapps", "common", "No Man's Sky"), found.Dir)
}

// A --game-dir pointing somewhere plausible but wrong (the Binaries folder, a
// backup) is a mistake worth naming. Falling back to discovery would run the
// build against a different install than the one the user asked for. R3.3.
func TestAnExplicitGameDirIsValidatedAndSaysWhatIsMissing(t *testing.T) {
	empty := t.TempDir()
	_, err := steam.Locate(empty)
	require.Error(t, err)
	require.Contains(t, err.Error(), "GAMEDATA/PCBANKS")
	require.Contains(t, err.Error(), empty)
}

// GAMEDATA/MODS being a symlink is the legacy setup on this machine, and spec
// 004's deploy must not replace a symlink the user put there on purpose. Stat
// follows symlinks; only Lstat can tell the difference. R3.4.
func TestModsStateDistinguishesASymlinkFromADirectory(t *testing.T) {
	lib := t.TempDir()
	game := writeInstall(t, lib)

	in, err := steam.Locate(game)
	require.NoError(t, err)
	require.Equal(t, steam.ModsAbsent, in.ModsState)

	require.NoError(t, os.MkdirAll(filepath.Join(game, "GAMEDATA", "MODS"), 0o755))
	in, err = steam.Locate(game)
	require.NoError(t, err)
	require.Equal(t, steam.ModsDir, in.ModsState)

	require.NoError(t, os.Remove(filepath.Join(game, "GAMEDATA", "MODS")))
	target := filepath.Join(t.TempDir(), "elsewhere")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(game, "GAMEDATA", "MODS")))
	in, err = steam.Locate(game)
	require.NoError(t, err)
	require.Equal(t, steam.ModsSymlink, in.ModsState)
	require.Equal(t, target, in.ModsTarget)
}

// PakFiles has to be sorted, or two runs can disagree about which pak wins a
// duplicated internal path, and a build becomes non-reproducible. R3.4.
func TestPakFilesAreSortedAndExcludeNonPaks(t *testing.T) {
	lib := t.TempDir()
	game := writeInstall(t, lib)
	pcbanks := filepath.Join(game, "GAMEDATA", "PCBANKS")
	for _, name := range []string{"NMSARC.zzz.pak", "NMSARC.aaa.pak", "filenames.json", "BankSignatures.bin"} {
		require.NoError(t, os.WriteFile(filepath.Join(pcbanks, name), []byte("x"), 0o600))
	}

	in, err := steam.Locate(game)
	require.NoError(t, err)
	paks, err := in.PakFiles()
	require.NoError(t, err)
	require.Len(t, paks, 2)
	require.Equal(t, "NMSARC.aaa.pak", filepath.Base(paks[0]))
	require.Equal(t, "NMSARC.zzz.pak", filepath.Base(paks[1]))
}

// `detect` exists to explain a failure. If the candidate list were discarded on
// the way to the answer, the only thing the tool could say is "not found",
// which is exactly the case where the user needs to know where it looked. R3.2.
func TestEveryCandidateExaminedIsRetainedForDiagnostics(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("STEAM_ROOT", filepath.Join(t.TempDir(), "nowhere"))

	in, err := steam.Locate("")
	require.ErrorIs(t, err, steam.ErrNotFound)
	require.NotEmpty(t, in.Candidates)
	for _, c := range in.Candidates {
		require.NotEmpty(t, c.Reason, "a rejected candidate has to say why")
	}
}

// writeInstall lays out a minimal but valid install inside a Steam library and
// returns the game directory.
func writeInstall(t *testing.T, lib string) string {
	t.Helper()
	game := filepath.Join(lib, "steamapps", "common", "No Man's Sky")
	require.NoError(t, os.MkdirAll(filepath.Join(game, "GAMEDATA", "PCBANKS"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "steamapps"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lib, "steamapps", "appmanifest_275850.acf"),
		[]byte(appManifest), 0o600))
	return game
}
