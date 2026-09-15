package core_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/tweaks"
)

/*
libraryMods is the listing with the built-in tweaks taken out.

Every configuration holds the ten built-ins from spec 004 R1.4 onwards, disabled
until asked for. The tests below are about what the library does, so they filter
them rather than counting eleven where they mean one; the built-ins have their
own tests.
*/
func libraryMods(list core.ListModsResult) []core.ModInfo {
	var out []core.ModInfo
	for _, m := range list.Mods {
		if m.Source != core.SourceBuiltin {
			out = append(out, m)
		}
	}
	return out
}

// notices is every notice joined, so a test can look for one without depending
// on how many others were emitted alongside it.
func notices(list core.ListModsResult) string { return strings.Join(list.Notices, "\n") }

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
	mods := libraryMods(list)
	require.Len(t, mods, 1)
	require.True(t, mods[0].Enabled)
	require.Equal(t, core.ModOK, mods[0].Status)
	require.Equal(t, core.SourceLibrary, mods[0].Source)
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
	mods := libraryMods(list)
	require.Len(t, mods, 1)
	require.False(t, mods[0].Enabled)
	require.Contains(t, notices(list), "added 1 script(s) found in the library, disabled: Surprise")

	// And it is persisted, so `mods enable Surprise` can find it.
	again, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	require.Len(t, libraryMods(again), 1)
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
	mods := libraryMods(list)
	require.Len(t, mods, 2)
	require.Equal(t, "First", mods[0].Name)
	require.Equal(t, core.ModMissing, mods[0].Status)
	require.Contains(t, notices(list), "have no .lua in the library: First")
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
	require.False(t, libraryMods(list)[0].Enabled)

	_, err = core.SetModEnabled(t.Context(), core.SetModEnabledRequest{Names: []string{"Nope"}, Enabled: true})
	require.ErrorContains(t, err, `no mod named "Nope"`)

	res, err := core.RemoveMod(t.Context(), core.RemoveModRequest{Name: "Speedy", DeleteFile: true})
	require.NoError(t, err)
	require.NoFileExists(t, res.Deleted)
	list, err = core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	require.Empty(t, libraryMods(list))
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

/*
R1.4: the built-ins are in every configuration, in their own order, disabled.

Disabled matters more than the order does. Installing a mod builder is not
consent to change anyone's game, and a fresh install that quietly enabled ten
tweaks would produce a first build nobody asked for.
*/
func TestTheBuiltInsAreListedInOrderAndStartDisabled(t *testing.T) {
	bare(t)

	list, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)

	var names []string
	for _, m := range list.Mods {
		require.Equal(t, core.SourceBuiltin, m.Source)
		require.False(t, m.Enabled, "%s is enabled on a fresh install", m.Name)
		require.Equal(t, core.ModOK, m.Status)
		require.Empty(t, m.Path, "a built-in has no file in the library")
		require.Positive(t, m.Params, "%s declares no parameters", m.Name)
		names = append(names, m.Name)
	}
	require.Equal(t, tweaks.Names(), names)
	require.Contains(t, notices(list), "added 12 built-in tweak(s), disabled")

	// Persisted, so the second call has nothing to add and nothing to say.
	again, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	require.Empty(t, again.Notices)
	require.Len(t, again.Mods, len(tweaks.Names()))
}

/*
R1.4: a library script with a built-in's name is shadowed, not applied twice.

This is what happens to anyone who imported the reference script set before the
built-ins existed. The build must use one copy, the interface must say which,
and the ignored one must be removable.
*/
func TestALibraryScriptWithABuiltInsNameIsShadowed(t *testing.T) {
	bare(t)
	cfg := config.Defaults()
	require.NoError(t, config.MkdirAll(cfg.Paths().Library))
	writeScript(t, cfg.Paths().Library, "ItemValueBoost", minimalScript)

	list, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	require.Len(t, list.Mods, len(tweaks.Names()), "one entry, not two")
	require.Contains(t, notices(list), "same name as a built-in tweak and are ignored")

	var shadowed core.ModInfo
	for _, m := range list.Mods {
		if m.Name == "ItemValueBoost" {
			shadowed = m
		}
	}
	require.True(t, shadowed.Shadowed)
	require.Equal(t, core.SourceBuiltin, shadowed.Source)
	require.FileExists(t, shadowed.ShadowedPath)

	// Remove deletes the ignored copy and leaves the built-in in place.
	res, err := core.RemoveMod(t.Context(), core.RemoveModRequest{Name: "ItemValueBoost"})
	require.NoError(t, err)
	require.True(t, res.KeptEntry)
	require.NoFileExists(t, shadowed.ShadowedPath)

	after, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	require.Len(t, after.Mods, len(tweaks.Names()))
	for _, m := range after.Mods {
		require.False(t, m.Shadowed)
	}
}

// R1.4: a built-in cannot be removed. It is compiled in, so reconcile would put
// it straight back; disabling is the action that means what remove would mean.
func TestABuiltInCannotBeRemoved(t *testing.T) {
	bare(t)
	_, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)

	_, err = core.RemoveMod(t.Context(), core.RemoveModRequest{Name: "BigStacks"})
	require.ErrorIs(t, err, core.ErrBuiltInMod)
	require.ErrorContains(t, err, "disable it instead")
}

// Spec 010: a library script can be read and rewritten in place; the new text
// has to load, the previous text is kept beside the file, and a built-in is
// read-only.
func TestModScriptsAreReadAndRewrittenWithAGuardAndABackup(t *testing.T) {
	bare(t)
	lib := config.Defaults().Paths().Library
	require.NoError(t, os.MkdirAll(lib, 0o750))
	script := "NMS_MOD_DEFINITION_CONTAINER = {\n  MOD_FILENAME = \"Edit.pak\",\n  MOD_AUTHOR = \"t\",\n" +
		"  MODIFICATIONS = { { MBIN_CHANGE_TABLE = { { MBIN_FILE_SOURCE = \"GCGAMEPLAYGLOBALS.GLOBAL.MBIN\",\n" +
		"    EXML_CHANGE_TABLE = { { VALUE_CHANGE_TABLE = { {\"JetpackFillRate\", 999} } } } } } } }\n}\n"
	path := filepath.Join(lib, "Edit.lua")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o600))
	_, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)

	got, err := core.ReadModScript(t.Context(), core.ModScriptRequest{Name: "Edit"})
	require.NoError(t, err)
	require.Equal(t, script, got.Text)
	require.Equal(t, path, got.Path)
	require.False(t, got.ReadOnly)

	// Check: a verdict, nothing written.
	edited := strings.Replace(script, "999", "1234", 1)
	res, err := core.WriteModScript(t.Context(), core.WriteModScriptRequest{Name: "Edit", Text: edited, Check: true})
	require.NoError(t, err)
	require.True(t, res.Loads)
	require.Equal(t, 1, res.Blocks)
	require.False(t, res.Written)
	require.Equal(t, script, read(t, path))

	// Text that does not load is refused, and written with Force.
	broken := "NMS_MOD_DEFINITION_CONTAINER = {"
	_, err = core.WriteModScript(t.Context(), core.WriteModScriptRequest{Name: "Edit", Text: broken})
	require.ErrorContains(t, err, "does not load")
	require.Equal(t, script, read(t, path))
	res, err = core.WriteModScript(t.Context(), core.WriteModScriptRequest{Name: "Edit", Text: broken, Force: true})
	require.NoError(t, err)
	require.True(t, res.Written)
	require.False(t, res.Loads)
	require.Equal(t, broken, read(t, path))
	require.Equal(t, script, read(t, path+".bak"), "the previous text is kept")

	// A good edit lands, and the .bak now holds the broken text it replaced.
	res, err = core.WriteModScript(t.Context(), core.WriteModScriptRequest{Name: "Edit", Text: edited})
	require.NoError(t, err)
	require.True(t, res.Written)
	require.Equal(t, edited, read(t, path))
	require.Equal(t, broken, read(t, path+".bak"))
	list, err := core.ListMods(t.Context(), core.ListModsRequest{})
	require.NoError(t, err)
	for _, m := range list.Mods {
		require.NotContains(t, m.Name, ".bak", "the backup is not a mod")
	}

	// Identical text writes nothing.
	res, err = core.WriteModScript(t.Context(), core.WriteModScriptRequest{Name: "Edit", Text: edited})
	require.NoError(t, err)
	require.False(t, res.Written)

	// A built-in reads but does not write.
	bi, err := core.ReadModScript(t.Context(), core.ModScriptRequest{Name: list.Mods[0].Name})
	if list.Mods[0].Source == core.SourceBuiltin {
		require.NoError(t, err)
		require.True(t, bi.ReadOnly)
		require.NotEmpty(t, bi.Text)
		_, err = core.WriteModScript(t.Context(), core.WriteModScriptRequest{Name: list.Mods[0].Name, Text: "x"})
		require.ErrorIs(t, err, core.ErrReadOnlyScript)
	}
	_, err = core.ReadModScript(t.Context(), core.ModScriptRequest{Name: "Nope"})
	require.Error(t, err)
}
