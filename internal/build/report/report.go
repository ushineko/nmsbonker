/*
Package report is the build's result model and its two renderings (spec 002 R4.4).

The Markdown is a port of the legacy BUILD_REPORT.md, prose included. That is
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
	"strings"
	"time"

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
	Total   time.Duration `json:"total"`
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

The structure and most of the prose are the legacy report's. Three header lines
are new: the game build the report was produced against, the compiler
compatibility verdict from the round-trip check (R3.4), and the script keys this
engine ignored -- each of them a thing that silently changes what a build
produces and that the legacy report gave the reader no way to see.
*/
func Markdown(r *Result) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	w("# No Man's Sky combined-mod build report")
	w("")
	w("- Generated: %s", r.Generated.Format("2006-01-02 15:04"))
	w("- Game: No Man's Sky, Steam buildid %s; MBINCompiler %s", orUnknown(r.GameBuildID), orUnknown(r.CompilerVersion))
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
	w("- Timings: cache %s, merge %s, compile %s, total %s",
		round(r.Timings.Cache), round(r.Timings.Merge), round(r.Timings.Compile), round(r.Timings.Total))
	w("")
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
