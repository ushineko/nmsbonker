package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ushineko/nmsbonker/internal/build"
	"github.com/ushineko/nmsbonker/internal/build/audit"
	"github.com/ushineko/nmsbonker/internal/build/cache"
	"github.com/ushineko/nmsbonker/internal/build/report"
)

/*
Re-running the reward-amount audit without rebuilding (spec 005 R1.5).

The audit is part of a build, but its thresholds are a judgement call and
judgement calls are made by trying numbers. A build takes seconds at best and
minutes on a cold cache; re-reading the merge a build already left in the
workspace takes about as long as reading a nine-megabyte file, which is what
makes "set the ratio limit to 5 and look again" a thing somebody will actually
do rather than a thing the settings merely permit.

Nothing here rebuilds, recompiles, extracts or deploys. It reads the kept merge
and the pristine cache, both of which are already on disk, and says what the
build that produced them came out at.
*/

// AuditRequest re-runs the amount audit over the last build's merge (R1.5).
type AuditRequest struct {
	Request
	// ModName overrides the configured output folder, which is what names the
	// workspace directory the merge was kept in.
	ModName string
}

// AuditResult is what the re-run found (R1.5).
type AuditResult struct {
	Run build.AuditRun `json:"run"`
	// MergeDir is where the merged MXML was read from, so a user who wants to
	// look at a flagged entry by hand knows which file to open.
	MergeDir string `json:"mergeDir"`
	// CacheDir is the pristine cache the stock values came from.
	CacheDir string `json:"cacheDir"`
	// Thresholds are the limits this run used, which are the settings in force
	// rather than the ones the build used.
	Thresholds audit.Thresholds `json:"thresholds"`
	// GameBuildID is the game build the cache belongs to.
	GameBuildID string `json:"gameBuildId"`
}

// ErrNoMerge reports that there is no kept merge to audit.
var ErrNoMerge = errors.New("no merged MXML in the workspace; run `nmsbonker build` first")

/*
Audit re-checks the merged reward tables the last build left in the workspace
(R1.5).

The plan is rebuilt from the current settings because the attribution needs the
edit blocks in build order, and because a mod list that has changed since the
build is worth noticing: the audit then reports against a merge those mods did
not produce, and the mismatch shows up as a table it cannot attribute rather
than as a wrong answer stated confidently.
*/
func Audit(ctx context.Context, req AuditRequest) (AuditResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return AuditResult{}, err
	}
	modName := req.ModName
	if modName == "" {
		modName = s.cfg.ModName
	}

	out := AuditResult{Thresholds: s.cfg.Audit.Thresholds()}
	out.GameBuildID = s.auditBuildID()
	out.CacheDir = filepath.Join(s.paths.Cache, "game", out.GameBuildID)
	out.MergeDir = filepath.Join(s.paths.Workspace, modName+".work", "mxml")
	if _, err := os.Stat(out.MergeDir); err != nil {
		return out, fmt.Errorf("%w (looked in %s)", ErrNoMerge, out.MergeDir)
	}

	scripts, err := s.loadScripts(ctx, false)
	if err != nil {
		return out, err
	}
	plan := build.NewPlan(scripts)
	if len(plan.Enabled) == 0 {
		return out, ErrNoEnabledMods
	}

	entries := cache.Read(out.CacheDir)
	sources := make(map[string]build.Source, len(entries))
	for key, entry := range entries {
		sources[key] = build.Source{
			MXML: cache.Path(out.CacheDir, entry), Internal: entry.Internal,
		}
	}

	run, err := build.AuditKeptMerge(build.AuditOptions{
		Plan: plan, Sources: sources, Workspace: s.paths.Workspace,
		ModName: modName, Thresholds: out.Thresholds,
	})
	if err != nil {
		return out, err
	}
	out.Run = run
	req.Events.logf(LevelInfo, "audited %d table(s) in %s: %s",
		len(run.Checked), run.Duration.Round(1e6), run.Result.Summary())
	for _, missing := range run.Missing {
		req.Events.logf(LevelWarn, "%s has no merged MXML in %s", missing, out.MergeDir)
	}
	return out, nil
}

/*
auditBuildID names the cache tree the pristine files live in.

The Steam manifest is the authority, exactly as it is for a build. When the game
is not where the settings say -- an external drive that is not mounted, most
often -- the last report's game build is used instead, because the question
"what did that build come out at" has an answer that does not need the game to
be present.
*/
func (s *session) auditBuildID() string {
	if s.installErr == nil && s.install != nil && s.install.BuildID != "" {
		return s.install.BuildID
	}
	if res, err := report.Load(filepath.Join(s.reportsDir(), "latest", "report.json")); err == nil {
		if res.GameBuildID != "" {
			return res.GameBuildID
		}
	}
	return "unknown"
}
