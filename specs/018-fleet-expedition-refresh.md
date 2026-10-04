# Spec 018: Fleet expedition refresh

**Issue**: #21

## Status: INCOMPLETE

## Context

Frigate expeditions refresh once per real-world day. The save keeps
`LastKnownDay` (days since 1970-01-01; 20730 on 2026-10-04) and
`ExpeditionSeedsSelectedToday` (the seeds already picked). `GCFLEETGLOBALS`
carries `OverrideExpeditionSecondsPerDay` (-1, off, in the shipped file) and
`NumberOfExpeditionChoices` (5). The override looks like a developer setting;
whether the released executable reads it is unknown, and only the game can say.

Resetting through the save (clearing the picked seeds, or winding the day back)
was considered and not pursued: the seeds are derived from the day, so it would
most likely offer the same expeditions again, and it only works with the game
closed.

## Requirements

- R1 Built-in `FleetExpeditionRefresh`, group Ships, after `FleetNoDamage`,
  marked experimental in its description.
- R2 `MINUTES_PER_DAY` (1–1440, default 10) sets
  `OverrideExpeditionSecondsPerDay` to that many minutes in seconds.
- R3 `CHOICES` (1–10, default 5, the stock value) sets
  `NumberOfExpeditionChoices`.
- R4 Value edits only; starts disabled like every built-in.

## Acceptance Criteria

- [x] AC1 `tweaks.Names()` lists 29 built-ins with `FleetExpeditionRefresh` on
  the Ships page (`internal/tweaks/tweaks_test.go`,
  `internal/core/mods_test.go`).
- [x] AC2 Built against the installed game with the real compiler, it reports
  `WORKING` with nothing skipped (`TestEveryBuiltInFindsEveryKeyInTheInstalledGame`).
- [x] AC3 The compiled MBIN shows `OverrideExpeditionSecondsPerDay` 600 and
  `NumberOfExpeditionChoices` 5 at the defaults, nothing else changed.
- [ ] AC4 In game: after the expeditions offered are used up, new ones appear
  within the configured minutes. Recorded by the user.
- [ ] AC5 In game: `CHOICES` above 5 offers that many expeditions. Recorded by
  the user.

## Risks & Assumptions

- If the executable ignores the override, AC4 fails and the tweak is reduced to
  `CHOICES` or withdrawn; the spec records which.
- A short day may also move other day-keyed fleet behaviour; nothing else in
  `GCFLEETGLOBALS` names the override, but the in-game check watches for it.
- The board's layout may not show more than five; AC5 checks.
- Rollback: disable and rebuild. The save is not edited.

## E2E Test Plan

1. Enable the tweak (defaults), build, deploy, start the game.
2. Use up the offered expeditions; wait ten minutes of play; check the board
   (AC4).
3. Set `CHOICES` to 8, rebuild and deploy; check how many are offered (AC5).
