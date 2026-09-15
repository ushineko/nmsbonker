/*
Package cache keeps a decompiled copy of the game files a build edits
(spec 002 R3).

Every merge starts from a pristine MXML. Producing one means finding the file in
the game's pak archives, extracting it, and running MBINCompiler over it -- a
second or two per file, a hundred files per build. The cache makes that a
one-off per game update: entries are keyed by the pak's (size, mtime) and the
compiler's version, so a game patch invalidates only the paks it touched and a
compiler upgrade invalidates everything, which is the correct answer in both
cases.

Nothing here writes to the game directory.
*/
package cache

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

	"github.com/ushineko/nmsbonker/internal/hgpak"
	"github.com/ushineko/nmsbonker/internal/mbin"
)

// manifestVersion guards the on-disk shape. A build that changes what an entry
// records bumps it, and older caches are rebuilt rather than misread.
const manifestVersion = 1

// ManifestName is the file recording what the cache holds.
const ManifestName = "manifest.json"

/*
Key normalises a MBIN_FILE_SOURCE the way the reference builder did.

Backslashes to forward slashes, then upper case. Scripts spell the same file
half a dozen ways -- "METADATA\REALITY\TABLES\REWARDTABLE.MBIN",
"metadata/reality/tables/rewardtable.mbin" -- and the whole pipeline, targets
included, groups them by this key.
*/
func Key(source string) string {
	return strings.ToUpper(strings.ReplaceAll(source, `\`, "/"))
}

// Entry is one cached file (R3.1).
type Entry struct {
	Source   string `json:"source"`
	Internal string `json:"internal"`
	Pak      string `json:"pak"`
	PakSize  int64  `json:"pakSize"`
	PakMTime int64  `json:"pakMTime"`
	// CompilerVersion is what produced the MXML. A different compiler produces
	// a different MXML, so the entry is not reusable across versions.
	CompilerVersion string `json:"compilerVersion"`
	// MXML is the decompiled file, relative to the cache directory.
	MXML string `json:"mxml"`
	// Raw is the extracted MBIN, relative to the cache directory.
	Raw string `json:"raw"`
	// ResolvedBy is "path" or "basename". A source that only resolved by
	// basename is working by luck and the build report says so.
	ResolvedBy string `json:"resolvedBy"`
}

// ReasonNotInPaks is the miss reason for a source no archive holds, as opposed
// to one the compiler could not decompile; callers tell the two apart by it.
const ReasonNotInPaks = "in none of the indexed paks"

// Miss is a source the cache could not produce, and why (R3.3).
type Miss struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// Result is what one Ensure call did.
type Result struct {
	// Entries is keyed by Key(source).
	Entries   map[string]Entry
	Misses    []Miss
	Extracted int
	Reused    int
	Duration  time.Duration
}

// MXMLPath resolves an entry's MXML to an absolute path.
func (c *Cache) MXMLPath(e Entry) string { return filepath.Join(c.dir, filepath.FromSlash(e.MXML)) }

// Path is MXMLPath for a caller that has a directory rather than an open cache.
func Path(dir string, e Entry) string { return filepath.Join(dir, filepath.FromSlash(e.MXML)) }

/*
Read opens a cache directory read-only and returns what its manifest holds
(spec 005 R1.5).

`nmsbonker audit` re-checks a merge that already happened, so it needs the
pristine files that merge started from and must not be able to produce one:
extracting and decompiling needs the game and the compiler, and a command whose
whole point is "answer in a second without rebuilding" cannot wait for either. A
manifest that is missing or from another version reads as an empty cache, which
the caller reports as "nothing to compare against" rather than treating as a
failure.
*/
func Read(dir string) map[string]Entry {
	c := &Cache{dir: dir, entries: map[string]Entry{}}
	c.load()
	return c.entries
}

/*
Cache is the decompiled-file store for one game build.

Dir is per game buildid (cache_dir/game/<buildid>), so switching between an
installed game and a second copy, or rolling a game update back, does not mix
two games' files in one tree.
*/
type Cache struct {
	dir      string
	index    *hgpak.Index
	compiler *mbin.Compiler
	version  string
	workers  int

	// Log receives one line per interesting step; may be nil.
	Log func(msg string)
	// Progress reports extraction progress; may be nil.
	Progress func(done, total int, what string)

	mu      sync.Mutex
	entries map[string]Entry
	loaded  bool
}

// New opens the cache directory for one game build and compiler.
func New(dir string, index *hgpak.Index, compiler *mbin.Compiler, compilerVersion string, workers int) *Cache {
	if workers < 1 {
		workers = 1
	}
	return &Cache{
		dir: dir, index: index, compiler: compiler,
		version: compilerVersion, workers: workers,
		entries: map[string]Entry{},
	}
}

// Dir is where the cache lives.
func (c *Cache) Dir() string { return c.dir }

func (c *Cache) logf(format string, args ...any) {
	if c.Log != nil {
		c.Log(fmt.Sprintf(format, args...))
	}
}

// load reads the manifest. A missing or unreadable manifest is a cold cache,
// not an error: it is an optimisation and must not be able to block a build.
func (c *Cache) load() {
	if c.loaded {
		return
	}
	c.loaded = true
	b, err := os.ReadFile(filepath.Join(c.dir, ManifestName))
	if err != nil {
		return
	}
	var doc struct {
		Version int              `json:"version"`
		Entries map[string]Entry `json:"entries"`
	}
	if err := json.Unmarshal(b, &doc); err != nil || doc.Version != manifestVersion {
		return
	}
	for k, v := range doc.Entries {
		c.entries[k] = v
	}
}

func (c *Cache) save() error {
	if err := os.MkdirAll(c.dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", c.dir, err)
	}
	doc := struct {
		Version int              `json:"version"`
		Entries map[string]Entry `json:"entries"`
	}{manifestVersion, c.entries}
	b, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		return fmt.Errorf("encode cache manifest: %w", err)
	}
	path := filepath.Join(c.dir, ManifestName)
	tmp, err := os.CreateTemp(c.dir, ".manifest-*.json")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", c.dir, err)
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
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// job is one source that has to be produced.
type job struct {
	key    string
	source string
	loc    hgpak.Location
	byBase bool
	pakInfo
}

type pakInfo struct {
	size  int64
	mtime int64
}

/*
Ensure produces a cached MXML for every source, in parallel (R3.2).

A source that is in no pak, or that MBINCompiler will not decompile, is recorded
as a miss and does not stop the others: the reference builder reported "no cached
MXML for X" against the mod that wanted it and built everything else, and a
build that refuses to run because one mod names a file the game no longer ships
is a worse tool.
*/
func (c *Cache) Ensure(ctx context.Context, sources []string, force bool) (Result, error) {
	started := time.Now()
	c.mu.Lock()
	c.load()
	c.mu.Unlock()

	out := Result{Entries: map[string]Entry{}}
	var todo []job
	seen := map[string]bool{}

	for _, src := range sources {
		key := Key(src)
		if seen[key] {
			continue
		}
		seen[key] = true

		loc, byBase, ok := c.index.Resolve(src)
		if !ok {
			out.Misses = append(out.Misses, Miss{Source: src, Reason: ReasonNotInPaks})
			continue
		}
		info, err := statPak(loc.Pak)
		if err != nil {
			out.Misses = append(out.Misses, Miss{Source: src, Reason: err.Error()})
			continue
		}
		if !force {
			if entry, ok := c.reusable(key, info); ok {
				out.Entries[key] = entry
				out.Reused++
				continue
			}
		}
		todo = append(todo, job{key: key, source: src, loc: loc, byBase: byBase, pakInfo: info})
	}

	if len(todo) > 0 {
		c.logf("cache: extracting and decompiling %d file(s)", len(todo))
		produced, misses := c.produce(ctx, todo)
		for k, v := range produced {
			out.Entries[k] = v
			out.Extracted++
		}
		out.Misses = append(out.Misses, misses...)

		c.mu.Lock()
		for k, v := range produced {
			c.entries[k] = v
		}
		err := c.save()
		c.mu.Unlock()
		if err != nil {
			return out, err
		}
	}

	sort.Slice(out.Misses, func(i, j int) bool { return out.Misses[i].Source < out.Misses[j].Source })
	out.Duration = time.Since(started)
	if err := ctx.Err(); err != nil {
		return out, fmt.Errorf("prepare the pristine cache: %w", err)
	}
	return out, nil
}

// reusable reports a cached entry that is still current.
func (c *Cache) reusable(key string, info pakInfo) (Entry, bool) {
	c.mu.Lock()
	entry, ok := c.entries[key]
	c.mu.Unlock()
	if !ok {
		return Entry{}, false
	}
	if entry.PakSize != info.size || entry.PakMTime != info.mtime || entry.CompilerVersion != c.version {
		return Entry{}, false
	}
	if _, err := os.Stat(c.MXMLPath(entry)); err != nil {
		// The manifest says it is there and it is not; treat it as absent
		// rather than handing the build a path that will fail to open.
		return Entry{}, false
	}
	return entry, true
}

// produce extracts and decompiles the outstanding jobs concurrently.
func (c *Cache) produce(ctx context.Context, todo []job) (map[string]Entry, []Miss) {
	workers := min(c.workers, len(todo))
	var (
		mu   sync.Mutex
		out  = map[string]Entry{}
		miss []Miss
		done int
		wg   sync.WaitGroup
	)
	jobs := make(chan job)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				entry, err := c.one(ctx, j)
				mu.Lock()
				if err != nil {
					miss = append(miss, Miss{Source: j.source, Reason: err.Error()})
				} else {
					out[j.key] = entry
				}
				done++
				if c.Progress != nil {
					c.Progress(done, len(todo), j.loc.Name)
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
	return out, miss
}

/*
one extracts a single file and decompiles it.

The placement rule is the one build_cache.py had to learn the hard way: the file
goes where the *pak index* says it lives, not where the extraction tool put it.
hgpaktool invented a "GLOBALS/" prefix for root-level globals, and a cache that
believed it could not find the file again. The native reader has no such quirk,
but keeping the index as the authority means the cache layout does not depend on
the extractor at all.
*/
func (c *Cache) one(ctx context.Context, j job) (Entry, error) {
	if c.compiler == nil {
		return Entry{}, errors.New("no MBINCompiler is installed; run `nmsbonker tools ensure`")
	}
	if err := checkInternal(j.loc.Name); err != nil {
		return Entry{}, err
	}
	pak, err := hgpak.Open(j.loc.Pak)
	if err != nil {
		return Entry{}, err
	}
	defer func() { _ = pak.Close() }()

	data, err := pak.ReadFile(j.loc.Name)
	if err != nil {
		return Entry{}, err
	}

	rawRel := path("raw", j.loc.Name)
	rawPath := filepath.Join(c.dir, filepath.FromSlash(rawRel))
	if err := os.MkdirAll(filepath.Dir(rawPath), 0o750); err != nil {
		return Entry{}, fmt.Errorf("create %s: %w", filepath.Dir(rawPath), err)
	}
	if err := os.WriteFile(rawPath, data, 0o600); err != nil {
		return Entry{}, fmt.Errorf("write %s: %w", rawPath, err)
	}

	// Decompile into a scratch directory next to the target: MBINCompiler names
	// its output from the input's basename, and two sources with the same
	// basename in different directories would otherwise race for one name.
	tmp, err := os.MkdirTemp(c.dir, ".decompile-")
	if err != nil {
		return Entry{}, fmt.Errorf("create temp dir in %s: %w", c.dir, err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	produced, err := c.compiler.Decompile(ctx, rawPath, tmp)
	if err != nil {
		return Entry{}, err
	}

	mxmlRel := MXMLRel(j.loc.Name)
	mxmlPath := filepath.Join(c.dir, filepath.FromSlash(mxmlRel))
	if err := os.MkdirAll(filepath.Dir(mxmlPath), 0o750); err != nil {
		return Entry{}, fmt.Errorf("create %s: %w", filepath.Dir(mxmlPath), err)
	}
	if err := move(produced, mxmlPath); err != nil {
		return Entry{}, err
	}

	resolvedBy := "path"
	if j.byBase {
		resolvedBy = "basename"
	}
	return Entry{
		Source: j.source, Internal: j.loc.Name, Pak: filepath.Base(j.loc.Pak),
		PakSize: j.size, PakMTime: j.mtime, CompilerVersion: c.version,
		MXML: mxmlRel, Raw: rawRel, ResolvedBy: resolvedBy,
	}, nil
}

/*
MXMLRel is where a pak-internal path's decompiled copy lives (R3.1).

Upper case with the extension swapped: metadata/reality/tables/rewardtable.mbin
becomes mxml/METADATA/REALITY/TABLES/REWARDTABLE.MXML. Upper case because that
is the spelling the build output uses under GAMEDATA/MODS, so a cached file and
the MBIN built from it are findable by the same name.
*/
func MXMLRel(internal string) string {
	upper := strings.ToUpper(internal)
	if i := strings.LastIndexByte(upper, '.'); i > strings.LastIndexByte(upper, '/') {
		upper = upper[:i]
	}
	return "mxml/" + upper + ".MXML"
}

/*
checkInternal refuses a pak-internal path that would escape the cache directory.

The path comes out of an archive's own manifest, so it is data, not something
this program chose. A pak carrying "../../.ssh/authorized_keys" is far-fetched
-- it would have to be planted in the game's PCBANKS -- but the cost of not
checking is writing an arbitrary file as the user, and the check is three lines.
*/
func checkInternal(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return fmt.Errorf("refusing an absolute pak-internal path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return fmt.Errorf("refusing a pak-internal path that escapes the cache: %q", name)
		}
	}
	return nil
}

// path joins forward-slash cache-relative components.
func path(parts ...string) string { return strings.Join(parts, "/") }

func statPak(p string) (pakInfo, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return pakInfo{}, fmt.Errorf("stat %s: %w", p, err)
	}
	return pakInfo{size: fi.Size(), mtime: fi.ModTime().UnixNano()}, nil
}

// move renames a file, falling back to copy-and-remove across filesystems.
func move(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("read %s: %w", from, err)
	}
	// The destination is built from a pak-internal path that checkInternal has
	// already refused to let escape the cache directory.
	if err := os.WriteFile(to, data, 0o600); err != nil { //nolint:gosec // guarded by checkInternal
		return fmt.Errorf("write %s: %w", to, err)
	}
	if err := os.Remove(from); err != nil {
		return fmt.Errorf("remove %s: %w", from, err)
	}
	return nil
}
