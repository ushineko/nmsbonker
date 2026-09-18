# Validation report — spec 012, adopt fynedesygn

**Date**: 2026-09-17
**Spec**: [`012`](../specs/012-adopt-fynedesygn.md) adopt fynedesygn. A
milestone: the window's design system moves from hand-copied files to a
library dependency.
**Scope**: `internal/gui` (every file), `go.mod`/`go.sum`, `README.md`,
`docs/architecture.md`, `.claude/CLAUDE.md`. No change under `internal/core`
or the CLI.

**Environment**: Go 1.27.1 (go.mod `go 1.26.0`), golangci-lint v2.12.2
(under `GOTOOLCHAIN=go1.26.0`), govulncheck v1.1.4, Fyne v2.8.1,
fynedesygn v0.1.1, headless (no display server used).

---

## Summary

`internal/gui` imports `github.com/ushineko/fynedesygn` v0.1.1 for the
theme, fonts, cursor fix, widgets, table, dialogs, log pane, step list,
forms and the shell, and deletes its copies. Four commits: the leaves, the
shell, the documentation, this reconciliation. Behaviour is kept where the
library's shape allowed and recorded in the spec's "Gaps found" where it did
not; the library was not modified.

---

## Phase 3 — Tests

| Gate | Result |
| --- | --- |
| `make test` (`-race -tags parity`, 21 packages) | **pass** |
| `internal/gui` | **pass** — 56 tests, headless, HOME/XDG/STEAM_ROOT sandboxed |
| `tests/parity` (CLI ↔ GUI) | **pass** — `Actions()` unchanged, no allow-list change |
| `go vet ./...` | **clean** |
| `make lint` (golangci-lint v2.12.2) | **0 issues** |
| `gofmt -l internal cmd tests` | **clean** |

Tests deleted because the library carries them: the log model's cap, drop
count, index tolerance and dirty flag; the follow-tail rule; the chooser tap;
`pickerStart`; the banner hold and stale-timer tests; `find_test.go`
(`walk`, `findSlider`, `findEntry`, `cardText`, replaced by `fynetest`).

Tests added: `TestASavedAppearanceFromThePreviousBuildIsReadUnchanged` (AC5),
`TestEverySectionRendersHeadlesslyInEveryScheme` (AC7, nine schemes),
`TestProgressEventsAdvanceTheStepList` and
`TestBuildLinesKeepCoresWordingAndLevel` (AC6),
`TestPressingCancelDisablesItAtOnce`,
`TestFinishingMarksEveryStepThroughTheReportDone`,
`TestResetPutsTheStandingNotesBack`,
`TestSectionSelectionResolvesNamesAndFallsBackToTheFirst`,
`TestAboutNamesTheProgramAndItsFacts`. The control tests now build the real
Build section and read its buttons instead of driving `drawControls`.

---

## Phase 4 — Code quality

- Dead code: none left; every helper the library replaced is deleted, and the
  `Copied from` headers with them.
- Duplication: the twelve loaders share one `load` helper (inline off screen,
  `Busy` on a goroutine on screen) instead of twelve `go func` blocks.
- Encapsulation: `buildRun` keeps the run's state and the progress-to-step
  mapping (`buildrun.go`); drawing is the library's. `app.go` is the `ui`
  state, the section registry and `shellOptions`.

## Phase 5 — Security

- `govulncheck -mode binary bin/nmsbonker` (CLI, `CGO_ENABLED=0`): **no
  vulnerabilities found**.
- `govulncheck -mode binary bin/nmsbonker-gui` (GUI, `CGO_ENABLED=1`): **no
  vulnerabilities found**.
- New dependency: `github.com/ushineko/fynedesygn` v0.1.1 (same author, MIT),
  pinned by `go.sum`; `golang.org/x/{image,net,sys,text}` rose with it. No
  `replace` directive. No secrets, no new subprocess paths (the desktop
  opener moved into the library unchanged: one argv element, no shell).

## Phase 5.5 — Release safety

- Rollback: `git revert` of the four commits. The previous binary reads the
  same three appearance keys and the same `config.json`; the two new keys the
  library writes (`appearance.mono`, `appearance.scale`) are ignored by it.
- Additive: no operation, flag or config key removed; `SectionNames()`,
  `SchemeNames()` (first five unchanged, four appended) and `Actions()` keep
  their contract.

---

## Line counts (`internal/gui`, AC11)

| | Before (29a77d6) | After (a64693f) |
| --- | --- | --- |
| Non-test `.go` | 8,244 | 6,059 |
| Test `.go` | 1,375 | 1,290 |

Counted as the sum of `wc -l` over each `.go` file under `internal/gui` at
that commit. An earlier revision of this table gave the "after" figure as
7,344, which was measured before the shell commit landed and never corrected;
the breakdown below sums to the same 6,059.

Deleted: `theme.go` (288), `fonts.go` (236), `cursor_linux.go` (100),
`cursor_other.go` (10), `views_table.go` (144), `dialogs.go` (255),
`buildlog.go` (409), `find_test.go` (102). Added: `buildrun.go` (207).
`app.go` 984 → 382, `state.go` 596 → 497, `views_build.go` 350 → 236,
`views_appearance.go` 106 → 23, `views_about.go` 127 → 81.

---

## Deviations

- **R2.7 landed in the shell commit, not the leaves commit.**
  `shell.AppearanceSection` and `shell.AboutSection` build against a
  `*shell.Shell`, which the leaves commit does not have; that commit ported
  the Appearance section onto `fdtheme.Appearance` (same keys) and the shell
  commit swapped both sections for the library's.
- **R2.6's `forms.IntRange` / `forms.NumericEntry` were not adopted.** The
  settings form validates no numbers of its own (core validates on Save, and
  the ratio limit is not a whole number), so there was no place for them.
- **`go.mod` moved to `go 1.26.0`**, which the library requires. README and
  the project guidelines say so; CI already builds with Go 1.27.
- **AC9 (a manual run under a throwaway HOME) is left to the user.** The spec
  stays INCOMPLETE until it is recorded.
- Behaviour differences adopted from the library are listed in the spec's
  "Gaps found"; the one worth a look in the manual run is the Build toolbar:
  as before this change, the shell's busy popup is modal and appears 300 ms
  into a build, so the toolbar's Cancel is reachable only in that window.
  That was the previous build's behaviour too (`busy` showed the same modal
  popup) and is not changed here, but it is worth confirming what the user
  sees.
