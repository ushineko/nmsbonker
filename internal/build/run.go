package build

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/nmsbonker/internal/build/audit"
	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/mbin"
	"github.com/ushineko/nmsbonker/internal/mxml"
)

// Source is one cached pristine file the merge starts from.
type Source struct {
	// MXML is the absolute path of the decompiled file.
	MXML string
	// Internal is the pak-internal path, which decides where the built MBIN
	// goes under the mod folder.
	Internal string
}

// Options configure one build run (R4.2).
type Options struct {
	// Sources maps cache.Key(source) to the pristine file to start from. A
	// target with no entry is reported and skipped, not fatal.
	Sources map[string]Source
	// Compiler gates every output; a build without one produces nothing.
	Compiler *mbin.Compiler
	// Workspace is the build directory (config workspace_dir).
	Workspace string
	// ModName is the folder created under it, and later under GAMEDATA/MODS.
	ModName string
	// Workers bounds concurrent target processing.
	Workers int

	// Emit receives every report event, in plan order.
	Emit func(mxml.Event)
	// Progress reports completed targets.
	Progress func(done, total int, what string)

	CompilerVersion     string
	GameBuildID         string
	Compatibility       string
	CompatibilityDetail string
	CacheTime           time.Duration
	CacheReused         int
	CacheBuilt          int
	CacheMisses         []string
	// Params are the parameter overrides the scripts were loaded with, recorded
	// in the report so a front end can tell whether the output on disk was
	// built from the settings currently in force (spec 004 R2.1).
	Params map[string]map[string]float64
	// Audit are the reward-amount limits the merged tables are judged against
	// (spec 005 R1.2). The zero value is treated as the defaults, so a caller
	// that does not care still gets an audit.
	Audit audit.Thresholds
}

// outcome is one target's finished work, held until its turn to be reported.
type outcome struct {
	events  []mxml.Event
	target  report.TargetResult
	merge   time.Duration
	compile time.Duration
	audit   time.Duration
	// amounts is the reward-amount audit of this target, when it is one of the
	// audited tables (spec 005 R1.1).
	amounts *audit.Result
}

// runner carries the per-run state the target workers share.
type runner struct {
	opts    Options
	modRoot string
	workDir string
}

/*
Run merges, compiles and places every target (R4.2).

Targets are independent, so they are processed concurrently up to Workers; their
events are buffered and replayed in plan order afterwards. That ordering is not
cosmetic. The per-mod applied/skipped tallies, the "keys not found" lists and the
report line order are all derived from this stream, and a build whose report
changes between runs cannot be diffed against the one before a game update --
which is the main thing anyone does with it.
*/
func Run(ctx context.Context, plan *Plan, opts Options) (*report.Result, error) {
	started := time.Now()
	if opts.Workers < 1 {
		opts.Workers = 1
	}
	if opts.Audit == (audit.Thresholds{}) {
		opts.Audit = audit.Defaults()
	}
	r := &runner{
		opts:    opts,
		modRoot: filepath.Join(opts.Workspace, opts.ModName),
		workDir: filepath.Join(opts.Workspace, opts.ModName+".work"),
	}
	if err := r.prepare(); err != nil {
		return nil, err
	}

	res := &report.Result{
		ModName: opts.ModName, Generated: time.Now(), OutputDir: r.modRoot,
		CompilerVersion: opts.CompilerVersion, GameBuildID: opts.GameBuildID,
		Compatibility: opts.Compatibility, CompatibilityDetail: opts.CompatibilityDetail,
		UnsupportedKeys: plan.Unsupported, CacheMisses: opts.CacheMisses,
		CacheReused: opts.CacheReused, CacheBuilt: opts.CacheBuilt,
		Complex: plan.Complex, Workers: opts.Workers, Params: opts.Params,
	}

	stats := newTally()
	var events []mxml.Event
	for _, e := range plan.Issues {
		events = append(events, e)
		stats.record(e)
		r.emit(e)
	}

	outcomes := r.process(ctx, plan)

	for _, o := range outcomes {
		res.Timings.Merge += o.merge
		res.Timings.Compile += o.compile
		res.Timings.Audit += o.audit
		if o.amounts != nil {
			if res.Audit == nil {
				res.Audit = &audit.Result{Thresholds: opts.Audit}
			}
			res.Audit.Add(*o.amounts)
		}
		for _, e := range o.events {
			events = append(events, e)
			stats.record(e)
			r.emit(e)
		}
		res.Targets = append(res.Targets, o.target)
		switch o.target.Outcome {
		case report.OutcomeBuilt, report.OutcomeDegraded:
			res.Built++
			for _, m := range o.target.Mods {
				stats.file(m, o.target.Internal)
			}
			if o.target.Outcome == report.OutcomeDegraded {
				res.Degraded = append(res.Degraded, report.DegradedFile{
					Internal: o.target.Internal, Mods: o.target.SkippedMods,
				})
				stats.degrade(o.target.SkippedMods)
			}
		default:
			res.Dropped++
			if o.target.Outcome == report.OutcomeDropped {
				stats.drop(o.target.Mods)
			}
		}
	}

	if res.Audit != nil {
		res.Audit.Sort()
	}
	res.Applied, res.Skipped, res.Capped = stats.applied, stats.skipped, stats.capped
	res.Mods = stats.rows(plan)
	res.Lines = report.Events(events)
	res.Timings.Cache = opts.CacheTime

	// A cancelled run is undone rather than published. process() returns as
	// soon as the context closes, so what is under <ModName> at this point is
	// whichever targets happened to finish -- a mod folder missing most of its
	// files, which would deploy and would break the game quietly. prepare()
	// moved the previous output to <ModName>.prev exactly so this is
	// recoverable, so it goes back and the partial run is thrown away.
	//
	// Checked before mirrorGlobals and before .prev is removed, which is the
	// whole of the fix: those two lines used to run first and consumed the copy
	// this needs (spec 003 AC3).
	if err := ctx.Err(); err != nil {
		res.Timings.Total = time.Since(started)
		if rerr := r.restorePrevious(); rerr != nil {
			return res, fmt.Errorf("build: %w; the previous output could not be put back: %w", err, rerr)
		}
		return res, fmt.Errorf("build: %w", err)
	}

	if err := r.mirrorGlobals(); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(r.modRoot + ".prev"); err != nil {
		return nil, fmt.Errorf("remove %s: %w", r.modRoot+".prev", err)
	}
	res.Timings.Total = time.Since(started)
	return res, nil
}

/*
restorePrevious undoes an interrupted run.

The partial output goes, and the folder prepare() set aside comes back under its
own name. A first-ever build has nothing set aside, and then the correct result
is no mod folder at all rather than a folder holding a third of one.

The work directory is left where it is. It holds the intermediate MXML, which is
what someone chasing "why did that file not compile" wants to look at, and the
next run clears it anyway.
*/
func (r *runner) restorePrevious() error {
	prev := r.modRoot + ".prev"
	if err := os.RemoveAll(r.modRoot); err != nil {
		return fmt.Errorf("remove %s: %w", r.modRoot, err)
	}
	if _, err := os.Stat(prev); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // nothing was there before this run
		}
		return fmt.Errorf("stat %s: %w", prev, err)
	}
	if err := os.Rename(prev, r.modRoot); err != nil {
		return fmt.Errorf("restore %s: %w", r.modRoot, err)
	}
	return nil
}

func (r *runner) emit(e mxml.Event) {
	if r.opts.Emit != nil {
		r.opts.Emit(e)
	}
}

/*
prepare clears the workspace, keeping the previous output until this run has
finished (R4.3).

Renaming rather than deleting means a build that fails halfway has not destroyed
the mod folder the user currently has deployed. The rename is undone by Run on
success; a crash leaves <ModName>.prev on disk, which is the state the user
wants to find.
*/
func (r *runner) prepare() error {
	prev := r.modRoot + ".prev"
	if err := os.RemoveAll(prev); err != nil {
		return fmt.Errorf("remove %s: %w", prev, err)
	}
	if _, err := os.Stat(r.modRoot); err == nil {
		if err := os.Rename(r.modRoot, prev); err != nil {
			return fmt.Errorf("rename %s: %w", r.modRoot, err)
		}
	}
	if err := os.RemoveAll(r.workDir); err != nil {
		return fmt.Errorf("remove %s: %w", r.workDir, err)
	}
	for _, dir := range []string{r.modRoot, filepath.Join(r.workDir, "mxml"), filepath.Join(r.workDir, "tmp")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

// process runs the targets concurrently and returns their outcomes in plan
// order.
func (r *runner) process(ctx context.Context, plan *Plan) []outcome {
	out := make([]outcome, len(plan.Targets))
	workers := min(r.opts.Workers, len(plan.Targets))
	if workers < 1 {
		return out
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		done int
	)
	jobs := make(chan int)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				out[i] = r.target(ctx, plan.Targets[i])
				mu.Lock()
				done++
				if r.opts.Progress != nil {
					r.opts.Progress(done, len(plan.Targets), plan.Targets[i].Key)
				}
				mu.Unlock()
			}
		}()
	}
feed:
	for i := range plan.Targets {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	return out
}

/*
target merges one file, compiles it and places the result (R4.2).

The fallback is the interesting part. When the merged document will not
recompile and some of the edits were structural (ADD or REMOVE), the merge is
redone from pristine with only the value edits and compiled again. If that
works, the file ships "degraded": every mod's value edits are in it and the
structural ones are named in the report. That is almost always the right trade,
because a reward table that adds one new entry is worth less than the twenty
value edits that would be lost by dropping the file.

The retry's events are appended to the first attempt's, not replacing them,
which is what the reference builder did and why its totals exceed the merge-only
counts in the golden index.
*/
func (r *runner) target(ctx context.Context, t *Target) outcome {
	res := report.TargetResult{
		Key: t.Key, Source: t.Source, Blocks: len(t.Items), Mods: t.Mods(),
	}
	src, ok := r.opts.Sources[t.Key]
	if !ok {
		res.Outcome = report.OutcomeNoSource
		res.Error = "no cached MXML"
		return outcome{target: res, events: []mxml.Event{{
			Kind: mxml.WARN, Mod: t.Items[0].Mod, Detail: "no cached MXML for " + t.Items[0].Source,
		}}}
	}
	res.Internal = InternalUpper(src.Internal)

	raw, err := os.ReadFile(src.MXML)
	if err != nil {
		res.Outcome = report.OutcomeNoSource
		res.Error = err.Error()
		return outcome{target: res, events: []mxml.Event{{
			Kind: mxml.WARN, Mod: t.Items[0].Mod, Detail: "no cached MXML for " + t.Items[0].Source,
		}}}
	}
	pristine := strings.Split(string(raw), "\n")

	mergeStart := time.Now()
	lines, events := merge(pristine, t.Items)
	merged := time.Since(mergeStart)

	// The audit runs on the merge, before the compiler is asked whether the
	// document is valid (R1.1): a file that will not recompile is exactly the
	// one whose amounts nobody will otherwise look at, and the audit costs the
	// same either way.
	auditStart := time.Now()
	amounts, auditEvents := r.audit(t, pristine, lines, res.Internal)
	events = append(events, auditEvents...)
	audited := time.Since(auditStart)

	compileStart := time.Now()
	built, cerr := r.compile(ctx, res.Internal, lines)
	if cerr != nil {
		var safe []Item
		for _, it := range t.Items {
			if !it.Block.Structural() {
				safe = append(safe, it)
			}
		}
		if len(safe) > 0 && len(safe) < len(t.Items) {
			retryStart := time.Now()
			lines2, events2 := merge(pristine, safe)
			merged += time.Since(retryStart)
			events = append(events, events2...)
			if b2, err2 := r.compile(ctx, res.Internal, lines2); err2 == nil {
				built, cerr = b2, nil
				res.SkippedMods = structuralMods(t.Items)
				res.Outcome = report.OutcomeDegraded
			}
		}
	}
	compiled := time.Since(compileStart)

	if cerr != nil {
		res.Outcome = report.OutcomeDropped
		res.Error = cerr.Error()
		for _, it := range t.Items {
			events = append(events, mxml.Event{
				Kind: mxml.WARN, Mod: it.Mod, File: mxml.Base(it.Source),
				Detail: fmt.Sprintf("RECOMPILE FAILED for %s -> DROPPED", src.Internal),
			})
		}
		return outcome{events: events, target: res, merge: merged, compile: compiled,
			audit: audited, amounts: amounts}
	}

	dest := filepath.Join(r.modRoot, filepath.FromSlash(res.Internal))
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		res.Outcome = report.OutcomeDropped
		res.Error = err.Error()
		return outcome{events: events, target: res, merge: merged, compile: compiled,
			audit: audited, amounts: amounts}
	}
	if err := moveFile(built, dest); err != nil {
		res.Outcome = report.OutcomeDropped
		res.Error = err.Error()
		return outcome{events: events, target: res, merge: merged, compile: compiled,
			audit: audited, amounts: amounts}
	}
	res.Output = dest
	if res.Outcome == "" {
		res.Outcome = report.OutcomeBuilt
	}

	note := ""
	if len(res.SkippedMods) > 0 {
		note = " (structural add/remove skipped for: " + strings.Join(res.SkippedMods, ", ") + ")"
	}
	events = append(events, mxml.Info("built %s from %d edit-block(s)%s", res.Internal, len(t.Items), note))
	return outcome{events: events, target: res, merge: merged, compile: compiled,
		audit: audited, amounts: amounts}
}

// merge applies every item to a fresh copy of the pristine lines.
func merge(pristine []string, items []Item) ([]string, []mxml.Event) {
	lines := slices.Clone(pristine)
	var events []mxml.Event
	for _, it := range items {
		var evs []mxml.Event
		lines, evs = applyOne(lines, it)
		events = append(events, evs...)
	}
	return lines, events
}

/*
applyOne runs one block and turns a panic into a warning.

The reference builder wrapped each block in `try/except` and reported "exception on
X" rather than aborting the build; a Go panic from an index calculation on an
unexpectedly shaped MXML has to do the same, or one malformed game file takes
the other ninety-nine down with it.
*/
func applyOne(lines []string, it Item) (out []string, events []mxml.Event) {
	defer func() {
		if p := recover(); p != nil {
			out = lines
			events = []mxml.Event{{
				Kind: mxml.WARN, Mod: it.Mod, File: mxml.Base(it.Source),
				Detail: fmt.Sprintf("exception on %s: %v", mxml.Base(it.Source), p),
			}}
		}
	}()
	return mxml.Apply(lines, it.Block, mxml.ApplyContext{Mod: it.Mod, Source: it.Source})
}

// structuralMods names the mods whose structural edits a degraded build left
// out, sorted, as the report line quotes them.
func structuralMods(items []Item) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		if it.Block.Structural() && !seen[it.Mod] {
			seen[it.Mod] = true
			out = append(out, it.Mod)
		}
	}
	sort.Strings(out)
	return out
}

/*
compile writes the merged MXML and runs MBINCompiler over it (R4.2).

The MXML is kept under <ModName>.work/mxml so that a rejected file can be looked
at: "MBINCompiler said no" is not a diagnosis, and the merged document is the
only place the answer can be. Each target compiles into its own temp directory,
because the compiler names its output from the input's basename and two targets
can share one.
*/
func (r *runner) compile(ctx context.Context, internalUpper string, lines []string) (string, error) {
	rel := strings.TrimSuffix(internalUpper, filepath.Ext(internalUpper)) + ".MXML"
	mxmlPath := filepath.Join(r.workDir, "mxml", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(mxmlPath), 0o750); err != nil {
		return "", fmt.Errorf("create %s: %w", filepath.Dir(mxmlPath), err)
	}
	// The path comes from the cache, which refuses a pak-internal path that
	// would escape its own directory; this joins it under the workspace.
	//nolint:gosec // the internal path is validated where it enters the cache
	if err := os.WriteFile(mxmlPath, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", mxmlPath, err)
	}
	if r.opts.Compiler == nil {
		return "", errNoCompiler
	}
	tmp := filepath.Join(r.workDir, "tmp", strings.ReplaceAll(internalUpper, "/", "_"))
	if err := os.RemoveAll(tmp); err != nil {
		return "", fmt.Errorf("remove %s: %w", tmp, err)
	}
	return r.opts.Compiler.Compile(ctx, mxmlPath, tmp)
}

// errNoCompiler is the state a build cannot proceed from: nothing to gate the
// output with, so nothing may be shipped.
var errNoCompiler = fmt.Errorf("no MBINCompiler is installed; run `nmsbonker tools ensure`")

/*
mirrorGlobals copies the root-level globals into a GLOBALS/ subfolder (R4.2).

The game mounts them at the pak root, so the copy at the root is the one that
takes effect. The duplicate matches the layout AMUMSS produces and the layout
every working combine folder in the wild has, and a game that reads only one of
them is unaffected by the other. It costs a few hundred kilobytes and removes a
whole class of "my mod is not loading" report.
*/
func (r *runner) mirrorGlobals() error {
	entries, err := os.ReadDir(r.modRoot)
	if err != nil {
		return fmt.Errorf("read %s: %w", r.modRoot, err)
	}
	var globals []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".MBIN") {
			continue
		}
		if (strings.HasPrefix(name, "GC") && strings.Contains(name, "GLOBALS")) || name == "GCCREATUREGLOBALS.MBIN" {
			globals = append(globals, name)
		}
	}
	if len(globals) == 0 {
		return nil
	}
	dir := filepath.Join(r.modRoot, "GLOBALS")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	for _, name := range globals {
		data, err := os.ReadFile(filepath.Join(r.modRoot, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		//nolint:gosec // name is a directory entry this build just wrote
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", filepath.Join(dir, name), err)
		}
	}
	return nil
}

// moveFile renames, falling back to copy-and-remove across filesystems.
func moveFile(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("read %s: %w", from, err)
	}
	// Both ends are inside this build's own workspace, under a path the cache
	// already refused to let escape its directory.
	if err := os.WriteFile(to, data, 0o600); err != nil { //nolint:gosec // workspace-internal path
		return fmt.Errorf("write %s: %w", to, err)
	}
	if err := os.Remove(from); err != nil {
		return fmt.Errorf("remove %s: %w", from, err)
	}
	return nil
}
