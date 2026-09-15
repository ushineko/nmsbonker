package save

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

/*
Starship and freighter inventories (spec 011).

Every owned ship is an entry of PlayerStateData.ShipOwnership with the same
three inventories the exosuit has; PrimaryShip is the index of the one the
player is flying. The freighter's inventories sit directly on the player state
as FreighterInventory and FreighterInventory_TechOnly. Their grids grow as slots
are bought, up to the same ceiling as the exosuit's (edits.go), and the editor
grows them the same way. The class letter is separate from the grid: changing
it does not resize anything, which is also what the other editors do.
*/

// Names this file addresses in the mapping.
const (
	namePrimaryShip      = "PrimaryShip"
	nameResource         = "Resource"
	nameFilename         = "Filename"
	nameClass            = "Class"
	nameInventoryClass   = "InventoryClass"
	nameFreighterInv     = "FreighterInventory"
	nameFreighterTech    = "FreighterInventory_TechOnly"
	nameShipName         = "Name"
	nameInventoryLayout  = "InventoryLayout"
	nameInventoryCargo   = "Inventory_Cargo"
	nameFreighterCargo   = "FreighterInventory_Cargo"
	nameCurrentFreighter = "CurrentFreighter"
)

// ShipType is one kind of procedural starship: the in-game name and the scene
// file the game generates it from. The seed stays; the type decides which
// generator the seed feeds, so a type change gives a new ship of that kind.
type ShipType struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Scene is the Resource.Filename, upper-case as the save spells it.
	Scene string `json:"scene"`
}

// ShipTypes are the procedural starship kinds, from the scene files in the
// game's paks (models/common/spacecraft/*/*_proc.scene.mbin, 2026-09-15).
// Corvettes are modular, not procedural, and are not here.
//
//nolint:gochecknoglobals // a fixed table from game data
var ShipTypes = []ShipType{
	{Key: "fighter", Label: "Fighter", Scene: "MODELS/COMMON/SPACECRAFT/FIGHTERS/FIGHTER_PROC.SCENE.MBIN"},
	{Key: "hauler", Label: "Hauler", Scene: "MODELS/COMMON/SPACECRAFT/DROPSHIPS/DROPSHIP_PROC.SCENE.MBIN"},
	{Key: "explorer", Label: "Explorer", Scene: "MODELS/COMMON/SPACECRAFT/SCIENTIFIC/SCIENTIFIC_PROC.SCENE.MBIN"},
	{Key: "shuttle", Label: "Shuttle", Scene: "MODELS/COMMON/SPACECRAFT/SHUTTLE/SHUTTLE_PROC.SCENE.MBIN"},
	{Key: "exotic", Label: "Exotic", Scene: "MODELS/COMMON/SPACECRAFT/S-CLASS/S-CLASS_PROC.SCENE.MBIN"},
	{Key: "solar", Label: "Solar", Scene: "MODELS/COMMON/SPACECRAFT/SAILSHIP/SAILSHIP_PROC.SCENE.MBIN"},
	{Key: "living", Label: "Living Ship", Scene: "MODELS/COMMON/SPACECRAFT/S-CLASS/BIOPARTS/BIOSHIP_PROC.SCENE.MBIN"},
	{Key: "sentinel", Label: "Sentinel Interceptor", Scene: "MODELS/COMMON/SPACECRAFT/SENTINELSHIP/SENTINELSHIP_PROC.SCENE.MBIN"},
}

// FreighterTypes are the two player freighter kinds.
//
//nolint:gochecknoglobals // a fixed table from game data
var FreighterTypes = []ShipType{
	{Key: "regular", Label: "Freighter", Scene: "MODELS/COMMON/SPACECRAFT/INDUSTRIAL/FREIGHTER_PROC.SCENE.MBIN"},
	{Key: "capital", Label: "Capital Freighter", Scene: "MODELS/COMMON/SPACECRAFT/INDUSTRIAL/CAPITALFREIGHTER_PROC.SCENE.MBIN"},
}

// typeByKey finds a type in a table by key.
func typeByKey(table []ShipType, key string) (ShipType, bool) {
	for _, t := range table {
		if t.Key == key {
			return t, true
		}
	}
	return ShipType{}, false
}

// typeOfScene names the type a scene file belongs to, "" when it is none of
// the table's (a corvette, or a model this table predates).
func typeOfScene(table []ShipType, scene string) string {
	scene = strings.ToUpper(strings.ReplaceAll(scene, "\\", "/"))
	for _, t := range table {
		if t.Scene == scene {
			return t.Key
		}
	}
	return ""
}

// ShipSummary is one owned starship as the editor shows it.
type ShipSummary struct {
	Index int `json:"index"`
	// Name is the player's name for it, often empty; Kind is read from the
	// model file (Fighter, Hauler, …) so an unnamed ship is still tellable.
	Name string `json:"name,omitempty"`
	Kind string `json:"kind"`
	// Type is the ShipTypes key the model belongs to, "" when it is none.
	Type    string           `json:"type,omitempty"`
	Class   string           `json:"class"`
	Primary bool             `json:"primary"`
	Items   InventorySummary `json:"items"`
	Tech    InventorySummary `json:"tech"`
}

// Label is how a front end names the ship in a list.
func (s ShipSummary) Label() string {
	label := fmt.Sprintf("%d: %s", s.Index+1, s.Kind)
	if s.Name != "" {
		label += " “" + s.Name + "”"
	}
	if s.Class != "" {
		label += " (" + s.Class + ")"
	}
	if s.Primary {
		label += " — current"
	}
	return label
}

// FreighterSummary is the freighter's two inventories.
type FreighterSummary struct {
	Present bool   `json:"present"`
	Class   string `json:"class,omitempty"`
	// Type is the FreighterTypes key of CurrentFreighter, "" when unknown.
	Type  string           `json:"type,omitempty"`
	Items InventorySummary `json:"items"`
	Tech  InventorySummary `json:"tech"`
}

// Ships lists the owned starships in save order.
func Ships(ps *Node, m *Mapping) []ShipSummary {
	primary, _ := ps.Member(m.Resolve(namePrimaryShip)).Int()
	list := ps.Member(m.Resolve(nameShipOwnership))
	out := make([]ShipSummary, 0, list.Len())
	for i := range list.Len() {
		ship := list.Index(i)
		items, errI := Inventory(ship.Member(m.Resolve(nameInventory)), m)
		tech, errT := Inventory(ship.Member(m.Resolve(nameInventoryTech)), m)
		if errI != nil || errT != nil {
			continue // a slot the game keeps empty for a ship not yet owned
		}
		if file, _ := ship.Member(m.Resolve(nameResource)).Member(m.Resolve(nameFilename)).String(); file == "" {
			continue // an unowned hangar slot: no model, a 1×1 grid
		}
		s := ShipSummary{Index: i, Primary: int64(i) == primary, Items: items, Tech: tech}
		s.Name, _ = ship.Member(m.Resolve(nameShipName)).String()
		file, _ := ship.Member(m.Resolve(nameResource)).Member(m.Resolve(nameFilename)).String()
		s.Type = typeOfScene(ShipTypes, file)
		s.Kind = shipKind(ship, m)
		if t, ok := typeByKey(ShipTypes, s.Type); ok {
			s.Kind = t.Label
		}
		s.Class = inventoryClass(ship.Member(m.Resolve(nameInventory)), m)
		out = append(out, s)
	}
	return out
}

// Freighter reads the freighter's inventories; Present is false for a save
// with none.
func Freighter(ps *Node, m *Mapping) FreighterSummary {
	items, errI := Inventory(ps.Member(m.Resolve(nameFreighterInv)), m)
	tech, errT := Inventory(ps.Member(m.Resolve(nameFreighterTech)), m)
	if errI != nil || errT != nil {
		return FreighterSummary{}
	}
	file, _ := ps.Member(m.Resolve(nameCurrentFreighter)).Member(m.Resolve(nameFilename)).String()
	return FreighterSummary{
		Present: items.Max() > 0, Items: items, Tech: tech,
		Class: inventoryClass(ps.Member(m.Resolve(nameFreighterInv)), m),
		Type:  typeOfScene(FreighterTypes, file),
	}
}

// shipKind turns the model path into a word: FIGHTER_PROC.SCENE.MBIN → Fighter.
func shipKind(ship *Node, m *Mapping) string {
	file, _ := ship.Member(m.Resolve(nameResource)).Member(m.Resolve(nameFilename)).String()
	base := path.Base(strings.ReplaceAll(file, "\\", "/"))
	base = strings.TrimSuffix(strings.ToUpper(base), ".SCENE.MBIN")
	base = strings.TrimSuffix(base, "_PROC")
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if base == "" {
		return "Ship"
	}
	return strings.ToUpper(base[:1]) + strings.ToLower(base[1:])
}

func inventoryClass(inv *Node, m *Mapping) string {
	c, _ := inv.Member(m.Resolve(nameClass)).Member(m.Resolve(nameInventoryClass)).String()
	return c
}

// shipInventory finds one inventory of one ship, by index (or the primary
// ship for -1) and inventory name.
func shipInventory(ps *Node, m *Mapping, index int, inv string) (*Node, int, error) {
	list := ps.Member(m.Resolve(nameShipOwnership))
	if index < 0 {
		p, ok := ps.Member(m.Resolve(namePrimaryShip)).Int()
		if !ok {
			return nil, 0, fmt.Errorf("the save names no %s", namePrimaryShip)
		}
		index = int(p)
	}
	ship := list.Index(index)
	if ship == nil {
		return nil, 0, fmt.Errorf("no ship at index %d (the save has %d)", index, list.Len())
	}
	node := ship.Member(m.Resolve(inv))
	if node == nil {
		return nil, 0, fmt.Errorf("ship %d has no %s", index, inv)
	}
	return node, index, nil
}

// Classes are the inventory classes the game uses, worst to best.
//
//nolint:gochecknoglobals // a fixed table
var Classes = []string{"C", "B", "A", "S"}

// ValidClass says whether a class letter is one of the four.
func ValidClass(c string) bool {
	for _, k := range Classes {
		if k == c {
			return true
		}
	}
	return false
}

/*
setClass writes a class letter into every inventory of a ship or freighter.

The class lives on each of the three inventories (items, technology, cargo)
as Class.InventoryClass, and the game reads the item inventory's for the
badge; all three are set so they agree. The class is what the game shows and
what gates a class-locked upgrade. It is not the stat bonuses: those are the
BaseStatValues rolled when the ship was generated and are left alone, so an
S-class made this way flies with the numbers it had.
*/
func setClass(owner *Node, m *Mapping, field, prefix string, want string) (Change, bool, error) {
	if !ValidClass(want) {
		return Change{}, false, fmt.Errorf("%s %q is not one of C, B, A, S", field, want)
	}
	var old string
	changed := false
	for _, inv := range []string{nameInventory, nameInventoryTech, nameInventoryCargo} {
		node := owner.Member(m.Resolve(inv))
		if node == nil {
			continue
		}
		class := node.Member(m.Resolve(nameClass))
		if class == nil || class.Type() != TypeObject {
			continue
		}
		cur, _ := class.Member(m.Resolve(nameInventoryClass)).String()
		if inv == nameInventory {
			old = cur
		}
		if cur != want {
			class.Set(m.Resolve(nameInventoryClass), NewString(want))
			changed = true
		}
	}
	if old == "" && !changed {
		return Change{}, false, fmt.Errorf("%s: no inventory carries a class", field)
	}
	if !changed {
		return Change{}, false, nil
	}
	return Change{Field: field, Path: prefix + "/Class", Old: old, New: want}, true, nil
}

// setFreighterClass is setClass for the freighter, whose inventories sit on
// the player state under their own names.
func setFreighterClass(ps *Node, m *Mapping, prefix, want string) (Change, bool, error) {
	if !ValidClass(want) {
		return Change{}, false, fmt.Errorf("freighter class %q is not one of C, B, A, S", want)
	}
	var old string
	changed := false
	for _, inv := range []string{nameFreighterInv, nameFreighterTech, nameFreighterCargo} {
		node := ps.Member(m.Resolve(inv))
		if node == nil {
			continue
		}
		class := node.Member(m.Resolve(nameClass))
		if class == nil || class.Type() != TypeObject {
			continue
		}
		cur, _ := class.Member(m.Resolve(nameInventoryClass)).String()
		if inv == nameFreighterInv {
			old = cur
		}
		if cur != want {
			class.Set(m.Resolve(nameInventoryClass), NewString(want))
			changed = true
		}
	}
	if old == "" && !changed {
		return Change{}, false, fmt.Errorf("the save has no freighter inventory; no freighter is owned")
	}
	if !changed {
		return Change{}, false, nil
	}
	return Change{Field: "freighter class", Path: prefix + nameFreighterInv + "/Class", Old: old, New: want}, true, nil
}

// setScene points a Resource at another type's generator, keeping the seed.
func setScene(resource *Node, m *Mapping, table []ShipType, field, path, want string) (Change, bool, error) {
	t, ok := typeByKey(table, want)
	if !ok {
		keys := make([]string, 0, len(table))
		for _, k := range table {
			keys = append(keys, k.Key)
		}
		return Change{}, false, fmt.Errorf("%s %q is not one of %s", field, want, strings.Join(keys, ", "))
	}
	if resource == nil || resource.Type() != TypeObject {
		return Change{}, false, fmt.Errorf("%s: no Resource to change", field)
	}
	file := resource.Member(m.Resolve(nameFilename))
	if file == nil {
		return Change{}, false, fmt.Errorf("%s: the Resource has no Filename", field)
	}
	old, _ := file.String()
	if strings.EqualFold(old, t.Scene) {
		return Change{}, false, nil
	}
	oldKey := typeOfScene(table, old)
	if oldKey == "" {
		oldKey = old
	}
	file.SetString(t.Scene)
	return Change{Field: field, Path: path + "/" + nameFilename, Old: oldKey, New: t.Key}, true, nil
}

// applyShipAndFreighter is the ship and freighter half of Apply.
func applyShipAndFreighter(ps *Node, m *Mapping, prefix string, c ChangeSet, add func(Change, bool)) error {
	for _, cur := range []struct {
		inv, field string
		want       *int
		ceiling    GridCeiling
	}{
		{nameInventory, "ship item slots", c.ShipItemSlots, CeilingItems},
		{nameInventoryTech, "ship technology slots", c.ShipTechSlots, CeilingTech},
	} {
		if cur.want == nil {
			continue
		}
		node, index, err := shipInventory(ps, m, c.Ship, cur.inv)
		if err != nil {
			return err
		}
		ch, ok, err := setSlotCount(node, m, cur.field,
			prefix+nameShipOwnership+"/"+strconv.Itoa(index)+"/"+cur.inv, *cur.want, cur.ceiling)
		if err != nil {
			return fmt.Errorf("ship %d %s: %w", index, cur.inv, err)
		}
		add(ch, ok)
	}
	if c.ShipType != nil {
		list := ps.Member(m.Resolve(nameShipOwnership))
		index := c.Ship
		if index < 0 {
			p, ok := ps.Member(m.Resolve(namePrimaryShip)).Int()
			if !ok {
				return fmt.Errorf("the save names no %s", namePrimaryShip)
			}
			index = int(p)
		}
		ship := list.Index(index)
		if ship == nil {
			return fmt.Errorf("no ship at index %d (the save has %d)", index, list.Len())
		}
		ch, ok, err := setScene(ship.Member(m.Resolve(nameResource)), m, ShipTypes, "ship type",
			prefix+nameShipOwnership+"/"+strconv.Itoa(index)+"/"+nameResource, *c.ShipType)
		if err != nil {
			return err
		}
		add(ch, ok)
	}
	if c.FreighterType != nil {
		res := ps.Member(m.Resolve(nameCurrentFreighter))
		if res == nil {
			return fmt.Errorf("the save has no %s; no freighter is owned", nameCurrentFreighter)
		}
		ch, ok, err := setScene(res, m, FreighterTypes, "freighter type", prefix+nameCurrentFreighter, *c.FreighterType)
		if err != nil {
			return err
		}
		add(ch, ok)
	}
	if c.ShipClass != nil {
		list := ps.Member(m.Resolve(nameShipOwnership))
		index := c.Ship
		if index < 0 {
			p, ok := ps.Member(m.Resolve(namePrimaryShip)).Int()
			if !ok {
				return fmt.Errorf("the save names no %s", namePrimaryShip)
			}
			index = int(p)
		}
		ship := list.Index(index)
		if ship == nil {
			return fmt.Errorf("no ship at index %d (the save has %d)", index, list.Len())
		}
		ch, ok, err := setClass(ship, m, "ship class", prefix+nameShipOwnership+"/"+strconv.Itoa(index), *c.ShipClass)
		if err != nil {
			return err
		}
		add(ch, ok)
	}
	if c.FreighterClass != nil {
		ch, ok, err := setFreighterClass(ps, m, prefix, *c.FreighterClass)
		if err != nil {
			return err
		}
		add(ch, ok)
	}
	for _, cur := range []struct {
		inv, field string
		want       *int
		ceiling    GridCeiling
	}{
		{nameFreighterInv, "freighter item slots", c.FreighterItemSlots, CeilingItems},
		{nameFreighterTech, "freighter technology slots", c.FreighterTechSlots, CeilingTech},
	} {
		if cur.want == nil {
			continue
		}
		node := ps.Member(m.Resolve(cur.inv))
		if node == nil {
			return fmt.Errorf("the save has no %s; no freighter is owned", cur.inv)
		}
		ch, ok, err := setSlotCount(node, m, cur.field, prefix+cur.inv, *cur.want, cur.ceiling)
		if err != nil {
			return fmt.Errorf("%s: %w", cur.inv, err)
		}
		add(ch, ok)
	}
	return nil
}
