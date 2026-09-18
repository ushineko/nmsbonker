# Spec 013: A reachable Cancel during a build

> **Note**: This work has no associated issue tracker ticket. The repository
> is a personal public project without an issue tracker.

## Status: COMPLETE

## Executive Summary

`startBuild` hands `cancelBuild` to `shell.BusyCancellable` (fynedesygn
v0.1.4), so a running build can be cancelled from the progress popup. The
popup is modal, and the Build toolbar's Cancel behind it had been on screen,
enabled and unclickable from 300 ms into every build since before the library
existed. One line of wiring plus the version bump; nothing else changes.
Reviewers should read `startBuild` in `internal/gui/state.go`.

## Context

Spec 012's validation report left one question for the manual run: the shell's
busy popup is modal and appears 300 ms into a build, so the Build toolbar's
Cancel — the one button deliberately left enabled while a build runs — is
behind a surface that swallows clicks. The manual run confirmed it. From
300 ms into a build until it ends, Cancel is on screen, enabled, and
unclickable; the only way out was to wait or kill the window.

It is not a regression from the adoption. `internal/gui/app.go` before the
port put up the same modal popup with the same rationale, and the button was
just as unreachable there. The adoption inherited it and made it the library's
to fix, which fynedesygn did in its spec 008: `shell.BusyCancellable` takes the
job's own cancel and puts it on the popup, where it is the one control in
front of the user.

`startBuild` holds the indicator itself rather than running through
`PerformCancellable`, because it pumps the log around `core.Build`, keeps the
`cancelled` flag that decides the summary banner, and reports its own outcome.
It therefore needs the `Busy` variant, not the `Perform` one.

## Requirements

- R1 `go.mod` requires `github.com/ushineko/fynedesygn` v0.1.4.
- R2 `startBuild` hands `cancelBuild` to `shell.BusyCancellable`, so the popup
  and the toolbar run the same cancellation: the `cancelled` flag, the
  "Cancelling." banner and the dead button.
- R3 Nothing else changes. Same preference keys, same operations, same
  behaviour everywhere a build is not running.

## Acceptance Criteria

- [x] AC1 `go.mod` requires fynedesygn v0.1.4 and `go mod tidy` leaves the
  `golang.org/x/*` modules at or above their current versions (R1).
- [x] AC2 `startBuild` calls `BusyCancellable` with `cancelBuild` itself, not
  a second cancellation path (R2).
- [x] AC3 `make test` (race, parity tag), `make lint` and `go vet` clean;
  `govulncheck -mode binary` on both binaries reports nothing (R3).
- [x] AC4 A build is started in the window and cancelled from the popup: the
  step list marks the running step cancelled, the banner says the previous
  output was put back, and the button is dead once pressed. Manual, and it has
  to be: the popup exists only on screen, and `testUI` is headless on purpose
  so that loads run inline (quirk 11). What each half is worth on its own is
  already covered — fynedesygn's `TestBusyCancellablePutsTheJobsOwnCancelOnThePopup`
  pins that the popup carries the cancel it was handed and that tapping it
  calls that cancel, and this program's `TestCancellingMarksOnlyTheRunningStep`
  and `TestPressingCancelDisablesItAtOnce` pin what `cancelBuild` does. The
  press is what joins them. _Done: a build started and cancelled from the
  popup in a window under a throwaway HOME, with the real game archives and
  MBINCompiler; the running step was marked cancelled, the banner reported the
  previous output put back, and the button was dead once pressed._

## Risks & Assumptions

- **Assumption**: `cancelBuild` is safe to call from the popup's button. It
  runs on the UI thread either way, and it already guards on `run.running`
  and a nil `run.cancel`, so a second press does nothing.
- **Risk**: none to the build itself; the cancellation path is the one that
  was already there, reached from a second button.
- **Rollback**: `git revert`; the previous binary reads the same
  configuration and preferences.
