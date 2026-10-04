# Spec 016: Fleet takes no damage

**Issue**: #19

## Status: INCOMPLETE

## Context

A frigate can come back from an expedition damaged. Each damaged frigate has to
be repaired by teleporting to it and finding the broken part, one ship at a
time. The request (#19) is an option that turns expedition damage off.

Two values in the game data decide whether a frigate is damaged:

- `GCFLEETGLOBALS.GLOBAL.MBIN`, `PercentChanceOfDamageOnFailedEvent`: a range,
  stock `X` 0 and `Y` 20 (percent). The chance climbs through it as a frigate
  goes on more expeditions (`LowDamageNumberOfExpeditions` 3,
  `RampDamageNumberOfExpeditions` 10). It applies to an ordinary failed event.
- `METADATA/REALITY/TABLES/EXPEDITIONEVENTTABLE.MBIN`, `FailureDamageChance`
  on each of 58 intervention events: 35 at 0, one at 10, two at 25, three at 50,
  17 at 100 (game buildid of 2026-10-03).

## Requirements

- R1 Built-in `FleetNoDamage`, group Ships, no parameters (a switch), placed
  after `FleetExpeditionTime` in build order.
- R2 It sets both ends of `PercentChanceOfDamageOnFailedEvent` to 0, and every
  `FailureDamageChance` in the expedition event table to 0. It adds and removes
  nothing.
- R3 Like every built-in (spec 014 R1.1), it starts disabled.

## Acceptance Criteria

- [x] AC1 `tweaks.Names()` lists 28 built-ins, `FleetNoDamage` among them on
  the Ships page, disabled on an existing configuration
  (`internal/tweaks/tweaks_test.go`, `internal/core/mods_test.go`).
- [x] AC2 Built against the installed game with the real compiler, the tweak
  reports `WORKING` with nothing skipped and nothing dropped
  (`TestEveryBuiltInFindsEveryKeyInTheInstalledGame`, game buildid of
  2026-10-03).
- [x] AC3 The compiled MBINs, decompiled again, show
  `PercentChanceOfDamageOnFailedEvent` at 0 and 0, every `FailureDamageChance`
  at 0, and no other value changed. Stock `X` is already 0, so the diff is
  `Y` 20 to 0 and 23 `FailureDamageChance` entries to 0.
- [ ] AC4 In game, several expeditions return with failed events and no frigate
  damaged. Recorded by the user.

## Risks & Assumptions

- The executable is assumed to read only these two values when deciding damage.
  The data shows no third source; AC4 is the check.
- Frigates already damaged stay damaged; the tweak changes the roll, not the
  save.
- `PRECEDING_KEY_WORDS` anchors on `PercentChanceOfDamageOnFailedEvent`, whose
  first children are `X` and `Y`. If an update renames it, the keyword misses
  and the build report says so; the integration test catches it before a
  release.
- Rollback: disable the tweak and rebuild, or `nmsbonker rollback`.

## E2E Test Plan

1. Enable the tweak, build, deploy.
2. In game, send several frigates on expeditions; debriefs with failed events
   show no damaged frigates (AC4).
