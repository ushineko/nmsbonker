package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ushineko/nmsbonker/internal/build/audit"
	"github.com/ushineko/nmsbonker/internal/tweaks"
)

/*
The amount audit's fix (spec 017).

The audit names the mods behind every flagged amount; this turns that into the
parameter changes that bring the amounts back under the limits, and, asked to,
saves them. It lowers built-in multipliers only. A library script is named in
the plan and never edited, and nothing here raises a limit or rebuilds: the new
values take effect at the next build, which the front ends ask for.
*/

// AuditFixRequest works out, and optionally applies, the audit's fix.
type AuditFixRequest struct {
	Request
	// ModName is as for AuditRequest.
	ModName string
	// Apply saves the changes; without it the plan is only reported.
	Apply bool
}

// AuditFixResult is the plan, judged against the limits in force now.
type AuditFixResult struct {
	Audit AuditResult `json:"audit"`
	Plan  audit.Plan  `json:"plan"`
	// Applied reports that the changes were saved.
	Applied bool `json:"applied"`
}

// ErrParamsChangedSinceBuild reports that a multiplier the fix would lower no
// longer has the value the audited build used, so the audit's arithmetic is
// about a build that is not the one the settings describe.
var ErrParamsChangedSinceBuild = errors.New(
	"tweak parameters have changed since the last build; rebuild, then fix")

// AuditFix re-runs the audit and works out which multipliers to lower.
func AuditFix(ctx context.Context, req AuditFixRequest) (AuditFixResult, error) {
	res, err := Audit(ctx, AuditRequest{Request: req.Request, ModName: req.ModName})
	if err != nil {
		return AuditFixResult{}, err
	}
	out := AuditFixResult{Audit: res}
	if res.Run.Result == nil || len(res.Run.Result.Flags) == 0 {
		return out, nil
	}

	out.Plan, err = PlanAuditFix(ctx, PlanAuditFixRequest{Request: req.Request, Result: res.Run.Result})
	if err != nil || !req.Apply || len(out.Plan.Changes) == 0 {
		return out, err
	}
	s, err := open(req.Request)
	if err != nil {
		return out, err
	}
	for _, c := range out.Plan.Changes {
		s.cfg.SetParam(c.Mod, c.Param, c.New)
	}
	if err := s.cfg.Save(); err != nil {
		return out, err
	}
	out.Applied = true
	req.Events.logf(LevelInfo, "lowered %d multiplier(s); rebuild to apply them", len(out.Plan.Changes))
	return out, nil
}

// PlanAuditFixRequest plans the fix for an audit the caller already holds:
// the window's, which may be the last build's or a re-check's.
type PlanAuditFixRequest struct {
	Request
	Result *audit.Result
}

// PlanAuditFix works out which multipliers to lower for an audit result,
// without re-running the audit or saving anything.
func PlanAuditFix(_ context.Context, req PlanAuditFixRequest) (audit.Plan, error) {
	if req.Result == nil || len(req.Result.Flags) == 0 {
		return audit.Plan{}, nil
	}
	s, err := open(req.Request)
	if err != nil {
		return audit.Plan{}, err
	}
	knobs, stale := s.auditKnobs()
	if len(stale) > 0 {
		return audit.Plan{}, fmt.Errorf("%w (%s)", ErrParamsChangedSinceBuild, strings.Join(stale, ", "))
	}
	return audit.Recommend(req.Result.Flags, req.Result.Thresholds, knobs), nil
}

/*
auditKnobs lists the enabled built-ins' parameters that declare what they
scale, at the values the last build used.

stale names any of them whose value now differs from the build's, because then
the audit's factors describe a build the settings no longer produce.
*/
func (s *session) auditKnobs() (knobs []audit.Knob, stale []string) {
	built, _ := s.builtParams()
	for _, m := range s.cfg.Mods {
		if !m.Enabled || !tweaks.Has(m.Name) {
			continue
		}
		tw, ok := tweaks.Get(m.Name)
		if !ok {
			continue
		}
		overrides := s.cfg.ParamsFor(m.Name)
		for _, p := range tw.Params {
			if len(p.Scales) == 0 {
				continue
			}
			now := p.Default
			if v, ok := overrides[p.Name]; ok {
				now = v
			}
			was := p.Default
			if v, ok := built[m.Name][p.Name]; ok {
				was = v
			}
			if was != now {
				stale = append(stale, m.Name+"."+p.Name)
				continue
			}
			knobs = append(knobs, audit.Knob{
				Mod: m.Name, ModLabel: tw.Header.Name, Param: p.Name, ParamLabel: p.Label,
				Categories: p.Scales, Current: now, Min: p.Min,
			})
		}
	}
	return knobs, stale
}
