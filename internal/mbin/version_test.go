package mbin_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/mbin"
)

// The tag pads the minor component ("v7.01.0") and the binary's own output does
// not ("v7.1.0.1"). Comparing either as a string puts 7.02 and 7.1 in different
// orders, so both are parsed into integers. R5.1.
func TestReleaseTagsParseIntoNumbersRatherThanStrings(t *testing.T) {
	for _, tc := range []struct {
		tag   string
		want  mbin.Version
		valid bool
	}{
		{"v7.02.0-pre1", mbin.Version{Major: 7, Minor: 2, Patch: 0, Pre: "pre1", Tag: "v7.02.0-pre1"}, true},
		{"v7.01.0-pre1", mbin.Version{Major: 7, Minor: 1, Patch: 0, Pre: "pre1", Tag: "v7.01.0-pre1"}, true},
		{"v6.45.0", mbin.Version{Major: 6, Minor: 45, Patch: 0, Tag: "v6.45.0"}, true},
		{"7.1.0", mbin.Version{Major: 7, Minor: 1, Patch: 0, Tag: "7.1.0"}, true},
		{"latest", mbin.Version{}, false},
		{"v7.1", mbin.Version{}, false},
		{"", mbin.Version{}, false},
	} {
		got, ok := mbin.ParseVersion(tc.tag)
		require.Equal(t, tc.valid, ok, tc.tag)
		if tc.valid {
			require.Equal(t, tc.want, got, tc.tag)
		}
	}
}

// Ordering decides which release `tools ensure` installs. A prerelease sorts
// below the release of the same number, and 7.2 above 7.10 would install a
// compiler two game versions out of date.
func TestVersionOrderingIsNumericAndPrereleasesSortLow(t *testing.T) {
	v := func(s string) mbin.Version {
		parsed, ok := mbin.ParseVersion(s)
		require.True(t, ok, s)
		return parsed
	}
	require.Negative(t, v("v7.01.0").Compare(v("v7.02.0")))
	require.Negative(t, v("v7.02.0").Compare(v("v7.10.0")))
	require.Negative(t, v("v7.02.0-pre1").Compare(v("v7.02.0")))
	require.Negative(t, v("v7.02.0-pre1").Compare(v("v7.02.0-pre2")))
	require.Positive(t, v("v7.02.1").Compare(v("v7.02.0")))
	require.Zero(t, v("v7.02.0").Compare(v("v7.02.0")))
	require.True(t, v("v7.01.0-pre1").SameMajorMinor(v("7.1.0")))
}

// MBINCompiler's output format is not documented: the binary prints
// "MBINCompiler v7.01.0-pre1" and a file's stamp is "Compiled with MBINCompiler
// v7.1.0.1", a four-part version. Reading the first triple out of arbitrary
// text is what keeps a format change from blocking a build. R6.1.
func TestTheVersionIsFoundInsideMBINCompilersUndocumentedOutput(t *testing.T) {
	got, ok := mbin.FindVersion("MBINCompiler v7.01.0-pre1")
	require.True(t, ok)
	require.Equal(t, "7.1.0", got.Numeric())

	got, ok = mbin.FindVersion("Compiled with MBINCompiler v7.1.0.1")
	require.True(t, ok)
	require.Equal(t, "7.1.0", got.Numeric(), "the trailing .1 is a build number, not the patch")

	_, ok = mbin.FindVersion("[ERROR]: Invalid file type.")
	require.False(t, ok, "an unparseable answer degrades to unknown, it does not become 0.0.0")
}

// R6.2: the status line the user reads. compiler-older is the case that matters
// -- a build with it is allowed but flagged, because libMBIN's structs are
// written against a specific game release.
func TestCompatibilityNamesTheThreeStatesAndUnknown(t *testing.T) {
	c, _ := mbin.ParseVersion("v7.01.0-pre1")
	require.Equal(t, mbin.CompatMatch, mbin.Compatibility(c, mustVersion(t, "7.1.0"), true, true))
	require.Equal(t, mbin.CompatOlder, mbin.Compatibility(c, mustVersion(t, "7.2.0"), true, true))
	require.Equal(t, mbin.CompatNewer, mbin.Compatibility(c, mustVersion(t, "6.45.0"), true, true))
	require.Equal(t, mbin.CompatUnknown, mbin.Compatibility(c, mbin.Version{}, true, false))
	require.Equal(t, mbin.CompatUnknown, mbin.Compatibility(mbin.Version{}, c, false, true))
}

func mustVersion(t *testing.T, s string) mbin.Version {
	t.Helper()
	v, ok := mbin.ParseVersion(s)
	require.True(t, ok)
	return v
}
