package save

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

/*
The typed edits (R5.4) and the summary the editor shows (R5.2).

Every function here takes the tree and the mapping and works in names; the
mapping turns them into the keys the tree holds. Nothing is hard-coded to an
obfuscated key, so a mapping that renames one is followed automatically, and a
mapping that lacks one fails with the name in the error rather than editing the
wrong field.

Currencies are unsigned 32-bit in the game. Slot counts are bounded by the grid
the save already has: this editor unlocks cells the game knows about and does
not grow the grid, because the grid's size is the game's to decide and a grid
larger than the game's tables is the "game might break" case other editors warn
about.
*/

// MaxCurrency is the largest value the game's counters hold.
const MaxCurrency = math.MaxUint32

// MinBaseVersion is the oldest save layout this package edits: Waypoint 4.0,
// which introduced the context split everything here walks (R5.5).
const MinBaseVersion = 4140

// The names this package addresses. They are looked up in the mapping, never
// used as keys.
const (
	nameVersion           = "Version"
	namePlatform          = "Platform"
	nameActiveContext     = "ActiveContext"
	nameBaseContext       = "BaseContext"
	nameExpeditionContext = "ExpeditionContext"
	nameCommonState       = "CommonStateData"
	namePlayerState       = "PlayerStateData"
	nameSaveName          = "SaveName"
	nameTotalPlayTime     = "TotalPlayTime"
	nameUnits             = "Units"
	nameNanites           = "Nanites"
	nameSpecials          = "Specials"
	nameHealth            = "Health"
	nameShield            = "Shield"
	nameInventory         = "Inventory"
	nameInventoryTech     = "Inventory_TechOnly"
	nameWidth             = "Width"
	nameHeight            = "Height"
	nameValidSlotIndices  = "ValidSlotIndices"
	nameSlots             = "Slots"
	nameIndex             = "Index"
	nameX                 = "X"
	nameY                 = "Y"
	nameID                = "Id"
	nameShipOwnership     = "ShipOwnership"
	nameMultitools        = "Multitools"
)

// ErrNoMapping reports an operation that needs the key mapping and has none.
var ErrNoMapping = errors.New("the key mapping is needed for this; run `nmsbonker tools ensure`")

// ErrTooOld reports a save from before Waypoint 4.0.
var ErrTooOld = errors.New("save predates Waypoint 4.0 and is not supported")

// ErrOccupied reports a slot cell that holds an item and cannot be removed.
var ErrOccupied = errors.New("slot holds an item")

// ErrNothingToDo reports an empty change set.
var ErrNothingToDo = errors.New("no change was asked for")

// ChangeSet is what EditSave may change. A nil field is "leave it".
type ChangeSet struct {
	Units         *uint64 `json:"units,omitempty"`
	Nanites       *uint64 `json:"nanites,omitempty"`
	Quicksilver   *uint64 `json:"quicksilver,omitempty"`
	Health        *int64  `json:"health,omitempty"`
	Shield        *int64  `json:"shield,omitempty"`
	SuitItemSlots *int    `json:"suit_item_slots,omitempty"`
	SuitTechSlots *int    `json:"suit_tech_slots,omitempty"`
}

// Empty says whether the set asks for anything.
func (c ChangeSet) Empty() bool {
	return c.Units == nil && c.Nanites == nil && c.Quicksilver == nil &&
		c.Health == nil && c.Shield == nil && c.SuitItemSlots == nil && c.SuitTechSlots == nil
}

// Change is one edit that was, or would be, made.
type Change struct {
	Field string `json:"field"`
	Path  string `json:"path"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

// Version reads the payload's version and splits it (R5.5).
func Version(root *Node, m *Mapping) (VersionInfo, error) {
	key, ok := m.Key(nameVersion)
	if !ok {
		return VersionInfo{}, ErrNoMapping
	}
	v, ok := root.Member(key).Int()
	if !ok {
		return VersionInfo{}, fmt.Errorf("payload has no integer %s", nameVersion)
	}
	return SplitVersion(v), nil
}

// VersionInfo is the payload version taken apart.
type VersionInfo struct {
	// Version is the number as written.
	Version int64 `json:"version"`
	// Base is the game data version without the mode and season offsets.
	Base int64 `json:"base"`
	// GameMode is the preset the offset encodes (1 normal … 6 seasonal). Since
	// Waypoint every non-permadeath, non-seasonal save writes 1.
	GameMode int64 `json:"game_mode"`
	// Season is the expedition number for a seasonal save.
	Season int64 `json:"season"`
}

/*
SplitVersion undoes the game's packing: Version = base + 512·mode + 65536·season.

The base has stayed within one 512-wide band (4098 at Foundation, 4223 at
Cosmos) so it is recovered from the remainder rather than from thresholds,
which is what lets a save from a game newer than this code still report a
sensible number.
*/
func SplitVersion(v int64) VersionInfo {
	info := VersionInfo{Version: v}
	if v < 4096 {
		info.Base = v
		return info
	}
	info.Base = 4096 + v%512
	rest := (v - info.Base) / 512
	info.GameMode = rest % 128
	info.Season = rest / 128
	return info
}

// GameModeName is the preset's name for display.
func GameModeName(mode int64) string {
	switch mode {
	case 1:
		return "Normal"
	case 2:
		return "Creative"
	case 3:
		return "Survival"
	case 4:
		return "Ambient"
	case 5:
		return "Permadeath"
	case 6:
		return "Seasonal"
	}
	return "mode " + strconv.FormatInt(mode, 10)
}

/*
PlayerState finds the player state the game would load (R5.4).

Omega 4.50 split the save into BaseContext and ExpeditionContext and named
the live one in ActiveContext; a Waypoint-era save (4.0 to 4.4) still carries
one PlayerStateData at the top. Both layouts are read, and the returned prefix
is the path the change list shows, so a change on a flat save reads
"PlayerStateData/Units" rather than claiming a context it has not got.
*/
func PlayerState(root *Node, m *Mapping) (*Node, string, error) {
	if m == nil {
		return nil, "", ErrNoMapping
	}
	info, err := Version(root, m)
	if err != nil {
		return nil, "", err
	}
	if info.Base < MinBaseVersion {
		return nil, "", fmt.Errorf("%w (base version %d)", ErrTooOld, info.Base)
	}
	if flat := root.Member(m.Resolve(namePlayerState)); flat != nil && flat.Type() == TypeObject {
		return flat, namePlayerState, nil
	}
	context := nameBaseContext
	if active, ok := root.Member(m.Resolve(nameActiveContext)).String(); ok && active == "Season" {
		context = nameExpeditionContext
	}
	ps := root.Member(m.Resolve(context)).Member(m.Resolve(namePlayerState))
	if ps == nil || ps.Type() != TypeObject {
		return nil, "", fmt.Errorf("%s has no %s", context, namePlayerState)
	}
	return ps, context + "/" + namePlayerState, nil
}

// InventorySummary is the shape of one inventory.
type InventorySummary struct {
	Width    int `json:"width"`
	Height   int `json:"height"`
	Valid    int `json:"valid"`
	Occupied int `json:"occupied"`
}

// Max is the most cells the grid can hold.
func (s InventorySummary) Max() int { return s.Width * s.Height }

// Inventory summarises an inventory node.
func Inventory(inv *Node, m *Mapping) (InventorySummary, error) {
	if inv == nil {
		return InventorySummary{}, errors.New("inventory is missing")
	}
	w, okW := inv.Member(m.Resolve(nameWidth)).Int()
	h, okH := inv.Member(m.Resolve(nameHeight)).Int()
	valid := inv.Member(m.Resolve(nameValidSlotIndices))
	slots := inv.Member(m.Resolve(nameSlots))
	if !okW || !okH || valid == nil || valid.Type() != TypeArray || slots == nil || slots.Type() != TypeArray {
		return InventorySummary{}, errors.New("inventory lacks Width, Height, ValidSlotIndices or Slots")
	}
	return InventorySummary{Width: int(w), Height: int(h), Valid: valid.Len(), Occupied: slots.Len()}, nil
}

// Summary is what InspectSave shows (R5.2).
type Summary struct {
	VersionInfo
	Platform    string           `json:"platform"`
	StatePath   string           `json:"state_path"`
	SaveName    string           `json:"save_name"`
	PlayTime    int64            `json:"play_time_seconds"`
	Units       int64            `json:"units"`
	Nanites     int64            `json:"nanites"`
	Quicksilver int64            `json:"quicksilver"`
	Health      int64            `json:"health"`
	Shield      int64            `json:"shield"`
	Ships       int              `json:"ships"`
	Multitools  int              `json:"multitools"`
	SuitItems   InventorySummary `json:"suit_items"`
	SuitTech    InventorySummary `json:"suit_tech"`
	// Unmapped are the keys the mapping does not name (R3.3).
	Unmapped []string `json:"unmapped,omitempty"`
}

// Summarize reads the values the editor shows and edits.
func Summarize(root *Node, m *Mapping) (Summary, error) {
	ps, context, err := PlayerState(root, m)
	if err != nil {
		return Summary{}, err
	}
	var s Summary
	s.VersionInfo, _ = Version(root, m)
	s.Platform, _ = root.Member(m.Resolve(namePlatform)).String()
	s.StatePath = context
	common := root.Member(m.Resolve(nameCommonState))
	if common == nil {
		common = ps // before Omega the shared fields lived in the player state
	}
	s.SaveName, _ = common.Member(m.Resolve(nameSaveName)).String()
	s.PlayTime, _ = common.Member(m.Resolve(nameTotalPlayTime)).Int()
	s.Units, _ = ps.Member(m.Resolve(nameUnits)).Int()
	s.Nanites, _ = ps.Member(m.Resolve(nameNanites)).Int()
	s.Quicksilver, _ = ps.Member(m.Resolve(nameSpecials)).Int()
	s.Health, _ = ps.Member(m.Resolve(nameHealth)).Int()
	s.Shield, _ = ps.Member(m.Resolve(nameShield)).Int()
	s.Ships = ps.Member(m.Resolve(nameShipOwnership)).Len()
	s.Multitools = ps.Member(m.Resolve(nameMultitools)).Len()
	if s.SuitItems, err = Inventory(ps.Member(m.Resolve(nameInventory)), m); err != nil {
		return s, fmt.Errorf("%s: %w", nameInventory, err)
	}
	if s.SuitTech, err = Inventory(ps.Member(m.Resolve(nameInventoryTech)), m); err != nil {
		return s, fmt.Errorf("%s: %w", nameInventoryTech, err)
	}
	s.Unmapped = m.Unmapped(root)
	return s, nil
}

/*
Apply makes the changes in the tree and reports each one (R5.4).

Values that already hold what was asked are not reported and not rewritten, so
a form that submits every field changes only the fields the user touched. The
tree is edited in place; a caller doing a dry run simply does not write it.
*/
func Apply(root *Node, m *Mapping, c ChangeSet) ([]Change, error) {
	if c.Empty() {
		return nil, ErrNothingToDo
	}
	ps, context, err := PlayerState(root, m)
	if err != nil {
		return nil, err
	}
	prefix := context + "/"
	var changes []Change
	add := func(ch Change, ok bool) {
		if ok {
			changes = append(changes, ch)
		}
	}

	for _, cur := range []struct {
		name string
		want *uint64
	}{
		{nameUnits, c.Units}, {nameNanites, c.Nanites}, {nameSpecials, c.Quicksilver},
	} {
		if cur.want == nil {
			continue
		}
		if *cur.want > MaxCurrency {
			return nil, fmt.Errorf("%s %d is above the game's maximum %d", cur.name, *cur.want, uint64(MaxCurrency))
		}
		ch, ok, err := setInt(ps, m, prefix, cur.name, int64(*cur.want)) //nolint:gosec // bounded by MaxCurrency
		if err != nil {
			return nil, err
		}
		add(ch, ok)
	}
	for _, cur := range []struct {
		name string
		want *int64
	}{{nameHealth, c.Health}, {nameShield, c.Shield}} {
		if cur.want == nil {
			continue
		}
		if *cur.want < 0 {
			return nil, fmt.Errorf("%s cannot be negative", cur.name)
		}
		ch, ok, err := setInt(ps, m, prefix, cur.name, *cur.want)
		if err != nil {
			return nil, err
		}
		add(ch, ok)
	}
	for _, cur := range []struct {
		name, field string
		want        *int
	}{
		{nameInventory, "suit item slots", c.SuitItemSlots},
		{nameInventoryTech, "suit technology slots", c.SuitTechSlots},
	} {
		if cur.want == nil {
			continue
		}
		ch, ok, err := setSlotCount(ps.Member(m.Resolve(cur.name)), m, cur.field, prefix+cur.name, *cur.want)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", cur.name, err)
		}
		add(ch, ok)
	}
	return changes, nil
}

func setInt(ps *Node, m *Mapping, prefix, name string, want int64) (Change, bool, error) {
	node := ps.Member(m.Resolve(name))
	if node == nil {
		return Change{}, false, fmt.Errorf("player state has no %s", name)
	}
	old, ok := node.Int()
	if !ok {
		return Change{}, false, fmt.Errorf("%s is %s, not an integer", name, node.Raw())
	}
	if old == want {
		return Change{}, false, nil
	}
	node.SetInt(want)
	return Change{
		Field: name, Path: prefix + name,
		Old: strconv.FormatInt(old, 10), New: strconv.FormatInt(want, 10),
	}, true, nil
}

/*
setSlotCount unlocks or locks cells until the inventory has `want` of them.

Unlocking appends the missing cells in row-major order, which is the order the
game itself unlocks them in. Locking takes cells off the end of the list and
refuses to remove one that holds an item, naming the cell and the item: a
player who wants that cell gone can move the item first, and a tool that
silently deleted it would be a tool that deletes inventory.
*/
func setSlotCount(inv *Node, m *Mapping, field, path string, want int) (Change, bool, error) {
	sum, err := Inventory(inv, m)
	if err != nil {
		return Change{}, false, err
	}
	if want < 1 || want > sum.Max() {
		return Change{}, false, fmt.Errorf("%d slot(s) is outside 1..%d (the grid is %d×%d)",
			want, sum.Max(), sum.Width, sum.Height)
	}
	if want == sum.Valid {
		return Change{}, false, nil
	}
	xKey, yKey := m.Resolve(nameX), m.Resolve(nameY)
	valid := inv.Member(m.Resolve(nameValidSlotIndices))
	cell := func(n *Node) (int64, int64) {
		x, _ := n.Member(xKey).Int()
		y, _ := n.Member(yKey).Int()
		return x, y
	}

	if want > sum.Valid {
		have := map[[2]int64]bool{}
		for i := range valid.Len() {
			x, y := cell(valid.Index(i))
			have[[2]int64{x, y}] = true
		}
		for y := int64(0); y < int64(sum.Height) && valid.Len() < want; y++ {
			for x := int64(0); x < int64(sum.Width) && valid.Len() < want; x++ {
				if have[[2]int64{x, y}] {
					continue
				}
				c := NewObject()
				c.Set(xKey, NewInt(x))
				c.Set(yKey, NewInt(y))
				valid.Append(c)
			}
		}
	} else {
		occupied := map[[2]int64]string{}
		slots := inv.Member(m.Resolve(nameSlots))
		for i := range slots.Len() {
			s := slots.Index(i)
			x, y := cell(s.Member(m.Resolve(nameIndex)))
			id, _ := s.Member(m.Resolve(nameID)).String()
			occupied[[2]int64{x, y}] = id
		}
		for i := sum.Valid - 1; i >= want; i-- {
			x, y := cell(valid.Index(i))
			if id, taken := occupied[[2]int64{x, y}]; taken {
				return Change{}, false, fmt.Errorf("%w: cell (%d,%d) holds %s; move it first", ErrOccupied, x, y, id)
			}
		}
		valid.Truncate(want)
	}
	return Change{
		Field: field, Path: path + "/" + nameValidSlotIndices,
		Old: strconv.Itoa(sum.Valid), New: strconv.Itoa(want),
	}, true, nil
}
