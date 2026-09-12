/*
Package audit looks at the reward amounts a merged table actually ends up with
(spec 005 R1).

The build report counts edits. Every edit can be individually correct and the
result still be wrong, because reward amounts *compound*: two mods that each
multiply the same AmountMax by a reasonable number produce an unreasonable one,
and the build reports both as OK. That happened -- salvaged data at 2,500,000 a
stack from a x250 library script and a x10 built-in in the same build order --
and nothing in the report could see it, because nothing in the report looked at
a value.

So this package parses reward blocks out of the pristine and the merged MXML,
compares the amounts, and names the mods that moved them. It reports; it never
edits and never disables anything. The caps that stop the compounding are the
engine's (R2); this is the half that tells you a cap is needed.

Nothing here knows about the build. It takes lines and gives back findings, so
the same code serves the build, the standalone `nmsbonker audit` re-run over the
kept merge, and the tests.
*/
package audit

import (
	"fmt"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/ushineko/nmsbonker/internal/mxml"
)

/*
The reward wrapper properties this package understands (R1.1).

Each is the inner `<Property name="Gc…">` that opens a reward payload, not the
`<Property name="Reward" value="Gc…">` that introduces it -- the outer one
carries a value attribute and the matcher below requires there to be none, which
is what keeps one reward from being counted twice.
*/
const (
	KindSpecificProduct   = "GcRewardSpecificProduct"
	KindSpecificSubstance = "GcRewardSpecificSubstance"
	KindMoney             = "GcRewardMoney"
	KindMultiProducts     = "GcRewardMultiSpecificProducts"
	KindMultiItems        = "GcRewardMultiSpecificItems"
)

// The categories a block's amount is judged against (R1.2). They decide which
// threshold applies, and they are what the game's own limits are expressed in.
const (
	CategoryProduct   = "product"
	CategorySubstance = "substance"
	CategoryUnits     = "units"
	CategoryNanites   = "nanites"
	CategorySpecials  = "specials"
)

// MaxInt32 is the value a clamped integer lands on. A merged amount that equals
// it is always flagged: the engine clamps rather than overflowing, so the value
// is a symptom of arithmetic that ran off the end of the field and never a
// number a script asked for.
const MaxInt32 = 2147483647

// tables are the internal paths this package audits, by basename without the
// extension (R1.1).
//
//nolint:gochecknoglobals // a fixed lookup, read-only after initialisation
var tables = map[string]bool{
	"REWARDTABLE":           true,
	"EXPEDITIONREWARDTABLE": true,
}

/*
TableName reports the audited table an internal path names, and whether it is
one at all.

Matching on the basename rather than the whole path because the same table is
spelled a dozen ways by the scripts and normalised differently by every stage;
"REWARDTABLE" is the part that does not move.
*/
func TableName(internal string) (name string, ok bool) {
	base := path.Base(strings.ReplaceAll(strings.ToUpper(internal), `\`, "/"))
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	return base, tables[base]
}

// Block is one reward payload's amounts, as parsed out of an MXML (R1.1).
type Block struct {
	// Kind is the wrapper property that opened it.
	Kind string `json:"kind"`
	// EntryID is the enclosing reward entry's Id, which is what a user can look
	// up. Empty when the block sits outside any entry.
	EntryID string `json:"entryId"`
	// Item is the product or substance ID, or the currency for a money reward.
	Item string `json:"item"`
	// Category decides which threshold the amount is judged against.
	Category string  `json:"category"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	// Line is the wrapper's 1-based line number, for anyone reading the kept
	// merge by hand.
	Line int `json:"line"`
}

// key is what pairs a pristine block with a merged one when the counts differ.
func (b Block) key() string { return b.EntryID + "\x00" + b.Kind + "\x00" + b.Item }

// Thresholds are the limits a merged amount is judged against (R1.2).
type Thresholds struct {
	MaxProduct   float64 `json:"maxProduct"`
	MaxSubstance float64 `json:"maxSubstance"`
	MaxUnits     float64 `json:"maxUnits"`
	MaxNanites   float64 `json:"maxNanites"`
	MaxSpecials  float64 `json:"maxSpecials"`
	MaxRatio     float64 `json:"maxRatio"`
}

/*
Defaults are the thresholds a machine with no configuration uses (R1.2).

They are chosen to sit just above what this project's own tweaks can reach from
stock values and well below anything that breaks a stack: the product and
substance numbers are the stack limits BigStacks raises the game to, so a single
reward that fills an entire raised stack is the point at which someone should be
told. Every one of them is a setting.
*/
func Defaults() Thresholds {
	return Thresholds{
		MaxProduct:   99999,
		MaxSubstance: 999999,
		MaxUnits:     100000000,
		MaxNanites:   1000000,
		MaxSpecials:  100000,
		MaxRatio:     100,
	}
}

// limit is the threshold for one category, and false when the category has none.
func (t Thresholds) limit(category string) (float64, bool) {
	switch category {
	case CategoryProduct:
		return t.MaxProduct, t.MaxProduct > 0
	case CategorySubstance:
		return t.MaxSubstance, t.MaxSubstance > 0
	case CategoryUnits:
		return t.MaxUnits, t.MaxUnits > 0
	case CategoryNanites:
		return t.MaxNanites, t.MaxNanites > 0
	case CategorySpecials:
		return t.MaxSpecials, t.MaxSpecials > 0
	default:
		return 0, false
	}
}

// Contribution is one mod's effect on one flagged block's amounts (R1.3).
type Contribution struct {
	Mod string `json:"mod"`
	// Factor is what the mod multiplied the running AmountMax by, or 0 when the
	// value before its pass was 0 and there is no factor to state.
	Factor float64 `json:"factor"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

// String renders a contribution the way the report quotes it:
// "BetterRewards x250 -> 250000".
func (c Contribution) String() string {
	if c.Factor <= 0 {
		return fmt.Sprintf("%s -> %s", c.Mod, Amount(c.Max))
	}
	return fmt.Sprintf("%s x%s -> %s", c.Mod, Amount(c.Factor), Amount(c.Max))
}

// Flag is one reward block whose merged amount broke a threshold (R1.4).
type Flag struct {
	Table       string  `json:"table"`
	EntryID     string  `json:"entryId"`
	Kind        string  `json:"kind"`
	Item        string  `json:"item"`
	Category    string  `json:"category"`
	PristineMin float64 `json:"pristineMin"`
	PristineMax float64 `json:"pristineMax"`
	MergedMin   float64 `json:"mergedMin"`
	MergedMax   float64 `json:"mergedMax"`
	// Ratio is merged over pristine AmountMax, or 0 when the stock value was 0
	// and there is no ratio to state.
	Ratio float64 `json:"ratio"`
	// Reasons are why it is flagged, in the order they were tested.
	Reasons []string `json:"reasons"`
	// Contributors are the mods that moved it, in build order (R1.3). Empty
	// until Attribute has run.
	Contributors []Contribution `json:"contributors,omitempty"`
	// at is the flagged block's index in the merged block list, and pristineAt
	// and pristineLine are where the same block sits in the stock document.
	// The cumulative re-merge follows it by line while the line count is
	// unchanged and falls back to a full re-parse when a structural edit has
	// moved everything. None of the three is reported.
	at           int
	pristineAt   int
	pristineLine int
}

// ContributorText renders the attribution as one cell of the report table.
func (f Flag) ContributorText() string {
	if len(f.Contributors) == 0 {
		return "not attributed"
	}
	parts := make([]string, 0, len(f.Contributors))
	for _, c := range f.Contributors {
		parts = append(parts, c.String())
	}
	return strings.Join(parts, ", ")
}

// Result is one table's findings (R1.4).
type Result struct {
	Flags []Flag `json:"flags"`
	// Unauditable is how many blocks could not be paired because an ADD or a
	// REMOVE changed the structure. They are not judged, and saying how many
	// were skipped is the difference between "clean" and "clean as far as this
	// could see".
	Unauditable int        `json:"unauditable"`
	Thresholds  Thresholds `json:"thresholds"`
	// Tables are the audited tables this covers, in the order they were merged.
	Tables []string `json:"tables"`
	// Blocks is how many reward blocks were compared.
	Blocks int `json:"blocks"`
}

/*
Parse reads every reward block out of an MXML's lines (R1.1).

Line-based, like the edit engine, and for the same reason: this has to see
exactly what the engine wrote, including the indentation the SECTION_UP walks
depend on. Depth is the tab count, so a child is found by depth rather than by
"the next line", which is what stops a nested Currency table from being read as
the reward's own item.
*/
func Parse(lines []string) []Block {
	var out []Block
	// ids[d] is the most recent Id seen at depth d, which is how the enclosing
	// entry is found in one forward pass rather than by scanning backwards from
	// every block.
	var ids []string

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		depth := mxml.NTabs(line)
		trimmed := strings.TrimSpace(line)

		if id, ok := selfClosingValue(trimmed, "Id"); ok {
			ids = setAt(ids, depth, id)
			continue
		}
		kind, ok := wrapperKind(trimmed)
		if !ok {
			continue
		}
		last := mxml.CloseIndex(lines, i)
		entry := enclosingID(ids, depth)
		if kind == KindMultiItems {
			out = append(out, multiItems(lines, i, last, entry)...)
		} else if b, ok := simpleBlock(lines, i, last, depth, kind, entry); ok {
			out = append(out, b)
		}
		i = last
	}
	return out
}

// wrapperKind matches an opening `<Property name="Gc…">` with no value
// attribute, which is the payload wrapper rather than the Reward line above it.
func wrapperKind(trimmed string) (string, bool) {
	for _, kind := range []string{
		KindSpecificProduct, KindSpecificSubstance, KindMoney,
		KindMultiProducts, KindMultiItems,
	} {
		if trimmed == `<Property name="`+kind+`">` {
			return kind, true
		}
	}
	return "", false
}

// simpleBlock reads a reward payload whose amounts are its own direct children.
func simpleBlock(lines []string, from, to, depth int, kind, entry string) (Block, bool) {
	b := Block{Kind: kind, EntryID: entry, Line: from + 1}
	var haveMin, haveMax bool
	for j := from + 1; j <= to && j < len(lines); j++ {
		d := mxml.NTabs(lines[j])
		trimmed := strings.TrimSpace(lines[j])
		if d == depth+2 && kind == KindMoney {
			// The currency is one level in, inside the GcCurrency table; the
			// outer line of that table is named "Currency" too and carries the
			// type name rather than the currency.
			if v, ok := selfClosingValue(trimmed, "Currency"); ok && b.Item == "" {
				b.Item = v
			}
			continue
		}
		if d != depth+1 {
			continue
		}
		if b.Item == "" && kind != KindMoney {
			for _, name := range []string{"ID", "Substance", "Id"} {
				if v, ok := selfClosingValue(trimmed, name); ok {
					b.Item = v
				}
			}
		}
		if v, ok := numericValue(trimmed, "AmountMin"); ok && !haveMin {
			b.Min, haveMin = v, true
		}
		if v, ok := numericValue(trimmed, "AmountMax"); ok && !haveMax {
			b.Max, haveMax = v, true
		}
		if v, ok := numericValue(trimmed, "Amount"); ok && !haveMin && !haveMax {
			b.Min, b.Max = v, v
			haveMin, haveMax = true, true
		}
	}
	if !haveMin && !haveMax {
		// A payload with no amount at all -- a product list with a random pick
		// and no count, say -- has nothing to audit.
		return Block{}, false
	}
	b.Category = categoryOf(kind, b.Item, "")
	return b, true
}

/*
multiItems reads a GcRewardMultiSpecificItems payload as one block per item.

The shape is different from every other reward: instead of one amount pair it
holds a list of entries, each with its own Id and a single Amount. Treating the
list as one block would audit the first entry and ignore the rest, so each entry
becomes a block of its own with Min and Max both set to its Amount.
*/
func multiItems(lines []string, from, to int, entry string) []Block {
	var out []Block
	const opener = `<Property name="Items" value="GcMultiSpecificItemEntry"`
	for j := from + 1; j <= to && j < len(lines); j++ {
		trimmed := strings.TrimSpace(lines[j])
		if !strings.HasPrefix(trimmed, opener) || strings.HasSuffix(trimmed, "/>") {
			continue
		}
		itemDepth := mxml.NTabs(lines[j])
		itemEnd := mxml.CloseIndex(lines, j)
		b := Block{Kind: KindMultiItems, EntryID: entry, Line: j + 1}
		rewardType := ""
		have := false
		for k := j + 1; k <= itemEnd && k < len(lines); k++ {
			if mxml.NTabs(lines[k]) != itemDepth+1 {
				continue
			}
			inner := strings.TrimSpace(lines[k])
			if v, ok := selfClosingValue(inner, "Id"); ok && b.Item == "" {
				b.Item = v
			}
			if v, ok := selfClosingValue(inner, "MultiItemRewardType"); ok {
				rewardType = v
			}
			if v, ok := numericValue(inner, "Amount"); ok && !have {
				b.Min, b.Max, have = v, v, true
			}
		}
		j = itemEnd
		if !have {
			continue
		}
		b.Category = categoryOf(KindMultiItems, b.Item, rewardType)
		out = append(out, b)
	}
	return out
}

// categoryOf decides which threshold a block's amount is judged against.
func categoryOf(kind, item, rewardType string) string {
	switch kind {
	case KindSpecificSubstance:
		return CategorySubstance
	case KindSpecificProduct, KindMultiProducts:
		return CategoryProduct
	case KindMultiItems:
		if strings.EqualFold(rewardType, "Substance") {
			return CategorySubstance
		}
		return CategoryProduct
	case KindMoney:
		switch strings.ToLower(item) {
		case "units":
			return CategoryUnits
		case "nanites":
			return CategoryNanites
		case "specials":
			return CategorySpecials
		}
	}
	return ""
}

// selfClosingValue reads `<Property name="NAME" value="V" />`.
func selfClosingValue(trimmed, name string) (string, bool) {
	prefix := `<Property name="` + name + `" value="`
	if !strings.HasPrefix(trimmed, prefix) || !strings.HasSuffix(trimmed, "/>") {
		return "", false
	}
	rest := trimmed[len(prefix):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// numericValue reads a named property's value as a number.
func numericValue(trimmed, name string) (float64, bool) {
	v, ok := selfClosingValue(trimmed, name)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// setAt records an Id at a depth and forgets anything deeper, which has gone
// out of scope by the time a shallower Id is seen.
func setAt(ids []string, depth int, id string) []string {
	for len(ids) <= depth {
		ids = append(ids, "")
	}
	ids[depth] = id
	for d := depth + 1; d < len(ids); d++ {
		ids[d] = ""
	}
	return ids
}

// enclosingID is the nearest Id recorded at a shallower depth (R1.1).
func enclosingID(ids []string, depth int) string {
	for d := min(depth, len(ids)) - 1; d >= 0; d-- {
		if ids[d] != "" {
			return ids[d]
		}
	}
	return ""
}

/*
Compare pairs pristine and merged blocks and flags the amounts that broke a
threshold (R1.2).

Pairing is by order when the counts match, which is the ordinary case and the
only one where order is meaningful: the engine's value edits never move a line.
When an ADD or a REMOVE has changed the count the order means nothing, so the
blocks are paired by (entry, kind, item) in sequence and whatever is left over
is counted as not auditable rather than compared against the wrong stock value.
*/
func Compare(table string, pristine, merged []Block, th Thresholds) Result {
	res := Result{Thresholds: th, Blocks: len(merged)}
	if table != "" {
		res.Tables = []string{table}
	}

	pairs, unmatched := pair(pristine, merged)
	res.Unauditable = unmatched
	for mergedIdx, pristineIdx := range pairs {
		m := merged[mergedIdx]
		if pristineIdx < 0 {
			continue
		}
		p := pristine[pristineIdx]
		reasons, ratio := judge(p, m, th)
		if len(reasons) == 0 {
			continue
		}
		res.Flags = append(res.Flags, Flag{
			Table: table, EntryID: m.EntryID, Kind: m.Kind, Item: m.Item,
			Category: m.Category, PristineMin: p.Min, PristineMax: p.Max,
			MergedMin: m.Min, MergedMax: m.Max, Ratio: ratio, Reasons: reasons,
			at: mergedIdx, pristineAt: pristineIdx, pristineLine: p.Line - 1,
		})
	}
	return res
}

/*
pair maps each merged block onto its pristine counterpart, or -1.

The key-based fallback walks both lists once and matches greedily in order: two
blocks with the same entry, kind and item are interchangeable for this purpose,
and pairing the first with the first is the only ordering either list carries.
*/
func pair(pristine, merged []Block) (pairs []int, unmatched int) {
	pairs = make([]int, len(merged))
	if len(pristine) == len(merged) {
		for i := range merged {
			pairs[i] = i
		}
		return pairs, 0
	}

	byKey := map[string][]int{}
	for i, b := range pristine {
		byKey[b.key()] = append(byKey[b.key()], i)
	}
	used := 0
	for i, b := range merged {
		queue := byKey[b.key()]
		if len(queue) == 0 {
			pairs[i] = -1
			unmatched++
			continue
		}
		pairs[i] = queue[0]
		byKey[b.key()] = queue[1:]
		used++
	}
	// Pristine blocks nothing matched were removed by a structural edit; they
	// are as unauditable as a merged block with no stock counterpart.
	unmatched += len(pristine) - used
	return pairs, unmatched
}

// judge tests one paired block against the thresholds and returns why it is
// flagged, in the order R1.2 lists them.
func judge(pristine, merged Block, th Thresholds) (reasons []string, ratio float64) {
	if limit, ok := th.limit(merged.Category); ok && merged.Max > limit {
		reasons = append(reasons, fmt.Sprintf("%s amount %s over the %s limit",
			merged.Category, Amount(merged.Max), Amount(limit)))
	}
	switch {
	case pristine.Max > 0:
		ratio = merged.Max / pristine.Max
	case pristine.Min > 0:
		ratio = merged.Min / pristine.Min
	}
	if th.MaxRatio > 0 && ratio > th.MaxRatio {
		reasons = append(reasons, fmt.Sprintf("x%s over the x%s limit",
			Amount(ratio), Amount(th.MaxRatio)))
	}
	if merged.Max == MaxInt32 || merged.Min == MaxInt32 {
		reasons = append(reasons, "int32 saturation")
	}
	return reasons, ratio
}

/*
Tracker follows the flagged blocks through a cumulative re-merge (R1.3).

The attribution question is "which mod made this 2,500,000", and the only source
of truth for it is running the merge again and looking after each edit. That is
one extra merge of the audited file, not one per mod: the cumulative state after
edits 1..k *is* the running document, so the caller applies one edit block and
calls Observe, and the tracker diffs the flagged blocks.

Two things bound the cost. Only flagged blocks are followed, so an Observe reads
a handful of positions rather than the document -- as long as the line count is
what the stock file had, which is true of every value edit and therefore of
almost every pass. When an ADD or a REMOVE has changed the count, that pass
falls back to a full re-parse, and if even the block count then matches neither
the stock nor the final document the observation is dropped rather than
attributing a change to whichever reward happens to sit at that index.
*/
type Tracker struct {
	flags []Flag
	// total and pristineTotal are the block counts of the final merge and of
	// the stock document, which is how a re-parsed pass is indexed.
	total         int
	pristineTotal int
	// lines is the stock document's line count. A pass with this many lines has
	// had nothing inserted or deleted, so the recorded line positions hold.
	lines int
	last  []Block
}

// NewTracker starts each flag from its pristine amounts, which is the state
// before the first edit.
func NewTracker(flags []Flag, pristine []Block, pristineLines, total int) *Tracker {
	t := &Tracker{
		flags: flags, total: total, pristineTotal: len(pristine),
		lines: pristineLines, last: make([]Block, len(flags)),
	}
	for i, f := range flags {
		t.last[i] = Block{Min: f.PristineMin, Max: f.PristineMax}
	}
	return t
}

/*
Observe records what one edit block did to the flagged blocks.

An edit that changed none of them records nothing, so the contributor list names
the mods that actually moved the value rather than every mod that touched the
file. A mod with two blocks that both multiply the same reward appears twice,
which is the honest answer: the value really was multiplied twice.
*/
func (t *Tracker) Observe(mod string, lines []string) {
	now, ok := t.read(lines)
	if !ok {
		return
	}
	for i := range t.flags {
		was := t.last[i]
		if now[i].Min == was.Min && now[i].Max == was.Max {
			continue
		}
		c := Contribution{Mod: mod, Min: now[i].Min, Max: now[i].Max}
		switch {
		case was.Max > 0:
			c.Factor = now[i].Max / was.Max
		case was.Min > 0:
			c.Factor = now[i].Min / was.Min
		}
		t.flags[i].Contributors = append(t.flags[i].Contributors, c)
		t.last[i] = now[i]
	}
}

// read takes the flagged blocks' current amounts, by line where it can and by a
// full re-parse where it must.
func (t *Tracker) read(lines []string) ([]Block, bool) {
	out := make([]Block, len(t.flags))
	if len(lines) == t.lines {
		ok := true
		for i := range t.flags {
			b, found := amountsAt(lines, t.flags[i].pristineLine)
			if !found {
				ok = false
				break
			}
			out[i] = b
		}
		if ok {
			return out, true
		}
	}

	blocks := Parse(lines)
	var index func(i int) int
	switch len(blocks) {
	case t.total:
		index = func(i int) int { return t.flags[i].at }
	case t.pristineTotal:
		index = func(i int) int { return t.flags[i].pristineAt }
	default:
		return nil, false
	}
	for i := range t.flags {
		at := index(i)
		if at < 0 || at >= len(blocks) {
			return nil, false
		}
		out[i] = blocks[at]
	}
	return out, true
}

/*
amountsAt re-reads one already-located block's amounts.

It is the cheap half of Observe: the block's kind and its enclosing entry were
settled when it was flagged and cannot change under a value edit, so all a pass
needs is the two numbers. The line is checked to still open the shape it opened
before, which is what makes the caller's "the line count is unchanged" test safe
rather than merely likely.
*/
func amountsAt(lines []string, at int) (Block, bool) {
	if at < 0 || at >= len(lines) {
		return Block{}, false
	}
	trimmed := strings.TrimSpace(lines[at])
	depth := mxml.NTabs(lines[at])
	if kind, isWrapper := wrapperKind(trimmed); isWrapper && kind != KindMultiItems {
		return simpleBlock(lines, at, mxml.CloseIndex(lines, at), depth, kind, "")
	}
	const opener = `<Property name="Items" value="GcMultiSpecificItemEntry"`
	if strings.HasPrefix(trimmed, opener) && !strings.HasSuffix(trimmed, "/>") {
		end := mxml.CloseIndex(lines, at)
		for k := at + 1; k <= end && k < len(lines); k++ {
			if mxml.NTabs(lines[k]) != depth+1 {
				continue
			}
			if v, ok := numericValue(strings.TrimSpace(lines[k]), "Amount"); ok {
				return Block{Kind: KindMultiItems, Min: v, Max: v}, true
			}
		}
	}
	return Block{}, false
}

// Flags returns the tracked flags with their attribution filled in.
func (t *Tracker) Flags() []Flag { return t.flags }

/*
Amount renders a number the way the report quotes it.

Whole numbers with no decimal point, because every amount in these tables is an
integer field and "250000.000000" in a report cell is noise. A fractional
multiplier keeps enough digits to be recognisable and no more.
*/
func Amount(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', 4, 64)
}

/*
Add folds one table's findings into a whole build's (R1.4).

A build audits more than one table, and the report has one section rather than
one per file, so the flags are concatenated and the counts summed. Sorting is
left to the caller: Add is used while targets are still arriving, and sorting on
every one of them would be work thrown away.
*/
func (r *Result) Add(other Result) {
	r.Flags = append(r.Flags, other.Flags...)
	r.Unauditable += other.Unauditable
	r.Blocks += other.Blocks
	r.Tables = append(r.Tables, other.Tables...)
	if r.Thresholds == (Thresholds{}) {
		r.Thresholds = other.Thresholds
	}
}

/*
Sort orders the flags the way the report reads them: worst first (R1.4).

By ratio descending, because the number that says "this is wrong" is how far the
value moved, not how large it is -- a mission payout of ten million from a stock
two million is fine and a salvage stack of 2,500,000 from a stock 4 is not.
Ties fall back to the merged amount and then to the names, so two runs of the
same build produce the same table.
*/
func (r *Result) Sort() {
	sort.SliceStable(r.Flags, func(i, j int) bool {
		a, b := r.Flags[i], r.Flags[j]
		switch {
		case a.Ratio != b.Ratio:
			return a.Ratio > b.Ratio
		case a.MergedMax != b.MergedMax:
			return a.MergedMax > b.MergedMax
		case a.Table != b.Table:
			return a.Table < b.Table
		case a.EntryID != b.EntryID:
			return a.EntryID < b.EntryID
		default:
			return a.Item < b.Item
		}
	})
}

// Clean reports an audit that found nothing, which is what the summary line and
// the green marker in the window both key off.
func (r *Result) Clean() bool { return r == nil || len(r.Flags) == 0 }

// Summary is the one-line form the CLI prints after a build (R1.5).
func (r *Result) Summary() string {
	if r == nil {
		return "not run"
	}
	if len(r.Flags) == 0 {
		if r.Unauditable > 0 {
			return fmt.Sprintf("clean (%d block(s) not auditable)", r.Unauditable)
		}
		return "clean"
	}
	return fmt.Sprintf("%d flagged; see report", len(r.Flags))
}

/*
Range renders a block's amounts as the report's Stock and Built columns.

One number when the minimum and the maximum agree, which most rewards do, and a
range when they do not -- printing "4-4" for every fixed reward would make the
two that are genuinely ranges harder to spot rather than easier.
*/
func Range(minv, maxv float64) string {
	if minv == maxv {
		return Amount(maxv)
	}
	return Amount(minv) + "-" + Amount(maxv)
}
