/*
Package build merges every enabled mod's edits into one collision-free set of
MBINs (spec 002 R4).

The shape of the pipeline is the reference builder's and the reason for it is worth
stating: two mods that edit the same game file cannot each ship their own copy,
because the game loads one of them and the other's edits vanish. So the unit of
work is the *target* -- one game file -- and every mod's edits to it are applied
in the configured order into a single merged document, which is then gated on a
clean MBINCompiler recompile. A file the compiler rejects is never shipped.
*/
package build

import (
	"sort"
	"strings"

	"github.com/ushineko/nmsbonker/internal/build/cache"
	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/mxml"
)

// Item is one mod's one edit to one file.
type Item struct {
	Mod string
	// Source is the spelling the script used, which the report quotes.
	Source string
	Block  *modscript.Block
}

// Target is one game file and every edit destined for it, in build order.
type Target struct {
	// Key is cache.Key of the source: the identity two spellings share.
	Key string
	// Source is the first spelling seen, which decides which cached file is
	// read and what a "no cached MXML" warning names.
	Source string
	Items  []Item
}

// ModsFor lists the mods whose targets name a source, in build order; nil when
// no target does. For the report's account of a file the compiler refused.
func (p *Plan) ModsFor(source string) []string {
	for _, t := range p.Targets {
		if t.Source == source {
			return t.Mods()
		}
	}
	return nil
}

// Mods lists the contributing scripts in order, de-duplicated.
func (t *Target) Mods() []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range t.Items {
		if !seen[it.Mod] {
			seen[it.Mod] = true
			out = append(out, it.Mod)
		}
	}
	return out
}

// Script is one configured mod and the definition that loaded for it.
//
// Err carries a load failure rather than being handled by the caller because
// the reference builder reported those failures in the build report, in build
// order, ahead of everything else: a mod whose .lua has gone missing has to
// appear in the same list as one whose edits did not apply.
type Script struct {
	Name    string
	Enabled bool
	Def     *modscript.Definition
	Err     error
	// Missing is true when the library has no file for this entry at all.
	Missing bool
}

// Plan is the work one build will do (R4.1).
type Plan struct {
	// Targets are in first-seen order, which is the order the report follows.
	Targets []*Target
	// Enabled lists the enabled mod names in build order; the report has a row
	// for each, including the ones that did nothing.
	Enabled []string
	// Complex lists mods carrying an ADD or REMOVE key, in first-seen order.
	// Verdict WORKING* comes from this, and so does the retry set when a merged
	// file will not recompile.
	Complex []string
	// Issues are the load failures, as report events, in build order.
	Issues []mxml.Event
	// Unsupported lists every script key the engine ignored, sorted.
	Unsupported []string
	// Sources lists every distinct MBIN_FILE_SOURCE, for the cache to produce.
	Sources []string
}

/*
NewPlan groups every enabled script's edits by the file they target (R4.1).

Named NewPlan rather than Plan because the result type carries that name; the
spec's `build.Plan(defs, order)` is this function, with the mod order and the
load failures folded into one input so that a script that would not load keeps
its place in the build order instead of being reported out of sequence.
*/
func NewPlan(scripts []Script) *Plan {
	plan := &Plan{}
	byKey := map[string]*Target{}
	complexSeen := map[string]bool{}
	unsupported := map[string]bool{}
	sourceSeen := map[string]bool{}

	for _, s := range scripts {
		if !s.Enabled {
			continue
		}
		plan.Enabled = append(plan.Enabled, s.Name)

		switch {
		case s.Missing:
			plan.Issues = append(plan.Issues, mxml.Event{
				Kind: mxml.WARN, Mod: s.Name, Detail: ".lua not found",
			})
			continue
		case s.Err != nil:
			plan.Issues = append(plan.Issues, mxml.Event{
				Kind: mxml.WARN, Mod: s.Name, Detail: "dump failed: " + s.Err.Error(),
			})
			continue
		case s.Def == nil:
			continue
		}

		for _, k := range s.Def.UnsupportedKeys() {
			unsupported[k] = true
		}
		for _, mod := range s.Def.Modifications {
			for _, ch := range mod.Changes {
				for _, src := range ch.Sources {
					if !sourceSeen[src] {
						sourceSeen[src] = true
						plan.Sources = append(plan.Sources, src)
					}
					key := cache.Key(src)
					target, ok := byKey[key]
					if !ok {
						target = &Target{Key: key, Source: src}
						byKey[key] = target
						plan.Targets = append(plan.Targets, target)
					}
					for _, blk := range ch.Blocks {
						target.Items = append(target.Items, Item{Mod: s.Name, Source: src, Block: blk})
						// Key presence, not effect: a block with REMOVE = false
						// still marks the mod complex, because that is what the
						// reference builder's `"REMOVE" in blk` test did and what
						// the retry set is chosen by.
						if blk.Structural() && !complexSeen[s.Name] {
							complexSeen[s.Name] = true
							plan.Complex = append(plan.Complex, s.Name)
						}
					}
				}
			}
		}
	}

	plan.Unsupported = make([]string, 0, len(unsupported))
	for k := range unsupported {
		plan.Unsupported = append(plan.Unsupported, k)
	}
	sort.Strings(plan.Unsupported)
	return plan
}

// Blocks is how many edits the plan will attempt.
func (p *Plan) Blocks() int {
	n := 0
	for _, t := range p.Targets {
		n += len(t.Items)
	}
	return n
}

// isComplex reports whether a mod carries structural edits.
func (p *Plan) isComplex(name string) bool {
	for _, n := range p.Complex {
		if n == name {
			return true
		}
	}
	return false
}

// InternalUpper is the output path for a pak-internal path: upper-cased, with
// the separators the game uses.
func InternalUpper(internal string) string {
	return strings.ToUpper(strings.ReplaceAll(internal, `\`, "/"))
}
