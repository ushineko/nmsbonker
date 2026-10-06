# Spec 020: Favoured rewards reaches Nexus missions and every expedition

**Issue**: #27

## Status: INCOMPLETE

## Context

Spec 019's Favoured rewards multiplies an item's weight wherever the item is
listed. In game it changed mission board rewards and nothing else, because the
lists Nexus missions and frigate expeditions pay from mostly do not list the
favoured items:

- All 27 Nexus missions (`multiplayermissiontable`) pay from `R_NEXUS_MED`,
  which of the nine holds only the Salvaged Frigate Module. `R_NEXUS_MEGA`,
  which holds most of them, is not referenced by any mission file.
- Each of the 15 expedition events (`EXPEDITIONEVENTTABLE`) pays its success
  from its own list. Only `R_DIPLOMATIC_0` and `R_MINING_1`..`3` held any
  favoured item (Storage Augmentation, Frigate Module); combat and exploration
  held none, and Multi-tool, Exosuit, reactors, Fusion Ignitor, Stasis Device
  and Storm Crystal were absent from the table.

## Requirements

- R1 Each favoured item missing from `R_NEXUS_MED` or from one of the 15
  expedition success lists (`R_DIPLOMATIC_0`..`3`, `R_COMBAT_0`..`2`,
  `R_EXPLORATION_0`..`3`, `R_MINING_0`..`3`) is added to it as a
  `GcRewardSpecificProduct` item, amount 1 (Storm Crystal 10). Units-only
  lists are included (operator decision, 2026-10-06).
- R2 An added item's base weight is its typical share times that list's stock
  weight total: Storage Augmentation and Frigate Module 6.8%, Multi-tool 4.4%,
  Exosuit 3%, Spawning Sac 1%, A-Class Reactor 1.3%, S-Class 0.6%, Fusion
  Ignitor and Stasis Device 0.3% each, Storm Crystal 0.8%. At slider 1 each
  added item has about that chance in its list.
- R3 The adds run before the weighting, so the sliders and `AMOUNT_MULT`
  apply to added items as to listed ones.
- R4 The per-list missing items and base weights are a static table in the
  script, generated from the stock tables (game build 25625620).
- R5 Tabs and newlines in the added XML are built with `string.char`: the
  loader doubles backslashes before Lua runs, so `"\t"` arrives as two
  characters (the first attempt put the whole ADD on one line, and the
  weighting then ran away to int32 maximum).

## Acceptance Criteria

- [x] AC1 Built against the installed game with the real compiler:
  `WORKING*`, nothing skipped, nothing dropped
  (`TestEveryBuiltInFindsEveryKeyInTheInstalledGame`).
- [x] AC2 Decompiled at defaults (x5): every expedition success list and
  `R_NEXUS_MED` gives a Storage Augmentation about 14–16% of the time (e.g.
  `R_COMBAT_0` 15.0%, `R_NEXUS_MED` 16.4%); stock lines changed are only
  target `PercentageChance` values; 9 items inserted in `REWARDTABLE`, 144 in
  `EXPEDITIONREWARDTABLE`.
- [ ] AC3 In game: Nexus missions and combat or exploration expeditions give
  favoured items. Recorded by the user.

## Risks & Assumptions

- Units-only expedition lists now sometimes pay an item instead of units, more
  often at higher slider values.
- `R_NEXUS_MEGA` stays weighted but is not known to be used.
- The static table drifts if a game update adds a favoured item to one of
  these lists; the result is a duplicate entry, which the game tolerates in a
  SelectAlways list. The integration test still requires `WORKING*`.
- Rollback: disable the tweak and rebuild.

## E2E Test Plan

1. Rebuild and deploy with Favoured rewards enabled.
2. Complete Nexus missions and run combat and exploration expeditions; favoured
   items appear among their rewards (AC3).
