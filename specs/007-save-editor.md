# Spec 007 — Save editor: read, inspect and edit the game's save files natively

## Status: COMPLETE

## Context

The exosuit's slot-purchase limit (one pod per star system) is enforced by the
game executable and cannot be lifted by a pak mod; the attempt on 2026-09-12
built, deployed and changed nothing (recorded in the project memory). The slot
count itself, however, lives in the save file, and every established save editor
unlocks it there. The same is true of the currencies and of a handful of other
values the user wants to set once rather than grind. This spec adds a native
save reader and editor to nmsbonker, in Go, with no Java, .NET or Python.

This reverses a documented stance. Spec 004 R5 and the README ("Not a save
editor", "nmsbonker never writes into the prefix itself") made save backup a
one-way copy precisely because a tool that writes into the save directory can
destroy a save by getting one path wrong. The reversal is deliberate and is
bounded: the backup operation stays one-way and unconditional, and the only
code path that writes into the prefix is the one specified here, behind a
mandatory pre-write backup, a game-not-running check and an atomic write.

### Recon: facts measured on this machine (2026-09-14, game buildid 25233815)

Measured read-only against the user's own profile with a scratchpad probe;
nothing under the prefix was written.

- Save directory: `<compatdata>/275850/pfx/drive_c/users/steamuser/AppData/
  Roaming/HelloGames/NMS/st_<steamid>/`, the path `core.savesDir()` already
  resolves. It holds `save.hg`, `save2.hg` … `save18.hg` with a sibling
  `mf_<name>.hg` each, plus `accountdata.hg`, `mf_accountdata.hg`, a
  `cache/` folder of DDS thumbnails and `steam_autocloud.vdf`.
- Slot numbering (libNOM.io, confirmed by the files present): `save.hg` is
  file 1; `saveN.hg` is file N; slot = ⌈N/2⌉, odd N is the autosave, even N
  the manual save. Fifteen slots, thirty files at most. The newest pair here is
  `save17.hg`/`save18.hg` (slot 9).
- Data container: concatenated chunks, each a 16-byte header of four
  little-endian uint32 (magic `0xFEEDA1E5`, compressed size, decompressed
  size, zero) followed by a raw LZ4 *block* (not the LZ4 frame format).
  Decompressed chunks are 0x80000 bytes except the last. `save18.hg` is four
  chunks, 114,719 bytes on disk, 1,966,855 bytes decompressed. The payload is
  single-line JSON followed by one NUL byte.
- `accountdata.hg` is the same JSON-plus-NUL, uncompressed (starts with `{`).
  This spec does not touch it.
- JSON keys are three-character obfuscated tokens. MBINCompiler's GitHub
  releases attach `mapping.json` (`{"libMBIN_version":…,"Mapping":[{"Key":
  "F2P","Value":"Version"},…]}`); the v7.02.0-pre1 file has 1,462 distinct
  keys and names every key in `save18.hg`. The older `save.hg` (Version 4665)
  has one key, `bQN`, the current mapping no longer names. Two keys are
  ambiguous (`V86`, `NE3` each carry two names).
- Top level: `Version` 4735, `Platform` `"Win|Final"`, `ActiveContext`
  `"Main"`, `CommonStateData` (`SaveName`, `TotalPlayTime`, season state),
  `BaseContext` (`GameMode`, `PlayerStateData`, `SpawnStateData`),
  `DiscoveryManagerData`. An `ExpeditionContext` appears when a season is
  active. Version = base version + 512 × game mode (+ season × 128 × 512);
  4735 = 4223 + 512 (Normal). Everything Waypoint 4.0 or later except
  permadeath and seasonal writes game mode 1 here; difficulty lives in
  `PlayerStateData.DifficultyState`.
- `PlayerStateData` (263 keys) carries `Units` 162,810,044, `Nanites` 36,793,
  `Specials` (quicksilver) 0, `Health` 60, `Shield` 100, `Energy` 40,
  `ShipOwnership`, `Multitools`, `KnownTech`, `KnownProducts`, `FreighterInventory`,
  `Chest1Inventory` … `Chest10Inventory`, `DifficultyState`.
- Exosuit inventories under `PlayerStateData`: `Inventory` is the item
  inventory, `Width` 10 × `Height` 12, 34 entries in `ValidSlotIndices`, 27
  occupied `Slots`; `Inventory_TechOnly` is 10 × 6 with 13 valid; and
  `Inventory_Cargo` is a vestigial 7 × 5 with zero valid slots. This settles
  which key Waypoint's merge kept: the 120-slot item inventory is `Inventory`.
  An inventory unlocks a cell by listing its `{X,Y}` in `ValidSlotIndices`;
  `Slots` holds occupied cells only, each `{Type, Id, Amount, MaxAmount,
  DamageFactor, FullyInstalled, Index{X,Y}}`.
- Manifest (`mf_*.hg`): 432 bytes for the current saves, 384 for the older
  ones, XXTEA-encrypted with the key `["NAESEVADNAYRTNRG" as four LE uint32]`
  whose first word is replaced by `rotl(slot ^ 0x1422CB8C, 13) * 5 +
  0xE6546B64`, `slot` being N + 1 for `saveN.hg` (`save.hg` → 2,
  `accountdata.hg` → 1), delta `0x9E3779B9`, six rounds (eight only for the
  104-byte pre-Frontiers layout). Decrypting `mf_save18.hg` with slot 19 gave
  header `0xEEEEEEBE`, format 2004, decompressed size 1,966,855, on-disk size
  114,719, base version 4223, game mode 1, season 0, play time 17,088 s,
  summary "On freighter (DS-8 Iojirish)", difficulty 1, timestamp
  1789449910, which equals the data file's mtime to the second. **The
  manifest records the data file's exact sizes, so an edited save needs its
  manifest rewritten**; libNOM.io does so and sets both files' mtimes to the
  manifest timestamp.
- Steam Cloud syncs every `.hg` in the profile (38 entries in
  `userdata/<id>/275850/remotecache.vdf`). A local write while the game is
  closed is uploaded on the next launch, sometimes after a conflict prompt
  where "local" is the right answer.
- No existing Go implementation exists. References: zencq/libNOM.io (C#, the
  format oracle), goatfungus/NMSSaveEditor (Java, soft limits 10×12 / 10×6
  and "the game might break above this"), oxur/nms-save (Rust, read-only).
  The game's own vanilla maxima since Waypoint are 120 item and 60 technology
  slots for the exosuit.
- Round-trip hazards recorded by libNOM.io: some technology ids appear as raw
  non-UTF-8 bytes between `^` and `#`; the game escapes `/` as `\/`; the
  payload is one line. Go's `encoding/json` into maps reorders keys and would
  re-encode strings, so the codec must preserve untouched bytes verbatim.

## Requirements

### R1 — Save container codec (`internal/save`, new package)

- R1.1 `Decode(raw []byte)` walks the chunk headers, validates the magic and
  sizes, LZ4-block-decompresses each chunk and returns the JSON bytes without
  the trailing NUL. A payload that starts with `{` is returned as is
  (uncompressed layout). A bad magic, a short chunk or a decompressed size
  disagreeing with the header is an error naming the chunk index.
- R1.2 `Encode(json []byte)` appends the NUL, splits at 0x80000 bytes,
  compresses each piece as an LZ4 block and writes the 16-byte headers. LZ4
  comes from `github.com/pierrec/lz4/v4` (block API; new dependency, the only
  one this spec adds).
- R1.3 Round trip: `Encode(Decode(x))` decompresses to the same bytes as `x`
  (compressed bytes need not match; the game's compressor is not ours).

### R2 — Manifest codec

- R2.1 `DecodeMeta(raw, slot)` and `EncodeMeta(meta, slot)` implement the
  XXTEA scheme above with six rounds for any length other than 104 bytes,
  eight for 104. Decoding checks the `0xEEEEEEBE` header and returns a struct
  with the known fields (format, decompressed size, on-disk size, base
  version, game mode, season, play time, save name, save summary, difficulty,
  timestamp) and the remaining bytes verbatim, so an unknown tail survives a
  rewrite untouched.
- R2.2 The slot for a file name is derived in one place: `save.hg` → 2,
  `saveN.hg` → N + 1, `accountdata.hg` → 1.
- R2.3 Encode of a decoded manifest with no field changed reproduces the
  original ciphertext byte for byte (this proves the cipher direction and the
  verbatim tail).

### R3 — Byte-preserving JSON document

- R3.1 A parser produces a tree whose scalars keep their original raw bytes
  (numbers, strings with their escapes, and any non-UTF-8 content) and whose
  objects keep key order. Serialising an unedited tree reproduces the input
  bytes exactly. Only nodes that were assigned new values are re-encoded, as
  integers, booleans or JSON strings with `/` escaped as `\/`.
- R3.2 Path addressing uses mapped names (`BaseContext/PlayerStateData/Units`)
  with the current mapping and falls back to the raw key when a segment is not
  in the mapping. Ambiguous mapped names resolve by the first entry in the
  mapping file, and the CLI's `--raw` form accepts obfuscated keys directly.
  The tree is never rewritten to deobfuscated keys on disk.
- R3.3 A `Coverage` report counts the distinct keys the mapping does not name,
  so a save from a game version newer than the mapping shows it.

### R4 — Mapping acquisition

- R4.1 `tools ensure` downloads `mapping.json` from the same MBINCompiler
  release it installs, beside the binary, when the release attaches one; the
  install verifies it parses. Its absence on a release is a warning, not a
  failure, and the save operations then say which release lacked it.
- R4.2 `tools list` and the Tools section show whether a mapping is present
  for the installed release. The mapping is not embedded in the binary: it
  tracks the game and belongs with the compiler that tracks it.

### R5 — Core operations (headless, `internal/core`)

All take a `SlotRef` naming a slot 1–15 and `auto` or `manual`, resolved to a
file pair, and all read the manifest and payload through R1–R3.

- R5.1 `ListSaveSlots` enumerates the profile: for each existing file, slot,
  kind, file name, manifest name and summary, play time, base version, game
  mode, difficulty, timestamp, on-disk size, and which pair is newest. It also
  reports the mapping status (R4) and whether the game is running (R6.2).
- R5.2 `InspectSave` decodes one save and returns the values this spec edits
  plus context: version and derived base version and mode, active context,
  units, nanites, quicksilver, health, shield, exosuit item and technology
  grid sizes with valid and occupied counts, ship and multitool counts, play
  time, coverage (R3.3). It writes nothing.
- R5.3 `ExportSave` writes the decoded JSON to a path outside the prefix
  (default under the workspace), optionally pretty-printed and with keys
  deobfuscated for reading. `ImportSave` takes a JSON file, re-obfuscates keys
  if the file was exported deobfuscated, and writes it through R6. This is the
  raw-edit escape hatch NomNom and goatfungus both offer.
- R5.4 `EditSave` applies a typed change set and writes through R6:
  - `Units`, `Nanites`, `Quicksilver`: integers 0 … 4,294,967,295.
  - `SuitItemSlots`, `SuitTechSlots`: target count of unlocked cells,
    1 … Width × Height (120 and 60 at the vanilla grid). Unlocking appends the
    missing `{X,Y}` in row-major order; shrinking removes only cells that hold
    no item and refuses otherwise, naming the occupied cell. The grid size is
    not changed.
  - `Health`, `Shield`: integers within the value's current range (a value
    above the game's maximum is clamped by the game; the editor states this).
  - The change set is applied to the context named by `ActiveContext`; a save
    whose active context has no `PlayerStateData` is an error.
  - `DryRun` returns the planned changes (path, old, new) and writes nothing.
- R5.5 Every operation refuses a payload whose `Version` is below 4140
  (pre-Waypoint layout, untested here) with a message saying so; nothing else
  about the version is gated. A newer game is handled by coverage warnings,
  not refusals.

### R6 — The write path (the only code that writes into the prefix)

- R6.1 Before any write the whole profile is copied with the existing backup
  (spec 004 R5.1), unconditionally and every time, not once per process, and
  the backup directory is named in the result. A failed backup aborts the
  write.
- R6.2 The write refuses when the game is running. Detection reads
  `/proc/*/comm` and `/proc/*/cmdline` for `NMS.exe`; a `--force` flag exists
  for the CLI and a confirmation for the GUI, both spelled out as unsafe.
- R6.3 Writes are atomic per file: temp file in the profile directory, fsync,
  rename over the target; data file first, then manifest. The manifest is
  rewritten with the new decompressed and on-disk sizes and the current Unix
  time; save name, summary, base version, mode, season and play time are
  carried over unchanged; both files' mtimes are set to the manifest
  timestamp. File modes of the originals are preserved.
- R6.4 The result names the backup taken, the files written and carries a
  fixed Steam Cloud note: if Steam shows a cloud conflict on the next launch,
  choose the local file.
- R6.5 `accountdata.hg`, the `cache/` folder and any file not in the addressed
  pair are never opened for writing.

### R7 — CLI (`nmsbonker saves …`)

- R7.1 `saves slots [--json]` (R5.1) as a table: slot, kind, name, summary,
  play time, base version and game mode, written time, newest marked. No
  release-name table: the base version is what the manifest holds and does
  not go stale when the game updates.
- R7.2 `saves inspect <slot>[:auto|manual] [--json]` (R5.2).
- R7.3 `saves export <slot> [--out path] [--pretty] [--names]` and
  `saves import <slot> <file> [--force]` (R5.3, R6).
- R7.4 `saves edit <slot> [--units N] [--nanites N] [--quicksilver N]
  [--suit-slots N] [--suit-tech-slots N] [--health N] [--shield N]
  [--dry-run] [--force]` (R5.4, R6). With no change flag it errors. The
  default kind is the newest of the pair, printed so the choice is visible.
- R7.5 Existing `saves backup` and `saves list` are unchanged.

### R8 — GUI (new `Saves` section)

- R8.1 A tenth section, `Saves`, listing the slots (R5.1) with the newest
  pair marked, a game-running banner in the fixed-height flash region, and
  the existing backup listing moved here from the Overview dialog (the
  Overview keeps its one-line summary).
- R8.2 Selecting a slot shows the inspect card (R5.2) and an edit form for the
  R5.4 fields with the current values prefilled, a `Preview` (dry run) that
  lists the planned changes, and `Apply` that runs the write and reports the
  backup directory and the Steam Cloud note. Export and Import use file
  dialogs and stay out of the prefix for the exported file.
- R8.3 Every new CLI leaf has a GUI action registered in `gui.Actions()`;
  the parity guard passes with no new allow-list entry.

### R9 — Documentation

- R9.1 README: replace the "Not a save editor" non-goal with a "Save editor"
  section (what it edits, the backup-first rule, the game-closed rule, the
  Steam Cloud note, and that it is not a slot copier or an inventory item
  editor); update the `saves` command listing and the workspace table.
- R9.2 `docs/architecture.md`: add `internal/save` to the package map and
  amend "The game directory is read-only except during deploy" to name the
  save write path as the second exception. Update the doc comments in
  `core/saves.go` and `gui/views_game.go` that promise the prefix is never
  written.
- R9.3 `.claude/CLAUDE.md` architecture rule updated the same way.

## Acceptance Criteria

- [x] AC1 Synthetic round trip: a hand-written obfuscated JSON payload encoded
  by R1.2 decodes by R1.1 to the same bytes; a payload larger than 0x80000
  produces more than one chunk with the correct headers; corrupt magic and
  a lying decompressed size are rejected with the chunk index in the error.
- [x] AC2 Manifest round trip: a synthetic 432-byte manifest encrypted by
  R2.1 decrypts to the same fields and tail; encoding an unchanged decoded
  manifest reproduces its ciphertext exactly; slot derivation matches
  `save.hg` → 2, `save2.hg` → 3, `save18.hg` → 19, `accountdata.hg` → 1.
- [x] AC3 Golden fidelity on real saves (`NMSBONKER_SAVE_DIR` set, skip when
  absent): for every `saveN.hg` in the directory, decode → parse (R3) →
  serialise reproduces the decompressed payload byte for byte, and the
  manifest decodes with header `0xEEEEEEBE`, sizes equal to the actual file's
  decompressed and on-disk sizes, and re-encodes to its own ciphertext. The
  manifest timestamp is logged against the data file's mtime rather than
  asserted (see Status notes: only the newest pair agreed). This is the
  integration-boundary test for the codec against the real downstream format.
- [x] AC4 A synthetic save whose `Inventory` has 34 valid cells edited to 60
  gains exactly 26 `{X,Y}` entries in row-major order after the existing
  ones, no `Slots` change, and the rest of the payload byte-identical;
  shrinking below an occupied cell is refused naming the cell; a target above
  Width × Height is refused.
- [x] AC5 `EditSave` on a copy of a real profile in a temporary directory
  (`NMSBONKER_SAVE_DIR`, skip when absent): the backup directory exists and
  holds the untouched originals; the data file re-decodes with the new units
  value and every other byte of the payload unchanged; the manifest's sizes
  equal the new file; both mtimes equal the manifest timestamp; original file
  modes preserved; `accountdata.hg` mtime unchanged.
- [x] AC6 With a fake `/proc` root (test hook) containing an `NMS.exe`
  process, `EditSave` refuses without `Force` and proceeds with it; a
  refusal takes no backup (a refusal has no side effects, and a copy per
  refused click would rotate useful backups out), a forced write does.
- [x] AC7 `DryRun` returns the planned changes and leaves the profile
  directory's file list, sizes and mtimes unchanged.
- [x] AC8 `tools ensure` on a release that attaches `mapping.json` places it
  beside the binary (release-client test with a recorded listing); on one that
  does not, the result carries a warning and the install succeeds.
- [x] AC9 The parity guard (`make test`, `tests/parity`) passes with the five
  new leaves and no new allow-list entry; the CLI still builds with
  `CGO_ENABLED=0` and `go list -deps ./cmd/nmsbonker` names neither
  `internal/gui` nor Fyne.
- [x] AC10 In game (recorded in Status notes with the game buildid): after
  `saves edit --suit-slots 120 --suit-tech-slots 60` on the newest slot with
  the game closed, the game loads the slot, the exosuit shows the full 10×12
  item and 10×6 technology grids unlocked, and the currencies edited in the
  same run display as set. The slot-select screen shows the same name,
  summary and play time as before the edit.
- [x] AC11 README, architecture map and project rules updated per R9; the
  phrase "never writes into the prefix" no longer appears anywhere in the
  repository except in the history of spec 004.

## Risks & Assumptions

- **Writing into the save directory is the risk this project previously
  refused to take.** Mitigations are R6.1 (backup every time), R6.2
  (game-closed check), R6.3 (atomic per-file writes, data before manifest),
  R6.5 (nothing else opened for writing). Rollback of a bad edit: close the
  game, copy the named backup's `st_*` folder back, as the README already
  documents. The retention of ten backups is unchanged; a session of many
  edits will rotate older pre-deploy backups out, which R6.4's output makes
  visible by naming the backup taken.
- **Steam Cloud.** A conflict prompt on the next launch is expected behaviour,
  not a fault; choosing "cloud" there discards the edit. The note in R6.4
  exists for that moment. Nothing here toggles Steam settings.
- **Game version drift.** The container and manifest layouts have been stable
  from Worlds Part II 5.5 through Cosmos 7.0x (measured here on format 2004).
  A future layout change shows up as AC3 failing on the user's saves and as
  unmapped-key coverage warnings, not as a silent bad write, because R3
  refuses to serialise a tree it could not parse and R1 refuses a chunk it
  could not validate.
- **Raw bytes in the payload.** Technology ids with non-UTF-8 bytes are
  preserved by R3.1 because untouched scalars are copied, not re-encoded. The
  editor never assigns a string containing such bytes.
- **Ambiguous mapping names** (`V86`, `NE3`) are resolved first-wins for
  display and path lookup; neither is on any path this spec edits.
- **Above-vanilla slot counts** are refused by R5.4 (bounded by the grid), so
  the "game might break" case goatfungus warns about is not reachable here.
  Lifting the grid itself would need a pak-side inventory-table change and is
  out of scope.
- **Health and Shield** ranges are not fully known; the editor writes what is
  asked within the field's type and states the game may clamp.
- **Slot copy/move, difficulty settings, inventory item edits and
  `accountdata.hg`** are out of scope. Each needs its own design (slot moves
  re-key the manifest; item edits need the game's item tables).
- **Restore from backup** stays a documented manual copy. The write path now
  exists, so a `saves restore` operation is a small follow-up; it is left out
  to keep this spec to the editor.
- Assumption: the user's profile is the only `st_*` folder present, as today.
  With several, `SlotRef` resolves against the newest profile and the result
  names it; a `--profile` flag is a follow-up.
- New dependency `github.com/pierrec/lz4/v4` (pure Go, BSD-3). Approved by
  accepting this spec.

## Alternatives Considered

- Considered shelling out to goatfungus's editor or NomNom; rejected because
  both need a Java or .NET runtime, neither shipped Cosmos support at the time
  of writing, and the project's rule is a native Go pipeline.
- Considered `encoding/json` into ordered structs; rejected because a schema
  of several hundred types tracks the game update by update, and any field
  the schema lacks is silently dropped on write. A byte-preserving tree fails
  loudly instead.
- Considered embedding `mapping.json`; rejected because it changes with the
  game and MBINCompiler already publishes it beside the compiler this project
  installs.
- Considered skipping the manifest rewrite because a 2020 report says the
  game loads a save without one; rejected because the manifest records the
  data file's exact sizes and every maintained editor rewrites it.
- Considered editing the compressed file in place (patching one chunk);
  rejected because a value change moves every byte after it and the chunk
  boundaries with it.

## Technical Notes

- Package layout: `internal/save/container.go` (R1), `meta.go` (R2),
  `document.go` (R3), `mapping.go` (mapping load and path resolution),
  `edits.go` (the typed change set, pure functions over the tree),
  `profile.go` (slot and file naming). `internal/core/saveedit.go` wires
  sessions, backup and the write path; `internal/save` never imports `core`
  or knows the prefix path.
- Game-running detection lives in `internal/core` behind a `procRoot`
  variable defaulting to `/proc`, so AC6 can point it at a fixture.
- The XXTEA implementation is thirty lines; the probe that confirmed it is in
  this session's scratchpad and is reproduced in `meta_test.go` as the known
  vector (synthetic plaintext, not the user's manifest).
- Version arithmetic: base = Version − 512 × (mode + 128 × season) where mode
  is the value from the manifest; when no manifest is readable, mode is
  `Version / 512` clamped to 1 … 6 and season is 0 (libNOM.io's thresholds).
- The CLI's `saves` group currently reads "Copy the game's saves out of the
  Proton prefix"; its short help changes to cover editing.

## Executive Summary

nmsbonker can now read and edit No Man's Sky saves natively: a new
`internal/save` package handles the chunked LZ4 container, the XXTEA manifest
and a byte-preserving JSON tree, and `internal/core` gains list, inspect,
export, import and typed-edit operations behind one guarded write path that
backs the profile up, refuses while the game runs and writes atomically. This
reverses the spec 004 "never writes into the prefix" stance for that one path
only. Reviewers should start with `writeSave` in `internal/core/saveedit.go`,
then `document.go` and `edits.go` in `internal/save`, then the golden tests
that run against the user's own saves.

## Status notes

### Recon confirmations (2026-09-14, game buildid 25233815, Cosmos 7.0x)

- All eighteen save files in the user's profile (base versions 4142 through
  4223, Normal and Seasonal) decode, parse and serialise back byte for byte,
  and every manifest decrypts with the slot key, records the file's exact
  sizes, and re-encrypts to its own ciphertext (`internal/save/golden_test.go`
  with `NMSBONKER_SAVE_DIR`).
- The manifest timestamp equalled the data file's mtime only for the newest
  pair (`save17.hg`, `save18.hg`, delta 0 s). Older files differ by 1 s to
  40 min, consistent with Steam Cloud downloads giving files the download's
  mtime. AC3 was reworded to log the delta; the write path still sets both
  mtimes to the manifest timestamp, which is what the game does for the files
  it writes itself.
- Waypoint-era saves (base 4142–4158, 2023–2024) carry `PlayerStateData` at the
  top level with no `BaseContext`; the context split arrived with Omega 4.50.
  `save.PlayerState` reads both layouts, so R5.5's threshold of base 4140
  stands and the flat layout is a supported case rather than an error.
- The current MBINCompiler release at implementation time was v7.02.0-pre2;
  its `mapping.json` (libMBIN 7.2.0.2, 1,471 entries) names every key in the
  Cosmos saves and misses one to three keys in each 2023–2025 save, which the
  coverage count reports.

### Acceptance run on the real profile (2026-09-14)

`TestGoldenEditSaveOnACopyOfARealProfile` copied the profile into a temporary
Steam layout and edited the newest save (slot 9 manual, `save18.hg`): units
162,810,044 → 162,810,045 and item slots 34 → 120. The written payload equalled
the original with exactly those edits applied, the manifest recorded the new
sizes (1,966,855 → 1,969,323 bytes decompressed, 114,719 → 115,877 on disk)
and the write timestamp, the file mode was kept, `accountdata.hg` was untouched
and the backup held the original bytes. The user's own profile was not written.

CLI smoke run against the live install: `tools ensure` installed v7.02.0-pre2
with its mapping; `saves slots` listed all nine slots with names, summaries and
versions and marked slot 9 manual as newest; `saves inspect 9` reported 34 of
120 item and 13 of 60 technology slots; `saves edit 9 --suit-slots 120
--suit-tech-slots 60 --dry-run` listed the two changes and wrote nothing.

### Codex review (2026-09-14, spec-aware, uncommitted tree)

Twelve findings; ten acted on, two not:

- Fixed: export destinations are made absolute, symlink-resolved through
  their longest existing prefix and refused anywhere under the compatdata
  tree or the game directory (was a lexical check on the save folder only);
  consecutive writes within one second now wait for the clock rather than
  reuse a backup directory name; imports pass the same player-state and
  version gate as typed edits; a failure after the backup names the backup
  directory in the error and the result keeps the write details; the JSON
  parser rejects invalid escapes and non-JSON number spellings while still
  accepting the game's raw technology-id bytes (all eighteen real saves still
  parse); `--no-network` no longer downloads the mapping; the GUI keeps form
  drafts across the rebuild a Preview causes; the Tools card shows whether a
  release has its mapping; the real-profile edit test checks a units-only
  edit against a literal-substring oracle independent of the editor, and
  checks the manifest's mode and mtime.
- Not changed: the reviewer asked that a refused write (game running) still
  take a backup, as the original AC6 text said; the criterion was reworded
  instead (see AC6), since a refusal with a side effect is the worse
  behaviour. The reviewer also noted AC3's timestamp relaxation and AC10's
  open state; both are recorded above for the user's acceptance.

### Security review (2026-09-14)

- `govulncheck` (run under `GOTOOLCHAIN=go1.26.0`, the version the tool was
  built with) reported eighteen standard-library findings, every one fixed in
  a go1.26 point release; the installed binaries were built with go1.27.1, so
  none applies to them. No finding touches `github.com/pierrec/lz4/v4`
  v4.1.29, the one dependency this spec added.
- The diff carries no credentials, no personal paths and no Steam id; the
  golden tests read those from the environment and skip without them.
- Nothing is logged from a save's contents beyond the file name and sizes.

### In-game check (2026-09-15, game buildid 25233815)

The user ran the installed 0.2.0 build against slot 9 and loaded the game;
their report was "basic functionality works". Recorded as the AC10 result on
that word; no further detail was given.
