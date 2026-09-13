package mxml

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ushineko/nmsbonker/internal/modscript"
)

// sectionUpToLookback is how far back the SECTION_UP_TO scan looks for its
// marker before giving up on a match. Twelve lines is the reference limit.
const sectionUpToLookback = 12

/*
Apply runs one EXML_CHANGE_TABLE block over an MXML's lines (R2.1).

It returns the edited lines -- ADD and REMOVE change the length, so the slice
identity is not stable -- and the report events the edit produced. The order of
the branches below is the reference order and it is significant: CURRENCY_MULT and
WRAPPER_MULT return before anything else is consulted, and the REPLACE_TYPE=ALL
multi-section path is only taken when a very specific combination of keys is
present.
*/
func Apply(lines []string, blk *modscript.Block, actx ApplyContext) ([]string, []Event) {
	n := len(lines)
	start, end := 0, n

	// PRECEDING_KEY_WORDS walks a cursor forward, each keyword searched from
	// the line after the previous one's hit. The quoted spelling is tried first
	// so that a keyword naming an XML value ("PlanetEngine") does not match the
	// substring of a longer identifier.
	for _, kw := range blk.PrecedingKeyWords {
		if kw == "" {
			continue
		}
		j := FindKW(lines, `"`+kw+`"`, start, end)
		if j < 0 {
			j = FindKW(lines, kw, start, end)
		}
		if j >= 0 {
			start = j + 1
		}
	}

	if blk.CurrencyMult != nil {
		return applyCurrencyMult(lines, blk.CurrencyMult, blk.Cap, actx)
	}
	if blk.WrapperMult != nil {
		return applyWrapperMult(lines, blk.WrapperMult, blk.Cap, actx)
	}

	groups := blk.ForEachSKWGroup
	if len(groups) == 0 {
		if blk.HasSKW {
			groups = [][]string{blk.SpecialKeyWords}
		} else {
			// One pass over the whole file from `start`, with no anchor.
			groups = [][]string{nil}
		}
	}

	// The guard is deliberately this specific: the reference engine took this path
	// only for REPLACE_TYPE=ALL with a bare SPECIAL_KEY_WORDS, a value table,
	// and neither a truthy ADD nor a truthy REMOVE. Loosening any clause sends
	// blocks down a branch that edits different lines.
	if blk.ReplaceType == "ALL" && blk.HasSKW && len(blk.ForEachSKWGroup) == 0 &&
		blk.HasVCT && blk.Add == "" && !blk.Remove {
		return applyAllSections(lines, blk, actx, start, end)
	}
	return applyGroups(lines, blk, actx, groups, start, end)
}

/*
applyCurrencyMult multiplies the amounts in every matching GcRewardMoney block.

This op exists because doing the same edit with SPECIAL_KEY_WORDS double-applied
across overlapping sections: the reward table nests money rewards inside larger
entries, and a keyword anchor that matched the outer entry multiplied the inner
one twice. Walking the file block by block and jumping past each closing tag
makes the count exact.
*/
func applyCurrencyMult(lines []string, cm *modscript.CurrencyMult, limit float64,
	actx ApplyContext,
) ([]string, []Event) {
	if cm.MultErr != "" {
		return lines, []Event{actx.warn("exception on %s: %s", actx.File(), cm.MultErr)}
	}
	from, to, ok := entryScope(lines, cm.Entry)
	if !ok {
		return lines, []Event{actx.warn("entry %s not found in %s", cm.Entry, actx.File())}
	}
	const opener = `<Property name="GcRewardMoney">`
	needle := `value="` + cm.Currency + `"`
	count, capped := 0, 0

	for i := from; i < to; {
		if strings.TrimSpace(lines[i]) != opener {
			i++
			continue
		}
		last := CloseIndex(lines, i)
		matched := false
		for j := i; j <= last && j < len(lines); j++ {
			if strings.Contains(lines[j], needle) {
				matched = true
				break
			}
		}
		if matched {
			for j := i; j <= last && j < len(lines); j++ {
				var n int
				lines[j], n = scaleLine(lines[j], []string{"AmountMin", "AmountMax"}, cm.Mult, limit)
				capped += n
			}
			count++
		}
		i = last + 1
	}
	e := actx.ok("CURRENCY_MULT %s x%s%s%s across %d GcRewardMoney blocks in %s%s",
		cm.Currency, modscript.PyRepr(cm.Mult), capText(limit), entryText(cm.Entry), count,
		actx.File(), cappedText(capped))
	e.Capped = capped
	return lines, []Event{e}
}

// applyWrapperMult multiplies named keys once inside every block opened by the
// wrapper property. Same motivation as CURRENCY_MULT: no keyword overlap, so no
// double application.
func applyWrapperMult(lines []string, wm *modscript.WrapperMult, limit float64,
	actx ApplyContext,
) ([]string, []Event) {
	if wm.MultErr != "" {
		return lines, []Event{actx.warn("exception on %s: %s", actx.File(), wm.MultErr)}
	}
	from, to, ok := entryScope(lines, wm.Entry)
	if !ok {
		return lines, []Event{actx.warn("entry %s not found in %s", wm.Entry, actx.File())}
	}
	opener := `<Property name="` + wm.Wrapper + `">`
	count, capped := 0, 0

	for i := from; i < to; {
		if strings.TrimSpace(lines[i]) != opener {
			i++
			continue
		}
		last := CloseIndex(lines, i)
		// From i+1, not i: the wrapper's own line carries no amount, and the
		// reference op skipped it.
		for j := i + 1; j <= last && j < len(lines); j++ {
			var n int
			lines[j], n = scaleLine(lines[j], wm.Keys, wm.Mult, limit)
			capped += n
		}
		count++
		i = last + 1
	}
	e := actx.ok("WRAPPER_MULT %s x%s%s%s across %d blocks in %s%s",
		wm.Wrapper, modscript.PyRepr(wm.Mult), capText(limit), entryText(wm.Entry), count,
		actx.File(), cappedText(capped))
	e.Capped = capped
	return lines, []Event{e}
}

/*
entryScope bounds an op to one reward-table entry (spec 006 R1.1).

An empty entry is the whole file, which is what every script written before the
key existed means. Otherwise the entry is the GcGenericRewardTableEntry whose
opening line MBINCompiler stamped with _id="<entry>"; failing that, the line
`name="Id" value="<entry>"` and its parent, for an MXML written without the
attribute. Not found is reported by the caller, not guessed at here: an op that
fell back to the whole file would silently multiply every entry in the table.
*/
func entryScope(lines []string, entry string) (from, to int, ok bool) {
	if entry == "" {
		return 0, len(lines), true
	}
	if i := FindKW(lines, `_id="`+entry+`"`, 0, len(lines)); i >= 0 {
		return i, CloseIndex(lines, i) + 1, true
	}
	i := FindKW(lines, `name="Id" value="`+entry+`"`, 0, len(lines))
	if i < 0 {
		return 0, 0, false
	}
	start := walkUp(lines, i, 1)
	return start, CloseIndex(lines, start) + 1, true
}

// entryText is the " in entry X" clause of an event, empty when the op ran
// over the whole file so the pre-006 event text is unchanged.
func entryText(entry string) string {
	if entry == "" {
		return ""
	}
	return " in entry " + entry
}

// scaleLine multiplies the line's value when it names one of the keys, and
// reports how many of them the cap held back (R2.1). A value that will not
// parse as a number is left alone, which is the reference `except: nv = ov`.
func scaleLine(line string, keys []string, mult, limit float64) (out string, capped int) {
	for _, key := range keys {
		if !strings.Contains(line, `name="`+key+`"`) {
			continue
		}
		old, ok := GetVal(line)
		if !ok {
			continue
		}
		f, ok := modscript.ParseFloat(old)
		if !ok {
			continue
		}
		r, bit := clamp(f*mult, limit)
		nv, err := FormatNum(r, old, modscript.ITOFOff)
		if err != nil {
			continue
		}
		if bit {
			capped++
		}
		line = SetVal(line, nv)
	}
	return line, capped
}

/*
clamp applies a block's CAP to an arithmetic result (R2.1).

A cap of zero -- which is what an absent CAP decodes to, and what every
reference script means -- returns the result untouched, so an engine run over a
script that does not use the key produces exactly the bytes it produced before
the key existed. A negative cap is treated the same way: "no ceiling" is a more
useful reading of a nonsense value than "clamp everything to a negative number".
*/
func clamp(result, limit float64) (value float64, capped bool) {
	if limit <= 0 || result <= limit {
		return result, false
	}
	return limit, true
}

/*
capText names the ceiling in a report line, and is empty for a block that has
none -- which is what keeps the golden report lines byte-identical (R2.2).

The number is rendered plainly rather than through PyRepr, which prints a whole
number as "50000.0" because that is Python's repr of a float and the multipliers
beside it were captured that way. A cap is a stack size, nobody wrote it as a
float, and reproducing a Python quirk for a field Python never saw would be
imitation rather than parity.
*/
func capText(limit float64) string {
	if limit <= 0 {
		return ""
	}
	return " cap " + strconv.FormatFloat(limit, 'f', -1, 64)
}

// cappedText says how often the cap actually bit. Silent when it did not, so a
// cap set generously enough never to matter adds nothing to read.
func cappedText(capped int) string {
	if capped == 0 {
		return ""
	}
	return fmt.Sprintf(" (capped %d)", capped)
}

/*
applyAllSections is the REPLACE_TYPE=ALL + SPECIAL_KEY_WORDS path (R2.1).

It edits every section the keywords match, not just the first, which is how a
mod multiplies the amounts in all 65 Units reward entries with one block. The
scope of each match is a SECTION_UP_TO marker if one is given, else a SECTION_UP
walk by tab depth, else the matched line's own section.

Note the cursor arithmetic at the end: `pos = max(sectionEnd, anchor+1)`. Using
sectionEnd alone would loop forever on a self-closing anchor, and anchor+1 alone
would re-enter a section already edited.
*/
func applyAllSections(lines []string, blk *modscript.Block, actx ApplyContext, start, end int) ([]string, []Event) {
	matched, capped := 0, 0
	pos := start

	for {
		anchor := -1
		cursor := pos
		for _, kw := range blk.SpecialKeyWords {
			j := FindKW(lines, `"`+kw+`"`, cursor, end)
			if j < 0 {
				j = FindKW(lines, kw, cursor, end)
			}
			if j < 0 {
				anchor = -1
				break
			}
			cursor = j + 1
			anchor = j
		}
		if anchor < 0 {
			break
		}

		found := anchor
		scopeStart, scopeEnd := anchor, anchor+1
		switch {
		case blk.SectionUpTo != "":
			k := anchor
			limit := max(0, anchor-sectionUpToLookback)
			for k > limit && !strings.Contains(lines[k], blk.SectionUpTo) {
				k--
			}
			if !strings.Contains(lines[k], blk.SectionUpTo) {
				// The marker is not within reach, so this match is not the
				// structure the script meant; skip it rather than editing a
				// section chosen by accident.
				pos = found + 1
				continue
			}
			scopeStart, scopeEnd = k, CloseIndex(lines, k)+1
		case blk.SectionUp != 0:
			scopeStart = walkUp(lines, anchor, blk.SectionUp)
			scopeEnd = CloseIndex(lines, scopeStart) + 1
		default:
			if !strings.HasSuffix(strings.TrimSpace(lines[anchor]), "/>") {
				scopeEnd = CloseIndex(lines, anchor) + 1
			}
		}

		for _, vc := range blk.ValueChanges {
			j := FindProp(lines, vc.Key, scopeStart, scopeEnd)
			if j < 0 {
				continue
			}
			nv, bit := newValue(blk, lines[j], vc)
			if bit {
				capped++
			}
			lines[j] = SetVal(lines[j], nv)
		}
		matched++
		pos = max(scopeEnd, found+1)
	}

	keys := make([]string, 0, len(blk.ValueChanges))
	for _, vc := range blk.ValueChanges {
		keys = append(keys, vc.Key)
	}
	op := blk.MathOperation
	if op == "" {
		op = "="
	}
	last := blk.SpecialKeyWords[len(blk.SpecialKeyWords)-1]
	if matched > 0 {
		e := actx.ok("%s x%s%s across %d '%s' sections in %s%s",
			PyList(keys), op, capText(blk.Cap), matched, last, actx.File(), cappedText(capped))
		e.Capped = capped
		return lines, []Event{e}
	}
	return lines, []Event{actx.warn("no '%s' sections found in %s", last, actx.File())}
}

/*
applyGroups is the general path: one anchor per SKW group, then one operation.

Three shapes share it. FOREACH_SKW_GROUP gives several groups; a plain
SPECIAL_KEY_WORDS gives one; neither gives a single nil group meaning "the whole
file from start". REMOVE returns after the first group it acts on, ADD continues
to the next, and a VALUE_CHANGE_TABLE runs to the end.
*/
func applyGroups(lines []string, blk *modscript.Block, actx ApplyContext,
	groups [][]string, start, end int) ([]string, []Event) {
	var events []Event

	for _, kws := range groups {
		scopeStart, scopeEnd := start, end
		if len(kws) > 0 {
			anchor := -1
			cursor := start
			for _, kw := range kws {
				j := FindKW(lines, `"`+kw+`"`, cursor, end)
				if j < 0 {
					j = FindKW(lines, kw, cursor, end)
				}
				if j < 0 {
					// The reference loop breaks here without clearing the anchor,
					// so a group whose first keywords matched and whose last
					// did not still anchors -- on the last keyword that hit.
					// Several mods in the golden set depend on that: clearing
					// the anchor turns their applied edits into "SKW not found"
					// warnings.
					break
				}
				// The next keyword's search starts *at* this hit, not after it,
				// so two keywords can match the same line.
				cursor = j
				anchor = j
			}
			if anchor < 0 {
				events = append(events, actx.warn("SKW %s not found in %s", PyList(kws), actx.File()))
				continue
			}
			if blk.SectionUp != 0 {
				scopeStart = walkUp(lines, anchor, blk.SectionUp)
				scopeEnd = CloseIndex(lines, scopeStart) + 1
			} else {
				scopeStart = anchor
				scopeEnd = CloseIndex(lines, anchor) + 1
				if strings.HasSuffix(strings.TrimSpace(lines[anchor]), "/>") {
					scopeEnd = anchor + 1
				}
			}
		}

		switch {
		case blk.Remove:
			lines = deleteRange(lines, scopeStart, scopeEnd)
			events = append(events, actx.ok("REMOVE section %s in %s", PyList(kws), actx.File()))
			// A REMOVE stops the whole block, not just this group.
			return lines, events
		case blk.Add != "":
			ins := strings.Split(blk.Add, "\n")
			lines = insertAt(lines, scopeEnd, ins)
			events = append(events, actx.ok("ADD block in %s (%d lines)", actx.File(), len(ins)))
		case blk.HasVCT:
			events = append(events, applyValueChanges(lines, blk, actx, scopeStart, scopeEnd)...)
		}
	}
	return lines, events
}

// applyValueChanges edits the keys of one VALUE_CHANGE_TABLE inside a scope.
func applyValueChanges(lines []string, blk *modscript.Block, actx ApplyContext, from, to int) []Event {
	var events []Event
	for _, vc := range blk.ValueChanges {
		var idxs []int
		for j := FindProp(lines, vc.Key, from, to); j >= 0; j = FindProp(lines, vc.Key, j+1, to) {
			idxs = append(idxs, j)
			if blk.ReplaceType != "ALL" {
				break
			}
		}
		if len(idxs) == 0 {
			events = append(events, Event{
				Kind: WARN, Mod: actx.Mod, File: actx.File(), NotFound: vc.Key,
				Detail: "key '" + vc.Key + "' not found in " + actx.File() + " scope",
			})
			continue
		}
		nv, capped := "", 0
		for _, j := range idxs {
			var bit bool
			nv, bit = newValue(blk, lines[j], vc)
			if bit {
				capped++
			}
			lines[j] = SetVal(lines[j], nv)
		}
		// The reported value is the last one written, which is what the reference
		// f-string picked up from the loop variable.
		e := actx.ok("%s -> %s (%dx) in %s%s", vc.Key, nv, len(idxs), actx.File(), cappedText(capped))
		e.Capped = capped
		events = append(events, e)
	}
	return events
}

/*
newValue computes what to write into a line for one value change.

With no MATH_OPERATION the script's value is written verbatim, as Python's
str() rendered it, and the block's CAP is not consulted. With one, both sides are parsed as floats and the result is
formatted against the old value's spelling; anything that Python's float() or
round() would have refused falls back to writing the script's value verbatim,
which is the reference `except Exception: nv = str(val)`.
*/
func newValue(blk *modscript.Block, line string, vc modscript.ValueChange) (value string, capped bool) {
	if blk.MathOperation == "" {
		// No arithmetic, so no ceiling: CAP bounds a computed result, and a
		// value the script wrote out in full is the value the author asked for.
		return vc.Value.String(), false
	}
	old, hasVal := GetVal(line)
	if !hasVal {
		return vc.Value.String(), false
	}
	o, ok := modscript.ParseFloat(old)
	if !ok {
		return vc.Value.String(), false
	}
	v, ok := vc.Value.Float()
	if !ok {
		return vc.Value.String(), false
	}
	var r float64
	switch blk.MathOperation {
	case "*":
		r = o * v
	case "+":
		r = o + v
	case "-":
		r = o - v
	case "/":
		// Division by zero leaves the value alone rather than producing an
		// infinity; the reference dict spelled it `o/v if v else o`.
		r = o
		if v != 0 {
			r = o / v
		}
	default:
		return vc.Value.String(), false
	}
	r, capped = clamp(r, blk.Cap)
	nv, err := FormatNum(r, old, blk.IntegerToFloat)
	if err != nil {
		return vc.Value.String(), false
	}
	return nv, capped
}

// walkUp climbs `levels` indentation levels from a line, by tab depth (R2.3).
//
// It walks past self-closing lines and text lines alike, because the measure is
// the tab count and nothing else. That is how a SECTION_UP of 1 from a
// <Property name="Id" value="X" /> reaches the entry that contains it.
func walkUp(lines []string, from, levels int) int {
	target := NTabs(lines[from]) - levels
	k := from
	for k > 0 && NTabs(lines[k]) > target {
		k--
	}
	return k
}

// deleteRange is Python's `del lines[s:e]`, with the same tolerance for
// out-of-range bounds.
func deleteRange(lines []string, s, e int) []string {
	s = max(s, 0)
	e = min(e, len(lines))
	if s >= len(lines) || e <= s {
		return lines
	}
	return append(lines[:s], lines[e:]...)
}

// insertAt splices lines in at `at`, appending when `at` is past the end, which
// is what a sequence of Python list.insert calls did.
func insertAt(lines []string, at int, ins []string) []string {
	at = min(max(at, 0), len(lines))
	out := make([]string, 0, len(lines)+len(ins))
	out = append(out, lines[:at]...)
	out = append(out, ins...)
	out = append(out, lines[at:]...)
	return out
}
