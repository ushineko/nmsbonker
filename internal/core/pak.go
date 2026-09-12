package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/hgpak"
)

// PakListRequest lists paks, or the contents of one pak (R7.2).
type PakListRequest struct {
	Request
	// Pak names one archive by file name or path; "" lists the archives
	// themselves.
	Pak string
	// Glob filters the listed file names.
	Glob string
}

// PakSummary is one archive.
type PakSummary struct {
	Name  string
	Path  string
	Size  int64
	Files int
}

// PakEntry is one file inside an archive.
type PakEntry struct {
	Name string
	Pak  string
	Size uint64
}

// PakListResult is either a list of archives or a list of files in one.
type PakListResult struct {
	PCBanksDir string
	Paks       []PakSummary
	Entries    []PakEntry
}

// PakList lists the archives, or one archive's contents (R7.2).
func PakList(ctx context.Context, req PakListRequest) (PakListResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return PakListResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return PakListResult{}, err
	}
	paks, err := s.install.PakFiles()
	if err != nil {
		return PakListResult{}, err
	}
	out := PakListResult{PCBanksDir: s.install.PCBanksDir}

	if req.Pak == "" {
		// The file counts come from the index rather than from opening 97
		// archives, so `pak list` stays instant once the index is warm.
		idx, _, err := s.index(ctx, req.Events)
		if err != nil {
			return out, err
		}
		for _, p := range paks {
			summary := PakSummary{Name: filepath.Base(p), Path: p, Files: idx.PakFileCount(p)}
			if fi, err := os.Stat(p); err == nil {
				summary.Size = fi.Size()
			}
			out.Paks = append(out.Paks, summary)
		}
		return out, nil
	}

	path, err := matchPak(paks, req.Pak)
	if err != nil {
		return out, err
	}
	pak, err := hgpak.Open(path)
	if err != nil {
		return out, err
	}
	defer func() { _ = pak.Close() }()

	glob := hgpak.Normalise(req.Glob)
	for _, name := range pak.Names() {
		if glob != "" && !globMatch(name, glob) {
			continue
		}
		entry, _ := pak.Stat(name)
		out.Entries = append(out.Entries, PakEntry{Name: name, Pak: filepath.Base(path), Size: entry.Size})
	}
	sort.Slice(out.Entries, func(i, j int) bool { return out.Entries[i].Name < out.Entries[j].Name })
	return out, nil
}

// matchPak resolves a --pak argument to one archive.
//
// Accepts a full path, a file name, or a unique case-insensitive fragment,
// because the real names ("NMSARC.Precache.pak") are long and mixed-case and
// nobody types them accurately.
func matchPak(paks []string, want string) (string, error) {
	lower := strings.ToLower(want)
	var exact, partial []string
	for _, p := range paks {
		base := strings.ToLower(filepath.Base(p))
		switch {
		case p == want || base == lower:
			exact = append(exact, p)
		case strings.Contains(base, lower):
			partial = append(partial, p)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	switch len(partial) {
	case 1:
		return partial[0], nil
	case 0:
		return "", fmt.Errorf("no pak in PCBANKS matches %q", want)
	default:
		names := make([]string, 0, len(partial))
		for _, p := range partial {
			names = append(names, filepath.Base(p))
		}
		return "", fmt.Errorf("%q matches %d paks: %s", want, len(partial), strings.Join(names, ", "))
	}
}

// globMatch is the same matching rule the index uses, so `pak list GLOB` and
// `pak find GLOB` never disagree about what a pattern means.
func globMatch(name, pattern string) bool {
	if !strings.ContainsAny(pattern, "*?[") {
		return strings.Contains(name, pattern)
	}
	if ok, err := filepath.Match(pattern, name); err == nil && ok {
		return true
	}
	ok, err := filepath.Match(pattern, hgpak.Base(name))
	return err == nil && ok
}

// PakFindRequest searches every archive at once (R7.2).
type PakFindRequest struct {
	Request
	Glob string
}

// PakFindResult is where each match lives.
type PakFindResult struct {
	Glob    string
	Matches []PakEntry
	// IndexedPaks and IndexedFiles say how much was searched.
	IndexedPaks  int
	IndexedFiles int
}

// PakFind searches the index (R7.2).
func PakFind(ctx context.Context, req PakFindRequest) (PakFindResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return PakFindResult{}, err
	}
	idx, stats, err := s.index(ctx, req.Events)
	if err != nil {
		return PakFindResult{}, err
	}
	out := PakFindResult{Glob: req.Glob, IndexedPaks: stats.Paks, IndexedFiles: stats.Files}
	for _, loc := range idx.Find(req.Glob) {
		out.Matches = append(out.Matches, PakEntry{Name: loc.Name, Pak: filepath.Base(loc.Pak)})
	}
	return out, nil
}

// PakExtractRequest extracts one file from the archives (R7.2).
type PakExtractRequest struct {
	Request
	// Name is the internal path, in any spelling.
	Name string
	// OutDir is where to write it; "" means the current directory.
	OutDir string
}

// PakExtractResult says what was written.
type PakExtractResult struct {
	Name string
	Pak  string
	Path string
	Size int
	// ByBasename is true when the exact path missed and the basename fallback
	// answered, which a build report should mention.
	ByBasename bool
}

/*
PakExtract writes one file out of the archives (R7.2).

The internal directory structure is preserved under OutDir rather than the file
being dropped in flat. The legacy pipeline learned this the hard way in the
other direction: hgpaktool invents a "GLOBALS/" prefix for root-level globals,
and build_cache.py had to ignore where hgpaktool put the file and re-derive the
path from the index. Writing the index's own path means an extraction tree can
be compared with, or fed back into, the pak that produced it.
*/
func PakExtract(ctx context.Context, req PakExtractRequest) (PakExtractResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return PakExtractResult{}, err
	}
	idx, _, err := s.index(ctx, req.Events)
	if err != nil {
		return PakExtractResult{}, err
	}
	loc, byBasename, ok := idx.Resolve(req.Name)
	if !ok {
		return PakExtractResult{}, fmt.Errorf("%w: %s is in none of the %d indexed paks",
			hgpak.ErrNotFound, req.Name, idx.Len())
	}

	pak, err := hgpak.Open(loc.Pak)
	if err != nil {
		return PakExtractResult{}, err
	}
	defer func() { _ = pak.Close() }()
	data, err := pak.ReadFile(loc.Name)
	if err != nil {
		return PakExtractResult{}, err
	}

	outDir := req.OutDir
	if outDir == "" {
		outDir = "."
	}
	dest := filepath.Join(config.ExpandPath(outDir), filepath.FromSlash(loc.Name))
	if err := config.MkdirAll(filepath.Dir(dest)); err != nil {
		return PakExtractResult{}, err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil { //nolint:gosec // extracted game data the user asked for
		return PakExtractResult{}, fmt.Errorf("write %s: %w", dest, err)
	}
	req.Events.logf(LevelInfo, "extracted %s (%d bytes) from %s", loc.Name, len(data), filepath.Base(loc.Pak))

	return PakExtractResult{
		Name:       loc.Name,
		Pak:        filepath.Base(loc.Pak),
		Path:       dest,
		Size:       len(data),
		ByBasename: byBasename,
	}, nil
}
