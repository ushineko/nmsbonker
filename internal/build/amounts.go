package build

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/nmsbonker/internal/build/audit"
	"github.com/ushineko/nmsbonker/internal/mxml"
)

/*
The reward-amount audit, as a step of the build (spec 005 R1).

The interesting part is the attribution, and the shape it takes here is the
whole reason it is affordable. R1.3 asks which mods moved a flagged amount, and
the only honest answer comes from running the merge again and looking after each
edit. Doing that as "re-merge the first k mods, for k = 1..n" would be quadratic
in a nine-megabyte document; doing it as one merge with a look after each block
is linear, because the state after k edits *is* the running document.

The audit only runs on the tables where amounts can compound into something the
game cannot hold -- the two reward tables -- so a build whose mods edit neither
of them pays nothing for any of this.
*/

// maxAuditWarnings is how many flagged amounts reach the build log as warnings
// (R1.6). The report has all of them; a log with four hundred AUDIT lines in it
// is a log nobody scrolls, and the summary line says how many there were.
const maxAuditWarnings = 25

/*
audit compares one merged target against its pristine source and attributes what
it flags (R1.1, R1.3).

Returns nil for a file that is not one of the audited tables, which is most of
them: the caller distinguishes "not audited" from "audited and clean", and so
does the report.
*/
func (r *runner) audit(t *Target, pristine, merged []string, internal string) (*audit.Result, []mxml.Event) {
	table, ok := audit.TableName(internal)
	if !ok {
		return nil, nil
	}

	stock := audit.Parse(pristine)
	built := audit.Parse(merged)
	res := audit.Compare(table, stock, built, r.opts.Audit)
	if len(res.Flags) > 0 {
		res.Flags = attribute(res.Flags, pristine, stock, len(built), t.Items)
	}
	return &res, auditEvents(table, res.Flags)
}

/*
attribute re-runs the merge one edit block at a time, watching the flagged
blocks (R1.3).

The pass is fed the same items in the same order as the real merge, so what it
sees is what happened -- including a mod applying its own multiplier twice,
which is how a stock amount of 4 becomes 250,000 before any other mod has
touched it. A block that changes none of the flagged amounts records nothing, so
the contributor list is the mods that actually moved the number rather than
every mod that edited the file.

Panics are swallowed the same way the real merge swallows them: this is a second
run of code that already ran, so a panic here is a bug in the tracking rather
than something the user can act on, and it must not take a finished build down.
*/
func attribute(flags []audit.Flag, pristine []string, stock []audit.Block,
	total int, items []Item,
) []audit.Flag {
	tracker := audit.NewTracker(flags, stock, len(pristine), total)
	lines := slices.Clone(pristine)
	for _, it := range items {
		lines, _ = applyOne(lines, it)
		tracker.Observe(it.Mod, lines)
	}
	return tracker.Flags()
}

/*
auditEvents turns flags into build-log warnings (R1.6).

They carry the table as their "mod" so the rendered line names what was audited,
and they are marked so the per-mod tallies leave them alone: a flagged amount is
not a skipped edit, and counting it as one would turn a mod whose every edit
applied into a WORKING~ row.
*/
func auditEvents(table string, flags []audit.Flag) []mxml.Event {
	if len(flags) == 0 {
		return nil
	}
	shown := flags
	if len(shown) > maxAuditWarnings {
		shown = shown[:maxAuditWarnings]
	}
	out := make([]mxml.Event, 0, len(shown)+1)
	for _, f := range shown {
		out = append(out, mxml.Event{
			Kind: mxml.WARN, Mod: table, Audit: true,
			Detail: "AUDIT " + f.EntryID + "/" + f.Item + " " +
				audit.Range(f.PristineMin, f.PristineMax) + " -> " +
				audit.Range(f.MergedMin, f.MergedMax) + " (x" + audit.Amount(f.Ratio) + "): " +
				strings.Join(contributorNames(f), ", "),
		})
	}
	if len(flags) > len(shown) {
		out = append(out, mxml.Event{
			Kind: mxml.WARN, Mod: table, Audit: true,
			Detail: "AUDIT " + strconv.Itoa(len(flags)-len(shown)) +
				" further flagged amount(s) are in the report",
		})
	}
	return out
}

// contributorNames is the mod list a log line ends with, in build order and
// repeated where a mod multiplied the same amount more than once.
func contributorNames(f audit.Flag) []string {
	if len(f.Contributors) == 0 {
		return []string{"not attributed"}
	}
	out := make([]string, 0, len(f.Contributors))
	for _, c := range f.Contributors {
		out = append(out, c.Mod)
	}
	return out
}

/*
Re-running the audit over a merge that already happened (R1.5).

The build keeps its merged MXML under <MOD_NAME>.work/mxml so that a rejected
file can be looked at. That makes the audit re-runnable for free: the merged
document and the pristine cache are both still on disk, so changing a threshold
and asking again costs a parse rather than a build. Which is the point --
"is x100 the right ratio limit here" is a question you answer by trying three
numbers, and three builds is ten minutes.
*/

// AuditOptions configure a re-run over the kept merge (R1.5).
type AuditOptions struct {
	// Plan is the same plan a build would run, which supplies the targets and,
	// for the attribution, the edit blocks in build order.
	Plan *Plan
	// Sources maps cache.Key(source) to the pristine file, exactly as a build's
	// Options.Sources does.
	Sources map[string]Source
	// Workspace and ModName locate <Workspace>/<ModName>.work/mxml.
	Workspace string
	ModName   string
	// Thresholds are the limits to judge against; the zero value is the
	// defaults.
	Thresholds audit.Thresholds
}

// AuditRun is what a re-run found (R1.5).
type AuditRun struct {
	Result *audit.Result `json:"result"`
	// Missing names audited tables whose merged MXML is not in the workspace,
	// which is what a re-run after a `build --recache` that dropped the file,
	// or before any build at all, looks like.
	Missing []string `json:"missing,omitempty"`
	// Checked names the tables that were compared.
	Checked  []string      `json:"checked"`
	Duration time.Duration `json:"duration"`
}

/*
AuditKeptMerge re-audits the merged MXML a previous build left in the workspace.

It deliberately does not rebuild, recompile or touch the cache: everything it
reads is a file that is already there, and the answer it gives is about the
build that produced them, not about a build that would happen now. If the
settings have moved on since, the report's own header is where that shows.
*/
func AuditKeptMerge(opts AuditOptions) (AuditRun, error) {
	started := time.Now()
	if opts.Thresholds == (audit.Thresholds{}) {
		opts.Thresholds = audit.Defaults()
	}
	out := AuditRun{Result: &audit.Result{Thresholds: opts.Thresholds}}
	mxmlDir := filepath.Join(opts.Workspace, opts.ModName+".work", "mxml")

	for _, t := range opts.Plan.Targets {
		src, ok := opts.Sources[t.Key]
		if !ok {
			continue
		}
		internal := InternalUpper(src.Internal)
		table, audited := audit.TableName(internal)
		if !audited {
			continue
		}
		rel := strings.TrimSuffix(internal, filepath.Ext(internal)) + ".MXML"
		mergedPath := filepath.Join(mxmlDir, filepath.FromSlash(rel))
		merged, err := readLines(mergedPath)
		if err != nil {
			out.Missing = append(out.Missing, table)
			continue
		}
		pristine, err := readLines(src.MXML)
		if err != nil {
			return out, err
		}

		stock := audit.Parse(pristine)
		built := audit.Parse(merged)
		res := audit.Compare(table, stock, built, opts.Thresholds)
		if len(res.Flags) > 0 {
			res.Flags = attribute(res.Flags, pristine, stock, len(built), t.Items)
		}
		out.Result.Add(res)
		out.Checked = append(out.Checked, table)
	}
	out.Result.Sort()
	out.Duration = time.Since(started)
	return out, nil
}

// readLines splits a file the way the engine and the audit both see it.
func readLines(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return strings.Split(string(raw), "\n"), nil
}
