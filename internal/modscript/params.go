package modscript

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

/*
Script parameters: the numbers at the top of a mod script (spec 004 R1.2).

Every AMUMSS script this project has met puts its tuning constants at the top,
before NMS_MOD_DEFINITION_CONTAINER, as `NAME = <number>` with a comment saying
what to change it to. That convention is the parameter contract: this file reads
those declarations and rewrites one of them, and nothing else in the pipeline
needs to know a parameter exists.

Two sources, in order of authority. A built-in tweak carries `-- @param` header
lines that name bounds and a label, which is what lets the window draw a slider
with a range rather than a text box. A library script downloaded from Nexus
carries no such thing, so its assignments are scanned instead and offered
unbounded: guessing a maximum for someone else's constant would be inventing a
fact.
*/

// Parameter kinds, as they are spelled in a `kind=` header attribute and in
// `tweaks list --json`. A parameter is integer unless its header says otherwise;
// most of these are multipliers and a fractional stack limit is not a thing.
//
// Spelled ParamInt/ParamFloat rather than reusing the Kind type above: that one
// ranks a scalar the engine read out of a change table, and these two names
// describe a widget and a number format. Sharing an identifier would tie a UI
// decision to the type the golden fixtures are compared through.
const (
	ParamInt   = "int"
	ParamFloat = "float"
)

// Param is one tunable constant in a mod script (R1.2).
type Param struct {
	// Name is the Lua global, which is also the key in config.params.
	Name string `json:"name"`
	// Label is what a front end shows. For a script with no header it is the
	// name, because inventing a friendlier one would be inventing a meaning.
	Label string  `json:"label"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Step  float64 `json:"step"`
	// Default is the script's own value: what Reset restores.
	Default float64 `json:"default"`
	// Kind is ParamInt or ParamFloat and decides both the widget and how a value
	// is written back into the script.
	Kind string `json:"kind"`
	// Current is the value in force, which is Default until an override says
	// otherwise. Filled in by whoever holds the overrides; Parameters leaves it
	// equal to Default.
	Current float64 `json:"current"`
	// Bounded reports whether Min/Max/Step mean anything. False for a library
	// script, whose parameters were inferred rather than declared, and which
	// therefore gets a numeric field instead of a slider (R2.2).
	Bounded bool `json:"bounded"`
}

// Header is the `-- @tweak` block a built-in carries (R1.1).
type Header struct {
	// Name is the display name; empty when the script has no @tweak line.
	Name string `json:"name,omitempty"`
	// Group is one of the eight groups the Tweaks section is laid out in.
	Group string `json:"group,omitempty"`
	// Desc is the @desc lines joined into one paragraph, falling back to the
	// script's first comment block.
	Desc string `json:"desc,omitempty"`
}

// number is a Lua numeric literal: integer, decimal or exponent form.
const number = `[-+]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][-+]?[0-9]+)?`

//nolint:gochecknoglobals // compiled once; these are constants in every sense but Go's
var (
	tweakLineRE = regexp.MustCompile(`^\s*--\s*@tweak\s+(.*)$`)
	descLineRE  = regexp.MustCompile(`^\s*--\s*@desc\s?(.*)$`)
	paramLineRE = regexp.MustCompile(`^\s*--\s*@param\s+([A-Za-z_][A-Za-z0-9_]*)\s*(.*)$`)
	attrRE      = regexp.MustCompile(`([a-z]+)\s*=\s*(?:"([^"]*)"|(` + number + `)|([A-Za-z]+))`)
	assignRE    = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(` + number + `)\s*(?:--.*)?$`)
	commentLine = regexp.MustCompile(`^\s*--\s?(.*)$`)
	containerRE = regexp.MustCompile(`\b` + ContainerName + `\b`)
	localAssign = regexp.MustCompile(`^\s*local\b`)
)

// ErrNoAssignment reports a parameter override for a name the script does not
// assign at the top level. The script changed shape under a saved override, and
// silently doing nothing would be a value the user set and the build ignored.
var ErrNoAssignment = errors.New("no top-level assignment to substitute")

/*
ParseHeader reads a script's `-- @tweak` block (R1.1).

The description falls back to the first comment block when there is no @desc,
because the library scripts and the built-ins all open with one and
throwing it away would leave the Tweaks section with a name and nothing else.
*/
func ParseHeader(src []byte) Header {
	var h Header
	var desc []string
	for _, line := range headLines(src) {
		if m := tweakLineRE.FindStringSubmatch(line); m != nil {
			for _, a := range attrRE.FindAllStringSubmatch(m[1], -1) {
				switch a[1] {
				case "name":
					h.Name = attrText(a)
				case "group":
					h.Group = attrText(a)
				}
			}
			continue
		}
		if m := descLineRE.FindStringSubmatch(line); m != nil {
			desc = append(desc, strings.TrimSpace(m[1]))
		}
	}
	if len(desc) > 0 {
		h.Desc = strings.Join(desc, " ")
		return h
	}
	h.Desc = firstCommentBlock(src)
	return h
}

/*
Parameters lists a script's tunable constants (R1.2).

Declared parameters win: a `-- @param` line names the bounds and the label, and
the assignment below it supplies the default. A script with no header falls back
to every top-level `IDENT = <number>` before the container, which is the shape
every one of these scripts is written in.
*/
func Parameters(src []byte) []Param {
	assigned := assignments(src)
	declared := declaredParams(src)

	if len(declared) > 0 {
		out := make([]Param, 0, len(declared))
		for _, p := range declared {
			if v, ok := assigned[p.Name]; ok {
				// The assignment is the script's own value and therefore the
				// authority on the default; a header that has drifted from it
				// would otherwise make Reset restore a number the script never
				// had.
				p.Default = v
			}
			p.Current = p.Default
			out = append(out, p)
		}
		return out
	}

	names := assignmentOrder(src)
	out := make([]Param, 0, len(names))
	for _, name := range names {
		v := assigned[name]
		out = append(out, Param{
			Name: name, Label: name, Default: v, Current: v,
			Kind: kindOf(v), Bounded: false,
		})
	}
	return out
}

// DuplicateAssignments lists the parameter names a script assigns more than
// once at the top level (R1's risk note).
//
// An override is a substitution of the *first* assignment, so a script that
// assigns the same global twice would take the second value and quietly ignore
// what the user set. `mods check` warns about that rather than leaving it to be
// discovered in game.
func DuplicateAssignments(src []byte) []string {
	counts := map[string]int{}
	var order []string
	for _, line := range headLines(src) {
		m := assignRE.FindStringSubmatch(line)
		if m == nil || localAssign.MatchString(line) {
			continue
		}
		if counts[m[1]] == 0 {
			order = append(order, m[1])
		}
		counts[m[1]]++
	}
	var out []string
	for _, name := range order {
		if counts[name] > 1 {
			out = append(out, name)
		}
	}
	return out
}

/*
Override rewrites one assignment in a copy of the script (R1.3).

Textual substitution of the assignment line, anchored to the first match, on an
in-memory copy. The file on disk is never touched: a built-in is embedded in the
binary and cannot be, and a library script belongs to whoever wrote it. The
alternative -- injecting the value through a metatable on _G -- loses to the
script's own assignment, which runs afterwards.

A name the script does not assign is an error rather than a no-op. The usual
cause is a script that changed shape under a saved override, and a build that
silently ignores a number the user set is worse than one that stops and says so.
*/
func Override(src []byte, name string, value float64, kind string) ([]byte, error) {
	lines := strings.Split(string(src), "\n")
	for i, line := range lines {
		if containerRE.MatchString(line) {
			break
		}
		m := assignRE.FindStringSubmatch(line)
		if m == nil || m[1] != name || localAssign.MatchString(line) {
			continue
		}
		// The trailing comment is kept: it is the script author's note about
		// what the number means, and it is still true of a different number.
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		rest := ""
		if idx := strings.Index(line, "--"); idx >= 0 {
			rest = "  " + line[idx:]
		}
		lines[i] = indent + name + " = " + FormatValue(value, kind) + rest
		return []byte(strings.Join(lines, "\n")), nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNoAssignment, name)
}

// OverrideAll applies every override to a copy of the script, in name order so
// that a failure reports the same parameter on every run.
func OverrideAll(src []byte, values map[string]float64, kinds map[string]string) ([]byte, error) {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	out := src
	for _, name := range names {
		next, err := Override(out, name, values[name], kinds[name])
		if err != nil {
			return nil, err
		}
		out = next
	}
	return out, nil
}

/*
FormatValue renders a parameter value as a Lua numeric literal.

An integer parameter is written without a decimal point, because that is what
the engine's integer/float split keys off: `AmountMin * 10` and `AmountMin * 10.0`
produce differently-shaped values in the merged MXML, and a slider dragged to a
whole number must not change the shape of the output.
*/
func FormatValue(v float64, kind string) string {
	if kind != ParamFloat && v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	text := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.Contains(text, ".") {
		// A float parameter keeps its point so that the script still says
		// "this is a fraction" after a round trip through a whole number.
		text += ".0"
	}
	return text
}

// kindOf guesses a parameter's kind from the literal the script assigns.
func kindOf(v float64) string {
	if v == math.Trunc(v) {
		return ParamInt
	}
	return ParamFloat
}

// declaredParams reads the `-- @param` lines.
func declaredParams(src []byte) []Param {
	var out []Param
	for _, line := range headLines(src) {
		m := paramLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		p := Param{Name: m[1], Label: m[1], Kind: ParamInt, Bounded: true}
		for _, a := range attrRE.FindAllStringSubmatch(m[2], -1) {
			switch a[1] {
			case "label":
				p.Label = attrText(a)
			case "min":
				p.Min = attrNumber(a)
			case "max":
				p.Max = attrNumber(a)
			case "step":
				p.Step = attrNumber(a)
			case "default":
				p.Default = attrNumber(a)
			case "kind":
				if attrText(a) == ParamFloat {
					p.Kind = ParamFloat
				}
			}
		}
		if p.Step == 0 {
			p.Step = 1
		}
		if p.Max <= p.Min {
			// A header with no usable range is treated as undeclared bounds
			// rather than drawing a slider that cannot move.
			p.Bounded = false
		}
		out = append(out, p)
	}
	return out
}

// assignments maps every top-level numeric assignment to its value.
func assignments(src []byte) map[string]float64 {
	out := map[string]float64{}
	for _, line := range headLines(src) {
		m := assignRE.FindStringSubmatch(line)
		if m == nil || localAssign.MatchString(line) {
			continue
		}
		if _, seen := out[m[1]]; seen {
			continue // the first assignment is the one an override rewrites
		}
		v, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		out[m[1]] = v
	}
	return out
}

// assignmentOrder lists those names in the order the script declares them,
// which is the order a front end shows them in.
func assignmentOrder(src []byte) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range headLines(src) {
		m := assignRE.FindStringSubmatch(line)
		if m == nil || localAssign.MatchString(line) || seen[m[1]] {
			continue
		}
		if _, err := strconv.ParseFloat(m[2], 64); err != nil {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	return out
}

// headLines is every line before the container assignment. Everything after it
// is the definition table, where an `X = 5` is a field rather than a parameter.
func headLines(src []byte) []string {
	lines := strings.Split(string(src), "\n")
	for i, line := range lines {
		if containerRE.MatchString(line) {
			return lines[:i]
		}
	}
	return lines
}

// firstCommentBlock is the run of comment lines a script opens with, joined.
func firstCommentBlock(src []byte) string {
	var out []string
	for _, line := range headLines(src) {
		if strings.TrimSpace(line) == "" && len(out) == 0 {
			continue
		}
		m := commentLine.FindStringSubmatch(line)
		if m == nil {
			if len(out) > 0 {
				break
			}
			continue
		}
		text := strings.TrimSpace(m[1])
		if strings.HasPrefix(text, "@") {
			continue
		}
		out = append(out, text)
	}
	return strings.TrimSpace(strings.Join(out, " "))
}

// attrText is the string form of a key=value attribute, quoted or bare.
func attrText(m []string) string {
	switch {
	case m[2] != "":
		return m[2]
	case m[3] != "":
		return m[3]
	}
	return m[4]
}

// attrNumber is the numeric form, or zero.
func attrNumber(m []string) float64 {
	v, err := strconv.ParseFloat(attrText(m), 64)
	if err != nil {
		return 0
	}
	return v
}
