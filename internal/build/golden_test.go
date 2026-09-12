package build_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/build"
	"github.com/ushineko/nmsbonker/internal/build/cache"
	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/mbin"
	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/mxml"
)

/*
Golden parity against the legacy Python builder (spec 002 R8).

The fixtures are game-derived and are never committed: they are generated on the
user's machine by tools/legacy/make_golden.py running the legacy builder against
the installed game, and these tests skip unless they are pointed at them.

Three stages, each answering a different question. A: does the embedded Lua
interpreter decode the same change tables the legacy dumper did. B: does the
edit engine produce byte-identical merged documents and the same 504 report
lines. C: does the whole pipeline, against the real game and the real compiler,
reach the same per-mod verdicts.
*/
func goldenDirs(t *testing.T) (golden, legacy string) {
	t.Helper()
	golden = os.Getenv("NMSBONKER_GOLDEN_DIR")
	legacy = os.Getenv("NMSBONKER_LEGACY_DIR")
	if golden == "" || legacy == "" {
		t.Skip("NMSBONKER_GOLDEN_DIR and NMSBONKER_LEGACY_DIR are unset; skipping the golden parity tests")
	}
	return golden, legacy
}

// goldenOrder reads the build order out of the legacy mods.conf.
func goldenOrder(t *testing.T, golden string) []config.ModEntry {
	t.Helper()
	f, err := os.Open(filepath.Join(golden, "mods.conf"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	var out []config.ModEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, flag, found := strings.Cut(line, "\t")
		enabled := true
		if found {
			switch strings.ToLower(strings.TrimSpace(flag)) {
			case "0", "n", "no", "off", "false":
				enabled = false
			}
		}
		out = append(out, config.ModEntry{Name: strings.TrimSpace(name), Enabled: enabled})
	}
	require.NoError(t, sc.Err())
	require.NotEmpty(t, out)
	return out
}

/*
R8.2 -- Stage A: the Lua stage.

Semantic, not textual: Lua's pairs() has no defined order, so the legacy dumper's
key order is whatever that run produced. Numbers compare as float64 through the
JSON decoder, which is exactly the comparison the engine's inputs need to
survive.
*/
func TestGoldenStageAEveryScriptDecodesToTheLegacyDump(t *testing.T) {
	golden, legacy := goldenDirs(t)

	for _, entry := range goldenOrder(t, golden) {
		t.Run(entry.Name, func(t *testing.T) {
			def, err := modscript.Load(t.Context(), filepath.Join(legacy, "lua-src", entry.Name+".lua"))
			require.NoError(t, err)

			var got, want any
			require.NoError(t, json.Unmarshal(modscript.DumpJSON(def), &got))
			raw, err := os.ReadFile(filepath.Join(golden, "dumped", entry.Name+".json"))
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &want))
			require.Equal(t, want, got)
		})
	}
}

// goldenIndex is the shape make_golden.py wrote.
type goldenIndex struct {
	Targets []struct {
		Target   string `json:"target"`
		Internal string `json:"internal"`
		Blocks   int    `json:"blocks"`
		Merged   string `json:"merged"`
	} `json:"targets"`
	Applied int `json:"applied"`
	Skipped int `json:"skipped"`
}

type manifestEntry struct {
	Internal string `json:"internal"`
	Pak      string `json:"pak"`
	MXML     string `json:"mxml"`
}

/*
R8.3 -- Stage B: the edit engine.

Every target's merged text is compared byte for byte against what the legacy
builder produced from the same pristine input, and every report line against the
504 it printed. This is the acceptance oracle for the whole engine: a single
differing byte means a mod behaves differently in game than it did before the
port, and the report lines catch the cases where the bytes happen to agree but
the engine took a different path to them.

No compiler and no game install are needed: the pristine MXMLs come from the
legacy cache the fixtures were generated against.
*/
func TestGoldenStageBTheMergeIsByteIdenticalAndSoIsTheReport(t *testing.T) {
	golden, legacy := goldenDirs(t)

	var index goldenIndex
	raw, err := os.ReadFile(filepath.Join(golden, "index.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &index))

	var manifest map[string]manifestEntry
	raw, err = os.ReadFile(filepath.Join(golden, "manifest.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &manifest))

	order := goldenOrder(t, golden)
	scripts := make([]build.Script, 0, len(order))
	for _, entry := range order {
		def, err := modscript.Load(t.Context(), filepath.Join(legacy, "lua-src", entry.Name+".lua"))
		require.NoError(t, err)
		scripts = append(scripts, build.Script{Name: entry.Name, Enabled: entry.Enabled, Def: def})
	}
	plan := build.NewPlan(scripts)
	require.Len(t, plan.Targets, len(index.Targets), "the same targets, grouped the same way")

	var lines []string
	for i, target := range plan.Targets {
		require.Equal(t, index.Targets[i].Target, target.Key, "targets are in first-seen order")
		require.Len(t, target.Items, index.Targets[i].Blocks)

		entry, ok := manifest[cache.Key(target.Source)]
		require.True(t, ok, "the legacy manifest has %s", target.Key)

		pristine, err := os.ReadFile(filepath.Join(legacy, entry.MXML))
		require.NoError(t, err)
		merged := strings.Split(string(pristine), "\n")
		for _, item := range target.Items {
			var events []mxml.Event
			merged, events = mxml.Apply(merged, item.Block,
				mxml.ApplyContext{Mod: item.Mod, Source: item.Source})
			for _, e := range events {
				lines = append(lines, e.Line())
			}
		}

		want, err := os.ReadFile(filepath.Join(golden, filepath.FromSlash(index.Targets[i].Merged)))
		require.NoError(t, err)
		require.Equal(t, string(want), strings.Join(merged, "\n"),
			"merged %s differs from the legacy builder's output", index.Targets[i].Internal)
	}

	raw, err = os.ReadFile(filepath.Join(golden, "report_lines.txt"))
	require.NoError(t, err)
	wantLines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	require.Equal(t, len(wantLines), len(lines), "the same number of report lines")
	for i := range wantLines {
		require.Equal(t, wantLines[i], lines[i], "report line %d", i)
	}
}

// verdictRow matches a row of the legacy BUILD_REPORT.md table.
var verdictRow = regexp.MustCompile(`^\| (.+?) \| (WORKING\*|WORKING~|WORKING|PARTIAL|NOT BUILT) \| (\d+) \| (\d+) \|`)

// legacyVerdicts parses the verdict table out of the legacy report.
func legacyVerdicts(t *testing.T, golden string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(golden, "BUILD_REPORT.md"))
	require.NoError(t, err)

	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if m := verdictRow.FindStringSubmatch(line); m != nil {
			out[m[1]] = m[2]
		}
	}
	require.NotEmpty(t, out)
	return out
}

/*
R8.4 -- Stage C: the whole pipeline against the real game.

Everything Stage B cannot cover: the pak index, extraction, decompilation with
the installed compiler, the recompile gate, the degrade-and-drop fallback, and
the verdict each mod ends up with. The comparison is against the verdict table
the legacy builder wrote, because the merged bytes here come from *this*
compiler's decompilation and need not equal the fixture's.

It needs the game and an installed compiler, and it takes a few seconds per run
because it decompiles a hundred files into a scratch cache.
*/
func TestGoldenStageCTheWholePipelineReachesTheLegacyVerdicts(t *testing.T) {
	golden, legacy := goldenDirs(t)
	if os.Getenv("NMSBONKER_GAME_DIR") == "" {
		t.Skip("NMSBONKER_GAME_DIR is unset; skipping the end-to-end stage")
	}
	tools := config.Defaults().Paths().Tools
	if len(mbin.Installed(tools)) == 0 {
		t.Skipf("no MBINCompiler installed in %s; run `nmsbonker tools ensure`", tools)
	}

	root := t.TempDir()
	cfgPath := filepath.Join(root, "config.json")
	writeGoldenConfig(t, cfgPath, root, filepath.Join(legacy, "lua-src"), tools, goldenOrder(t, golden))

	started := time.Now()
	res, err := core.Build(t.Context(), core.BuildRequest{Request: core.Request{ConfigPath: cfgPath}})
	require.NoError(t, err)
	t.Logf("stage C: %d built, %d dropped, %d applied, %d skipped, %s (cold cache)",
		res.Report.Built, res.Report.Dropped, res.Report.Applied, res.Report.Skipped, time.Since(started).Round(time.Millisecond))

	require.Equal(t, core.CompatOK, res.Compatibility.Status, res.Compatibility.Detail)
	require.Equal(t, 0, res.Report.Dropped, "AC2: nothing may be dropped")

	want := legacyVerdicts(t, golden)
	require.Len(t, res.Report.Mods, len(want))
	got := map[string]string{}
	for _, m := range res.Report.Mods {
		got[m.Name] = m.Verdict
	}
	require.Equal(t, want, got)

	// AC2: 100 MBINs, and the output tree holds every one of them plus the
	// GLOBALS/ mirror.
	require.Equal(t, len(want), len(res.Report.Mods))
	built := 0
	for _, target := range res.Report.Targets {
		if target.Output != "" {
			require.FileExists(t, target.Output)
			built++
		}
	}
	require.Equal(t, res.Report.Built, built)
	require.FileExists(t, res.ReportPaths.Markdown)

	loaded, err := report.Load(res.ReportPaths.JSON)
	require.NoError(t, err)
	require.Equal(t, res.Report.Built, loaded.Built)
}

// writeGoldenConfig points a scratch config at the legacy library and the
// installed tools, keeping the cache and the workspace inside the test's own
// directory so a run cannot disturb the user's.
func writeGoldenConfig(t *testing.T, path, root, library, tools string, order []config.ModEntry) {
	t.Helper()
	doc := map[string]any{
		"library_dir":   library,
		"tools_dir":     tools,
		"cache_dir":     filepath.Join(root, "cache"),
		"workspace_dir": filepath.Join(root, "build"),
		"mod_name":      "GOLDEN COMBINE",
		"mods":          order,
	}
	raw, err := json.MarshalIndent(doc, "", " ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}
