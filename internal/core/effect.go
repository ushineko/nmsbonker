package core

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/ushineko/nmsbonker/internal/build/cache"
	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/modscript"
)

/*
Library hygiene signals (spec 005 R4).

A mod library accumulates. Scripts written for a game version three updates ago
still load, still produce edit blocks, and still report OK on the two of forty
keys they can still find; a script whose whole purpose was a x10 loot multiplier
sits under a built-in that does the same thing and multiplies on top of it. Both
are invisible in a report that says "12 applied, 30 skipped" and nothing else.

So `mods check` and the Mods detail dialog carry a note derived mechanically
from the last build: how much of the script landed, and whether an enabled
built-in is editing the same keys. It is information, not a recommendation --
nothing here disables anything or suggests that it should be disabled, because
"overlaps" is sometimes exactly what the user arranged.
*/

// Effect notes, in the order they are tested (R4.1).
const (
	// EffectNoEdits is a script none of whose edits applied.
	EffectNoEdits = "no effective edits"
	// EffectMostlyFailing is one that applied fewer edits than it skipped,
	// which is what a script written for an older game version looks like.
	EffectMostlyFailing = "mostly failing"
	// EffectStructuralSkipped is one whose ADD or REMOVE was dropped to get the
	// merged file to recompile.
	EffectStructuralSkipped = "structural edits skipped"
)

/*
lastReport reads the last build's report, or nil.

Nil is an ordinary answer -- nothing has been built yet -- and every caller
treats it as "no signal" rather than as an error: a script's effectiveness is a
fact about a build, and before the first build there is no fact to state.
*/
func (s *session) lastReport() *report.Result {
	res, err := report.Load(filepath.Join(s.reportsDir(), "latest", "report.json"))
	if err != nil {
		return nil
	}
	return res
}

/*
signature is the set of (file, key) pairs a script edits (R4.1).

The key is a VALUE_CHANGE_TABLE key where there is one, and the wrapper or
currency name where the edit is a WRAPPER_MULT or a CURRENCY_MULT -- those name
the block they multiply rather than the field, and comparing them by field would
miss the overlap that matters most, two mods multiplying the same reward.

A block with neither (a bare ADD or REMOVE) contributes the file alone, which is
deliberately weak: two mods adding different entries to the reward table do not
overlap in any sense the user can act on.
*/
func signature(def *modscript.Definition) map[string]bool {
	out := map[string]bool{}
	if def == nil {
		return out
	}
	for _, mod := range def.Modifications {
		for _, ch := range mod.Changes {
			for _, src := range ch.Sources {
				file := cache.Key(src)
				for _, blk := range ch.Blocks {
					for _, vc := range blk.ValueChanges {
						out[file+"\x00"+vc.Key] = true
					}
					if blk.WrapperMult != nil {
						out[file+"\x00"+blk.WrapperMult.Wrapper] = true
					}
					if blk.CurrencyMult != nil {
						out[file+"\x00GcRewardMoney:"+blk.CurrencyMult.Currency] = true
					}
				}
			}
		}
	}
	return out
}

// overlaps reports which of the enabled built-ins edit the same (file, key)
// pairs as this script, in name order.
func overlaps(name string, own map[string]bool, builtins map[string]map[string]bool) []string {
	var out []string
	for builtin, keys := range builtins {
		if builtin == name {
			continue
		}
		for k := range own {
			if keys[k] {
				out = append(out, builtin)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

/*
effectNote is the one-line verdict on how much of a script is doing anything
(R4.1).

Derived only from counts the last build recorded, so it says nothing a reader
could not work out from the report table -- which is the point: the report table
has one row per mod and forty rows, and the fact that this particular script
applied nothing is the fact that gets lost in it.
*/
func effectNote(row *report.ModResult, degraded bool, overlapping []string) string {
	var parts []string
	switch {
	case row == nil:
		// No build to judge against. Overlap is still worth saying: it is a
		// property of the scripts, not of a build.
	case row.Applied == 0:
		parts = append(parts, EffectNoEdits)
	case row.Skipped >= row.Applied:
		parts = append(parts, EffectMostlyFailing)
	}
	if degraded {
		parts = append(parts, EffectStructuralSkipped)
	}
	for _, name := range overlapping {
		parts = append(parts, "overlaps built-in "+name)
	}
	return strings.Join(parts, "; ")
}

// degradedFor reports whether the last build dropped this mod's structural
// edits from any file.
func degradedFor(res *report.Result, name string) bool {
	if res == nil {
		return false
	}
	for _, d := range res.Degraded {
		for _, m := range d.Mods {
			if m == name {
				return true
			}
		}
	}
	return false
}

// modRow finds a mod's row in the last report, or nil.
func modRow(res *report.Result, name string) *report.ModResult {
	if res == nil {
		return nil
	}
	for i := range res.Mods {
		if res.Mods[i].Name == name {
			return &res.Mods[i]
		}
	}
	return nil
}
