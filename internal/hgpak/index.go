package hgpak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

/*
Index maps an internal pak path to the archive holding it (R4.5).

Two lookups, not one. The exact map is the normalised full path, which is what
the game and a correctly written script use. The basename map is a fallback,
because the reference Python builder resolved by basename when the full path
missed, and the scripts in the wild rely on it: several write
"GLOBALS\GCGAMEPLAYGLOBALS.GLOBAL.MBIN" for a file that actually lives at the
pak root. Dropping the fallback would silently stop applying those mods, which
is worse than resolving a path the author spelled loosely.

Building the index opens every pak and reads only its manifest. Over the real
97-pak install that is a few seconds; the result is cached on disk keyed by each
pak's (size, mtime) so a game update re-reads only the paks it actually changed.
*/
type Index struct {
	byPath map[string]Location
	paks   []pakRecord

	// The basename map is built on first use, not at load. It is only ever
	// consulted when an exact path missed, which on a healthy script set is
	// never; building it eagerly doubled the warm-load time (AC6's 200 ms
	// budget) to populate a map most runs do not read.
	baseOnce sync.Once
	byBase   map[string][]Location
}

// Location is where a file lives.
type Location struct {
	// Pak is the absolute path of the archive.
	Pak string
	// Name is the normalised internal path within it.
	Name string
}

// pakRecord is one archive's contribution, and the cache's unit of staleness.
//
// Files is one newline-joined string rather than a JSON array. The real install
// holds ~195k paths, and an array of that many JSON strings costs ~50 ms extra
// to parse and allocate on every warm load -- a quarter of AC6's whole budget,
// spent re-creating strings that are immediately concatenated into map keys.
type pakRecord struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime_unix_nano"`
	Files string `json:"files"`
}

// names splits a record's packed file list.
func (r pakRecord) names() []string {
	if r.Files == "" {
		return nil
	}
	return strings.Split(r.Files, "\n")
}

// indexFileVersion guards the on-disk cache shape. A build that changes what is
// stored bumps it, and older caches are rebuilt rather than misread.
const indexFileVersion = 1

type indexFile struct {
	Version int         `json:"version"`
	Paks    []pakRecord `json:"paks"`
}

// IndexStats reports what a build actually did, for `status` and the timings
// AC6 asks for.
type IndexStats struct {
	Paks      int
	Files     int
	Reindexed int
	Reused    int
	Duration  time.Duration
	// FromCache is true when a usable cache file was read.
	FromCache bool
	// Saved is true when the cache was rewritten.
	Saved bool
}

// ProgressFunc reports index-build progress. It is called from one goroutine.
type ProgressFunc func(done, total int, what string)

// LoadIndex reads a cached index without touching any pak (R4.5).
//
// A missing or unreadable cache is not an error: it is a cold start, and the
// caller rebuilds. Refusing to run because a cache file is corrupt would make a
// cache -- an optimisation -- into a dependency.
func LoadIndex(cachePath string) (*Index, bool) {
	b, err := os.ReadFile(cachePath)
	if err != nil {
		return newIndex(nil), false
	}
	var doc indexFile
	if err := json.Unmarshal(b, &doc); err != nil || doc.Version != indexFileVersion {
		return newIndex(nil), false
	}
	return newIndex(doc.Paks), true
}

// BuildIndex indexes the given paks, reusing anything still current in the
// cache at cachePath and writing the cache back when it changed (R4.5).
func BuildIndex(ctx context.Context, cachePath string, paks []string, workers int, progress ProgressFunc) (*Index, IndexStats, error) {
	started := time.Now()
	cached, fromCache := LoadIndex(cachePath)
	stats := IndexStats{FromCache: fromCache}

	previous := make(map[string]pakRecord, len(cached.paks))
	for _, rec := range cached.paks {
		previous[rec.Path] = rec
	}

	type job struct {
		idx  int
		path string
	}
	records := make([]pakRecord, len(paks))
	var todo []job

	for i, p := range paks {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		fi, err := os.Stat(abs)
		if err != nil {
			return nil, stats, fmt.Errorf("stat %s: %w", abs, err)
		}
		if rec, ok := previous[abs]; ok && rec.Size == fi.Size() && rec.MTime == fi.ModTime().UnixNano() {
			records[i] = rec
			stats.Reused++
			continue
		}
		records[i] = pakRecord{Path: abs, Size: fi.Size(), MTime: fi.ModTime().UnixNano()}
		todo = append(todo, job{idx: i, path: abs})
	}
	stats.Reindexed = len(todo)

	if len(todo) > 0 {
		if workers < 1 {
			workers = 1
		}
		if workers > len(todo) {
			workers = len(todo)
		}
		var (
			mu       sync.Mutex
			firstErr error
			done     int
			wg       sync.WaitGroup
		)
		jobs := make(chan job)
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range jobs {
					names, err := manifestOf(j.path)
					mu.Lock()
					if err != nil && firstErr == nil {
						firstErr = err
					}
					records[j.idx].Files = names
					done++
					if progress != nil {
						progress(done, len(todo), filepath.Base(j.path))
					}
					mu.Unlock()
				}
			}()
		}
	feed:
		for _, j := range todo {
			select {
			case <-ctx.Done():
				break feed
			case jobs <- j:
			}
		}
		close(jobs)
		wg.Wait()
		if firstErr != nil {
			return nil, stats, firstErr
		}
		if err := ctx.Err(); err != nil {
			return nil, stats, fmt.Errorf("index paks: %w", err)
		}
	}

	idx := newIndex(records)
	stats.Paks = len(records)
	stats.Files = len(idx.byPath)
	stats.Duration = time.Since(started)

	// Write the cache only when something changed. A `status` run on an
	// unchanged install should not keep rewriting a multi-megabyte file.
	if cachePath != "" && (stats.Reindexed > 0 || !fromCache || len(cached.paks) != len(records)) {
		if err := saveIndex(cachePath, records); err != nil {
			return idx, stats, err
		}
		stats.Saved = true
	}
	return idx, stats, nil
}

// manifestOf opens a pak and reads only its filename manifest.
//
// A file in PCBANKS that is not a pak (the game ships BankSignatures.bin and
// filenames.json there) is reported as an empty contribution rather than an
// error: PakFiles already filters by extension, so reaching here means a
// genuinely odd file, and failing the whole index over one is worse than
// indexing the other 96.
func manifestOf(path string) (string, error) {
	f, err := Open(path)
	if err != nil {
		if errors.Is(err, ErrNotHGPAK) || errors.Is(err, ErrUnsupportedVersion) {
			return "", nil
		}
		return "", err
	}
	defer func() { _ = f.Close() }()
	return strings.Join(f.Names(), "\n"), nil
}

func newIndex(records []pakRecord) *Index {
	total := 0
	for _, rec := range records {
		total += strings.Count(rec.Files, "\n") + 1
	}
	idx := &Index{
		byPath: make(map[string]Location, total),
		paks:   records,
	}
	for _, rec := range records {
		for _, name := range rec.names() {
			// First pak wins. PakFiles sorts, so "first" is stable across runs
			// and two builds cannot disagree about which copy of a duplicated
			// internal path they read.
			if _, exists := idx.byPath[name]; !exists {
				idx.byPath[name] = Location{Pak: rec.Path, Name: name}
			}
		}
	}
	return idx
}

// basenames builds the fallback map on demand. See the field comment.
func (i *Index) basenames() map[string][]Location {
	i.baseOnce.Do(func() {
		i.byBase = make(map[string][]Location, len(i.byPath))
		for _, rec := range i.paks {
			for _, name := range rec.names() {
				base := Base(name)
				i.byBase[base] = append(i.byBase[base], Location{Pak: rec.Path, Name: name})
			}
		}
	})
	return i.byBase
}

func saveIndex(cachePath string, records []pakRecord) error {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(cachePath), err)
	}
	b, err := json.Marshal(indexFile{Version: indexFileVersion, Paks: records})
	if err != nil {
		return fmt.Errorf("encode pak index: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(cachePath), ".pak-index-*.json")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", filepath.Dir(cachePath), err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := os.Rename(name, cachePath); err != nil {
		return fmt.Errorf("replace %s: %w", cachePath, err)
	}
	return nil
}

// Len is how many distinct internal paths the index holds.
func (i *Index) Len() int { return len(i.byPath) }

// Paks lists the indexed archive paths, sorted.
func (i *Index) Paks() []string {
	out := make([]string, 0, len(i.paks))
	for _, rec := range i.paks {
		out = append(out, rec.Path)
	}
	sort.Strings(out)
	return out
}

// PakFileCount is how many files a given archive contributed.
func (i *Index) PakFileCount(pak string) int {
	for _, rec := range i.paks {
		if rec.Path == pak {
			return len(rec.names())
		}
	}
	return 0
}

// Resolve finds the archive holding a path (R4.5).
//
// byBasename is true when the exact path missed and the basename fallback
// answered, which the caller reports: a script whose path only resolves by
// basename is working by luck and the build report should say so.
func (i *Index) Resolve(name string) (loc Location, byBasename, ok bool) {
	norm := Normalise(name)
	if loc, ok := i.byPath[norm]; ok {
		return loc, false, true
	}
	if locs := i.basenames()[Base(norm)]; len(locs) > 0 {
		return locs[0], true, true
	}
	return Location{}, false, false
}

// Find returns every internal path matching a glob, sorted (R7.2 PakFind).
//
// The pattern is matched against the whole normalised path with
// filepath.Match, and a pattern with no wildcard is matched as a substring:
// `pak find rewardtable` should find metadata/reality/tables/rewardtable.mbin,
// because typing the full path is exactly what the user is using this to avoid.
func (i *Index) Find(pattern string) []Location {
	norm := Normalise(pattern)
	literal := !hasGlobMeta(norm)
	var out []Location
	for name, loc := range i.byPath {
		if matches(name, norm, literal) {
			out = append(out, loc)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Name != out[b].Name {
			return out[a].Name < out[b].Name
		}
		return out[a].Pak < out[b].Pak
	})
	return out
}

func matches(name, pattern string, literal bool) bool {
	if literal {
		// Both sides are already lower-cased by Normalise, so a plain substring
		// test is the case-insensitive one.
		return strings.Contains(name, pattern)
	}
	// filepath.Match does not let '*' cross a separator, which would make
	// `*rewardtable*` miss every file that is not at the pak root. Matching the
	// basename too covers the common case without a second pattern language.
	if ok, err := filepath.Match(pattern, name); err == nil && ok {
		return true
	}
	ok, err := filepath.Match(pattern, Base(name))
	return err == nil && ok
}

func hasGlobMeta(s string) bool { return strings.ContainsAny(s, "*?[") }

// Freshness describes the cached index without opening a single pak (R7.2).
//
// `status` wants to say whether the index is current; rebuilding it to find out
// would make the cheap command the expensive one.
type Freshness struct {
	Exists  bool
	Written time.Time
	Paks    int
	Files   int
	// Stale counts paks whose (size, mtime) no longer matches the cache, plus
	// paks the cache has never seen.
	Stale int
	// Missing counts indexed paks that are no longer on disk.
	Missing int
}

// IndexFreshness reports the state of the cache against the current paks.
func IndexFreshness(cachePath string, paks []string) Freshness {
	idx, ok := LoadIndex(cachePath)
	out := Freshness{Exists: ok}
	if !ok {
		out.Stale = len(paks)
		return out
	}
	if fi, err := os.Stat(cachePath); err == nil {
		out.Written = fi.ModTime()
	}
	known := make(map[string]pakRecord, len(idx.paks))
	for _, rec := range idx.paks {
		known[rec.Path] = rec
		out.Files += strings.Count(rec.Files, "\n") + 1
	}
	out.Paks = len(idx.paks)

	present := make(map[string]bool, len(paks))
	for _, p := range paks {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		present[abs] = true
		fi, err := os.Stat(abs)
		if err != nil {
			continue
		}
		rec, seen := known[abs]
		if !seen || rec.Size != fi.Size() || rec.MTime != fi.ModTime().UnixNano() {
			out.Stale++
		}
	}
	for path := range known {
		if !present[path] {
			out.Missing++
		}
	}
	return out
}
