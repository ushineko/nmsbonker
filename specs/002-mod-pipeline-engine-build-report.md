# Spec 002 — Mod pipeline: Lua script loader, MXML edit engine, cache, build, report, deploy

## Status: INCOMPLETE

## Context

Phase 2 of 4. On top of spec 001 this delivers the actual mod pipeline, a
faithful Go port of the legacy Python builder
(`~/Games/nms-modding/builder/nms_build_mods.py`, `build_cache.py`,
`dump_mod.lua`, `build.sh`). The legacy pipeline is the behavioural oracle:
golden fixtures generated from it on 2026-09-11 live in
`~/Games/nms-modding/golden/` (game-derived, never committed):

```
golden/
  mods.conf            # 27 scripts, all enabled, in build order
  manifest.json        # legacy cache manifest: SOURCE(upper) -> {internal, pak, mxml}
  dumped/<mod>.json    # dump_mod.lua output per script (the Lua stage oracle)
  merged/<INTERNAL>    # merged MXML text per target after ALL blocks applied (100 files)
  index.json           # per-target block counts + per-mod applied/skipped/notfound
  report_lines.txt     # the engine's OK/WARN lines, in order (504 lines)
  BUILD_REPORT.md      # the legacy end-to-end report (verdicts per mod)
```

The pristine MXML inputs referenced by `manifest.json` (`mxml` paths relative to
`~/Games/nms-modding/`) are the legacy cache; the golden merge used them
directly. The scripts are in `~/Games/nms-modding/lua-src/` (27 files; 11 are
this project's own tweaks, 16 are third-party and must not be committed).

How the legacy engine works, in the detail the port must reproduce
(`nms_build_mods.py`, read it alongside this spec):

1. **Script → change tables.** `dump_mod.lua` doubles every backslash in the
   source (so `"METADATA\REALITY\X.MBIN"` is a valid literal), `load`s it with the
   full standard library, runs it, and serialises the global
   `NMS_MOD_DEFINITION_CONTAINER` to JSON (integral numbers as `%d`, others as
   `tostring`, empty table as `[]`).
2. **Targets.** For each enabled mod in `mods.conf` order, for each
   `MODIFICATIONS[].MBIN_CHANGE_TABLE[]`, `MBIN_FILE_SOURCE` (string or list of
   strings) × each `EXML_CHANGE_TABLE[]` block appends `(modname, src, block)` to
   `targets[norm(src)]` where `norm` = backslash→slash, upper-case. Targets are
   processed in first-seen order.
3. **Per target**: the pristine MXML is split on `\n` into lines; every block is
   applied in sequence by `apply_block`; the result joined with `\n` and
   compiled. If compile fails and some blocks carry `ADD`/`REMOVE`, retry from
   pristine with only the non-structural blocks; if that compiles, the file ships
   "degraded" and the structural mods are listed. Otherwise the file is dropped
   and every contributing mod gets a `RECOMPILE FAILED … DROPPED` warning.
4. **`apply_block` semantics** (line-based, tab-indentation-aware):
   - `PRECEDING_KEY_WORDS` (string or list): each keyword advances `start` to the
     line after its first match searching `"kw"` (quoted) first, then bare `kw`.
   - `CURRENCY_MULT {CURRENCY, MULT}`: for every `<Property name="GcRewardMoney">`
     block whose lines contain `value="<CURRENCY>"`, multiply `AmountMin`/`AmountMax`
     via `fmt_num(old*mult, old, None)`; one OK line; return.
   - `WRAPPER_MULT {WRAPPER, MULT, KEYS=[AmountMin,AmountMax]}`: for every
     `<Property name="<WRAPPER>">` block, multiply the KEYS inside; one OK line;
     return.
   - `REPLACE_TYPE=="ALL"` + `SPECIAL_KEY_WORDS` + `VALUE_CHANGE_TABLE`, without
     `FOREACH_SKW_GROUP`/`ADD`/`REMOVE`: iterate every SKW match from `start`;
     scope = `SECTION_UP_TO` marker (look back ≤12 lines; skip match if absent),
     else `SECTION_UP` levels up by tab depth, else the matched line's own section
     (or just the line if self-closing); apply the VCT (first occurrence of each
     key within scope, `MATH_OPERATION`/`INTEGER_TO_FLOAT` aware); `pos =
     max(section_end, match+1)`; one OK line naming the keys, op and match count,
     or one WARN if zero matches; return.
   - Otherwise, for each SKW group (`FOREACH_SKW_GROUP` list of lists, else the
     single `SPECIAL_KEY_WORDS`, else `[None]` = whole file from `start`): locate
     the anchor by matching keywords in order (each search starts at the previous
     hit, quoted then bare); missing → WARN `SKW … not found` and continue to
     the next group. Scope = `SECTION_UP` walk-up by tab depth, else the anchor's
     section (`anchor..close_index`, or just the anchor line if self-closing).
     Then: `REMOVE` truthy → delete `lines[s:e]`, OK, **return**; `ADD` → insert
     the ADD text's lines (split on `\n`) at `e`, OK, **continue** to next group;
     `VALUE_CHANGE_TABLE` → for each `[key, val]`: find `name="key"` on
     `<Property` lines in scope (first only, or all if `REPLACE_TYPE=="ALL"`);
     none → WARN with `notfound=key`; else new value = math result or `str(val)`;
     OK line `key -> nv (Nx) in file`.
   - `fmt_num(result, old, itof)`: integer-ness is `"." not in old`;
     `itof=="PRESERVE"` keeps old kind, other truthy forces float, falsy keeps
     old kind. Integers: `round()` (Python banker's rounding on `.5`!) then clamp
     to int32; floats: `"%f"` then strip trailing zeros and a trailing dot,
     `"0"` if empty. Math exceptions (non-numeric old) fall back to `str(val)`.
   - `get_val`/`set_val` operate on the first `value="…"` on the line.
   - Keys the legacy engine reads: `PRECEDING_KEY_WORDS`, `SPECIAL_KEY_WORDS`,
     `FOREACH_SKW_GROUP`, `SECTION_UP`, `SECTION_UP_TO`, `VALUE_CHANGE_TABLE`,
     `MATH_OPERATION`, `INTEGER_TO_FLOAT`, `REPLACE_TYPE`, `ADD`, `REMOVE`,
     `CURRENCY_MULT`, `WRAPPER_MULT`. Present in the scripts but **ignored**:
     `LINE_OFFSET`, `VALUE_MATCH`, `VALUE_MATCH_TYPE`, `VALUE_MATCH_OPTIONS`,
     `SECTION_ACTIVE`, `PAK_FILE_SOURCE`, `NMS_VERSION`, `MOD_*`. The port
     ignores them identically (parity) but records them per block as
     `Unsupported` so the report can say so.
5. **Report**: per mod `applied`, `skipped`, `notfound` keys, files built;
   verdicts: `NOT BUILT` (0 applied), `PARTIAL` (in a degraded/dropped file),
   `WORKING*` (has ADD/REMOVE and compiled), `WORKING~` (skipped > 0), else
   `WORKING`. Sorted by verdict rank then name. `BUILD_REPORT.md` has the
   header facts, legend, table, the degraded-files section, and the fixed
   how-to-fix prose (copy the legacy text).
6. **Output**: `MODS/<MOD_NAME>/<INTERNAL_UPPER>`; after all targets, every
   `GC*GLOBALS*.MBIN` and `GCCREATUREGLOBALS.MBIN` at the mod root is also copied
   into `<MOD_NAME>/GLOBALS/` (harmless duplicate, matches the known-good layout).
7. **Cache** (`build_cache.py`): sources resolved against the pak index by full
   lower-cased path, then by basename; extraction placement uses the index's
   internal path, not hgpaktool's directory (it invents a `GLOBALS/` prefix).
   The MXML is stored at `cache/mxml/<INTERNAL_UPPER minus ext>.MXML`. Rebuilt
   wholesale when `NMSARC.globals.pak` is newer than the manifest.

Legacy results for reference (BUILD_REPORT.md, 2026-09-11): 100 MBINs built,
0 dropped; 16 WORKING, 7 WORKING~, 1 WORKING*, 2 PARTIAL (BetterRewards,
Crashed Freighter Loot — REWARDTABLE structural edits rejected), 1 NOT BUILT
(SpeedIncreaseGrowthAndHarvest).

## What this is not

- Not a new edit engine. Parity first; improvements (e.g. honouring
  `VALUE_MATCH`) are a later spec with new golden fixtures.
- Not the GUI (003), not tweak parameters, GCMODSETTINGS writing, rollback or
  migration (004). Deploy here is the minimal "install the built folder".
- Not AMUMSS's own `.pak`-producing mode; output is loose files only (the only
  mode the game's 5.50+ loader needs).

## Requirements

### R1 — Script loader (`internal/modscript`)

- R1.1 Embedded Lua via `github.com/yuin/gopher-lua` (Lua 5.1 semantics; the 27
  reference scripts use only `table.insert`, `table.concat`, `string.rep`,
  arithmetic and string concatenation). Preprocess exactly like `dump_mod.lua`:
  replace every `\` with `\\` before compiling.
- R1.2 Sandbox: open `base`, `table`, `string`, `math` only; `os`, `io`, `debug`,
  `package`/`require`, `load`/`loadstring`/`dofile`/`loadfile` are absent;
  execution is bounded by a context timeout (default 5 s) and gopher-lua's
  instruction-count hook; a script that fails to compile/run returns an error
  naming the file, the Lua message, and the stage (`load` / `exec` /
  `no container`), mirroring the dumper's exit codes.
- R1.3 Result model:
  ```go
  type Definition struct {
      Name          string            // script basename without .lua
      Path          string
      ModFilename   string            // MOD_FILENAME
      Author        string            // MOD_AUTHOR (+ LUA_AUTHOR if distinct)
      Description   string            // MOD_DESCRIPTION
      NMSVersion    string            // NMS_VERSION
      Modifications []Modification    // in order
      Globals       map[string]Value  // top-level numeric globals (spec 004 uses)
  }
  type Modification struct { PakFileSource string; Changes []MBINChange }
  type MBINChange   struct { Sources []string; Blocks []Block }
  type Block struct {
      PrecedingKeyWords []string; SpecialKeyWords []string; ForEachSKWGroup [][]string
      SectionUp int; SectionUpTo string
      ValueChanges []ValueChange          // {Key string; Value Value}
      MathOperation string; IntegerToFloat IntegerToFloat // Off | Force | Preserve
      ReplaceType string
      Add string; HasAdd bool; Remove bool
      CurrencyMult *CurrencyMult; WrapperMult *WrapperMult
      Unsupported []string                // keys present but not interpreted
      Raw map[string]any                  // as-decoded, for `check --json`
  }
  ```
  `Value` is a string/number/bool sum type whose `String()` reproduces the
  Python `str()` the legacy engine saw after JSON decoding: booleans `True`/`False`
  (they never occur in VCTs in practice; keep for parity), integral numbers as
  `%d`, other numbers as Python `repr(float)` (shortest round-trip; exponent
  form below 1e-4 and at/above 1e16; `1e-05` style two-digit exponent).
  `Float()` parses either. Mixed-type VCT entries (`{"Key", "string"}` and
  `{"Key", 5}`) are both common.
- R1.4 `modscript.Load(ctx, path) (*Definition, error)` and
  `modscript.DumpJSON(*Definition) []byte` producing the same shape as
  `dump_mod.lua` (used by the golden test and `check --json`). JSON comparison
  is semantic (parsed), not textual: table key order in Lua is undefined.
- R1.5 A `lua_test.go` covers: backslash doubling, a script with a syntax error,
  one that never defines the container, one using `string.rep`/loops to build
  an `ADD` payload, integral vs fractional number rendering, and the sandbox
  (a script calling `os.execute`/`io.open` errors out rather than running).

### R2 — Edit engine (`internal/mxml`)

- R2.1 `mxml.Apply(lines []string, blk *modscript.Block, ctx ApplyContext) (lines []string, events []Event)`
  is a faithful port of `apply_block` including every branch, quirk and message
  listed in Context §4. `Event{Kind OK|WARN|INFO; Mod, Detail, File string; NotFound string}`
  and `Event.Line()` renders exactly the legacy text:
  `"   OK  {mod}: {detail}"` / `"  WARN {mod}: {detail}"` / `"       {detail}"`.
  Python list reprs inside details (`['AmountMin', 'AmountMax']`,
  `['Id', 'WORD']`) are reproduced by a `pyList([]string)` helper.
- R2.2 Helpers are exported for tests and reuse: `NTabs`, `CloseIndex`,
  `FindKW`, `FindProp`, `GetVal`, `SetVal`, `FormatNum(result float64, old string, itof)`
  with **round-half-to-even** to match Python's `round()`.
- R2.3 Unit tests use small hand-written MXML snippets (no game data) and cover:
  each branch of `Apply`; `SECTION_UP` walking past self-closing lines; ADD
  inserting after the section; REMOVE deleting the whole section; `REPLACE_TYPE
  ALL` within scope; the ALL+SKW multi-section path incl. `SECTION_UP_TO`
  lookback failure; `FormatNum` on `.5` cases (`2.5→2`, `3.5→4`), int32 clamp,
  float trailing-zero stripping, `PRESERVE`/`FORCE`; math on a non-numeric old
  value falling back to `str(val)`; PRECEDING_KEY_WORDS as string vs list.

### R3 — Pristine cache (`internal/build/cache`)

- R3.1 Location: `cache_dir/game/<buildid>/` with `raw/<internal path>` (as in
  the pak, lower-case) and `mxml/<INTERNAL UPPER minus ext>.MXML`, plus
  `manifest.json` entries `{source, internal, pak, pakSize, pakMTime,
  compilerVersion, mxml}`.
- R3.2 `Ensure(ctx, sources []string) (Resolved map[string]Entry, misses []string)`:
  resolve each source via `hgpak.Index` (full path, then basename fallback,
  recorded as `resolvedBy`); extract with the native reader; decompile with
  `mbin.Compiler`; entries are reused when the pak `(size, mtime)` and compiler
  version are unchanged; `force` re-extracts everything. Extraction and
  decompilation run concurrently up to `config.parallel`. Progress events name
  the file.
- R3.3 A miss (not in any pak) or a decompile failure is reported per source and
  does not abort the build; the affected blocks produce the legacy
  `no cached MXML for …` WARN.

### R4 — Build orchestrator (`internal/build`)

- R4.1 `build.Plan(defs []*Definition, order []config.ModEntry) *Plan` computes
  targets in first-seen order with their `(mod, source, block)` lists and the
  `complexFlag` set (mods with any ADD/REMOVE).
- R4.2 `build.Run(ctx, Plan, Options) (*Result, error)` implements Context §3,
  §5, §6: merge, write `workspace_dir/<MOD_NAME>.work/mxml/<INTERNAL>.MXML`
  (kept for inspection), compile into a per-target temp dir, degrade/drop
  logic, place the MBIN at `workspace_dir/<MOD_NAME>/<INTERNAL_UPPER>`, GLOBALS
  mirror. Targets compile concurrently up to `config.parallel` **but events and
  the report are emitted in plan order** (buffer per target) so
  `report_lines` are deterministic.
- R4.3 The previous workspace output is renamed to `<MOD_NAME>.prev` before a
  run and removed after a successful one.
- R4.4 `Result` carries: per-target outcome (`built` | `degraded` | `dropped` |
  `no-source`), per-mod stats and verdict, totals, timings (cache, merge,
  compile), compiler version, game buildid, and `Events`. `report.Markdown(Result)`
  renders `BUILD_REPORT.md` with the legacy structure (header bullets, legend,
  table, degraded section, how-to-fix prose, NOT BUILT prose) plus one new
  line `- Unsupported script keys ignored: …` when any block had them.
  `report.JSON` writes `report.json` beside it. Both land in
  `workspace_dir/<MOD_NAME>/../reports/latest/` (and a timestamped copy).
- R4.5 Build never writes under the game directory.

### R5 — Deploy (minimal; `core.Deploy`)

- R5.1 Precondition: a successful build exists. If `GAMEDATA/MODS` is a symlink,
  refuse with a message explaining the legacy setup and pointing at `migrate`
  (spec 004) or `--replace-symlink`, which removes only the symlink (never its
  target) and creates a real directory.
- R5.2 Copy `workspace_dir/<MOD_NAME>/` to `GAMEDATA/MODS/<MOD_NAME>.tmp-<pid>`,
  archive any existing `GAMEDATA/MODS/<MOD_NAME>` to
  `$XDG_DATA_HOME/nmsbonker/archive/<MOD_NAME>-<UTC timestamp>/`, then rename the
  temp dir into place. Report the archive path. `GCMODSETTINGS.MXML` is read and
  its `DisableAllMods`/entry state is *reported* (with a warning if the game has
  disabled mods), not written — that is spec 004.

### R6 — Mod library and ordering (`core`)

- R6.1 The library is `library_dir/*.lua`; `config.mods` is the ordered
  `{name, enabled}` list. `core.ListMods` reconciles: scripts on disk but not in
  config are appended enabled=false with a notice; config entries with no file
  are reported `missing` (kept, so a temporarily moved file keeps its slot).
- R6.2 Operations: `AddMod{Path}` (copies into the library; refuses to overwrite
  unless `Replace`), `RemoveMod{Name, DeleteFile bool}`, `SetModEnabled`,
  `MoveMod{Name, To int}`, `ImportDir{Dir, Pattern}`, `CheckMods` (loads every
  enabled script, returns per-script definition summary: targets, block count,
  unsupported keys, load errors).
- R6.3 `core.Build{Recache, Deploy bool}` runs R3+R4 (+R5 when `Deploy`) and
  streams events; `core.Report` returns the latest `report.json`.

### R7 — CLI additions

- `mods list|add PATH…|remove NAME [--delete]|enable NAME…|disable NAME…|move NAME POS|import DIR [--pattern]|check [--json]`
- `build [--recache] [--deploy] [--mod-name NAME]` (prints the event stream,
  then the report table; exit 1 if any target was dropped, 0 otherwise — a
  `NOT BUILT` mod alone is exit 0 with a warning, matching the legacy script's
  tolerance),
- `deploy [--replace-symlink]`, `report [--json]`.

### R8 — Golden parity test (integration, `internal/build/golden_test.go`)

- R8.1 Skips unless `NMSBONKER_GOLDEN_DIR` and `NMSBONKER_LEGACY_DIR` are set.
- R8.2 Stage A (Lua): for every script named in `golden/mods.conf`, load
  `<legacy>/lua-src/<name>.lua` and assert `DumpJSON` is semantically equal to
  `golden/dumped/<name>.json` (numbers compared as float64; strings exact).
- R8.3 Stage B (engine): build the plan from the golden `mods.conf` order; for
  each target in `golden/index.json`, read the pristine MXML from
  `<legacy>/<manifest.mxml>`, apply all blocks, and assert the joined text is
  **byte-identical** to `golden/merged/<INTERNAL>`; collect all event lines and
  assert they equal `golden/report_lines.txt` line by line.
- R8.4 Stage C (end to end, additionally needs `NMSBONKER_GAME_DIR` and an
  installed compiler): `core.Build` against the real game with the library
  pointed at `<legacy>/lua-src` and order from `golden/mods.conf` produces the
  same set of 100 output paths as `golden/BUILD_REPORT.md` implies (count) and
  the same per-mod verdicts as its table. Time budget: < 3 min warm cache.

### R9 — Tests beyond golden

- Unit tests per R1.5 and R2.3; `Plan` ordering; verdict derivation table
  test; report rendering snapshot on a synthetic result; deploy symlink refusal
  and archive behaviour on `t.TempDir()` fakes.

## Acceptance Criteria

- [ ] AC1 Golden Stage A and B pass (27 scripts, 100 targets byte-identical,
  504 report lines identical).
- [ ] AC2 Golden Stage C passes: 100 MBINs, 0 dropped, verdicts equal to the
  legacy `BUILD_REPORT.md` (16 WORKING, 7 WORKING~, 1 WORKING*, 2 PARTIAL,
  1 NOT BUILT).
- [ ] AC3 `nmsbonker mods import ~/Games/nms-modding/lua-src` then
  `nmsbonker build` succeeds on this machine with no Wine/Python/Lua involved
  (verify: `command -v wine lua python3` absence is not required, but the
  process tree of the build contains only `nmsbonker` and
  `MBINCompiler-linux-dotnet10`).
- [ ] AC4 `nmsbonker deploy` refuses on the current symlinked `GAMEDATA/MODS`
  with an actionable message; with `--replace-symlink` it installs the folder
  and archives nothing (nothing existed) — verified on a temp fake game dir in
  tests, and on the real install only after the user has confirmed migration
  (spec 004), so the real-install check is **deferred to spec 004**.
- [ ] AC5 Build output tree matches the legacy layout: root globals +
  `GLOBALS/` mirror + `METADATA/…` + `MODELS/…`; `diff -r` against
  `~/Games/nms-modding/MODS/COSMOS COMBINE` shows only files whose MBIN differ
  because of the embedded compiler version/GUID, not different file sets
  (document the diff command and outcome).
- [ ] AC6 `make lint`, `make test` pass headless; unit coverage of
  `internal/mxml` ≥ 85 % (it is the correctness core; the number is a floor for
  this package only).
- [ ] AC7 A script calling `os.execute` in the library is rejected at
  `mods check` with a clear error and does not execute.
- [ ] AC8 Repo contains no `.lua` from `lua-src` and no MXML/MBIN (grep +
  `.gitignore` backstop).

## Risks & Assumptions

- Concurrent compilation changes nothing semantically (each target is
  independent) but could expose MBINCompiler's own log file being shared;
  mitigate with `-Q` (no log) once the error path is proven to still capture
  stderr, or serialise on failure to re-run once with logging for the message.
- Python `round()` banker's rounding vs Go `math.RoundToEven`: covered by tests
  and the golden set (MaterialYield10x/ItemValueBoost exercise `.5` cases).
- gopher-lua number formatting differs from Lua 5.5 `tostring` for some floats
  (`%.14g`); the golden Stage A compares numerically, and the engine formats
  from float64 via the Python-repr helper, so textual differences upstream do
  not leak into the merge.
- Rollback: `git revert`; deploy archives what it replaces.

## Alternatives Considered

- Calling the system `lua` binary like the legacy dumper: rejected — a
  single-binary desktop app should not depend on an interpreter, and a sandbox is
  wanted for third-party scripts.
- An XML DOM-based engine: rejected for this phase — parity with the line-based
  legacy behaviour (including its quirks) is the acceptance oracle.
