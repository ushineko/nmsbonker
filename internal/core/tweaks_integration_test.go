package core_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/build/report"
	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/tweaks"
)

/*
Spec 014 R1.2: every built-in, built against the installed game, finds every
key it asks for.

The unit tests prove a script loads and declares what it edits. Only the game
can say whether those keys still exist: the scanner's range and recharge left
GCGAMEPLAYGLOBALS for SCANDATATABLE in an update, and a script written against
the old layout loads, plans and reports OK on the keys it still finds. So this
builds every built-in with the real compiler and fails on any row that is not
WORKING (or WORKING*, for one that adds entries) with nothing skipped.

Library, workspace and cache are throwaway; only the game and the compiler
the user already installed are real.
*/
func TestEveryBuiltInFindsEveryKeyInTheInstalledGame(t *testing.T) {
	withRealGame(t)
	tools := realTools(t)

	root := t.TempDir()
	cfgPath := filepath.Join(root, "config.json")
	cfg := map[string]string{
		"game_dir":      os.Getenv("NMSBONKER_GAME_DIR"),
		"library_dir":   filepath.Join(root, "library"),
		"workspace_dir": filepath.Join(root, "build"),
		"cache_dir":     filepath.Join(root, "cache"),
		"tools_dir":     tools,
		"mod_name":      "TWEAKS",
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(cfg["library_dir"], 0o700))
	require.NoError(t, os.WriteFile(cfgPath, raw, 0o600))
	req := core.Request{ConfigPath: cfgPath}

	_, err = core.ListMods(t.Context(), core.ListModsRequest{Request: req})
	require.NoError(t, err)
	_, err = core.SetModEnabled(t.Context(), core.SetModEnabledRequest{
		Request: req, Names: tweaks.Names(), Enabled: true,
	})
	require.NoError(t, err)

	res, err := core.Build(t.Context(), core.BuildRequest{Request: req})
	require.NoError(t, err)

	rows := map[string]report.ModResult{}
	for _, m := range res.Report.Mods {
		rows[m.Name] = m
	}
	for _, name := range tweaks.Names() {
		m, ok := rows[name]
		require.Truef(t, ok, "%s has no row in the report", name)
		require.Positivef(t, m.Applied, "%s applied nothing", name)
		// MaterialYield10x reaches into entity files some of which hold no
		// amounts; that is spec 004's known WORKING~, not a regression here.
		if name == "MaterialYield10x" {
			continue
		}
		// Spec 015 retired 014 R1.3, so a built-in may add entries. That
		// one reports WORKING*, and a dropped ADD would make it PARTIAL.
		require.Containsf(t, []string{report.Working, report.WorkingStructural}, m.Verdict,
			"%s: keys not found %v", name, m.NotFound)
		require.Zerof(t, m.Skipped, "%s skipped edits", name)
	}
	require.Zero(t, res.Report.Dropped, "a built-in's file failed to recompile")
	require.Empty(t, res.Report.Overlaps, "built-ins never overlap each other")
}
