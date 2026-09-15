# Spec 011 — Starship and freighter inventories and class

## Status: COMPLETE

## Context

The save editor unlocked exosuit slots (spec 007). Ships and the freighter
carry the same three inventories, and their class letter, and a player who
wants a full S-class hauler today either grinds or uses another editor. This
adds both to the typed editor.

Measured on the user's save (2026-09-15): `PlayerStateData.ShipOwnership` is a
list of ships each with `Name`, `Resource.Filename` (the model, e.g.
`FIGHTER_PROC.SCENE.MBIN`), `Inventory`, `Inventory_TechOnly`,
`Inventory_Cargo`; `PrimaryShip` is the index being flown. An A-class fighter's
grids were 10×6 items and 10×3 technology; the freighter's (`FreighterInventory`,
`FreighterInventory_TechOnly`, `FreighterInventory_Cargo`) were 7×5 and 7×3.
The class letter is `Class.InventoryClass` on each inventory.

## Requirements

- R1 `save.Summary` gains `ShipList` (index, name, kind from the model file,
  class, primary flag, item and technology inventory summaries; empty
  ownership entries skipped) and `Freighter` (present, class, the two
  inventories).
- R2 `save.ChangeSet` gains `Ship` (index; -1 is the primary), `ShipItemSlots`,
  `ShipTechSlots`, `ShipClass`, `FreighterItemSlots`, `FreighterTechSlots`,
  `FreighterClass`. Slot counts unlock cells up to the game's ceiling for the
  inventory, growing the grid a row at a time when the count needs more than
  the save's grid holds, as the game does when slots are bought (see R5). A
  class is one of C, B, A, S and is written to all three inventories of the
  owner; the stat bonuses (`BaseStatValues`) are untouched, and the grid is
  not changed by a class change (the game does not tie the two, and neither
  does goatfungus's editor, which offers a separate resize).
- R6 Type: `ShipType` and `FreighterType` (keys from `save.ShipTypes`,
  `save.FreighterTypes`, the procedural scene files in the paks: fighter,
  hauler, explorer, shuttle, exotic, solar, living, sentinel; regular and
  capital freighter) rewrite `Resource.Filename` (`CurrentFreighter.Filename`)
  and keep the seed, so the game regenerates the craft as that kind. CLI
  `--ship-type`, `--freighter-type`; GUI drop-downs. Corvettes are modular,
  not procedural, and are not offered.
- R5 The ceiling comes from `metadata/reality/tables/inventorytable.mbin`
  (game buildid 25320008, read 2026-09-15): every ship, freighter and exosuit
  size type carries the same `Bounds` `MaxWidthLarge`/`MaxHeightLarge` of
  10×12 for items and 10×6 for technology. `save.CeilingItems` and
  `save.CeilingTech` hold those; the exosuit edit uses them too, replacing
  spec 007's "never grow the grid" rule, which measured the save's grid as if
  it were the ceiling (an A-class fighter with 59 slots bought had grown to
  10×6, so the grid in the save is the grid so far).
- R3 CLI: `saves inspect` lists the ships and the freighter; `saves edit`
  takes `--ship I`, `--ship-slots`, `--ship-tech-slots`, `--ship-class`,
  `--freighter-slots`, `--freighter-tech-slots`, `--freighter-class`.
- R4 GUI: a Starship group with a ship drop-down (the current ship selected),
  class and two slot fields; a Freighter group when one is owned.

## Acceptance Criteria

- [x] AC1 On a synthetic save with two ships and a freighter: the summary
  names kinds, classes and the primary; edits on the primary ship and the
  freighter change six things; the other ship is untouched; the class lands on
  all three inventories; a ship by index; grid bounds, a missing index, a bad
  class letter and an unchanged class behave as specified.
- [x] AC2 `saves inspect 9` on the real profile lists the ships with their
  grids; a class dry run lists one change and writes nothing.
- [x] AC3 In game: a ship set to S shows the S badge and its unlocked slots;
  the freighter likewise. Recorded by the user.

## Risks & Assumptions

- The ceiling is read from the current game's table and held as a constant.
  A future table with a larger ceiling makes the editor conservative, not
  wrong; a smaller one would need this constant lowered. Above-ceiling counts
  are refused, so the "game might break" case stays unreachable.
- A class change does not resize; a slot count does. Whether the game accepts
  a grid grown here exactly as it accepts one grown by purchase is for AC3 to
  observe.
- Same write path and guards as spec 007.

## Status notes
### In-game check (2026-09-15, game buildid 25320008)

The user ran the installed build against slot 9 and reported "working".
Recorded as the open criterion's result on that word.

## Executive Summary

Ships and the freighter join the typed editor: slot counts within their grids
and the class letter, from the CLI and the window. `internal/save/ships.go`.
