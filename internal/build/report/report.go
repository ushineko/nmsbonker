/*
Package report is the build's result model and its two renderings (spec 002 R4.4).

The Markdown is a port of the reference BUILD_REPORT.md, prose included. That is
deliberate: the file is what the user reads after every game update to decide
which mods to re-download, and the wording of its "how to fix" sections is the
accumulated answer to that question. The JSON beside it is what `nmsbonker
report` and the GUI read back.
*/
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/nmsbonker/internal/build/audit"
	"github.com/ushineko/nmsbonker/internal/mxml"
)

// Verdicts, in the order the table sorts them (R4.4).
const (
	// Working means every edit the mod asked for landed.
	Working = "WORKING"
	// WorkingSkipped means the mod applied edits but some keys were missing,
	// usually because a game update renamed or removed them.
	WorkingSkipped = "WORKING~"
	// WorkingStructural means the mod adds or removes whole entries and the
	// result recompiled; it needs verifying in game.
	WorkingStructural = "WORKING*"
	// Partial means the value edits shipped but the structural ones were
	// dropped to get the file to recompile.
	Partial = "PARTIAL"
	// NotBuilt means nothing the mod asked for applied.
	NotBuilt = "NOT BUILT"
)

// Target outcomes (R4.4).
const (
	OutcomeBuilt    = "built"
	OutcomeDegraded = "degraded"
	OutcomeDropped  = "dropped"
	OutcomeNoSource = "no-source"
)

// rank orders the table: the mods that need attention sort last, where a reader
// who stops halfway has still seen the ones that are fine.
func rank(verdict string) int {
	switch verdict {
	case Working:
		return 0
	case WorkingSkipped:
		return 1
	case WorkingStructural:
		return 2
	case Partial:
		return 3
	case NotBuilt:
		return 4
	default:
		return 9
	}
}

// ModResult is one row of the report table.
type ModResult struct {
	Name    string `json:"name"`
	Verdict string `json:"verdict"`
	Applied int    `json:"applied"`
	Skipped int    `json:"skipped"`
	// Files are the built MBINs this mod contributed to.
	Files []string `json:"files"`
	// NotFound lists the keys its edits could not find, in the order reported.
	NotFound []string `json:"notFound"`
}

// TargetResult is what happened to one game file.
type TargetResult struct {
	// Key is the normalised source path the target was grouped under.
	Key string `json:"key"`
	// Source is the spelling the first contributing script used.
	Source string `json:"source"`
	// Internal is the pak-internal path, upper-cased, which is also the output
	// path under the mod folder.
	Internal string `json:"internal"`
	Blocks   int    `json:"blocks"`
	Outcome  string `json:"outcome"`
	// Output is the MBIN written, absolute.
	Output string `json:"output,omitempty"`
	// SkippedMods are the mods whose structural edits were left out to make the
	// file recompile.
	SkippedMods []string `json:"skippedMods,omitempty"`
	// Error is why a dropped or source-less target failed.
	Error string `json:"error,omitempty"`
	// Mods are the scripts that contributed edits, in order.
	Mods []string `json:"mods"`
}

// CompilerFailure is a game file MBINCompiler could not handle: it did not
// decompile from the pak, or the merged result did not recompile. These are
// the symptom of a compiler that does not match the game, which a game update
// or a bad compiler release produces, and they are reported apart from
// everything else because the fix is a different compiler, not a different mod.
type CompilerFailure struct {
	// Internal is the pak-internal path, or the source spelling when the file
	// never got as far as an internal path.
	Internal string `json:"internal"`
	// Stage is "decompile" or "recompile".
	Stage string `json:"stage"`
	// Detail is the compiler's own account.
	Detail string `json:"detail"`
	// Mods are the scripts that wanted the file, in order.
	Mods []string `json:"mods,omitempty"`
}

// Compiler failure stages.
const (
	StageDecompile = "decompile"
	StageRecompile = "recompile"
)

// FirstLine is a compiler message cut to its first line, for a table cell or
// a report bullet; the full text is in the JSON.
func FirstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i]) + " …"
	}
	return s
}

// DegradedFile records a file that shipped without its structural edits.
type DegradedFile struct {
	Internal string   `json:"internal"`
	Mods     []string `json:"mods"`
}

// Timings are the measured phases of a build (R4.4).
type Timings struct {
	Cache   time.Duration `json:"cache"`
	Merge   time.Duration `json:"merge"`
	Compile time.Duration `json:"compile"`
	// Audit is the reward-amount audit, including the cumulative re-merge the
	// attribution needs (spec 005 R1.3). Summed across workers like the rest.
	Audit time.Duration `json:"audit"`
	Total time.Duration `json:"total"`
}

// Line is one report line, kept in both renderings so the JSON carries the same
// detail the console printed.
type Line struct {
	Kind     string `json:"kind"`
	Mod      string `json:"mod,omitempty"`
	Detail   string `json:"detail"`
	File     string `json:"file,omitempty"`
	NotFound string `json:"notFound,omitempty"`
}

// Result is the whole outcome of one build (R4.4).
type Result struct {
	ModName   string    `json:"modName"`
	Generated time.Time `json:"generated"`
	OutputDir string    `json:"outputDir"`

	Built   int `json:"built"`
	Dropped int `json:"dropped"`
	Applied int `json:"applied"`
	Skipped int `json:"skipped"`

	Targets  []TargetResult `json:"targets"`
	Mods     []ModResult    `json:"mods"`
	Degraded []DegradedFile `json:"degraded"`
	// CompilerFailures are the files MBINCompiler could not decompile or
	// recompile, the sign of a compiler that does not match the game.
	CompilerFailures []CompilerFailure `json:"compilerFailures,omitempty"`
	// Complex lists mods carrying ADD or REMOVE, in first-seen order.
	Complex []string `json:"complex"`
	Lines   []Line   `json:"lines"`

	Timings         Timings `json:"timings"`
	CompilerVersion string  `json:"compilerVersion"`
	GameBuildID     string  `json:"gameBuildId"`
	// Compatibility is the round-trip check's verdict (R3.4).
	Compatibility       string `json:"compatibility"`
	CompatibilityDetail string `json:"compatibilityDetail,omitempty"`

	// UnsupportedKeys are script keys this engine ignored, across all mods.
	UnsupportedKeys []string `json:"unsupportedKeys"`
	// CacheMisses are sources no pak held, or that would not decompile.
	CacheMisses []string `json:"cacheMisses"`
	CacheReused int      `json:"cacheReused"`
	CacheBuilt  int      `json:"cacheBuilt"`
	// Workers is how many targets were processed at once, which is what makes
	// the summed merge and compile timings larger than the wall clock.
	Workers int `json:"workers"`
	/*
		Params records the parameter overrides this build was produced with
		(spec 004 R2.1).

		It is what makes "unbuilt changes" a fact rather than a session flag: a
		front end compares the settings in force against this and knows whether
		the mod folder on disk was built from them, across restarts and across
		front ends. It is also the answer to "what was this built with" three
		weeks later, which is why it is in the Markdown as well.
	*/
	Params map[string]map[string]float64 `json:"params,omitempty"`

	/*
		Audit is what the merged reward amounts came out at (spec 005 R1.4).

		Nil when no audited table was part of the build, which is not the same
		as clean and is why the field is a pointer: "nothing to audit" and
		"audited and found nothing" are different answers to "is my reward table
		sane", and a front end that showed a green marker for the first would be
		making a claim nobody checked.
	*/
	Audit *audit.Result `json:"audit,omitempty"`
	// Capped is how many values a block's CAP held back across the whole build
	// (spec 005 R2.2). Zero when no cap bit, which is the ordinary state.
	Capped int `json:"capped"`
}

// Events converts engine events into the stored line form.
func Events(events []mxml.Event) []Line {
	out := make([]Line, 0, len(events))
	for _, e := range events {
		out = append(out, Line{
			Kind: e.Kind.String(), Mod: e.Mod, Detail: e.Detail,
			File: e.File, NotFound: e.NotFound,
		})
	}
	return out
}

// Render renders a stored line the way the console printed it.
func (l Line) Render() string {
	switch l.Kind {
	case "ok":
		return "   OK  " + l.Mod + ": " + l.Detail
	case "warn":
		return "  WARN " + l.Mod + ": " + l.Detail
	default:
		return "       " + l.Detail
	}
}

/*
paramLine summarises the non-default parameters one line of the header.

Only the ones the user changed: the defaults are in the scripts and repeating
them would bury the two numbers that are actually interesting in twenty that
are not.
*/
func paramLine(r *Result) string {
	mods := make([]string, 0, len(r.Params))
	for mod, params := range r.Params {
		if len(params) > 0 {
			mods = append(mods, mod)
		}
	}
	if len(mods) == 0 {
		return ""
	}
	sort.Strings(mods)
	var parts []string
	for _, mod := range mods {
		names := make([]string, 0, len(r.Params[mod]))
		for name := range r.Params[mod] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			parts = append(parts, fmt.Sprintf("%s.%s=%s", mod, name,
				strconv.FormatFloat(r.Params[mod][name], 'f', -1, 64)))
		}
	}
	return strings.Join(parts, ", ")
}

// SortMods orders the table rows by verdict then name (R4.4).
func SortMods(mods []ModResult) {
	sort.SliceStable(mods, func(i, j int) bool {
		ri, rj := rank(mods[i].Verdict), rank(mods[j].Verdict)
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(mods[i].Name) < strings.ToLower(mods[j].Name)
	})
}

// maxNotedKeys is how many missing keys a WORKING~ row names before eliding.
const maxNotedKeys = 6

/*
Markdown renders BUILD_REPORT.md (R4.4).

The structure and most of the prose are the reference report's. Three header lines
are new: the game build the report was produced against, the compiler
compatibility verdict from the round-trip check (R3.4), and the script keys this
engine ignored -- each of them a thing that silently changes what a build
produces and that the reference report gave the reader no way to see.
*/
func Markdown(r *Result) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	w("# No Man's Sky combined-mod build report")
	w("")
	w("- Generated: %s", r.Generated.Format("2006-01-02 15:04"))
	w("- Game: No Man's Sky, Steam buildid %s", orUnknown(r.GameBuildID))
	w("- Compiler: %s", orUnknown(r.CompilerVersion))
	w("- Compiler compatibility: %s", compatLine(r))
	w("- Output: `MODS/%s/` (%d merged MBIN files)", r.ModName, r.Built)
	w("- Totals: %d MBINs built, %d dropped, %d edits applied, %d skipped",
		r.Built, r.Dropped, r.Applied, r.Skipped)
	if len(r.UnsupportedKeys) > 0 {
		w("- Unsupported script keys ignored: %s", strings.Join(r.UnsupportedKeys, ", "))
	}
	if len(r.CacheMisses) > 0 {
		w("- Sources not found in the game's paks: %s", strings.Join(r.CacheMisses, ", "))
	}
	if len(r.CompilerFailures) > 0 {
		w("- **MBINCompiler %s could not handle %d file(s)** — a compiler that does not match "+
			"this game build; run `nmsbonker tools check` and pin a matching release:",
			orUnknown(r.CompilerVersion), len(r.CompilerFailures))
		for _, f := range r.CompilerFailures {
			w("  - `%s` did not %s: %s", f.Internal, f.Stage, FirstLine(f.Detail))
		}
	}
	if line := paramLine(r); line != "" {
		w("- Parameters changed from their defaults: %s", line)
	}
	// Merge and compile are sums over the targets, which run concurrently, so
	// they are routinely larger than the wall clock. Saying so beats a reader
	// concluding the numbers are wrong.
	w("- Timings: %s wall clock; cache %s, merge %s, audit %s and compile %s summed across %d worker(s)",
		round(r.Timings.Total), round(r.Timings.Cache), round(r.Timings.Merge),
		round(r.Timings.Audit), round(r.Timings.Compile), r.Workers)
	if r.Capped > 0 {
		w("- Capped %d value(s) at a tweak's CAP", r.Capped)
	}
	w("")
	writeAudit(w, r)
	w("Legend: **WORKING** all edits applied; **WORKING~** applied, some keys not " +
		"found (renamed/removed by a game update — verify); **WORKING(star)** applies " +
		"structural add/remove that recompiled — verify in-game; **PARTIAL** value edits " +
		"applied but structural add/remove skipped; **NOT BUILT** nothing applied.")
	w("")
	w("| Mod | Status | Edits | Skipped | Notes |")
	w("|-----|--------|------:|--------:|-------|")
	for _, m := range r.Mods {
		w("| %s | %s | %d | %d | %s |", m.Name, m.Verdict, m.Applied, m.Skipped, note(m))
	}
	w("")

	if len(r.Degraded) > 0 {
		w("## Files where structural edits were skipped")
		for _, d := range r.Degraded {
			w("- `%s` — from %s (recompile rejected the added/removed XML; the file still "+
				"ships with all value edits)", d.Internal, strings.Join(d.Mods, ", "))
		}
		w("")
	}

	w("## How to fix a PARTIAL / WORKING(star) mod")
	w("- These add or remove whole reward/table entries, which this builder applies heuristically.")
	w("- If in-game behaviour is wrong, download the updated mod from Nexus and drop its built "+
		"`.MBIN` into `MODS/%s/` (same relative path), or rebuild that one script in AMUMSS on Windows.", r.ModName)
	w("")
	w("## NOT BUILT / limited")
	w("- Mods marked NOT BUILT use section-matching (SPECIAL_KEY_WORDS) that this builder " +
		"resolves too loosely for their target structure, or target keys the game renamed or " +
		"removed. They ship nothing (the game keeps stock).")
	w("- WORKING~ 'keys not found' are usually values a game update removed (the scanner rework " +
		"dropped PulseRange/ChargeTime from gameplayglobals, for instance); the applied edits " +
		"still land.")
	w("- For any of these, the reliable fix is an updated Nexus download dropped into the " +
		"combine folder, or a Windows AMUMSS build of that one script.")
	return b.String()
}

// maxAuditRows is how many flagged amounts the Markdown table lists before it
// says how many more there are. Fifty is a page; a build with a compounding
// script in it produces hundreds, and the hundredth is not read.
const maxAuditRows = 50

/*
writeAudit renders the `## Amount audit` section (R1.4).

It sits directly under the totals because it is a fact about the build as a
whole, ahead of the per-mod table: the mod table says every edit applied, and
this section is the one that can say the result is nevertheless wrong.

The advice paragraph is the point of the section. A flagged amount is not a
failure of any one mod -- each edit was correct -- so "which mod do I remove" has
no answer without it, and the two real answers (stop the compounding, or put a
ceiling on it) are the ones spelled out.
*/
func writeAudit(w func(string, ...any), r *Result) {
	a := r.Audit
	if a == nil {
		return
	}
	w("## Amount audit")
	w("")
	if len(a.Flags) == 0 {
		w("No reward amount exceeds the configured limits (%d block(s) checked in %s).",
			a.Blocks, orUnknown(strings.Join(a.Tables, ", ")))
		if a.Unauditable > 0 {
			w("")
			w("%d block(s) could not be checked because an ADD or REMOVE changed the "+
				"table's structure.", a.Unauditable)
		}
		w("")
		return
	}

	w("%d of %d reward amount(s) in %s are above the configured limits "+
		"(product %s, substance %s, units %s, nanites %s, quicksilver %s, ratio x%s).",
		len(a.Flags), a.Blocks, orUnknown(strings.Join(a.Tables, ", ")),
		audit.Amount(a.Thresholds.MaxProduct), audit.Amount(a.Thresholds.MaxSubstance),
		audit.Amount(a.Thresholds.MaxUnits), audit.Amount(a.Thresholds.MaxNanites),
		audit.Amount(a.Thresholds.MaxSpecials), audit.Amount(a.Thresholds.MaxRatio))
	w("")
	w("| Table | Entry | Item | Stock | Built | x | Why | Contributors |")
	w("|-------|-------|------|------:|------:|--:|-----|--------------|")
	shown := a.Flags
	if len(shown) > maxAuditRows {
		shown = shown[:maxAuditRows]
	}
	for _, f := range shown {
		w("| %s | %s | %s | %s | %s | %s | %s | %s |",
			f.Table, orUnknown(f.EntryID), orUnknown(f.Item),
			audit.Range(f.PristineMin, f.PristineMax),
			audit.Range(f.MergedMin, f.MergedMax),
			ratioText(f.Ratio), strings.Join(f.Reasons, "; "), f.ContributorText())
	}
	if len(a.Flags) > len(shown) {
		w("")
		w("%d further flagged amount(s) are not listed; `report.json` carries all of them.",
			len(a.Flags)-len(shown))
	}
	if a.Unauditable > 0 {
		w("")
		w("%d block(s) could not be checked because an ADD or REMOVE changed the table's "+
			"structure.", a.Unauditable)
	}
	w("")
	w("Every edit above applied correctly; the amounts are large because they " +
		"compound. Each mod multiplies what the mod before it left, so two reasonable " +
		"multipliers make an unreasonable amount and nothing in the per-mod table can " +
		"see it. Two ways out, and they can be combined: disable or re-tune the script " +
		"named most often in the Contributors column, which is the one doing most of " +
		"the multiplying; or put a ceiling on it with the built-in tweaks' cap " +
		"parameters (Tweaks, or `nmsbonker tweaks set NAME LOOT_CAP 50000`), which " +
		"clamps the result whatever ran before it. The limits themselves are settings: " +
		"`nmsbonker config set audit.max_ratio 5` and re-run `nmsbonker audit` to " +
		"re-check without rebuilding.")
	w("")
}

// ratioText renders the x column, and a dash where the stock value was zero and
// there is no ratio to state.
func ratioText(ratio float64) string {
	if ratio <= 0 {
		return "-"
	}
	return "x" + audit.Amount(ratio)
}

func note(m ModResult) string {
	switch m.Verdict {
	case Partial:
		return "new-entry add/remove not applied; base value edits kept"
	case WorkingStructural:
		return "adds/removes reward or text entries — confirm in game"
	case WorkingSkipped:
		seen := map[string]bool{}
		var keys []string
		for _, k := range m.NotFound {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		suffix := ""
		if len(keys) > maxNotedKeys {
			keys = keys[:maxNotedKeys]
			suffix = " …"
		}
		return "keys not found: " + strings.Join(keys, ", ") + suffix
	default:
		return ""
	}
}

func compatLine(r *Result) string {
	if r.CompatibilityDetail == "" {
		return strings.ToUpper(orUnknown(r.Compatibility))
	}
	return strings.ToUpper(orUnknown(r.Compatibility)) + " (" + r.CompatibilityDetail + ")"
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func round(d time.Duration) string { return d.Round(time.Millisecond).String() }

// JSON renders the machine-readable form (R4.4).
func JSON(r *Result) ([]byte, error) {
	b, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		return nil, fmt.Errorf("encode build report: %w", err)
	}
	return append(b, '\n'), nil
}

// Paths are where a written report landed.
type Paths struct {
	Dir       string
	Markdown  string
	JSON      string
	Timestamp string
}

/*
Write saves the report under dir, both as "latest" and under a timestamp (R4.4).

Two copies rather than one because the two readers want different things: the
GUI and `nmsbonker report` want a stable path, and a user comparing this build
with the one before a game update wants the old one to still exist. Timestamped
directories are cheap -- a report is a few kilobytes -- and nothing deletes them
automatically, which is the right default for a file whose whole job is to be
looked at later.
*/
func Write(dir string, r *Result) (Paths, error) {
	stamp := r.Generated.UTC().Format("20060102-150405")
	latest := filepath.Join(dir, "latest")

	md := Markdown(r)
	js, err := JSON(r)
	if err != nil {
		return Paths{}, err
	}
	for _, target := range []string{latest, filepath.Join(dir, stamp)} {
		if err := os.MkdirAll(target, 0o750); err != nil {
			return Paths{}, fmt.Errorf("create %s: %w", target, err)
		}
		if err := os.WriteFile(filepath.Join(target, "BUILD_REPORT.md"), []byte(md), 0o600); err != nil {
			return Paths{}, fmt.Errorf("write %s: %w", target, err)
		}
		if err := os.WriteFile(filepath.Join(target, "report.json"), js, 0o600); err != nil {
			return Paths{}, fmt.Errorf("write %s: %w", target, err)
		}
	}
	return Paths{
		Dir:       latest,
		Markdown:  filepath.Join(latest, "BUILD_REPORT.md"),
		JSON:      filepath.Join(latest, "report.json"),
		Timestamp: filepath.Join(dir, stamp),
	}, nil
}

// Load reads back the latest report.json (R6.3 core.Report).
func Load(path string) (*Result, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &r, nil
}
