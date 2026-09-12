package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
)

// writeScript puts a minimal valid mod script somewhere outside the library.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	path := filepath.Join(dir, name+".lua")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

const minimalScript = `NMS_MOD_DEFINITION_CONTAINER = {
["MOD_FILENAME"] = "x.pak",
["MODIFICATIONS"] = { { ["MBIN_CHANGE_TABLE"] = { {
  ["MBIN_FILE_SOURCE"] = "GCGAMEPLAYGLOBALS.GLOBAL.MBIN",
  ["EXML_CHANGE_TABLE"] = { { ["VALUE_CHANGE_TABLE"] = { {"A", 1} } } }
} } } }
}`

// R6.2: adding a script copies it in and puts it in the build order, enabled.
func TestAddingAScriptCopiesItIntoTheLibraryAndEnablesIt(t *testing.T) {
	root := bare(t)
	src := writeScript(t, filepath.Join(root, "downloads"), "Speedy", minimalScript)

	res, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Enabled: true})
	require.NoError(t, err)
	require.Equal(t, []string{"Speedy"}, res.Added)
	require.FileExists(t, filepath.Join(res.LibraryDir, "Speedy.lua"))

	list, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	require.Len(t, list.Mods, 1)
	require.True(t, list.Mods[0].Enabled)
	require.Equal(t, core.ModOK, list.Mods[0].Status)
}

/*
R6.2: adding a script that is already there is refused unless asked.

Two mods from different authors routinely share a filename, and overwriting one
with the other silently would change what the next build produces with no trace
of why.
*/
func TestAddingOverAnExistingScriptNeedsReplace(t *testing.T) {
	root := bare(t)
	src := writeScript(t, filepath.Join(root, "downloads"), "Speedy", minimalScript)
	_, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Enabled: true})
	require.NoError(t, err)

	_, err = core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}})
	require.ErrorContains(t, err, "already in the library")

	res, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Replace: true})
	require.NoError(t, err)
	require.Equal(t, []string{"Speedy"}, res.Replaced)
}

// Only .lua files are mod scripts; a user pointing at the wrong file in a
// download folder should be told, not have it copied in. R6.2.
func TestAddingSomethingThatIsNotALuaScriptIsRefused(t *testing.T) {
	root := bare(t)
	other := filepath.Join(root, "notes.txt")
	require.NoError(t, os.MkdirAll(root, 0o750))
	require.NoError(t, os.WriteFile(other, []byte("hi"), 0o600))

	_, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{other}})
	require.ErrorContains(t, err, "is not a .lua mod script")
}

// R6.2: importing a directory adds every script in name order.
func TestImportingADirectoryAddsEveryScript(t *testing.T) {
	root := bare(t)
	dir := filepath.Join(root, "downloads")
	for _, n := range []string{"Charlie", "Alpha", "Bravo"} {
		writeScript(t, dir, n, minimalScript)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.md"), []byte("x"), 0o600))

	res, err := core.ImportDir(t.Context(), core.ImportDirRequest{Dir: dir, Enabled: true})
	require.NoError(t, err)
	require.Equal(t, []string{"Alpha", "Bravo", "Charlie"}, res.Added, "in name order, and the .md is skipped")
}

/*
R6.1: a script that appears in the library on its own is added disabled.

A file landing in a directory is not consent to build it into the mod the user
is about to install in their game. The notice is what tells them it is there.
*/
func TestAScriptThatAppearsInTheLibraryIsAddedDisabledWithANotice(t *testing.T) {
	bare(t)
	cfg := config.Defaults()
	require.NoError(t, config.MkdirAll(cfg.Paths().Library))
	writeScript(t, cfg.Paths().Library, "Surprise", minimalScript)

	list, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	require.Len(t, list.Mods, 1)
	require.False(t, list.Mods[0].Enabled)
	require.Contains(t, list.Notices[0], "added 1 script(s) found in the library, disabled: Surprise")

	// And it is persisted, so `mods enable Surprise` can find it.
	again, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	require.Len(t, again.Mods, 1)
	require.Empty(t, again.Notices)
}

/*
R6.1: an entry whose file has gone is kept and reported missing.

The usual cause is a file being edited or moved for a minute. Dropping it would
lose the position it holds in a hand-tuned load order, which is the one thing
the user cannot reconstruct.
*/
func TestAnEntryWhoseScriptIsGoneKeepsItsPlace(t *testing.T) {
	root := bare(t)
	dir := filepath.Join(root, "downloads")
	writeScript(t, dir, "First", minimalScript)
	writeScript(t, dir, "Second", minimalScript)
	_, err := core.ImportDir(t.Context(), core.ImportDirRequest{Dir: dir, Enabled: true})
	require.NoError(t, err)

	cfg := config.Defaults()
	require.NoError(t, os.Remove(filepath.Join(cfg.Paths().Library, "First.lua")))

	list, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	require.Len(t, list.Mods, 2)
	require.Equal(t, "First", list.Mods[0].Name)
	require.Equal(t, core.ModMissing, list.Mods[0].Status)
	require.Contains(t, list.Notices[0], "have no .lua in the library: First")
}

// R6.2: enable, disable and remove.
func TestEnableDisableAndRemove(t *testing.T) {
	root := bare(t)
	src := writeScript(t, filepath.Join(root, "downloads"), "Speedy", minimalScript)
	_, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Enabled: true})
	require.NoError(t, err)

	_, err = core.SetModEnabled(t.Context(), core.SetModEnabledRequest{Names: []string{"Speedy"}})
	require.NoError(t, err)
	list, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	require.False(t, list.Mods[0].Enabled)

	_, err = core.SetModEnabled(t.Context(), core.SetModEnabledRequest{Names: []string{"Nope"}, Enabled: true})
	require.ErrorContains(t, err, `no mod named "Nope"`)

	res, err := core.RemoveMod(t.Context(), core.RemoveModRequest{Name: "Speedy", DeleteFile: true})
	require.NoError(t, err)
	require.NoFileExists(t, res.Deleted)
	list, err = core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	require.Empty(t, list.Mods)
}

/*
R6.2: the build order is the conflict rule, so moving an entry has to work.

Two mods that change the same value are resolved by which one is applied later,
and this is the only control the user has over that.
*/
func TestMovingAModReordersTheBuild(t *testing.T) {
	root := bare(t)
	dir := filepath.Join(root, "downloads")
	for _, n := range []string{"A", "B", "C"} {
		writeScript(t, dir, n, minimalScript)
	}
	_, err := core.ImportDir(t.Context(), core.ImportDirRequest{Dir: dir, Enabled: true})
	require.NoError(t, err)

	res, err := core.MoveMod(t.Context(), core.MoveModRequest{Name: "C", To: 1})
	require.NoError(t, err)
	require.Equal(t, []string{"C", "A", "B"}, res.Mods)
	require.Equal(t, 3, res.From)

	res, err = core.MoveMod(t.Context(), core.MoveModRequest{Name: "C", To: 3})
	require.NoError(t, err)
	require.Equal(t, []string{"A", "B", "C"}, res.Mods)

	_, err = core.MoveMod(t.Context(), core.MoveModRequest{Name: "C", To: 9})
	require.ErrorContains(t, err, "outside 1..3")
}

// R6.2: check reports what each script says, and how many files it edits.
func TestCheckReportsWhatEachScriptEdits(t *testing.T) {
	root := bare(t)
	src := writeScript(t, filepath.Join(root, "downloads"), "Speedy", minimalScript)
	_, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Enabled: true})
	require.NoError(t, err)

	res, err := core.CheckMods(t.Context(), core.CheckModsRequest{IncludeDump: true})
	require.NoError(t, err)
	require.Equal(t, 1, res.OK)
	require.Equal(t, 0, res.Failed)
	require.True(t, res.Mods[0].OK)
	require.Equal(t, []string{"GCGAMEPLAYGLOBALS.GLOBAL.MBIN"}, res.Mods[0].Targets)
	require.Equal(t, 1, res.Mods[0].Blocks)
	require.Contains(t, string(res.Mods[0].Dump), "MOD_FILENAME")
}

/*
AC7: a script that tries to run a command is rejected at `mods check`.

The assertion that matters is the last one: the marker file the script tried to
create is not there, so the script was stopped rather than reported after the
fact.
*/
func TestCheckRejectsAScriptThatTriesToRunACommand(t *testing.T) {
	root := bare(t)
	marker := filepath.Join(root, "pwned")
	src := writeScript(t, filepath.Join(root, "downloads"), "Evil",
		"os.execute(\"touch "+marker+"\")\n"+minimalScript)
	_, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Enabled: true})
	require.NoError(t, err)

	res, err := core.CheckMods(t.Context(), core.CheckModsRequest{})
	require.NoError(t, err, "check reports a bad script, it is not itself a failure")
	require.Equal(t, 1, res.Failed)
	require.False(t, res.Mods[0].OK)
	require.Contains(t, res.Mods[0].Error, "Evil.lua")
	require.NoFileExists(t, marker)
}

// A configured entry with no file is a check failure with a plain reason rather
// than a load error about a path. R6.2.
func TestCheckReportsAMissingScriptPlainly(t *testing.T) {
	root := bare(t)
	src := writeScript(t, filepath.Join(root, "downloads"), "Speedy", minimalScript)
	_, err := core.AddMod(t.Context(), core.AddModRequest{Paths: []string{src}, Enabled: true})
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(config.Defaults().Paths().Library, "Speedy.lua")))

	res, err := core.CheckMods(t.Context(), core.CheckModsRequest{})
	require.NoError(t, err)
	require.Equal(t, "no .lua in the library", res.Mods[0].Error)
}
