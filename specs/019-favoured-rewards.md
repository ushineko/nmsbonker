# Spec 019: Favoured rewards

**Issue**: #22

## Status: COMPLETE

## Executive Summary

A new built-in, `FavouredRewards` (Rewards page), multiplies the reward-list
weight of nine commonly farmed items wherever they appear in the reward and
expedition reward tables, with one amount multiplier for all of them.
Reviewers should look first at the anchor (the whole ID line as one keyword,
R5) and why the `{"ID", id}` pair was not used.

## Context

Mission board (`R_MB_*`), Nexus (`R_NEXUS_*`) and frigate expedition
(`EXPEDITIONREWARDTABLE`) rewards are mostly `SelectAlways` lists: one item is
picked and each item's `PercentageChance` is a weight against the rest
(`R_MB_MEGA`'s weights add up to 159). Items people farm sit low: a Storage
Augmentation is about 4% of a mission board reward, an S-Class Reactor 0.6%.
In `GiveAll` lists the number is the item's own chance.

Tweak parameters are numbers, so "any item" would need text parameters; this
spec takes a fixed list of common farm targets instead.

## Requirements

- R1 Built-in `FavouredRewards`, group Rewards, after `MissionBoardRewards`.
- R2 One weight multiplier per target (1–100, default 5): Storage Augmentation
  (`SHIP_INV_TOKEN`), Multi-tool Expansion Slot (`WEAP_INV_TOKEN`), Exosuit
  Expansion Unit (`SUIT_INV_TOKEN`), Spawning Sac (`ALIEN_INV_TOKEN`), Salvaged
  Frigate Module (`FRIG_TOKEN`), S- and A-Class Reactor (`SHIP_CORE_S`,
  `SHIP_CORE_A`), Fusion Ignitor and Stasis Device (`ULTRAPROD1`,
  `ULTRAPROD2`), Storm Crystal (`STORM_CRYSTAL`). Names from the game's English
  text.
- R3 `AMOUNT_MULT` (1–100, default 1, `scales="product"`) multiplies the same
  items' AmountMin and AmountMax.
- R4 Every reward-table item whose `ID` is a target, in `REWARDTABLE` and
  `EXPEDITIONREWARDTABLE`, has its `PercentageChance` multiplied by its weight.
  No other line changes. Bundles (`GcMultiSpecificItemEntry`) are not touched.
- R5 The anchor is one keyword holding the whole ID line. The ALL path searches
  each keyword from the line after the previous hit, so the pair `{"ID", id}`
  misses every entry whose ID line is itself the first `"ID"` hit (observed:
  ship salvage entries alternated changed/unchanged). The engine is unchanged
  (golden parity).

## Acceptance Criteria

- [x] AC1 `tweaks.Names()` lists 30 built-ins with `FavouredRewards` on the
  Rewards page (`internal/tweaks/tweaks_test.go`, `internal/core/mods_test.go`).
- [x] AC2 Built against the installed game with the real compiler, it reports
  `WORKING` with nothing skipped (`TestEveryBuiltInFindsEveryKeyInTheInstalledGame`).
- [x] AC3 Decompiled output, game buildid 25625620, `AMOUNT_MULT` 2: every
  target occurrence edited (REWARDTABLE 37/37, 29/29, 14/14, 17/17, 7/7, 7/7,
  1/1, 1/1, 20/20; EXPEDITIONREWARDTABLE 4/4, 6/6, 4/4) and every changed line
  is a target's PercentageChance, AmountMin or AmountMax.
- [x] AC4 In game: a favoured item comes up noticeably more often from mission
  board rewards. Recorded by the user, 2026-10-04.

## Risks & Assumptions

- Weights apply wherever the item is in a list, not only mission board, Nexus
  and expeditions (e.g. ship salvage, which already gives a Storage
  Augmentation at 20–100%). That is the point of farming; the description
  says "wherever a reward is picked from a list".
- In a `GiveAll` list a chance above 100 is assumed to mean certain.
- A higher weight for one item lowers the others' odds in the same list.
- Rollback: disable and rebuild.

## E2E Test Plan

1. Enable with Storage Augmentation weight 20, build, deploy.
2. Complete several mission board missions; Storage Augmentations come up far
   more often than one in twenty-five rewards (AC4).
