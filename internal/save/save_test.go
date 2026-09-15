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
		{":rc", "GroupId"}, {"gUR", "Stats"}, {">MX", "Value"}, {">vs", "IntValue"},
		{"aBc", "PrimaryShip"}, {"NTx", "Resource"}, {"93M", "Filename"}, {"B@N", "Class"}, {"1o6", "InventoryClass"},
		{"gan", "Inventory_Cargo"}, {"8ZP", "FreighterInventory"}, {"0wS", "FreighterInventory_TechOnly"},
		{"FdP", "FreighterInventory_Cargo"}, {"NKm", "Name"}, {"CuF", "CurrentFreighter"}, {"@EL", "Seed"},
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
		`"aBc":1,"@Cs":[{"NKm":"","NTx":{"93M":"MODELS/COMMON/SPACECRAFT/FIGHTERS/FIGHTER_PROC.SCENE.MBIN"},` +
		`";l5":{":No":[],"hl?":` + cells(10, 20) + `,"B@N":{"1o6":"C"},"=Tb":10,"N9>":4},` +
		`"PMT":{":No":[],"hl?":` + cells(10, 5) + `,"B@N":{"1o6":"C"},"=Tb":10,"N9>":2},` +
		`"gan":{":No":[],"hl?":[],"B@N":{"1o6":"C"},"=Tb":7,"N9>":5}},` +
		`{"NKm":"Ada","NTx":{"93M":"MODELS/COMMON/SPACECRAFT/DROPSHIPS/DROPSHIP_PROC.SCENE.MBIN"},` +
		`";l5":{":No":[{"b2n":"^GOLD","1o9":1,"3ZH":{">Qh":0,"XJ>":0}}],"hl?":` + cells(10, 59) + `,"B@N":{"1o6":"A"},"=Tb":10,"N9>":6},` +
		`"PMT":{":No":[],"hl?":` + cells(10, 28) + `,"B@N":{"1o6":"A"},"=Tb":10,"N9>":3},` +
		`"gan":{":No":[],"hl?":[],"B@N":{"1o6":"A"},"=Tb":7,"N9>":5}},{}],"OsQ":[{}],` +
		`"8ZP":{":No":[],"hl?":` + cells(7, 19) + `,"B@N":{"1o6":"A"},"=Tb":7,"N9>":5},` +
		`"0wS":{":No":[],"hl?":` + cells(7, 12) + `,"B@N":{"1o6":"A"},"=Tb":7,"N9>":3},` +
		`"FdP":{":No":[],"hl?":[],"B@N":{"1o6":"A"},"=Tb":7,"N9>":5},` +
		`"CuF":{"93M":"MODELS/COMMON/SPACECRAFT/INDUSTRIAL/FREIGHTER_PROC.SCENE.MBIN","@EL":[true,"0x1"]},` +
		`"Path":"MODELS\/COMMON\/X.MBIN","F":1.0,"Neg":-0.0,"E":1e5,` +
		`"gUR":[{":rc":"^SYSTEM_STATS","gUR":[{"b2n":"^WAR_STANDING",">MX":{">vs":15}}]},` +
		`{":rc":"^GLOBAL_STATS","gUR":[{"b2n":"^WAR_STANDING",">MX":{">vs":135}},{"b2n":"^EGUILD_STAND",">MX":{}},` +
		`{"b2n":"^TRA_STANDING",">MX":{">vs":45}},{"b2n":"^EXP_STANDING",">MX":{">vs":5}},{"b2n":"^WGUILD_STAND",">MX":{">vs":17}}]}]}}}`)
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
	require.Contains(t, string(n.Bytes()), `"a":"p/q"`, "slashes are written plain, as the game's own paths are")
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
	require.Len(t, nilMap.Unmapped(root), 46, "with no mapping every key is unmapped")
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

	// Above the game's ceiling.
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

// --- faction standing (spec 008) ---------------------------------------------

func TestStandingsReadTheGlobalStatsAndEditsWriteThemBack(t *testing.T) {
	m := testMapping(t)
	src := payload(4735)
	root, err := save.Parse(src)
	require.NoError(t, err)

	sum, err := save.Summarize(root, m)
	require.NoError(t, err)
	require.Len(t, sum.Standings, 6)
	byKey := map[string]save.StandingValue{}
	for _, sv := range sum.Standings {
		byKey[sv.Key] = sv
	}
	require.EqualValues(t, 135, byKey["vykeen"].Value, "the global value, not the per-system 15")
	require.EqualValues(t, 45, byKey["gek"].Value)
	require.EqualValues(t, 5, byKey["korvax"].Value)
	require.EqualValues(t, 17, byKey["mercenaries"].Value)
	require.EqualValues(t, 0, byKey["explorers"].Value)
	require.True(t, byKey["explorers"].Present, "an empty union is a present standing of zero")
	require.False(t, byKey["merchants"].Present, "the fixture has no ^TGUILD_STAND entry")

	// Levels, as the game shows them: 135 is level 9, 45 level 7, 5 level 2,
	// 17 level 4, 0 level 1.
	require.Equal(t, 9, byKey["vykeen"].Level)
	require.Equal(t, 7, byKey["gek"].Level)
	require.Equal(t, 2, byKey["korvax"].Level)
	require.Equal(t, 4, byKey["mercenaries"].Level)
	require.Equal(t, 1, byKey["explorers"].Level)
	require.Equal(t, 0, save.StandingLevel(-3), "hostile")

	changes, err := save.Apply(root, m, save.ChangeSet{Standings: map[string]int64{
		"vykeen": 5, "explorers": 2, "mercenaries": 1, "korvax": 2,
	}})
	require.NoError(t, err)
	require.Len(t, changes, 3, "korvax was already level 2")
	out := string(root.Bytes())
	require.Contains(t, out, `{"b2n":"^WAR_STANDING",">MX":{">vs":21}}`, "level 5 starts at 21")
	require.Contains(t, out, `{"b2n":"^EGUILD_STAND",">MX":{">vs":3}}`, "level 2 starts at 3")
	require.Contains(t, out, `{"b2n":"^WGUILD_STAND",">MX":{}}`, "level 1 is zero, written the way the game writes it")
	require.Contains(t, out, `{":rc":"^SYSTEM_STATS","gUR":[{"b2n":"^WAR_STANDING",">MX":{">vs":15}}]}`, "per-system standing untouched")
	require.Equal(t, "Vy'keen standing", changes[0].Field)
	require.Equal(t, "level 9 (135)", changes[0].Old)
	require.Equal(t, "level 5 (21)", changes[0].New)

	_, err = save.Apply(root, m, save.ChangeSet{Standings: map[string]int64{"merchants": 1}})
	require.ErrorContains(t, err, "^TGUILD_STAND")
	_, err = save.Apply(root, m, save.ChangeSet{Standings: map[string]int64{"gek": 10}})
	require.ErrorContains(t, err, "outside 1..9")
	_, err = save.Apply(root, m, save.ChangeSet{Standings: map[string]int64{"sentinels": 1}})
	require.ErrorContains(t, err, "unknown faction")
}

// Spec 009: paths address array elements by index, and Replace puts a value at
// an existing path only.
func TestLookupIndexesArraysAndReplaceNeedsAnExistingPath(t *testing.T) {
	m := testMapping(t)
	root, err := save.Parse(payload(4735))
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, save.SplitPath("/a//b/"))

	id, ok := m.Lookup(root, "BaseContext/PlayerStateData/Inventory/Slots/1/Id").String()
	require.True(t, ok)
	require.Equal(t, "^CARBON", id)
	require.Nil(t, m.Lookup(root, "BaseContext/PlayerStateData/Inventory/Slots/x"))
	require.Nil(t, m.Lookup(root, "BaseContext/PlayerStateData/Inventory/Slots/9"))

	require.True(t, m.Replace(root, "BaseContext/PlayerStateData/Inventory/Slots/1/Id", save.NewString("^GOLD")))
	id, _ = m.Lookup(root, "BaseContext/PlayerStateData/Inventory/Slots/1/Id").String()
	require.Equal(t, "^GOLD", id)
	require.True(t, m.Replace(root, "BaseContext/PlayerStateData/Inventory/Slots/0", save.NewObject()))
	require.Equal(t, 0, m.Lookup(root, "BaseContext/PlayerStateData/Inventory/Slots/0").Len())
	require.False(t, m.Replace(root, "BaseContext/PlayerStateData/NewKey", save.NewInt(1)), "no adding")
	require.False(t, m.Replace(root, "BaseContext/PlayerStateData/Inventory/Slots/7", save.NewInt(1)))
	require.False(t, m.Replace(root, "", save.NewInt(1)))
	require.Equal(t, "object", root.TypeName())
	require.Equal(t, "missing", (*save.Node)(nil).TypeName())
}

// --- starships and the freighter (spec 011) -----------------------------------

func TestShipsAndFreighterAreSummarisedAndTheirSlotsAndClassEdited(t *testing.T) {
	m := testMapping(t)
	root, err := save.Parse(payload(4735))
	require.NoError(t, err)
	sum, err := save.Summarize(root, m)
	require.NoError(t, err)

	require.Len(t, sum.ShipList, 2, "the empty third entry is skipped")
	require.Equal(t, "Fighter", sum.ShipList[0].Kind)
	require.Equal(t, "fighter", sum.ShipList[0].Type)
	require.Equal(t, "hauler", sum.ShipList[1].Type)
	require.Equal(t, "Hauler", sum.ShipList[1].Kind, "the in-game name, not the model's")
	require.Equal(t, "regular", sum.Freighter.Type)
	require.Equal(t, "C", sum.ShipList[0].Class)
	require.False(t, sum.ShipList[0].Primary)
	require.Equal(t, save.InventorySummary{Width: 10, Height: 4, Valid: 20}, sum.ShipList[0].Items)
	require.Equal(t, "Hauler", sum.ShipList[1].Kind)
	require.Equal(t, "Ada", sum.ShipList[1].Name)
	require.True(t, sum.ShipList[1].Primary)
	require.Equal(t, "2: Hauler “Ada” (A) — current", sum.ShipList[1].Label())
	require.Equal(t, save.InventorySummary{Width: 10, Height: 6, Valid: 59, Occupied: 1}, sum.ShipList[1].Items)
	require.True(t, sum.Freighter.Present)
	require.Equal(t, "A", sum.Freighter.Class)
	require.Equal(t, save.InventorySummary{Width: 7, Height: 5, Valid: 19}, sum.Freighter.Items)
	require.Equal(t, save.InventorySummary{Width: 7, Height: 3, Valid: 12}, sum.Freighter.Tech)

	// Slots on the primary ship (Ship -1) and on the freighter; class on both.
	sixty, thirty, fItems, fTech := 60, 30, 35, 21
	sClass, fClass := "S", "S"
	changes, err := save.Apply(root, m, save.ChangeSet{
		Ship: -1, ShipItemSlots: &sixty, ShipTechSlots: &thirty, ShipClass: &sClass,
		FreighterItemSlots: &fItems, FreighterTechSlots: &fTech, FreighterClass: &fClass,
	})
	require.NoError(t, err)
	require.Len(t, changes, 6)
	require.Equal(t, "BaseContext/PlayerStateData/ShipOwnership/1/Inventory/ValidSlotIndices", changes[0].Path)
	require.Equal(t, "ship class", changes[2].Field)
	require.Equal(t, "A", changes[2].Old)
	require.Equal(t, "S", changes[2].New)
	after, err := save.Summarize(root, m)
	require.NoError(t, err)
	require.Equal(t, 60, after.ShipList[1].Items.Valid)
	require.Equal(t, 30, after.ShipList[1].Tech.Valid)
	require.Equal(t, "S", after.ShipList[1].Class)
	require.Equal(t, "C", after.ShipList[0].Class, "the other ship is untouched")
	require.Equal(t, 35, after.Freighter.Items.Valid)
	require.Equal(t, "S", after.Freighter.Class)
	out := string(root.Bytes())
	require.Equal(t, 3, strings.Count(out, `"B@N":{"1o6":"S"}`)-3, "all three inventories of ship and freighter carry the class")

	// A named ship by index, bounds and bad input.
	ten := 10
	changes, err = save.Apply(root, m, save.ChangeSet{Ship: 0, ShipItemSlots: &ten})
	require.NoError(t, err)
	require.Equal(t, "20", changes[0].Old)
	// Past the grid the save has, up to the game's ceiling: the grid grows a
	// row at a time, the way the game grows it when slots are bought.
	grow := 75
	changes, err = save.Apply(root, m, save.ChangeSet{Ship: 1, ShipItemSlots: &grow})
	require.NoError(t, err)
	require.Equal(t, "60", changes[0].Old)
	require.Equal(t, "75", changes[0].New)
	grown, err := save.Summarize(root, m)
	require.NoError(t, err)
	require.Equal(t, save.InventorySummary{Width: 10, Height: 8, Valid: 75, Occupied: 1}, grown.ShipList[1].Items)
	full := 120
	_, err = save.Apply(root, m, save.ChangeSet{Ship: 1, ShipItemSlots: &full})
	require.NoError(t, err)
	grown, _ = save.Summarize(root, m)
	require.Equal(t, 12, grown.ShipList[1].Items.Height)
	over := 121
	_, err = save.Apply(root, m, save.ChangeSet{Ship: 1, ShipItemSlots: &over})
	require.ErrorContains(t, err, "1..120")
	techOver := 61
	_, err = save.Apply(root, m, save.ChangeSet{Ship: 1, ShipTechSlots: &techOver})
	require.ErrorContains(t, err, "1..60")
	_, err = save.Apply(root, m, save.ChangeSet{Ship: 7, ShipItemSlots: &ten})
	require.ErrorContains(t, err, "no ship at index 7")
	bad := "X"
	_, err = save.Apply(root, m, save.ChangeSet{Ship: 0, ShipClass: &bad})
	require.ErrorContains(t, err, "not one of C, B, A, S")
	same := "S"
	changes, err = save.Apply(root, m, save.ChangeSet{FreighterClass: &same})
	require.NoError(t, err)
	require.Empty(t, changes, "already S")

	// Types: the scene file changes, the seed stays, the label follows.
	exotic, capital := "exotic", "capital"
	changes, err = save.Apply(root, m, save.ChangeSet{Ship: 0, ShipType: &exotic, FreighterType: &capital})
	require.NoError(t, err)
	require.Len(t, changes, 2)
	require.Equal(t, "fighter", changes[0].Old)
	require.Equal(t, "exotic", changes[0].New)
	require.Equal(t, "BaseContext/PlayerStateData/ShipOwnership/0/Resource/Filename", changes[0].Path)
	typed, err := save.Summarize(root, m)
	require.NoError(t, err)
	require.Equal(t, "exotic", typed.ShipList[0].Type)
	require.Equal(t, "Exotic", typed.ShipList[0].Kind)
	require.Equal(t, "capital", typed.Freighter.Type)
	require.Contains(t, string(root.Bytes()), `"93M":"MODELS/COMMON/SPACECRAFT/S-CLASS/S-CLASS_PROC.SCENE.MBIN"`)
	require.Contains(t, string(root.Bytes()), `"CuF":{"93M":"MODELS/COMMON/SPACECRAFT/INDUSTRIAL/CAPITALFREIGHTER_PROC.SCENE.MBIN","@EL":[true,"0x1"]}`, "the seed is kept")
	nope := "corvette"
	_, err = save.Apply(root, m, save.ChangeSet{Ship: 0, ShipType: &nope})
	require.ErrorContains(t, err, "not one of fighter, hauler")
	changes, err = save.Apply(root, m, save.ChangeSet{Ship: 0, ShipType: &exotic})
	require.NoError(t, err)
	require.Empty(t, changes, "already exotic")
}
