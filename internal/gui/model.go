package gui

import (
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/logpane"

	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/core"
)

// The mappings in this file are the presentation layer's own: they say how to
// paint what core returns, which is a different question from what core
// returns. Keeping them apart is what stops a rendering decision from landing
// in the package both front ends share. The presentation type itself is the
// library's fd.Status; the domain rankers below map onto it.

// logLevel maps a core log level onto the log pane's, which is what colours a
// build line. The mapping lives here so the pane never learns a domain level.
func logLevel(l core.Level) logpane.Level {
	switch l {
	case core.LevelDebug:
		return logpane.Debug
	case core.LevelWarn:
		return logpane.Warn
	case core.LevelError:
		return logpane.Error
	case core.LevelInfo:
		return logpane.Info
	}
	return logpane.Info
}

// compatStatus ranks the round-trip compatibility verdict (spec 002 R3.4).
//
// A mismatch is a warning rather than a failure on purpose: the build still
// runs, and the report names the file that proved the compiler disagrees with
// this install. Painting it red would say "you cannot build", which is not what
// core means by it.
func compatStatus(v string) fd.Status {
	switch v {
	case core.CompatOK:
		return fd.StatusGood
	case core.CompatMismatch:
		return fd.StatusWarn
	case core.CompatNoCompiler:
		return fd.StatusBad
	}
	return fd.StatusInfo
}

// verdictStatus colours a report row. The five verdicts are the reference
// builder's and their meanings are in the report legend, which the Report
// section reproduces rather than paraphrasing.
func verdictStatus(v string) fd.Status {
	switch v {
	case report.Working:
		return fd.StatusGood
	case report.WorkingSkipped, report.WorkingStructural:
		return fd.StatusWarn
	case report.Partial, report.NotBuilt:
		return fd.StatusBad
	}
	return fd.StatusInfo
}

// modsStateStatus ranks what GAMEDATA/MODS currently is.
//
// A symlink means the game reads mods through the link, so a deploy would
// write into whatever the link points at rather than into the game. That is a
// warning the Overview has to carry, because it is the one state where the
// obvious action does the wrong thing.
func modsStateStatus(state string) fd.Status {
	switch state {
	case "dir":
		return fd.StatusGood
	case "symlink":
		return fd.StatusWarn
	case "absent":
		return fd.StatusInfo
	}
	return fd.StatusInfo
}

// reportLegend is the legend from BUILD_REPORT.md, reused verbatim rather than
// re-worded (R6). The report file and this section must not explain the same
// five verdicts differently.
const reportLegend = "WORKING all edits applied · WORKING~ applied, some keys not found " +
	"(renamed or removed by a game update — verify) · WORKING* applies structural " +
	"add/remove that recompiled — verify in game · PARTIAL value edits applied but " +
	"structural add/remove skipped · NOT BUILT nothing applied."
