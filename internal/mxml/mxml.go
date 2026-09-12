/*
Package mxml is the line-based MXML edit engine (spec 002 R2).

It is a port of apply_block from the reference Python builder, quirks included.
That is deliberate: the correctness oracle for this phase is a golden set
captured from that builder against the installed game, and an engine that
"improves" a rule is an engine whose output nobody has verified in game. Where a
behaviour looks like a bug -- a keyword group that half-matches still anchors on
its last hit, a section walk that runs to the end of the file when it starts on
a self-closing line -- the comment says so and the code keeps it.

The engine works on lines, not on a parsed document, for the same reason: the
scripts were written against a line-oriented tool, and their SECTION_UP counts
and keyword anchors describe MBINCompiler's exact indentation, not the XML tree.
*/
package mxml

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/ushineko/nmsbonker/internal/modscript"
)

// Int32 bounds. A merged value outside them makes MBINCompiler overflow the
// field rather than reject the file, so the reference engine clamped and so does
// this one.
const (
	maxInt32 = 2147483647
	minInt32 = -2147483648
)

// NTabs counts the leading tabs on a line, which is how the engine measures
// nesting depth (R2.2).
func NTabs(line string) int {
	n := 0
	for i := range len(line) {
		if line[i] != '\t' {
			break
		}
		n++
	}
	return n
}

/*
CloseIndex finds the </Property> closing the opening tag at i (R2.2).

Two behaviours worth knowing before relying on it. It counts every
non-self-closing <Property> as an opener, including the one on line i, so it
needs to be called on an opener. And when it is called on a *self-closing* line
the depth never reaches zero and it returns the last line of the file -- which
is what makes a SECTION_UP walk that lands on a self-closing line scope the
whole remainder of the document. Both callers that can hit that case guard it;
the ones that do not are reproducing the reference behaviour on purpose.
*/
func CloseIndex(lines []string, i int) int {
	depth := 0
	for j := i; j < len(lines); j++ {
		s := strings.TrimSpace(lines[j])
		if strings.HasPrefix(s, "<Property") && !strings.HasSuffix(s, "/>") {
			depth++
		}
		if s == "</Property>" {
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return len(lines) - 1
}

// FindKW returns the first line in [start, end) containing kw, or -1 (R2.2).
func FindKW(lines []string, kw string, start, end int) int {
	if start < 0 {
		start = 0
	}
	end = min(end, len(lines))
	for j := start; j < end; j++ {
		if strings.Contains(lines[j], kw) {
			return j
		}
	}
	return -1
}

// FindProp returns the first <Property> line in [start, end) naming key (R2.2).
func FindProp(lines []string, key string, start, end int) int {
	pat := `name="` + key + `"`
	if start < 0 {
		start = 0
	}
	end = min(end, len(lines))
	for j := start; j < end; j++ {
		if strings.Contains(lines[j], pat) && strings.Contains(lines[j], "<Property") {
			return j
		}
	}
	return -1
}

// valueRE matches the first value="..." attribute on a line.
//
//nolint:gochecknoglobals // a compiled regexp, the standard way to hold one
var valueRE = regexp.MustCompile(`(value=")([^"]*)(")`)

// GetVal reads the first value="..." on a line (R2.2). ok is false when the
// line has none, which the engine treats as "nothing to compute from".
func GetVal(line string) (val string, ok bool) {
	m := valueRE.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[2], true
}

// SetVal replaces the first value="..." on a line (R2.2). A line with no value
// attribute is returned unchanged, which is what Python's re.sub did.
func SetVal(line, newval string) string {
	loc := valueRE.FindStringSubmatchIndex(line)
	if loc == nil {
		return line
	}
	// loc[4]:loc[5] is the captured old value.
	return line[:loc[4]] + newval + line[loc[5]:]
}

// ErrNotFinite reports an arithmetic result Python's round() would have refused
// (an infinity or a NaN), which sends the caller down its str(val) fallback.
var ErrNotFinite = errors.New("arithmetic result is not a finite number")

/*
FormatNum renders an arithmetic result the way the reference fmt_num did (R2.2).

Three rules, all load-bearing:

  - Integer-ness is decided by the *old* value's spelling: a value the MXML
    wrote as "5" stays an integer, one written "5.0" becomes a float, and
    INTEGER_TO_FLOAT overrides that either way. Getting this wrong writes a
    fractional value into an integer field and MBINCompiler rejects the file.
  - Integers go through Python's round(), which is round-half-to-even, not
    round-half-up: 2.5 becomes 2 and 3.5 becomes 4. Several mods multiply by
    values that land exactly on .5, so the golden merge disagrees at once if
    this is math.Round.
  - Floats go through "%f" -- six decimal places -- and then have their
    trailing zeros and any trailing dot stripped. That is a lossy rendering the
    scripts were tuned against; printing the shortest round-trip instead
    changes hundreds of values in the golden set.
*/
func FormatNum(result float64, old string, itof modscript.IntegerToFloat) (string, error) {
	// ITOFPreserve and ITOFOff both keep the old value's kind; only ITOFForce
	// changes the answer. The reference code spelled all three branches because it
	// tested a raw string, and PRESERVE had to be recognised before the generic
	// "any truthy value means float".
	oldIsInt := !strings.Contains(old, ".")
	keepInt := oldIsInt
	if itof == modscript.ITOFForce {
		keepInt = false
	}

	if keepInt {
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return "", ErrNotFinite
		}
		v := math.RoundToEven(result)
		switch {
		case v > maxInt32:
			v = maxInt32
		case v < minInt32:
			v = minInt32
		}
		return strconv.FormatInt(int64(v), 10), nil
	}

	switch {
	case math.IsNaN(result):
		return "nan", nil
	case math.IsInf(result, 1):
		return "inf", nil
	case math.IsInf(result, -1):
		return "-inf", nil
	}
	r := strconv.FormatFloat(result, 'f', 6, 64)
	r = strings.TrimRight(r, "0")
	r = strings.TrimRight(r, ".")
	if r == "" {
		return "0", nil
	}
	return r, nil
}

/*
PyList renders a []string as Python's repr of a list (R2.1).

The report lines carry these verbatim -- "['AmountMin', 'AmountMax'] x* across
65 'Units' sections" -- so the spacing after the comma and the single quotes are
part of the format, not decoration.
*/
func PyList(items []string) string {
	if items == nil {
		return "None"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, s := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(PyQuote(s))
	}
	b.WriteByte(']')
	return b.String()
}

/*
PyQuote renders a string the way Python's repr does.

Single quotes, unless the string contains one and no double quote, in which case
Python switches to double quotes rather than escaping. Keywords in practice are
XML identifiers, but the rule is cheap to honour and a mod that names a section
with an apostrophe should not shift every following report line.
*/
func PyQuote(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + escapePy(s) + `"`
	}
	return "'" + strings.ReplaceAll(escapePy(s), "'", `\'`) + "'"
}

func escapePy(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return r.Replace(s)
}

// Base is Python's os.path.basename on the source spelling (R2.1).
//
// POSIX basename splits on "/" only, so a source written the Windows way keeps
// its backslashes and appears in the report whole:
// "METADATA\REALITY\TABLES\REWARDTABLE.MBIN". The report lines in the golden
// set are full of them, so splitting on backslash here would break every one.
func Base(src string) string {
	if i := strings.LastIndexByte(src, '/'); i >= 0 {
		return src[i+1:]
	}
	return src
}
