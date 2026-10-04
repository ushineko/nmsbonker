# Spec 014: The common library mods as built-in tweaks, on pages

**Issue**: #7

## Status: COMPLETE

## Executive Summary

Fourteen new built-in tweaks replace the third-party library scripts people
were running. Each is written for this project, credits the mod that had the
idea, and is checked against the current game by an integration test. The
Tweaks section becomes five pages under fynedesygn's new `shell.Tabs`
(v0.1.82). The build report names a library mod that changes what a built-in
changes, using the same `Signature` comparison `mods check` already used, now
moved into `internal/build`. Reviewers should look first at the scanner and
chef scripts, which differ from the originals on purpose (see Status notes),
and at `overlaps` in `internal/build/plan.go`.

## Context

The twelve built-in tweaks cover rewards and the economy. The other effects
people run with them still come from third-party AMUMSS scripts in the
library. The set on the development machine shows what those are: faster
refiners, faster mining, movement speed, scanner range and recharge, ship
transfer range, fleet expedition time, frigate rewards, pulse engine speed and
atmospheric hover, instant dialogue text, a shorter hold-to-confirm,
technology stacking, and a Nexus chef who keeps talking.

Each of those becomes a built-in tweak, with parameters where a number makes
sense. Library scripts keep working exactly as before.

The repository is public, and the rules forbid committing third-party
scripts. Every new built-in is therefore written for this project. It changes
the same game fields to the same effect, with its own parameters, and its
header names the community mod that had the idea. No text is copied from the
originals. A list of game file paths is a fact about the game, not anyone's
expression. Where a list is needed, it is the one `MaterialYield10x` already
carries.

At twenty-six tweaks, one flat list is too long to read. The section becomes
a `shell.Tabs` (fynedesygn spec 053) with one page per subject, as hotaru's
Create section works.

A library script whose effect a built-in now covers would edit the same value
twice. The build reports that overlap. Nothing is switched off or deleted
automatically.

## Requirements

### R1 New built-ins

Fourteen scripts under `internal/tweaks/scripts/`, appended to the build
order after the existing twelve, and named so that none collides with a
common library filename:

| Name | Page | Effect | Parameters (default) |
|------|------|--------|----------------------|
| MiningSpeed | Gathering | divides `Health` on the mineable entity files | speed ×5 |
| MiningLaser | Gathering | `LaserBeamMineRate` ×, terrain resource amounts × | laser ×7, terrain ×10 |
| RefinerSpeed | Gathering | divides recipe `TimeToMake` | speed ×10 |
| MovementSpeed | Player | ground run, jetpack and swim speeds × | run ×3, jetpack ×10, swim ×3 |
| ScannerBoost | Player | binocular scan time; tool and ship scan range and recharge | binoculars 0 s, range ×2, recharge 2 s |
| TechStacking | Player | `MaxNumSameGroupTech` | 25 |
| ShipTransferRange | Ships | `ShipInteractRadius` | 1000000 m |
| PulseEngineSpeed | Ships | `MiniWarpSpeed` × | ×4 |
| AtmosphereHover | Ships | hover and planet-engine minimum speeds near zero | none (switch only) |
| FleetExpeditionTime | Ships | seconds per expedition event, normal and easy | 5 s |
| FrigateRewards | Ships | `EXPEDITIONREWARDTABLE` units, nanites, products, substances × | ×10 each |
| InstantText | Interface | every punctuation and default text delay | 0.003 s |
| QuickConfirm | Interface | hold-to-confirm times, with the mouse not doubled | 0.15 s |
| ChefKeepsTalking | Interface | the Nexus chef's dialogue stays open | none (switch only) |

- R1.1 Every new built-in is disabled on an existing configuration and on a
  fresh one. This is already how reconcile treats a built-in it has not seen.
- R1.2 Every edit in a new built-in finds its key in the current game build:
  zero `keys not found` in the build report.
- R1.3 No new built-in carries an `ADD` or `REMOVE` key. The original instant
  text mod's ADD blob is not reproduced, because `DefaultDelay` already covers
  every character without one. *Retired by spec 015: a built-in may add
  entries.*

### R2 Pages

- R2.1 The `group` header names one of five pages, in this order: Rewards,
  Gathering, Player, Ships, Interface. The existing twelve move onto them.
  `MaterialYield10x`, `SpaceMiningBoost` and `BigStacks` go to Gathering.
  The other nine go to Rewards.
- R2.2 The Tweaks section is a `shell.Tabs` with one part per page. Each part
  lists its tweaks in build order, with the existing card. The card tag reads
  "build order N", because the page already says the subject. Each part has
  the same Apply and build, Reset all and Refresh strip.
- R2.3 fynedesygn stores the chosen page, so it survives rebuilds, leaving
  the section, and a restart. `--section Tweaks` still opens the section.
- R2.4 `tweaks list` (CLI) shows the page in its group column, as it does
  today.

### R3 Overlap with library mods

- R3.1 The build finds every game file where an enabled library mod and an
  enabled built-in both change the same key. A key is a `VALUE_CHANGE_TABLE`
  key name, or the wrapper or currency of a `WRAPPER_MULT` or
  `CURRENCY_MULT`. This is `build.Signature`, the comparison `mods check` has
  used since spec 005, moved out of `core` so the two cannot disagree. Each
  pair is reported once as library mod, built-in, file and keys.
- R3.2 `report.json` carries the pairs under `overlaps`. The library mod's
  row in `BUILD_REPORT.md`, `report` (CLI) and the GUI's Report section
  carries a note: "overlaps built-in X". The Mods detail dialog already said
  this (spec 005 R4.1).
- R3.3 Built-in against built-in is not an overlap. The currency tweaks
  compound on purpose (spec 006). Library against library is not reported
  either, because that has always been the build order's job.
- R3.4 Nothing is disabled or deleted automatically.

### R4 Parity

- R4.1 No new core operation. The CLI and GUI already share `ListTweaks`,
  `SetTweakParam`, `ResetTweak` and `SetModEnabled`. Overlaps travel in the
  existing build report.

## Acceptance Criteria

- [x] R1 `tweaks.Names()` lists 26 built-ins and `tweaks.Files()` matches it
      (existing test).
- [x] R1 Every built-in declares a known page, so none lands under "Other"
      (new test).
- [x] R1 No built-in's `MOD_AUTHOR` is anyone but nmsbonker, and no script
      carries `ADD` or `REMOVE` (new test).
- [x] R1.2 A build against the installed game with all 26 enabled and no
      library mods reports every new built-in as WORKING, with no keys not
      found. This is integration: real game data, real MBINCompiler.
- [x] R1.2 A merged-MXML spot check shows each new built-in's target value
      changed as described (refiner `TimeToMake`, scan `PulseRange` for
      TOOL, `MiniWarpSpeed`, `ShipInteractRadius`, `KeepOpen` for the chef,
      `DefaultDelay`).
- [x] R2 The Tweaks section is a `shell.Tabs` with five pages in order, and
      each page holds exactly its tweaks in build order (GUI test).
- [x] R2 The window is photographed on two pages.
- [x] R3 A unit test plans a library mod and a built-in that edit the same
      key in one file, plus a pair that edit different keys. Exactly the
      first pair is reported. Built-in against built-in is not reported.
- [x] R3 A build with the development machine's library and the matching
      built-ins enabled lists the expected overlaps in `report`
      (integration).
- [x] fynedesygn is bumped to the release that carries `shell.Tabs`.
- [x] README: the tweak table lists the new built-ins, the Tweaks screenshot
      is refreshed, and the changelog has an entry.

## Status notes

- **Two scripts differ from the originals on purpose.** Both were found by
  diffing the merged MXML against pristine, not from the report, which said OK
  for both.
  - The scanner's `PulseRange` and `ChargeTime` live in `SCANDATATABLE` now.
    `SPECIAL_KEY_WORDS` are matched in sequence, not as key/value pairs, so
    `{"ID", "TOOL"}` ran on to the next entry, and the TOOL entry was missed.
    The built-in anchors each entry on its quoted `_id`.
  - The chef's `KeepOpen` is a sibling of the option's `Cost`, and the
    original anchored on the self-closing `Cost` line. Its scope was that one
    line, so nothing changed, and the report still said OK. The library copy
    on the development machine has been a no-op under this engine all along.
    The built-in uses `SECTION_UP = 1` and changes exactly the 9 judging
    options.
- **Atmosphere hover** uses `SPECIAL_KEY_WORDS {"PlanetEngine"}` with ALL.
  `PRECEDING_KEY_WORDS` with ALL changed all 23 `MinSpeed` values after the
  first planet engine, including space and boost engines. The built-in changes
  exactly the 6 planet engines.
- Spot check (all 14 enabled, no library mods): refiner `TimeToMake` 1684 of
  1684 divided by 10. Scanner TOOL range 200→400, TOOL_HARD 150→300, SHIP
  30000→60000, recharge 2 s each. `MiniWarpSpeed` 30000→120000.
  `ShipInteractRadius` 1000000. `MaxNumSameGroupTech` 25. All six
  `DefaultDelay` and every `Delay` 0.003. 9 `KeepOpen` false→true.
  `Health` divided on all 69 entity files.
- Overlap integration (the development machine's twelve library scripts plus
  the fourteen built-ins): each library script reports exactly the built-ins
  that replace it, and no built-in reports an overlap.
- `TestEveryBuiltInFindsEveryKeyInTheInstalledGame` automates R1.2. It skips
  when `NMSBONKER_GAME_DIR` is unset.
- The tab-strip shape went into fynedesygn first (its spec 053, #162, v0.1.82),
  as this repository's rules require. No gap is left in the library.

## Risks & Assumptions

- Defaults are moderate: frigate rewards ×10, not the ×75 of the mod they
  replace. Anyone who wants the old numbers sets them once.
- Existing configurations get the new built-ins disabled at the end of the
  order (R1.1), so an upgrade changes nothing in the game until the user
  turns one on.
- Overlap detection compares key names inside one file, which is coarse. Two
  mods that change `Amount` in different rows of the recipe table are
  reported as overlapping. The report states a fact ("both change X in F"),
  and the user decides.
- Rollback: revert the merge. New built-ins that were never enabled leave
  entries in `config.json`, and reconcile reports them as missing scripts
  that can be removed.

## E2E Test Plan

On the development machine, with the real game:

1. `nmsbonker tweaks enable` all fourteen new built-ins, then `nmsbonker build`.
   Each new built-in reports WORKING with zero skipped (R1.2).
2. Enable the library mods they replace as well, and build again. `report`
   lists one overlap per replaced effect (R3).
3. Turn the library copies off, deploy, and check in game that refining,
   scanning, pulse speed and dialogue text behave as configured. This step
   is the user's.
4. Open the GUI on Tweaks, switch to Ships, close and reopen it. Ships is
   still the page showing (R2.3).
