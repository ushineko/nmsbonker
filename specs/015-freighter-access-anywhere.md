# Spec 015: Freighter access anywhere

**Issue**: #17

## Status: COMPLETE

## Executive Summary

A new switch-only built-in, `FreighterAccessAnywhere` (Ships page), makes every
freighter behave as if the Matter Beam were installed. It adds the Matter Beam's
only stat bonus to the core freighter hyperdrive. Because the tweak adds an
entry, spec 014 R1.3 ("no built-in adds or removes entries") is retired.
Reviewers should look first at the script's anchor (two keywords plus
`SECTION_UP`), and at the integration test, which now accepts `WORKING*`.

## Context

The freighter inventory can be reached remotely only once the Matter Beam
(`F_TELEPORT`) is built and installed on the freighter. That costs materials and
a technology slot. The request (#17) is an option that behaves as if the beam
were installed, without installing it in game.

In `NMS_REALITY_GCTECHNOLOGYTABLE.MBIN` the Matter Beam does one thing: it grants
the stat `Freighter_Teleport` with a bonus of 100. No global setting covers
remote freighter access. Every freighter carries the core hyperdrive
`F_HYPERDRIVE`, which cannot be removed. If the executable reads the stat rather
than looking for the item, a `Freighter_Teleport` bonus on the hyperdrive grants
the effect.

## Requirements

- R1 Built-in `FreighterAccessAnywhere`, group Ships, no parameters (a switch).
  It adds one `GcStatsBonus` (`Freighter_Teleport`, 100, level 1) to
  `F_HYPERDRIVE`'s `StatBonuses`, after its last existing bonus.
- R2 Like every built-in (spec 014 R1.1), it starts disabled.
- R3 Spec 014 R1.3 is retired: a built-in may carry `ADD` or `REMOVE`. The
  recompile gate still applies. A dropped structural edit shows as `PARTIAL` in
  the build report, and the integration test fails on it.

## Acceptance Criteria

- [x] AC1 `tweaks.Names()` lists 27 built-ins, `FreighterAccessAnywhere` among
  them on the Ships page, disabled on an existing configuration
  (`internal/tweaks/tweaks_test.go`, `internal/core/mods_test.go`).
- [x] AC2 Built against the installed game with the real compiler, the tweak
  reports `WORKING*` with nothing skipped and nothing dropped
  (`TestEveryBuiltInFindsEveryKeyInTheInstalledGame`, game buildid of
  2026-10-03).
- [x] AC3 The compiled MBIN, decompiled again, shows `F_HYPERDRIVE` with its four
  stock bonuses unchanged and a fifth, `Freighter_Teleport` 100.
- [x] AC4 In game, with no Matter Beam installed: the freighter reports "item
  teleportation enabled", and the freighter inventory is reachable while the
  freighter is in another star system. Recorded by the user, 2026-10-03.

## Risks & Assumptions

- The anchor is `F_HYPERDRIVE` then `Freighter_Fleet_Boost`, its last bonus. If
  an update reorders or removes that bonus, the keyword misses and the build
  reports it. The integration test catches that before a release.
- Rollback: disable the tweak and rebuild, or `nmsbonker rollback`. The save is
  untouched, so removing the tweak returns the freighter to stock.
- Storage containers on the freighter were not part of the in-game check.

## E2E Test Plan

1. Enable the tweak, build, deploy.
2. In game, on a save whose freighter lacks the Matter Beam, open the freighter
   tech inventory: "item teleportation enabled" is shown (AC4).
3. Warp to a system without the freighter. The freighter inventory is still
   reachable from the exosuit inventory screen (AC4).

Result, 2026-10-03: both observed by the user on a dev build of this branch.
