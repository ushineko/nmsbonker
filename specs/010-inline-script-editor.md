# Spec 010 — Edit a library mod's script in place

## Status: COMPLETE

## Context

A mod is a `.lua` file, and the quickest tweak to one is a line changed in it.
Until now that meant Open script, an external editor, and a rebuild. The Mods
section gains an editor over the script, with the sandbox as a check before
anything is written.

## Requirements

- R1 `core.ReadModScript(name)`: the script's text as on disk (or as compiled
  in, for a built-in, marked read-only), its path, size and mtime.
- R2 `core.WriteModScript(name, text, check, force)`: loads the text through
  the sandbox (`modscript.LoadSource`) and reports whether it loads and how
  many change blocks it declares; `check` stops there. Otherwise the text is
  written atomically over the library file, the previous text kept beside it
  as `<name>.lua.bak` (which the library listing ignores), unchanged text is
  not written, text that does not load is refused unless `force`, and a
  built-in is refused with `ErrReadOnlyScript`.
- R3 CLI: `mods show <name> [--json]` prints the text; `mods write <name>
  <file|-> [--check] [--force]`.
- R4 GUI: an Edit script… row action in Mods opens a dialog with a monospace
  editor, Revert, Check and Save; a built-in opens read-only with a note
  pointing at Tweaks. Save reports the path and the .bak, and invalidates the
  mod list so the next build and check see the new text.

## Acceptance Criteria

- [x] AC1 A library script reads back byte for byte with its path; a check
  reports loads/blocks and writes nothing; text that does not load is refused
  and the file is unchanged; `force` writes it and the `.bak` holds the
  previous text; a good edit lands and the `.bak` rolls; identical text writes
  nothing; the `.bak` is not listed as a mod; a built-in reads and refuses to
  write.
- [x] AC2 The parity guard passes with `mods show` and `mods write`.
- [x] AC3 In the window: Edit script… on a library mod, a value changed,
  Check says it loads, Save, and the next build applies the new value.
  Recorded by the user.

## Risks & Assumptions

- The write goes into the user's library, not the game; the `.bak` is the
  undo, one deep. Parameter overrides in the settings still apply on top of
  the edited text at build time.
- A saved script that no longer declares a `@param` the settings override
  would make that override unmatched; `mods check` reports it as before.

## Status notes
### In-game check (2026-09-15, game buildid 25320008)

The user ran the installed build against slot 9 and reported "working".
Recorded as the open criterion's result on that word.

## Executive Summary

Library scripts can be edited from the window (and via `mods show`/`mods
write`), with the sandbox as the gate and a `.bak` as the undo.
`internal/core/modedit.go` is the whole of it.
