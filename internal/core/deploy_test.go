package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
)

// fakeGame builds a directory that steam.Locate will accept as an install:
// GAMEDATA/PCBANKS is the only thing it validates.
func fakeGame(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "game")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "GAMEDATA", "PCBANKS"), 0o750))
	return dir
}

// builtOutput fills the workspace with something worth deploying.
func builtOutput(t *testing.T, modName string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(config.Defaults().Paths().Workspace, modName)
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return dir
}

/*
R5.1: deploy refuses a symlinked GAMEDATA/MODS, with the reason.

A symlink here points at a mod tree outside the game install. Installing "into
the game" would silently write into the user's other tree, and removing the link
without saying so would take their existing setup out of the loading path.
*/
func TestDeployRefusesASymlinkedModsDirectoryAndSaysWhy(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	target := filepath.Join(root, "external-mods")
	require.NoError(t, os.MkdirAll(target, 0o750))
	require.NoError(t, os.Symlink(target, filepath.Join(game, "GAMEDATA", "MODS")))
	builtOutput(t, config.DefaultModName, map[string]string{"A.MBIN": "one"})

	_, err := core.Deploy(t.Context(), core.DeployRequest{Request: core.Request{GameDir: game}})
	require.ErrorIs(t, err, core.ErrModsSymlink)
	require.ErrorContains(t, err, target)
	require.ErrorContains(t, err, "--replace-symlink")
}

/*
AC4/R5.1: --replace-symlink removes the link and never its target.

The link is the thing in the way; what it points at is the user's own mod tree.
Deleting that would be unrecoverable, so the test asserts the target's contents
survive.
*/
func TestReplaceSymlinkRemovesTheLinkButNotWhatItPointsAt(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	target := filepath.Join(root, "external-mods")
	require.NoError(t, os.MkdirAll(target, 0o750))
	keep := filepath.Join(target, "PRECIOUS.MBIN")
	require.NoError(t, os.WriteFile(keep, []byte("do not delete"), 0o600))
	modsDir := filepath.Join(game, "GAMEDATA", "MODS")
	require.NoError(t, os.Symlink(target, modsDir))
	builtOutput(t, config.DefaultModName, map[string]string{
		"A.MBIN": "one", "METADATA/B.MBIN": "two",
	})

	res, err := core.Deploy(t.Context(), core.DeployRequest{
		Request: core.Request{GameDir: game}, ReplaceSymlink: true,
	})
	require.NoError(t, err)
	require.Equal(t, target, res.ReplacedSymlink)
	require.Empty(t, res.Archived, "nothing was installed before, so nothing was archived")
	require.Equal(t, 2, res.Files)

	require.FileExists(t, keep, "the symlink's target is untouched")
	fi, err := os.Lstat(modsDir)
	require.NoError(t, err)
	require.True(t, fi.IsDir())
	require.Zero(t, fi.Mode()&os.ModeSymlink, "and it is a real directory now")
	require.FileExists(t, filepath.Join(modsDir, config.DefaultModName, "A.MBIN"))
	require.FileExists(t, filepath.Join(modsDir, config.DefaultModName, "METADATA", "B.MBIN"))
}

/*
R5.2: an existing mod folder is archived, not overwritten.

"The new build broke my save" needs an undo, and the undo has to exist before
the new build is installed. The archive is timestamped so a second deploy does
not overwrite the first one's copy.
*/
func TestDeployArchivesTheFolderItReplaces(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	modsDir := filepath.Join(game, "GAMEDATA", "MODS")
	previous := filepath.Join(modsDir, config.DefaultModName)
	require.NoError(t, os.MkdirAll(previous, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(previous, "OLD.MBIN"), []byte("previous"), 0o600))
	builtOutput(t, config.DefaultModName, map[string]string{"NEW.MBIN": "fresh"})

	res, err := core.Deploy(t.Context(), core.DeployRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.NotEmpty(t, res.Archived)
	require.FileExists(t, filepath.Join(res.Archived, "OLD.MBIN"))
	require.FileExists(t, filepath.Join(previous, "NEW.MBIN"))
	require.NoFileExists(t, filepath.Join(previous, "OLD.MBIN"), "the new folder replaced it, it was not merged into")
}

// R5.1: nothing to deploy is a clear refusal, not an empty mod folder installed
// over a working one.
func TestDeployWithNoBuildRefuses(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)

	_, err := core.Deploy(t.Context(), core.DeployRequest{Request: core.Request{GameDir: game}})
	require.ErrorIs(t, err, core.ErrNoBuild)
}

// R5.2: the game's own mod switch is reported, never written. Turning mods on
// behind the user's back is not this tool's decision.
func TestDeployReportsTheGamesDisableAllModsSwitch(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	settings := filepath.Join(game, "Binaries", "SETTINGS", "GCMODSETTINGS.MXML")
	require.NoError(t, os.MkdirAll(filepath.Dir(settings), 0o750))
	require.NoError(t, os.WriteFile(settings, []byte(
		`<Data template="GcModSettings"><Property name="DisableAllMods" value="True" /></Data>`), 0o600))
	builtOutput(t, config.DefaultModName, map[string]string{"A.MBIN": "one"})

	res, err := core.Deploy(t.Context(), core.DeployRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.True(t, res.DisableAllMods)
	require.Contains(t, res.Warnings[0], "DisableAllMods=true")

	after, err := os.ReadFile(settings)
	require.NoError(t, err)
	require.Contains(t, string(after), `value="True"`, "reported, not written")
}

// R6.3: asking for a report before anything has been built says so.
func TestReportBeforeAnyBuildSaysSo(t *testing.T) {
	bare(t)
	_, err := core.Report(t.Context(), core.ReportRequest{})
	require.ErrorIs(t, err, core.ErrNoReport)
}

// R6.3: a build with nothing enabled refuses rather than installing an empty
// mod folder over a working one.
func TestBuildWithNoEnabledModsRefuses(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)

	_, err := core.Build(t.Context(), core.BuildRequest{Request: core.Request{GameDir: game}})
	require.Error(t, err)
	require.NotErrorIs(t, err, core.ErrNoEnabledMods, "the missing compiler is reported first")
	require.ErrorContains(t, err, "tools ensure")
}
