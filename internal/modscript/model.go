package modscript

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// IntegerToFloat is how a block's INTEGER_TO_FLOAT setting decides the shape of
// an arithmetic result (R1.3).
type IntegerToFloat uint8

// The three states the reference engine's `if itof=="PRESERVE" / elif itof:` test
// collapses to.
const (
	// ITOFOff keeps whatever kind the old value had. This is also what an
	// absent or falsy setting means.
	ITOFOff IntegerToFloat = iota
	// ITOFForce writes a float even where the old value was an integer.
	ITOFForce
	// ITOFPreserve keeps the old value's kind. It is spelled separately from
	// ITOFOff because the reference engine tests for the literal "PRESERVE" first
	// and treats every other truthy value as ITOFForce.
	ITOFPreserve
)

// String names the setting for `mods check --json`.
func (i IntegerToFloat) String() string {
	switch i {
	case ITOFForce:
		return "force"
	case ITOFPreserve:
		return "preserve"
	default:
		return "off"
	}
}

// ValueChange is one {key, value} pair out of a VALUE_CHANGE_TABLE.
type ValueChange struct {
	Key   string
	Value Value
}

// CurrencyMult multiplies the amounts inside every GcRewardMoney block whose
// currency matches (R1.3).
type CurrencyMult struct {
	Currency string
	Mult     float64
	// MultErr carries Python's message for a MULT that float() would refuse, so
	// the engine can report the same failure the reference builder did instead of
	// silently multiplying by zero.
	MultErr string
}

// WrapperMult multiplies named keys inside every block with a given wrapper
// property (R1.3).
type WrapperMult struct {
	Wrapper string
	Mult    float64
	Keys    []string
	MultErr string
}

/*
Block is one EXML_CHANGE_TABLE entry: a single edit to make to one MXML.

The field set is exactly what the reference engine read. Keys the scripts carry but
the engine never looked at -- LINE_OFFSET, VALUE_MATCH, SECTION_ACTIVE and
friends -- are not decoded into behaviour; they are listed in Unsupported so the
build report can say which mods are relying on something this engine ignores,
which is the difference between a mod that quietly does the wrong thing and one
the user has been told about.
*/
type Block struct {
	PrecedingKeyWords []string
	SpecialKeyWords   []string
	ForEachSKWGroup   [][]string
	// HasSKW distinguishes an absent SPECIAL_KEY_WORDS from an empty one only
	// for reporting; the engine treats both as "no anchor", as Python's
	// truthiness did.
	HasSKW       bool
	SectionUp    int
	SectionUpTo  string
	ValueChanges []ValueChange
	// HasVCT is true when VALUE_CHANGE_TABLE was present and non-empty.
	HasVCT         bool
	MathOperation  string
	IntegerToFloat IntegerToFloat
	ReplaceType    string

	Add    string
	HasAdd bool
	Remove bool
	// HasRemove is key presence, not truthiness. The build's fallback pass
	// retries a failed target with the blocks that carry neither ADD nor
	// REMOVE as *keys*, so a block with REMOVE = false removes nothing yet is
	// still dropped by the retry. Reproducing that needs both facts.
	HasRemove bool

	CurrencyMult *CurrencyMult
	WrapperMult  *WrapperMult

	/*
		Cap is an upper bound applied to an arithmetic result before it is
		formatted (spec 005 R2.1).

		Absent, zero or negative means no cap, which is what every reference
		script means: none of them carries the key, so the golden output is the
		output of an engine that never looks at it. It is honoured by
		WRAPPER_MULT, CURRENCY_MULT and a VALUE_CHANGE_TABLE that has a
		MATH_OPERATION -- the three ops that multiply an existing value and can
		therefore produce a number the game cannot hold in a stack.
	*/
	Cap float64
	// HasCap is key presence, which is what `mods check --json` reports; the
	// engine only ever consults Cap.
	HasCap bool

	// Unsupported names the keys this block carries that the engine ignores.
	Unsupported []string
	// Raw is the block as decoded, for `mods check --json`.
	Raw map[string]any
}

/*
Structural reports whether the block carries an ADD or REMOVE key.

Key presence, not effect: this is the test the reference builder used both to flag
a mod as complex (verdict WORKING*) and to decide which blocks to leave out when
a merged file failed to recompile.
*/
func (b *Block) Structural() bool { return b.HasAdd || b.HasRemove }

// MBINChange is one MBIN_CHANGE_TABLE entry: the files to edit and the edits.
type MBINChange struct {
	Sources []string
	Blocks  []*Block
}

// Modification is one MODIFICATIONS entry.
type Modification struct {
	PakFileSource string
	Changes       []MBINChange
}

// Definition is a decoded mod script (R1.3).
type Definition struct {
	Name          string
	Path          string
	ModFilename   string
	Author        string
	Description   string
	NMSVersion    string
	Modifications []Modification
	// Globals are the script's top-level numeric tuning constants.
	Globals map[string]Value
	// Container is the decoded table, kept for DumpJSON and `check --json`.
	Container map[string]any
}

// Targets lists every distinct MBIN_FILE_SOURCE the definition names, in the
// order it names them. Used by the cache to decide what to extract.
func (d *Definition) Targets() []string {
	seen := map[string]bool{}
	var out []string
	for _, mod := range d.Modifications {
		for _, ch := range mod.Changes {
			for _, src := range ch.Sources {
				if !seen[src] {
					seen[src] = true
					out = append(out, src)
				}
			}
		}
	}
	return out
}

// BlockCount is how many edits the definition makes in total.
func (d *Definition) BlockCount() int {
	n := 0
	for _, mod := range d.Modifications {
		for _, ch := range mod.Changes {
			n += len(ch.Blocks) * max(1, len(ch.Sources))
		}
	}
	return n
}

// UnsupportedKeys lists every ignored key any of the definition's blocks
// carried, sorted and de-duplicated.
func (d *Definition) UnsupportedKeys() []string {
	seen := map[string]bool{}
	for _, mod := range d.Modifications {
		for _, ch := range mod.Changes {
			for _, blk := range ch.Blocks {
				for _, k := range blk.Unsupported {
					seen[k] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// interpreted is the set of block keys the engine acts on. Anything else in a
// block is reported as unsupported.
//
//nolint:gochecknoglobals // a lookup table, read-only after initialisation
var interpreted = map[string]bool{
	"PRECEDING_KEY_WORDS": true, "SPECIAL_KEY_WORDS": true, "FOREACH_SKW_GROUP": true,
	"SECTION_UP": true, "SECTION_UP_TO": true, "VALUE_CHANGE_TABLE": true,
	"MATH_OPERATION": true, "INTEGER_TO_FLOAT": true, "REPLACE_TYPE": true,
	"ADD": true, "REMOVE": true, "CURRENCY_MULT": true, "WRAPPER_MULT": true,
	"CAP": true,
}

// decode turns the container tree into the typed model.
func decode(path string, container map[string]any) (*Definition, error) {
	def := &Definition{
		Name:        strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Path:        path,
		ModFilename: scalarString(container["MOD_FILENAME"]),
		Description: scalarString(container["MOD_DESCRIPTION"]),
		NMSVersion:  scalarString(container["NMS_VERSION"]),
		Container:   container,
	}
	def.Author = joinAuthors(scalarString(container["MOD_AUTHOR"]), scalarString(container["LUA_AUTHOR"]))

	for _, m := range listOf(container["MODIFICATIONS"]) {
		entry, ok := m.(map[string]any)
		if !ok {
			continue
		}
		mod := Modification{PakFileSource: scalarString(entry["PAK_FILE_SOURCE"])}
		for _, c := range listOf(entry["MBIN_CHANGE_TABLE"]) {
			change, ok := c.(map[string]any)
			if !ok {
				continue
			}
			mod.Changes = append(mod.Changes, decodeChange(change))
		}
		def.Modifications = append(def.Modifications, mod)
	}
	if len(def.Modifications) == 0 {
		return nil, &LoadError{Path: path, Stage: StageMalformedDef,
			Message: ContainerName + " has no MODIFICATIONS entries"}
	}
	return def, nil
}

// joinAuthors reports MOD_AUTHOR, adding LUA_AUTHOR when the two differ (R1.3).
func joinAuthors(mod, lua string) string {
	switch {
	case lua == "" || lua == mod:
		return mod
	case mod == "":
		return lua
	default:
		return mod + " / " + lua
	}
}

func decodeChange(change map[string]any) MBINChange {
	out := MBINChange{}
	// MBIN_FILE_SOURCE is a string or a list of strings; the reference loader
	// silently dropped non-string list entries, which this keeps so a script
	// with a stray nil in the list behaves the same.
	switch src := change["MBIN_FILE_SOURCE"].(type) {
	case Value:
		if src.Kind() == KindString && src.String() != "" {
			out.Sources = []string{src.String()}
		}
	case []any:
		for _, item := range src {
			if v, ok := item.(Value); ok && v.Kind() == KindString {
				out.Sources = append(out.Sources, v.String())
			}
		}
	}
	for _, b := range listOf(change["EXML_CHANGE_TABLE"]) {
		blk, ok := b.(map[string]any)
		if !ok {
			continue
		}
		out.Blocks = append(out.Blocks, decodeBlock(blk))
	}
	return out
}

func decodeBlock(raw map[string]any) *Block {
	blk := &Block{Raw: raw}

	blk.PrecedingKeyWords = keywordList(raw["PRECEDING_KEY_WORDS"])
	blk.SpecialKeyWords = keywordList(raw["SPECIAL_KEY_WORDS"])
	blk.HasSKW = len(blk.SpecialKeyWords) > 0
	for _, g := range listOf(raw["FOREACH_SKW_GROUP"]) {
		blk.ForEachSKWGroup = append(blk.ForEachSKWGroup, keywordList(g))
	}
	blk.SectionUp = scalarInt(raw["SECTION_UP"])
	blk.SectionUpTo = scalarString(raw["SECTION_UP_TO"])
	blk.MathOperation = scalarString(raw["MATH_OPERATION"])
	blk.ReplaceType = scalarString(raw["REPLACE_TYPE"])

	if itof, ok := raw["INTEGER_TO_FLOAT"]; ok {
		switch v := itof.(type) {
		case Value:
			switch {
			case v.Kind() == KindString && v.String() == "PRESERVE":
				blk.IntegerToFloat = ITOFPreserve
			case v.Truthy():
				blk.IntegerToFloat = ITOFForce
			}
		default:
			blk.IntegerToFloat = ITOFForce
		}
	}

	for _, pair := range listOf(raw["VALUE_CHANGE_TABLE"]) {
		items, ok := pair.([]any)
		if !ok || len(items) < 2 {
			continue
		}
		blk.ValueChanges = append(blk.ValueChanges, ValueChange{
			Key:   scalarString(items[0]),
			Value: scalarValue(items[1]),
		})
	}
	blk.HasVCT = len(blk.ValueChanges) > 0

	if add, ok := raw["ADD"]; ok {
		blk.HasAdd = true
		blk.Add = scalarString(add)
	}
	if remove, ok := raw["REMOVE"]; ok {
		blk.HasRemove = true
		blk.Remove = scalarValue(remove).Truthy()
	}
	if capv, ok := raw["CAP"]; ok {
		blk.HasCap = true
		if f, ok := scalarValue(capv).Float(); ok {
			blk.Cap = f
		}
	}
	if cm, ok := raw["CURRENCY_MULT"].(map[string]any); ok {
		mult, err := multOf(cm["MULT"])
		blk.CurrencyMult = &CurrencyMult{
			Currency: scalarString(cm["CURRENCY"]), Mult: mult, MultErr: err,
		}
	}
	if wm, ok := raw["WRAPPER_MULT"].(map[string]any); ok {
		mult, err := multOf(wm["MULT"])
		keys := []string{"AmountMin", "AmountMax"}
		if v, ok := wm["KEYS"]; ok {
			keys = keywordList(v)
		}
		blk.WrapperMult = &WrapperMult{
			Wrapper: scalarString(wm["WRAPPER"]), Mult: mult, Keys: keys, MultErr: err,
		}
	}

	for key := range raw {
		if !interpreted[key] {
			blk.Unsupported = append(blk.Unsupported, key)
		}
	}
	sort.Strings(blk.Unsupported)
	return blk
}

// multOf is float(x) with Python's default of 1 for an absent MULT, and
// Python's own error text when the value is not a number.
func multOf(v any) (float64, string) {
	if v == nil {
		return 1, ""
	}
	val := scalarValue(v)
	if val.IsNil() {
		return 1, ""
	}
	f, ok := val.Float()
	if !ok {
		if val.Kind() == KindString {
			return 0, fmt.Sprintf("could not convert string to float: %s", pyQuote(val.String()))
		}
		return 0, fmt.Sprintf("float() argument must be a string or a real number, not %s", pyQuote("table"))
	}
	return f, ""
}

/*
keywordList normalises a keyword setting to a list of strings.

A setting is a string or a list, and the reference engine handled both by wrapping
the string. An empty string produces no keywords, matching `if pkw:`, which is
what makes ItemValueBoost's PRECEDING_KEY_WORDS="" mean "start at the top".

One deliberate divergence: the reference code would iterate a *string*
SPECIAL_KEY_WORDS character by character, because Python strings are iterable.
No script does that, the golden set contains no instance of it, and reproducing
it would mean matching one letter at a time against the MXML. A string is
treated as a single keyword instead.
*/
func keywordList(v any) []string {
	switch t := v.(type) {
	case Value:
		if s := t.String(); s != "" && !t.IsNil() {
			return []string{s}
		}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, scalarString(item))
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	return nil
}

// listOf reads a list-shaped tree node, treating anything else as empty.
func listOf(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return nil
}

// scalarValue reads a scalar node, or Nil for a table.
func scalarValue(v any) Value {
	if s, ok := v.(Value); ok {
		return s
	}
	return Nil
}

// scalarString is Python's str() of a scalar node, and "" for anything else.
func scalarString(v any) string {
	s, ok := v.(Value)
	if !ok || s.IsNil() {
		return ""
	}
	return s.String()
}

// scalarInt reads an integer setting, defaulting to 0 (`blk.get("SECTION_UP",0)`).
func scalarInt(v any) int {
	s, ok := v.(Value)
	if !ok {
		return 0
	}
	f, ok := s.Float()
	if !ok {
		return 0
	}
	return int(f)
}

// pyQuote renders a string the way Python's repr does, which is what its
// exception messages embed.
func pyQuote(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}
