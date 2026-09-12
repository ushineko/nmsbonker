package tweaks_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/tweaks"
)

// R1.1: every embedded script is in the default order and vice versa. A script
// embedded but not listed ships in the binary and is reachable from nowhere;
// one listed but not embedded is a name the interface offers and cannot load.
func TestEveryEmbeddedScriptIsListedAndEveryListedScriptIsEmbedded(t *testing.T) {
	files := tweaks.Files()
	require.Len(t, files, 10, "the built-in set is ten scripts")

	listed := map[string]bool{}
	for _, n := range tweaks.Names() {
		listed[n] = true
		_, ok := tweaks.Source(n)
		require.Truef(t, ok, "%s is in the default order but not embedded", n)
	}
	for _, f := range files {
		require.Truef(t, listed[f], "%s is embedded but not in the default order", f)
	}
}

/*
R1.1/R1.2: each built-in declares a header and parameters that match its code.

The header is what the Tweaks section draws from and the parameters are what it
writes to, so a script whose @param names a global it does not assign would show
a slider that changes nothing. Checking it here rather than in the GUI is the
difference between a test and a bug report.
*/
func TestEveryBuiltInDeclaresAHeaderAndUsableParameters(t *testing.T) {
	groups := map[string]bool{tweaks.GroupOther: true}
	for _, g := range tweaks.Groups {
		groups[g] = true
	}

	for _, tw := range tweaks.All() {
		t.Run(tw.Name, func(t *testing.T) {
			require.NotEmpty(t, tw.Header.Name)
			require.NotEqual(t, tweaks.GroupOther, tw.Header.Group,
				"every built-in belongs to one of the declared groups")
			require.True(t, groups[tw.Header.Group])
			require.NotEmpty(t, tw.Header.Desc, "the card needs something to say")
			require.NotEmpty(t, tw.Params)

			src, ok := tweaks.Source(tw.Name)
			require.True(t, ok)
			require.Empty(t, modscript.DuplicateAssignments(src),
				"a parameter assigned twice would ignore the user's override")

			for _, p := range tw.Params {
				require.NotEmpty(t, p.Label)
				require.True(t, p.Bounded, "a built-in declares its bounds")
				require.Greater(t, p.Max, p.Min)
				require.GreaterOrEqual(t, p.Default, p.Min)
				require.LessOrEqual(t, p.Default, p.Max)
				require.Equal(t, p.Default, p.Current)

				// The override path has to find the assignment, or the value
				// the user sets is silently dropped.
				_, err := modscript.Override(src, p.Name, p.Default, p.Kind)
				require.NoErrorf(t, err, "no assignment to substitute for %s", p.Name)
			}
		})
	}
}

// R1.1: the headers are comments, so every built-in still loads as a script and
// still says who wrote it. A script that will not load is a built-in that turns
// into a red row in the mod table on a fresh install.
func TestEveryBuiltInLoadsThroughTheSandbox(t *testing.T) {
	for _, name := range tweaks.Names() {
		src, ok := tweaks.Source(name)
		require.True(t, ok)
		def, err := modscript.LoadSource(t.Context(), name+".lua", src)
		require.NoErrorf(t, err, "%s does not load", name)
		require.Equal(t, "nmsbonker", def.Author, "%s", name)
		require.NotEmpty(t, def.Targets(), "%s edits nothing", name)
	}
}

// Source hands out a copy. The caller rewrites parameter assignments in the
// bytes it gets back, and the embedded copy has to survive that for the next
// build.
func TestSourceIsACopy(t *testing.T) {
	first, ok := tweaks.Source("ItemValueBoost")
	require.True(t, ok)
	first[0] = 'X'
	second, ok := tweaks.Source("ItemValueBoost")
	require.True(t, ok)
	require.NotEqual(t, byte('X'), second[0])
}
