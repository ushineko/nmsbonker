# Spec 009 — Raw JSON browser and inline editor

## Status: COMPLETE

## Context

Spec 007's export and import cover the whole save as a file. For the change
the typed editor has no field for, a person wants to look at one part of the
JSON and edit it in place without a round trip through another program. A save
is two megabytes on one line, so "show the JSON in a text box" is not an
editor: the tab browses by path and edits one node at a time.

## Requirements

- R1 `core.GetSaveNode(slot, path, raw)`: the node at a slash-separated path
  (names or keys; a number for an array element; "" is the root), its type and
  compact size, its children (name, key, type, size, item count), and, when the
  node is at most 256 KiB, its JSON indented with keys named (or raw). A path
  that leads nowhere is an error naming it.
- R2 `core.SetSaveNode(slot, path, json, dryRun, force)`: parse the text with
  the strict grammar, turn named keys back, replace the node at the path (which
  must exist: this replaces, never adds), refuse the root (that is import),
  re-check the save still has a player state, and write through spec 007's
  guarded path. Unchanged content writes nothing. A dry run reports whether it
  would change.
- R3 `save.Mapping.Lookup` accepts array indices; `save.Mapping.Replace` puts
  a node at an existing path.
- R4 CLI: `saves get <slot> [PATH] [--raw] [--json]` prints the JSON, or the
  children when the node is too large; `saves set <slot> PATH <file|->`.
- R5 GUI: the Raw JSON tab is a browser: a path field with Load and Up, a
  children table to descend by clicking, a monospace editor for a node small
  enough, with Revert, Check (dry run) and Apply…; the whole-save Export and
  Import stay below. The path, node and edited text survive the rebuilds an
  operation causes, and reset when another save is selected.

## Acceptance Criteria

- [x] AC1 On a synthetic save, the root lists its five children by name and
  inlines its JSON; a path with an index reaches one slot; `--raw` keeps keys;
  a missing path is an error.
- [x] AC2 A dry-run set reports a change and writes nothing; a set replaces
  the node and nothing else in the game's spelling, counting the keys turned
  back; an identical value writes nothing; invalid JSON, a missing path, the
  root and a change that removes the player state are refused with nothing
  written.
- [x] AC3 `Lookup` indexes arrays and rejects non-numeric or out-of-range
  segments; `Replace` refuses to add a member or element.
- [x] AC4 `saves get 9 CommonStateData` on the real profile prints the node
  with named keys; the parity guard passes with the two new leaves.
- [x] AC5 In the window, a node is loaded, edited, checked and applied, and
  the save re-reads with the change. Recorded by the user.

## Risks & Assumptions

- The same write path and guards as spec 007. The 256 KiB inline cap is a
  usability bound, not a safety one; `saves set` from a file has no cap.
- Replace-not-add is deliberate: a typo in a path must not create a key the
  game has never heard of.

## Status notes
### In-game check (2026-09-15, game buildid 25320008)

The user ran the installed build against slot 9 and reported "working".
Recorded as the open criterion's result on that word.

## Executive Summary

The Raw JSON tab browses a save by path and edits one node inline, through
`core.GetSaveNode` and `core.SetSaveNode`; the CLI has the same as `saves get`
and `saves set`. Reviewers should look at `Replace` in `internal/save/mapping.go`
and `SetSaveNode` in `internal/core/saveedit.go`.
