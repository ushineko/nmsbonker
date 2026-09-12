/*
Package mbin acquires and drives MBINCompiler (spec 001 R5).

MBINCompiler (github.com/monkeyman192/MBINCompiler) is the only maintained
MBIN<->MXML converter, and libMBIN's struct definitions change with every game
release, so parsing MBIN in Go was rejected: it would be a fork of libMBIN that
had to be re-derived every patch. This package downloads the right release,
verifies it runs, and runs it.
*/
package mbin

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

/*
Version is a parsed MBINCompiler release version.

Release tags look like "v7.02.0-pre1", where major.minor tracks the game
version. The minor component is zero-padded in the tag and is not in the
binary's own output ("Compiled with MBINCompiler v7.1.0.1"), so both are parsed
into integers and compared numerically -- string comparison would put "7.02"
and "7.1" in different worlds.

The prerelease suffix is parsed but the upstream `prerelease` flag on the GitHub
release is not trusted: v7.01.0-pre1 is published as a full release.
*/
type Version struct {
	Major int
	Minor int
	Patch int
	// Pre is the suffix after the patch, without the dash ("pre1"), or "".
	Pre string
	// Tag is the release tag this came from, when it came from one.
	Tag string
}

// versionRE finds an M.m.p triple and any suffix attached to it. Anchored at a
// non-digit boundary so that "7.1.0.1" yields 7.1.0 and not 1.0.1.
var versionRE = regexp.MustCompile(`(?:^|[^0-9.])(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.\-]+))?`)

/*
tagRE is what a release tag must match, in full, to be used at all.

A tag arrives from api.github.com and becomes a directory name under the tools
directory, so it is untrusted input on a path. findVersion is deliberately
lenient -- it looks for an M.m.p triple *somewhere* in a string, which is what
reading MBINCompiler's undocumented output needs -- and that leniency accepts
"../../evil-1.2.3" and "v1.2.3/x" just as happily as "v7.02.0-pre1". Anchoring
the whole tag here is what keeps a repository that anyone can publish a release
to from choosing where this program writes.

The prerelease suffix must start with an alphanumeric, so "v1.2.3-.." is
refused: a segment of nothing but dots is not a version and has no business
being a directory name.
*/
var tagRE = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[A-Za-z0-9][A-Za-z0-9.]*)?$`)

// ValidTag reports whether a release tag is usable, both as a version and as a
// single path segment under the tools directory (R5.3).
func ValidTag(tag string) bool {
	if !tagRE.MatchString(tag) {
		return false
	}
	// Defence in depth: tagRE already forbids a separator, but the property
	// that actually matters at every call site is "this is one path element",
	// so it is asserted rather than inferred.
	return filepath.Base(tag) == tag && tag != "." && tag != ".."
}

// ParseVersion reads a release tag such as "v7.02.0-pre1".
//
// A tag that is not a well-formed vM.m.p is ignored rather than rejected
// (R5.1): the repository has carried tags for tooling and branches, and one of
// those must not stop the tool from finding the release it needs. It is ignored
// for the second reason too -- see ValidTag -- which is that a tag it cannot
// vouch for must never reach a file path.
func ParseVersion(tag string) (Version, bool) {
	if !ValidTag(tag) {
		return Version{}, false
	}
	v, ok := findVersion(tag)
	if !ok {
		return Version{}, false
	}
	v.Tag = tag
	return v, true
}

// findVersion pulls the first M.m.p triple out of arbitrary text (R6.1).
//
// Used on MBINCompiler's own output, which is not a documented format: the
// binary says "MBINCompiler v7.01.0-pre1" and a file says "Compiled with
// MBINCompiler v7.1.0.1". Leniency here is deliberate -- a version that cannot
// be read degrades to "unknown" and never blocks a build.
func findVersion(s string) (Version, bool) {
	m := versionRE.FindStringSubmatch(s)
	if m == nil {
		return Version{}, false
	}
	major, err1 := strconv.Atoi(m[1])
	minor, err2 := strconv.Atoi(m[2])
	patch, err3 := strconv.Atoi(m[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return Version{}, false
	}
	return Version{Major: major, Minor: minor, Patch: patch, Pre: m[4]}, true
}

// FindVersion is findVersion, exported for the game-data version reader (R6.1)
// and its tests.
func FindVersion(s string) (Version, bool) { return findVersion(s) }

// Numeric is the "M.m.p" form, without the tag's zero padding or suffix.
func (v Version) Numeric() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// String is the version as a human reads it.
func (v Version) String() string {
	if v.Pre != "" {
		return v.Numeric() + "-" + v.Pre
	}
	return v.Numeric()
}

// SameMajorMinor reports whether two versions track the same game release,
// which is the compatibility test the whole selection turns on (R5.2, R6.2).
func (v Version) SameMajorMinor(o Version) bool {
	return v.Major == o.Major && v.Minor == o.Minor
}

// Compare orders two versions: negative if v sorts before o.
//
// A prerelease sorts before the release of the same number, as semver has it.
// Between two prereleases the suffix is compared as a string, which is enough
// for "pre1" < "pre2" and is not worth more: the selection only ever needs the
// highest, and two prereleases of the same patch are hours apart.
func (v Version) Compare(o Version) int {
	for _, pair := range [][2]int{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case v.Pre == o.Pre:
		return 0
	case v.Pre == "":
		return 1
	case o.Pre == "":
		return -1
	}
	return strings.Compare(v.Pre, o.Pre)
}

// Compatibility describes how an installed compiler relates to the game's data
// version (R6.2).
const (
	CompatMatch    = "match"
	CompatOlder    = "compiler-older"
	CompatNewer    = "compiler-newer"
	CompatUnknown  = "unknown"
	VersionUnknown = "unknown"
)

// Compatibility compares a compiler version with the game data version.
func Compatibility(compiler, game Version, haveCompiler, haveGame bool) string {
	if !haveCompiler || !haveGame {
		return CompatUnknown
	}
	switch {
	case compiler.SameMajorMinor(game):
		return CompatMatch
	case compiler.Compare(game) < 0:
		return CompatOlder
	default:
		return CompatNewer
	}
}
