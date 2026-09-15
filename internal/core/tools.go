package core

import (
	"context"
	"fmt"
	"os"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/mbin"
)

// EnsureToolsRequest asks for a usable MBINCompiler.
type EnsureToolsRequest struct {
	Request
	// Pin overrides the configured pin for this call only.
	Pin string
}

// EnsureToolsResult reports what `tools ensure` did (R7.2).
type EnsureToolsResult struct {
	Tag            string
	Flavor         string
	Bin            string
	Reason         string
	AlreadyPresent bool
	// ListingSource says where the release list came from, so a user acting on
	// a stale answer can see that it is stale.
	ListingSource string
	// Warning is a non-fatal problem, typically "GitHub was unreachable, this
	// came from the cache".
	Warning string
	// Attempts records each flavor tried, so a fallback is visible.
	Attempts []string
	// GameDataVersion is what the selection was matched against, or "unknown".
	GameDataVersion string
	// Mapping is the save-key mapping installed beside the compiler (spec 007
	// R4.1); MappingWarning says why there is none.
	Mapping        string
	MappingWarning string
}

/*
EnsureTools installs the MBINCompiler this game needs (R5.1-R5.3, R7.2).

Ordering matters and is not obvious: the game data version is read *before* the
release listing, because it decides which release to pick (R5.2), and reading it
needs a compiler that may not exist yet. On a first run it is therefore unknown
and the highest release is installed; the second run, with that compiler in
place, can read the version and would pick the matching release if the first
guess was wrong. That is why this operation is safe -- and useful -- to run
twice.
*/
func EnsureTools(ctx context.Context, req EnsureToolsRequest) (EnsureToolsResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return EnsureToolsResult{}, err
	}
	out := EnsureToolsResult{GameDataVersion: mbin.VersionUnknown}

	pin := req.Pin
	if pin == "" {
		pin = s.cfg.MBINCompiler.Pin
	}

	gdv, err := gameDataVersion(ctx, s, req.Events)
	if err != nil {
		return out, err
	}
	if gdv.Known {
		out.GameDataVersion = gdv.Version.Numeric()
	}

	req.Events.progress(1, 3, "listing MBINCompiler releases")
	client := s.releaseClient(req.Request)
	releases, source, warning := client.Releases(ctx)
	out.ListingSource = source
	if warning != nil {
		out.Warning = warning.Error()
		req.Events.logf(LevelWarn, "%v; using the cached release listing", warning)
	}
	if len(releases) == 0 {
		if warning != nil {
			return out, fmt.Errorf("list MBINCompiler releases: %w", warning)
		}
		return out, mbin.ErrNoReleases
	}

	req.Events.progress(2, 3, "choosing a release")
	selection, err := mbin.Select(releases, pin, gdv.Version, gdv.Known)
	if err != nil {
		return out, err
	}
	out.Tag = selection.Release.Tag
	out.Reason = selection.Reason
	req.Events.logf(LevelInfo, "MBINCompiler %s: %s", selection.Release.Tag, selection.Reason)

	if err := config.MkdirAll(s.paths.Tools); err != nil {
		return out, err
	}

	req.Events.progress(3, 3, "installing "+selection.Release.Tag)
	install, err := mbin.Install(ctx, s.paths.Tools, selection.Release, s.cfg.MBINCompiler.Flavor, nil)
	out.Attempts = install.Attempts
	if err != nil {
		return out, err
	}
	out.AlreadyPresent = install.AlreadyPresent
	out.Flavor = install.Flavor
	out.Bin = install.Compiler.Bin
	// The save-key mapping (spec 007 R4.1) is fetched separately so that
	// --no-network keeps its promise: an installed compiler with no mapping
	// is reported, not downloaded around.
	if req.NoNetwork {
		install.MappingWarning = ""
		if install.Mapping == "" {
			install.MappingWarning = "not fetched: --no-network"
		}
	} else {
		mbin.InstallMapping(ctx, install, selection.Release, nil)
	}
	out.Mapping = install.Mapping
	out.MappingWarning = install.MappingWarning
	if out.MappingWarning != "" {
		req.Events.logf(LevelWarn, "save-key mapping: %s", out.MappingWarning)
	}
	return out, nil
}

// ListToolsRequest asks what is installed.
type ListToolsRequest struct {
	Request
}

// ToolEntry is one installed MBINCompiler release.
type ToolEntry struct {
	Tag    string
	Dir    string
	Active bool
	Flavor string
	Bin    string
	// Version is what the binary reports, or why it could not be asked. A
	// release whose binary no longer runs is exactly what this list is for.
	Version string
	// Mapping says whether the save-key mapping sits beside it (spec 007 R4.2).
	Mapping bool
}

// ListToolsResult is the installed set (R7.2).
type ListToolsResult struct {
	ToolsDir string
	Pin      string
	Entries  []ToolEntry
}

// ListTools reports the installed releases, highest first, without the network.
func ListTools(ctx context.Context, req ListToolsRequest) (ListToolsResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ListToolsResult{}, err
	}
	out := ListToolsResult{ToolsDir: s.paths.Tools, Pin: s.cfg.MBINCompiler.Pin}

	active, _ := mbin.Locate(s.paths.Tools, s.cfg.MBINCompiler.Pin)
	for _, tag := range mbin.Installed(s.paths.Tools) {
		// Installed only yields tags that passed mbin.ValidTag, so Dir cannot
		// fail here; skipping rather than reporting keeps the listing honest if
		// that ever stops being true.
		dir, err := mbin.Dir(s.paths.Tools, tag)
		if err != nil {
			continue
		}
		entry := ToolEntry{Tag: tag, Dir: dir}
		if c, err := mbin.Locate(s.paths.Tools, tag); err == nil {
			entry.Flavor = c.Flavor
			entry.Bin = c.Bin
			_, statErr := os.Stat(mbin.MappingPath(c.Bin))
			entry.Mapping = statErr == nil
			if v, err := c.Version(ctx); err == nil {
				entry.Version = v
			} else {
				entry.Version = "does not run: " + err.Error()
			}
		} else {
			entry.Version = "incomplete install"
		}
		entry.Active = active != nil && active.Tag == tag
		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}

// PinToolRequest sets or clears the compiler pin (R8.1 `tools pin|unpin`).
type PinToolRequest struct {
	Request
	// Tag is the release to pin, or "" to unpin.
	Tag string
}

// PinToolResult reports the new pin.
type PinToolResult struct {
	Pin       string
	Installed bool
}

// PinTool records which release to use, and says whether it is installed.
//
// Pinning a release that is not installed is allowed: the natural order is to
// decide which one you want and then fetch it, and refusing here would force
// the user to install a version they have already decided against.
func PinTool(_ context.Context, req PinToolRequest) (PinToolResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return PinToolResult{}, err
	}
	if req.Tag != "" && !mbin.ValidTag(req.Tag) {
		return PinToolResult{}, fmt.Errorf(
			"%q does not look like an MBINCompiler release tag (expected vM.m.p, optionally -suffix)", req.Tag)
	}
	if err := s.cfg.Set("mbincompiler.pin", req.Tag); err != nil {
		return PinToolResult{}, err
	}
	if err := s.cfg.Save(); err != nil {
		return PinToolResult{}, err
	}
	_, err = mbin.Locate(s.paths.Tools, req.Tag)
	return PinToolResult{Pin: req.Tag, Installed: err == nil}, nil
}

// ListReleasesRequest asks what MBINCompiler releases exist upstream.
type ListReleasesRequest struct {
	Request
}

// ReleaseInfo is one release, as the "check for updates" listing shows it.
type ReleaseInfo struct {
	Tag        string `json:"tag"`
	Prerelease bool   `json:"prerelease"`
	Installed  bool   `json:"installed"`
	Active     bool   `json:"active"`
	// Selected marks the release automatic selection would install.
	Selected bool `json:"selected"`
	Assets   int  `json:"assets"`
}

// ListReleasesResult is the release listing and how it was chosen from (R2.5).
type ListReleasesResult struct {
	// Source says where the listing came from: the network, or the cache when
	// GitHub was unreachable or answered "not modified".
	Source string `json:"source"`
	// Warning is a non-fatal problem, typically "GitHub was unreachable, this
	// came from the cache". A stale answer is still an answer, but the user
	// should be able to see that it is stale.
	Warning         string        `json:"warning,omitempty"`
	Pin             string        `json:"pin,omitempty"`
	GameDataVersion string        `json:"gameDataVersion"`
	Selected        string        `json:"selected,omitempty"`
	Reason          string        `json:"reason,omitempty"`
	Releases        []ReleaseInfo `json:"releases"`
}

/*
ListReleases reports the upstream releases without installing anything (R2.5).

`tools ensure` also lists them, but it installs as well, and "what is available"
is a question worth being able to ask on its own -- before a game update, or
when deciding whether a pin is holding an old release back. Offline-tolerant for
the same reason `ensure` is: the cached listing with a warning beats a dialog
saying the network is down.
*/
func ListReleases(ctx context.Context, req ListReleasesRequest) (ListReleasesResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ListReleasesResult{}, err
	}
	out := ListReleasesResult{Pin: s.cfg.MBINCompiler.Pin, GameDataVersion: mbin.VersionUnknown}

	gdv, err := gameDataVersion(ctx, s, req.Events)
	if err != nil {
		return out, err
	}
	if gdv.Known {
		out.GameDataVersion = gdv.Version.Numeric()
	}

	releases, source, warning := s.releaseClient(req.Request).Releases(ctx)
	out.Source = source
	if warning != nil {
		out.Warning = warning.Error()
		req.Events.logf(LevelWarn, "%v; using the cached release listing", warning)
	}
	if len(releases) == 0 {
		if warning != nil {
			return out, fmt.Errorf("list MBINCompiler releases: %w", warning)
		}
		return out, mbin.ErrNoReleases
	}

	if selection, err := mbin.Select(releases, s.cfg.MBINCompiler.Pin, gdv.Version, gdv.Known); err == nil {
		out.Selected, out.Reason = selection.Release.Tag, selection.Reason
	}

	installed := map[string]bool{}
	for _, tag := range mbin.Installed(s.paths.Tools) {
		installed[tag] = true
	}
	active, _ := mbin.Locate(s.paths.Tools, s.cfg.MBINCompiler.Pin)
	for _, r := range releases {
		out.Releases = append(out.Releases, ReleaseInfo{
			Tag:        r.Tag,
			Prerelease: r.Prerelease,
			Installed:  installed[r.Tag],
			Active:     active != nil && active.Tag == r.Tag,
			Selected:   r.Tag == out.Selected,
			Assets:     len(r.Assets),
		})
	}
	return out, nil
}

// RemoveToolRequest deletes one installed MBINCompiler release.
type RemoveToolRequest struct {
	Request
	Tag string
}

// RemoveToolResult says what was removed and how much disk it returned.
type RemoveToolResult struct {
	Tag   string `json:"tag"`
	Dir   string `json:"dir"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
}

/*
RemoveTool deletes an installed release (R2.5).

It refuses the release that is currently in use. Removing it would leave the
next build reaching for a compiler that is not there, and the fix -- pin
something else first, or install a newer one -- is a decision the user should
make deliberately rather than discover from a failed build. Nothing else is
touched: the cache built with that compiler stays, because it is keyed by the
game build and is still correct.
*/
func RemoveTool(_ context.Context, req RemoveToolRequest) (RemoveToolResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return RemoveToolResult{}, err
	}
	out := RemoveToolResult{Tag: req.Tag}
	if !mbin.ValidTag(req.Tag) {
		return out, fmt.Errorf(
			"%q does not look like an MBINCompiler release tag (expected vM.m.p, optionally -suffix)", req.Tag)
	}
	dir, err := mbin.Dir(s.paths.Tools, req.Tag)
	if err != nil {
		return out, err
	}
	out.Dir = dir
	if _, err := os.Stat(dir); err != nil {
		return out, fmt.Errorf("%s is not installed under %s", req.Tag, s.paths.Tools)
	}
	if active, err := mbin.Locate(s.paths.Tools, s.cfg.MBINCompiler.Pin); err == nil && active.Tag == req.Tag {
		return out, fmt.Errorf(
			"%s is the release builds are using; pin or install another one first "+
				"(`nmsbonker tools pin TAG`, `nmsbonker tools ensure`)", req.Tag)
	}
	files, bytes, err := treeSize(dir)
	if err != nil {
		return out, err
	}
	if err := os.RemoveAll(dir); err != nil {
		return out, fmt.Errorf("remove %s: %w", dir, err)
	}
	out.Files, out.Bytes = files, bytes
	return out, nil
}
