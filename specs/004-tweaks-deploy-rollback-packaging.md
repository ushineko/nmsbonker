# Spec 004 — Tweaks (parameterised built-in mods), mod settings, deploy/rollback, save backup, packaging

## Status: INCOMPLETE

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

- R2.1 New section **Tweaks** between Mods and Build: grouped cards; each
  built-in shows an On/Off `Check`, its description (from a `-- @desc` header or
  the first comment block), and one row per parameter: label, `widget.Slider`
  bound to the parameter's bounds/step, a numeric `Entry` kept in sync, the
  default in dim text, and `Reset`. Changing a value marks the section
  "unbuilt changes" (a warn-coloured line in the card header, not a banner) until
  the next successful build; `Apply and build` button at the top.
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

- [ ] AC1 Built-ins at default values reproduce the reference dump JSON (R1.5) and
  the spec 002 golden suite still passes.
- [ ] AC2 Setting `MATERIAL_MULTIPLIER` to 20 via the Tweaks slider (and via
  `tweaks set`) changes the merged MXML for the affected entity files by exactly
  the expected factor and the on-disk embedded script is unchanged.
- [ ] AC3 On a fake game dir whose `GAMEDATA/MODS` is a symlink, the Overview
  `Replace symlink and deploy…` action (and `deploy --replace-symlink`) leaves
  `GAMEDATA/MODS/<MOD_NAME>` as a real directory with the built files, the
  settings entry enabled, and the symlink's former target untouched; the
  message text contains no reference to any previous setup.
- [ ] AC4 `deploy` twice in a row archives the first deployment; `rollback`
  restores it; `archive list` shows both; retention prunes to 5 in a test on
  fakes.
- [ ] AC5 `DisableAllMods=true` in a fake settings file is flipped to false by
  deploy with a warning; `mods-off` sets it true; both preserve the BOM, tabs
  and unknown properties byte-for-byte outside the changed values.
- [ ] AC6 `saves backup` creates a timestamped copy of the `st_*` folders and
  never writes into the prefix.
- [ ] AC7 `install.sh` places both binaries, the desktop file and the icon; the
  app appears in the KDE launcher with its icon on Wayland; `uninstall.sh`
  removes exactly those files.
- [ ] AC8 README, architecture doc, screenshots and the validation report
  exist; `make lint`, `make test`, parity and golden suites pass.
- [ ] AC9 The parity oracle is called the *reference* implementation
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
