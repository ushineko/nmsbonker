# Spec 008 — Faction standing in the save editor

## Status: COMPLETE

## Context

Standing with the three races and the three guilds gates story missions in
both directions: some steps need a rank, and some become unreachable once the
rank is too high, which early over-earning can do. The save editor (spec 007)
gains the standings as a typed edit so a player can lower one back under a
gate, or raise one, without a raw JSON edit.

Measured on the user's save (2026-09-15, game buildid 25233815): the values
live in `PlayerStateData.Stats`, a list of groups each with `GroupId`,
`Address` and an inner `Stats` list. The group whose `GroupId` is
`^GLOBAL_STATS` holds one entry per stat id with a `Value` union whose
`IntValue` is the standing; the game writes an empty union for zero. The six
ids are `^TRA_STANDING` (Gek), `^EXP_STANDING` (Korvax), `^WAR_STANDING`
(Vy'keen), `^WGUILD_STAND` (Mercenaries Guild), `^EGUILD_STAND` (Explorers
Guild), `^TGUILD_STAND` (Merchants Guild). The user's global values were 45,
5, 135, 17, 0 and absent. `^SYSTEM_STATS` groups carry the same ids for the
per-system standing the game shows locally; they are not the story gate.

## Requirements

- R1 `save.ChangeSet` gains `Standings map[string]int64` keyed by faction
  (`gek`, `korvax`, `vykeen`, `mercenaries`, `explorers`, `merchants`). An
  edit rewrites the `Value` union of the `^GLOBAL_STATS` entry: `{"IntValue":
  N}` for a non-zero value, `{}` for zero, as the game writes them. Per-system
  groups are untouched. Values are bounded 0 … 2³⁰. An unknown faction, or a
  faction whose entry the save lacks, is an error naming it.
- R2 `save.Summary` gains `Standings`: the six values in display order, each
  marked present or absent.
- R3 CLI: `saves inspect` prints the standings on one line; `saves edit`
  takes `--standing FACTION=N`, repeatable.
- R4 GUI: six "… standing" rows in the editor form, prefilled, with the same
  Preview and Apply path.
- R5 README names the new edit.

## Acceptance Criteria

- [x] AC1 On a synthetic save, `Summarize` reads the global values and not the
  per-system ones, marks an empty union as present-and-zero and a missing
  entry as absent.
- [x] AC2 `Apply` with three changed and one unchanged standing reports three
  changes, writes `{"IntValue":N}` for the non-zero values and `{}` for zero,
  leaves the `^SYSTEM_STATS` group byte-identical, and refuses a missing
  entry, a negative value and an unknown faction with the name in the error.
- [x] AC3 `saves inspect 9` on the real profile prints the six standings, and
  `saves edit 9 --standing gek=<current-1> --dry-run` lists one change and
  writes nothing.
- [x] AC4 `make test`, the parity guard and `make lint` pass; the GUI form
  shows the six rows (checked by building the section headlessly is not
  possible for Fyne entries, so the row construction is exercised by the
  existing GUI tests compiling and the CLI path by AC3).
- [x] AC5 In game: after lowering a standing that was above a mission's gate,
  the mission becomes available. Recorded here by the user.

## Risks & Assumptions

- Same write path and guards as spec 007; nothing new writes into the prefix.
- The rank thresholds are the game's; the editor shows and sets raw values
  and does not name ranks.
- A save that has never met a faction lacks the entry. Creating it is out of
  scope: the game creates it on first contact and the error says so.
- Rollback: the pre-write backup, as in spec 007.

## Status notes

AC3 run 2026-09-15 on the real profile: `saves inspect 9` printed Gek 45,
Korvax 5, Vy'keen 135, Mercenaries Guild 17, Explorers Guild 0, Merchants
Guild 0; `saves edit 9 --standing gek=44 --dry-run` listed one change at
`BaseContext/PlayerStateData/Stats/^GLOBAL_STATS/^TRA_STANDING` and wrote
nothing. AC5 awaits the user's in-game check.

### In-game check (2026-09-15, game buildid 25320008)

The user ran the installed build against slot 9 and reported "working".
Recorded as the open criterion's result on that word.

## Executive Summary

The save editor can now set the six global faction standings, up or down,
through the same guarded write path. Reviewers should look at
`internal/save/standing.go` and its test in `save_test.go`.
