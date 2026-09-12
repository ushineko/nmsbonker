package mbin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Repo is where MBINCompiler is published.
const Repo = "monkeyman192/MBINCompiler"

// DefaultAPIBase is the GitHub API root. Overridable so the tests can serve a
// fixture from httptest instead of reaching the network (R9.1).
const DefaultAPIBase = "https://api.github.com"

// httpTimeout bounds a release listing or an asset download (R5.1).
const httpTimeout = 20 * time.Second

// ErrNoNetwork reports that a fetch was needed but --no-network was given.
var ErrNoNetwork = errors.New("network access is disabled (--no-network)")

// Asset is one downloadable file on a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Release is one MBINCompiler release.
type Release struct {
	Tag        string  `json:"tag_name"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`

	// Version is the parsed Tag. Releases whose tag does not parse are dropped
	// before a caller sees them.
	Version Version `json:"-"`
}

// Asset returns the named asset.
func (r Release) Asset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// Source says where a release listing came from, which `tools ensure` reports:
// a user acting on a stale list should be able to see that it is stale.
const (
	SourceNetwork      = "github"
	SourceNotModified  = "cache (unchanged upstream)"
	SourceCacheOffline = "cache (offline)"
)

// Client lists MBINCompiler releases.
type Client struct {
	// HTTP is the transport; nil means a client with the standard timeout.
	HTTP *http.Client
	// APIBase overrides DefaultAPIBase (tests).
	APIBase string
	// CachePath is the ETag-cached listing (R5.1).
	CachePath string
	// UserAgent identifies this program to GitHub. Required: the API rejects
	// requests without one.
	UserAgent string
	// Token is an optional GitHub token, taken from the environment by the
	// caller. It is never written to the cache and never logged (R5.1).
	Token string
	// NoNetwork forbids any request; the cache is used alone.
	NoNetwork bool
}

// cacheDoc is the on-disk ETag cache.
//
// The raw response body is stored rather than the parsed releases: a build that
// learns to read a new field then gets it from an existing cache instead of
// silently seeing zero values until the cache expires.
type cacheDoc struct {
	ETag    string          `json:"etag"`
	Fetched time.Time       `json:"fetched"`
	Body    json.RawMessage `json:"body"`
}

// Releases lists MBINCompiler's recent releases (R5.1).
//
// Returns the source so the caller can report a stale answer. A network failure
// with a usable cache is a warning, not an error: `tools ensure` on a laptop
// with no connection should still install from what it already knows.
func (c *Client) Releases(ctx context.Context) (releases []Release, source string, warning error) {
	cached := c.readCache()

	if c.NoNetwork {
		if cached == nil {
			return nil, "", ErrNoNetwork
		}
		return parseReleases(cached.Body), SourceCacheOffline, nil
	}

	body, etag, notModified, err := c.fetch(ctx, cached)
	switch {
	case err != nil && cached != nil:
		return parseReleases(cached.Body), SourceCacheOffline, err
	case err != nil:
		return nil, "", err
	case notModified:
		return parseReleases(cached.Body), SourceNotModified, nil
	}

	c.writeCache(cacheDoc{ETag: etag, Fetched: time.Now().UTC(), Body: body})
	return parseReleases(body), SourceNetwork, nil
}

func (c *Client) fetch(ctx context.Context, cached *cacheDoc) (body json.RawMessage, etag string, notModified bool, err error) {
	base := c.APIBase
	if base == "" {
		base = DefaultAPIBase
	}
	url := base + "/repos/" + Repo + "/releases?per_page=30"

	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", false, fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		// Read from the environment by the caller and never persisted: it is
		// not written to the cache file and no code path logs the header.
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if cached != nil && cached.ETag != "" {
		req.Header.Set("If-None-Match", cached.ETag)
	}

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: httpTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", false, fmt.Errorf("list %s releases: %w", Repo, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified && cached != nil {
		return nil, cached.ETag, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", false, fmt.Errorf("list %s releases: %s", Repo, resp.Status)
	}
	// 8 MiB is far more than a 30-release listing needs and stops a wrong URL
	// from being read into memory without bound.
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", false, fmt.Errorf("read %s release listing: %w", Repo, err)
	}
	return b, resp.Header.Get("ETag"), false, nil
}

// parseReleases decodes a listing and drops entries whose tag does not parse.
func parseReleases(body []byte) []Release {
	var raw []Release
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	out := make([]Release, 0, len(raw))
	for _, r := range raw {
		v, ok := ParseVersion(r.Tag)
		if !ok {
			continue
		}
		r.Version = v
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version.Compare(out[j].Version) > 0 })
	return out
}

func (c *Client) readCache() *cacheDoc {
	if c.CachePath == "" {
		return nil
	}
	b, err := os.ReadFile(c.CachePath)
	if err != nil {
		return nil
	}
	var doc cacheDoc
	if err := json.Unmarshal(b, &doc); err != nil || len(doc.Body) == 0 {
		return nil
	}
	return &doc
}

// writeCache stores the listing. Failing to cache is not worth failing the
// operation the user asked for; the next run simply refetches.
func (c *Client) writeCache(doc cacheDoc) {
	if c.CachePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.CachePath), 0o750); err != nil {
		return
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return
	}
	_ = os.WriteFile(c.CachePath, b, 0o600)
}

// Selection is the outcome of choosing a release (R5.2).
type Selection struct {
	Release Release
	// Reason explains the choice in one line, for `tools ensure` output.
	Reason string
}

// ErrNoReleases reports an empty or unusable release listing.
var ErrNoReleases = errors.New("no MBINCompiler release with a parseable version tag")

// Select chooses the release to install (R5.2).
//
// A pin wins outright. Otherwise the highest release whose major.minor matches
// the game's data version, because that pairing is what libMBIN's struct
// definitions are written against; a mismatched compiler decompiles to
// plausible-looking MXML with the wrong fields. With no known game version --
// the first run, before any compiler exists to ask -- the highest release is
// the best guess and the reason says so.
func Select(releases []Release, pin string, game Version, haveGame bool) (Selection, error) {
	if len(releases) == 0 {
		return Selection{}, ErrNoReleases
	}
	if pin != "" {
		for _, r := range releases {
			if r.Tag == pin {
				return Selection{Release: r, Reason: "pinned to " + pin}, nil
			}
		}
		return Selection{}, fmt.Errorf("pinned release %s is not among the %d listed releases", pin, len(releases))
	}
	if haveGame {
		for _, r := range releases {
			if r.Version.SameMajorMinor(game) {
				return Selection{
					Release: r,
					Reason: fmt.Sprintf("highest %d.%d release, matching game data version %s",
						game.Major, game.Minor, game.Numeric()),
				}, nil
			}
		}
		return Selection{
			Release: releases[0],
			Reason: fmt.Sprintf("no release matches game data version %s; using the highest, %s",
				game.Numeric(), releases[0].Tag),
		}, nil
	}
	return Selection{
		Release: releases[0],
		Reason:  "game data version is unknown (no compiler installed yet); using the highest release, " + releases[0].Tag,
	}, nil
}
