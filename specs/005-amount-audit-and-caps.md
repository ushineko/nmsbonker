# Spec 005 — Reward-amount audit in the build report, and caps on the multiplier tweaks

## Status: COMPLETE

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
- R2.5 `BigStacks` gains `ANTIMATTER_HARVESTER_CAP` (default 20, 0 = leave
  stock): sets `MaxCapacity` in
  `MODELS/PLANETS/BIOMES/COMMON/BUILDINGS/PARTS/BUILDABLEPARTS/TECH/ANTIMATTERHARVESTER/ENTITIES/ANTIMATTERHARVESTER.ENTITY.MBIN`
  (stock `-1` means "a full stack", which with raised stack caps is 99,999
  antimatter per harvester). This replaces the one useful edit of a library
  script the audit retired. Scope the edit the way that script did
  (`PRECEDING_KEY_WORDS` `GcMaintenanceElement`), or by a `WRAPPER_MULT`-style
  deterministic locate if that is cleaner; a unit test on a synthetic snippet
  and the real build must show exactly one `MaxCapacity` changed.
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

- [x] AC1 Golden stages A/B/C unchanged and passing; the built-in
  dump-equality test passes with `CAP` stripped. Stage B: 100 of 100 targets
  byte-identical and all 504 report lines matching, unchanged. Stage C: 100
  built, 0 dropped, every one of the 27 verdicts equal to the reference table.
  `TestBuiltInsDecodeAsTheReferenceCopiesDo` passes with `CAP` stripped from
  both sides and `ANTIMATTER_HARVESTER_CAP` set to 0.
- [x] AC2 With `BetterRewards` re-enabled in a scratch config and every cap
  set to 0 (the state the incident happened in), the build flags 211 of 4064
  amounts, including `BP_SALVAGE` 2-4 → 1,250,000-2,500,000 at ×625,000 with
  contributors `BetterRewards x250 -> 1000, BetterRewards x250 -> 250000,
  ChestAndLootMaterials10x x10 -> 2500000`, exactly **206** entries at ratio
  ×250, and exactly **four** int32-saturated money entries (`R_CV_HIGH`,
  `R_MB_HIGH`, `R_CV_MEGA`, `R_MB_MEGA`, all Units). The report's 211 flags and
  `nmsbonker audit --json`'s 211 compare equal field by field, contributors
  included.
- [x] AC3 With the current mod list (compounding scripts disabled), mirrored
  into a scratch config, the build reports `amount audit: clean` at default
  thresholds over 4064 blocks. `config set audit.max_ratio 5` followed by
  `nmsbonker audit` — no rebuild — flags 2270 amounts, every one of them a ×10
  loot entry attributed to `ChestAndLootMaterials10x`, in **109 ms**.
- [x] AC4 `internal/mxml/cap_test.go`: a cap clamps a `CURRENCY_MULT`, a
  `WRAPPER_MULT` and a `VALUE_CHANGE_TABLE` `*`, and with no cap the merged
  bytes and the report line are asserted in full against what the engine
  produced before the key existed. Also covered: a cap that never bites says
  nothing about it, a negative cap is no cap, a verbatim (no-`MATH_OPERATION`)
  value is never clamped, and the int32 clamp and the cap both hold.
- [x] AC5 `tweaks set ChestAndLootMaterials10x LOOT_CAP 1000` then `build`:
  the merged `REWARDTABLE`'s largest `GcRewardSpecificProduct` amount is
  **exactly 1000** across 1689 blocks and its largest
  `GcRewardSpecificSubstance` amount is **exactly 1000** across 359, and the
  report says `capped values: 191`.
- [x] AC6 Verified on screen in both states: a green marker with "No reward
  amount exceeds the configured limits — 4064 block(s) checked (from the last
  build)", and a red one with the flag table, the 161-not-shown line and the
  advice paragraph. Settings shows the `Audit limits` group with its six fields
  and Reset; Tweaks draws the cap parameters as ordinary sliders with no new UI
  code (R3.2 verified, "Largest mined amount" and "Largest asteroid resource
  amount" among them). `internal/gui/views_audit_test.go` covers the three
  states, the row contents, the clipboard form, the markers and the Settings
  group; the parity guard passes with `audit` claimed.
- [x] AC7 `mods check` gained a `LAST BUILD` column (`NOT BUILT 0/1`) and the
  note column carries the effectiveness signal. A script whose only key the
  game no longer has reports `no effective edits; keys not found: …`; in the
  AC2 configuration `BetterRewards` reports `structural edits skipped; overlaps
  built-in ChestAndLootMaterials10x; overlaps built-in LearnMoreWords`.
- [x] AC8 `make lint` 0 issues; `make test` (race + parity tag) green;
  `go test -race -count=1 -tags parity ./...` with all three gated variables
  set, green including golden A/B/C; `make build` and `make build-gui` both
  produce binaries; `govulncheck -mode=binary` reports no vulnerabilities for
  either; both grep gates return nothing.

## Risks & Assumptions

- Attribution by cumulative re-merge assumes the block pairing survives each
  pass; when a mod's structural edit shifts the count mid-sequence, attribute
  what can be paired and mark the rest.
- Default caps are judgement calls (generous relative to stock, far below
  anything that breaks stacks); all are sliders.
- Rollback: `git revert`; caps default on but only bite above 50,000, which
  no single ×10/×5 built-in reaches from stock values.

---

## Status notes

Verified 2026-09-11 against Steam buildid `25233815`, MBINCompiler
**v7.02.0-pre1** (`dotnet10`), Go 1.27.1, golangci-lint v2.12.2, Fyne v2.8.1, on
KDE/Wayland.

**Nothing of the user's was touched.** Every command below ran with `--config`
pointing at a scratch settings file under a temporary root, with its own copy of
the mod library, its own workspace and its own pristine cache. The real
`config.json`, the real library and the real build output are as they were, and
nothing was deployed, rolled back or undeployed against the game — the only
thing read from the install is its `.pak` archives.

### The audit reads values; the caps stop them

The two halves answer different questions and both are needed. The audit says
"this reward came out at 2,500,000 and these three edits made it", which no
per-mod verdict can. The cap says "and it will not, whatever your library
does". A capped build is still flagged, deliberately: the ratio is the fact
about the library, and the cap only hides its effect.

### Deviations from the spec text, and why

- **The `## Amount audit` section sits after the header bullets and before the
  verdict legend.** R1.4 asks for it "right after the totals", and the totals
  are a bullet in the header block; a `##` heading inside that list would break
  it, and putting the section after the mod table would put the finding after
  the thing it contradicts. The legend belongs to the table it explains, so the
  audit goes above both.
- **A cap is rendered plainly in a report line, not as Python's `repr`.** The
  multipliers beside it print as `x5.0` because the golden fixtures were
  captured from a Python builder; a cap is a stack size that no reference script
  ever wrote, so `cap 50000` rather than `cap 50000.0`. The count follows as
  `(capped N)` and is omitted when the cap never bit.
- **Attribution is per edit block, not per mod pass.** R1.3 says "one mod at a
  time"; the spec's own example names `BetterRewards` twice, which only happens
  at block granularity, and AC2 asks for `BetterRewards ×2`. Per block is also
  *cheaper*, not dearer: the cumulative state after k edits is the running
  document, so the whole attribution is one extra merge with a look after each
  block rather than one re-merge per mod. Reading a flagged block costs a few
  lines while the line count matches the stock document's, and falls back to a
  full re-parse only after a structural edit has moved things.
- **`GcRewardMultiSpecificItems` carries `Amount` per nested item in this game
  version**, not `AmountMin`/`AmountMax` as R1.1 assumes. Each `Items` entry
  becomes one audited block with its own Id and its `MultiItemRewardType`
  deciding product or substance; treating the wrapper as one block would have
  audited the first entry of 1484 and ignored the rest.
- **BigStacks anchors the harvester edit on two keywords, not on
  `PRECEDING_KEY_WORDS GcMaintenanceElement`.** R2.5 suggests the latter, and it
  is wrong on this file: `ANTIMATTERHARVESTER.ENTITY.MBIN` holds *two*
  `GcMaintenanceElement` blocks with a `MaxCapacity` each — `MAINT_FUEL4` first,
  `ANTIMATTER` second — and the library script this replaces disambiguated with
  `SECTION_ACTIVE = {2}`, a key this engine ignores. A bare
  `PRECEDING_KEY_WORDS` would therefore have capped the fuel slot: one
  `MaxCapacity` changed, the wrong one, and nothing in the report able to say
  so. `SPECIAL_KEY_WORDS = {"GcMaintenanceElement", "ANTIMATTER"}` anchors on
  the second block and scopes the value change to it.
- **An `audit.*` limit of 0 restores the default rather than removing the
  limit.** A threshold of zero would flag every reward in the game, so it is
  never what somebody typing `0` meant.
- **Audit warnings are marked and left out of the per-mod tallies.** A flagged
  amount is not a skipped edit; counting it as one would turn a mod whose every
  edit landed into a `WORKING~` row about a value it may not have touched.
- **The overlap signal is deliberately coarse.** R4.1 asks for a comparison of
  `(file, VCT key)` pairs and wrapper names, and that is what it does — so
  `LearnMoreWords` and `ChestAndLootMaterials10x` are reported as overlapping,
  both editing `AmountMin`/`AmountMax` in `REWARDTABLE`, although their
  `SPECIAL_KEY_WORDS` anchors put them on different sections. It answers "these
  two could compound"; the amount audit answers whether they did, and the Mods
  dialog says so in as many words.
- **Golden Stage C now reports 466 applied edits rather than 465.** Not a
  regression: the stage builds the embedded built-ins for the ten names the
  reference library shares, and BigStacks gained the harvester edit. The file
  was already a target (`StacksizeChanger` edits it), so the built count is
  still 100 and every verdict is unchanged.
- **`nmsbonker audit` needs the game only to name the cache directory.** With
  the install missing it falls back to the last report's game buildid, because
  "what did that build come out at" has an answer that does not need the game to
  be present.
- **The GUI's `audit` affordance is `Re-check amounts` on the Report section**,
  and the Report's facts card gained a `Capped values` row that is present in
  both states so it cannot appear and disappear between builds.

### R2.5 on the real file

Built against the installed game with `ANTIMATTER_HARVESTER_CAP` at its default
of 20, the merged `ANTIMATTERHARVESTER.ENTITY.MXML` differs from the pristine
copy by exactly one line:

```
640c640
< 						<Property name="MaxCapacity" value="-1" />
---
> 						<Property name="MaxCapacity" value="20" />
```

Line 619, the `MAINT_FUEL4` element's `MaxCapacity`, is untouched. With the
parameter at 0 the script produces no change-table entry for the file at all and
declares one target instead of two.

### Caps bite in the current configuration

At their defaults, and with no compounding script in the library, the caps hold
back **11 values** in a build of the current mod list:

| Tweak | Cap | Values held back |
| --- | --- | --- |
| `ChestAndLootMaterials10x` | `LOOT_CAP` 50000 | 1 |
| `LearnMoreWords` | `WORDS_CAP` 25 | 8 |
| `NaniteRewardBuff` | `NANITES_CAP` 250000 | 2 |

That is a behaviour change from spec 004's output, and it is the intended one:
those eleven values were above the ceiling this phase introduces. Setting the
cap to 0 restores the previous, unbounded result for any tweak.

### Measured numbers

| What | Measured |
| --- | --- |
| Audit, clean build (22 mods, no flags), summed over 15 workers | 74 ms |
| Audit, 195 flags with attribution, summed over 15 workers | 1.36 s |
| Audit, 211 flags with attribution (every cap off) | 1.22 s |
| `nmsbonker audit`, 211 flags, re-read of the kept merge | 1.03 s |
| `nmsbonker audit`, 2270 flags at `max_ratio 5` | 109 ms |
| Reward blocks compared (REWARDTABLE + EXPEDITIONREWARDTABLE) | 4064 |
| Build wall clock, warm cache, audit included | 4.8-5.1 s |

The attribution's cost is bounded by the fast path: while a pass has the stock
document's line count — which every value edit does — a flagged block is read
from its recorded position rather than by re-parsing nine megabytes. The 2270-
flag re-run is faster than the 211-flag one because it needs no re-parse at all.

### What a reviewer should look at first

`internal/build/audit/audit.go`'s `Parse` and `Tracker`, and
`internal/build/amounts.go`'s `attribute`. Everything else is plumbing; those
three are where an error would be a confident wrong answer rather than an
obvious one — a block paired against the wrong stock value, or a change
attributed to the mod that happened to run next.
