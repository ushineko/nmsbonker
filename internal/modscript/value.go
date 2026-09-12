package modscript

import (
	"math"
	"strconv"
	"strings"
)

/*
Kind names what a decoded Lua scalar turned into once it had been through the
reference dumper's JSON and Python's json.loads (spec 002 R1.3).

The distinction between KindInt and KindFloat is not cosmetic. It decides what
Python's str() printed into the build report and, more importantly, what
apply_block wrote into the MXML when a value change had no MATH_OPERATION:
str(5) is "5" and str(5.0) is "5.0", and an MXML that says value="5.0" where the
game wants an integer field is a file MBINCompiler rejects.
*/
type Kind uint8

// The scalar kinds a change table can hold.
const (
	KindNil Kind = iota
	KindBool
	KindString
	KindInt
	KindFloat
)

/*
Value is one scalar out of a mod script: a string, a number or a boolean.

Numbers carry the integer/float split the reference pipeline established. The
dumper wrote an integral number below 1e15 with "%d" and everything else with
tostring(); Python's json.loads then produced an int for the first and a float
for the second. Reproducing that split here, rather than storing float64 and
guessing later, is what makes String() able to print exactly what the reference
builder printed.
*/
type Value struct {
	kind Kind
	b    bool
	s    string
	i    int64
	f    float64
}

// Nil is the zero Value: an absent entry.
var Nil = Value{}

// StringValue makes a string scalar.
func StringValue(s string) Value { return Value{kind: KindString, s: s} }

// IntValue makes an integer scalar.
func IntValue(i int64) Value { return Value{kind: KindInt, i: i} }

// FloatValue makes a floating-point scalar.
func FloatValue(f float64) Value { return Value{kind: KindFloat, f: f} }

// BoolValue makes a boolean scalar.
func BoolValue(b bool) Value { return Value{kind: KindBool, b: b} }

// Kind reports which of the scalar kinds this is.
func (v Value) Kind() Kind { return v.kind }

// IsNil reports the absent value.
func (v Value) IsNil() bool { return v.kind == KindNil }

// IsNumber reports an int or a float.
func (v Value) IsNumber() bool { return v.kind == KindInt || v.kind == KindFloat }

/*
Truthy is Python's truth test on the decoded value, which is what every
`if blk.get(...)` in the reference engine actually asked.

It matters in two places that look like they could not care: ADD is applied only
when the string is non-empty, and INTEGER_TO_FLOAT is "force float" for any
truthy value that is not the literal "PRESERVE". A block carrying
REMOVE = false is a block that removes nothing -- but, separately, still counts
as structural when the build decides what to retry without (see Block.Structural).
*/
func (v Value) Truthy() bool {
	switch v.kind {
	case KindNil:
		return false
	case KindBool:
		return v.b
	case KindString:
		return v.s != ""
	case KindInt:
		return v.i != 0
	case KindFloat:
		return v.f != 0
	default:
		return false
	}
}

/*
String reproduces Python's str() of this value (R1.3).

The reference engine wrote str(val) into the MXML whenever a change had no
MATH_OPERATION, and printed it into the report either way, so every quirk of
Python's rendering is load-bearing: True/False capitalised, integers bare,
floats through repr() -- shortest round-trip, ".0" forced onto whole numbers,
and scientific notation with a two-digit exponent below 1e-4.
*/
func (v Value) String() string {
	switch v.kind {
	case KindNil:
		return "None"
	case KindBool:
		if v.b {
			return "True"
		}
		return "False"
	case KindString:
		return v.s
	case KindInt:
		return strconv.FormatInt(v.i, 10)
	case KindFloat:
		return PyRepr(v.f)
	default:
		return ""
	}
}

/*
Float parses this value as a number the way Python's float() would (R1.3).

A value change may spell its number either way -- {"BaseValue", 3} and
{"HoverMinSpeed", "0.0001"} are both idiomatic in the scripts -- and
MATH_OPERATION has to work on both. ok is false where Python's float() would
have raised, which is the branch the engine falls back on by writing str(val)
verbatim.
*/
func (v Value) Float() (float64, bool) {
	switch v.kind {
	case KindInt:
		return float64(v.i), true
	case KindFloat:
		return v.f, true
	case KindString:
		return ParseFloat(v.s)
	default:
		return 0, false
	}
}

/*
ParseFloat is Python's float(str): surrounding whitespace is allowed, hex float
literals and digit separators are not.

Go's strconv.ParseFloat accepts "0x1p-2" and "1_000_000", which Python rejects
outright. Letting them through would turn a value the reference builder wrote
verbatim into an arithmetic result, so they are refused here instead.
*/
func ParseFloat(s string) (float64, bool) {
	t := strings.TrimSpace(s)
	if t == "" || strings.ContainsAny(t, "xX_") {
		return 0, false
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

/*
PyRepr renders a float the way Python's repr() does.

CPython prints the shortest decimal that round-trips, switching to scientific
notation when the decimal point would fall at or before position -4 or after
position 16 -- so 0.0001 stays fixed, 1e-05 does not, 1e15 stays fixed and 1e16
does not. Go's %g uses different thresholds and its shortest form drops the
".0" that Python always keeps on a whole number, so neither of the obvious
formatters is a substitute.
*/
func PyRepr(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	neg := math.Signbit(f)
	// 'e' with precision -1 is the shortest round-tripping form, which is the
	// same set of digits CPython's dtoa produces; only the layout differs.
	sci := strconv.FormatFloat(math.Abs(f), 'e', -1, 64)
	mantissa, expPart, _ := strings.Cut(sci, "e")
	exp, err := strconv.Atoi(expPart)
	if err != nil {
		return sci
	}
	digits := strings.Replace(mantissa, ".", "", 1)
	// decpt is where the decimal point sits relative to the digit string, which
	// is the quantity CPython's format_float_short thresholds are written in.
	decpt := exp + 1

	var out string
	switch {
	case decpt <= -4 || decpt > 16:
		out = digits[:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		out += "e" + expSuffix(exp)
	case decpt <= 0:
		out = "0." + strings.Repeat("0", -decpt) + digits
	case decpt >= len(digits):
		out = digits + strings.Repeat("0", decpt-len(digits)) + ".0"
	default:
		out = digits[:decpt] + "." + digits[decpt:]
	}
	if neg {
		out = "-" + out
	}
	return out
}

// expSuffix renders an exponent the way Python does: always signed, always at
// least two digits ("1e-05", not "1e-5").
func expSuffix(exp int) string {
	sign := "+"
	if exp < 0 {
		sign = "-"
		exp = -exp
	}
	s := strconv.Itoa(exp)
	if len(s) < 2 {
		s = "0" + s
	}
	return sign + s
}
