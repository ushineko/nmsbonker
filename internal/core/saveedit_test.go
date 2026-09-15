package core_test

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/save"
)

/*
The save editor's guarded write path (spec 007 AC5-AC8).

The fixture is a fake Steam library with a Proton prefix holding one profile,
two synthetic saves built through the real codec, and a fake compiler install
with a mapping beside it. Nothing here reads the user's own saves; the golden
test in internal/save does that, read-only.
*/

// savePayload is a Waypoint-shaped save with the fields the editor edits.
func savePayload(version int, units int64) []byte {
	cells := func(w, n int) string {
		parts := make([]string, 0, n)
		for i := range n {
			parts = append(parts, fmt.Sprintf(`{">Qh":%d,"XJ>":%d}`, i%w, i/w))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	return []byte(`{"F2P":` + fmt.Sprint(version) + `,"8>q":"Win|Final","XTp":"Main",` +
		`"<h0":{"Pk4":"Test save","Lg8":3600},` +
		`"vLc":{"6f=":{"wGS":` + fmt.Sprint(units) + `,"7QL":10,"kN;":0,"ZeS":60,"AIV":100,` +
		`";l5":{":No":[{"b2n":"^OXYGEN","1o9":1,"3ZH":{">Qh":0,"XJ>":0}}],"hl?":` + cells(10, 34) + `,"=Tb":10,"N9>":12},` +
		`"PMT":{":No":[],"hl?":` + cells(10, 13) + `,"=Tb":10,"N9>":6},` +
		`"@Cs":[{}],"OsQ":[{}],"Path":"A\/B"}}}`)
}

const testMappingJSON = `{"libMBIN_version":"7.2.0.1","Mapping":[
{"Key":"F2P","Value":"Version"},{"Key":"8>q","Value":"Platform"},{"Key":"XTp","Value":"ActiveContext"},
{"Key":"vLc","Value":"BaseContext"},{"Key":"2YS","Value":"ExpeditionContext"},{"Key":"<h0","Value":"CommonStateData"},
{"Key":"6f=","Value":"PlayerStateData"},{"Key":"Pk4","Value":"SaveName"},{"Key":"Lg8","Value":"TotalPlayTime"},
{"Key":"wGS","Value":"Units"},{"Key":"7QL","Value":"Nanites"},{"Key":"kN;","Value":"Specials"},
{"Key":"ZeS","Value":"Health"},{"Key":"AIV","Value":"Shield"},{"Key":";l5","Value":"Inventory"},
{"Key":"PMT","Value":"Inventory_TechOnly"},{"Key":"=Tb","Value":"Width"},{"Key":"N9>","Value":"Height"},
{"Key":"hl?","Value":"ValidSlotIndices"},{"Key":":No","Value":"Slots"},{"Key":"3ZH","Value":"Index"},
{"Key":">Qh","Value":"X"},{"Key":"XJ>","Value":"Y"},{"Key":"b2n","Value":"Id"},{"Key":"1o9","Value":"Amount"},
{"Key":"@Cs","Value":"ShipOwnership"},{"Key":"OsQ","Value":"Multitools"}]}`

// writeSaveFixture puts one encoded save and its manifest into a profile,
// with the manifest sizes and timestamp the game would have written.
func writeSaveFixture(t *testing.T, profile string, ref save.SlotRef, payload []byte, when time.Time) {
	t.Helper()
	enc, err := save.Encode(payload)
	require.NoError(t, err)
	plain := make([]byte, 432)
	binary.LittleEndian.PutUint32(plain[0x04:], 2004)
	binary.LittleEndian.PutUint32(plain[0x38:], uint32(len(payload)+1))
	binary.LittleEndian.PutUint32(plain[0x3C:], uint32(len(enc)))
	binary.LittleEndian.PutUint32(plain[0x44:], 4223)
	binary.LittleEndian.PutUint16(plain[0x48:], 1)
	binary.LittleEndian.PutUint64(plain[0x4C:], 3600)
	copy(plain[0x58:], "Test save")
	copy(plain[0xD8:], "On a planet")
	binary.LittleEndian.PutUint32(plain[0x158:], 1)
	binary.LittleEndian.PutUint32(plain[0x164:], uint32(when.Unix()))
	binary.LittleEndian.PutUint32(plain[0x168:], 2004)
	meta, err := save.NewMeta(plain)
	require.NoError(t, err)
	slot, err := save.SlotIndex(ref.DataFile())
	require.NoError(t, err)
	data := filepath.Join(profile, ref.DataFile())
	mf := filepath.Join(profile, ref.MetaFile())
	require.NoError(t, os.WriteFile(data, enc, 0o755)) //nolint:gosec // the game's own mode, which must survive
	require.NoError(t, os.WriteFile(mf, meta.Encode(slot), 0o755))
	require.NoError(t, os.Chtimes(data, when, when))
	require.NoError(t, os.Chtimes(mf, when, when))
}

// editorFixture is a game, a profile with slot 9 auto and manual saves (manual
// newer), an account data file, and a compiler install carrying the mapping.
func editorFixture(t *testing.T) (game, profile string) {
	t.Helper()
	root := bare(t)
	game, lib := steamGame(t, root)
	dir := filepath.Join(lib, "steamapps", "compatdata", "275850",
		"pfx", "drive_c", "users", "steamuser", "AppData", "Roaming", "HelloGames", "NMS")
	profile = filepath.Join(dir, "st_1")
	require.NoError(t, os.MkdirAll(profile, 0o750))
	older := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	newer := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeSaveFixture(t, profile, save.SlotRef{Slot: 9, Kind: save.KindAuto}, savePayload(4735, 1000), older)
	writeSaveFixture(t, profile, save.SlotRef{Slot: 9, Kind: save.KindManual}, savePayload(4735, 2000), newer)
	require.NoError(t, os.WriteFile(filepath.Join(profile, "accountdata.hg"), []byte(`{"F2P":4098}`+"\x00"), 0o600))

	tools := filepath.Join(config.Defaults().Paths().Tools, "mbincompiler", "v7.02.0-pre1")
	require.NoError(t, os.MkdirAll(tools, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(tools, "MBINCompiler-linux-dotnet10"), []byte("#!/bin/sh\n"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(tools, "libMBIN-linux-dotnet10.so"), []byte("x"), 0o640))
	require.NoError(t, os.WriteFile(filepath.Join(tools, "mapping.json"), []byte(testMappingJSON), 0o640))
	return game, profile
}

func fileTimes(t *testing.T, path string) (time.Time, os.FileMode, int64) {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	return fi.ModTime(), fi.Mode().Perm(), fi.Size()
}

// R5.1: the listing reads every manifest and marks the file the game would load.
func TestListSaveSlotsReadsTheManifestsAndMarksTheNewest(t *testing.T) {
	game, profile := editorFixture(t)
	res, err := core.ListSaveSlots(t.Context(), core.ListSaveSlotsRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.Equal(t, profile, res.Profile)
	require.True(t, res.Mapping.Present)
	require.Len(t, res.Slots, 2)
	require.Equal(t, save.KindAuto, res.Slots[0].Kind)
	require.False(t, res.Slots[0].Newest)
	require.True(t, res.Slots[1].Newest, "the manual save was written later")
	require.Equal(t, "Test save", res.Slots[1].Name)
	require.Equal(t, "On a planet", res.Slots[1].Summary)
	require.EqualValues(t, 4223, res.Slots[1].BaseVersion)
	require.Equal(t, "Normal", res.Slots[1].GameMode)
}

// R5.2: inspect resolves "slot 9" to the newer half and reads its values.
func TestInspectSaveChoosesTheNewestHalfWhenNoKindIsGiven(t *testing.T) {
	game, _ := editorFixture(t)
	res, err := core.InspectSave(t.Context(), core.InspectSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9},
	})
	require.NoError(t, err)
	require.Equal(t, save.KindManual, res.Ref.Kind)
	require.EqualValues(t, 2000, res.Summary.Units)
	require.Equal(t, 34, res.Summary.SuitItems.Valid)
	require.Equal(t, "Test save", res.Meta.Name)

	res, err = core.InspectSave(t.Context(), core.InspectSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9, Kind: save.KindAuto},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1000, res.Summary.Units)

	_, err = core.InspectSave(t.Context(), core.InspectSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 3},
	})
	require.ErrorIs(t, err, core.ErrNoSave)
}

/*
AC5: the write takes a backup first, rewrites the pair atomically with the
manifest describing the new file, keeps the modes, sets both times, and touches
nothing else in the profile.
*/
func TestEditSaveBacksUpThenRewritesThePairAndNothingElse(t *testing.T) {
	game, profile := editorFixture(t)
	data := filepath.Join(profile, "save18.hg")
	mf := filepath.Join(profile, "mf_save18.hg")
	account := filepath.Join(profile, "accountdata.hg")
	auto := filepath.Join(profile, "save17.hg")
	before, err := os.ReadFile(data)
	require.NoError(t, err)
	accountBefore, _, _ := fileTimes(t, account)
	autoBefore, _, _ := fileTimes(t, auto)

	units := uint64(123456)
	slots := 120
	res, err := core.EditSave(t.Context(), core.EditSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9},
		Changes: save.ChangeSet{Units: &units, SuitItemSlots: &slots},
	})
	require.NoError(t, err)
	require.Len(t, res.Changes, 2)
	require.NotNil(t, res.Write)
	w := *res.Write

	// The backup holds the untouched original.
	require.DirExists(t, w.Backup)
	backedUp, err := os.ReadFile(filepath.Join(w.Backup, "st_1", "save18.hg"))
	require.NoError(t, err)
	require.Equal(t, before, backedUp)
	require.Equal(t, core.SteamCloudNote, w.Note)

	// The data file re-decodes with the edit and every other byte unchanged.
	after, err := os.ReadFile(data)
	require.NoError(t, err)
	payload, err := save.Decode(after)
	require.NoError(t, err)
	m, err := save.ParseMapping([]byte(testMappingJSON))
	require.NoError(t, err)
	want, err := save.Parse(savePayload(4735, 2000))
	require.NoError(t, err)
	_, err = save.Apply(want, m, save.ChangeSet{Units: &units, SuitItemSlots: &slots})
	require.NoError(t, err)
	require.Equal(t, string(want.Bytes()), string(payload), "the edit and nothing else")

	// The manifest describes the new file, and both files carry the write time
	// and their original modes.
	metaRaw, err := os.ReadFile(mf)
	require.NoError(t, err)
	meta, err := save.DecodeMeta(metaRaw, 19)
	require.NoError(t, err)
	require.EqualValues(t, len(payload)+1, meta.DecompressedSize())
	require.EqualValues(t, len(after), meta.DiskSize())
	require.EqualValues(t, w.Timestamp.Unix(), meta.Timestamp())
	require.Equal(t, "Test save", meta.SaveName(), "carried over")
	dataTime, dataMode, dataSize := fileTimes(t, data)
	mfTime, mfMode, _ := fileTimes(t, mf)
	require.Equal(t, w.Timestamp.Unix(), dataTime.Unix())
	require.Equal(t, w.Timestamp.Unix(), mfTime.Unix())
	require.Equal(t, os.FileMode(0o755), dataMode, "the game's mode is kept")
	require.Equal(t, os.FileMode(0o755), mfMode)
	require.Equal(t, w.Bytes, dataSize)

	// R6.5: the account data and the other half of the slot are untouched.
	accountAfter, _, _ := fileTimes(t, account)
	autoAfter, _, _ := fileTimes(t, auto)
	require.Equal(t, accountBefore, accountAfter)
	require.Equal(t, autoBefore, autoAfter)

	// No temporary file was left behind.
	entries, err := os.ReadDir(profile)
	require.NoError(t, err)
	for _, e := range entries {
		require.False(t, strings.HasPrefix(e.Name(), "."), "leftover %s", e.Name())
	}
}

// AC7: a dry run reports the changes and leaves the profile exactly as it was.
func TestEditSaveDryRunWritesNothing(t *testing.T) {
	game, profile := editorFixture(t)
	snapshot := func() map[string][3]any {
		out := map[string][3]any{}
		entries, err := os.ReadDir(profile)
		require.NoError(t, err)
		for _, e := range entries {
			fi, err := e.Info()
			require.NoError(t, err)
			out[e.Name()] = [3]any{fi.Size(), fi.ModTime(), fi.Mode()}
		}
		return out
	}
	before := snapshot()
	backups := config.Defaults().Paths().SaveBackup

	units := uint64(5)
	res, err := core.EditSave(t.Context(), core.EditSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9},
		Changes: save.ChangeSet{Units: &units}, DryRun: true,
	})
	require.NoError(t, err)
	require.True(t, res.DryRun)
	require.Len(t, res.Changes, 1)
	require.Equal(t, "2000", res.Changes[0].Old)
	require.Nil(t, res.Write)
	require.Equal(t, before, snapshot())
	require.NoDirExists(t, backups, "a dry run takes no backup either")

	// A change set that changes nothing writes nothing, dry run or not.
	same := uint64(2000)
	res, err = core.EditSave(t.Context(), core.EditSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9},
		Changes: save.ChangeSet{Units: &same},
	})
	require.NoError(t, err)
	require.Empty(t, res.Changes)
	require.Nil(t, res.Write)
	require.Equal(t, before, snapshot())
}

// AC6: a running game refuses the write; Force overrides it and still backs up.
func TestEditSaveRefusesWhileTheGameRunsUnlessForced(t *testing.T) {
	game, _ := editorFixture(t)
	proc := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(proc, "4242"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "4242", "comm"), []byte("NMS.exe\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "4242", "cmdline"),
		[]byte("Z:\\games\\No Man's Sky\\Binaries\\NMS.exe\x00"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(proc, "self"), 0o750))
	defer core.SetProcRoot(proc)()

	list, err := core.ListSaveSlots(t.Context(), core.ListSaveSlotsRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.True(t, list.GameRunning)

	units := uint64(7)
	_, err = core.EditSave(t.Context(), core.EditSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9},
		Changes: save.ChangeSet{Units: &units},
	})
	require.ErrorIs(t, err, core.ErrGameRunning)
	require.NoDirExists(t, config.Defaults().Paths().SaveBackup, "a refusal takes no backup")

	res, err := core.EditSave(t.Context(), core.EditSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9},
		Changes: save.ChangeSet{Units: &units}, Force: true,
	})
	require.NoError(t, err)
	require.NotNil(t, res.Write)
	require.True(t, res.Write.Forced)
	require.DirExists(t, res.Write.Backup)
}

// R5.3: export lands outside the prefix and import brings a named export back.
func TestExportAndImportRoundTripThroughANamedFile(t *testing.T) {
	game, profile := editorFixture(t)
	exp, err := core.ExportSave(t.Context(), core.ExportSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9}, Names: true, Pretty: true,
	})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(exp.Out, config.Defaults().Paths().Workspace), "under the workspace by default")
	body, err := os.ReadFile(exp.Out)
	require.NoError(t, err)
	require.Contains(t, string(body), `"Units": 2000`)
	require.Equal(t, 1, exp.Unnamed, "Path is not in the mapping")

	// R5.3: nothing is exported into the prefix, whichever way it is spelled.
	link := filepath.Join(t.TempDir(), "link-into-profile")
	require.NoError(t, os.Symlink(profile, link))
	for _, out := range []string{
		filepath.Join(profile, "dump.json"),
		filepath.Join(profile, "..", "st_1", "dump.json"),
		filepath.Join(link, "dump.json"),
		filepath.Join(link, "new-dir", "dump.json"),
		filepath.Join(game, "GAMEDATA", "dump.json"),
	} {
		_, err = core.ExportSave(t.Context(), core.ExportSaveRequest{
			Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9}, Out: out,
		})
		require.ErrorContains(t, err, "refusing to export", out)
	}
	require.NoFileExists(t, filepath.Join(profile, "dump.json"))

	edited := strings.Replace(string(body), `"Units": 2000`, `"Units": 31337`, 1)
	in := filepath.Join(t.TempDir(), "edited.json")
	require.NoError(t, os.WriteFile(in, []byte(edited), 0o600))
	imp, err := core.ImportSave(t.Context(), core.ImportSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9}, In: in,
	})
	require.NoError(t, err)
	require.Positive(t, imp.Obfuscated)
	require.DirExists(t, imp.Write.Backup)

	after, err := os.ReadFile(filepath.Join(profile, "save18.hg"))
	require.NoError(t, err)
	payload, err := save.Decode(after)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(string(savePayload(4735, 2000)), `"wGS":2000`, `"wGS":31337`, 1), string(payload),
		"the import reproduces the game's own spelling, edit included")

	_, err = core.ImportSave(t.Context(), core.ImportSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9}, In: exp.Out + ".missing",
	})
	require.Error(t, err)

	// R5.5 on import: a pre-Waypoint payload, or a JSON file that is not a
	// save at all, is refused before anything is written.
	backups, err := os.ReadDir(config.Defaults().Paths().SaveBackup)
	require.NoError(t, err)
	for name, body := range map[string][]byte{
		"old.json":     savePayload(4098+512, 1),
		"notsave.json": []byte(`{"hello":"world"}`),
	} {
		path := filepath.Join(t.TempDir(), name)
		require.NoError(t, os.WriteFile(path, body, 0o600))
		_, err = core.ImportSave(t.Context(), core.ImportSaveRequest{
			Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9}, In: path,
		})
		require.ErrorContains(t, err, "not a save this editor writes", name)
	}
	after2, err := os.ReadFile(filepath.Join(profile, "save18.hg"))
	require.NoError(t, err)
	require.Equal(t, after, after2, "nothing was written")
	backupsAfter, err := os.ReadDir(config.Defaults().Paths().SaveBackup)
	require.NoError(t, err)
	require.Len(t, backupsAfter, len(backups), "no backup was taken for a refused import")
}

// R6.1: two writes in quick succession get two backups, and the first still
// holds the untouched originals rather than the first edit's output.
func TestConsecutiveEditsKeepDistinctBackups(t *testing.T) {
	game, profile := editorFixture(t)
	original, err := os.ReadFile(filepath.Join(profile, "save18.hg"))
	require.NoError(t, err)

	var dirs []string
	for _, v := range []uint64{11, 22} {
		units := v
		res, err := core.EditSave(t.Context(), core.EditSaveRequest{
			Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9},
			Changes: save.ChangeSet{Units: &units},
		})
		require.NoError(t, err)
		require.NotNil(t, res.Write)
		dirs = append(dirs, res.Write.Backup)
	}
	require.NotEqual(t, dirs[0], dirs[1])
	first, err := os.ReadFile(filepath.Join(dirs[0], "st_1", "save18.hg"))
	require.NoError(t, err)
	require.Equal(t, original, first, "the first backup is the original")
	second, err := os.ReadFile(filepath.Join(dirs[1], "st_1", "save18.hg"))
	require.NoError(t, err)
	payload, err := save.Decode(second)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"wGS":11`, "the second backup is the state before the second edit")
}

// R4.1/R4.2: without the mapping, the listing still works and says so, and the
// editor refuses with the same message.
func TestTheEditorSaysWhenTheMappingIsMissing(t *testing.T) {
	game, _ := editorFixture(t)
	require.NoError(t, os.Remove(filepath.Join(config.Defaults().Paths().Tools, "mbincompiler", "v7.02.0-pre1", "mapping.json")))

	list, err := core.ListSaveSlots(t.Context(), core.ListSaveSlotsRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.False(t, list.Mapping.Present)
	require.Contains(t, list.Mapping.Warning, "tools ensure")
	require.Len(t, list.Slots, 2, "the manifests need no mapping")

	units := uint64(1)
	_, err = core.EditSave(t.Context(), core.EditSaveRequest{
		Request: core.Request{GameDir: game}, Slot: core.SlotSelector{Slot: 9},
		Changes: save.ChangeSet{Units: &units},
	})
	require.ErrorContains(t, err, "tools ensure")

	tools, err := core.ListTools(t.Context(), core.ListToolsRequest{Request: core.Request{GameDir: game}})
	require.NoError(t, err)
	require.Len(t, tools.Entries, 1)
	require.False(t, tools.Entries[0].Mapping)
}

func TestParseSlotSelector(t *testing.T) {
	sel, err := core.ParseSlotSelector("9")
	require.NoError(t, err)
	require.Equal(t, core.SlotSelector{Slot: 9}, sel)
	sel, err = core.ParseSlotSelector("2:manual")
	require.NoError(t, err)
	require.Equal(t, core.SlotSelector{Slot: 2, Kind: save.KindManual}, sel)
	_, err = core.ParseSlotSelector("99")
	require.ErrorIs(t, err, save.ErrSlot)
}
