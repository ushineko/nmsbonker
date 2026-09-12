package modscript

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

/*
DumpJSON serialises a definition in the reference dumper's JSON shape (R1.4).

This exists for one reason: the golden Lua stage compares what this package
decoded against dump_mod.lua's output for the same script, and a comparison
needs both sides in the same notation. It is also what `mods check --json`
prints, so a user debugging a script sees the same view the parity test does.

The comparison is semantic, not textual. Lua's pairs() has no defined order, so
the reference dumper's object key order is whatever that run happened to produce;
keys are sorted here to make this side reproducible, not to match it.
*/
func DumpJSON(def *Definition) []byte {
	var b strings.Builder
	encodeNode(&b, def.Container)
	return []byte(b.String())
}

// DumpJSONIndent is DumpJSON re-formatted for a human reading `mods check --json`.
func DumpJSONIndent(def *Definition) []byte {
	var out any
	if err := json.Unmarshal(DumpJSON(def), &out); err != nil {
		return DumpJSON(def)
	}
	pretty, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		return DumpJSON(def)
	}
	return pretty
}

func encodeNode(b *strings.Builder, v any) {
	switch t := v.(type) {
	case Value:
		encodeScalar(b, t)
	case []any:
		b.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			encodeNode(b, item)
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			encodeString(b, k)
			b.WriteByte(':')
			encodeNode(b, t[k])
		}
		b.WriteByte('}')
	default:
		b.WriteString("null")
	}
}

func encodeScalar(b *strings.Builder, v Value) {
	switch v.Kind() {
	case KindString:
		encodeString(b, v.String())
	case KindInt:
		b.WriteString(v.String())
	case KindFloat:
		f, _ := v.Float()
		// tostring() in Lua is "%.14g"; the dumper wrote it unquoted, so the
		// JSON number carries whatever that produced.
		b.WriteString(strconv.FormatFloat(f, 'g', 14, 64))
	case KindBool:
		if v.String() == "True" {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	default:
		b.WriteString("null")
	}
}

/*
encodeString escapes exactly what the reference dumper escaped.

Its table covers the quote, the backslash, newline, carriage return and tab, and
sends every other control character through \u00xx. Anything above U+007F went
out as raw bytes, which is valid UTF-8 JSON, so it is written through unchanged
here too.
*/
func encodeString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := range len(s) {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				fmt.Fprintf(b, `\u%04x`, c)
				continue
			}
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
}
