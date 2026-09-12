# Spec 004 — Tweaks (parameterised built-in mods), mod settings, deploy/rollback, save backup, packaging

## Status: COMPLETE

## Context

Phase 4 of 4. Turns the working builder into the "trainer" the repository
description promises, finishes the deploy story, and packages the app.

Built-in tweaks are the 11 scripts authored by this project's author (MIT).
The reviewer hands the implementer the source directory at implementation
time; they are the scripts named below:

| Script | Parameter(s) | Effect |
|---|---|---|
| MaterialYield10x | `MATERIAL_MULTIPLIER` | mined substance amounts ×N (planet + asteroid entities) |
| ChestAndLootMaterials10x | `LOOT_MULTIPLIER` | material rewards in REWARDTABLE ×N (chests, drops, salvage) |
| MoneyAndNanites5x | `CUR_MULT` | Units and Nanites rewards ×N (`CURRENCY_MULT`) |
| NaniteRewardBuff | `NANITE_MULT` (verify name in file) | Nanites ×N on top |
| MissionStandingBuff | `STANDING_MULT` | faction/race standing ×N (`WRAPPER_MULT`) |
| BigStacks | substance/product caps (verify names) | inventory stack limits |
| ScanValue50x | `SCAN_MULT` (verify) | discovery/scan payouts ×N; starship flat value |
| SpaceMiningBoost | asteroid min/max multipliers (verify) | asteroid resources ×N, voxel chance |
| ItemValueBoost | `VALUE_MULT` (verify) | item BaseValue ×N (market prices) |
| LearnMoreWords | `WORDS_MULT` (verify) | words learned per interaction ×N |

(The implementer reads each script and records the actual global names in the
spec's Status notes; the table above is indicative.) Third-party scripts are
never embedded; users import them into their library with `mods import`.

Every built-in already follows the pattern `NAME = <number>` at the top of the
file, before `NMS_MOD_DEFINITION_CONTAINER = {…}`, and the container reads the
global. That is the parameter contract.

Deployment facts: the game rewrites
`Binaries/SETTINGS/GCMODSETTINGS.MXML` (format below) and sets
`DisableAllMods=true` after a crash; loose-file mods need an entry with
`Enabled=true` (the game adds one on first sight and prompts at the title
screen). Saves live in the Proton prefix:
`<library>/steamapps/compatdata/275850/pfx/drive_c/users/steamuser/AppData/Roaming/HelloGames/NMS/st_<id>/mf_save*.hg|save*.hg`
and are Steam-Cloud synced.

```xml
<?xml version="1.0" encoding="utf-8"?>
<Data template="GcModSettings">
	<Property name="DisableAllMods" value="false" />
	<Property name="Data">
		<Property name="Data" value="GcModSettingsInfo" _index="0">
			<Property name="Name" value="COSMOS COMBINE" />
			<Property name="Author" value="" />
			<Property name="ID" value="0" />
			<Property name="AuthorID" value="0" />
			<Property name="LastUpdated" value="0" />
			<Property name="ModPriority" value="0" />
			<Property name="Enabled" value="true" />
			<Property name="EnabledVR" value="true" />
			<Property name="Dependencies" />
		</Property>
	</Property>
</Data>
```
(The file starts with a UTF-8 BOM; preserve it and the tab indentation.)

## What this is not

- Not a save editor. Save backup is copy-only.
- Not a Nexus client; no downloading of third-party mods.
- Not Windows/macOS support (packaging is Linux; the CLI cross-compiles but is
  untested elsewhere and the README says so).

## Requirements

### R1 — Built-in tweaks (`internal/tweaks`)

- R1.1 The 11 scripts are copied into `internal/tweaks/scripts/*.lua`
  (`go:embed`), each gaining a header block:
  ```lua
  -- @tweak name="Material yield" group="Mining" 
  -- @param MATERIAL_MULTIPLIER label="Mined amount multiplier" min=1 max=100 step=1 default=10
  MATERIAL_MULTIPLIER = 10
  ```
  `@tweak` gives the display name and group (Mining, Loot, Currency, Standing,
  Inventory, Economy, Exploration, Language). `@param` lines describe every
  tunable global; `kind=float` allows fractional values (default integer).
  Scripts keep working unmodified in AMUMSS (comments only).
- R1.2 `modscript.Parameters(src) []Param` parses `@param` headers and, for
  scripts without headers (library scripts), falls back to scanning top-level
  `IDENT = <number literal>` statements that precede the container (label =
  IDENT, no bounds). `Param{Name, Label, Min, Max, Step, Default, Kind, Current}`.
- R1.3 Overrides live in `config.params: {"<mod name>": {"<NAME>": <number>}}`.
  They are applied by **textual substitution of the assignment line** in an
  in-memory copy before Lua execution (regex `^\s*NAME\s*=\s*<number>` anchored
  to the first match; error if not found — the script changed shape). The file
  on disk is never modified.
- R1.4 Built-ins appear in the mod list as source `builtin` (library scripts are
  `library`); they cannot be removed or re-ordered relative to each other's
  *files*, but do take part in the single global build order like any mod.
  Default order on a fresh config: the reference `mods.conf` order for the
  built-ins (MaterialYield10x, ChestAndLootMaterials10x, MoneyAndNanites5x,
  BigStacks, ScanValue50x, SpaceMiningBoost, ItemValueBoost, LearnMoreWords,
  NaniteRewardBuff, MissionStandingBuff); default state **disabled** (a fresh
  install changes nothing until asked). `Reset to defaults` restores the
  script's own values. When a library script with the same basename as a
  built-in is present (a user who imported the reference script set before the
  built-ins existed), `mods list`/`check` warn about the duplicate and the
  library copy is ignored by the build; the Mods section shows it as
  `shadowed by built-in` with a `Remove` action.
- R1.5 Golden parity (spec 002) must still hold with built-ins at default
  values: the embedded scripts, executed with no overrides, produce the same
  dump JSON as the reference copies in `$NMSBONKER_REFERENCE_DIR/lua-src` (a
  test asserts this when the variable is set; headers are comments, so they
  must).

### R2 — Tweaks section (GUI) and CLI

- R2.1 New section **Tweaks** between Mods and Build: one flat list of cards in
  ascending build order, a switched-off tweak keeping its slot rather than
  sinking; each built-in shows an On/Off `Check`, a dim `<group> · build order
  <n>` tag after its name (the subject it belongs to, and the position that
  decides which of two tweaks editing the same value wins), its description
  (from a `-- @desc` header or the first comment block), and one row per
  parameter: label, `widget.Slider` bound to the parameter's bounds/step, a
  numeric `Entry` kept in sync, the default in dim text, and `Reset`. Changing a
  value marks the section "unbuilt changes" (a warn-coloured line in the card
  header, not a banner) until the next successful build; `Apply and build`
  button at the top.
- R2.2 Library scripts with detected parameters show the same row UI in an
  expandable "Parameters" block in the Mods section's detail dialog (`Details…`
  row action), unbounded numeric entry only.
- R2.3 CLI: `tweaks list [--json]`, `tweaks set NAME PARAM VALUE`,
  `tweaks reset NAME [PARAM]`, `tweaks enable|disable NAME`.

### R3 — Mod settings (`GCMODSETTINGS.MXML`)

- R3.1 `internal/modsettings`: parse to a small struct preserving unknown
  properties and order; write back with BOM, CRLF-or-LF as found, tabs.
- R3.2 `core.Deploy` (extending spec 002 R5): after installing the folder, back
  up `GCMODSETTINGS.MXML` to the archive dir, then ensure an entry `Name=<MOD_NAME>`
  exists with `Enabled=true`, `EnabledVR=true`, and set `DisableAllMods=false`
  when it was true, emitting a warning that the game had disabled mods (likely
  after a crash). Other entries untouched. If the file does not exist, do
  nothing and say so (the game creates it).
- R3.3 `core.ModsToggle{DisableAll bool}` flips only `DisableAllMods`
  (non-destructive kill switch), exposed as `nmsbonker mods-off|mods-on` and an
  Overview button pair `Disable all mods` / `Enable mods`.

### R4 — Rollback and archive

- R4.1 Archive dir `$XDG_DATA_HOME/nmsbonker/archive/<MOD_NAME>-<UTC ts>/` holds
  the replaced mod folder and the settings backup; retention: keep the newest 5,
  delete older on each deploy (report what was pruned).
- R4.2 `core.Rollback{Timestamp}` (default newest): swaps the current deployed
  folder with the archived one (the current becomes a new archive entry) and
  restores the settings backup from that entry. `nmsbonker rollback [TS]`,
  `nmsbonker archive list`, Report section `Roll back…` (confirm, states which
  archive and that the workspace build is not touched).
- R4.3 `core.Undeploy` removes `GAMEDATA/MODS/<MOD_NAME>` (archiving it) and
  leaves settings alone. CLI `undeploy`, Overview `Remove deployed mod…`.

### R5 — Save backup

- R5.1 `core.BackupSaves` copies every `st_*/` directory of the NMS roaming
  folder in the Proton prefix to `$XDG_DATA_HOME/nmsbonker/save-backup/<UTC ts>/`,
  retention 10, and reports size/count. Absent prefix → reported, not an error.
- R5.2 First deploy in a process (and any deploy after a game-buildid change)
  triggers a backup automatically unless `config.save_backup=false`; CLI
  `saves backup|list`; Overview button `Back up saves` with the last backup time.
  Restore is manual (documented path); the tool does not write into the prefix.

### R6 — Symlinked `GAMEDATA/MODS`

- R6.1 A symlinked `GAMEDATA/MODS` is handled generically (spec 002 R5.1): the
  refusal message says only that the folder is a symlink, where it points, and
  that `--replace-symlink` removes the link (never its target) and creates a
  real directory. No reference to any previous setup.
- R6.2 The Overview install card shows `GAMEDATA/MODS: symlink → <target>` with
  a warn marker and a `Replace symlink and deploy…` action; its confirm dialog
  states the three facts above and that the link's target is left untouched.
  After success the card shows `directory`.

### R7 — Packaging, install, docs

- R7.1 `packaging/io.ushineko.nmsbonker.desktop` (basename = app ID, the
  load-bearing comment copied from angou), `packaging/nmsbonker.svg`, MIME not
  needed. `install.sh`/`uninstall.sh` at the root: idempotent, `--dry-run`,
  `--no-gui`; installs `nmsbonker` and `nmsbonker-gui` to `~/.local/bin`, the
  desktop file to `~/.local/share/applications`, the icon to
  `~/.local/share/icons/hicolor/scalable/apps`; a GUI build failure names the
  missing packages and continues with the CLI. Uninstaller removes only what
  it placed; user data (config, library, cache, archive, save backups) stays and
  its locations are printed.
- R7.2 Makefile: `build-gui`, `build-all` (CLI for linux/darwin × amd64/arm64
  into `dist/`, GUI host-only, GUI failure non-fatal), `install`, `release`
  (tar.gz per target + `SHA256SUMS`), `screenshots` (runs `tools/screenshot.sh
  --all`).
- R7.3 `tools/screenshot.sh` adapted from angou: drives `nmsbonker-gui
  --section X --scheme "Breeze Dark"`, captures each section into
  `assets/<section>.png`, checks focus and aspect ratio. Screenshots of
  Overview, Mods, Tweaks, Build (mid-run), Report are committed to `assets/`.
- R7.4 README completed in angou's style. **The opening paragraph must state
  the use case: No Man's Sky installed through Steam and run under Proton on
  Linux, where the community builder AMUMSS cannot run (Windows batch tooling,
  fails under Wine; the community answer is a Windows VM).** Then: what it is, how the game loads mods
  (loose files, one merged folder, why), prerequisites, install, first run
  (detect → install compiler → import scripts or enable tweaks → build →
  deploy), CLI reference, where files live (a table of XDG paths), limitations
  (heuristic ADD/REMOVE, ignored AMUMSS keys, Linux only, mods must be rebuilt
  after each game update and MBINCompiler must have a matching release),
  troubleshooting (game disabled mods after a crash; compiler mismatch; missing
  .NET), credits (MBINCompiler, HGPAKtool format reference, AMUMSS script
  format), changelog `0.1.0`.
- R7.5 `docs/architecture.md`: package map, data flow diagram (text), the
  parity/golden story and how to regenerate golden fixtures from the reference
  Python builder (the `make_golden.py` harness under `tools/reference/` is this
  project's own code and contains no game data).
- R7.6 Validation report for the `0.1.0` milestone under `validation-reports/`
  (angou template), including govulncheck output and binary sizes. No version
  bump or tag without the user's say-so.

## Acceptance Criteria

- [x] AC1 Built-ins at default values reproduce the reference dump JSON (R1.5) and
  the spec 002 golden suite still passes.
- [x] AC2 Setting `MATERIAL_MULTIPLIER` to 20 via the Tweaks slider (and via
  `tweaks set`) changes the merged MXML for the affected entity files by exactly
  the expected factor and the on-disk embedded script is unchanged.
- [x] AC3 On a fake game dir whose `GAMEDATA/MODS` is a symlink, the Overview
  `Replace symlink and deploy…` action (and `deploy --replace-symlink`) leaves
  `GAMEDATA/MODS/<MOD_NAME>` as a real directory with the built files, the
  settings entry enabled, and the symlink's former target untouched; the
  message text contains no reference to any previous setup.
- [x] AC4 `deploy` twice in a row archives the first deployment; `rollback`
  restores it; `archive list` shows both; retention prunes to 5 in a test on
  fakes.
- [x] AC5 `DisableAllMods=true` in a fake settings file is flipped to false by
  deploy with a warning; `mods-off` sets it true; both preserve the BOM, tabs
  and unknown properties byte-for-byte outside the changed values.
- [x] AC6 `saves backup` creates a timestamped copy of the `st_*` folders and
  never writes into the prefix.
- [x] AC7 `install.sh` places both binaries, the desktop file and the icon; the
  app appears in the KDE launcher with its icon on Wayland; `uninstall.sh`
  removes exactly those files.
- [x] AC8 README, architecture doc, screenshots and the validation report
  exist; `make lint`, `make test`, parity and golden suites pass.
- [x] AC9 The parity oracle is called the *reference* implementation
  everywhere: a case-insensitive grep over the repository (excluding `.git`) for
  the superseded term this project used for it returns nothing, and no committed
  file contains a personal absolute path.

## Risks & Assumptions

- Parameter substitution by regex assumes one assignment per parameter at the
  top level; the `@param` header is authoritative for built-ins, and a script
  that reassigns the global later would silently ignore the override — the
  `check` command warns when a parameter name is assigned more than once.
- Replacing a `GAMEDATA/MODS` symlink is reversible by hand (`ln -sfn`) and the
  dialog says so. Steam "verify integrity" restores stock files if anything
  goes wrong.
- Retention deletes archives; the counts are conservative and printed.
- Rollback: `git revert` for code; `nmsbonker rollback` for the game folder.

## Alternatives Considered

- Injecting parameters through a Lua metatable on `_G`: rejected — the script's
  own assignment would overwrite the injected value; textual substitution is
  simple and visible in `check --json`.
- Writing built-ins as Go-native ops instead of Lua: rejected — keeping them as
  AMUMSS scripts keeps them portable to AMUMSS users and keeps one engine.

## Status notes

Verified 2026-09-11 against Steam buildid `25233815` (97 paks, 194,531 internal
paths), MBINCompiler **v7.02.0-pre1** (`dotnet10`, .NET runtime present), Fyne
**v2.8.1**, Go 1.27.1, golangci-lint v2.12.2, on KDE/Wayland. The full evidence
is in [`validation-reports/2026-09-11-v0.1.0.md`](../validation-reports/2026-09-11-v0.1.0.md);
what follows is what the spec text got wrong and what was decided instead.

**Nothing was deployed to the real install.** Deploy, rollback, undeploy,
`mods-off` and `mods-on` were exercised against temporary fake game directories,
in tests and by hand on the command line. The one operation run against the real
game is `saves backup`, which reads the Proton prefix and writes into a scratch
data directory; the prefix held 45 files before it and 45 after.

**2026-09-12 — the Tweaks section is one build-order list.** The per-group cards
(Mining / Loot / …) became a single list ordered by build position and the `#13`
badge became `<group> · build order 13`; presentation only, no operation or
stored value changed.

### The built-in set is ten scripts, not eleven

The table in the Context section lists ten rows and calls them eleven. Ten is
right: `StackingTechnologyModules`, which is in the reference library alongside
them, is a third-party script and is not this project's to embed. It stays where
every other third-party script stays — the user's library — and the count is
asserted by `TestEveryEmbeddedScriptIsListedAndEveryListedScriptIsEmbedded`.

### The parameter names, read out of the scripts

| Script | Group | Parameters (declared range) |
| --- | --- | --- |
| MaterialYield10x | Mining | `MATERIAL_MULTIPLIER` 1–100, default 10 |
| ChestAndLootMaterials10x | Loot | `LOOT_MULTIPLIER` 1–100, default 10 |
| MoneyAndNanites5x | Currency | `CUR_MULT` 1–100, default 5 |
| BigStacks | Inventory | `SUBSTANCE` 1–9999999 default 999999; `PRODUCT` 1–9999999 default 99999 |
| ScanValue50x | Exploration | `SCAN_MULTIPLIER` 1–200 default 50; `SHIP_FLAT` 0–1000000 default 25000 |
| SpaceMiningBoost | Mining | `AST_MULT` 1–100 default 20; `VOXEL_CHANCE` 0–1 step 0.05 default 1.0, `kind=float` |
| ItemValueBoost | Economy | `VALUE_MULT` 1–50 default 3, `kind=float` |
| LearnMoreWords | Language | `WORD_MULT` 1–50, default 5 |
| NaniteRewardBuff | Currency | `NANITE_MULT` 1–100, default 10 |
| MissionStandingBuff | Standing | `STANDING_MULT` 1–50, default 5 |

The spec guessed `SCAN_MULT`, `WORDS_MULT` and "substance/product caps"; the real
names are above. `kind=float` is on exactly the two parameters where a fraction
is meaningful, and the distinction is not cosmetic: a value written into a
`VALUE_CHANGE_TABLE` with no `MATH_OPERATION` goes into the MXML verbatim, so
`VOXEL_CHANCE` must stay `1.0` and `SUBSTANCE` must stay `999999`. Where there
*is* a `MATH_OPERATION`, the engine takes the old value's kind and the script's
kind does not matter, which is why `VALUE_MULT` can be a float safely.

### R1.5 compares MODIFICATIONS, not the whole container

One field was changed on purpose: `MOD_AUTHOR`, which named whoever assembled
the reference copies and now says `nmsbonker`, since these are the project's own
scripts. It is metadata — the build report is the only thing that reads it — so
`TestBuiltInsDecodeAsTheReferenceCopiesDo` compares the `MODIFICATIONS` subtree
in full and asserts `MOD_AUTHOR == "nmsbonker"` separately. Everything the edit
engine acts on is under `MODIFICATIONS`. No other line of any script differs;
the header block is `--` comments prepended to the file.

### Deviations from the spec text, and why

- **An archive entry is a directory, not the mod folder.** R4.1 asks one entry
  to hold both the replaced folder and the settings backup, so the layout is
  `archive/<MOD_NAME>-<UTC>/{mod/, GCMODSETTINGS.MXML, archive.json}` rather
  than spec 002's "the folder, renamed". `readArchiveEntry` still reads a
  spec-002-shaped directory rather than hiding it: it may be the only copy
  somebody has of what a deploy displaced.
- **An entry is written on every deploy, including the first.** The settings
  file is edited on every deploy, and "undo" has to include undoing that. An
  entry with no `mod/` records "nothing was installed at this point"; rolling
  back to it removes what is installed now, and both the CLI and the dialog say
  so. It is also why `archive list` shows two entries after two deploys rather
  than one.
- **The build report gained a `params` field.** It records the overrides the
  build was produced with, which is what makes R2.1's "unbuilt changes" a fact a
  front end can read back across restarts and across front ends rather than a
  flag the window has to remember. It also answers "what was this built with"
  three weeks later, so it is in the Markdown header too.
- **A settings file with no mod list is a warning, not a failure.** By the time
  the settings are written the mod folder is already installed; refusing there
  would leave the user with a new mod in the game and an error about an XML
  document. The game rewrites the file when it next starts.
- **Out-of-range parameter values are clamped, not refused.** A slider cannot
  produce one; typing can, and so can a hand-edited settings file. The nearest
  legal value with a banner saying so beats an error, and the CLI says the same
  thing. A library script has no declared bounds and is not clamped.
- **`tweaks set` and `tweaks reset` take any mod, not only a built-in.** R2.2
  asks the Mods detail dialog to offer the same controls for a library script's
  detected parameters; one operation serving both is what keeps them behaving
  the same way, and it is why the CLI can tune a library script too.
- **A built-in cannot be removed, and `mods remove` on one deletes its shadow.**
  R1.4's "shadowed by built-in" row needs a Remove that means something:
  `RemoveMod` on a built-in name deletes the library script of that name and
  leaves the built-in in the build order, and refuses with `ErrBuiltInMod` when
  there is no shadow to delete.
- **`save_backup` is a `config set` key.** R5.2 names the setting but not how to
  change it; `nmsbonker config set save_backup false` is the answer, which keeps
  it reachable from the window's Settings form for free.
- **The Mods table gained a Source column** (`builtin` / `library`, and
  `built-in *` for a shadowed one), and a `Details…` row action, which is where
  R2.2's parameter block lives.
- **`Reset all to defaults` on the Tweaks section** is not in R2.1. It exists
  because R2.1's per-parameter Reset makes undoing an afternoon of fiddling ten
  clicks; it is behind a confirmation naming the count.
- **The desktop entry declares one main category.** `Categories=Game;Utility;`
  makes `desktop-file-validate` warn that the entry can appear twice in a menu.
  `Utility` alone, with `Keywords` carrying "No Man's Sky", "NMS", "AMUMSS" and
  "Proton", is what a launcher search actually matches on.

### From the phase-3 review, folded in here

- **The Overview's "Game data version" row is gone.** Spec 002 R3.4 retired the
  concept: the game's own MBINs carry no libMBIN version, so the row was
  reporting a parse of a filename as though it were a fact about the install.
  `core.Status` still carries the field and the CLI still prints it, because
  `tools releases` uses the same parse to decide which release to install and
  removing it there is a spec-001 question rather than a spec-004 one.
- **The Compatibility row is the round-trip check.** `core.ToolCheck` runs in the
  background on the first Overview load, once per process, behind the busy strip;
  the row reads "checking…" until it lands, and pressing `tools check` in the
  Tools section fills the same field so there is one verdict rather than two.
- **`Replace symlink and deploy…`** is an action on the install card, with the
  three facts of R6.1 in its confirmation and no reference to anything that might
  have created the link.

### Not done

- **No mid-run Build screenshot** (R7.3 asks for one). The window is a native
  Wayland client, `kdotool` can raise a window but cannot inject a click into
  one, and `nmsbonker-gui` has no flag that starts a build — adding one would be
  an operation with no CLI counterpart, which the parity rule forbids. Overview,
  Mods, Tweaks and Report were captured instead, the last against a finished
  build. The capture harness builds a Steam-shaped fake install under
  `/tmp/nmsbonker-demo` whose `PCBANKS` is a symlink to the real archives, so the
  images show real numbers and no path belonging to anybody.
- **Two readers for `GCMODSETTINGS.MXML`.** `steam.ReadModSettings` (spec 001,
  read-only, used by `status`) and `internal/modsettings` (this phase, read and
  write) both parse it. They agree; one of them should go, and doing it in the
  milestone commit was not worth the churn.
- **No version bump and no tag.** `VERSION` stays `0.1.0`, which is what the
  validation report describes.
