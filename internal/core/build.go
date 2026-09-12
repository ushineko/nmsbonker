package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ushineko/nmsbonker/internal/build"
	"github.com/ushineko/nmsbonker/internal/build/cache"
	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/mbin"
	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/mxml"
)

// BuildRequest runs the whole pipeline (spec 002 R6.3).
type BuildRequest struct {
	Request
	// Recache re-extracts and re-decompiles every source file.
	Recache bool
	// Deploy installs the result under GAMEDATA/MODS when the build succeeds.
	Deploy bool
	// ReplaceSymlink is passed through to the deploy step.
	ReplaceSymlink bool
	// ModName overrides the configured output folder name for this run.
	ModName string
	// All builds disabled mods too, for a one-off "what would this do" run.
	All bool
}

// BuildResult is the report plus where it was written (R6.3).
type BuildResult struct {
	Report      *report.Result
	ReportPaths report.Paths
	OutputDir   string
	// Compatibility is the round-trip check run before the build (R3.4).
	Compatibility ToolCheckResult
	// Deployed is set when the build also installed itself.
	Deployed *DeployResult
}

// ErrNoEnabledMods is a build with nothing to do. Reported rather than
// producing an empty mod folder that would silently replace a working one.
var ErrNoEnabledMods = errors.New("no mods are enabled; run `nmsbonker mods enable NAME` first")

/*
Build merges every enabled mod and writes the report (R6.3).

The order of the phases is fixed by what each one needs. Compatibility first,
because a compiler that cannot round-trip this install's files will produce
suspect output from every later phase and the user should be told before they
wait for it. Then the pak index and the pristine cache, which is where the time
goes on a cold run. Then the merge and the recompile gate, and only then the
report -- which is written even when targets were dropped, because that is
exactly when it is worth reading.
*/
func Build(ctx context.Context, req BuildRequest) (BuildResult, error) {
	started := time.Now()
	s, err := open(req.Request)
	if err != nil {
		return BuildResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return BuildResult{}, err
	}
	modName := req.ModName
	if modName == "" {
		modName = s.cfg.ModName
	}

	compiler, err := mbin.Locate(s.paths.Tools, s.cfg.MBINCompiler.Pin)
	if err != nil {
		return BuildResult{}, fmt.Errorf("%w; run `nmsbonker tools ensure`", err)
	}
	compilerVersion := compiler.Tag
	if v, err := compiler.Version(ctx); err == nil {
		compilerVersion = v
	}

	req.Events.progress(1, 5, "checking compiler compatibility")
	compat, err := toolCheck(ctx, s, req.Events, nil)
	if err != nil {
		return BuildResult{}, err
	}
	if compat.Status != CompatOK {
		req.Events.logf(LevelWarn, "compiler compatibility: %s (%s); %s",
			compat.Status, compat.Detail, compat.Advice)
	}

	req.Events.progress(2, 5, "loading mod scripts")
	scripts, err := s.loadScripts(ctx, req.All)
	if err != nil {
		return BuildResult{}, err
	}
	plan := build.NewPlan(scripts)
	if len(plan.Enabled) == 0 {
		return BuildResult{}, ErrNoEnabledMods
	}
	req.Events.logf(LevelInfo, "%d mod(s), %d target file(s), %d edit block(s)",
		len(plan.Enabled), len(plan.Targets), plan.Blocks())

	req.Events.progress(3, 5, "indexing the game archives")
	idx, _, err := s.index(ctx, req.Events)
	if err != nil {
		return BuildResult{}, err
	}

	req.Events.progress(4, 5, "preparing pristine game files")
	store := cache.New(s.cacheDir(), idx, compiler, compiler.Tag, s.cfg.Workers())
	store.Log = func(msg string) { req.Events.logf(LevelDebug, "%s", msg) }
	store.Progress = func(done, total int, what string) {
		req.Events.progress(done, total, "decompiling "+what)
	}
	cached, err := store.Ensure(ctx, plan.Sources, req.Recache)
	if err != nil {
		return BuildResult{}, err
	}
	sources := make(map[string]build.Source, len(cached.Entries))
	for key, entry := range cached.Entries {
		sources[key] = build.Source{MXML: store.MXMLPath(entry), Internal: entry.Internal}
		if entry.ResolvedBy == "basename" {
			req.Events.logf(LevelWarn, "%s resolved by basename to %s", entry.Source, entry.Internal)
		}
	}
	var misses []string
	for _, m := range cached.Misses {
		misses = append(misses, m.Source+" ("+m.Reason+")")
		req.Events.logf(LevelWarn, "no game file for %s: %s", m.Source, m.Reason)
	}

	req.Events.progress(5, 5, "merging and compiling")
	result, err := build.Run(ctx, plan, build.Options{
		Sources: sources, Compiler: compiler, Workspace: s.paths.Workspace,
		ModName: modName, Workers: s.cfg.Workers(),
		Emit:            func(e mxml.Event) { req.Events.logf(LevelDebug, "%s", e.Line()) },
		Progress:        func(done, total int, what string) { req.Events.progress(done, total, "building "+what) },
		CompilerVersion: compilerVersion, GameBuildID: s.install.BuildID,
		Compatibility: compat.Status, CompatibilityDetail: compat.Detail,
		CacheTime: cached.Duration, CacheReused: cached.Reused, CacheBuilt: cached.Extracted,
		CacheMisses: misses, Params: s.cfg.Params,
	})
	if err != nil {
		return BuildResult{}, err
	}
	result.Timings.Total = time.Since(started)

	paths, err := report.Write(s.reportsDir(), result)
	if err != nil {
		return BuildResult{}, err
	}
	out := BuildResult{
		Report: result, ReportPaths: paths, OutputDir: result.OutputDir, Compatibility: compat,
	}

	if req.Deploy {
		deployed, err := deploy(ctx, s, DeployRequest{
			Request: req.Request, ReplaceSymlink: req.ReplaceSymlink, ModName: modName,
		})
		if err != nil {
			return out, err
		}
		out.Deployed = &deployed
	}
	return out, nil
}

/*
cacheDir is the pristine cache for this game build.

Keyed by the Steam buildid so that a game update starts a new tree rather than
invalidating the old one entry by entry, and so that rolling an update back
finds its cache still there. An install with no readable manifest (a copy made
outside Steam) falls back to a fixed name, which is correct: there is nothing to
distinguish it from.
*/
func (s *session) cacheDir() string {
	build := s.install.BuildID
	if build == "" {
		build = "unknown"
	}
	return filepath.Join(s.paths.Cache, "game", build)
}

// reportsDir is where BUILD_REPORT.md and report.json land (R4.4).
func (s *session) reportsDir() string { return filepath.Join(s.paths.Workspace, "reports") }

// loadScripts loads every configured mod, keeping failures in build order.
func (s *session) loadScripts(ctx context.Context, all bool) ([]build.Script, error) {
	list, changed, err := reconcile(s)
	if err != nil {
		return nil, err
	}
	if changed {
		if err := s.cfg.Save(); err != nil {
			return nil, err
		}
	}
	out := make([]build.Script, 0, len(list.Mods))
	for _, m := range list.Mods {
		script := build.Script{Name: m.Name, Enabled: m.Enabled || all}
		switch {
		case m.Status == ModMissing:
			script.Missing = true
		case script.Enabled:
			// The overrides are applied here, to a copy, before the script is
			// ever executed (spec 004 R1.3). Everything downstream sees a
			// script with the user's numbers already in it.
			src, _, err := s.scriptSource(m)
			if err != nil {
				script.Err = err
				break
			}
			def, err := modscript.LoadSource(ctx, scriptPath(m), src)
			if err != nil {
				script.Err = err
			} else {
				script.Def = def
			}
		}
		out = append(out, script)
	}
	return out, nil
}

// ReportRequest reads back the last build's report (R6.3).
type ReportRequest struct {
	Request
}

// ReportResult is the stored report and where it came from.
type ReportResult struct {
	Path     string
	Markdown string
	Report   *report.Result
}

// ErrNoReport reports that nothing has been built yet.
var ErrNoReport = errors.New("no build report yet; run `nmsbonker build`")

// Report returns the latest build report without rebuilding anything.
func Report(_ context.Context, req ReportRequest) (ReportResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ReportResult{}, err
	}
	path := filepath.Join(s.reportsDir(), "latest", "report.json")
	res, err := report.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return ReportResult{}, ErrNoReport
	}
	if err != nil {
		return ReportResult{}, err
	}
	md, err := os.ReadFile(filepath.Join(s.reportsDir(), "latest", "BUILD_REPORT.md"))
	if err != nil {
		md = []byte(report.Markdown(res))
	}
	return ReportResult{Path: path, Report: res, Markdown: string(md)}, nil
}
