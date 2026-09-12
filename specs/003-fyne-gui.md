# Spec 003 — Fyne GUI: application shell and sections

## Status: INCOMPLETE

## Context

Phase 3 of 4. Adds the desktop front end `cmd/nmsbonker-gui` over the
`internal/core` operations from specs 001 and 002. The design system and
engineering idioms are **copied from `~/git/angou/internal/gui`** (same author,
MIT), not reinvented. Read these files before writing any GUI code:
`app.go`, `theme.go`, `fonts.go`, `views.go`, `dialogs.go`, `state.go`,
`icon.go`, `cursor_linux.go`, `cursor_other.go`, and the tests
`flash_test.go`, `browse_test.go`, `firstrun_test.go`.

Idioms to carry over verbatim (see angou for the rationale comments, which are
part of the house style and should be kept when copying):

- `app.NewWithID("io.ushineko.nmsbonker")`; window title `"nmsbonker <version>"`;
  default size `1180×760`; `SetMaster`; `applyCursorTheme()` before toolkit init.
- One window: `Border{top: header, bottom: frame, center: HSplit(nav list 0.16,
  scrolling content)}`; `frame = VBox{fixed 48 px flash slot, status bar}` with
  an 18 px busy strip on the right of the status bar. No menu bar, no tabs; each
  section builds its own inline toolbar of `NewButtonWithIcon`.
- `kdeTheme` with the five palettes (Breeze Dark default, Breeze Light, Oxygen
  Dark, Adwaita Dark, Adwaita Light), text size 12 default (10–18), padding 3,
  radius 2, system-font scanner, Appearance section with scheme/font/size
  selects, Reset, and a live Sample block. Preferences hold appearance only.
- All state on `*ui`, mutated on the UI thread; sections rebuilt from scratch
  on `refresh`; explicit `…OK bool` loaded flags; three redraw levels
  (`show` / `refresh` / `rebuild`) plus `invalidate`.
- Every core call runs on a goroutine inside `u.busy(what)`; results hop back
  with `fyne.Do`; `fyne.DoAndWait` is not used. Flash banners for reporting
  (good 6 s, warn 12 s, bad until dismissed), modal dialogs only for asking.
  `confirmDestructive` with `DangerImportance`; row action buttons disabled
  until a row is selected.
- Helpers to copy: `heading`, `statusText`, `marker`, `dim`, `sep`, `action`,
  `fixedHeight`/`fixedWidth`, `browseButton`/`withBrowse` (with the
  `Show()`-before-`Resize()` note), `humanSize`, `humanAgo`.
- `F5` and `Ctrl+R` reload. `--section`, `--scheme`, `--version` flags on the
  GUI binary (plain `flag`, not cobra). `gui.SectionNames()`, `gui.SchemeNames()`,
  `gui.Actions()` exported.
- Tests run headless with `test.NewApp()`; a helper `testUI(t)`; names are
  sentences; every test says which bug it prevents.

What is new here versus angou: this app runs **long, chatty, cancellable
operations** (a build takes 30 s–3 min and emits hundreds of lines). The Build
section therefore has a live log pane and a Cancel button, and `core.Events`
is bridged to the UI. angou's "no logging" stance does not apply to build
output, which is the product; it still applies to secrets (there are none) and
to not spraying stderr from the GUI.

## What this is not

- Not tweak parameters, deploy/rollback controls beyond "Deploy" and
  GCMODSETTINGS editing (spec 004 adds the Tweaks section and the rollback
  controls to the sections defined here).
- Not packaging/desktop entry/install script (spec 004).
- Not a mod *editor* (text editing of `.lua`): the Mods section opens a script
  in the system editor via `xdg-open`; that is all.

## Requirements

### R1 — Binary and bootstrap

- R1.1 `cmd/nmsbonker-gui/main.go` mirrors angou's: `--section`, `--scheme`,
  `--version`, then `gui.Run(gui.Options{...})`. Built by `make build-gui`
  (`CGO_ENABLED=1`); `make build` (CLI) must remain CGO-free and must not import
  `internal/gui`.
- R1.2 `internal/gui` copies angou's `theme.go`, `fonts.go`, `icon.go`,
  `cursor_*.go` and the shell/helpers from `app.go`/`dialogs.go`/`state.go`
  with the app-specific parts replaced. Attribution: a top-of-file comment
  "Copied from angou (same author) — keep in sync by hand" on each copied file.
- R1.3 App icon: a new hand-drawn ≤ 3-shape 64×64 SVG under
  `packaging/nmsbonker.svg`, embedded via `internal/gui/assets`. It must read at
  16 px. (Suggested motif: a hexagon — NMS's UI language — with a wrench or a
  bold "×N".)

### R2 — Sections (navigation order)

1. **Overview** — install card (game dir, buildid, data version, PCBANKS pak
   count, `GAMEDATA/MODS` state incl. the legacy-symlink warning,
   `DisableAllMods` state with a warning marker when true); tools card
   (compiler version + compatibility marker, dotnet runtime, pak index age);
   library card (enabled/total mods, last build time and verdict counts from
   `core.Report`); primary actions `Build`, `Build and deploy…` (the latter
   confirms and states what is replaced/archived), `Refresh`. Empty/first-run
   state explains itself: no game found → how to set `game_dir` (Browse…);
   no compiler → `Install MBINCompiler` button (runs `EnsureTools`).
2. **Mods** — a `widget.Table` (sortable headers like angou's store table) with
   columns: `#` (order), `On` (checkbox rendered as ✓/–, toggled by the row
   action or double-click), `Name`, `Author`, `Targets`, `Last verdict`
   (coloured via `statusText`: WORKING green, WORKING~/WORKING* warn, PARTIAL /
   NOT BUILT red, `—` dim). Toolbar: `Add…` (file chooser filtered to `.lua`,
   copies into the library), `Import folder…`, `Remove…` (confirm; offers
   "also delete the file"), `Enable`, `Disable`, `Move up`, `Move down`,
   `Check` (runs `CheckMods` and shows per-script problems in a dialog),
   `Open script` (`xdg-open`), `Open library folder`. Selection is cleared when
   the sort changes. A footer line: "`N` of `M` enabled · build order is table
   order; lower rows apply later and win on conflicts."
3. **Build** — a step list on the left (Detect → Tools → Cache → Merge →
   Compile → Report [→ Deploy]) with `marker()` states, and on the right a
   fixed-height log pane: a `widget.List` of lines (monospace labels, auto-scroll
   to the end unless the user scrolled up, ≤ 5 000 lines retained, `Copy log`
   button). Buttons: `Build`, `Build and deploy…`, `Rebuild cache` (forces
   `Recache`), `Cancel` (enabled only while running; cancels the context; the
   step shows `cancelled`). After completion the step list shows totals and a
   `View report` button switches to Report.
4. **Report** — the latest `report.json` as a table (Mod, Status, Edits,
   Skipped, Notes) with verdict colouring, header facts above it (generated
   time, compiler, output dir, totals), a `Degraded files` block when present,
   and buttons `Open report folder`, `Open output folder`, `Deploy…`. Empty
   state: "No build yet."
5. **Tools** — installed MBINCompiler versions (table: tag, flavor, path,
   in-use), `Check for updates` (release list via core, offline-tolerant),
   `Install <newest matching>`, `Pin`/`Unpin` on a selected row, `Remove`
   (confirm) for non-active rows; dotnet runtime line; cache card: size on
   disk, entry count, buildid, `Clear cache…` (confirm). Pak index card: pak
   count, age, `Rebuild index`.
6. **Settings** — a `widget.Form`: game dir (entry + Browse… + "auto" reset),
   library dir, mod name, parallel jobs (Select 1…NumCPU), compiler flavor
   (Select), with `Save`/`Revert`; a note stating exactly which file is written
   (`config.json` path) and that appearance lives in Fyne preferences.
7. **Appearance** — copied from angou.
8. **About** — copied shape: icon 72×72, name + version (commit), blurb,
   `Project documentation` hyperlink to the GitHub README, capability notes
   (What it builds / What it never touches / Where files live), Facts form
   (Compiler, Game buildid, Config path, Licence).

### R3 — Status bar

`dim("game") <buildid or "not found"> · dim("compiler") <version or "none"> ·
dim("mods") <enabled>/<total> · dim("output") <MOD_NAME> … spacer … busy strip`.
Sequential HBox, never Border-center.

### R4 — Event bridge and cancellation

- R4.1 `core.Events.Log` → append to the Build log model (`fyne.Do`), also
  mirrored into the step list state (`Progress` events set the current step).
  Warn/error lines from the build are *not* individually flashed; one flash
  summarises the result (`Built 100 files; 2 mods partial`), bad if anything was
  dropped.
- R4.2 One operation at a time: while a build runs, other core-invoking buttons
  are disabled (not hidden); `busy` refcount drives it.
- R4.3 Cancel: `context.WithCancel` per run; `core.Build` must return promptly
  (MBINCompiler processes are killed by the runner from spec 001 R5.4).

### R5 — Parity and tests

- R5.1 `gui.Actions()` lists every core operation the GUI can reach; a test in
  `tests/parity/` (build tag `parity`, run by `make test` — it needs no display)
  compares it with the cobra command tree from `internal/cli` and fails on
  drift outside a commented allow-list (e.g. `version`, `detect --json`).
- R5.2 Headless tests: flash behaviour (copied), Mods table renders the
  reconciliation states (enabled / disabled / missing) from a fake result,
  Build log model caps at 5 000 lines and auto-scroll toggles off when the user
  scrolls up, Cancel button state follows running state, Settings form Save
  writes the config through core and Revert restores, `SectionNames()` works
  without an app, `require.NotPanics` on tapping every Browse… button.
- R5.3 A manual smoke checklist in the spec's Status notes (run the GUI on this
  machine, exercise each section) — screenshots are optional here, required in
  spec 004.

### R6 — Copy voice

Sentence case, second person, plain words, consequences named; trailing
ellipsis on buttons that open a dialog; destructive dialogs state what is *not*
touched; empty states explain themselves. Reuse the legacy report legend text
where the report is shown.

## Acceptance Criteria

- [ ] AC1 `make build-gui` succeeds; `./nmsbonker-gui --version` prints the
  version; `make build` still succeeds with `CGO_ENABLED=0`.
- [ ] AC2 Launching on this machine shows Overview with the real install,
  compiler and library facts; `--section build --scheme "Breeze Light"` opens
  there in that scheme without persisting it.
- [ ] AC3 A full build from the GUI streams log lines live, shows step
  progress, can be cancelled mid-compile (the step list says cancelled, no
  partial output is left in `workspace_dir/<MOD_NAME>`, the `.prev` output is
  restored), and on completion the Report section shows the same table as
  `nmsbonker report`.
- [ ] AC4 Mods: add, disable, move and remove round-trip through `config.json`
  and are reflected by `nmsbonker mods list`.
- [ ] AC5 Parity test passes; `gui.Actions()` covers every command from spec
  001 and 002 except the documented allow-list.
- [ ] AC6 `make test` passes headless (no `DISPLAY`/`WAYLAND_DISPLAY`) including
  all GUI unit tests; `make lint` clean.
- [ ] AC7 No transient element reflows the layout: banners, busy strip and
  step states occupy fixed slots (verify by inspection while a build runs).

## Risks & Assumptions

- Fyne 2.8.1 (angou's version). `widget.Table` with checkbox-like cells is
  rendered as text glyphs (✓/–), toggled via row actions, to avoid per-cell
  widget complexity.
- The log `widget.List` with 5 000 monospace rows is cheap; if scroll
  performance is poor, fall back to a `widget.TextGrid` of the visible tail.
- `xdg-open` may be absent on odd systems; failure is a warn flash.
- Rollback: `git revert`; the GUI writes only `config.json` and Fyne
  preferences outside of core operations.

## Alternatives Considered

- A shared Go module for the angou theme/helpers: rejected — two consumers,
  same author; hand-copying with a "keep in sync" note is less ceremony.
- Data binding (`fyne.io/fyne/v2/data/binding`): rejected — angou deliberately
  rebuilds views from plain state; consistency wins.
