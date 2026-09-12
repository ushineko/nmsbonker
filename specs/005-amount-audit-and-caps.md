# Spec 005 — Reward-amount audit in the build report, and caps on the multiplier tweaks

## Status: INCOMPLETE

## Context

An in-game observation (2026-09-11): salvaged data stacks in the millions. The
cause was **compounding**: a third-party library script (BetterRewards,
configured by its own header at resources ×25, units ×75, nanites ×55,
salvaged data ×250, mission rewards ×750) multiplied reward amounts, and the
built-in tweaks multiplied again on top, in build order. Measured on the
merged `REWARDTABLE`: `BP_SALVAGE` 2–4 → 1,250,000–2,500,000 (×625,000); 206
material rewards at ×250 instead of ×10; four money rewards saturated at the
int32 ceiling (2,147,483,647). The build reported every one of those edits as
`OK`, because each edit was individually correct. The user disabled the
compounding scripts; afterwards every material reward is exactly ×10 and every
money reward exactly ×5.

Two gaps to close in the tool:

1. **The report cannot see a bad outcome.** It counts edits; it does not look
   at resulting values. It must audit the merged amounts and name the scripts
   that contributed.
2. **The multiplier tweaks have no ceiling.** A multiply-in-place op will
   happily produce a value the game cannot hold in a stack.

Facts to rely on:

- Reward blocks in `METADATA/REALITY/TABLES/REWARDTABLE.MBIN` (and
  `EXPEDITIONREWARDTABLE.MBIN`) are `<Property name="GcRewardSpecificProduct">`,
  `GcRewardSpecificSubstance`, `GcRewardMoney` (with `Currency` Units /
  Nanites / Specials), `GcRewardMultiSpecificProducts`,
  `GcRewardMultiSpecificItems`, each carrying `AmountMin`/`AmountMax`
  and an item `ID`/`Substance` or `Currency`. The enclosing reward entry's
  `Id` is the nearest preceding `<Property name="Id" value="…">` at a
  shallower depth.
- Stock stack caps are 9,999 for substances and mostly 9,999 (some 5/20/50)
  for products; the `BigStacks` tweak raises them to 999,999 / 99,999 via
  `DIFFICULTYCONFIG.MBIN` (`SubstanceStackLimit`, `ProductStackLimit`).
- The engine (`internal/mxml`) already clamps integers to int32; a clamped
  value is a symptom, never intended.
- The merge is `merge(pristine, items)` in `internal/build`; blocks are
  applied in build order and the pristine lines are available, so per-mod
  attribution can be computed by re-merging cumulatively for the audited
  files only (REWARDTABLE is ~9.6 MB of text; a handful of cumulative passes
  costs about a second).

## What this is not

- Not a change to reference-engine semantics: golden stages A/B/C must still
  pass unchanged. Caps are a new, opt-in block option that no reference script
  uses.
- Not an automatic fix: the audit reports and warns; it never edits a
  third-party script or disables a mod by itself.
- Not a general "balance" system; thresholds are simple and configurable.

## Requirements

### R1 — Amount audit (`internal/build/audit`)

- R1.1 After a target is merged and before it is compiled, if its internal
  path is one of the audited tables (`REWARDTABLE`, `EXPEDITIONREWARDTABLE`),
  parse pristine and merged lines into reward blocks (wrapper kind, entry Id,
  item/currency, AmountMin/Max). Pair blocks by order **only when the block
  counts match**; when structural edits changed the count, pair by
  `(entryId, kind, item)` sequence and report the unmatched remainder as
  "not auditable (structure changed by ADD/REMOVE)".
- R1.2 A block is **flagged** when any of: merged `AmountMax` >
  `audit.max_product` (products, default 99,999), > `audit.max_substance`
  (substances, default 999,999), > `audit.max_units` (default 100,000,000),
  > `audit.max_nanites` (default 1,000,000), > `audit.max_specials`
  (quicksilver, default 100,000); merged/pristine ratio > `audit.max_ratio`
  (default 100) with pristine > 0; or merged equals 2,147,483,647 (int32
  saturation, always flagged). Thresholds live in `config.audit.*` and are
  settable via `config set`.
- R1.3 **Attribution**: for a flagged block, list every mod whose blocks
  changed that block's `AmountMin`/`AmountMax`, with the value after that
  mod's pass (e.g. `BetterRewards ×250 → 1000, BetterRewards ×250 → 250000,
  ChestAndLootMaterials10x ×10 → 2500000`). Implement by re-merging the
  audited target cumulatively, one mod at a time in build order, and diffing
  the flagged blocks' amounts after each pass. Only flagged blocks are
  attributed, so the cost is bounded.
- R1.4 Output: `report.Result.Audit` = `{Flags []Flag; Unauditable int;
  Thresholds}`; `Flag{Table, EntryID, Kind, Item, PristineMin/Max,
  MergedMin/Max, Ratio, Reasons []string, Contributors []Contribution}`.
  `BUILD_REPORT.md` gains a section `## Amount audit` right after the totals:
  either `No reward amount exceeds the configured limits.` or a table (Table,
  Entry, Item, Stock, Built, ×, Why, Contributors) sorted by ratio desc,
  capped at 50 rows with a count of the rest, followed by one paragraph that
  says what to do (disable or re-tune the compounding script; the tweak caps
  from R2). `report.json` carries the full list.
- R1.5 The CLI `build` prints an `amount audit:` summary line (`clean` or
  `N flagged; see report`) and exits 0 regardless (an audit flag is a warning);
  `nmsbonker audit [--json]` re-runs the audit over the latest workspace merge
  (`<MOD_NAME>.work/mxml`) without rebuilding, so thresholds can be changed
  and re-checked in a second.
- R1.6 Events: a `WARN` line per flagged block in the build log
  (`AUDIT <entry>/<item> 4 -> 2500000 (x625000): BetterRewards, BetterRewards,
  ChestAndLootMaterials10x`) so the GUI log and the CLI stream show it live.

### R2 — Caps on multiplying edits (engine + tweaks)

- R2.1 New optional block key **`CAP`** (number) honoured by: `WRAPPER_MULT`,
  `CURRENCY_MULT`, and `VALUE_CHANGE_TABLE` with `MATH_OPERATION`. After the
  arithmetic, the result is clamped to `≤ CAP` before `FormatNum`. Absent or
  `0` means no cap. Reference scripts never set it, so golden output is
  unchanged (a unit test asserts that a block without `CAP` is byte-identical
  to the pre-change behaviour on a synthetic snippet, and the golden suite
  proves it at scale). Optional `FLOOR` is **not** added (no use case).
- R2.2 The cap is recorded in the event detail (`… x10 cap 50000 …`) and
  counted (`capped N values`) so the report says when a cap actually bit.
- R2.3 Built-in tweaks gain cap parameters with `@param` headers, defaults
  chosen to be generous but game-safe:
  - `ChestAndLootMaterials10x`: `LOOT_CAP` default 50000 (products and
    substances).
  - `MaterialYield10x`: `YIELD_CAP` default 50000.
  - `MoneyAndNanites5x`: `UNITS_CAP` default 50000000, `NANITES_CAP` default
    250000.
  - `NaniteRewardBuff`: `NANITES_CAP` default 250000.
  - `SpaceMiningBoost`: `AST_CAP` default 5000.
  - `ScanValue50x`: `SCAN_CAP` default 5000000.
  - `MissionStandingBuff`: `STANDING_CAP` default 500.
  - `LearnMoreWords`: `WORDS_CAP` default 25.
  Caps apply to whatever value the block sees, so with a compounding script
  ahead of ours the cap is the final ceiling; the audit still flags the ratio.
- R2.4 R1.5 of spec 004 (built-ins at default values reproduce the reference
  dump `MODIFICATIONS`) must be updated: the reference dump predates caps, so
  the test compares after stripping `CAP` keys, and says so.

### R3 — Front ends

- R3.1 GUI Report section: an `Amount audit` block above the mod table:
  green `marker` + "No reward amount exceeds the configured limits" or a
  warn/bad marker with a table of flags (Entry, Item, Stock → Built, ×,
  Contributors) and the advice paragraph; `Copy audit` button. Overview's
  library card shows `last build: N amount flags` with a warn marker when
  N > 0.
- R3.2 Tweaks section renders the new cap parameters like any other parameter
  (they arrive via `@param`, so this should need no new UI code — verify).
- R3.3 Settings section: an `Audit limits` form group for the five thresholds
  and the ratio, with reset-to-defaults. CLI: `config set audit.max_product
  …` works already through R1.2.
- R3.4 Parity test allow-list updated for `audit`.

### R4 — Library hygiene signals in `mods check` / Mods section

- R4.1 `mods check` (and the Mods `Details…` dialog) reports for each script,
  from the last report: verdict, applied/skipped, the not-found keys, and a
  one-line **effectiveness note** derived mechanically: `no effective edits`
  (0 applied), `mostly failing` (skipped ≥ applied), `structural edits
  skipped`, `overlaps built-in <name>` when the script targets the same file
  *and* keys as an enabled built-in tweak (compare the sets of
  `(file, VCT key)` pairs and wrapper names). This is information only.

## Acceptance Criteria

- [ ] AC1 Golden stages A/B/C unchanged and passing; the built-in
  dump-equality test passes with `CAP` stripped.
- [ ] AC2 With the compounding script enabled in a scratch config
  (re-enable `BetterRewards` there only), `build` flags `BP_SALVAGE`
  (ratio 625000, contributors `BetterRewards ×2, ChestAndLootMaterials10x`),
  the 206 ×250 entries and the four int32-saturated money entries; the
  report's audit table and `nmsbonker audit --json` agree.
- [ ] AC3 With the user's current config (compounding scripts disabled), the
  audit is clean at default thresholds, and setting `audit.max_ratio 5` flags
  the ×10 loot entries — proving thresholds are live without a rebuild via
  `nmsbonker audit`.
- [ ] AC4 A synthetic MXML test shows `CAP` clamping a `WRAPPER_MULT`,
  a `CURRENCY_MULT` and a `VALUE_CHANGE_TABLE` `*` result, and that an absent
  `CAP` leaves output byte-identical to the previous engine.
- [ ] AC5 Setting `LOOT_CAP` to 1000 via `tweaks set` produces a merged
  `REWARDTABLE` whose largest product/substance reward is exactly 1000 and the
  report says how many values were capped.
- [ ] AC6 GUI Report shows the audit block in both states; Settings edits the
  thresholds; Tweaks shows the cap sliders; headless tests cover the audit
  block rendering; parity test passes.
- [ ] AC7 `mods check` prints the effectiveness notes; a script with 0 applied
  edits says `no effective edits`.
- [ ] AC8 `make lint`, `make test`, gated suites, both grep gates clean; no
  personal path or superseded term anywhere.

## Risks & Assumptions

- Attribution by cumulative re-merge assumes the block pairing survives each
  pass; when a mod's structural edit shifts the count mid-sequence, attribute
  what can be paired and mark the rest.
- Default caps are judgement calls (generous relative to stock, far below
  anything that breaks stacks); all are sliders.
- Rollback: `git revert`; caps default on but only bite above 50,000, which
  no single ×10/×5 built-in reaches from stock values.
