package modscript_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/modscript"
)

// script writes a mod script into a scratch directory and returns its path.
func script(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// container wraps one EXML_CHANGE_TABLE body in the smallest complete
// definition, so a test can be about the one thing it is testing.
func container(inner string) string {
	return `NMS_MOD_DEFINITION_CONTAINER = {
["MOD_FILENAME"] = "t.pak",
["MODIFICATIONS"] = { { ["MBIN_CHANGE_TABLE"] = { {
  ["MBIN_FILE_SOURCE"] = "GCGAMEPLAYGLOBALS.GLOBAL.MBIN",
  ["EXML_CHANGE_TABLE"] = { ` + inner + ` }
} } } }
}`
}

/*
R1.1: every backslash in the source is doubled before compiling.

Scripts spell game paths the Windows way, and "METADATA\REALITY\X.MBIN" is not
compilable Lua -- \R is an invalid escape. Without the doubling the whole
third-party library fails to load, which is exactly the failure the reference
dumper was written to avoid.
*/
func TestWindowsPathsInScriptsCompileBecauseBackslashesAreDoubled(t *testing.T) {
	def, err := modscript.Load(t.Context(), script(t, "paths.lua", `
NMS_MOD_DEFINITION_CONTAINER = {
["MODIFICATIONS"] = { { ["MBIN_CHANGE_TABLE"] = { {
  ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\REWARDTABLE.MBIN",
  ["EXML_CHANGE_TABLE"] = { { ["VALUE_CHANGE_TABLE"] = { {"A", 1} } } }
} } } }
}`))
	require.NoError(t, err)
	require.Equal(t, []string{`METADATA\REALITY\TABLES\REWARDTABLE.MBIN`},
		def.Modifications[0].Changes[0].Sources)
}

// A source list is as common as a single source, and the reference loader accepted
// both shapes for the same key. R1.3.
func TestOneChangeTableCanNameSeveralSourceFiles(t *testing.T) {
	def, err := modscript.Load(t.Context(), script(t, "multi.lua", `
NMS_MOD_DEFINITION_CONTAINER = {
["MODIFICATIONS"] = { { ["MBIN_CHANGE_TABLE"] = { {
  ["MBIN_FILE_SOURCE"] = { "A.MBIN", "B.MBIN" },
  ["EXML_CHANGE_TABLE"] = { { ["VALUE_CHANGE_TABLE"] = { {"A", 1} } } }
} } } }
}`))
	require.NoError(t, err)
	require.Equal(t, []string{"A.MBIN", "B.MBIN"}, def.Modifications[0].Changes[0].Sources)
}

// R1.2: a script that will not compile names the file, the stage and Lua's own
// message. "load failed" without the Lua message is not actionable.
func TestASyntaxErrorIsReportedAsALoadFailureWithLuasMessage(t *testing.T) {
	_, err := modscript.Load(t.Context(), script(t, "broken.lua", "NMS_MOD_DEFINITION_CONTAINER = {"))
	require.Error(t, err)

	var loadErr *modscript.LoadError
	require.ErrorAs(t, err, &loadErr)
	require.Equal(t, modscript.StageLoad, loadErr.Stage)
	require.Contains(t, err.Error(), "broken.lua")
	require.NotEmpty(t, loadErr.Message)
}

// A script that raises at run time is a different failure from one that will
// not compile, and the report says which. R1.2.
func TestAScriptThatRaisesIsReportedAsAnExecFailure(t *testing.T) {
	_, err := modscript.Load(t.Context(), script(t, "boom.lua", `error("nope")`))
	require.Error(t, err)

	var loadErr *modscript.LoadError
	require.ErrorAs(t, err, &loadErr)
	require.Equal(t, modscript.StageExec, loadErr.Stage)
	require.Contains(t, loadErr.Message, "nope")
}

// A .lua that runs cleanly but never assigns the container is not a mod. The
// reference dumper exited 5 for it; saying "no container" rather than "0 edits"
// is what tells the user they downloaded the wrong file. R1.2.
func TestAScriptThatNeverAssignsTheContainerIsRejected(t *testing.T) {
	_, err := modscript.Load(t.Context(), script(t, "empty.lua", `local x = 1`))
	require.Error(t, err)

	var loadErr *modscript.LoadError
	require.ErrorAs(t, err, &loadErr)
	require.Equal(t, modscript.StageNoContainer, loadErr.Stage)
}

/*
AC7 / R1.2: the sandbox.

A mod script is a third-party download. os and io are never opened and the
loaders that would let a script fetch more code are removed, so a script that
tries to run a command fails at `mods check` with a Lua error instead of
running it. The assertion that matters is the second one: the file the script
tried to create is not there.
*/
func TestAScriptCallingOsExecuteFailsInsteadOfRunningIt(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	_, err := modscript.Load(t.Context(), script(t, "evil.lua",
		`os.execute("touch `+marker+`")`+"\n"+container(`{ ["VALUE_CHANGE_TABLE"] = { {"A", 1} } }`)))
	require.Error(t, err)
	require.NoFileExists(t, marker, "the sandbox has to stop it, not just report it")

	var loadErr *modscript.LoadError
	require.ErrorAs(t, err, &loadErr)
	require.Equal(t, modscript.StageExec, loadErr.Stage)
}

// io.open is the other half of the same door: a script must not be able to read
// the user's files and stuff them into a MOD_DESCRIPTION. R1.2.
func TestAScriptCallingIoOpenFailsInsteadOfReadingTheDisk(t *testing.T) {
	_, err := modscript.Load(t.Context(), script(t, "peek.lua", `local f = io.open("/etc/passwd")`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "peek.lua")
}

// load/loadstring/dofile/require are removed after the base library registers
// them, which is easy to get wrong by opening the library and forgetting the
// second step. R1.2.
func TestTheCodeLoadingFunctionsAreGone(t *testing.T) {
	for _, name := range []string{"load", "loadstring", "dofile", "loadfile", "require"} {
		_, err := modscript.Load(t.Context(), script(t, name+".lua", name+`("return 1")`))
		require.Error(t, err, "%s must not be callable", name)
	}
}

// R1.2: a script with an endless loop stops at the deadline rather than hanging
// the build. gopher-lua checks the context between instructions.
func TestAnEndlessScriptStopsAtTheDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := modscript.Load(ctx, script(t, "spin.lua", `while true do end`))
	require.Error(t, err)
	require.Less(t, time.Since(started), 10*time.Second)
}

/*
R1.5: a script that builds its ADD payload with loops and string.rep.

Several real scripts generate hundreds of lines of XML this way rather than
writing them out, so table.insert, table.concat and string.rep have to be
present and the resulting multi-line string has to survive decoding intact.
*/
func TestAnAddPayloadBuiltWithLoopsAndStringRepSurvives(t *testing.T) {
	// Real scripts write their XML with [[long strings]] and literal tabs, not
	// with "\t" or \" escapes: the backslash doubling above makes every escape
	// sequence in a quoted string a literal backslash, so the library has always
	// been written without them.
	def, err := modscript.Load(t.Context(), script(t, "gen.lua", `
local parts = {}
local nl = string.char(10)
for i = 1, 3 do
  table.insert(parts, string.rep(string.char(9), 2) .. [[<Property value="]] .. i .. [[" />]])
end
`+container(`{ ["ADD"] = table.concat(parts, nl) }`)))
	require.NoError(t, err)

	blk := def.Modifications[0].Changes[0].Blocks[0]
	require.True(t, blk.HasAdd)
	require.Equal(t, []string{
		"\t\t<Property value=\"1\" />",
		"\t\t<Property value=\"2\" />",
		"\t\t<Property value=\"3\" />",
	}, strings.Split(blk.Add, "\n"))
}

/*
R1.3: integral numbers render as integers and fractional ones through Python's
repr.

This is the difference between writing value="5" and value="5.0" into an MXML
that MBINCompiler then has to accept, and between a report line reading
"x5" and "x5.0". The reference pipeline settled it by writing "%d" for integral
numbers under 1e15 and tostring() otherwise, then letting Python's json decide
int or float; the same split is reproduced here.
*/
func TestNumbersKeepTheIntegerFloatSplitTheReferencePipelineEstablished(t *testing.T) {
	def, err := modscript.Load(t.Context(), script(t, "nums.lua", container(
		`{ ["VALUE_CHANGE_TABLE"] = { {"Int", 5}, {"Whole", 10/2}, {"Frac", 0.5}, {"Tiny", 0.00001}, {"Str", "7"} } }`)))
	require.NoError(t, err)

	got := map[string]string{}
	for _, vc := range def.Modifications[0].Changes[0].Blocks[0].ValueChanges {
		got[vc.Key] = vc.Value.String()
	}
	require.Equal(t, map[string]string{
		"Int":   "5",
		"Whole": "5", // 10/2 is 5.0 in Lua and "%d" made it an int again
		"Frac":  "0.5",
		"Tiny":  "1e-05", // Python's repr, not Go's "1e-05" by accident
		"Str":   "7",
	}, got)
}

// PyRepr is the piece of the above that is easiest to get subtly wrong, so it
// is pinned directly against CPython's repr(). R1.3.
func TestPyReprMatchesPythonsFloatRendering(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{0, "0.0"}, {1, "1.0"}, {-2.5, "-2.5"}, {0.1, "0.1"},
		{0.0001, "0.0001"}, {0.00001, "1e-05"}, {1.5e-7, "1.5e-07"},
		{1e15, "1000000000000000.0"}, {1e16, "1e+16"}, {1.5e16, "1.5e+16"},
		{1e100, "1e+100"}, {1.0 / 3.0, "0.3333333333333333"},
	} {
		require.Equal(t, tc.want, modscript.PyRepr(tc.in), "repr(%v)", tc.in)
	}
}

// The tuning constants scripts put at the top level are what spec 004 turns
// into GUI fields; they have to survive loading. R1.3.
func TestTopLevelNumericGlobalsAreKept(t *testing.T) {
	def, err := modscript.Load(t.Context(), script(t, "knob.lua",
		"SPEED = 4\nNAME = \"x\"\n"+container(`{ ["VALUE_CHANGE_TABLE"] = { {"A", SPEED} } }`)))
	require.NoError(t, err)
	require.Equal(t, "4", def.Globals["SPEED"].String())
	require.NotContains(t, def.Globals, "NAME", "only numeric knobs")
}

/*
R2/R4: keys the engine does not interpret are recorded rather than dropped.

A mod relying on VALUE_MATCH gets the edit applied without the match, which may
be wrong. Listing the key is what lets the build report say so instead of the
user finding out in game.
*/
func TestKeysTheEngineIgnoresAreRecordedAsUnsupported(t *testing.T) {
	def, err := modscript.Load(t.Context(), script(t, "vm.lua", container(
		`{ ["VALUE_MATCH"] = "x", ["LINE_OFFSET"] = 2, ["VALUE_CHANGE_TABLE"] = { {"A", 1} } }`)))
	require.NoError(t, err)
	require.Equal(t, []string{"LINE_OFFSET", "VALUE_MATCH"},
		def.Modifications[0].Changes[0].Blocks[0].Unsupported)
	require.Equal(t, []string{"LINE_OFFSET", "VALUE_MATCH"}, def.UnsupportedKeys())
}

/*
R1.3: REMOVE = false removes nothing but still marks the block structural.

The reference builder tested truthiness to decide what to do and key presence to
decide what to retry without, and the two answers differ for exactly this block.
Collapsing them would change which mods a failed recompile drops.
*/
func TestRemoveFalseIsStillAStructuralBlock(t *testing.T) {
	def, err := modscript.Load(t.Context(), script(t, "rm.lua", container(
		`{ ["REMOVE"] = false, ["VALUE_CHANGE_TABLE"] = { {"A", 1} } }`)))
	require.NoError(t, err)

	blk := def.Modifications[0].Changes[0].Blocks[0]
	require.False(t, blk.Remove)
	require.True(t, blk.HasRemove)
	require.True(t, blk.Structural())
}

// R1.4: DumpJSON is the notation the golden Lua stage compares in, so it has to
// be parseable JSON carrying the decoded tree.
func TestDumpJSONReproducesTheContainer(t *testing.T) {
	def, err := modscript.Load(t.Context(), script(t, "dump.lua", container(
		`{ ["SPECIAL_KEY_WORDS"] = {"A","B"}, ["VALUE_CHANGE_TABLE"] = { {"K", 1.5} } }`)))
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(modscript.DumpJSON(def), &doc))
	require.Equal(t, "t.pak", doc["MOD_FILENAME"])

	mods, ok := doc["MODIFICATIONS"].([]any)
	require.True(t, ok)
	require.Len(t, mods, 1)
}

// An empty Lua table dumped as {} rather than [] would make the golden
// comparison fail on every script that carries one; the reference dumper wrote [].
func TestAnEmptyTableDumpsAsAnEmptyArray(t *testing.T) {
	def, err := modscript.Load(t.Context(), script(t, "et.lua", container(
		`{ ["PRECEDING_KEY_WORDS"] = {}, ["VALUE_CHANGE_TABLE"] = { {"A", 1} } }`)))
	require.NoError(t, err)
	require.Contains(t, string(modscript.DumpJSON(def)), `"PRECEDING_KEY_WORDS":[]`)
}
