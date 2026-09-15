package core_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/save"
)

/*
Spec 007 AC5 on a copy of a real profile.

NMSBONKER_SAVE_DIR names an st_* directory and NMSBONKER_MAPPING_FILE a
mapping.json. The profile is copied into a fake Steam layout in a temporary
directory -- the user's own files are only read -- and the editor's write path
runs against the copy. Skips when either variable is unset; no save is
committed (public-repository rule).
*/
func TestGoldenEditSaveOnACopyOfARealProfile(t *testing.T) {
	src := os.Getenv("NMSBONKER_SAVE_DIR")
	mappingFile := os.Getenv("NMSBONKER_MAPPING_FILE")
	if src == "" || mappingFile == "" {
		t.Skip("NMSBONKER_SAVE_DIR or NMSBONKER_MAPPING_FILE is unset; skipping the golden edit test")
	}
	root := bare(t)
	t.Cleanup(core.SetProcRoot(t.TempDir()))
	game, lib := steamGame(t, root)
	dir := filepath.Join(lib, "steamapps", "compatdata", "275850",
		"pfx", "drive_c", "users", "steamuser", "AppData", "Roaming", "HelloGames", "NMS")
	profile := filepath.Join(dir, filepath.Base(src))
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, copyDir(src, profile))

	tools := filepath.Join(config.Defaults().Paths().Tools, "mbincompiler", "v7.02.0-pre1")
	require.NoError(t, os.MkdirAll(tools, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(tools, "MBINCompiler-linux-dotnet10"), []byte("#!/bin/sh\n"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(tools, "libMBIN-linux-dotnet10.so"), []byte("x"), 0o640))
	mapping, err := os.ReadFile(mappingFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tools, "mapping.json"), mapping, 0o640))
	m, err := save.ParseMapping(mapping)
	require.NoError(t, err)

	req := core.Request{GameDir: game}
	list, err := core.ListSaveSlots(t.Context(), core.ListSaveSlotsRequest{Request: req})
	require.NoError(t, err)
	var newest core.SaveSlotInfo
	for _, sl := range list.Slots {
		if sl.Newest {
			newest = sl
		}
	}
	require.NotZero(t, newest.Slot, "a newest save was marked")
	sel := core.SlotSelector{Slot: newest.Slot, Kind: newest.Kind}

	before, err := os.ReadFile(newest.File)
	require.NoError(t, err)
	beforeInfo, err := os.Stat(newest.File)
	require.NoError(t, err)
	account := filepath.Join(profile, "accountdata.hg")
	accountBefore, err := os.Stat(account)
	require.NoError(t, err)

	insp, err := core.InspectSave(t.Context(), core.InspectSaveRequest{Request: req, Slot: sel})
	require.NoError(t, err)
	units := uint64(insp.Summary.Units) + 1 //nolint:gosec // a real save's units are positive
	full := insp.Summary.SuitItems.Max()

	// First a units-only edit, checked against an oracle that shares nothing
	// with the editor: the original payload with one literal substring
	// replaced. The key comes from the mapping, the old value from inspect.
	unitsKey, ok := m.Key("Units")
	require.True(t, ok)
	originalPayload := mustDecode(t, before)
	needle := fmt.Sprintf(`"%s":%d,`, unitsKey, insp.Summary.Units)
	require.Equal(t, 1, strings.Count(string(originalPayload), needle), "the units field is found exactly once")
	independent := strings.Replace(string(originalPayload), needle, fmt.Sprintf(`"%s":%d,`, unitsKey, units), 1)
	metaBefore, err := os.Stat(filepath.Join(profile, save.SlotRef{Slot: newest.Slot, Kind: newest.Kind}.MetaFile()))
	require.NoError(t, err)

	first, err := core.EditSave(t.Context(), core.EditSaveRequest{
		Request: req, Slot: sel, Changes: save.ChangeSet{Units: &units},
	})
	require.NoError(t, err)
	require.NotNil(t, first.Write)
	afterUnits, err := os.ReadFile(newest.File)
	require.NoError(t, err)
	require.Equal(t, independent, string(mustDecode(t, afterUnits)), "units changed and no other byte")
	metaAfter, err := os.Stat(filepath.Join(profile, save.SlotRef{Slot: newest.Slot, Kind: newest.Kind}.MetaFile()))
	require.NoError(t, err)
	require.Equal(t, metaBefore.Mode().Perm(), metaAfter.Mode().Perm(), "the manifest keeps its mode")
	require.Equal(t, first.Write.Timestamp.Unix(), metaAfter.ModTime().Unix(), "the manifest carries the write time")

	// Then the slot unlock on top, checked through the editor's own tree.
	res, err := core.EditSave(t.Context(), core.EditSaveRequest{
		Request: req, Slot: sel, Changes: save.ChangeSet{SuitItemSlots: &full},
	})
	require.NoError(t, err)
	require.NotNil(t, res.Write)
	w := *res.Write
	require.NotEqual(t, first.Write.Backup, w.Backup, "each write has its own backup")

	// The first backup holds the original bytes; the second, the state before
	// the second write.
	backedUp, err := os.ReadFile(filepath.Join(first.Write.Backup, filepath.Base(src), filepath.Base(newest.File)))
	require.NoError(t, err)
	require.Equal(t, before, backedUp)
	backedUp2, err := os.ReadFile(filepath.Join(w.Backup, filepath.Base(src), filepath.Base(newest.File)))
	require.NoError(t, err)
	require.Equal(t, afterUnits, backedUp2)

	// The written payload is the original with exactly these edits applied.
	want, err := save.Parse(mustDecode(t, before))
	require.NoError(t, err)
	_, err = save.Apply(want, m, save.ChangeSet{Units: &units, SuitItemSlots: &full})
	require.NoError(t, err)
	after, err := os.ReadFile(newest.File)
	require.NoError(t, err)
	require.Equal(t, want.Bytes(), mustDecode(t, after), "the edit and every other byte as the game wrote it")

	// The manifest matches the new file; modes and times are right.
	ref := save.SlotRef{Slot: newest.Slot, Kind: newest.Kind}
	slot, err := save.SlotIndex(ref.DataFile())
	require.NoError(t, err)
	metaRaw, err := os.ReadFile(filepath.Join(profile, ref.MetaFile()))
	require.NoError(t, err)
	meta, err := save.DecodeMeta(metaRaw, slot)
	require.NoError(t, err)
	require.EqualValues(t, len(want.Bytes())+1, meta.DecompressedSize())
	require.EqualValues(t, len(after), meta.DiskSize())
	require.EqualValues(t, w.Timestamp.Unix(), meta.Timestamp())
	require.Equal(t, newest.Name, meta.SaveName())
	require.Equal(t, newest.Summary, meta.SaveSummary())
	afterInfo, err := os.Stat(newest.File)
	require.NoError(t, err)
	require.Equal(t, beforeInfo.Mode().Perm(), afterInfo.Mode().Perm())
	require.Equal(t, w.Timestamp.Unix(), afterInfo.ModTime().Unix())
	accountAfter, err := os.Stat(account)
	require.NoError(t, err)
	require.Equal(t, accountBefore.ModTime(), accountAfter.ModTime(), "accountdata.hg untouched")

	// The edited save inspects with the new values.
	again, err := core.InspectSave(t.Context(), core.InspectSaveRequest{Request: req, Slot: sel})
	require.NoError(t, err)
	require.EqualValues(t, units, again.Summary.Units)
	require.Equal(t, full, again.Summary.SuitItems.Valid)
	t.Logf("edited %s: units %d -> %d, item slots %d -> %d, %d bytes -> %d",
		filepath.Base(newest.File), insp.Summary.Units, units, insp.Summary.SuitItems.Valid, full, len(before), len(after))
}

func mustDecode(t *testing.T, raw []byte) []byte {
	t.Helper()
	payload, err := save.Decode(raw)
	require.NoError(t, err)
	return payload
}

// copyDir copies the regular files of one directory level, keeping modes and
// times, which is all a save profile needs (the cache folder is skipped).
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		target := filepath.Join(dst, e.Name())
		if err := os.WriteFile(target, b, fi.Mode().Perm()); err != nil {
			return err
		}
		if err := os.Chtimes(target, fi.ModTime(), fi.ModTime()); err != nil {
			return err
		}
	}
	return nil
}
