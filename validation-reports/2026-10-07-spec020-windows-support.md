# Validation report — spec 020, Windows support

**Date**: 2026-10-07 21:55
**Spec**: [`specs/020-windows-support.md`](../specs/020-windows-support.md), issue #31
**Branch**: `feat/windows-support`
**Machine**: Windows 11 Pro 10.0.26200, Go 1.27.0 windows/amd64, MSYS2 UCRT64
gcc 16.2.0, .NET runtime 10.0.12, Steam + No Man's Sky buildid 25732212
**Status**: PASSED for what can be checked on this machine; the Linux test run
is CI's, and AC4, AC6 (real-profile edit), AC8 (build/deploy from the window),
AC9 and AC10 are open (see the spec).

---

## Phase 3: Tests

| Run | Command | Result |
| --- | --- | --- |
| Windows, cgo, race, parity | `go test -count=1 -race -tags parity ./...` | 18/18 packages pass, GUI included |
| Windows, no cgo (CI's mode) | `CGO_ENABLED=0 go test $(go list -e ./... \| grep -v gui)` | all pass |
| Windows, real game | `NMSBONKER_GAME_DIR=… go test -run 'Real\|Installed\|Pak' ./internal/...` | `TestEveryBuiltInFindsEveryKeyInTheInstalledGame` passes (62 s); see below for the two that did not |
| Linux | `GOOS=linux go vet` (non-GUI, tests included) | clean; the test run itself is CI's (no Linux machine or WSL here) |
| macOS | `GOOS=darwin CGO_ENABLED=0 go build ./cmd/nmsbonker` | builds |

Two real-game tests did not pass, neither for a code reason:

- `TestStatusFindsTheRealInstall` requires `GCMODSETTINGS.MXML`, which this
  fresh install does not have until the game has been started with a mod
  folder present (the spec records that `Binaries\SETTINGS` holds only `HDR\`).
- `TestBuildingThePakIndexIsFastColdAndFasterWarm`: warm load 321 ms against a
  200 ms budget while the 62 s build test ran beside it; alone, three runs gave
  144, 181 and 188 ms.

New tests: Windows config defaults and `~\`; Steam roots de-duplicated across
spellings, junction at `MODS` reported as a link, save folder under AppData
(and inside the Proton prefix on Linux); the Toolhelp process scan; `/proc`
scan split into its own Unix test; deploy and rollback refusing while the game
runs; case-folded export refusal; rename retry against a real open-file lock;
the missing-.NET hint with a real exit code; every built-in meaning the same
with CRLF line endings.

Coverage was not measured for this change.

## End to end on this machine

| Step | Result |
| --- | --- |
| `status` | game found from the registry, buildid 25732212, MODS absent, save folder `%APPDATA%\HelloGames\NMS` |
| `tools ensure` | `MBINCompiler-dotnet10.exe` v7.04.1-pre3 installed and verified, `mapping.json` fetched |
| `tools ensure`, `self-contained` flavor | `MBINCompiler.exe` exits `0x80008096` (needs .NET 8): reported with the install hint, falls back to dotnet10 |
| `build`, all 30 built-ins | all WORKING (one WORKING\*), 88 MBINs, 0 dropped, 330 edits, compiler round-trip compatible, 32 s cold |
| decompiled MXML line endings | all 88 LF; the CR bytes found were inside raw `.mbin` files |
| `deploy` ×2, scratch game dir | 94 files installed, save backup taken, second deploy archived the first |
| deploy/rollback with `NMS.exe` running | both refused, `MODS` unchanged; undeploy worked after it exited |
| `saves slots/inspect/export` | real profile decoded; upper-cased export path into the save folder refused |
| `nmsbonker-gui.exe` | GUI subsystem, icon embedded, renders at 150% scaling with game and compiler found |

Test traces were removed afterwards: scratch game directory, the three archive
entries the scratch deploys created, the exported save JSON. The 30 built-ins
were disabled again. The installed compiler, the cache and two save backups
(copies of the real profile) were kept.

## Phase 4: Code quality

- Dead code: `core.savesRelative` and the Linux-only `process_other.go` were
  removed; the four `#!/bin/sh` compiler stubs replaced by one Go fake.
- Duplication: the four stub scripts became one program with a `kind` knob;
  every production `os.Rename` goes through one helper. The test helper
  `gameSaveDir` repeats the save path deliberately: it is the expectation.
- Encapsulation: platform differences sit in `_windows.go` / `_unix.go`
  (`_other.go`) pairs behind one function each, listed in
  `docs/architecture.md`.
- Status: PASSED

## Phase 5: Security review

- Dependencies: no module added and no version changed; three go from indirect
  to direct (`golang.org/x/sys`, `srwiley/rasterx`, `fyne-io/oksvg`).
  `govulncheck` is not installed on this machine, so no CVE scan was run.
- Lint: golangci-lint v2.12.2 clean as Windows (cgo), as Linux (non-GUI) and in
  `lint-windows` mode. The test-support package `internal/mbin/mbintest` joins
  `_test.go` files in the existing gosec/wrapcheck exclusion.
- Injection: the only new child processes are the existing compiler runner
  (fixed binary path, allow-listed environment) and the `dotnet --list-runtimes`
  probe. The Windows allow-list adds only system variables a process needs to
  start (`SystemRoot`, `TEMP`, `APPDATA`, …), none of them secrets.
- Access control: the export guard now also covers the Windows save folder,
  compared without case. Deploy, rollback and undeploy refuse while the game
  runs on Windows.
- Secrets and personal data: none in the change; the binary `.ico`/`.syso`
  checked for embedded paths (none). The registry is read, never written.
- CI: one new third-party action, `msys2/setup-msys2@v2`, pinned by major tag
  like the existing actions.
- Status: PASSED (CVE scan not run, documented above)

## Phase 5.5: Release safety

- Change type: code-only, plus CI.
- On-disk formats: unchanged on both platforms. Linux paths and behaviour are
  unchanged apart from the `SaveDir` refactor and the test helper.
- Rollback: revert the merge commit. Nothing on a Linux machine needs undoing.
- Rollout: the Windows zip appears on the next tagged release; until then
  Windows users build from source.
- Status: PASSED

## Overall

- All gates that can run here passed: YES
- Before release: bump `VERSION`, the README's version line and add the
  changelog entry (not done here, by the project rule to ask first); watch the
  first CI run of the two new Windows jobs.
