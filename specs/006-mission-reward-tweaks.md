# Spec 006 — Mission reward tweaks: Nexus and mission board, scoped by table entry

## Status: COMPLETE

## Context

An in-game observation (2026-09-13): Nexus missions pay about ten of an item,
two million units and seventeen thousand nanites, with every global multiplier
tweak enabled. Decompiling the pristine and the built `REWARDTABLE` side by side
showed the tweaks working exactly as configured: Nexus item rewards are 1 each in
stock (×10 → 10), medium-tier units are 400,000–500,000 (×5 → 2M–2.5M), nanites
300–350 (×5 then ×10 → 15,000–17,500). The rewards feel low because the stock
base is low, and a global multiplier lifts every reward by the same factor.

Quicksilver (`Currency=Specials`) is not multiplied by anything: the two
currency tweaks ask for `Units` and `Nanites` only.

Facts to rely on, measured on game buildid 25233815:

- Every reward entry in `METADATA/REALITY/TABLES/REWARDTABLE.MBIN` is a
  `GcGenericRewardTableEntry` whose opening line carries `_id="<Id>"` in the
  MXML MBINCompiler writes, followed by `<Property name="Id" value="<Id>" />`.
- Nexus missions draw on seven entries, all in the `MissionBoardTable` list:
  `R_NEXUS_MED`, `R_NEXUS_MEGA` (item lists), `R_NEXUS_MED_C`,
  `R_NEXUS_MEGA_C` (units, nanites, quicksilver), `R_NEXUS_CASH` (nanites,
  quicksilver), `R_NEXUS_QS`, `R_NEXUS_QS_PQ` (quicksilver). The multiplayer
  mission table references `R_NEXUS_MED`; `GcMultiplayerGlobals.
  EpicMissionRewardOverride` names `R_NEXUS_MEGA`.
- Station mission board missions (faction and guild agents; `npcmissiontable`
  and `missiontable`) draw on `R_MB_LOW`, `R_MB_MED`, `R_MB_HIGH`, `R_MB_MEGA`.
  Corvette missions (`corvettemissiontable`) draw on the parallel
  `R_CV_LOW`, `R_CV_MED`, `R_CV_HIGH`, `R_CV_MEGA`.
- Each entry mixes item blocks (`GcRewardSpecificProduct`,
  `GcRewardSpecificSubstance`, in `R_NEXUS_MEGA` also
  `GcRewardSpecificProductFromList`) and money blocks (`GcRewardMoney` with a
  `Currency`). Items and currency therefore need separate multipliers inside
  the same entry.
- The engine's existing multiplier ops `WRAPPER_MULT` and `CURRENCY_MULT`
  (spec 002 R1.3, spec 005 R2) run across the whole file. The
  `REPLACE_TYPE="ALL"` path is bounded only at its start (`PRECEDING_KEY_WORDS`
  advances `start`; `end` stays at end of file), and an anchor on the `Id` line
  scopes to that self-closing line alone. No existing key can say "every
  block of this kind inside entry X". Hard-coding one keyword group per item
  would need ~50 groups per entry and break on the next item-list change.
- The user's rule for built-ins: only for effects with no ready-made community
  script. A Nexus search found none for this.

## Requirements

### R1 — Entry scoping for the multiplier ops (engine, additive)

- R1.1 `CURRENCY_MULT` and `WRAPPER_MULT` accept an optional `ENTRY` key
  (string). When present, the op edits only blocks inside the reward-table
  entry whose opening line contains `_id="<ENTRY>"`; the scope is that line
  through its close. When no `_id` line matches, fall back to the line
  `name="Id" value="<ENTRY>"` walked up one level, so an MXML written without
  `_id` attributes still resolves.
- R1.2 With `ENTRY` absent, both ops behave exactly as before: same edits,
  same event text. The existing op tests and the golden suite prove it.
- R1.3 An `ENTRY` that is not found in the file produces one `WARN`
  (`entry <ENTRY> not found in <file>`) and no edits, so a renamed entry
  shows up in the report as a mod needing checking rather than as silence.
- R1.4 The `OK` event names the entry (`… x5.0 in entry R_NEXUS_MED across N
  blocks …`) so the report and the amount audit's attribution say which entry
  a mod touched. `CAP` applies as in spec 005 R2.1.
- R1.5 `mods check --json` reports `ENTRY` as interpreted, not unsupported.

### R2 — Built-in tweak: Nexus mission rewards

- R2.1 New built-in `NexusRewards` (group `Missions`) editing the seven Nexus
  entries only, on top of whatever the global tweaks did (it sits after them
  in build order).
- R2.2 Parameters, all `@param`-declared: `ITEM_MULT` (default 5, 1..100) on
  `GcRewardSpecificProduct`, `GcRewardSpecificProductFromList` and
  `GcRewardSpecificSubstance` blocks; `UNITS_MULT` (default 5), `NANITE_MULT`
  (default 1), `QS_MULT` (default 5), each 1..100, on `GcRewardMoney` blocks of
  that currency (`Specials` is quicksilver). Caps: `ITEM_CAP` 50000,
  `UNITS_CAP` 50000000, `NANITES_CAP` 250000, `QS_CAP` 100000 (the audit's
  default quicksilver ceiling). 0 = no ceiling, as in spec 005.
- R2.3 Every declared cap reaches at least one block (extends the spec 005
  cap-conformance test).

### R3 — Built-in tweak: mission board rewards

- R3.1 New built-in `MissionBoardRewards` (group `Missions`) editing the four
  mission board entries and the four corvette entries.
- R3.2 Same parameter set and caps as R2.2 minus quicksilver (these entries
  carry none); multipliers default to 1 so the tweak ships neutral and the
  user dials it in.

### R4 — Registration, front ends, docs

- R4.1 Both scripts are embedded and listed in the built-in order after
  `MissionStandingBuff`; `Missions` joins the group headings so the Tweaks
  section shows a card group rather than filing them under Other.
- R4.2 No new core operation: the CLI and GUI pick the new tweaks up through
  the existing `tweaks list/set` path and the Tweaks section. The parity test
  needs no allow-list change.
- R4.3 README's tweak list, if it names the built-ins, gains the two entries.

## Acceptance Criteria

- [x] AC1 `ENTRY` on `WRAPPER_MULT` multiplies only the wrapper blocks inside
  the named entry on a synthetic two-entry snippet; the other entry's block of
  the same wrapper is byte-identical.
- [x] AC2 `ENTRY` on `CURRENCY_MULT` multiplies only the matching-currency
  money blocks inside the named entry; same-currency blocks in another entry
  are untouched.
- [x] AC3 With no `_id` attribute in the snippet, `ENTRY` resolves through the
  `Id` line and its parent, and edits the same blocks.
- [x] AC4 An unknown `ENTRY` yields exactly one `WARN` naming the entry and
  changes no line.
- [x] AC5 Both ops without `ENTRY` produce output and event text identical to
  before the change (existing tests unchanged and passing; `go test ./...`
  green, golden suite green where the fixtures are present).
- [x] AC6 `NexusRewards` and `MissionBoardRewards` load through the sandbox,
  declare headers and usable parameters, and every declared cap reaches a
  block (the three existing built-in conformance tests cover them by
  enumeration).
- [x] AC7 On the real game table (`NMSBONKER_GAME_DIR` set), a build with the
  default parameters yields, in the built `REWARDTABLE`: `R_NEXUS_MED`
  `SHIP_CORE_C` amount 50 (1 ×10 global ×5), `R_NEXUS_MED_C` units
  10,000,000–12,500,000, nanites 15,000–17,500 unchanged, `R_NEXUS_QS`
  quicksilver 2000; `R_MB_MED` units 350,000–600,000 (global ×5 only) and
  items ×10 only. Verified by decompiling the built MBIN, recorded in Status
  notes.
- [x] AC8 The build report lists both tweaks as `WORKING` with no
  `not found` warnings, and the amount audit stays clean at defaults.
- [x] AC9 `Missions` appears as a group heading in the Tweaks section and the
  CLI `tweaks list` shows both tweaks with their parameters.

## Risks & Assumptions

- Additive engine key only; no golden fixture changes. A block without `ENTRY`
  takes the existing code path.
- Compounding is by design here: the entry tweaks multiply the global tweaks'
  output. The spec 005 caps and audit are the guard; defaults were chosen to
  stay under the audit's ratio and absolute thresholds (Nexus items end at
  ×50, units at ×25, quicksilver at ×5).
- The `_id` attribute is MBINCompiler output, present in every decompile this
  project has seen; the `Id`-line fallback covers its absence.
- Entry ids are stable across recent game versions but not guaranteed; R1.3
  makes a rename visible in the report.
- Rollback: disable the tweak (`tweaks` toggle) or revert the commit; the
  game directory only changes on deploy, which archives what it replaces.

## Alternatives Considered

- Per-item keyword groups (`FOREACH_SKW_GROUP` with `{"Id", entry, item}` and
  `SECTION_UP`): rejected, ~50 groups per entry and brittle against item-list
  changes.
- Bounding the `REPLACE_TYPE="ALL"` path at the preceding keyword's section
  end: rejected, it changes reference-engine semantics that the golden suite
  pins.
- Raising the global multipliers: rejected, it inflates chests, salvage and
  every other reward the user considers already right.

## Executive Summary

Two new built-in tweaks multiply mission rewards by reward-table entry: Nexus
missions (items, units, nanites, quicksilver) and station mission board plus
corvette missions (items, units, nanites), on top of the global tweaks. To make
that possible the engine's `WRAPPER_MULT` and `CURRENCY_MULT` ops gained an
optional `ENTRY` key that confines them to one `GcGenericRewardTableEntry`;
without the key they are unchanged. Reviewers should look first at
`entryScope` in `internal/mxml/apply.go` and its tests in `entry_test.go`, then
at the two scripts under `internal/tweaks/scripts/`.

## Status notes

### Acceptance run (2026-09-13, game buildid 25233815)

Both tweaks enabled at defaults alongside the ten existing built-ins and the
twelve library mods; `nmsbonker build` without deploy; the built
`REWARDTABLE.MBIN` decompiled with MBINCompiler v7.02.0-pre1 and compared to
the pristine table extracted from the pak.

| Entry | Field | Stock | Built | Path |
| --- | --- | --- | --- | --- |
| R_NEXUS_MED | SHIP_CORE_C amount | 1 | 50 | ×10 global, ×5 here |
| R_NEXUS_MED_C | units (first block) | 400,000–500,000 | 10,000,000–12,500,000 | ×5 global, ×5 here |
| R_NEXUS_MED_C | nanites | 300–350 | 15,000–17,500 | ×5 ×10 global, ×1 here |
| R_NEXUS_QS | quicksilver | 400 | 2,000 | ×5 here only |
| R_NEXUS_CASH | quicksilver | 150 | 750 | ×5 here only |
| R_MB_MED | HYPERFUEL2 amount | 1–2 | 10–20 | ×10 global, ×1 here |
| R_MB_MED | units | 70,000–120,000 | 350,000–600,000 | ×5 global, ×1 here |

Report: `NexusRewards WORKING 21 edits`, `MissionBoardRewards WORKING 40
edits`, no `not found` warnings, amount audit clean (4064 blocks checked).
`R_NEXUS_MED` carries no substance block, so its substance op reports
`across 0 blocks` as an OK line, not a warning; that is the whole-file ops'
existing behaviour and is left alone.

### What was not checked by eye

AC9's GUI half rests on the `Missions` group being in `tweaks.Groups`, which
the Tweaks section lays cards out under; the GUI test suite passes but the
heading was not looked at in a running window. The CLI half was checked.

### Deviations from the spec text

None. The golden parity test skips the two new scripts by name: they are
project-authored and have no reference copy to decode against, and the test's
"reference copy does not load" failure is deliberate for the ten that do.
