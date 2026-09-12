package steam_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/steam"
)

// The real file starts with a UTF-8 BOM, which encoding/xml rejects outright,
// and EnabledVR sits next to Enabled. A reader that used a real XML parser
// would fail on the first, and a sloppy prefix match would read the second as
// the mod's enabled flag. R3.5.
func TestModSettingsAreReadThroughTheBOMAndPastEnabledVR(t *testing.T) {
	const doc = "\ufeff" + `<?xml version="1.0" encoding="utf-8"?>
<Data template="GcModSettings">
	<Property name="DisableAllMods" value="false" />
	<Property name="Data">
		<Property name="Data" value="GcModSettingsInfo" _index="0">
			<Property name="Name" value="COSMOS COMBINE" />
			<Property name="ModPriority" value="0" />
			<Property name="Enabled" value="true" />
			<Property name="EnabledVR" value="false" />
		</Property>
		<Property name="Data" value="GcModSettingsInfo" _index="1">
			<Property name="Name" value="Something Else" />
			<Property name="ModPriority" value="7" />
			<Property name="Enabled" value="false" />
			<Property name="EnabledVR" value="true" />
		</Property>
	</Property>
</Data>`

	got := steam.ParseModSettings(doc)
	require.False(t, got.DisableAllMods)
	require.Len(t, got.Mods, 2)
	require.Equal(t, steam.ModSetting{Name: "COSMOS COMBINE", Enabled: true, ModPriority: 0}, got.Mods[0])
	require.Equal(t, steam.ModSetting{Name: "Something Else", Enabled: false, ModPriority: 7}, got.Mods[1])
}

// DisableAllMods is the one setting that makes every built mod inert, so
// `status` reports it. Missing it would leave a user staring at a correct build
// that the game ignores.
func TestDisableAllModsIsRead(t *testing.T) {
	got := steam.ParseModSettings(`<Property name="DisableAllMods" value="true" />`)
	require.True(t, got.DisableAllMods)
	require.Empty(t, got.Mods)
}

// A game that has never loaded a mod has no settings file. That is "no mods
// configured", not a failure of the command that asked.
func TestAMissingSettingsFileReadsAsNoMods(t *testing.T) {
	got, err := steam.ReadModSettings("/nonexistent/GCMODSETTINGS.MXML")
	require.NoError(t, err)
	require.False(t, got.DisableAllMods)
	require.Empty(t, got.Mods)
}
