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
	require.NoDirExists(t, mbin.Dir(tools, "v7.01.0-pre1"))
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
		dir := mbin.Dir(tools, tag)
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
	dir := mbin.Dir(tools, "v7.01.0-pre1")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "MBINCompiler-linux-dotnet10"), []byte("x"), 0o700))

	_, err := mbin.Locate(tools, "")
	require.ErrorIs(t, err, mbin.ErrNoCompiler)
}
