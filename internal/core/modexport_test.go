package core_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/modscript"
)

// Spec 022: an export is the script the build would load, so a parameter set in
// nmsbonker is the parameter's value in the file, and the file loads on its own.
func TestAnExportCarriesTheParameterValuesAndLoads(t *testing.T) {
	root := bare(t)
	_, err := core.SetTweakParam(t.Context(), core.SetTweakParamRequest{
		Name: "ChestAndLootMaterials10x", Param: "LOOT_MULTIPLIER", Value: 7,
	})
	require.NoError(t, err)

	dir := filepath.Join(root, "out")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	res, err := core.ExportMod(t.Context(), core.ExportModRequest{Name: "ChestAndLootMaterials10x", Out: dir})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "ChestAndLootMaterials10x.lua"), res.Path)

	text, err := os.ReadFile(res.Path)
	require.NoError(t, err)
	params := modscript.Parameters(text)
	got := map[string]float64{}
	for _, p := range params {
		got[p.Name] = p.Default
	}
	require.InDelta(t, 7, got["LOOT_MULTIPLIER"], 0, "the script's own value is now the one set")
	require.Contains(t, string(text), "-- Parameters as set in nmsbonker: LOOT_MULTIPLIER = 7.")

	_, err = modscript.LoadSource(t.Context(), res.Path, text)
	require.NoError(t, err, "an export loads without nmsbonker's settings behind it")

	// The description an imported copy shows is still the script's own: the
	// export note goes at the end, not the start.
	require.Equal(t, modscript.ParseHeader(text).Desc, modscript.ParseHeader(stripTrailer(text)).Desc)
}

func TestAnExportNeverOverwritesUnlessForced(t *testing.T) {
	root := bare(t)
	out := filepath.Join(root, "FleetNoDamage.lua")
	_, err := core.ExportMod(t.Context(), core.ExportModRequest{Name: "FleetNoDamage", Out: out})
	require.NoError(t, err)

	_, err = core.ExportMod(t.Context(), core.ExportModRequest{Name: "FleetNoDamage", Out: out})
	require.ErrorIs(t, err, core.ErrExportExists)

	_, err = core.ExportMod(t.Context(), core.ExportModRequest{Name: "FleetNoDamage", Out: out, Force: true})
	require.NoError(t, err)
}

// The note names what another toolchain will not understand, and says nothing
// about a script that uses only AMUMSS keys.
func TestAnExportNamesTheKeysOnlyNmsbonkerUnderstands(t *testing.T) {
	root := bare(t)
	res, err := core.ExportMod(t.Context(), core.ExportModRequest{Name: "MoneyAndNanites5x", Out: root})
	require.NoError(t, err)
	require.Contains(t, res.NmsbonkerOnly, "CAP")

	res, err = core.ExportMod(t.Context(), core.ExportModRequest{Name: "FleetNoDamage", Out: root})
	require.NoError(t, err)
	require.Empty(t, res.NmsbonkerOnly)
	text, err := os.ReadFile(res.Path)
	require.NoError(t, err)
	require.NotContains(t, string(text), "only nmsbonker understands")
}

func stripTrailer(text []byte) []byte {
	s := string(text)
	if i := strings.Index(s, "\n-- Exported by nmsbonker"); i >= 0 {
		s = s[:i+1]
	}
	return []byte(s)
}

/*
Spec 022 AC: an export is the working script. Exported with an override, put
in the library under another name and built in place of the built-in, it
produces the same merged table the built-in did. Game and compiler are the
user's; everything else is throwaway.
*/
func TestAnExportBuildsWhatTheBuiltInBuilt(t *testing.T) {
	withRealGame(t)
	tools := realTools(t)

	merged := func(t *testing.T, setup func(req core.Request, library string)) []byte {
		t.Helper()
		root := t.TempDir()
		cfgPath := filepath.Join(root, "config.json")
		library := filepath.Join(root, "library")
		require.NoError(t, os.MkdirAll(library, 0o700))
		raw := `{"game_dir":` + quote(os.Getenv("NMSBONKER_GAME_DIR")) +
			`,"library_dir":` + quote(library) +
			`,"workspace_dir":` + quote(filepath.Join(root, "build")) +
			`,"cache_dir":` + quote(filepath.Join(root, "cache")) +
			`,"tools_dir":` + quote(tools) + `,"mod_name":"RT"}`
		require.NoError(t, os.WriteFile(cfgPath, []byte(raw), 0o600))
		req := core.Request{ConfigPath: cfgPath}
		_, err := core.ListMods(t.Context(), core.ListModsRequest{Request: req})
		require.NoError(t, err)
		_, err = core.SetTweakParam(t.Context(), core.SetTweakParamRequest{
			Request: req, Name: "ChestAndLootMaterials10x", Param: "LOOT_MULTIPLIER", Value: 7,
		})
		require.NoError(t, err)
		setup(req, library)
		_, err = core.Build(t.Context(), core.BuildRequest{Request: req})
		require.NoError(t, err)
		out, err := os.ReadFile(filepath.Join(root, "build", "RT.work", "mxml",
			"METADATA", "REALITY", "TABLES", "REWARDTABLE.MXML"))
		require.NoError(t, err)
		return out
	}

	builtin := merged(t, func(req core.Request, _ string) {
		_, err := core.SetModEnabled(t.Context(), core.SetModEnabledRequest{
			Request: req, Names: []string{"ChestAndLootMaterials10x"}, Enabled: true,
		})
		require.NoError(t, err)
	})
	exported := merged(t, func(req core.Request, library string) {
		_, err := core.ExportMod(t.Context(), core.ExportModRequest{
			Request: req, Name: "ChestAndLootMaterials10x", Out: filepath.Join(library, "ExportedChest.lua"),
		})
		require.NoError(t, err)
		_, err = core.ListMods(t.Context(), core.ListModsRequest{Request: req})
		require.NoError(t, err)
		_, err = core.SetModEnabled(t.Context(), core.SetModEnabledRequest{
			Request: req, Names: []string{"ExportedChest"}, Enabled: true,
		})
		require.NoError(t, err)
	})
	require.Equal(t, string(builtin), string(exported))
}

func quote(s string) string { return strconv.Quote(s) }
