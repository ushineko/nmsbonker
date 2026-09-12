# Spec 003 — Fyne GUI: application shell and sections

## Status: COMPLETE

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
   count, `GAMEDATA/MODS` state incl. the symlink warning, with an `Open`
   button on that row that hands the directory to the desktop and is disabled
   when the state is `absent`; `DisableAllMods` state with a warning marker
   when true); tools card
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
   and buttons `Open report folder`, `Open output folder`,
   `Open game mods folder` (the deploy target, disabled when it is absent),
   `Deploy…`. Empty state: "No build yet."
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
touched; empty states explain themselves. Reuse the reference report legend text
where the report is shown.

## Acceptance Criteria

- [x] AC1 `make build-gui` succeeds; `./nmsbonker-gui --version` prints the
  version; `make build` still succeeds with `CGO_ENABLED=0`.
- [x] AC2 Launching on this machine shows Overview with the real install,
  compiler and library facts; `--section build --scheme "Breeze Light"` opens
  there in that scheme without persisting it.
- [x] AC3 A full build from the GUI streams log lines live, shows step
  progress, can be cancelled mid-compile (the step list says cancelled, no
  partial output is left in `workspace_dir/<MOD_NAME>`, the `.prev` output is
  restored), and on completion the Report section shows the same table as
  `nmsbonker report`.
- [x] AC4 Mods: add, disable, move and remove round-trip through `config.json`
  and are reflected by `nmsbonker mods list`.
- [x] AC5 Parity test passes; `gui.Actions()` covers every command from spec
  001 and 002 except the documented allow-list.
- [x] AC6 `make test` passes headless (no `DISPLAY`/`WAYLAND_DISPLAY`) including
  all GUI unit tests; `make lint` clean.
- [x] AC7 No transient element reflows the layout: banners, busy strip and
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

## Status notes

Verified 2026-09-11 against Steam buildid `25233815` (97 paks,
194,531 internal paths), MBINCompiler **v7.02.0-pre1** (`dotnet10`, .NET runtime
present), Fyne **v2.8.1**, Go 1.27.1, golangci-lint v2.12.2. The mod library for
the acceptance run is the same 27 scripts spec 002 used, through a scratch
`--config`.

**Nothing was deployed.** The real install's `GAMEDATA/MODS` is still a symlink
to a mod tree outside the install, and the window's Overview says so with a
warning marker. `core.Deploy` was exercised only by the phase-2 tests against
temporary fake game directories; the GUI's Deploy button was never pressed
against the real game.

### Manual smoke checklist (R5.3)

Driven under a nested `Xvfb :99` rather than the desktop session: the window is
a native Wayland client on this machine, and KDE's compositor owns the pointer,
so XTEST clicks cannot be aimed at it. Under Xvfb the window maps at a known
origin at exactly 1180×760 and `xdotool` drives it 1:1. Software GL (llvmpipe);
the window is responsive throughout a build at that.

| Section | What was exercised | Result |
| --- | --- | --- |
| Overview | Real install, tools and library cards | game dir, buildid `25233815`, 97 paks, `GAMEDATA/MODS symlink -> …` with a warning marker and the symlink paragraph, `DisableAllMods false`, compiler `v7.02.0-pre1 (dotnet10)`, .NET 10 present, 27 of 27 mods |
| Mods | Table, sort arrow, tap-to-toggle, select, Move down, Add…, Remove… | 27 rows with `#`/On/Name/Author/Files/Last verdict, verdicts coloured, footer "27 of 27 enabled · build order is table order…" |
| Build | Build, live log, step markers, Cancel, Rebuild cache | see AC3 |
| Report | Table, header facts, actions | identical to `nmsbonker report` (below) |
| Tools | Compiler table, tools dir, pin state, .NET line, cache card, archives card | active release marked `*`, Pin/Unpin/Remove correctly disabled with no selection and no pin |
| Settings | Form, resolved paths | reads and writes the scratch `config.json` |
| Appearance | `--scheme "Breeze Light"` for the run | applied without persisting: `preferences.json` still says `Breeze Dark` |
| About | Icon at 72 px, version, capability notes, facts | renders; the icon reads at 16 px on both a dark and a light ground |

### Acceptance evidence

- **AC1** `make build-gui` builds `nmsbonker-gui` (26,841,768 bytes);
  `./nmsbonker-gui --version` prints `nmsbonker-gui 0.1.0 (6c1b232)`;
  `make build` still builds the CLI with `CGO_ENABLED=0`, and
  `go list -deps ./cmd/nmsbonker` names neither `internal/gui` nor Fyne.
- **AC2** Launched with `--config <scratch> --section build --scheme "Breeze
  Light"`: opens on Build in the light scheme, status bar reads
  `game 25233815 · compiler v7.02.0-pre1 · mods 27/27 · output COSMOS COMBINE`.
  Overview shows the real install, tools and library facts. The saved scheme is
  unchanged afterwards.
- **AC3** A warm build streamed 685 log lines live, the step list walked
  Detect → Tools → Cache → Merge → Compile → Report, and the run finished with
  "Built 100 file(s) from 27 mod(s); 465 edit(s) applied, 115 skipped. 11 mod(s)
  need checking — see the report." A second run (`Rebuild cache`) was cancelled
  nine seconds in, mid-compile: the Compile step showed `cancelled; the previous
  output was put back`, Report stayed pending, and the banner said nothing was
  installed. On disk, `workspace_dir/COSMOS COMBINE` held the previous build's
  107 MBINs with `METADATA/REALITY/DEFAULTREALITY.MBIN` byte-identical
  (`06259963…`) to before the cancelled run, and no `COSMOS COMBINE.prev` was
  left behind. The Report section matches `nmsbonker --config <scratch> report`
  line for line: 100 built / 0 dropped, 465 applied / 115 skipped, compiler
  `MBINCompiler v7.02.0-pre1`, `compatible (2 file(s) round-tripped
  byte-identical outside the header)`, timings `6.19s wall clock; cache 1ms,
  merge 899ms and compile 23.71s summed across 8 worker(s)`.
- **AC4** Tapping the `On` cell of row 3 turned `BetterFrigateRewards` off and
  `mods list` immediately reported `no`. Selecting it and pressing `Move down`
  moved it from 3 to 4 in `mods list`. `Add…` copied a script from `$HOME` into
  the library as entry 28, enabled, author read from the script header, verdict
  `—`. `Remove…` with "also delete the file" ticked took it back out and deleted
  the `.lua`; `mods list` was back to 27.
- **AC5** `go test -tags parity ./tests/parity/` passes. The two sets are equal:
  27 leaf commands on each side, with `version` the only allow-list entry.
- **AC6** `env -u DISPLAY -u WAYLAND_DISPLAY make test` is green, including the
  GUI unit tests and the parity test. `make lint`: **0 issues**.
  `govulncheck -mode=binary` on both binaries: **No vulnerabilities found**,
  after raising `golang.org/x/image`, `x/net` and `x/text` off the versions Fyne
  2.8.1 pins (22 findings before, all transitive).
- **AC7** Compared pixel-for-pixel between an idle window and the same window
  mid-build: the left navigation column is byte-identical, and the only part of
  the status bar that differs is the 187×34 busy strip at its right-hand end.
  The cancelled run's warning banner appears in the reserved 48 px slot without
  moving the content above it.

### Bugs this found

Running a build in the window found four faults no test had:

- `internal/build.Run` deleted `<ModName>.prev` and returned on cancellation,
  so cancelling destroyed the mod folder the user had and left a third of a new
  one in its place. Fixed with a regression test; it was the CLI's bug too.
- The log pump stopped its ticker and then waited on the goroutine the ticker
  was the only thing waking, so a finished build never reported itself.
- Sections disable their buttons while work is running and do that as they are
  built, so the first section — drawn while the two startup loads were still in
  flight — came up permanently disabled.
- `show()` dropped the live build widgets *after* the incoming section had
  registered them, so any rebuild mid-build left the log streaming into nothing.

Two rendering faults as well: table cells took the colour of whichever row the
recycled widget last held (Importance must be set before `SetText`, which is
what refreshes the label), and the `GAMEDATA/MODS` row drew a replacement box
for `→`, which the bundled font lacks.

### Deviations from the spec text

- **`--config` on the GUI** (not in R1.1). Added so the window and
  `nmsbonker --config …` can be pointed at one settings document deliberately,
  which is also what made this acceptance run possible without touching the
  user's own library.
- **Five new core operations and their CLI commands.** R2.5 asks the Tools
  section for a release listing, a Remove, a cache card with a Clear and a
  Rebuild index; none of those operations existed. They landed in
  `internal/core` with `tools releases`, `tools remove`, `cache show`,
  `cache clear` and `pak reindex` beside them, because the project rule forbids
  a capability in one front end and not the other.
- **`StatusResult` gained `ModName` and `Compiler.Dotnet10`**, which the status
  bar, the Overview tools card and `status` all now show.
- **Tools also reaches `pak list`, `pak find` and `pak extract`** through an
  "Archives…" and a "Find a file…" dialog. Beyond R2.5's text, and the reason is
  the parity guard: the alternative was three allow-list entries excusing a
  whole command group from having a GUI.
- **The `On` column toggles on a tap in the cell**, not on a double-click:
  Fyne 2.8.1's table has no double-tap. The column is two characters wide and
  does nothing else, so a tap in it has no other possible meaning.
- **`Add…` takes one file at a time.** Fyne 2.8.1 has no multi-select file
  chooser; `core.AddMod` still takes a list, and `Import folder…` is the answer
  for a directory.
- **Sorting the mod table by anything but `#` disables Move up/Move down** and
  the footer stops claiming the rows are the build order. Sorted by name, "up"
  would move a mod to a position the user cannot see.
- **The Report section leads with the verdict table**, with the header facts
  under it. The other way round, a default-sized window opened on two rows of
  the table and a page of numbers.
- **`perform` runs synchronously when there is no content pane.** Fyne's test
  driver runs `fyne.Do` inline on the calling goroutine rather than serialising
  onto a main loop, so a worker refreshing a widget genuinely races the test
  driving it. With no window there is no render thread to keep free, so the
  goroutine buys nothing and costs determinism.
- **The parity test compares leaf commands** (`tools pin`, not `tools`).
  Comparing at the group level would let a whole subcommand land with no GUI
  surface as long as its siblings had one.

### Not done here

- Screenshots for the README: spec 004 R7.3 owns `tools/screenshot.sh`, and the
  capture harness this run used (nested Xvfb) is worth carrying into it.
- The compatibility line on Overview reads "unknown — not checked yet" even
  after a build reported `compatible`, because `core.Status` derives it from
  version parsing (spec 001 R6.2) while the build uses the round-trip check
  (spec 002 R3.4). Both are truthful; reconciling them is a core question, not
  a GUI one.

### Added after the acceptance run

- **2026-09-12** — `GAMEDATA/MODS` is reachable from the window: an `Open`
  button on the Overview row, `Open mods folder` on the game strip beside
  `Back up saves`, and `Open game mods folder` on Report beside
  `Open output folder`. All three call one helper (`ui.openModsDir`, the
  `xdg-open` path in `dialogs.go`) and are disabled when the state is `absent`;
  a symlinked MODS is opened through the link, which is what the game reads.
  Not a CLI operation and not in `Actions()`, so the parity guard is unchanged.
