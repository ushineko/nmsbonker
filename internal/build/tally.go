package build

import (
	"sort"

	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/mxml"
)

// modStats is what the report knows about one mod.
type modStats struct {
	applied  int
	skipped  int
	files    map[string]bool
	notFound []string
}

// tally accumulates the per-mod counters from the event stream, in plan order.
type tally struct {
	applied  int
	skipped  int
	mods     map[string]*modStats
	degraded map[string]bool
	dropped  map[string]bool
}

func newTally() *tally {
	return &tally{
		mods: map[string]*modStats{}, degraded: map[string]bool{}, dropped: map[string]bool{},
	}
}

func (t *tally) of(name string) *modStats {
	s, ok := t.mods[name]
	if !ok {
		s = &modStats{files: map[string]bool{}}
		t.mods[name] = s
	}
	return s
}

// record folds one event into the tallies.
func (t *tally) record(e mxml.Event) {
	switch e.Kind {
	case mxml.OK:
		t.applied++
		t.of(e.Mod).applied++
	case mxml.WARN:
		t.skipped++
		s := t.of(e.Mod)
		s.skipped++
		if e.NotFound != "" {
			s.notFound = append(s.notFound, e.NotFound)
		}
	case mxml.INFO:
	}
}

func (t *tally) file(mod, internal string) { t.of(mod).files[internal] = true }

func (t *tally) degrade(mods []string) {
	for _, m := range mods {
		t.degraded[m] = true
	}
}

func (t *tally) drop(mods []string) {
	for _, m := range mods {
		t.dropped[m] = true
	}
}

/*
rows derives each mod's verdict (R4.4).

The order of the tests is the reference order and it decides what a mod that is
both complex and degraded reports: PARTIAL wins over WORKING*, because "your
structural edits were dropped" is the more actionable of the two. A mod with
nothing applied is NOT BUILT whether or not it was also dropped, for the same
reason -- the user's next action is the same either way.
*/
func (t *tally) rows(plan *Plan) []report.ModResult {
	out := make([]report.ModResult, 0, len(plan.Enabled))
	for _, name := range plan.Enabled {
		s := t.of(name)
		files := make([]string, 0, len(s.files))
		for f := range s.files {
			files = append(files, f)
		}
		sort.Strings(files)

		var verdict string
		switch {
		case s.applied == 0:
			verdict = report.NotBuilt
		case t.degraded[name] || t.dropped[name]:
			verdict = report.Partial
		case plan.isComplex(name):
			verdict = report.WorkingStructural
		case s.skipped > 0:
			verdict = report.WorkingSkipped
		default:
			verdict = report.Working
		}
		out = append(out, report.ModResult{
			Name: name, Verdict: verdict, Applied: s.applied, Skipped: s.skipped,
			Files: files, NotFound: s.notFound,
		})
	}
	report.SortMods(out)
	return out
}
