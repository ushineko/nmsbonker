package core_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
)

// settingsFile writes the game's own mod settings document into a fake install:
// a UTF-8 BOM, CRLF endings, tabs, and a property this build does not know.
func settingsFile(t *testing.T, game, disableAll, enabled string) string {
	t.Helper()
	path := filepath.Join(game, "Binaries", "SETTINGS", "GCMODSETTINGS.MXML")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	doc := "\xef\xbb\xbf" + strings.Join([]string{
		`<?xml version="1.0" encoding="utf-8"?>`,
		`<Data template="GcModSettings">`,
		"\t" + `<Property name="DisableAllMods" value="` + disableAll + `" />`,
		"\t" + `<Property name="SomeFutureSetting" value="42" />`,
		"\t" + `<Property name="Data">`,
		"\t\t" + `<Property name="Data" value="GcModSettingsInfo" _index="0">`,
		"\t\t\t" + `<Property name="Name" value="` + config.DefaultModName + `" />`,
		"\t\t\t" + `<Property name="Author" value="" />`,
		"\t\t\t" + `<Property name="ID" value="0" />`,
		"\t\t\t" + `<Property name="AuthorID" value="0" />`,
		"\t\t\t" + `<Property name="LastUpdated" value="0" />`,
		"\t\t\t" + `<Property name="ModPriority" value="0" />`,
		"\t\t\t" + `<Property name="Enabled" value="` + enabled + `" />`,
		"\t\t\t" + `<Property name="EnabledVR" value="` + enabled + `" />`,
		"\t\t\t" + `<Property name="Dependencies" />`,
		"\t\t" + `</Property>`,
		"\t" + `</Property>`,
		`</Data>`,
		"",
	}, "\r\n")
	require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

/*
AC5: deploy turns the game's own kill switch back off, and warns.

The switch is set to true by the game after a crash, which silently stops every
mod loading. A user who has just asked for a mod to be installed did not mean to
leave mods off -- but they should be told the game had turned them off, because
the usual reason is that the game crashed.
*/
func TestDeployTurnsDisableAllModsOffAndSaysTheGameHadSetIt(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	settings := settingsFile(t, game, "true", "false")
	before := read(t, settings)
	builtOutput(t, config.DefaultModName, map[string]string{"A.MBIN": "one"})

	res, err := core.Deploy(t.Context(), core.DeployRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.False(t, res.DisableAllMods)
	require.True(t, res.SettingsChanged)
	require.False(t, res.SettingsAdded, "the entry was already there")
	require.Contains(t, strings.Join(res.Warnings, "\n"), "sets that after a crash")

	after := read(t, settings)
	require.Equal(t, []string{
		"\t" + `<Property name="DisableAllMods" value="false" />`,
		"\t\t\t" + `<Property name="Enabled" value="true" />`,
		"\t\t\t" + `<Property name="EnabledVR" value="true" />`,
	}, changedLines(t, before, after), "three values, and nothing else in the file")
	require.True(t, strings.HasPrefix(after, "\xef\xbb\xbf"), "the BOM survives")
	require.Contains(t, after, "SomeFutureSetting")

	// The file as it was is in the archive beside the deploy, so the change is
	// undoable (R4.1).
	require.NotEmpty(t, res.SettingsPath)
	list, err := core.ListArchive(t.Context(), core.ListArchiveRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.Len(t, list.Entries, 1)
	require.True(t, list.Entries[0].HasSettings)
	require.Equal(t, before, read(t, filepath.Join(list.Entries[0].Dir, "GCMODSETTINGS.MXML")))
}

// R3.2: a mod the game has never seen gets an entry rather than being installed
// and inert.
func TestDeployAddsAModSettingsEntryWhenThereIsNone(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	settings := settingsFile(t, game, "false", "true")
	builtOutput(t, "OTHER MOD", map[string]string{"A.MBIN": "one"})

	res, err := core.Deploy(t.Context(), core.DeployRequest{
		Request: core.Request{GameDir: game}, ModName: "OTHER MOD",
	})
	require.NoError(t, err)
	require.True(t, res.SettingsAdded)
	require.Contains(t, read(t, settings), `<Property name="Name" value="OTHER MOD" />`)
	require.Contains(t, read(t, settings), `_index="1"`)
}

// R3.2: a deploy that changes nothing writes nothing. A settings file rewritten
// on every deploy is a file whose modification time stops meaning anything.
func TestASecondDeployLeavesTheSettingsAlone(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	settings := settingsFile(t, game, "false", "true")
	builtOutput(t, config.DefaultModName, map[string]string{"A.MBIN": "one"})

	first, err := core.Deploy(t.Context(), core.DeployRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.False(t, first.SettingsChanged)
	after := read(t, settings)

	second, err := core.Deploy(t.Context(), core.DeployRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.False(t, second.SettingsChanged)
	require.Equal(t, after, read(t, settings))
}

/*
AC5: mods-off sets the switch and mods-on clears it, preserving the file.

The non-destructive kill switch: everything stays installed and every per-mod
entry keeps its own state, so turning mods back on restores exactly what was
loading before.
*/
func TestModsOffAndModsOnFlipOnlyTheSwitch(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	settings := settingsFile(t, game, "false", "true")
	before := read(t, settings)

	off, err := core.ModsToggle(t.Context(), core.ModsToggleRequest{
		Request: core.Request{GameDir: game}, DisableAll: true,
	})
	require.NoError(t, err)
	require.True(t, off.Changed)
	require.Equal(t, []string{config.DefaultModName}, off.Mods)
	require.Equal(t, []string{"\t" + `<Property name="DisableAllMods" value="true" />`},
		changedLines(t, before, read(t, settings)))

	// Idempotent: asking for what is already true writes nothing.
	again, err := core.ModsToggle(t.Context(), core.ModsToggleRequest{
		Request: core.Request{GameDir: game}, DisableAll: true,
	})
	require.NoError(t, err)
	require.False(t, again.Changed)

	on, err := core.ModsToggle(t.Context(), core.ModsToggleRequest{
		Request: core.Request{GameDir: game}, DisableAll: false,
	})
	require.NoError(t, err)
	require.True(t, on.Changed)
	require.Equal(t, before, read(t, settings), "byte for byte back to where it started")
}

/*
AC4: deploy twice, roll back, and the first deployment is in the game again.

The second half matters as much as the first: what was installed at the moment
of the rollback goes into a new archive entry rather than being deleted, so the
rollback is itself reversible.
*/
func TestDeployTwiceThenRollBack(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	settingsFile(t, game, "false", "true")
	req := core.Request{GameDir: game}
	dest := filepath.Join(game, "GAMEDATA", "MODS", config.DefaultModName)

	builtOutput(t, config.DefaultModName, map[string]string{"A.MBIN": "first"})
	_, err := core.Deploy(t.Context(), core.DeployRequest{Request: req})
	require.NoError(t, err)
	require.Equal(t, "first", read(t, filepath.Join(dest, "A.MBIN")))

	require.NoError(t, os.WriteFile(
		filepath.Join(config.Defaults().Paths().Workspace, config.DefaultModName, "A.MBIN"),
		[]byte("second"), 0o600))
	second, err := core.Deploy(t.Context(), core.DeployRequest{Request: req})
	require.NoError(t, err)
	require.NotEmpty(t, second.Archived, "the first deployment was archived")
	require.Equal(t, "second", read(t, filepath.Join(dest, "A.MBIN")))

	list, err := core.ListArchive(t.Context(), core.ListArchiveRequest{Request: req})
	require.NoError(t, err)
	require.Len(t, list.Entries, 2, "one entry per deploy")
	require.True(t, list.Entries[0].HasMod, "the newest holds the first deployment")
	require.False(t, list.Entries[1].HasMod, "the oldest predates any mod folder")

	back, err := core.Rollback(t.Context(), core.RollbackRequest{Request: req})
	require.NoError(t, err)
	require.False(t, back.Removed)
	require.Equal(t, 1, back.Files)
	require.Equal(t, "first", read(t, filepath.Join(dest, "A.MBIN")), "the first build is back")

	// The workspace is untouched: rolling back changes what is installed, not
	// what the next deploy would install.
	require.Equal(t, "second", read(t,
		filepath.Join(config.Defaults().Paths().Workspace, config.DefaultModName, "A.MBIN")))

	// And the build that was installed is now itself in the archive.
	require.NotEmpty(t, back.Archived)
	require.Equal(t, "second", read(t, filepath.Join(back.Archived, "mod", "A.MBIN")))
}

// AC4: an unknown timestamp names the ones that exist rather than failing bare.
func TestRollingBackToAnUnknownTimestampSaysWhatThereIs(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	req := core.Request{GameDir: game}

	_, err := core.Rollback(t.Context(), core.RollbackRequest{Request: req})
	require.ErrorIs(t, err, core.ErrNoArchive)

	builtOutput(t, config.DefaultModName, map[string]string{"A.MBIN": "one"})
	_, err = core.Deploy(t.Context(), core.DeployRequest{Request: req})
	require.NoError(t, err)

	list, err := core.ListArchive(t.Context(), core.ListArchiveRequest{Request: req})
	require.NoError(t, err)
	_, err = core.Rollback(t.Context(), core.RollbackRequest{Request: req, Timestamp: "nope"})
	require.ErrorIs(t, err, core.ErrNoArchive)
	require.ErrorContains(t, err, list.Entries[0].Timestamp)
}

// AC4/R4.1: retention keeps the newest five and says what it deleted.
func TestArchiveRetentionPrunesToFive(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	req := core.Request{GameDir: game}
	builtOutput(t, config.DefaultModName, map[string]string{"A.MBIN": "one"})

	// Seven older entries, hand-made so the test does not depend on the clock's
	// resolution to tell one deploy from the next.
	archive := config.Defaults().Paths().Archive
	for _, stamp := range []string{
		"20200101-000001Z", "20200101-000002Z", "20200101-000003Z",
		"20200101-000004Z", "20200101-000005Z", "20200101-000006Z", "20200101-000007Z",
	} {
		dir := filepath.Join(archive, config.DefaultModName+"-"+stamp, "mod")
		require.NoError(t, os.MkdirAll(dir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "OLD.MBIN"), []byte(stamp), 0o600))
	}

	res, err := core.Deploy(t.Context(), core.DeployRequest{Request: req})
	require.NoError(t, err)
	require.Equal(t, []string{
		"20200101-000003Z", "20200101-000002Z", "20200101-000001Z",
	}, res.Pruned, "the oldest three, named")

	list, err := core.ListArchive(t.Context(), core.ListArchiveRequest{Request: req})
	require.NoError(t, err)
	require.Len(t, list.Entries, core.ArchiveRetention)
	require.Equal(t, core.ArchiveRetention, list.Retention)
}

// R4.3: undeploy takes the folder out, archives it, and leaves the settings.
func TestUndeployArchivesTheFolderAndLeavesTheSettingsAlone(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	settings := settingsFile(t, game, "false", "true")
	req := core.Request{GameDir: game}
	builtOutput(t, config.DefaultModName, map[string]string{"A.MBIN": "one"})

	_, err := core.Deploy(t.Context(), core.DeployRequest{Request: req})
	require.NoError(t, err)
	after := read(t, settings)

	res, err := core.Undeploy(t.Context(), core.UndeployRequest{Request: req})
	require.NoError(t, err)
	require.Equal(t, 1, res.Files)
	require.NoDirExists(t, filepath.Join(game, "GAMEDATA", "MODS", config.DefaultModName))
	require.FileExists(t, filepath.Join(res.Archived, "mod", "A.MBIN"))
	require.Equal(t, after, read(t, settings), "the game's own mod switches are not this tool's to change")

	_, err = core.Undeploy(t.Context(), core.UndeployRequest{Request: req})
	require.ErrorIs(t, err, core.ErrNotDeployed)
}

/*
AC3: replacing a symlinked GAMEDATA/MODS deploys into a real directory.

Everything the acceptance criterion asks for, in one place: the link is gone,
what it pointed at is untouched, the built files are in the game, the settings
entry is on, and the message names none of it in terms of whatever the user had
set up before.
*/
func TestReplaceSymlinkDeploysAndEnablesTheMod(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)
	settings := settingsFile(t, game, "false", "false")
	target := filepath.Join(root, "somewhere-else")
	require.NoError(t, os.MkdirAll(target, 0o750))
	keep := filepath.Join(target, "PRECIOUS.MBIN")
	require.NoError(t, os.WriteFile(keep, []byte("do not delete"), 0o600))
	modsDir := filepath.Join(game, "GAMEDATA", "MODS")
	require.NoError(t, os.Symlink(target, modsDir))
	builtOutput(t, config.DefaultModName, map[string]string{"A.MBIN": "one"})

	refusal := core.Deploy
	_, err := refusal(t.Context(), core.DeployRequest{Request: core.Request{GameDir: game}})
	require.ErrorIs(t, err, core.ErrModsSymlink)
	require.ErrorContains(t, err, "--replace-symlink")
	requireNeutral(t, err.Error())

	res, err := core.Deploy(t.Context(), core.DeployRequest{
		Request: core.Request{GameDir: game}, ReplaceSymlink: true,
	})
	require.NoError(t, err)
	require.Equal(t, target, res.ReplacedSymlink)

	fi, err := os.Lstat(modsDir)
	require.NoError(t, err)
	require.True(t, fi.IsDir())
	require.Zero(t, fi.Mode()&os.ModeSymlink, "a real directory now")
	require.FileExists(t, filepath.Join(modsDir, config.DefaultModName, "A.MBIN"))
	require.FileExists(t, keep, "the link's former target is untouched")

	require.Contains(t, read(t, settings), `<Property name="Enabled" value="true" />`)
	require.False(t, res.DisableAllMods)
}

/*
AC3/R6.1: the symlink refusal describes the symlink and nothing else.

The tool this project replaces put a symlink there, and an error message that
named it would be a message that stops making sense the moment someone else's
setup produces the same state.
*/
func requireNeutral(t *testing.T, message string) {
	t.Helper()
	lower := strings.ToLower(message)
	for _, word := range []string{"amumss", "python", "wine", "reference", "previous setup", "builder"} {
		require.NotContainsf(t, lower, word, "the refusal mentions %q", word)
	}
	require.Contains(t, message, "symlink")
}

// AC6: a save backup copies every profile and writes nothing into the prefix.
func TestBackupSavesCopiesEveryProfile(t *testing.T) {
	root := bare(t)
	game, lib := steamGame(t, root)
	saves := fakeSaves(t, lib)

	res, err := core.BackupSaves(t.Context(), core.BackupSavesRequest{
		Request: core.Request{GameDir: game},
	})
	require.NoError(t, err)
	require.Empty(t, res.Skipped)
	require.Equal(t, 2, res.Profiles)
	require.Equal(t, 3, res.Files)
	require.Positive(t, res.Bytes)
	require.FileExists(t, filepath.Join(res.Dir, "st_1", "save.hg"))
	require.FileExists(t, filepath.Join(res.Dir, "st_2", "mf_save.hg"))

	// Nothing was written into the prefix: the same three files, unchanged.
	files, _, err := treeCount(saves)
	require.NoError(t, err)
	require.Equal(t, 3, files)
	require.Equal(t, "one", read(t, filepath.Join(saves, "st_1", "save.hg")))

	list, err := core.ListSaveBackups(t.Context(), core.ListSaveBackupsRequest{
		Request: core.Request{GameDir: game},
	})
	require.NoError(t, err)
	require.Len(t, list.Backups, 1)
	require.Equal(t, res.Timestamp, list.Backups[0].Timestamp)
	require.Equal(t, 2, list.Backups[0].Profiles)
	require.True(t, list.Enabled)
	require.Equal(t, core.SaveRetention, list.Retention)
}

// R5.1: no prefix is reported, not an error. A game that has never been started
// under Proton has no save folder, which is exactly the first-deploy case.
func TestBackupSavesWithNoPrefixSaysSo(t *testing.T) {
	root := bare(t)
	game := fakeGame(t, root)

	res, err := core.BackupSaves(t.Context(), core.BackupSavesRequest{
		Request: core.Request{GameDir: game},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Skipped)
	require.Zero(t, res.Profiles)
}

// R5.2: the first deploy of a process backs the saves up; the second does not.
func TestTheFirstDeployBacksUpTheSaves(t *testing.T) {
	root := bare(t)
	game, lib := steamGame(t, root)
	fakeSaves(t, lib)
	builtOutput(t, config.DefaultModName, map[string]string{"A.MBIN": "one"})
	req := core.Request{GameDir: game}

	first, err := core.Deploy(t.Context(), core.DeployRequest{Request: req})
	require.NoError(t, err)
	require.NotNil(t, first.SaveBackup)
	require.Equal(t, 2, first.SaveBackup.Profiles)

	second, err := core.Deploy(t.Context(), core.DeployRequest{Request: req})
	require.NoError(t, err)
	require.Nil(t, second.SaveBackup, "once per process, not once per press")
}

// R5.2: save_backup=false turns it off.
func TestSaveBackupCanBeTurnedOff(t *testing.T) {
	root := bare(t)
	game, lib := steamGame(t, root)
	fakeSaves(t, lib)
	builtOutput(t, config.DefaultModName, map[string]string{"A.MBIN": "one"})
	req := core.Request{GameDir: game}

	_, err := core.ConfigSet(t.Context(), core.ConfigSetRequest{
		Request: req, Key: "save_backup", Value: "false",
	})
	require.NoError(t, err)

	res, err := core.Deploy(t.Context(), core.DeployRequest{Request: req})
	require.NoError(t, err)
	require.Nil(t, res.SaveBackup)
	list, err := core.ListSaveBackups(t.Context(), core.ListSaveBackupsRequest{Request: req})
	require.NoError(t, err)
	require.Empty(t, list.Backups)
	require.False(t, list.Enabled)
}

// --- helpers ---------------------------------------------------------------

/*
fakeSaves builds the Proton prefix the game keeps its saves in.

The path is long and exact, and that is the point of building it in a test: the
save folder is eight directories inside compatdata, and a tool that gets one of
them wrong reports "no saves" on a machine that has plenty.
*/
func fakeSaves(t *testing.T, lib string) string {
	t.Helper()
	dir := filepath.Join(lib, "steamapps", "compatdata", "275850",
		"pfx", "drive_c", "users", "steamuser", "AppData", "Roaming", "HelloGames", "NMS")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "st_1"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "st_2"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "st_1", "save.hg"), []byte("one"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "st_1", "mf_save.hg"), []byte("two"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "st_2", "mf_save.hg"), []byte("three"), 0o600))
	return dir
}

/*
steamGame builds a fake install laid out the way Steam lays one out.

The layout is load-bearing rather than decorative: the Proton prefix is found
from the library directory, and the library is recovered from the game directory
on the assumption that it is `<library>/steamapps/common/<installdir>`. A fake
game in a flat temporary directory has no library and therefore no saves, which
is correct behaviour and useless for testing the save path.
*/
func steamGame(t *testing.T, root string) (game, lib string) {
	t.Helper()
	lib = filepath.Join(root, "SteamLibrary")
	game = filepath.Join(lib, "steamapps", "common", "No Man's Sky")
	require.NoError(t, os.MkdirAll(filepath.Join(game, "GAMEDATA", "PCBANKS"), 0o750))
	return game, lib
}

// treeCount counts the files under a directory, for the "nothing was written
// into the prefix" assertion.
func treeCount(dir string) (files int, bytes int64, err error) {
	err = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes, err
}

// changedLines is the lines of the second document that differ from the first.
func changedLines(t *testing.T, before, after string) []string {
	t.Helper()
	a := strings.Split(strings.TrimPrefix(before, "\xef\xbb\xbf"), "\r\n")
	b := strings.Split(strings.TrimPrefix(after, "\xef\xbb\xbf"), "\r\n")
	require.Equal(t, len(a), len(b), "the line count changed")
	var out []string
	for i := range a {
		if a[i] != b[i] {
			out = append(out, b[i])
		}
	}
	return out
}
