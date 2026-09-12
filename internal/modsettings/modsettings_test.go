package modsettings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/modsettings"
)

// The game's own file, byte for byte: a UTF-8 BOM, CRLF endings, tabs, and one
// property this build does not know the meaning of.
func gameFile(disableAll, enabled string) []byte {
	doc := "\xef\xbb\xbf" + strings.Join([]string{
		`<?xml version="1.0" encoding="utf-8"?>`,
		`<Data template="GcModSettings">`,
		"\t" + `<Property name="DisableAllMods" value="` + disableAll + `" />`,
		"\t" + `<Property name="SomeFutureSetting" value="42" />`,
		"\t" + `<Property name="Data">`,
		"\t\t" + `<Property name="Data" value="GcModSettingsInfo" _index="0">`,
		"\t\t\t" + `<Property name="Name" value="COSMOS COMBINE" />`,
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
	return []byte(doc)
}

// R3.1: a document that needs no change comes back byte-identical. Everything
// else in this package rests on that: an edit is measured as the difference
// from the file the game wrote, and a round trip that reformats has already
// taken ownership of a file this tool does not own.
func TestAnUneditedDocumentRoundTripsByteForByte(t *testing.T) {
	in := gameFile("false", "true")
	f := modsettings.Parse(in)
	require.Equal(t, in, f.Bytes())

	require.False(t, f.DisableAllMods())
	entries := f.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, "COSMOS COMBINE", entries[0].Name)
	require.True(t, entries[0].Enabled)
	require.True(t, entries[0].EnabledVR)
}

/*
AC5: flipping a value changes that value and nothing else.

Compared line by line rather than by looking for a substring: the BOM, the tab
indentation, the CRLF endings and the property this build has never heard of all
have to survive, and asserting that only one line differs is the only assertion
that actually says so.
*/
func TestFlippingDisableAllModsChangesOneLine(t *testing.T) {
	before := gameFile("true", "true")
	f := modsettings.Parse(before)
	require.True(t, f.DisableAllMods())
	require.True(t, f.SetDisableAllMods(false))
	after := f.Bytes()

	require.Equal(t, diffLines(t, before, after),
		[]string{`	<Property name="DisableAllMods" value="false" />`})
	require.True(t, strings.HasPrefix(string(after), "\xef\xbb\xbf"), "the BOM survives")
	require.Contains(t, string(after), "\r\n", "and so do the CRLF endings")
	require.Contains(t, string(after), `name="SomeFutureSetting" value="42"`,
		"a property this build does not understand is left where the game put it")

	// Setting it to what it already is changes nothing at all.
	require.False(t, f.SetDisableAllMods(false))
	require.Equal(t, after, f.Bytes())
}

// R3.2: an entry that is present but switched off is switched on, and only its
// two switches move.
func TestEnablingAnExistingEntryTouchesOnlyItsSwitches(t *testing.T) {
	before := gameFile("false", "false")
	f := modsettings.Parse(before)
	changed, added, err := f.EnableMod("COSMOS COMBINE")
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, added)

	require.Equal(t, []string{
		"\t\t\t" + `<Property name="Enabled" value="true" />`,
		"\t\t\t" + `<Property name="EnabledVR" value="true" />`,
	}, diffLines(t, before, f.Bytes()))

	// Idempotent: a second deploy must not rewrite the file.
	after := f.Bytes()
	changed, _, err = f.EnableMod("COSMOS COMBINE")
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, after, f.Bytes())
}

// R3.2: a mod the game has never seen gets an entry, in the game's own shape,
// with the _index the list has reached.
func TestEnablingAnUnknownModAddsAnEntry(t *testing.T) {
	f := modsettings.Parse(gameFile("false", "true"))
	changed, added, err := f.EnableMod("SOMETHING ELSE")
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, added)

	text := string(f.Bytes())
	require.Contains(t, text, `<Property name="Data" value="GcModSettingsInfo" _index="1">`)
	require.Contains(t, text, `<Property name="Name" value="SOMETHING ELSE" />`)
	require.Contains(t, text, `<Property name="Dependencies" />`)

	// Re-parsed, the game's original entry is still there and both are on.
	again := modsettings.Parse(f.Bytes())
	require.Len(t, again.Entries(), 2)
	for _, e := range again.Entries() {
		require.Truef(t, e.Enabled, "%s", e.Name)
		require.Truef(t, e.EnabledVR, "%s", e.Name)
	}
	require.Equal(t, "\r\n", lineEnding(f.Bytes()), "the new lines match the document")
}

// The game writes a self-closing Data property while it knows about no mods at
// all. That has to become a list before an entry can go in it.
func TestAnEmptyModListBecomesAList(t *testing.T) {
	doc := "\xef\xbb\xbf" + strings.Join([]string{
		`<Data template="GcModSettings">`,
		"\t" + `<Property name="DisableAllMods" value="false" />`,
		"\t" + `<Property name="Data" />`,
		`</Data>`,
		"",
	}, "\n")
	f := modsettings.Parse([]byte(doc))
	require.Empty(t, f.Entries())

	_, added, err := f.EnableMod("COSMOS COMBINE")
	require.NoError(t, err)
	require.True(t, added)

	again := modsettings.Parse(f.Bytes())
	require.Len(t, again.Entries(), 1)
	require.Equal(t, "COSMOS COMBINE", again.Entries()[0].Name)
	require.False(t, again.DisableAllMods())
}

// A mod folder name with an ampersand in it must not produce a settings file
// the game cannot parse.
func TestAModNameIsEscaped(t *testing.T) {
	f := modsettings.Parse(gameFile("false", "true"))
	_, _, err := f.EnableMod(`Bits & Bobs`)
	require.NoError(t, err)
	require.Contains(t, string(f.Bytes()), `value="Bits &amp; Bobs"`)
}

// Reading a file the game has never written is reported as its own condition,
// not as a failure: deploy says so and carries on.
func TestReadingAMissingFileIsItsOwnAnswer(t *testing.T) {
	_, err := modsettings.Read(filepath.Join(t.TempDir(), "nope.MXML"))
	require.ErrorIs(t, err, modsettings.ErrNoFile)
}

// Write replaces the file in one step, leaving no temporary behind.
func TestWriteReplacesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GCMODSETTINGS.MXML")
	require.NoError(t, os.WriteFile(path, gameFile("true", "true"), 0o600))

	f, err := modsettings.Read(path)
	require.NoError(t, err)
	require.True(t, f.SetDisableAllMods(false))
	require.NoError(t, f.Write(path))

	back, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, f.Bytes(), back)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary file left behind")
}

// diffLines is the lines that differ between two documents, taken from the
// second. Both are expected to have the same number of lines.
func diffLines(t *testing.T, before, after []byte) []string {
	t.Helper()
	a := strings.Split(strings.TrimPrefix(string(before), "\xef\xbb\xbf"), "\r\n")
	b := strings.Split(strings.TrimPrefix(string(after), "\xef\xbb\xbf"), "\r\n")
	require.Equal(t, len(a), len(b), "the line count changed")
	var out []string
	for i := range a {
		if a[i] != b[i] {
			out = append(out, b[i])
		}
	}
	return out
}

func lineEnding(b []byte) string {
	if strings.Contains(string(b), "\r\n") {
		return "\r\n"
	}
	return "\n"
}
