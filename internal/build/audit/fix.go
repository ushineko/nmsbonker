package audit

import (
	"fmt"
	"math"
	"slices"
	"sort"
)

/*
The audit's fix (spec 017): which multipliers to lower, and to what, so that
the flagged amounts come back under the limits.

The audit already knows every flagged block's chain of contributors and what
each one multiplied the amount by. When a contributor is a built-in tweak whose
parameter declares the category it scales, lowering that parameter lowers the
amount in proportion, so the fix is arithmetic rather than advice. A library
script has no declared parameter and is never touched; it is named instead.

Only multipliers are lowered, never limits raised and never caps set: a cap
cannot fix a ratio flag (a small reward multiplied x150 is far below any cap),
and a limit is the user's judgement of what is too much.
*/

// Knob is one parameter the fix may lower.
type Knob struct {
	Mod        string   `json:"mod"`
	ModLabel   string   `json:"modLabel"`
	Param      string   `json:"param"`
	ParamLabel string   `json:"paramLabel"`
	Categories []string `json:"categories"`
	// Current is the value the audited build used; Min is the lowest the
	// parameter accepts.
	Current float64 `json:"current"`
	Min     float64 `json:"min"`
}

// Change is one parameter the fix lowers.
type Change struct {
	Knob
	New float64 `json:"new"`
	// Flags is how many flagged amounts this change brings down.
	Flags int `json:"flags"`
}

// Unfixed is a mod whose multiplying the fix cannot undo, with how many flagged
// amounts still name it after the fix.
type Unfixed struct {
	Mod   string `json:"mod"`
	Flags int    `json:"flags"`
}

// Plan is the fix: the changes, and what they leave over the limits.
type Plan struct {
	Changes []Change `json:"changes"`
	// Fixed is how many flagged amounts the changes bring under the limits.
	Fixed int `json:"fixed"`
	// Remaining is how many stay over, and Unfixed names the mods behind them:
	// a library script, or a built-in already at its lowest setting.
	Remaining int       `json:"remaining"`
	Unfixed   []Unfixed `json:"unfixed,omitempty"`
}

// maxPasses bounds the narrowing loop. Each pass lowers at least one knob or
// stops, and a knob only goes down, so this is never reached in practice.
const maxPasses = 64

/*
Recommend works out the plan for a set of flags.

Each flag's predicted amount is its stock amount run through its contributors
again, with a lowered knob's factor taken as min(new value, old factor): exact
when nothing before it moved, because a factor below the knob's value means a
cap was holding it and a cap still holds. While any flag is predicted over, the
latest contributor in its chain with room to go down is lowered to the largest
value that brings it under, or to its minimum, in which case the next pass
moves on to the one before it. A knob shared by many flags takes the lowest
value any of them needs.
*/
func Recommend(flags []Flag, th Thresholds, knobs []Knob) Plan {
	index := map[string]int{}
	for i, k := range knobs {
		for _, c := range k.Categories {
			index[k.Mod+"\x00"+c] = i
		}
	}
	knobOf := func(mod, category string) (int, bool) {
		i, ok := index[mod+"\x00"+category]
		return i, ok
	}

	value := make([]float64, len(knobs))
	for i, k := range knobs {
		value[i] = k.Current
	}

	for range maxPasses {
		lowered := false
		for _, f := range flags {
			need := excess(f, predict(f, knobs, value, knobOf), th)
			if need <= within {
				continue
			}
			for _, c := range slices.Backward(f.Contributors) {
				i, ok := knobOf(c.Mod, f.Category)
				if !ok || value[i] <= knobs[i].Min {
					continue
				}
				target := math.Max(knobs[i].Min, math.Floor(value[i]/need))
				if target < value[i] {
					value[i] = target
					lowered = true
				}
				break
			}
		}
		if !lowered {
			break
		}
	}

	plan := Plan{}
	brought := make([]int, len(knobs))
	unfixed := map[string]int{}
	for _, f := range flags {
		if excess(f, predict(f, knobs, value, knobOf), th) > within {
			plan.Remaining++
			for _, c := range f.Contributors {
				if i, ok := knobOf(c.Mod, f.Category); !ok || value[i] <= knobs[i].Min {
					if c.Factor != 1 {
						unfixed[c.Mod]++
					}
				}
			}
			continue
		}
		plan.Fixed++
		for _, c := range f.Contributors {
			if i, ok := knobOf(c.Mod, f.Category); ok && value[i] < knobs[i].Current {
				brought[i]++
			}
		}
	}
	for i, k := range knobs {
		if value[i] < k.Current {
			plan.Changes = append(plan.Changes, Change{Knob: k, New: value[i], Flags: brought[i]})
		}
	}
	for mod, n := range unfixed {
		plan.Unfixed = append(plan.Unfixed, Unfixed{Mod: mod, Flags: n})
	}
	sort.Slice(plan.Unfixed, func(a, b int) bool {
		if plan.Unfixed[a].Flags != plan.Unfixed[b].Flags {
			return plan.Unfixed[a].Flags > plan.Unfixed[b].Flags
		}
		return plan.Unfixed[a].Mod < plan.Unfixed[b].Mod
	})
	return plan
}

// predict is a flag's AmountMax with the proposed knob values.
func predict(f Flag, knobs []Knob, value []float64, knobOf func(string, string) (int, bool)) float64 {
	if f.PristineMax <= 0 || len(f.Contributors) == 0 {
		return f.MergedMax
	}
	v := f.PristineMax
	for _, c := range f.Contributors {
		if c.Factor <= 0 {
			// The amount came from zero; there is no factor to scale, so
			// the contributor's own result stands.
			v = c.Max
			continue
		}
		factor := c.Factor
		if i, ok := knobOf(c.Mod, f.Category); ok && value[i] < knobs[i].Current {
			factor = math.Min(value[i], c.Factor)
		}
		v *= factor
	}
	return v
}

// within is the excess an amount may have and still count as under its limit:
// 1, plus room for the rounding in a product of float factors.
const within = 1 + 1e-9

// excess is how many times over its tightest limit an amount is: 1 or below
// means within every limit.
func excess(f Flag, amount float64, th Thresholds) float64 {
	need := 0.0
	if limit, ok := th.limit(f.Category); ok {
		need = amount / limit
	}
	if th.MaxRatio > 0 && f.PristineMax > 0 {
		need = math.Max(need, amount/f.PristineMax/th.MaxRatio)
	}
	return need
}

// Text is how every front end words one change.
func (c Change) Text() string {
	mod := c.ModLabel
	if mod == "" {
		mod = c.Mod
	}
	param := c.ParamLabel
	if param == "" {
		param = c.Param
	}
	return fmt.Sprintf("%s: %s %s -> %s (%d flagged amount(s))",
		mod, param, Amount(c.Current), Amount(c.New), c.Flags)
}

// Text is how every front end words a mod the fix cannot bring down.
func (u Unfixed) Text() string {
	return fmt.Sprintf("%s: %d flagged amount(s) it multiplies stay over; disable or re-tune it by hand",
		u.Mod, u.Flags)
}

// Lines are the headline, then one line per change and per mod left over: the
// plan as every front end prints it.
func (p Plan) Lines() []string {
	out := []string{p.Headline()}
	for _, c := range p.Changes {
		out = append(out, "• "+c.Text())
	}
	for _, u := range p.Unfixed {
		out = append(out, "• "+u.Text())
	}
	return out
}

// Headline says in one line what the plan does.
func (p Plan) Headline() string {
	switch {
	case len(p.Changes) == 0 && p.Remaining == 0:
		return "Nothing to fix."
	case len(p.Changes) == 0:
		return "No built-in multiplier can bring these down; the mods behind them are below."
	case p.Remaining == 0:
		return fmt.Sprintf("Lowering %d multiplier(s) brings all %d flagged amount(s) under the limits.",
			len(p.Changes), p.Fixed)
	default:
		return fmt.Sprintf("Lowering %d multiplier(s) brings %d of %d flagged amount(s) under the limits.",
			len(p.Changes), p.Fixed, p.Fixed+p.Remaining)
	}
}
