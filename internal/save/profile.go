package save

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

/*
Slot and file naming (R2.2, R5).

The game numbers its files, not its slots: `save.hg` is file 1, `saveN.hg` is
file N, and the slot screen shows file pairs -- an odd file is a slot's autosave
and the even file after it is the same slot's manual save. So slot 9 is
`save17.hg` (auto) and `save18.hg` (manual). The manifest cipher keys on yet a
third number, the file number plus one, because the game counts the account
data file as its first slot. All three live here so nothing else has to know.
*/

// MaxSlots is how many save slots the game offers.
const MaxSlots = 15

// Kind is which half of a slot a file is.
type Kind string

// The two halves of a slot.
const (
	KindAuto   Kind = "auto"
	KindManual Kind = "manual"
)

// SlotRef names one save file by slot and kind.
type SlotRef struct {
	Slot int  `json:"slot"`
	Kind Kind `json:"kind"`
}

// ErrSlot reports a slot reference that names nothing the game has.
var ErrSlot = errors.New("not a save slot")

// Valid says whether the reference is within the game's range.
func (r SlotRef) Valid() bool {
	return r.Slot >= 1 && r.Slot <= MaxSlots && (r.Kind == KindAuto || r.Kind == KindManual)
}

// FileNumber is the game's number for this file: 1 for slot 1 auto, 2 for slot
// 1 manual, and so on.
func (r SlotRef) FileNumber() int {
	n := (r.Slot-1)*2 + 1
	if r.Kind == KindManual {
		n++
	}
	return n
}

// DataFile is the save's file name.
func (r SlotRef) DataFile() string { return DataFileName(r.FileNumber()) }

// MetaFile is the manifest's file name.
func (r SlotRef) MetaFile() string { return MetaFileName(r.DataFile()) }

// String renders the reference the way the CLI accepts it: "9:manual".
func (r SlotRef) String() string { return fmt.Sprintf("%d:%s", r.Slot, r.Kind) }

// DataFileName is the game's name for file n: "save.hg" for 1, "saveN.hg" after.
func DataFileName(n int) string {
	if n == 1 {
		return "save.hg"
	}
	return "save" + strconv.Itoa(n) + ".hg"
}

// MetaFileName is the manifest beside a data file.
func MetaFileName(dataFile string) string { return "mf_" + dataFile }

// ParseFileName recognises a save data file name and says which slot it is.
// Manifests, the account data file and anything else are rejected.
func ParseFileName(name string) (SlotRef, bool) {
	if name == "save.hg" {
		return SlotRef{Slot: 1, Kind: KindAuto}, true
	}
	if !strings.HasPrefix(name, "save") || !strings.HasSuffix(name, ".hg") {
		return SlotRef{}, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "save"), ".hg"))
	if err != nil || n < 2 || n > MaxSlots*2 {
		return SlotRef{}, false
	}
	ref := SlotRef{Slot: (n + 1) / 2, Kind: KindAuto}
	if n%2 == 0 {
		ref.Kind = KindManual
	}
	return ref, true
}

// SlotIndex is the number the manifest cipher keys on (R2.2): 1 for the
// account data, then the file number plus one.
func SlotIndex(dataFile string) (uint32, error) {
	if dataFile == "accountdata.hg" {
		return 1, nil
	}
	ref, ok := ParseFileName(dataFile)
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrSlot, dataFile)
	}
	return uint32(ref.FileNumber()) + 1, nil //nolint:gosec // at most MaxSlots*2+1
}

/*
ParseSlotRef reads what a user typed: "9", "9:auto", "9:manual", "9/manual".

kindGiven tells the caller whether to fall back to the newest half of the pair,
which is what "9" alone should mean: the file the game would load.
*/
func ParseSlotRef(s string) (ref SlotRef, kindGiven bool, err error) {
	s = strings.TrimSpace(strings.ToLower(s))
	num, kind := s, ""
	if i := strings.IndexAny(s, ":/"); i >= 0 {
		num, kind = s[:i], s[i+1:]
	}
	n, convErr := strconv.Atoi(num)
	if convErr != nil || n < 1 || n > MaxSlots {
		return SlotRef{}, false, fmt.Errorf("%w: %q (slots are 1..%d, optionally :auto or :manual)", ErrSlot, s, MaxSlots)
	}
	ref = SlotRef{Slot: n, Kind: KindAuto}
	switch kind {
	case "":
		return ref, false, nil
	case "auto", "a":
		return ref, true, nil
	case "manual", "m":
		ref.Kind = KindManual
		return ref, true, nil
	}
	return SlotRef{}, false, fmt.Errorf("%w: %q is neither auto nor manual", ErrSlot, kind)
}
