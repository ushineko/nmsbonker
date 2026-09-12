package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ushineko/nmsbonker/internal/hgpak"
)

/*
The caches, as an operation rather than as a directory to be explained.

Three separate things live under the cache directory and they cost different
amounts to lose. The pristine tree (game/<buildid>/) is the expensive one: it is
every source file a build touched, extracted and decompiled, and rebuilding it
is the four to five seconds a cold build spends before it does any work. The pak
index is cheap to rebuild and stale often, because Steam updates the game. The
release listing is a few kilobytes and an ETag.

Deleting them is safe -- every one is derived from something else -- which is
exactly why the tool should be willing to do it rather than leaving the user to
work out which directory under $XDG_CACHE_HOME is which.
*/

// CacheInfoRequest asks what the caches hold.
type CacheInfoRequest struct {
	Request
}

// GameCache is one game build's pristine cache tree.
type GameCache struct {
	BuildID string `json:"buildId"`
	Dir     string `json:"dir"`
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
	// Current marks the tree the installed game would use. The others are
	// leftovers from builds before a game update, kept deliberately: rolling an
	// update back finds its cache still there.
	Current  bool      `json:"current"`
	Modified time.Time `json:"modified"`
}

// CacheInfoResult is what the caches hold and where (R2.5 cache card).
type CacheInfoResult struct {
	Dir            string      `json:"dir"`
	CurrentBuildID string      `json:"currentBuildId"`
	Games          []GameCache `json:"games"`
	TotalFiles     int         `json:"totalFiles"`
	TotalBytes     int64       `json:"totalBytes"`

	PakIndexPath  string       `json:"pakIndexPath"`
	PakIndexBytes int64        `json:"pakIndexBytes"`
	PakIndex      IndexSummary `json:"pakIndex"`

	ReleasesPath  string `json:"releasesPath"`
	ReleasesBytes int64  `json:"releasesBytes"`
}

// CacheInfo measures the caches without changing anything.
func CacheInfo(_ context.Context, req CacheInfoRequest) (CacheInfoResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return CacheInfoResult{}, err
	}
	out := CacheInfoResult{
		Dir:          s.paths.Cache,
		PakIndexPath: s.paths.PakIndex,
		ReleasesPath: s.paths.Releases,
	}
	if s.installErr == nil {
		out.CurrentBuildID = s.install.BuildID
		paks, err := s.install.PakFiles()
		if err != nil {
			req.Events.logf(LevelWarn, "listing paks: %v", err)
		}
		out.PakIndex = IndexSummary(hgpak.IndexFreshness(s.paths.PakIndex, paks))
	} else {
		out.PakIndex = IndexSummary(hgpak.IndexFreshness(s.paths.PakIndex, nil))
	}
	if fi, err := os.Stat(s.paths.PakIndex); err == nil {
		out.PakIndexBytes = fi.Size()
	}
	if fi, err := os.Stat(s.paths.Releases); err == nil {
		out.ReleasesBytes = fi.Size()
	}

	gameRoot := filepath.Join(s.paths.Cache, "game")
	entries, err := os.ReadDir(gameRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return out, fmt.Errorf("read %s: %w", gameRoot, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(gameRoot, e.Name())
		files, bytes, err := treeSize(dir)
		if err != nil {
			req.Events.logf(LevelWarn, "measuring %s: %v", dir, err)
			continue
		}
		g := GameCache{
			BuildID: e.Name(), Dir: dir, Files: files, Bytes: bytes,
			Current: e.Name() == out.CurrentBuildID,
		}
		if fi, err := e.Info(); err == nil {
			g.Modified = fi.ModTime()
		}
		out.Games = append(out.Games, g)
		out.TotalFiles += files
		out.TotalBytes += bytes
	}
	sort.Slice(out.Games, func(i, j int) bool { return out.Games[i].BuildID < out.Games[j].BuildID })
	return out, nil
}

// ClearCacheRequest removes cached game data.
type ClearCacheRequest struct {
	Request
	// All clears every game build's tree, not just the installed one's.
	All bool
	// IncludeIndex also removes the pak index, which the next operation
	// rebuilds from the archives.
	IncludeIndex bool
}

// ClearCacheResult says what was removed and how much it was worth.
type ClearCacheResult struct {
	Removed []string `json:"removed"`
	Files   int      `json:"files"`
	Bytes   int64    `json:"bytes"`
}

/*
ClearCache deletes derived data and nothing else (R2.5).

It will not touch the library, the workspace or the game. Everything it removes
is rebuilt by the next build, at the cost of the four or five seconds the
pristine cache saves; that is the whole trade, and it is why this needs a
confirmation in the GUI but not a warning in the docs.
*/
func ClearCache(_ context.Context, req ClearCacheRequest) (ClearCacheResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ClearCacheResult{}, err
	}
	var out ClearCacheResult

	targets := []string{}
	gameRoot := filepath.Join(s.paths.Cache, "game")
	switch {
	case req.All:
		targets = append(targets, gameRoot)
	case s.installErr == nil && s.install.BuildID != "":
		targets = append(targets, filepath.Join(gameRoot, s.install.BuildID))
	default:
		// No install and no --all: there is no way to tell which tree belongs
		// to the game that is not there, so say so rather than guessing.
		return out, errors.New(
			"no game install was found, so there is no current build to clear; pass --all to " +
				"clear every cached game build")
	}
	if req.IncludeIndex {
		targets = append(targets, s.paths.PakIndex)
	}

	for _, t := range targets {
		if _, err := os.Lstat(t); err != nil {
			continue // nothing there to remove
		}
		files, bytes, err := treeSizeOrFile(t)
		if err != nil {
			return out, err
		}
		if err := os.RemoveAll(t); err != nil {
			return out, fmt.Errorf("remove %s: %w", t, err)
		}
		out.Removed = append(out.Removed, t)
		out.Files += files
		out.Bytes += bytes
	}
	return out, nil
}

// RebuildIndexRequest forces the pak index to be built from the archives.
type RebuildIndexRequest struct {
	Request
}

// RebuildIndexResult is the freshly built index's shape.
type RebuildIndexResult struct {
	Path     string        `json:"path"`
	Paks     int           `json:"paks"`
	Files    int           `json:"files"`
	Bytes    int64         `json:"bytes"`
	Duration time.Duration `json:"duration"`
}

/*
RebuildIndex discards the pak index and reads every archive again (R2.5).

The index normally maintains itself: it re-reads only the paks whose size or
mtime changed. This exists for the case that check cannot see -- an archive
replaced with one of the same size and timestamp, or an index written by a build
that was interrupted -- where the answer to "why does it say that file is not
there" is to stop trusting the cache.
*/
func RebuildIndex(ctx context.Context, req RebuildIndexRequest) (RebuildIndexResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return RebuildIndexResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return RebuildIndexResult{}, err
	}
	out := RebuildIndexResult{Path: s.paths.PakIndex}
	if err := os.Remove(s.paths.PakIndex); err != nil && !errors.Is(err, os.ErrNotExist) {
		return out, fmt.Errorf("remove %s: %w", s.paths.PakIndex, err)
	}
	started := time.Now()
	_, stats, err := s.index(ctx, req.Events)
	if err != nil {
		return out, err
	}
	out.Paks, out.Files, out.Duration = stats.Paks, stats.Files, time.Since(started)
	if fi, err := os.Stat(s.paths.PakIndex); err == nil {
		out.Bytes = fi.Size()
	}
	return out, nil
}

// treeSizeOrFile is treeSize for a path that may be a plain file, used by
// ClearCache where the pak index is one and the cache trees are not.
func treeSizeOrFile(path string) (files int, bytes int64, err error) {
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("stat %s: %w", path, err)
	}
	if !fi.IsDir() {
		return 1, fi.Size(), nil
	}
	return treeSize(path)
}
