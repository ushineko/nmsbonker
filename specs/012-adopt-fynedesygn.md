# Spec 012: Adopt fynedesygn

> **Note**: This work has no associated issue tracker ticket. The repository
> is a personal public project without an issue tracker.

## Status: INCOMPLETE

## Executive Summary

`internal/gui` now imports `github.com/ushineko/fynedesygn` v0.1.1 for its
design system and runs the window on the library's shell; the hand-copied
theme, fonts, cursor fix, shell, runner, table, dialogs, log pane and step
list are deleted, and the package's non-test code goes from 8,230 to 7,344
lines (tests 1,375 to 1,290) while gaining the Windows and macOS schemes, a
monospace-font picker and an interface scale. Behaviour is kept: the same
preference keys, sections, operations and build flow. Reviewers should start
with `app.go` (the `ui` struct, `sections`, `shellOptions`, `onInvalidate`),
`buildrun.go` (the run over `steps.List` and `logpane.Pane`) and the "Gaps
found" list below, which records where the library's shape made the program
bend and what it lacks.

## Context

`internal/gui` carries the design system copied from angou under "keep in
sync by hand" headers (`theme.go`, `fonts.go`, `cursor_*.go`, `icon.go`,
`views_appearance.go`, the shell and small widgets in `app.go`, the dialogs)
and this project's own additions that the library has since generalised: the
detail table (`views_table.go`), the build log model, step list and
follow-tail rule (`buildlog.go`), the settings form shape and the slider and
entry pair (`views_tweaks.go`). All of it now lives in
`github.com/ushineko/fynedesygn`, extracted from this program, angou and
clockwork-orange; clockwork-orange adopted it in its v4.1.0.

The adoption replaces the copies with imports and keeps the window's
behaviour identical. Where the library's shape differs, the program follows
the library; anything the library lacks is recorded in "Gaps found" for a
library spec, not hacked around. This is a single-developer repository that
works on `main`; the adoption is done on a branch in a worktree and
fast-forwarded onto `main`.

## Requirements

### R1. Dependency

- R1.1 `go.mod` requires `github.com/ushineko/fynedesygn v0.1.1`. Fyne stays
  at v2.8.1; `go mod tidy` carries the library's raised `golang.org/x/*`.

### R2. Leaves (first commit)

- R2.1 `theme.go`, `fonts.go`, `cursor_linux.go`, `cursor_other.go` →
  `fynedesygn/theme`. The appearance triple becomes `fdtheme.Appearance`
  with the same preference keys (`appearance.scheme`, `appearance.font`,
  `appearance.textSize`), so saved choices survive. This program has no
  console font; `Appearance.Theme()` is the theme.
- R2.2 `views_table.go` → `fynedesygn/table` (`table.Detail`).
- R2.3 The small widgets in `app.go` (`dim`, `sep`, `statusText`, `marker`,
  `wrapped`, `heading`, `action`, `card`, `factRow`, `plainRow`,
  `rowWithAction`, `humanSize`, `humanAgo`, `orNone`), `note` in
  `views_overview.go`, and `fixedHeight`/`fixedWidth` → `fynedesygn/widgets`.
  `Status` and its constants → `fynedesygn.Status`; the domain rankers in
  `model.go` stay and return `fd.Status`.
- R2.4 `dialogs.go` → `fynedesygn/dialogs`: `confirmDestructive` →
  `ConfirmDestructive(win, ...)`, `confirmWithBody` → `ConfirmWithBody`,
  `pathDialog` → `Prompt`, `showDetail` → `ShowDetail`, `browseButton` /
  `withBrowse` / `chooseFile` (with `luaFilter`) / `chooseFolder` →
  the library's, `openPath` → `OpenPath` with this program's busy
  indicator and warning banner kept around it.
- R2.5 `buildlog.go`: `logModel` → `logpane.Model` (cap 5000, this
  program's), the follow-tail rule → `logpane.FollowTail`, the log list →
  `logpane.Pane` (`Options{Title, Height: logPaneHeight 420, Clipboard,
  Flash}`), the step list (`buildStep`, `stepState`, `stepIcon`,
  `stepStatus`, the step rows) → `fynedesygn/steps` (`steps.List` with
  `Advance`, `Finish`, `Stop`, `Reset`); `buildRun` keeps `running`,
  `cancelled`, `cancel`, `finished`, `summary` and the control buttons.
  `core.Level` maps to `logpane.Level` in one function; lines keep this
  program's `[LEVEL]` wording by appending through `Model().Append`.
- R2.6 `views_tweaks.go`'s `paramRow` slider and entry pair →
  `forms.SliderEntry` (`Format` from `modscript.FormatValue`, `Commit` →
  `applyParam`, `OnInvalid` → the existing warning banner). `views_settings.go`
  keeps its `settingsForm` shape (its Save/Revert tests pin it) and adopts
  `forms.IntRange` / `forms.NumericEntry` where it validates numbers.
- R2.7 `views_appearance.go` → `shell.AppearanceSection(<a build log line as
  the sample>)`; `views_about.go` → `shell.AboutSection(shell.About{...})`
  with this program's icon, tagline, notes, facts and link.

### R3. Shell (second commit)

- R3.1 `ui` keeps the program's state (config path, the ~20 loaded pairs,
  `run`, `inspect`, selections) and holds `sh *shell.Shell`. Delete from
  `app.go`/`state.go` everything the shell provides: nav, header, status bar
  frame, busy popup, flash, `swap`/`show`/`refresh`/`rebuild`/`redrawStatus`
  /`regate`, `perform`, `report`, `ok`, `invalidate`, `onScreen`, `working`,
  `gate`, `selectSection`.
- R3.2 Sections become `shell.Section`s from the existing builders
  (`shell.NewSection(title, icon, ...)`), Build with `OnDetach` for the run
  (`steps.List.Detach`, `logpane.Pane.Detach`). `SectionNames()`,
  `SchemeNames()` and `Actions()` stay exported (flag help, parity test).
- R3.3 `shell.Options`: AppID `io.ushineko.nmsbonker`, Name `nmsbonker`,
  Version, Icon, Sections, Section/Scheme from the program's options,
  StatusBar returning the game/compiler/mods/output segments, `OnCreate`
  storing the shell, `OnStart` running `loadStatus` and `loadMods`,
  `OnInvalidate` dropping the loaded flags and reloading (not the build
  log), `AlsoWorking` returning `u.run.running`. No `Theme` hook (no console
  font), no `OnTypedKey` (no keyboard navigation).
- R3.4 `Run` becomes `shell.Run(u.shellOptions(o))` after `loadAppearance`
  concerns move into the shell.

### R4. Tests and guards

- R4.1 Every test in `internal/gui` passes, adjusted to the new types; the
  tests that pinned copied code now covered by the library (`followTail`,
  `logModel` cap/replace, the chooser tap, `detailTable` cells) are deleted;
  `find_test.go` helpers are replaced by `fynetest`. `tests/parity` passes.
- R4.2 `make test`, `make lint`, `govulncheck -mode binary` on both binaries
  clean; every section renders headlessly in all nine schemes.
- R4.3 A test writes the three appearance keys with the previous names and
  asserts the theme (saved choices survive).

### R5. Documentation

- R5.1 `docs/architecture.md`, `README.md` (Credit and Appearance sections)
  and `.claude/CLAUDE.md` (design reference is the library, not angou)
  point at the library; "Gaps found" filled.

## Acceptance Criteria

- [x] AC1 No file under `internal/gui` carries a "Copied from" header;
  `theme.go`, `fonts.go`, `cursor_*.go`, `views_table.go`, `dialogs.go`,
  `buildlog.go`, `find_test.go` are gone (R2). The run's state moved to
  `buildrun.go`.
- [x] AC2 `go.mod` requires `github.com/ushineko/fynedesygn v0.1.3`; no
  `replace` (R1; the spec said v0.1.1, and 0.1.3 carries the fixes this
  adoption asked for).
- [x] AC3 The grep `func (u \*ui) (flash|busy|perform|report|ok|invalidate|refresh|rebuild|redrawStatus|swap|show|detach|gate|working|regate|selectSection)\(` over `internal/gui` finds nothing (R3.1).
- [x] AC4 `SectionNames()` returns the same list as before, in order, with
  no Fyne app; `Actions()` is unchanged and the parity test passes (R3.2).
  `TestSectionNamesNeedsNoApp`, `tests/parity`.
- [x] AC5 Saved appearance keys are read unchanged (R4.3).
  `TestASavedAppearanceFromThePreviousBuildIsReadUnchanged`.
- [x] AC6 The build section: starting a build advances the step list and
  streams into the pane; cancelling marks the running step cancelled; the
  headless build tests (`views_test.go`, `buildlog_test.go` survivors) pass
  (R2.5). `TestProgressEventsAdvanceTheStepList`,
  `TestBuildLinesKeepCoresWordingAndLevel`,
  `TestCancellingMarksOnlyTheRunningStep`, `TestPressingCancelDisablesItAtOnce`.
- [x] AC7 `make test` (parity tag), `make lint`, `go vet` clean; all nine
  schemes render every section headlessly (R4.2).
  `TestEverySectionRendersHeadlesslyInEveryScheme`.
- [x] AC8 `govulncheck -mode binary` on both binaries: no findings.
- [ ] AC9 The GUI is run on this machine under a throwaway HOME with the
  screenshot harness: Overview, Mods, Build, Tools, Settings, Appearance,
  About render; a banner shows (manual, recorded in the report).
  _Partly verified 2026-09-18: all seven sections captured and checked by
  eye under a throwaway HOME, XDG and STEAM_ROOT; the Build section shows
  the step list with its standing notes beside the log pane; Overview shows
  the detail table and an info banner floating over the toolbar; the status
  bar carries game, compiler, mods and output. Not exercised: a real build
  (no game install in the sandbox), Deploy, the save editor._
- [x] AC10 Documentation updated; "Gaps found" filled (R5).
- [x] AC11 Line counts of `internal/gui` (non-test) before and after are in
  the validation report.

## Risks & Assumptions

- **Assumption**: the library's behaviour matches the copies in every case
  the tests pin; a difference that is a library bug is fixed there first.
- **Risk**: the build section is this program's most involved piece (step
  rows, log, controls, deploy confirm); the port keeps `buildRun`'s state
  and swaps only the drawing.
- **Risk**: appearance keys must not change (AC5).
- **Rollback**: `git revert` of the adoption commits; the previous binary
  reads the same preferences and config.

## Gaps found

Recorded against fynedesygn 0.1.1 during implementation, then fixed in
fynedesygn 0.1.3 (its spec 007) and adopted here in the same branch: gaps
1 and 2 (`steps.Advance` never reopens a finished step; `steps.NewSteps`
with standing notes that `Reset` keeps), 3 (`logpane.Pane.SetFollowing`,
re-armed at the start of every build), 4 (`forms.SliderEntry` hands `Commit`
the typed value, so core's clamp banner fires again), 5 (`shell.Load`), 7
(`shell.About.URLText`). Gap 6 was sequencing, not a defect. The original
list, for the record:

What the library lacked, and how the adoption handled each without changing
the library:

1. **`steps.List.Advance` walks a step backwards.** It sets step *i* running
   unconditionally, so a late per-item message from the cache after the merge
   phase has been announced (which core does emit on a slow filesystem) would
   reopen a finished step. `buildRun.advance` checks the step's state first
   and drops the message when the step is already done, failed or cancelled;
   `TestTheStepListNeverGoesBackwards` pins it. Library candidate: an Advance
   that never moves a step out of Done.
2. **No standing notes for pending steps.** `steps.New` takes names only and
   `Reset` clears every note, while this program's step rows carry a note
   before the run reaches them ("the game install and the mod library").
   `buildRun` keeps `buildSteps()` and re-applies the notes with
   `Set(i, Pending, note)` after `Reset`. Library candidate: `New` taking
   `Step`s, or `Reset` keeping notes.
3. **`logpane.Pane` cannot be told to resume following.** `Detach` forgets
   the offset but there is no `SetFollowing` or `Reset`, so a reader who
   scrolled up during one build starts the next build with the pane not
   following; the "Follow the tail" box is the way back. The previous code
   re-armed following on every run. Recorded, not worked around.
4. **`forms.SliderEntry` clamps a typed value before `Commit`.** Core's
   `SetTweakParam` also clamps and reports `Clamped`, which fed the banner
   "X only takes A to B, so C was used"; the library hands the clamped value
   to `Commit`, so core never sees the out-of-range number and the banner
   does not fire for bounded parameters. The field still shows the value that
   was stored (`TestTypingAnOutOfRangeValueShowsWhatWasStored`). Library
   candidate: an `OnClamped` hook, or the raw value beside the clamped one.
5. **No loader shape beside `Perform`.** `Perform` runs inline when the shell
   is off screen; a section load that calls `Busy` from its own goroutine has
   no such path, so the headless tests would race their builders. The program
   carries a five-line `load(what, run)` helper (as clockwork-orange does per
   loader). Library candidate: `Shell.Load`.
6. **`shell.AppearanceSection` and `AboutSection` need a `*shell.Shell` to
   build**, so R2.7 could not land in the leaves commit (R2); the leaves
   commit ported the Appearance section onto `fdtheme.Appearance` and the
   shell commit replaced it with the library's section. Sequencing, not a
   defect.
7. **`shell.About` has no link caption.** The hyperlink shows the URL; the
   previous section read "Project documentation". Library candidate:
   `About.URLText`.

Differences adopted rather than worked around (the program changed to the
library's shape, as the Context section says):

- `Shell.Report` says "*what* failed: *err*" where `report` said "*what*:
  *err*", and `Report`/`OK` are UI-thread calls where the program's hopped
  with `fyne.Do` themselves; every call site that reports from a goroutine
  wraps the call.
- `dialogs.BrowseButton` reads "Browse..." (three dots) where the program
  had "Browse…" (the design rule avoids glyphs outside the bundled font).
- The log pane's rows are `canvas.Text`, so a long line clips rather than
  ending in an ellipsis; the Copy button is "Copy" rather than "Copy log"; a
  "Last updated" stamp appears under the list; the copied text's dropped-lines
  note starts with "..." rather than "…". Lines keep core's wording with no
  timestamp or level prefix, as before (the level is the row's colour).
- Step rows: the name is bold only while the step runs and takes the state's
  colour; the note is dimmed. The program painted the name bold always in a
  90 px column and coloured the note.
- The Appearance section is the library's: it adds a Monospace font picker
  (which reaches the log pane) and an Interface scale picker with "Restart
  the window now", spells "Colour scheme", rewords the notes, and its sample
  shows the library's status words over this program's build log line. The
  preference store gains `appearance.mono` and `appearance.scale`; the three
  previous keys are read unchanged (AC5).
- Nine schemes instead of five: Windows Dark, Windows Light, macOS Dark and
  macOS Light follow the original five in `--scheme` and the picker.
- The Build toolbar is gated in the builder from state (`Gate`, the report,
  the install) and rebuilt by the shell when work starts and stops, instead of
  a `drawControls` walking button references; Cancel is still disabled in
  place the moment it is pressed.
- `widgets.Note`, `PlainRow`, `FactRow`, `HumanSize` and `HumanAgo` are
  identical in output to the copies they replace.
- Headless only: section loads run inline when the shell has no window, so
  the every-section test builds each section against a sandboxed HOME and
  sees finished state.

## Alternatives Considered

- Considered adopting `forms.Form` for `settingsForm`; deferred, its
  Save/Revert tests pin the current shape and the gain is small.
