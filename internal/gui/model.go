package gui

import (
	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/core"
)

// The types and mappings in this file are the presentation layer's own: they
// say how to paint what core returns, which is a different question from what
// core returns. Keeping them apart is what stops a rendering decision from
// landing in the package both front ends share.

// Status ranks a value so a card or a table row can be read at a glance rather
// than parsed word by word.
type Status int

const (
	// StatusInfo is a plain fact with no judgement attached.
	StatusInfo Status = iota
	// StatusGood is a state the user wants to be in.
	StatusGood
	// StatusWarn is a state that needs an action but has broken nothing yet.
	StatusWarn
	// StatusBad is a state that is already costing the user something.
	StatusBad
)

// levelStatus ranks a core log line, which is what colours the build log.
func levelStatus(l core.Level) Status {
	switch l {
	case core.LevelWarn:
		return StatusWarn
	case core.LevelError:
		return StatusBad
	case core.LevelInfo:
		return StatusInfo
	}
	return StatusInfo
}

// compatStatus ranks the round-trip compatibility verdict (spec 002 R3.4).
//
// A mismatch is a warning rather than a failure on purpose: the build still
// runs, and the report names the file that proved the compiler disagrees with
// this install. Painting it red would say "you cannot build", which is not what
// core means by it.
func compatStatus(v string) Status {
	switch v {
	case core.CompatOK:
		return StatusGood
	case core.CompatMismatch:
		return StatusWarn
	case core.CompatNoCompiler:
		return StatusBad
	}
	return StatusInfo
}

// verdictStatus colours a report row. The five verdicts are the reference
// builder's and their meanings are in the report legend, which the Report
// section reproduces rather than paraphrasing.
func verdictStatus(v string) Status {
	switch v {
	case report.Working:
		return StatusGood
	case report.WorkingSkipped, report.WorkingStructural:
		return StatusWarn
	case report.Partial, report.NotBuilt:
		return StatusBad
	}
	return StatusInfo
}

// modsStateStatus ranks what GAMEDATA/MODS currently is.
//
// A symlink means the game reads mods through the link, so a deploy would
// write into whatever the link points at rather than into the game. That is a
// warning the Overview has to carry, because it is the one state where the
// obvious action does the wrong thing.
func modsStateStatus(state string) Status {
	switch state {
	case "dir":
		return StatusGood
	case "symlink":
		return StatusWarn
	case "absent":
		return StatusInfo
	}
	return StatusInfo
}

// reportLegend is the legend from BUILD_REPORT.md, reused verbatim rather than
// re-worded (R6). The report file and this section must not explain the same
// five verdicts differently.
const reportLegend = "WORKING all edits applied · WORKING~ applied, some keys not found " +
	"(renamed or removed by a game update — verify) · WORKING* applies structural " +
	"add/remove that recompiled — verify in game · PARTIAL value edits applied but " +
	"structural add/remove skipped · NOT BUILT nothing applied."
