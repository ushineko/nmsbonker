package mbin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/mbin"
)

// A trimmed copy of the shape api.github.com returns. Deliberately unsorted and
// carrying a tag that does not parse, because both occur upstream.
const releaseListing = `[
  {"tag_name": "v6.45.0", "prerelease": false, "assets": [
     {"name": "MBINCompiler-linux", "browser_download_url": "http://example/6.45/bin", "size": 1},
     {"name": "libMBIN-linux.so", "browser_download_url": "http://example/6.45/lib", "size": 1}]},
  {"tag_name": "v7.02.0-pre1", "prerelease": false, "assets": [
     {"name": "MBINCompiler-linux-dotnet10", "browser_download_url": "http://example/7.02/bin", "size": 1},
     {"name": "libMBIN-linux-dotnet10.so", "browser_download_url": "http://example/7.02/lib", "size": 1}]},
  {"tag_name": "tooling-2026", "prerelease": false, "assets": []},
  {"tag_name": "v7.01.0-pre1", "prerelease": false, "assets": [
     {"name": "MBINCompiler-linux-dotnet10", "browser_download_url": "http://example/7.01/bin", "size": 1},
     {"name": "libMBIN-linux-dotnet10.so", "browser_download_url": "http://example/7.01/lib", "size": 1},
     {"name": "MBINCompiler-linux", "browser_download_url": "http://example/7.01/bin-sc", "size": 1},
     {"name": "libMBIN-linux.so", "browser_download_url": "http://example/7.01/lib-sc", "size": 1}]}
]`

// serveListing returns a client pointed at a fixture, plus a counter of how
// many times the fixture was actually fetched.
func serveListing(t *testing.T, cachePath string) (*mbin.Client, *int) {
	t.Helper()
	fetches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/monkeyman192/MBINCompiler/releases", r.URL.Path)
		require.Equal(t, "30", r.URL.Query().Get("per_page"))
		require.NotEmpty(t, r.Header.Get("User-Agent"), "GitHub rejects a request with no User-Agent")
		if r.Header.Get("If-None-Match") == `"etag-1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fetches++
		w.Header().Set("ETag", `"etag-1"`)
		_, _ = w.Write([]byte(releaseListing))
	}))
	t.Cleanup(srv.Close)
	return &mbin.Client{
		APIBase:   srv.URL,
		CachePath: cachePath,
		UserAgent: "nmsbonker/test",
	}, &fetches
}

// Tags that do not parse are published upstream, and the listing does not
// arrive in version order. Both would otherwise decide which compiler gets
// installed. R5.1.
func TestTheReleaseListingIsSortedAndUnparseableTagsAreDropped(t *testing.T) {
	client, _ := serveListing(t, filepath.Join(t.TempDir(), "releases.json"))

	releases, source, err := client.Releases(context.Background())
	require.NoError(t, err)
	require.Equal(t, mbin.SourceNetwork, source)
	require.Len(t, releases, 3, "tooling-2026 has no vM.m.p prefix and is ignored")
	require.Equal(t, "v7.02.0-pre1", releases[0].Tag, "highest first")
	require.Equal(t, "v6.45.0", releases[2].Tag)
}

// The unauthenticated GitHub rate limit is 60 requests an hour. A conditional
// request costs nothing against it, so `status` can list releases as often as
// it likes -- but only if the ETag is actually sent and the 304 handled. R5.1.
func TestASecondListingIsAConditionalRequestThatCostsNothing(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "releases.json")
	client, fetches := serveListing(t, cache)

	_, _, err := client.Releases(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, *fetches)

	releases, source, err := client.Releases(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, *fetches, "the second call was answered 304")
	require.Equal(t, mbin.SourceNotModified, source)
	require.Len(t, releases, 3, "and still produced the full listing, from the cache")
}

// A laptop with no connection must still be able to install from what it
// already knows, and be told the answer is stale. Failing outright here would
// make `tools ensure` useless exactly when the tools directory is already
// populated. R5.1.
func TestAnUnreachableGitHubFallsBackToTheCacheWithAWarning(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "releases.json")
	client, _ := serveListing(t, cache)
	_, _, err := client.Releases(context.Background())
	require.NoError(t, err)

	offline := &mbin.Client{APIBase: "http://127.0.0.1:1", CachePath: cache, UserAgent: "nmsbonker/test"}
	releases, source, warning := offline.Releases(context.Background())
	require.Error(t, warning, "the caller is told the listing is stale")
	require.Equal(t, mbin.SourceCacheOffline, source)
	require.Len(t, releases, 3)

	// --no-network never dials at all, and says so when there is no cache.
	blocked := &mbin.Client{APIBase: "http://127.0.0.1:1", CachePath: cache, UserAgent: "nmsbonker/test", NoNetwork: true}
	releases, source, err = blocked.Releases(context.Background())
	require.NoError(t, err)
	require.Equal(t, mbin.SourceCacheOffline, source)
	require.Len(t, releases, 3)

	empty := &mbin.Client{APIBase: "http://127.0.0.1:1", CachePath: filepath.Join(t.TempDir(), "x.json"),
		UserAgent: "nmsbonker/test", NoNetwork: true}
	_, _, err = empty.Releases(context.Background())
	require.ErrorIs(t, err, mbin.ErrNoNetwork)
}

// A token is honoured from the environment and must not reach disk: the cache
// file is world-readable configuration, and a token in it would outlive the
// session that set it. R5.1.
func TestATokenIsSentButNeverWrittenToTheCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "releases.json")
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.Header().Set("ETag", `"etag-1"`)
		_, _ = w.Write([]byte(releaseListing))
	}))
	defer srv.Close()

	client := &mbin.Client{APIBase: srv.URL, CachePath: cache, UserAgent: "nmsbonker/test", Token: "ghp_secret"}
	_, _, err := client.Releases(context.Background())
	require.NoError(t, err)
	require.Equal(t, "Bearer ghp_secret", seen)

	b, err := os.ReadFile(cache)
	require.NoError(t, err)
	require.NotContains(t, string(b), "ghp_secret")
}

// Selection is what pairs the compiler with the game. libMBIN's struct
// definitions are written against a game release, so a compiler from a
// different major.minor decompiles to plausible MXML with the wrong fields.
// R5.2.
func TestSelectionPrefersTheGameVersionAndFallsBackToTheHighest(t *testing.T) {
	client, _ := serveListing(t, filepath.Join(t.TempDir(), "releases.json"))
	releases, _, err := client.Releases(context.Background())
	require.NoError(t, err)

	sel, err := mbin.Select(releases, "", mustVersion(t, "7.1.0"), true)
	require.NoError(t, err)
	require.Equal(t, "v7.01.0-pre1", sel.Release.Tag)
	require.Contains(t, sel.Reason, "7.1.0")

	sel, err = mbin.Select(releases, "", mbin.Version{}, false)
	require.NoError(t, err)
	require.Equal(t, "v7.02.0-pre1", sel.Release.Tag, "no compiler yet means no game version to match")
	require.Contains(t, sel.Reason, "unknown")

	sel, err = mbin.Select(releases, "", mustVersion(t, "9.9.0"), true)
	require.NoError(t, err)
	require.Equal(t, "v7.02.0-pre1", sel.Release.Tag)
	require.Contains(t, sel.Reason, "no release matches")

	sel, err = mbin.Select(releases, "v6.45.0", mustVersion(t, "7.1.0"), true)
	require.NoError(t, err)
	require.Equal(t, "v6.45.0", sel.Release.Tag, "a pin outranks the game version")

	_, err = mbin.Select(releases, "v9.99.0", mbin.Version{}, false)
	require.Error(t, err, "a pin that does not exist is an error, not a silent fallback")

	_, err = mbin.Select(nil, "", mbin.Version{}, false)
	require.ErrorIs(t, err, mbin.ErrNoReleases)
}

/*
The release listing is untrusted input that turns into a directory name.

Anyone can publish a release to a public repository, and the tag goes straight
into tools_dir/mbincompiler/<tag>. A tag that is not one clean path segment is
dropped on the way in, before anything can join it to a path, and reported at
debug level so `-v` shows why a release the user can see on GitHub was ignored.
R5.1, R5.3.
*/
func TestATraversalTagIsDroppedFromTheListingAndReported(t *testing.T) {
	const hostile = `[
  {"tag_name": "../../evil-1.2.3", "prerelease": false, "assets": [
     {"name": "MBINCompiler-linux-dotnet10", "browser_download_url": "http://example/evil/bin", "size": 1},
     {"name": "libMBIN-linux-dotnet10.so", "browser_download_url": "http://example/evil/lib", "size": 1}]},
  {"tag_name": "v7.02.0-pre1/x", "prerelease": false, "assets": []},
  {"tag_name": "v7.02.0-pre1", "prerelease": false, "assets": [
     {"name": "MBINCompiler-linux-dotnet10", "browser_download_url": "http://example/7.02/bin", "size": 1},
     {"name": "libMBIN-linux-dotnet10.so", "browser_download_url": "http://example/7.02/lib", "size": 1}]}
]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(hostile))
	}))
	defer srv.Close()

	var debug []string
	client := &mbin.Client{
		APIBase:   srv.URL,
		CachePath: filepath.Join(t.TempDir(), "releases.json"),
		UserAgent: "nmsbonker/test",
		Debug:     func(msg string) { debug = append(debug, msg) },
	}

	releases, _, err := client.Releases(context.Background())
	require.NoError(t, err)
	require.Len(t, releases, 1, "only the well-formed tag survives")
	require.Equal(t, "v7.02.0-pre1", releases[0].Tag)
	require.Len(t, debug, 2, "and each ignored tag is reported")
	require.Contains(t, strings.Join(debug, "\n"), "../../evil-1.2.3")
	require.Contains(t, strings.Join(debug, "\n"), "v7.02.0-pre1/x")

	// The one that survived is also the only one Select can hand to Install.
	sel, err := mbin.Select(releases, "", mbin.Version{}, false)
	require.NoError(t, err)
	require.True(t, mbin.ValidTag(sel.Release.Tag))
}
