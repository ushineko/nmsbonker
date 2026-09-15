package mbin_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/mbin"
)

// assetServer serves the fake compiler as both flavors' assets. brokenBin makes
// the dotnet10 binary unrunnable, which is the fallback case R5.3 describes.
func assetServer(t *testing.T, brokenBin bool) (*httptest.Server, mbin.Release) {
	t.Helper()
	mux := http.NewServeMux()
	body := fakeScript
	mux.HandleFunc("/dotnet10/bin", func(w http.ResponseWriter, _ *http.Request) {
		if brokenBin {
			_, _ = w.Write([]byte("\x7fELF this is not a runnable binary"))
			return
		}
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/dotnet10/lib", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("stub")) })
	mux.HandleFunc("/sc/bin", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
	mux.HandleFunc("/sc/lib", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("stub")) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	version, ok := mbin.ParseVersion("v7.01.0-pre1")
	require.True(t, ok)
	return srv, mbin.Release{
		Tag:     "v7.01.0-pre1",
		Version: version,
		Assets: []mbin.Asset{
			{Name: "MBINCompiler-linux-dotnet10", URL: srv.URL + "/dotnet10/bin"},
			{Name: "libMBIN-linux-dotnet10.so", URL: srv.URL + "/dotnet10/lib"},
			{Name: "MBINCompiler-linux", URL: srv.URL + "/sc/bin"},
			{Name: "libMBIN-linux.so", URL: srv.URL + "/sc/lib"},
		},
	}
}

// AC3: the first `tools ensure` installs and verifies, the second says the
// release is already there. Verification is running the binary, because a
// download that produced an unrunnable file is indistinguishable from a good
// one until it is executed -- and otherwise that first execution happens in the
// middle of a build. R5.3.
func TestInstallDownloadsVerifiesAndIsANoOpTheSecondTime(t *testing.T) {
	mbin.SetMaxProcesses(2)
	srv, release := assetServer(t, false)
	tools := t.TempDir()

	got, err := mbin.Install(t.Context(), tools, release, mbin.FlavorDotnet10, srv.Client())
	require.NoError(t, err)
	require.False(t, got.AlreadyPresent)
	require.Equal(t, mbin.FlavorDotnet10, got.Flavor)
	require.FileExists(t, filepath.Join(tools, "mbincompiler", "v7.01.0-pre1", "MBINCompiler-linux-dotnet10"))
	require.FileExists(t, filepath.Join(tools, "mbincompiler", "v7.01.0-pre1", "libMBIN-linux-dotnet10.so"))

	fi, err := os.Stat(got.Compiler.Bin)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&0o100, "the binary has to be executable")

	again, err := mbin.Install(t.Context(), tools, release, mbin.FlavorDotnet10, srv.Client())
	require.NoError(t, err)
	require.True(t, again.AlreadyPresent)
}

// The self-contained asset's actual runtime requirements are undocumented and
// the framework-dependent one needs .NET 10, so "this flavor does not run here"
// is a normal outcome to route around and report, not a failure. R5.3.
func TestAFlavorThatWillNotRunFallsBackToTheOtherAndSaysSo(t *testing.T) {
	mbin.SetMaxProcesses(2)
	srv, release := assetServer(t, true)
	tools := t.TempDir()

	got, err := mbin.Install(t.Context(), tools, release, mbin.FlavorDotnet10, srv.Client())
	require.NoError(t, err)
	require.Equal(t, mbin.FlavorSelfContained, got.Flavor)
	require.Len(t, got.Attempts, 2)
	require.Contains(t, got.Attempts[0], "dotnet10")
	require.Contains(t, got.Attempts[1], "installed and verified")
}

// A directory holding half a download looks installed to anything that only
// stats the binary. Removing it on failure means a retry is a clean download
// rather than a permanently broken install the user has to find by hand. R5.3.
func TestAFailedVerificationLeavesNothingBehind(t *testing.T) {
	mbin.SetMaxProcesses(2)
	version, ok := mbin.ParseVersion("v7.01.0-pre1")
	require.True(t, ok)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("#!/bin/sh\nexit 3\n"))
	}))
	defer srv.Close()

	tools := t.TempDir()
	_, err := mbin.Install(t.Context(), tools, mbin.Release{
		Tag: "v7.01.0-pre1", Version: version,
		Assets: []mbin.Asset{
			{Name: "MBINCompiler-linux-dotnet10", URL: srv.URL},
			{Name: "libMBIN-linux-dotnet10.so", URL: srv.URL},
			{Name: "MBINCompiler-linux", URL: srv.URL},
			{Name: "libMBIN-linux.so", URL: srv.URL},
		},
	}, mbin.FlavorAuto, srv.Client())
	require.Error(t, err)
	dir, err := mbin.Dir(tools, "v7.01.0-pre1")
	require.NoError(t, err)
	require.NoDirExists(t, dir)
}

// Both assets are required: the framework-dependent binary loads libMBIN from
// beside itself, and a directory with only the executable fails at conversion
// time with a message that says nothing about a missing download. R5.3.
func TestAReleaseWithOnlyOneOfTheTwoAssetsIsRefused(t *testing.T) {
	mbin.SetMaxProcesses(2)
	srv, release := assetServer(t, false)
	release.Assets = release.Assets[:1]

	_, err := mbin.Install(t.Context(), t.TempDir(), release, mbin.FlavorDotnet10, srv.Client())
	require.Error(t, err)
}

// `nmsbonker pak extract ... && Decompile` and every build need a compiler
// without asking GitHub first. A pin that is not installed is an error rather
// than a silent fall-through: the pin exists because the user decided which
// compiler this game version needs. R5.6.
func TestLocateFindsTheHighestInstalledReleaseAndHonoursAPin(t *testing.T) {
	tools := t.TempDir()
	for _, tag := range []string{"v6.45.0", "v7.01.0-pre1", "v7.02.0-pre1", "not-a-version"} {
		dir := filepath.Join(tools, "mbincompiler", tag)
		require.NoError(t, os.MkdirAll(dir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "MBINCompiler-linux-dotnet10"), []byte("x"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "libMBIN-linux-dotnet10.so"), []byte("x"), 0o600))
	}

	require.Equal(t, []string{"v7.02.0-pre1", "v7.01.0-pre1", "v6.45.0"}, mbin.Installed(tools))

	c, err := mbin.Locate(tools, "")
	require.NoError(t, err)
	require.Equal(t, "v7.02.0-pre1", c.Tag)

	c, err = mbin.Locate(tools, "v6.45.0")
	require.NoError(t, err)
	require.Equal(t, "v6.45.0", c.Tag)

	_, err = mbin.Locate(tools, "v9.99.0")
	require.ErrorIs(t, err, mbin.ErrNoCompiler)

	_, err = mbin.Locate(t.TempDir(), "")
	require.ErrorIs(t, err, mbin.ErrNoCompiler)
}

// A tools directory holding only the binary, or only the library, is not an
// install. Reporting it as one produces a conversion failure in the middle of a
// build instead of "run tools ensure". R5.6.
func TestAHalfPopulatedToolsDirectoryIsNotAnInstall(t *testing.T) {
	tools := t.TempDir()
	dir, err := mbin.Dir(tools, "v7.01.0-pre1")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "MBINCompiler-linux-dotnet10"), []byte("x"), 0o700))

	_, err = mbin.Locate(tools, "")
	require.ErrorIs(t, err, mbin.ErrNoCompiler)
}

/*
A release tag becomes a directory name under the tools directory, and it comes
from api.github.com.

findVersion is deliberately lenient because MBINCompiler's own output has no
documented format, and that leniency accepts an M.m.p triple anywhere in a
string -- including "../../evil-1.2.3". Anything reaching a file path has to be
held to the stricter rule instead, or a repository anyone can publish a release
to gets to choose where this program writes. R5.3.
*/
func TestATagThatIsNotOneWholePathSegmentIsRefused(t *testing.T) {
	for _, tag := range []string{
		"v7.02.0-pre1", "v6.45.0", "7.1.0", "v7.02.0-rc.2",
	} {
		require.True(t, mbin.ValidTag(tag), tag)
		_, ok := mbin.ParseVersion(tag)
		require.True(t, ok, tag)
	}

	for _, tag := range []string{
		"../../evil-1.2.3",
		"v1.2.3/x",
		"x/v1.2.3",
		"..",
		".",
		"v1.2.3-..",
		"v1.2.3\x00",
		"release v1.2.3",
		"v1.2.3 ",
		"tooling-2026",
		"latest",
		"v7.1",
		"",
	} {
		require.False(t, mbin.ValidTag(tag), "ValidTag accepted %q", tag)
		_, ok := mbin.ParseVersion(tag)
		require.False(t, ok, "ParseVersion accepted %q", tag)
	}
}

// Dir is the choke point: every path under the tools directory goes through it,
// so it refuses a tag it cannot vouch for rather than sanitising one quietly. A
// silently rewritten path installs a release under a name no later lookup finds.
func TestDirRefusesATagItCannotVouchFor(t *testing.T) {
	tools := t.TempDir()

	dir, err := mbin.Dir(tools, "v7.02.0-pre1")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(tools, "mbincompiler", "v7.02.0-pre1"), dir)

	for _, tag := range []string{"../../evil-1.2.3", "v1.2.3/x", "..", ""} {
		_, err := mbin.Dir(tools, tag)
		require.ErrorIs(t, err, mbin.ErrInvalidTag, tag)
	}
}

// A hostile or merely odd release must not be installable at all: Install joins
// the tag into the tools directory before it downloads anything.
func TestInstallRefusesAReleaseWhoseTagIsNotAPathSegment(t *testing.T) {
	srv, release := assetServer(t, false)
	release.Tag = "../../evil-1.2.3"

	tools := t.TempDir()
	_, err := mbin.Install(t.Context(), tools, release, mbin.FlavorDotnet10, srv.Client())
	require.ErrorIs(t, err, mbin.ErrInvalidTag)
	require.NoDirExists(t, filepath.Join(tools, "mbincompiler"), "nothing was created")
}

// The tools tree is a directory anything on the machine can write to, so what
// comes out of it is filtered by the same rule as what goes in.
func TestInstalledSkipsDirectoriesThatAreNotValidTags(t *testing.T) {
	tools := t.TempDir()
	base := filepath.Join(tools, "mbincompiler")
	for _, name := range []string{"v7.02.0-pre1", "v6.45.0", "not-a-version", "v1.2.3 stray", "latest"} {
		require.NoError(t, os.MkdirAll(filepath.Join(base, name), 0o750))
	}
	require.Equal(t, []string{"v7.02.0-pre1", "v6.45.0"}, mbin.Installed(tools))

	_, err := mbin.Locate(tools, "../../evil-1.2.3")
	require.ErrorIs(t, err, mbin.ErrInvalidTag)
}

/*
Spec 007 AC8: a release that attaches mapping.json gets it installed beside the
compiler; one that does not still installs, with the absence reported as a
warning rather than a failure. An install that is already present fetches a
missing mapping on its own, which is how an existing install picks it up.
*/
func TestInstallFetchesTheSaveKeyMappingWhenTheReleaseHasOne(t *testing.T) {
	mbin.SetMaxProcesses(2)
	srv, release := assetServer(t, false)
	tools := t.TempDir()

	// Without the asset: installed, warned. Install itself never fetches the
	// mapping (so --no-network can skip it); InstallMapping does.
	got, err := mbin.Install(t.Context(), tools, release, mbin.FlavorDotnet10, srv.Client())
	require.NoError(t, err)
	require.Empty(t, got.Mapping)
	require.Contains(t, got.MappingWarning, "no save-key mapping is installed")
	mbin.InstallMapping(t.Context(), got, release, srv.Client())
	require.Empty(t, got.Mapping)
	require.Contains(t, got.MappingWarning, "mapping.json")
	require.NoFileExists(t, mbin.MappingPath(got.Compiler.Bin))

	// The release grows the asset; the already-present install picks it up.
	mapping := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"libMBIN_version":"7.1.0.1","Mapping":[{"Key":"F2P","Value":"Version"}]}`))
	}))
	t.Cleanup(mapping.Close)
	release.Assets = append(release.Assets, mbin.Asset{Name: mbin.MappingAsset, URL: mapping.URL + "/mapping.json"})
	got, err = mbin.Install(t.Context(), tools, release, mbin.FlavorDotnet10, srv.Client())
	require.NoError(t, err)
	require.True(t, got.AlreadyPresent)
	mbin.InstallMapping(t.Context(), got, release, srv.Client())
	require.Equal(t, mbin.MappingPath(got.Compiler.Bin), got.Mapping)
	require.Empty(t, got.MappingWarning)
	require.FileExists(t, got.Mapping)

	// Already there: Install reports it without any network.
	got, err = mbin.Install(t.Context(), tools, release, mbin.FlavorDotnet10, nil)
	require.NoError(t, err)
	require.Equal(t, mbin.MappingPath(got.Compiler.Bin), got.Mapping)

	// A download that is not a mapping document is removed and reported.
	tools2 := t.TempDir()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>rate limited</html>"))
	}))
	t.Cleanup(bad.Close)
	release.Assets[len(release.Assets)-1].URL = bad.URL
	got, err = mbin.Install(t.Context(), tools2, release, mbin.FlavorDotnet10, srv.Client())
	require.NoError(t, err)
	mbin.InstallMapping(t.Context(), got, release, srv.Client())
	require.Empty(t, got.Mapping)
	require.Contains(t, got.MappingWarning, "not a mapping document")
	require.NoFileExists(t, mbin.MappingPath(got.Compiler.Bin))
}
