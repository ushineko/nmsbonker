package core

import (
	"context"
	"fmt"

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
		entry := ToolEntry{Tag: tag, Dir: mbin.Dir(s.paths.Tools, tag)}
		if c, err := mbin.Locate(s.paths.Tools, tag); err == nil {
			entry.Flavor = c.Flavor
			entry.Bin = c.Bin
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
	if req.Tag != "" {
		if _, ok := mbin.ParseVersion(req.Tag); !ok {
			return PinToolResult{}, fmt.Errorf("%q does not look like an MBINCompiler release tag (expected vM.m.p...)", req.Tag)
		}
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
