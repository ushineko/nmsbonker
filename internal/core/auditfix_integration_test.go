package core_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/core"
)

/*
Spec 017 AC: the fix, applied, makes the next build clean.

The planner's arithmetic is unit-tested on hand-written chains; only a real
build can say whether the factors the audit attributes are the ones a lowered
parameter actually changes. So this builds the compounding set a user really
had (loot x10 under mission-board items x15, currency x6 under mission-board
units x100), applies the fix, rebuilds, and requires the audit to come back
empty. Game and compiler are the user's; everything else is throwaway.
*/
func TestTheAuditFixMakesTheNextBuildClean(t *testing.T) {
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
		"mod_name":      "FIX",
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(cfg["library_dir"], 0o700))
	require.NoError(t, os.WriteFile(cfgPath, raw, 0o600))
	req := core.Request{ConfigPath: cfgPath}
	ctx := t.Context()

	_, err = core.ListMods(ctx, core.ListModsRequest{Request: req})
	require.NoError(t, err)
	_, err = core.SetModEnabled(ctx, core.SetModEnabledRequest{
		Request: req, Enabled: true,
		Names: []string{"ChestAndLootMaterials10x", "MoneyAndNanites5x", "MissionBoardRewards"},
	})
	require.NoError(t, err)
	for _, p := range []struct {
		mod, param string
		value      float64
	}{
		{"MoneyAndNanites5x", "CUR_MULT", 6},
		{"MoneyAndNanites5x", "UNITS_CAP", 500000000},
		{"MissionBoardRewards", "ITEM_MULT", 15},
		{"MissionBoardRewards", "UNITS_MULT", 100},
		{"MissionBoardRewards", "UNITS_CAP", 500000000},
	} {
		_, err = core.SetTweakParam(ctx, core.SetTweakParamRequest{
			Request: req, Name: p.mod, Param: p.param, Value: p.value,
		})
		require.NoError(t, err)
	}

	first, err := core.Build(ctx, core.BuildRequest{Request: req})
	require.NoError(t, err)
	require.NotNil(t, first.Report.Audit)
	require.NotEmpty(t, first.Report.Audit.Flags, "the compounding set is flagged to begin with")

	fix, err := core.AuditFix(ctx, core.AuditFixRequest{Request: req, Apply: true})
	require.NoError(t, err)
	require.True(t, fix.Applied)
	require.NotEmpty(t, fix.Plan.Changes)
	require.Zero(t, fix.Plan.Remaining, "every flag here comes from a built-in")

	_, err = core.AuditFix(ctx, core.AuditFixRequest{Request: req})
	require.ErrorIs(t, err, core.ErrParamsChangedSinceBuild,
		"after applying, the plan refuses to reason about the old build")

	second, err := core.Build(ctx, core.BuildRequest{Request: req})
	require.NoError(t, err)
	require.NotNil(t, second.Report.Audit)
	require.Empty(t, second.Report.Audit.Flags, "the fix brought every amount under the limits")
}
