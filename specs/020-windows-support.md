# Spec 020: Windows support

**Issue**: #31

## Status: IMPLEMENTED (AC4, AC6 real-profile edit, AC8 GUI build/deploy, AC9, AC10 open)

## Executive Summary

nmsbonker runs natively on Windows 10/11 against a Steam install of the game:
the CLI and the GUI find the game through the registry, build with the Windows
MBINCompiler, deploy into `GAMEDATA\MODS`, and back up and edit the saves in
`%APPDATA%\HelloGames\NMS`. Linux behaviour is unchanged. Reviewers should look
first at R4 (the save folder moves from "inside the Proton prefix" to a field on
`steam.Install`, because the write guard in `insideTheGame` depends on it) and
R6 (the child-process environment allow-list, which as written strips variables
a .NET program on Windows cannot start without).

## Context

The project was written for Steam + Proton on Linux, and its README says so.
Windows was never a target because the tool existed to avoid needing it. A
Windows build is now wanted for a bare-metal Windows 11 machine (RTX 3060 Ti,
Steam and the game freshly installed), and most of the pipeline turns out to be
platform-neutral already: the HGPAK reader, the Lua host, the MXML engine, the
save codec and the cache are pure Go.

Facts established on that machine, 2026-10-07:

- Go 1.27.0 windows/amd64. `CGO_ENABLED=0 go build ./cmd/nmsbonker` succeeds
  unmodified. MSYS2 UCRT64 gcc 16.2.0 is at `C:\msys64\ucrt64\bin` (not on
  the default `PATH`); with it first on `PATH`,
  `go build -tags migrated_fynedo -ldflags "-H windowsgui" ./cmd/nmsbonker-gui`
  succeeds unmodified. .NET runtime 10.0.12 installed via winget
  (`Microsoft.DotNet.Runtime.10`) for AC2. No `make`.
- Steam root `C:\Program Files (x86)\Steam`, from both
  `HKCU\Software\Valve\Steam\SteamPath` (stored lower-case with forward
  slashes: `c:/program files (x86)/steam`) and
  `HKLM\SOFTWARE\WOW6432Node\Valve\Steam\InstallPath`. `libraryfolders.vdf`
  lists the path with escaped backslashes, which `parseVDF` already unescapes.
- Game at `steamapps\common\No Man's Sky`, buildid `25732212`, 98 entries in
  `GAMEDATA\PCBANKS`, no `GAMEDATA\MODS` yet, `Binaries\SETTINGS` holds only
  `HDR\` (no `GCMODSETTINGS.MXML` until the game has run with a mod folder).
  `BUILTIN\Users` has full control of `GAMEDATA` (inherited from Steam's own
  ACL), so an unelevated process can write there. No `compatdata` directory.
- Saves at `%APPDATA%\HelloGames\NMS\st_<steamid>\` (Steam Cloud synced on
  first launch). Same `st_*` layout as under Proton.
- MBINCompiler releases (checked at `v7.04.1-pre3`) publish Windows assets
  beside the Linux ones: `MBINCompiler-dotnet10.exe` + `libMBIN-dotnet10.dll`
  (framework-dependent) and `MBINCompiler.exe` + `libMBIN.dll`.
- `go test ./internal/...` with `CGO_ENABLED=0`: everything outside
  `internal/gui` builds; 20 tests fail in `internal/mbin`, `internal/core` and
  `internal/build/cache`. 19 run a `#!/bin/sh` script as a stand-in compiler
  (`exec: ... executable file not found`); one asserts a save file keeps mode
  `0755`, which Windows reports as `0666`. `internal/gui` does not build
  without cgo, as on Linux.
- `core.autocrlf=true` and the repository has no `.gitattributes`, so this
  checkout holds the built-in tweak scripts with CRLF line endings while a
  Linux checkout holds LF.

## Requirements

### R1 Platform directories

- R1.1 On Windows the defaults are: settings `%APPDATA%\nmsbonker\`
  (`os.UserConfigDir`), data (library, tools, workspace, archive, save backup)
  `%LOCALAPPDATA%\nmsbonker\`, cache `%LOCALAPPDATA%\nmsbonker\cache\`
  (`os.UserCacheDir` + `nmsbonker\cache`). Data is local, not roaming: the
  workspace and cache are gigabytes and machine-specific.
- R1.2 `XDG_CONFIG_HOME`, `XDG_DATA_HOME` and `XDG_CACHE_HOME` are honoured on
  every platform when set, so the tests that set them keep working unchanged.
- R1.3 Linux defaults are unchanged.
- R1.4 `ExpandPath` also accepts `~\` on Windows. The bare-tilde refusal
  (`CheckCreatablePath`) is unchanged.

### R2 Steam discovery

- R2.1 On Windows, `steam.Roots()` returns, in order and de-duplicated:
  `STEAM_ROOT`; `HKCU\Software\Valve\Steam\SteamPath`;
  `HKLM\SOFTWARE\WOW6432Node\Valve\Steam\InstallPath`;
  `%ProgramFiles(x86)%\Steam`. Registry access uses
  `golang.org/x/sys/windows/registry` (already an indirect dependency).
- R2.2 `canonical()` folds case and separators on Windows, so `SteamPath`'s
  `c:/program files (x86)/steam` and the VDF's `C:\Program Files (x86)\Steam`
  are one library, not two.
- R2.3 `steam.Install` reports no `CompatDataDir` on Windows.
- R2.4 A directory junction at `GAMEDATA\MODS` is treated as `ModsSymlink`
  (Go reports junctions as `ModeIrregular`, not `ModeSymlink`), so deploy never
  writes through one without `--replace-symlink`.

### R3 MBINCompiler

- R3.1 Asset names come from a per-OS table. Windows: `dotnet10` →
  `MBINCompiler-dotnet10.exe` + `libMBIN-dotnet10.dll`; `self-contained` →
  `MBINCompiler.exe` + `libMBIN.dll`. Linux unchanged. The flavor names and the
  `auto` fallback order are unchanged.
- R3.2 `HasDotnet10` finds `dotnet.exe` the way the compiler's own host does:
  `DOTNET_ROOT`, then `PATH`, then `%ProgramFiles%\dotnet`. *Revised in
  implementation:* `PATH` alone reported "no runtime" in a terminal opened
  before the runtime was installed, while the compiler ran fine.
- R3.3 *Revised in implementation:* the release picker does not look at assets
  on either platform (the original wording was wrong about today's behaviour);
  a release without this platform's assets fails the install with a message
  naming the platform, as it already did for Linux.
- R3.4 *Added in implementation:* on Windows neither build is self-contained.
  `MBINCompiler.exe` (the `self-contained` flavor) is framework-dependent on
  .NET 8 (checked at v7.04.1-pre3). The flavor name is kept because it is a
  config value. When a compiler exits with the .NET host's "framework missing"
  code (`0x80008096`), the attempt line says to install the .NET 10 runtime.

### R4 Saves

- R4.1 `steam.Install` gains `SaveDir`: on Linux the existing
  `compatdata/275850/pfx/.../HelloGames/NMS` path (only when compatdata exists);
  on Windows `%APPDATA%\HelloGames\NMS` whether or not it exists yet (callers
  already report a missing folder, now as "start the game once"). `saves.go`
  and `saveedit.go` read `SaveDir` instead of joining `savesRelative`
  themselves; `status` prints it.
- R4.2 `insideTheGame` refuses exports into the game directory and into
  `SaveDir` (on Linux, still the whole compatdata tree). Containment is
  case-insensitive on Windows.
- R4.3 `gameRunning` on Windows enumerates processes
  (`CreateToolhelp32Snapshot`) for `NMS.exe`, case-insensitive. Linux keeps the
  `/proc` scan. The seam the tests use is a function variable rather than
  `procRoot`.
- R4.4 Save writes keep the existing file's mode where the OS has one; on
  Windows the read-only attribute is what survives. The test asserts "mode
  unchanged from before", not a literal `0755`.
- R4.5 Messages that say "Proton prefix" say "save folder" (with the path)
  on every platform.

### R5 Deploy and file replacement on Windows

- R5.1 Deploy refuses while `NMS.exe` is running on Windows, with the same
  `ErrGameRunning`. The running game holds its mod files open, and a rename
  that fails halfway leaves the archive and `MODS` out of step. Linux behaviour
  is unchanged. Rollback and undeploy, which move the same folder, refuse too.
- R5.2 Every rename in non-test code goes through `fsutil.Rename`, which
  retries `ERROR_ACCESS_DENIED`, `ERROR_SHARING_VIOLATION` and
  `ERROR_LOCK_VIOLATION` for up to 2 s with backoff (Linux: plain `os.Rename`). A scanner (Defender)
  briefly opening a file that was just written is the usual cause, and the
  retry is the standard answer to it. After the budget, the error is reported as
  today.
- R5.3 Atomic-replace writes (`config.go`, `modsettings.go`, `saveedit.go`,
  `mbin/install.go`) are checked to work when the destination exists on
  Windows (`os.Rename` uses `MoveFileEx` with `REPLACE_EXISTING`, which does,
  for files) and to fail cleanly when the destination is open.

### R6 Child processes

- R6.1 `minimalEnv` keeps, on Windows, the variables a Windows program needs to
  start and find its own files: `SystemRoot`, `windir`, `SystemDrive`,
  `ComSpec`, `PATHEXT`, `TEMP`, `TMP`, `USERPROFILE`, `APPDATA`,
  `LOCALAPPDATA`, `ProgramData`, `ProgramFiles`, `ProgramFiles(x86)`,
  `ProgramW6432`, `NUMBER_OF_PROCESSORS`, `PROCESSOR_ARCHITECTURE`, in
  addition to `PATH` and `DOTNET_*`. Still an allow-list; still nothing else.
  Windows environment names are matched case-insensitively.
- R6.2 Cancellation on Windows kills the compiler process (existing
  `exec.CommandContext` + `WaitDelay`). No job object: MBINCompiler is one
  process, and the tests no longer start a shell (R8).
- R6.3 Child processes are started without a console window
  (`CREATE_NO_WINDOW`), so a build does not flash one per conversion. *Revised
  in implementation:* always, not only under the GUI. The output is piped in
  both front ends, so a console adds nothing to the CLI either.

### R7 Line endings

- R7.1 A `.gitattributes` makes the checkout identical across platforms:
  `* text=auto eol=lf`, with `*.png` and other binaries marked `binary`. SVG
  stays text (marking it binary stored the CRLF working copy). Committed after
  a renormalise that produced no content change.
- R7.2 A script with CRLF line endings means what it means with LF: every
  built-in is loaded both ways and must give the same change tables, header,
  parameters, and parameter override (`internal/tweaks/crlf_test.go`). MXML
  needs no test: MBINCompiler writes LF on Windows too (all 88 decompiled files
  of a real build checked).

### R8 Tests run on Windows

- R8.1 The stand-in compiler is a Go program, not a shell script
  (`internal/mbin/mbintest/fakembin`), ported from the four shell stubs and
  selected by a `kind` file beside it. `mbintest.Install` builds it once per
  test process (cached in the temp directory by source hash) and installs it
  under this platform's asset names. *Revised in implementation:* not a
  re-executed test binary with an environment flag. The runner strips the
  environment, and a separate program needs no `TestMain` in four packages.
  It serves `internal/mbin`, `internal/build`, `internal/build/cache` and
  `internal/core`.
- R8.2 Tests that assert Unix modes or `/proc` layout are split by build tag or
  rewritten to assert the platform's behaviour.
- R8.3 `go test ./...` passes on Windows with `CGO_ENABLED=0` for every package
  except `internal/gui` and `cmd/nmsbonker-gui`, and on Windows with cgo for
  all of them. The game-data integration tests run when `NMSBONKER_GAME_DIR`
  is set, as on Linux.
- R8.4 CI runs the unit tests on `windows-latest` (CLI packages, no cgo) on
  every push.

### R9 GUI

- R9.1 `nmsbonker-gui.exe` builds with MSYS2 UCRT64 gcc and links with
  `-H windowsgui`, so it opens without a console.
- R9.2 The executable carries the application icon. *Decided:* committed.
  `tools/winicon` renders the SVG into `packaging/windows/nmsbonker.ico` (nine
  sizes, stroke widths scaled per size), and `windres` compiles
  `packaging/windows/nmsbonker.rc` into
  `cmd/nmsbonker-gui/rsrc_windows_amd64.syso` (`make winres`). A plain
  `go build` on Windows needs neither tool.
- R9.3 User-facing text that names Linux, Proton or XDG as the only case is
  made platform-neutral: the About blurb, the "Keep files where you expect"
  note, `cli/root.go`'s short description, `cli/saves.go`'s long help, the
  `--config` default in help text (printed from the resolved path rather than
  a literal).
- R9.4 Fynedesygn's behaviour on Windows (fonts, DPI scaling, window
  decorations, file dialogs) is checked against its design rules. Anything the
  library gets wrong goes to "Gaps found" for a library change, not into
  `internal/gui`.

### R10 Build and release

- R10.1 The Makefile is not required on Windows. The README gives the plain
  `go build` lines for both binaries. They stamp no version, so the binary
  reports `dev`; the CI zip carries the Makefile's version rule.
  `make lint-windows` lints the Windows files from Linux, and CI runs it.
- R10.2 `make build-all` adds `windows/amd64` to the static CLI targets.
- R10.3 A `windows-zip` job (`windows-latest`, MSYS2 UCRT64 gcc) builds
  `nmsbonker.exe` and `nmsbonker-gui.exe` on every push and uploads
  `nmsbonker-VERSION-windows-amd64.zip` (both executables, README, LICENSE).
  The Release job attaches it and adds it to `SHA256SUMS`. The tag/`VERSION` check and the changelog extraction
  are unchanged.
- R10.4 No installer, no Start-menu entry, no winget manifest in this spec.
  Installing is unzipping.

### R11 Documentation

- R11.1 README: Windows is a supported platform, with a per-platform table of
  where settings, data, cache, the game and the saves are found; a Windows
  quick start (download the zip, run `nmsbonker status`, build, deploy); and a
  note that the game must have been started once for `GCMODSETTINGS.MXML` to
  exist (deploy already says this when it is absent).
- R11.2 `docs/architecture.md` names the per-OS files (`*_windows.go`,
  `*_unix.go`) and what each one owns.

## Acceptance Criteria

- [x] AC1 On the Windows machine, `nmsbonker status` finds the game from the
  registry with no `STEAM_ROOT` or `--game-dir`, reports buildid (25732212),
  MODS state `absent`, and the save folder under `%APPDATA%`. Paths are shown
  in the file system's case, not the registry's lower case.
- [x] AC2 *Revised:* `nmsbonker tools ensure` installs and verifies
  `MBINCompiler-dotnet10.exe` v7.04.1-pre3 with the .NET 10 runtime present.
  With the `self-contained` flavor configured and no .NET 8, `MBINCompiler.exe`
  fails to start, the attempt line says to install the .NET 10 runtime, and
  `auto`'s fallback lands on `dotnet10`. The original wording ("with no runtime,
  self-contained works") was wrong about the Windows assets (R3.4).
- [x] AC3 `nmsbonker build` with all 30 built-ins: every one `WORKING` (one
  `WORKING*`), 88 MBINs built, 0 dropped, 330 edits, 0 skipped, compiler
  round-trip compatible, 32 s cold. `TestEveryBuiltInFindsEveryKeyInTheInstalledGame`
  passes on Windows with `NMSBONKER_GAME_DIR` set.
- [ ] AC4 Golden parity on Windows: the decompiled output of a build matches
  the same build's output on Linux byte for byte, for the same game buildid,
  scripts and compiler version. *Open:* needs a Linux build of buildid
  25732212. Supporting evidence: the checkout is byte-identical (R7.1), and
  MBINCompiler writes LF MXML on Windows (R7.2).
- [x] AC5 `nmsbonker deploy`, real build, into a scratch game directory (not
  the real game): 94 files under `GAMEDATA\MODS\COSMOS COMBINE` and a save backup
  of the real profile; a second deploy archived the first. With a process named
  `NMS.exe` running, deploy and rollback refused with "the game is running" and
  the `MODS` tree was unchanged; undeploy worked once it exited.
- [ ] AC6 Save editor: with the game closed, an edit backs up the profile and
  rewrites the slot; with the game running, it refuses. Exporting into the game
  directory or the save folder is refused, including through a differently
  cased spelling of the path. *Verified:* `saves slots`, `saves inspect` and
  `saves export` on the real Windows profile; an upper-cased export path into
  the save folder refused; edit, backup and refusal pass in the unit tests on
  NTFS. *Open:* an edit of a real profile, left to the user.
- [x] AC7 `go test ./...` passes on Windows (cgo with `-race -tags parity`,
  and no-cgo, R8.3) and on Linux. `make test` on Linux still includes the
  parity test and passes. *Verified:* Windows both ways locally; Linux `make
  test`, `make lint` and `make lint-windows` in CI on PR #32, with the Windows
  test job, the Windows zip and the Arch package.
- [ ] AC8 `nmsbonker-gui.exe` opens without a console window, shows the icon,
  and runs status, build (with a cancel partway through) and deploy without a
  console flash per conversion. *Verified:* GUI subsystem (no console), icon in
  the executable and the title bar, Overview and About render at 150% scaling
  with the game and compiler found. *Open:* build, cancel and deploy driven
  from the window.
- [ ] AC9 In game: the built-in mod deployed by AC5 is active on the Windows
  machine (one visible tweak checked). Recorded by the user.
- [ ] AC10 A pushed tag produces the Windows zip on the Release page alongside
  the existing assets (checked on the next release, not by a throwaway tag).

## Non-goals

- Game Pass / Microsoft Store and GOG installs. Game Pass saves use a different
  container format; GOG has no Steam manifest. `--game-dir` covers a GOG game
  directory for build and deploy, but saves are not located for it.
- Windows on ARM, 32-bit Windows.
- An installer, auto-update, or file associations.
- macOS (still CLI-only and untested, as the README says today).

## Risks & Assumptions

- ~~The Windows `MBINCompiler.exe` is assumed to run without a .NET runtime.~~
  Wrong: it needs .NET 8 (R3.4). The README says to install .NET 10.
- NTFS is case-insensitive. The engine writes MBIN paths from pak paths, which
  are already lower-cased for lookup; two scripts that emit the same file under
  different case would merge on Linux only if spelled identically, and collide
  on Windows. *Resolved:* output paths are already upper-cased
  (`build.InternalUpper`), so two spellings of one file are one target on both
  platforms.
- Paths longer than 260 characters: Go adds the `\\?\` prefix itself for
  absolute paths, so no manifest change is needed. A workspace path under a
  long user name plus deep pak paths is the case to watch.
- The cgo toolchain for the GUI on Windows is MSYS2 (UCRT64) gcc, locally and
  in CI. The README says to put its `bin` on `PATH` for the GUI build.
- Defender may slow the cache build. *Measured:* a cold build of all 30
  built-ins took 32 s (cache 13.6 s) with Defender on, so the README does not
  mention it.
- The warm pak-index timing test (200 ms budget) measured 144–188 ms alone and
  321 ms while a full build ran beside it. It is load-sensitive, as on Linux.
- Rollback: Linux paths are untouched apart from the `SaveDir` refactor (R4.1)
  and the test helper (R8.1). Reverting the merge commit restores 0.6.0
  behaviour; no on-disk format changes on either platform.

## Alternatives Considered

- **Keep XDG-style directories under `%USERPROFILE%`** (`.config`,
  `.local\share`). One code path, but every Windows tool that shows "where are
  my files" points at `%APPDATA%`/`%LOCALAPPDATA%`, and roaming profiles would
  sync the wrong things. Rejected.
- **Run under WSL.** WSL cannot see the Windows Steam registry, writes into
  NTFS through 9p slowly, and the GUI needs WSLg. Rejected; the point is a
  native tool.
- **Process detection via `tasklist`.** Spawns a process and parses localised
  output. Rejected in favour of the snapshot API.

## E2E Test Plan

1. On the Windows machine, unzip a local build; run `nmsbonker status` (AC1).
2. `nmsbonker tools install`; read the flavor attempts (AC2).
3. Enable all built-ins, `nmsbonker build`; read the report (AC3).
4. Start the game once, quit; `nmsbonker deploy`; deploy again (AC5).
5. Start the game, try deploy and a save edit, both refused (AC5, AC6).
6. Load a save; check one tweak in game (AC9).
7. Repeat 2–5 from `nmsbonker-gui.exe` (AC8).

## Gaps found

None so far. Checked at 150% display scaling (DPI 144): native title bar and
icon, the responsive tab layout, fonts, dark scheme, status bar. File dialogs
and the light schemes were not exercised.
