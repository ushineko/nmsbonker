# Spec 022: Export any mod's script

**Issue**: #28

## Status: INCOMPLETE

## Context

nmsbonker imports AMUMSS scripts (`mods add`, `mods import`) and shows them
(`mods show`, the script dialog), but cannot hand one back out with the values
set in it. The project is open source and its built-ins are meant to be plain
AMUMSS scripts (project rule), so a person should be able to take any of them
out, edit it by hand, or run it in another toolchain.

## Requirements

- R1 `core.ExportMod` writes a mod's script exactly as the next build loads it
  (`scriptSource`: built-in or library text with the parameter overrides
  substituted) to a file. `Out` is a directory (the file is `NAME.lua` in it)
  or a file path; empty is the current directory.
- R2 An existing file is not replaced unless `Force` (`ErrExportExists`).
- R3 A comment is appended (not prepended, so an imported copy's description,
  taken from its opening comments, is unchanged): exported by which version,
  when, from which built-in or library script; the parameter values set; and
  the keys in it that only nmsbonker understands (`CAP`, `CURRENCY_MULT`,
  `WRAPPER_MULT`), with what that means elsewhere.
- R4 CLI `nmsbonker mods export NAME [-o DIR|FILE] [--force] [--json]`.
- R5 GUI: Export… on every Tweaks card and as a Mods row action; a folder
  chooser, then a confirmation before replacing an existing file. The parity
  list carries `mods export`.

## Acceptance Criteria

- [x] AC1 An export carries the set parameter values as the script's own and
  loads through the sandbox on its own; the imported description is unchanged
  (`TestAnExportCarriesTheParameterValuesAndLoads`).
- [x] AC2 An existing file is refused without force and replaced with it
  (`TestAnExportNeverOverwritesUnlessForced`).
- [x] AC3 The note names nmsbonker-only keys, and is absent for a script with
  none (`TestAnExportNamesTheKeysOnlyNmsbonkerUnderstands`).
- [x] AC4 Integration, real game and compiler: a built-in exported with an
  override, imported into the library under another name and built in place of
  the built-in, gives a byte-identical merged REWARDTABLE
  (`TestAnExportBuildsWhatTheBuiltInBuilt`).
- [x] AC5 The CLI/GUI parity test passes with `mods export` on both sides; the
  Tweaks card with Export… rendered off-screen and inspected.
- [ ] AC6 In the window: Export… on a Tweaks card writes the file to the chosen
  folder, and asks before replacing. Recorded by the user.

## Risks & Assumptions

- Exported scripts using nmsbonker-only keys do not do the same thing in
  another toolchain; the note says so. Spec 021 rewrites the built-ins that can
  be rewritten in AMUMSS form.
- Writes go only where the user points; nothing under the game directory.
- Rollback: revert the commit; nothing persistent depends on it.

## E2E Test Plan

1. In Tweaks, Export… on Favoured rewards to a folder; open the file: the
   parameter values match the sliders, and the trailing note is there (AC6).
2. Export again to the same folder: the window asks before replacing (AC6).
