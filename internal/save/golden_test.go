package save_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/save"
)

/*
Golden fidelity against real saves (AC3).

NMSBONKER_SAVE_DIR names an st_* profile directory; every save data file in it
must decode, parse and serialise back to the same bytes, and its manifest must
decrypt to sizes that match the file. NMSBONKER_MAPPING_FILE, when set, names a
mapping.json and the summary must read from every save.

No save is committed (public-repository rule): the test skips when the variable
is unset.
*/
func TestGoldenRealSavesRoundTripByteForByte(t *testing.T) {
	dir := os.Getenv("NMSBONKER_SAVE_DIR")
	if dir == "" {
		t.Skip("NMSBONKER_SAVE_DIR is unset; skipping the golden save test")
	}
	var m *save.Mapping
	if path := os.Getenv("NMSBONKER_MAPPING_FILE"); path != "" {
		var err error
		m, err = save.LoadMapping(path)
		require.NoError(t, err)
	}

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	checked := 0
	for _, e := range entries {
		ref, ok := save.ParseFileName(e.Name())
		if !ok {
			continue
		}
		checked++
		t.Run(e.Name(), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			require.NoError(t, err)
			payload, err := save.Decode(raw)
			require.NoError(t, err)
			root, err := save.Parse(payload)
			require.NoError(t, err)
			require.Equal(t, payload, root.Bytes(), "R3.1 on a real save")

			metaRaw, err := os.ReadFile(filepath.Join(dir, ref.MetaFile()))
			require.NoError(t, err)
			slot, err := save.SlotIndex(e.Name())
			require.NoError(t, err)
			meta, err := save.DecodeMeta(metaRaw, slot)
			require.NoError(t, err)
			require.EqualValues(t, len(payload)+1, meta.DecompressedSize(), "payload plus NUL")
			if meta.DiskSize() != 0 {
				require.EqualValues(t, len(raw), meta.DiskSize())
			}
			if meta.HasTimestamp() && meta.Timestamp() != 0 {
				// The game writes the manifest timestamp and the file mtime
				// together, and the newest pair on the recon machine agreed
				// to the second. Older files did not: a Steam Cloud download
				// gives a file the download's mtime. Logged, not asserted.
				fi, err := e.Info()
				require.NoError(t, err)
				t.Logf("%s: manifest timestamp %d, file mtime %d (delta %ds)",
					e.Name(), meta.Timestamp(), fi.ModTime().Unix(), fi.ModTime().Unix()-int64(meta.Timestamp()))
			}
			require.Equal(t, metaRaw, meta.Encode(slot), "AC2 on a real manifest")

			if m != nil {
				info, err := save.Version(root, m)
				require.NoError(t, err)
				t.Logf("%s: version %d (base %d, %s), %d unmapped key(s)",
					e.Name(), info.Version, info.Base, save.GameModeName(info.GameMode), len(m.Unmapped(root)))
				if info.Base >= save.MinBaseVersion {
					s, err := save.Summarize(root, m)
					require.NoError(t, err)
					require.Positive(t, s.SuitItems.Max())
				}
			}
		})
	}
	require.Positive(t, checked, "the directory held no save files")
}
