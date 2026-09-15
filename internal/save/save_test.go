package save_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/save"
)

// testMapping is a mapping with the game's real keys for the names this package
// addresses, so a synthetic payload can look like a real one.
func testMapping(t *testing.T) *save.Mapping {
	t.Helper()
	pairs := [][2]string{
		{"F2P", "Version"}, {"8>q", "Platform"}, {"XTp", "ActiveContext"},
		{"vLc", "BaseContext"}, {"2YS", "ExpeditionContext"}, {"<h0", "CommonStateData"},
		{"6f=", "PlayerStateData"}, {"Pk4", "SaveName"}, {"Lg8", "TotalPlayTime"},
		{"wGS", "Units"}, {"7QL", "Nanites"}, {"kN;", "Specials"},
		{"ZeS", "Health"}, {"AIV", "Shield"},
		{";l5", "Inventory"}, {"PMT", "Inventory_TechOnly"},
		{"=Tb", "Width"}, {"N9>", "Height"}, {"hl?", "ValidSlotIndices"}, {":No", "Slots"},
		{"3ZH", "Index"}, {">Qh", "X"}, {"XJ>", "Y"}, {"b2n", "Id"}, {"1o9", "Amount"},
		{"@Cs", "ShipOwnership"}, {"OsQ", "Multitools"},
		// The real file's ambiguity, reproduced: one key, two names.
		{"V86", "RocketLockerInventory"}, {"V86", "FireteamSessionCount"},
	}
	var sb strings.Builder
	sb.WriteString(`{"libMBIN_version":"7.2.0.1","Mapping":[`)
	for i, p := range pairs {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"Key":%q,"Value":%q}`, p[0], p[1])
	}
	sb.WriteString(`]}`)
	m, err := save.ParseMapping([]byte(sb.String()))
	require.NoError(t, err)
	return m
}

// cells renders n row-major {X,Y} cells of a w-wide grid.
func cells(w, n int) string {
	parts := make([]string, 0, n)
	for i := range n {
		parts = append(parts, fmt.Sprintf(`{">Qh":%d,"XJ>":%d}`, i%w, i/w))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// payload is a Waypoint-shaped save with the fields the editor touches: a 10×12
// item grid with 34 cells unlocked and two items in it, a 10×6 tech grid with
// 13 unlocked.
func payload(version int) []byte {
	return []byte(`{"F2P":` + fmt.Sprint(version) + `,"8>q":"Win|Final","XTp":"Main",` +
		`"<h0":{"Pk4":"","Lg8":17088},` +
		`"vLc":{"6f=":{"wGS":162810044,"7QL":36793,"kN;":0,"ZeS":60,"AIV":100,` +
		`";l5":{":No":[{"b2n":"^OXYGEN","1o9":1287,"3ZH":{">Qh":0,"XJ>":0}},` +
		`{"b2n":"^CARBON","1o9":9999,"3ZH":{">Qh":3,"XJ>":3}}],` +
		`"hl?":` + cells(10, 34) + `,"=Tb":10,"N9>":12},` +
		`"PMT":{":No":[],"hl?":` + cells(10, 13) + `,"=Tb":10,"N9>":6},` +
		`"@Cs":[{},{},{}],"OsQ":[{}],"Path":"MODELS\/COMMON\/X.MBIN","F":1.0,"Neg":-0.0,"E":1e5}}}`)
}

// --- container (AC1) --------------------------------------------------------

func TestContainerRoundTripsAndSplitsAtTheChunkSize(t *testing.T) {
	small := payload(4735)
	enc, err := save.Encode(small)
	require.NoError(t, err)
	require.Equal(t, uint32(0xFEEDA1E5), binary.LittleEndian.Uint32(enc), "the magic opens the file")
	dec, err := save.Decode(enc)
	require.NoError(t, err)
	require.Equal(t, small, dec)

	// Larger than one chunk: several chunks, every header honest.
	big := bytes.Repeat([]byte(`{"k":"vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv"},`), save.ChunkMax/50+100)
	enc, err = save.Encode(big)
	require.NoError(t, err)
	chunks, total := 0, 0
	for off := 0; off < len(enc); chunks++ {
		require.Equal(t, uint32(0xFEEDA1E5), binary.LittleEndian.Uint32(enc[off:]))
		csize := int(binary.LittleEndian.Uint32(enc[off+4:]))
		dsize := int(binary.LittleEndian.Uint32(enc[off+8:]))
		require.Zero(t, binary.LittleEndian.Uint32(enc[off+12:]), "the fourth word is zero")
		require.LessOrEqual(t, dsize, save.ChunkMax)
		if off+16+csize < len(enc) {
			require.Equal(t, save.ChunkMax, dsize, "every chunk but the last is full")
		}
		total += dsize
		off += 16 + csize
	}
	require.Greater(t, chunks, 1)
	require.Equal(t, len(big)+1, total, "the NUL is inside the last chunk")
	dec, err = save.Decode(enc)
	require.NoError(t, err)
	require.Equal(t, big, dec)
}

func TestContainerRejectsALyingHeaderWithTheChunkIndex(t *testing.T) {
	enc, err := save.Encode(payload(4735))
	require.NoError(t, err)

	bad := append([]byte(nil), enc...)
	bad[0] = 0
	_, err = save.Decode(bad)
	require.ErrorContains(t, err, "chunk 0")
	require.ErrorContains(t, err, "magic")

	bad = append([]byte(nil), enc...)
	binary.LittleEndian.PutUint32(bad[8:], binary.LittleEndian.Uint32(bad[8:])+7)
	_, err = save.Decode(bad)
	require.ErrorContains(t, err, "chunk 0")

	_, err = save.Decode(nil)
	require.ErrorIs(t, err, save.ErrEmpty)
}

func TestContainerPassesAPlainPayloadThroughWithoutItsTerminator(t *testing.T) {
	dec, err := save.Decode([]byte(`{"F2P":4098}` + "\x00\x00"))
	require.NoError(t, err)
	require.Equal(t, `{"F2P":4098}`, string(dec))
}

func TestContainerSpellsAnIncompressibleChunk(t *testing.T) {
	// Seven bytes cannot be matched against anything; the literal-only block
	// must still decode.
	tiny := []byte(`{"a":1}`)
	enc, err := save.Encode(tiny)
	require.NoError(t, err)
	dec, err := save.Decode(enc)
	require.NoError(t, err)
	require.Equal(t, tiny, dec)
}

// --- manifest (AC2) ---------------------------------------------------------

func synthMeta(t *testing.T) *save.Meta {
	t.Helper()
	plain := make([]byte, 432)
	binary.LittleEndian.PutUint32(plain[0x04:], 2004)
	binary.LittleEndian.PutUint32(plain[0x38:], 1966855)
	binary.LittleEndian.PutUint32(plain[0x3C:], 114719)
	binary.LittleEndian.PutUint32(plain[0x44:], 4223)
	binary.LittleEndian.PutUint16(plain[0x48:], 1)
	binary.LittleEndian.PutUint64(plain[0x4C:], 17088)
	copy(plain[0x58:], "My save")
	copy(plain[0xD8:], "On freighter (Test)")
	binary.LittleEndian.PutUint32(plain[0x158:], 1)
	binary.LittleEndian.PutUint32(plain[0x164:], 1789449910)
	binary.LittleEndian.PutUint32(plain[0x168:], 2004)
	// An unknown tail, which must survive untouched.
	for i := 0x16C; i < len(plain); i++ {
		plain[i] = byte(i)
	}
	m, err := save.NewMeta(plain)
	require.NoError(t, err)
	return m
}

func TestManifestRoundTripsAndReproducesItsCiphertext(t *testing.T) {
	m := synthMeta(t)
	const slot = 19 // save18.hg
	enc := m.Encode(slot)
	require.NotEqual(t, m.Plain(), enc, "it is actually encrypted")

	dec, err := save.DecodeMeta(enc, slot)
	require.NoError(t, err)
	require.Equal(t, uint32(2004), dec.Format())
	require.Equal(t, uint32(1966855), dec.DecompressedSize())
	require.Equal(t, uint32(114719), dec.DiskSize())
	require.Equal(t, uint32(4223), dec.BaseVersion())
	require.Equal(t, uint16(1), dec.GameMode())
	require.Equal(t, uint64(17088), dec.PlayTime())
	require.Equal(t, "My save", dec.SaveName())
	require.Equal(t, "On freighter (Test)", dec.SaveSummary())
	require.Equal(t, uint32(1), dec.Difficulty())
	require.Equal(t, uint32(1789449910), dec.Timestamp())
	require.Equal(t, m.Plain(), dec.Plain(), "the tail came through")

	require.Equal(t, enc, dec.Encode(slot), "AC2: an unchanged manifest re-encodes byte for byte")

	_, err = save.DecodeMeta(enc, slot+1)
	require.ErrorIs(t, err, save.ErrMetaHeader, "the wrong slot does not decrypt")

	dec.SetSizes(5, 6)
	dec.SetTimestamp(7)
	again, err := save.DecodeMeta(dec.Encode(slot), slot)
	require.NoError(t, err)
	require.Equal(t, uint32(5), again.DecompressedSize())
	require.Equal(t, uint32(6), again.DiskSize())
	require.Equal(t, uint32(7), again.Timestamp())
	require.Equal(t, "My save", again.SaveName(), "everything else is as it was")
}

func TestManifestUsesEightRoundsForTheOriginalLayout(t *testing.T) {
	plain := make([]byte, 104)
	binary.LittleEndian.PutUint32(plain[0x44:], 4098)
	m, err := save.NewMeta(plain)
	require.NoError(t, err)
	dec, err := save.DecodeMeta(m.Encode(2), 2)
	require.NoError(t, err)
	require.Equal(t, uint32(4098), dec.BaseVersion())
	require.Empty(t, dec.SaveName(), "a layout without names has none")
	require.False(t, dec.HasTimestamp())
}

func TestSlotNaming(t *testing.T) {
	for _, tc := range []struct {
		file  string
		slot  int
		kind  save.Kind
		index uint32
	}{
		{"save.hg", 1, save.KindAuto, 2},
		{"save2.hg", 1, save.KindManual, 3},
		{"save17.hg", 9, save.KindAuto, 18},
		{"save18.hg", 9, save.KindManual, 19},
		{"save30.hg", 15, save.KindManual, 31},
	} {
		ref, ok := save.ParseFileName(tc.file)
		require.True(t, ok, tc.file)
		require.Equal(t, save.SlotRef{Slot: tc.slot, Kind: tc.kind}, ref)
		require.Equal(t, tc.file, ref.DataFile())
		require.Equal(t, "mf_"+tc.file, ref.MetaFile())
		idx, err := save.SlotIndex(tc.file)
		require.NoError(t, err)
		require.Equal(t, tc.index, idx)
	}
	idx, err := save.SlotIndex("accountdata.hg")
	require.NoError(t, err)
	require.Equal(t, uint32(1), idx)
	for _, bad := range []string{"mf_save.hg", "save31.hg", "save0.hg", "cache", "save1.hg"} {
		_, ok := save.ParseFileName(bad)
		require.False(t, ok, bad)
	}

	ref, given, err := save.ParseSlotRef("9")
	require.NoError(t, err)
	require.False(t, given)
	require.Equal(t, 9, ref.Slot)
	ref, given, err = save.ParseSlotRef("9:manual")
	require.NoError(t, err)
	require.True(t, given)
	require.Equal(t, save.KindManual, ref.Kind)
	for _, bad := range []string{"0", "16", "x", "9:sometimes"} {
		_, _, err = save.ParseSlotRef(bad)
		require.ErrorIs(t, err, save.ErrSlot, bad)
	}
}

// --- document (R3) ----------------------------------------------------------

func TestDocumentGivesBackTheBytesItWasGiven(t *testing.T) {
	// Escapes, escaped slashes, a raw non-UTF-8 technology id, float spellings.
	src := []byte(`{"a":"x\"y\\z\/wé","b":"^` + "\xff\x01\xfe" + `#","c":[1.0,-0.0,1e5,-7,0.30000000000000004],"d":{},"e":[],"f":null,"g":true}`)
	n, err := save.Parse(src)
	require.NoError(t, err)
	require.Equal(t, src, n.Bytes(), "R3.1: untouched bytes are reproduced exactly")

	s, ok := n.Member("a").String()
	require.True(t, ok)
	require.Equal(t, "x\"y\\z/wé", s)
	raw, ok := n.Member("b").String()
	require.True(t, ok)
	require.False(t, save.ValidUTF8(raw))

	// Pretty output re-parses to the same compact bytes.
	again, err := save.Parse(n.Pretty())
	require.NoError(t, err)
	require.Equal(t, src, again.Bytes())

	// An edit reformats only its own node.
	n.Member("c").Index(3).SetInt(42)
	require.Equal(t, strings.Replace(string(src), "-7", "42", 1), string(n.Bytes()))
	n.Member("a").SetString("p/q")
	require.Contains(t, string(n.Bytes()), `"a":"p\/q"`, "slashes are escaped the way the game writes them")
}

func TestDocumentRejectsWhatIsNotJSON(t *testing.T) {
	for _, bad := range []string{
		``, `{`, `{"a":}`, `[1,]`, `{"a":1}x`, `"unterminated`, `{"a" 1}`,
		// Escapes and numbers a hand-edited export could carry; the game
		// never writes them and they must not reach a save (R5.3).
		`{"a":"\q"}`, `{"a":"\u12"}`, `{"a":"\u12G4"}`, `{"a":"\"}`,
		`{"a":01}`, `{"a":1.}`, `{"a":.5}`, `{"a":1e}`, `{"a":-}`, `{"a":+1}`,
	} {
		_, err := save.Parse([]byte(bad))
		require.ErrorIs(t, err, save.ErrSyntax, bad)
	}
	// What the grammar does allow.
	for _, good := range []string{
		`{"a":0}`, `{"a":-0.0}`, `{"a":1e5}`, `{"a":1.5E-3}`, `{"a":"\u00e9\n\/"}`,
		"{\"a\":\"^\xff\x01#\"}", // raw bytes, not escapes: accepted
	} {
		_, err := save.Parse([]byte(good))
		require.NoError(t, err, good)
	}
}

// --- mapping (R3.2, R3.3) ---------------------------------------------------

func TestMappingNamesResolvesAndReportsCoverage(t *testing.T) {
	m := testMapping(t)
	require.Equal(t, "Version", m.Name("F2P"))
	require.Equal(t, "zzz", m.Name("zzz"), "an unknown key is shown as itself")
	require.Equal(t, "F2P", m.Resolve("Version"))
	require.Equal(t, "F2P", m.Resolve("F2P"), "a raw key passes through")
	require.Equal(t, "RocketLockerInventory", m.Name("V86"), "first entry wins for an ambiguous key")

	root, err := save.Parse(payload(4735))
	require.NoError(t, err)
	v, ok := m.Lookup(root, "Version").Int()
	require.True(t, ok)
	require.EqualValues(t, 4735, v)
	units, ok := m.Lookup(root, "BaseContext/PlayerStateData/Units").Int()
	require.True(t, ok)
	require.EqualValues(t, 162810044, units)
	require.Nil(t, m.Lookup(root, "BaseContext/Nope"))

	require.Equal(t, []string{"E", "F", "Neg", "Path"}, m.Unmapped(root), "the keys the mapping lacks, sorted")

	var nilMap *save.Mapping
	require.Equal(t, "F2P", nilMap.Name("F2P"))
	require.Len(t, nilMap.Unmapped(root), 30, "with no mapping every key is unmapped")
}

func TestMappingDeobfuscatesForExportAndBackForImport(t *testing.T) {
	m := testMapping(t)
	src := payload(4735)
	root, err := save.Parse(src)
	require.NoError(t, err)
	missed := m.Deobfuscate(root)
	require.Equal(t, 4, missed, "E, F, Neg and Path have no names")
	named := string(root.Bytes())
	require.Contains(t, named, `"Version":4735`)
	require.Contains(t, named, `"ValidSlotIndices":[{"X":0,"Y":0}`)
	require.NotContains(t, named, `"F2P"`)

	back, err := save.Parse([]byte(named))
	require.NoError(t, err)
	require.Positive(t, m.Obfuscate(back), "the named keys are turned back")
	require.Equal(t, src, back.Bytes(), "R5.3: a named export imports to the original bytes")

	// A file that was never deobfuscated imports unchanged too.
	raw, err := save.Parse(src)
	require.NoError(t, err)
	m.Obfuscate(raw)
	require.Equal(t, src, raw.Bytes())
}

// --- edits (AC4, R5.4, R5.5) ------------------------------------------------

func TestSplitVersion(t *testing.T) {
	for _, tc := range []struct{ v, base, mode, season int64 }{
		{4735, 4223, 1, 0},
		{4665, 4153, 1, 0},
		{4098, 4098, 0, 0},
		{4098 + 5*512, 4098, 5, 0},
		{1514546, 4146, 6, 23},
	} {
		got := save.SplitVersion(tc.v)
		require.Equal(t, save.VersionInfo{Version: tc.v, Base: tc.base, GameMode: tc.mode, Season: tc.season}, got)
	}
	require.Equal(t, "Normal", save.GameModeName(1))
	require.Equal(t, "Seasonal", save.GameModeName(6))
}

func TestSummarizeReadsWhatTheEditorShows(t *testing.T) {
	root, err := save.Parse(payload(4735))
	require.NoError(t, err)
	s, err := save.Summarize(root, testMapping(t))
	require.NoError(t, err)
	require.EqualValues(t, 4223, s.Base)
	require.Equal(t, "BaseContext/PlayerStateData", s.StatePath)
	require.EqualValues(t, 162810044, s.Units)
	require.EqualValues(t, 36793, s.Nanites)
	require.EqualValues(t, 60, s.Health)
	require.EqualValues(t, 100, s.Shield)
	require.Equal(t, 3, s.Ships)
	require.Equal(t, 1, s.Multitools)
	require.EqualValues(t, 17088, s.PlayTime)
	require.Equal(t, save.InventorySummary{Width: 10, Height: 12, Valid: 34, Occupied: 2}, s.SuitItems)
	require.Equal(t, save.InventorySummary{Width: 10, Height: 6, Valid: 13}, s.SuitTech)
	require.Len(t, s.Unmapped, 4)
}

func TestUnlockingSlotsAppendsRowMajorAndTouchesNothingElse(t *testing.T) {
	m := testMapping(t)
	src := payload(4735)
	root, err := save.Parse(src)
	require.NoError(t, err)

	want := 60
	changes, err := save.Apply(root, m, save.ChangeSet{SuitItemSlots: &want})
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "34", changes[0].Old)
	require.Equal(t, "60", changes[0].New)
	require.Equal(t, "BaseContext/PlayerStateData/Inventory/ValidSlotIndices", changes[0].Path)

	// AC4: exactly the 26 next cells, in row-major order, after the existing
	// ones; the rest of the payload byte-identical.
	expect := strings.Replace(string(src), cells(10, 34), cells(10, 60), 1)
	require.Equal(t, expect, string(root.Bytes()))
	inv := m.Lookup(root, "BaseContext/PlayerStateData/Inventory")
	sum, err := save.Inventory(inv, m)
	require.NoError(t, err)
	require.Equal(t, 60, sum.Valid)
	require.Equal(t, 2, sum.Occupied, "no Slots change")
}

func TestShrinkingSlotsRefusesAnOccupiedCellAndTheGridBoundsHold(t *testing.T) {
	m := testMapping(t)
	root, err := save.Parse(payload(4735))
	require.NoError(t, err)

	// Cell (3,3) is index 33 and holds ^CARBON; asking for 33 cells would drop it.
	want := 33
	_, err = save.Apply(root, m, save.ChangeSet{SuitItemSlots: &want})
	require.ErrorIs(t, err, save.ErrOccupied)
	require.ErrorContains(t, err, "(3,3)")
	require.ErrorContains(t, err, "^CARBON")

	// Above the grid.
	want = 121
	_, err = save.Apply(root, m, save.ChangeSet{SuitItemSlots: &want})
	require.ErrorContains(t, err, "1..120")

	// Below one.
	want = 0
	_, err = save.Apply(root, m, save.ChangeSet{SuitTechSlots: &want})
	require.ErrorContains(t, err, "1..60")

	// A shrink that touches only empty cells works, from the end.
	want = 34
	changes, err := save.Apply(root, m, save.ChangeSet{SuitItemSlots: &want})
	require.NoError(t, err)
	require.Empty(t, changes, "already 34: nothing to do, nothing reported")
	want = 12
	changes, err = save.Apply(root, m, save.ChangeSet{SuitTechSlots: &want})
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "13", changes[0].Old)
}

func TestCurrencyAndVitalsEditsAreBoundedAndSkipUnchangedValues(t *testing.T) {
	m := testMapping(t)
	root, err := save.Parse(payload(4735))
	require.NoError(t, err)

	units, nanites, same := uint64(4294967295), uint64(1000), uint64(0)
	health := int64(200)
	changes, err := save.Apply(root, m, save.ChangeSet{
		Units: &units, Nanites: &nanites, Quicksilver: &same, Health: &health,
	})
	require.NoError(t, err)
	require.Len(t, changes, 3, "quicksilver was already 0")
	require.Equal(t, "Units", changes[0].Field)
	require.Equal(t, "162810044", changes[0].Old)
	require.Equal(t, "4294967295", changes[0].New)
	require.Contains(t, string(root.Bytes()), `"wGS":4294967295,"7QL":1000,"kN;":0,"ZeS":200`)

	over := uint64(4294967296)
	_, err = save.Apply(root, m, save.ChangeSet{Units: &over})
	require.ErrorContains(t, err, "maximum")
	neg := int64(-1)
	_, err = save.Apply(root, m, save.ChangeSet{Shield: &neg})
	require.ErrorContains(t, err, "negative")

	_, err = save.Apply(root, m, save.ChangeSet{})
	require.ErrorIs(t, err, save.ErrNothingToDo)
}

func TestEditsRefuseAPreWaypointSaveAndNeedTheMapping(t *testing.T) {
	m := testMapping(t)
	root, err := save.Parse(payload(4098 + 512))
	require.NoError(t, err)
	one := uint64(1)
	_, err = save.Apply(root, m, save.ChangeSet{Units: &one})
	require.ErrorIs(t, err, save.ErrTooOld)

	root, err = save.Parse(payload(4735))
	require.NoError(t, err)
	_, err = save.Apply(root, nil, save.ChangeSet{Units: &one})
	require.ErrorIs(t, err, save.ErrNoMapping)
	_, err = save.Summarize(root, nil)
	require.ErrorIs(t, err, save.ErrNoMapping)
}
