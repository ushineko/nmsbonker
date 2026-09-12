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
			src, ok := tweaks.Source(name)
			require.True(t, ok)
			mine, err := modscript.LoadSource(t.Context(), name+".lua", src)
			require.NoError(t, err)

			path := filepath.Join(reference, "lua-src", name+".lua")
			theirs, err := modscript.Load(t.Context(), path)
			require.NoErrorf(t, err, "the reference copy of %s does not load", name)

			require.JSONEq(t, modifications(t, theirs), modifications(t, mine),
				"the headers changed what %s means", name)

			// And the one field that did change, changed to the right thing.
			require.Equal(t, "nmsbonker", mine.Author)
		})
	}
}

// modifications extracts the MODIFICATIONS subtree from a decoded definition,
// in the dumper's JSON notation.
func modifications(t *testing.T, def *modscript.Definition) string {
	t.Helper()
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(modscript.DumpJSON(def), &doc))
	raw, ok := doc["MODIFICATIONS"]
	require.True(t, ok, "the script declares no MODIFICATIONS")
	return string(raw)
}
