package save

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
)

/*
The manifest (R2).

Beside every save the game keeps `mf_<name>.hg`: a small, XXTEA-encrypted
record of what the save is, read by the slot-select screen without
decompressing two megabytes of JSON. It carries the save's decompressed and
on-disk sizes, which is why an edited save needs its manifest rewritten, and a
timestamp the game sets equal to the data file's mtime.

The cipher is XXTEA with a fixed round count and a key whose first word depends
on the slot, so a manifest copied to another slot's name does not decrypt.
Measured on this project's own saves (spec 007 recon): six rounds for every
layout since Frontiers, eight for the 104-byte layout before it.

Layouts, by total length:

	0x068 (104)  1.10 - 3.53   sizes, version, mode, season, play time
	0x168 (360)  Waypoint 4.0  + save name, summary, difficulty (one byte)
	0x180 (384)  Worlds I 5.0  + difficulty as uint32, slot id, timestamp, format again
	0x1B0 (432)  Worlds II 5.5 same fields, longer reserved tail

Everything after the fields this file knows is carried through verbatim, so a
field the game adds later survives a rewrite untouched.
*/

// MetaHeader is the first word of a decrypted manifest.
const MetaHeader = 0xEEEEEEBE

// The offsets of the known fields, shared by every layout that is long enough
// to hold them.
const (
	offFormat       = 0x04
	offDecompressed = 0x38
	offDisk         = 0x3C
	offBaseVersion  = 0x44
	offGameMode     = 0x48
	offSeason       = 0x4A
	offPlayTime     = 0x4C
	offSaveName     = 0x58
	offSummary      = 0xD8
	offDifficulty   = 0x158
	offTimestamp    = 0x164

	metaTextLen    = 128
	metaLenVanilla = 0x68
	metaLenWorlds  = 0x180
)

// ErrMetaHeader reports a manifest that did not decrypt to the expected header:
// the wrong slot, a file that is not a manifest, or a layout this code does not
// know the round count for.
var ErrMetaHeader = errors.New("manifest did not decrypt to a valid header")

// Meta is a decrypted manifest. The plaintext is kept whole so that a rewrite
// changes only the fields it means to.
type Meta struct {
	plain []byte
}

// DecodeMeta decrypts a manifest for the slot named by SlotIndex (R2.1).
func DecodeMeta(raw []byte, slot uint32) (*Meta, error) {
	if len(raw) < 8 || len(raw)%4 != 0 {
		return nil, fmt.Errorf("manifest is %d bytes; a manifest is a multiple of 4 bytes and at least 8", len(raw))
	}
	plain := make([]byte, len(raw))
	copy(plain, raw)
	xxtea(plain, metaKey(slot), metaRounds(len(plain)), false)
	m := &Meta{plain: plain}
	if m.u32(0) != MetaHeader {
		return nil, fmt.Errorf("%w (slot %d, %d bytes)", ErrMetaHeader, slot, len(raw))
	}
	return m, nil
}

// NewMeta builds a manifest from plaintext, for tests and for a save that has
// none. The header word is set; every other field is whatever the caller put
// there.
func NewMeta(plain []byte) (*Meta, error) {
	if len(plain) < 8 || len(plain)%4 != 0 {
		return nil, fmt.Errorf("manifest is %d bytes; a manifest is a multiple of 4 bytes and at least 8", len(plain))
	}
	m := &Meta{plain: append([]byte(nil), plain...)}
	binary.LittleEndian.PutUint32(m.plain[0:], MetaHeader)
	return m, nil
}

// Encode encrypts the manifest for a slot (R2.1). An unchanged manifest
// encodes to the bytes it was decoded from (R2.3).
func (m *Meta) Encode(slot uint32) []byte {
	out := make([]byte, len(m.plain))
	copy(out, m.plain)
	xxtea(out, metaKey(slot), metaRounds(len(out)), true)
	return out
}

// Len is the manifest's length in bytes, which names its layout.
func (m *Meta) Len() int { return len(m.plain) }

// Plain is a copy of the decrypted bytes.
func (m *Meta) Plain() []byte { return append([]byte(nil), m.plain...) }

// Format is the manifest format number: 2001 before Frontiers, 2002 from
// Frontiers, 2003 from Worlds Part I, 2004 from Worlds Part II.
func (m *Meta) Format() uint32 { return m.u32(offFormat) }

// DecompressedSize is the payload length the game recorded, NUL included.
func (m *Meta) DecompressedSize() uint32 { return m.u32(offDecompressed) }

// DiskSize is the data file's length on disk; zero in the Frontiers layout,
// which did not record it.
func (m *Meta) DiskSize() uint32 { return m.u32(offDisk) }

// SetSizes records a rewritten data file (R6.3). DiskSize is left at zero when
// the manifest never carried one, so a Frontiers-era save stays in its own
// convention.
func (m *Meta) SetSizes(decompressed, disk uint32) {
	m.put32(offDecompressed, decompressed)
	if m.u32(offDisk) != 0 || m.Format() >= 2003 {
		m.put32(offDisk, disk)
	}
}

// BaseVersion is the game data version without the mode and season offsets.
func (m *Meta) BaseVersion() uint32 { return m.u32(offBaseVersion) }

// GameMode is the preset mode: 1 normal, 2 creative, 3 survival, 4 ambient,
// 5 permadeath, 6 seasonal.
func (m *Meta) GameMode() uint16 { return m.u16(offGameMode) }

// Season is the expedition number for a seasonal save, else zero.
func (m *Meta) Season() uint16 { return m.u16(offSeason) }

// PlayTime is the total play time in seconds.
func (m *Meta) PlayTime() uint64 {
	if len(m.plain) < offPlayTime+8 {
		return 0
	}
	return binary.LittleEndian.Uint64(m.plain[offPlayTime:])
}

// SaveName is the name the player gave the save, "" when unnamed or when the
// layout predates names.
func (m *Meta) SaveName() string { return m.text(offSaveName) }

// SaveSummary is the game's one-line "where you are" for the slot screen.
func (m *Meta) SaveSummary() string { return m.text(offSummary) }

// Difficulty is the preset index the slot screen shows. One byte in the
// Waypoint layout, four from Worlds Part I.
func (m *Meta) Difficulty() uint32 {
	switch {
	case len(m.plain) >= metaLenWorlds:
		return m.u32(offDifficulty)
	case len(m.plain) > offDifficulty:
		return uint32(m.plain[offDifficulty])
	}
	return 0
}

// Timestamp is the Unix time the game wrote the save, from Worlds Part I on;
// zero for older layouts. It equals the data file's mtime when the game wrote
// both.
func (m *Meta) Timestamp() uint32 {
	if len(m.plain) < offTimestamp+4 {
		return 0
	}
	return m.u32(offTimestamp)
}

// HasTimestamp says whether the layout carries one, so a writer knows whether
// to set it.
func (m *Meta) HasTimestamp() bool { return len(m.plain) >= offTimestamp+4 }

// SetTimestamp records when a rewrite happened (R6.3). A layout without the
// field is left alone.
func (m *Meta) SetTimestamp(unix uint32) {
	if m.HasTimestamp() {
		m.put32(offTimestamp, unix)
	}
}

func (m *Meta) u32(off int) uint32 {
	if len(m.plain) < off+4 {
		return 0
	}
	return binary.LittleEndian.Uint32(m.plain[off:])
}

func (m *Meta) u16(off int) uint16 {
	if len(m.plain) < off+2 {
		return 0
	}
	return binary.LittleEndian.Uint16(m.plain[off:])
}

func (m *Meta) put32(off int, v uint32) {
	if len(m.plain) >= off+4 {
		binary.LittleEndian.PutUint32(m.plain[off:], v)
	}
}

// text reads a NUL-terminated field of metaTextLen bytes.
func (m *Meta) text(off int) string {
	if len(m.plain) < off+metaTextLen {
		return ""
	}
	field := m.plain[off : off+metaTextLen]
	for i, c := range field {
		if c == 0 {
			return string(field[:i])
		}
	}
	return string(field)
}

// --- the cipher -------------------------------------------------------------

const xxteaDelta = 0x9E3779B9

// metaRounds is the fixed round count for a layout. Six for everything since
// Frontiers; eight for the original 104-byte manifest.
func metaRounds(length int) int {
	if length == metaLenVanilla {
		return 8
	}
	return 6
}

// metaKey derives the slot's key: the game's fixed sixteen-byte key with its
// first word replaced by a mix of the slot index.
func metaKey(slot uint32) [4]uint32 {
	const fixed = "NAESEVADNAYRTNRG"
	var key [4]uint32
	for i := range key {
		key[i] = binary.LittleEndian.Uint32([]byte(fixed[i*4 : i*4+4]))
	}
	key[0] = bits.RotateLeft32(slot^0x1422CB8C, 13)*5 + 0xE6546B64
	return key
}

// xxtea runs the block cipher over the buffer in place, in whichever direction
// is asked, with a fixed round count rather than the reference 6+52/n.
func xxtea(buf []byte, key [4]uint32, rounds int, encrypt bool) {
	n := len(buf) / 4
	v := make([]uint32, n)
	for i := range v {
		v[i] = binary.LittleEndian.Uint32(buf[i*4:])
	}
	mx := func(sum, y, z, p, e uint32) uint32 {
		return (((z >> 5) ^ (y << 2)) + ((y >> 3) ^ (z << 4))) ^ ((sum ^ y) + (key[(p&3)^e] ^ z))
	}
	if encrypt {
		var sum uint32
		z := v[n-1]
		for r := 0; r < rounds; r++ {
			sum += xxteaDelta
			e := (sum >> 2) & 3
			for p := 0; p < n-1; p++ {
				y := v[p+1]
				v[p] += mx(sum, y, z, uint32(p), e) //nolint:gosec // p < n, a small buffer
				z = v[p]
			}
			y := v[0]
			v[n-1] += mx(sum, y, z, uint32(n-1), e) //nolint:gosec // n is a small buffer length
			z = v[n-1]
		}
	} else {
		sum := uint32(rounds) * xxteaDelta //nolint:gosec // rounds is 6 or 8
		y := v[0]
		for r := 0; r < rounds; r++ {
			e := (sum >> 2) & 3
			for p := n - 1; p > 0; p-- {
				z := v[p-1]
				v[p] -= mx(sum, y, z, uint32(p), e) //nolint:gosec // p < n, a small buffer
				y = v[p]
			}
			z := v[n-1]
			v[0] -= mx(sum, y, z, 0, e)
			y = v[0]
			sum -= xxteaDelta
		}
	}
	for i, w := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], w)
	}
}
