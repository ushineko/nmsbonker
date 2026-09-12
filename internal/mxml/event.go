package mxml

import "fmt"

// Kind ranks an engine event the way the reference report did (R2.1).
type Kind uint8

// The three event kinds. INFO exists for the build's own "built X from N
// blocks" lines, which share the report stream with the engine's.
const (
	OK Kind = iota
	WARN
	INFO
)

// String names the kind for report.json.
func (k Kind) String() string {
	switch k {
	case OK:
		return "ok"
	case WARN:
		return "warn"
	default:
		return "info"
	}
}

/*
Event is one line of the build report, and one unit of the per-mod tally.

Detail is pre-rendered rather than being built from structured fields because
the exact wording is the parity oracle: the golden set holds 504 of these lines
and they are compared byte for byte. Line() is the only renderer.
*/
type Event struct {
	Kind Kind
	// Mod is the script the event belongs to. Empty on an INFO line.
	Mod string
	// Detail is the message, already formatted.
	Detail string
	// File is Python's basename of the MBIN_FILE_SOURCE, or "" when the event
	// is not about one file.
	File string
	// NotFound names the key a WARN could not find, which the report groups
	// per mod. Empty for every other warning.
	NotFound string
}

// Line renders the event exactly as the reference Report did (R2.1).
func (e Event) Line() string {
	switch e.Kind {
	case OK:
		return "   OK  " + e.Mod + ": " + e.Detail
	case WARN:
		return "  WARN " + e.Mod + ": " + e.Detail
	default:
		return "       " + e.Detail
	}
}

// ApplyContext names who is editing what, for the events Apply produces.
type ApplyContext struct {
	// Mod is the script's name as the report shows it.
	Mod string
	// Source is the MBIN_FILE_SOURCE spelling from the script, backslashes and
	// all. Report lines quote its basename.
	Source string
}

// File is the basename the report quotes.
func (c ApplyContext) File() string { return Base(c.Source) }

func (c ApplyContext) ok(format string, args ...any) Event {
	return Event{Kind: OK, Mod: c.Mod, Detail: fmt.Sprintf(format, args...), File: c.File()}
}

func (c ApplyContext) warn(format string, args ...any) Event {
	return Event{Kind: WARN, Mod: c.Mod, Detail: fmt.Sprintf(format, args...), File: c.File()}
}

// Info builds a report line with no mod attached, the shape the build uses for
// its own "built X" lines.
func Info(format string, args ...any) Event {
	return Event{Kind: INFO, Detail: fmt.Sprintf(format, args...)}
}
