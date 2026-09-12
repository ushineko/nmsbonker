package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/tweaks"
)

/*
The built-in tweaks as an operation (spec 004 R2).

A tweak is a mod like any other -- it takes part in the one global build order,
it can be enabled and disabled, and the build loads it through the same Lua
sandbox. What is different is that its parameters are declared, so a front end
can offer a labelled control with a range instead of asking the user to edit a
Lua file, and that it is embedded, so there is something to build on a machine
that has downloaded nothing.

Enabling and disabling go through SetModEnabled; there is no second code path
for it. What lives here is the parameter half and the listing the Tweaks section
is drawn from.
*/

// TweakParam is one parameter with everything a control needs to draw itself.
type TweakParam struct {
	modscript.Param
	// Overridden reports that the value in force came from the settings rather
	// than from the script, which is what makes Reset worth offering.
	Overridden bool `json:"overridden"`
}

// TweakInfo is one built-in as the Tweaks section shows it (R2.1).
type TweakInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Group   string `json:"group"`
	Desc    string `json:"desc"`
	Enabled bool   `json:"enabled"`
	// Order is the mod's 1-based position in the build order, which is the
	// conflict-resolution rule it shares with every other mod.
	Order int `json:"order"`
	// Shadowed reports a library script of the same name, which is ignored.
	Shadowed bool         `json:"shadowed,omitempty"`
	Params   []TweakParam `json:"params"`
	// Unbuilt reports that a parameter has changed since the last successful
	// build, so the mod folder on disk was not made from these values (R2.1).
	Unbuilt bool `json:"unbuilt"`
}

// ListTweaksRequest asks for the built-ins and their parameters.
type ListTweaksRequest struct {
	Request
}

// ListTweaksResult is every built-in, in build order.
type ListTweaksResult struct {
	Tweaks []TweakInfo `json:"tweaks"`
	// Groups is the display order of the groups actually present.
	Groups []string `json:"groups"`
	// Unbuilt is true when any tweak's parameters differ from the last build's.
	Unbuilt bool `json:"unbuilt"`
	// LastBuild is when the report the comparison was made against was written;
	// zero when nothing has been built.
	LastBuild string `json:"lastBuild,omitempty"`
}

/*
ListTweaks describes every built-in tweak (R2.1).

Built in build order rather than in the order tweaks.Names() declares: a user
who has moved MoneyAndNanites5x below NaniteRewardBuff has said something about
how the two compound, and a listing that renumbered them would be describing a
build that will not happen.
*/
func ListTweaks(_ context.Context, req ListTweaksRequest) (ListTweaksResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ListTweaksResult{}, err
	}
	list, changed, err := reconcile(s)
	if err != nil {
		return ListTweaksResult{}, err
	}
	if changed {
		if err := s.cfg.Save(); err != nil {
			return ListTweaksResult{}, err
		}
	}

	built, lastBuild := s.builtParams()
	out := ListTweaksResult{LastBuild: lastBuild}
	present := map[string]bool{}
	for i, m := range list.Mods {
		if m.Source != SourceBuiltin {
			continue
		}
		tw, ok := tweaks.Get(m.Name)
		if !ok {
			continue
		}
		info := TweakInfo{
			Name: m.Name, Title: tw.Header.Name, Group: tw.Header.Group,
			Desc: tw.Header.Desc, Enabled: m.Enabled, Order: i + 1,
			Shadowed: m.Shadowed,
		}
		overrides := s.cfg.ParamsFor(m.Name)
		for _, p := range tw.Params {
			tp := TweakParam{Param: p}
			if v, ok := overrides[p.Name]; ok {
				tp.Current, tp.Overridden = v, true
			}
			info.Params = append(info.Params, tp)
		}
		info.Unbuilt = paramsDiffer(built[m.Name], overrides)
		out.Unbuilt = out.Unbuilt || info.Unbuilt
		present[info.Group] = true
		out.Tweaks = append(out.Tweaks, info)
	}
	for _, g := range append(tweaks.Groups, tweaks.GroupOther) {
		if present[g] {
			out.Groups = append(out.Groups, g)
		}
	}
	return out, nil
}

/*
builtParams reads the parameters the last successful build used.

Absent report, or a report from before this field existed, means "no idea":
returning an empty map makes every override look unbuilt, which is the safe
direction. Claiming a build is current when it might not be sends the user into
the game with a mod folder that does not match what the window says.
*/
func (s *session) builtParams() (map[string]map[string]float64, string) {
	path := filepath.Join(s.reportsDir(), "latest", "report.json")
	res, err := report.Load(path)
	if err != nil || res == nil {
		return nil, ""
	}
	return res.Params, res.Generated.Format("2006-01-02 15:04")
}

// paramsDiffer compares two override sets, treating absent and empty as equal.
func paramsDiffer(built, current map[string]float64) bool {
	if len(built) != len(current) {
		return true
	}
	for name, v := range current {
		if was, ok := built[name]; !ok || was != v {
			return true
		}
	}
	return false
}

// SetTweakParamRequest assigns one parameter (R2.3).
//
// Name is any mod, not only a built-in: the Mods section offers the same
// controls for a library script's detected parameters (R2.2), and one operation
// serving both is what keeps them behaving the same way.
type SetTweakParamRequest struct {
	Request
	Name  string
	Param string
	Value float64
}

// SetTweakParamResult reports the change.
type SetTweakParamResult struct {
	Name    string  `json:"name"`
	Param   string  `json:"param"`
	Old     float64 `json:"old"`
	New     float64 `json:"new"`
	Default float64 `json:"default"`
	// Clamped reports that the value was outside the declared bounds and was
	// brought back inside them.
	Clamped bool `json:"clamped,omitempty"`
}

// ErrNoSuchParam reports a parameter the named script does not declare.
var ErrNoSuchParam = errors.New("no such parameter")

/*
SetTweakParam records one parameter override (R1.3, R2.3).

The value is clamped to the declared bounds rather than refused. A slider cannot
produce an out-of-range value, so the only way to get one is by typing it or by
hand-editing the settings; taking the nearest legal value and saying so is more
useful than an error, and it keeps the CLI and the window agreeing about what a
number means. A library script has no declared bounds and is not clamped.
*/
func SetTweakParam(_ context.Context, req SetTweakParamRequest) (SetTweakParamResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return SetTweakParamResult{}, err
	}
	_, params, err := s.findParam(req.Name, req.Param)
	if err != nil {
		return SetTweakParamResult{}, err
	}

	out := SetTweakParamResult{
		Name: req.Name, Param: req.Param, Old: params.Current,
		New: req.Value, Default: params.Default,
	}
	if params.Bounded {
		clamped := math.Min(math.Max(req.Value, params.Min), params.Max)
		if clamped != req.Value {
			out.New, out.Clamped = clamped, true
		}
	}
	if params.Kind == modscript.ParamInt {
		out.New = math.Round(out.New)
	}
	s.cfg.SetParam(req.Name, req.Param, out.New)
	if err := s.cfg.Save(); err != nil {
		return out, err
	}
	return out, nil
}

// ResetTweakRequest restores the script's own values (R2.3).
//
// An empty Param resets every parameter of the mod.
type ResetTweakRequest struct {
	Request
	Name  string
	Param string
}

// ResetTweakResult lists what was restored.
type ResetTweakResult struct {
	Name  string   `json:"name"`
	Reset []string `json:"reset"`
}

// ResetTweak drops overrides so the script's own numbers apply again.
func ResetTweak(_ context.Context, req ResetTweakRequest) (ResetTweakResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ResetTweakResult{}, err
	}
	out := ResetTweakResult{Name: req.Name}
	if req.Param != "" {
		if _, _, err := s.findParam(req.Name, req.Param); err != nil {
			return out, err
		}
		if s.cfg.ResetParam(req.Name, req.Param) {
			out.Reset = []string{req.Param}
		}
	} else {
		if _, err := s.findMod(req.Name); err != nil {
			return out, err
		}
		out.Reset = s.cfg.ResetParams(req.Name)
	}
	if err := s.cfg.Save(); err != nil {
		return out, err
	}
	return out, nil
}

// findMod locates one entry in the reconciled build order.
func (s *session) findMod(name string) (ModInfo, error) {
	list, _, err := reconcile(s)
	if err != nil {
		return ModInfo{}, err
	}
	for _, m := range list.Mods {
		if strings.EqualFold(m.Name, name) {
			return m, nil
		}
	}
	return ModInfo{}, fmt.Errorf("no mod named %q is in the build order", name)
}

// findParam locates one declared parameter of one mod, with the value in force.
func (s *session) findParam(name, param string) (ModInfo, modscript.Param, error) {
	m, err := s.findMod(name)
	if err != nil {
		return ModInfo{}, modscript.Param{}, err
	}
	if m.Status == ModMissing {
		return m, modscript.Param{}, fmt.Errorf("%s has no .lua in the library", m.Name)
	}
	_, params, err := s.scriptSource(m)
	if err != nil {
		return m, modscript.Param{}, err
	}
	names := make([]string, 0, len(params))
	for _, p := range params {
		if p.Name == param {
			return m, p, nil
		}
		names = append(names, p.Name)
	}
	if len(names) == 0 {
		return m, modscript.Param{}, fmt.Errorf("%w: %s declares none", ErrNoSuchParam, m.Name)
	}
	return m, modscript.Param{}, fmt.Errorf("%w: %s has no %q (it declares %s)",
		ErrNoSuchParam, m.Name, param, strings.Join(names, ", "))
}
