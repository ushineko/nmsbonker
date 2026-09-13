package tweaks_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/tweaks"
)

/*
R1.5: the embedded built-ins still decode to what the reference copies decode to.

The scripts in this package were copied out of the reference pipeline's lua-src
and given `-- @tweak` / `-- @param` / `-- @desc` headers. Those are comments, so
the claim is that they changed nothing the engine reads — and a claim like that
is worth a test rather than an assurance, because a stray character inside a
change table would produce a subtly different mod that still builds.

MODIFICATIONS only, deliberately. One field *was* changed on purpose:
MOD_AUTHOR, which said who assembled the reference copies and now says
"nmsbonker", since these are the project's own scripts. It is metadata — it
names the author in the build report and nothing else reads it — so comparing
the whole container would fail on the one difference this test is not about.
Everything the edit engine acts on is under MODIFICATIONS, and that is compared
in full.

Two further differences are expected and are removed before the comparison
rather than being allowed to weaken it (spec 005 R2.4). The reference copies
predate the CAP key, so every CAP is stripped from both sides: a cap changes
what a block produces only when the result would exceed it, which is exactly
what the golden merge proves it does not do at default values. And BigStacks
gained an antimatter-harvester edit the reference copies have no equivalent of;
it is switched off for this comparison by setting its parameter to 0, which also
exercises R2.5's "0 leaves the game's own value alone" branch.

Skips unless NMSBONKER_REFERENCE_DIR points at the reference pipeline; the
reference scripts are not committed here.
*/
func TestBuiltInsDecodeAsTheReferenceCopiesDo(t *testing.T) {
	reference := os.Getenv("NMSBONKER_REFERENCE_DIR")
	if reference == "" {
		t.Skip("NMSBONKER_REFERENCE_DIR is unset; skipping the built-in parity test")
	}

	for _, name := range tweaks.Names() {
		t.Run(name, func(t *testing.T) {
			if projectAuthored[name] {
				t.Skip("written for this project; there is no reference copy to decode against")
			}
			src, ok := tweaks.Source(name)
			require.True(t, ok)
			if param, off := switchedOff[name]; off {
				stock, err := modscript.Override(src, param, 0, modscript.ParamInt)
				require.NoErrorf(t, err, "%s no longer declares %s", name, param)
				src = stock
			}
			requireDecodesAsReference(t, reference, name, src)
		})
	}
}

/*
switchedOff names a parameter to set to 0 for the comparison, because at its
default it adds an edit the reference copies predate (spec 005 R2.4, R2.5).

One entry, and it is the antimatter-harvester cap: BigStacks now edits a second
file, which the library script this project retired used to edit. Zeroing the
parameter removes that change-table entry entirely, which is both what makes the
rest of the script comparable and a live check that 0 really does mean "leave
the game's own value alone".
*/
//
//nolint:gochecknoglobals // a fixed lookup, read-only after initialisation
var switchedOff = map[string]string{
	"BigStacks": "ANTIMATTER_HARVESTER_CAP",
}

// projectAuthored names the built-ins that never had a reference copy: they
// were written here (spec 006), so there is nothing for them to decode "as".
// The other ten fail loudly when their copy is missing, and should.
//
//nolint:gochecknoglobals // a fixed lookup, read-only after initialisation
var projectAuthored = map[string]bool{
	"NexusRewards":        true,
	"MissionBoardRewards": true,
}

// requireDecodesAsReference is the comparison itself: the embedded script and
// the reference copy must decode to the same MODIFICATIONS.
func requireDecodesAsReference(t *testing.T, reference, name string, src []byte) {
	t.Helper()
	mine, err := modscript.LoadSource(t.Context(), name+".lua", src)
	require.NoError(t, err)

	path := filepath.Join(reference, "lua-src", name+".lua")
	theirs, err := modscript.Load(t.Context(), path)
	require.NoErrorf(t, err, "the reference copy of %s does not load", name)

	require.JSONEq(t, modifications(t, theirs), modifications(t, mine),
		"the headers changed what %s means", name)

	// And the one field that did change, changed to the right thing.
	require.Equal(t, "nmsbonker", mine.Author)
}

/*
modifications extracts the MODIFICATIONS subtree from a decoded definition, in
the dumper's JSON notation and with every CAP key removed (spec 005 R2.4).

Stripping rather than expecting: the reference dump was captured before the key
existed, so a CAP on this side is not a difference in what the script means to
the reference engine. What a cap actually does is proved by the engine's own
tests, and the golden suite is unchanged because no reference script sets one.
*/
func modifications(t *testing.T, def *modscript.Definition) string {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(modscript.DumpJSON(def), &doc))
	raw, ok := doc["MODIFICATIONS"]
	require.True(t, ok, "the script declares no MODIFICATIONS")
	out, err := json.Marshal(stripCaps(raw))
	require.NoError(t, err)
	return string(out)
}

// stripCaps deletes every "CAP" key from a decoded tree.
func stripCaps(node any) any {
	switch v := node.(type) {
	case map[string]any:
		delete(v, "CAP")
		for k, child := range v {
			v[k] = stripCaps(child)
		}
		return v
	case []any:
		for i, child := range v {
			v[i] = stripCaps(child)
		}
		return v
	default:
		return node
	}
}
