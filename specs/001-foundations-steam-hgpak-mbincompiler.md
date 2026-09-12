# Spec 001 — Foundations: project scaffold, config, Steam detection, native HGPAK reader, MBINCompiler manager, CLI skeleton

## Status: INCOMPLETE

## Context

`~/Games/nms-modding/` holds a working but ad-hoc Linux pipeline that rebuilds
No Man's Sky mods from AMUMSS `.lua` scripts: a bash driver, two Python scripts,
a Lua dumper, AMUMSS's `hgpaktool.exe` run under Wine, and a hand-downloaded
MBINCompiler. It is being refactored into `nmsbonker`, a Go project with a cobra
CLI and a Fyne GUI, published at `github.com/ushineko/nmsbonker`.

This spec is phase 1 of 4. It delivers everything *below* the mod pipeline: the
repository scaffold, configuration, locating the game through Steam, reading
the game's `.pak` archives natively (replacing Wine + hgpaktool), and acquiring
and driving MBINCompiler. Phase 2 (spec 002) builds the mod pipeline on top;
phase 3 (spec 003) the GUI; phase 4 (spec 004) tweaks, deploy/rollback,
migration and packaging.

Facts established during investigation (2026-09-11), to be relied on:

- Game: Steam app `275850`, install dir named `No Man's Sky`, data under
  `GAMEDATA/PCBANKS/*.pak` (101 entries, 97 `.pak`), mods under `GAMEDATA/MODS/`,
  mod settings at `Binaries/SETTINGS/GCMODSETTINGS.MXML`. Current Steam buildid
  `25233815`, game version line "COSMOS" (7.x).
- PC paks are **HGPAK version 2**. Layout (all little-endian): `"HGPAK"` + 3 pad
  bytes; at offset 8: `version u64`, `file_count u64`, `chunk_count u64`,
  `is_compressed u8` + 7 pad, `data_offset u64` (header ends at 0x30). Then
  `file_count` entries of `{md5 [16]byte, start_offset u64, decompressed_size u64}`.
  If compressed, then `chunk_count` × `u64` compressed chunk sizes; each chunk
  starts at `data_offset` + sum of previous sizes rounded up to 0x10. Chunks
  decompress to `0x10000` bytes (zstd on Windows/Linux paks; mac paks use lz4
  with 0x20000 chunks, Switch uses Oodle — both out of scope). A chunk whose
  zstd decode fails and whose stored length is exactly `0x10000` is stored raw.
  File entry 0 is the **filename manifest**: CRLF-separated relative paths, one per
  remaining entry, in order (`files[i]` ↔ `fileInfo[i+1]`). Entry offsets are
  absolute in the pak for the uncompressed case and relative to the start of the
  decompressed stream (offset − `data_offset`) for the compressed case. The md5 is
  of the lower-cased, forward-slash path. Reference implementation:
  `github.com/monkeyman192/HGPAKtool` (`hgpaktool/api.py`, MIT).
- Globals live at the pak root (`gcgameplayglobals.global.mbin`), not under
  `GLOBALS/`; scripts refer to them either way.
- MBINCompiler (`github.com/monkeyman192/MBINCompiler`) publishes per-release
  assets: `MBINCompiler-linux-dotnet10` + `libMBIN-linux-dotnet10.so`
  (framework-dependent, needs a .NET 10 runtime, present on this box) and
  `MBINCompiler-linux` + `libMBIN-linux.so`. Release tags look like
  `v7.02.0-pre1`; major.minor tracks the game version; `prerelease` flags are
  unreliable (7.01.0-pre1 is flagged as a full release). CLI: default mode
  converts the given path(s) MBIN↔MXML next to the source; options `-y/--overwrite`,
  `-q/--quiet`, `-Q/--nolog`, `-d/--output-dir <dir>`, `-i/--input-format`,
  `-o/--output-format`, `--include`, `--exclude`, `--no-threads`; `version [file]`
  prints the binary version, or the libMBIN version a `.MBIN`/`.MXML` was made
  with; `help` prints usage; there is no `--help`. It writes a log file next to
  the binary (`<binary>.log`). Exit code is non-zero on error.
- Local toolchain: Go 1.27.1, gcc 16, pkg-config has gl/x11/xcursor/xrandr/
  xinerama/xi/xxf86vm/wayland-client; `golangci-lint` v2.12.2 at `~/go/bin`;
  dotnet runtimes 9.0.19 and 10.0.11.
- Design/engineering reference: `~/git/angou` (module layout, Makefile, lint
  config, spec format, testing idioms). This spec copies its conventions.

## What this is not

- Not the mod pipeline (Lua, MXML editing, build, report): spec 002.
- Not the GUI: spec 003. This spec must leave `internal/core` shaped so the GUI
  can call it without refactoring.
- Not pak *writing*/repacking, not mac/Switch paks, not MBIN parsing in Go
  (MBINCompiler remains the only MBIN↔MXML converter).
- Not a Steam client integration; it reads Steam's on-disk manifests only.

## Requirements

### R1 — Repository scaffold

- R1.1 Module `github.com/ushineko/nmsbonker`, `go 1.25.0` in `go.mod` (no
  `toolchain` line; the local toolchain may be newer). Direct dependencies for
  this spec: `github.com/spf13/cobra`, `github.com/klauspost/compress` (zstd),
  `github.com/stretchr/testify`. Nothing else without a sentence of justification
  in the commit message.
- R1.2 Layout: `cmd/nmsbonker/main.go` (CLI, must build with `CGO_ENABLED=0`),
  `internal/buildinfo` (`Version`, `Commit`, defaults `dev`/`unknown`, injected
  by `-ldflags -X`), `internal/config`, `internal/steam`, `internal/hgpak`,
  `internal/mbin`, `internal/core`, `internal/cli`. `cmd/nmsbonker-gui` is
  created in spec 003, not now.
- R1.3 `Makefile` in angou's shape: `help` (awk `##` help), `setup`,
  `install-lint` (pinned `LINT_VERSION=v2.12.2`, SHA256-verified tarball),
  `lint` (uses `config/.golangci-v2.12.2.yml`), `test` (`go test -race ./...`),
  `coverage`, `build` (CLI, `CGO_ENABLED=0`, `-trimpath`, ldflags with
  VERSION/COMMIT), `clean`. `build-gui`/`build-all`/`install` are added by later
  specs; do not stub them.
- R1.4 `make lint` and `make test` pass with no display server and no game
  install present (integration tests skip, never fail, when their env vars are
  unset).
- R1.5 `README.md` keeps angou's style (lowercase H1, `**Version**:` line) and for
  this spec documents: what the tool does, prerequisites (.NET runtime, Steam
  install), `make build`, and the CLI commands below. Screenshots and the full
  changelog come with spec 004.

### R2 — Configuration and directories

- R2.1 Config file `$XDG_CONFIG_HOME/nmsbonker/config.json` (default
  `~/.config/nmsbonker/config.json`), overridable by `NMSBONKER_CONFIG`. Missing
  file = defaults; unknown keys are preserved on save (`json.RawMessage` or
  map-merge) so newer builds' keys survive older builds.
- R2.2 Fields (with defaults):
  - `game_dir` (`""` = auto-detect via R3); env `NMSBONKER_GAME_DIR` outranks it
    and the CLI `status` says so when it does.
  - `library_dir` (`$XDG_DATA_HOME/nmsbonker/library`) — user's `.lua` scripts.
  - `tools_dir` (`$XDG_DATA_HOME/nmsbonker/tools`).
  - `cache_dir` (`$XDG_CACHE_HOME/nmsbonker`).
  - `workspace_dir` (`$XDG_DATA_HOME/nmsbonker/build`).
  - `mod_name` (`"COSMOS COMBINE"`), the output folder under `GAMEDATA/MODS`.
  - `mbincompiler.pin` (`""` = automatic selection), `mbincompiler.flavor`
    (`"auto"` | `"dotnet10"` | `"self-contained"`).
  - `mods` (`[]`, ordered `{name, enabled}` — consumed by spec 002).
  - `parallel` (0 = `max(1, NumCPU/2)`), consumed by spec 002.
- R2.3 A leading `~` in any path field is expanded on load. A path that still
  contains a literal `~` component after expansion may be opened but is never
  created (angou rule).
- R2.4 `config.Load() (*Config, error)`, `Save`, `Paths()` returning resolved
  absolute directories; directories are created lazily by the operation that
  needs them, not by `Load`.

### R3 — Steam / game detection (`internal/steam`)

- R3.1 Candidate Steam roots, in order: `$STEAM_ROOT` if set,
  `~/.local/share/Steam`, `~/.steam/steam`, `~/.steam/root`,
  `~/.var/app/com.valvesoftware.Steam/.local/share/Steam`,
  `~/snap/steam/common/.local/share/Steam`. Parse
  `steamapps/libraryfolders.vdf` (VDF key/value; a tolerant line-based parser
  for `"path"` and nested `"apps"` blocks is sufficient — no full VDF library).
- R3.2 For each library, `steamapps/appmanifest_275850.acf` yields `installdir`,
  `buildid`, `name`, `LastUpdated`, `StateFlags`. Game dir =
  `<library>/steamapps/common/<installdir>`; it is valid only if
  `GAMEDATA/PCBANKS` exists. The first valid hit wins; every candidate examined
  is retained for diagnostics.
- R3.3 `steam.Locate(overrideDir string) (*Install, error)`; an explicit
  `overrideDir` skips discovery but is validated the same way, and a helpful
  error names what is missing (`GAMEDATA/PCBANKS`).
- R3.4 `Install` exposes: `Dir`, `LibraryDir`, `BuildID`, `Name`, `PCBanksDir`,
  `ModsDir` (`GAMEDATA/MODS`), `ModsState` (`absent` | `dir` | `symlink` with
  target), `ModSettingsPath` (`Binaries/SETTINGS/GCMODSETTINGS.MXML`) and whether
  it exists, `CompatDataDir` (`<library>/steamapps/compatdata/275850`, may be
  absent), `PakFiles() ([]string, error)` (sorted `.pak` paths in PCBANKS).
- R3.5 Reading `GCMODSETTINGS.MXML` is limited in this spec to
  `DisableAllMods` (bool) and the list of `{Name, Enabled, ModPriority}`
  entries, via a tolerant line/regex reader (writing it is spec 004).

### R4 — Native HGPAK reader (`internal/hgpak`)

- R4.1 `hgpak.Open(path) (*File, error)`: parses the header and index exactly as
  described in Context; rejects a wrong magic or `version != 2` with a typed
  error (`ErrNotHGPAK`, `ErrUnsupportedVersion`). Only the manifest chunks are
  decompressed at open; the rest is lazy.
- R4.2 `(*File).Names() []string` (manifest order), `Contains(name) bool`,
  `Stat(name) (Entry, bool)` (`Offset`, `Size`, `Hash [16]byte`),
  `ReadFile(name) ([]byte, error)`, `Open(name) (io.ReadCloser, error)`
  (streaming across chunk boundaries), `Close()`. Names are matched
  case-insensitively with backslashes normalised to `/`; the md5 helper
  `hgpak.HashPath(name) [16]byte` reproduces the game's hash (md5 of the
  lower-cased forward-slash path) and a test asserts it equals the index entry.
- R4.3 Decompressed chunks are cached in a small LRU (default 64 chunks ≈ 4 MiB)
  per open file; `ReadFile` on a 100-file table pak must not decompress the
  whole pak.
- R4.4 Uncompressed paks (`is_compressed == 0`) are supported (offsets absolute,
  no chunk table).
- R4.5 `hgpak.Index`: built over a PCBANKS directory, maps normalised internal
  path → pak path, plus a basename index used only as a fallback (the legacy
  builder resolved by basename when the full path missed; scripts sometimes
  write `GLOBALS\X.MBIN` for a root-level global). Built by opening each pak and
  reading its manifest only. Persisted to `cache_dir/pak-index.json` keyed by
  each pak's `(size, mtime)`; a changed pak re-indexes only that pak. Target:
  cold build of the index over the real 97 paks in under 5 s, warm load under
  200 ms (report the measured numbers in the validation notes).
- R4.6 Concurrency: `*File` is safe for concurrent `ReadFile` calls (mutex around
  the reader + cache, or per-call `ReaderAt`).

### R5 — MBINCompiler manager (`internal/mbin`)

- R5.1 Release discovery: `GET https://api.github.com/repos/monkeyman192/MBINCompiler/releases?per_page=30`
  with `User-Agent: nmsbonker/<version>`, optional `Authorization: Bearer $GITHUB_TOKEN`
  (never logged, never saved), 20 s timeout, ETag-cached JSON at
  `cache_dir/mbincompiler-releases.json` (used as-is when offline, with a
  warning). Parse `tag_name` into `(major, minor, patch, pre string)`; tags
  without a parseable `vM.m.p` prefix are ignored.
- R5.2 Selection: if `mbincompiler.pin` is set, that tag; else the highest
  version whose `major.minor` equals the game data version (R6) when known; else
  the highest version overall. The choice and the reason are reported.
- R5.3 Install: download the flavor's binary and its `libMBIN-*.so` (both assets
  are required; the loader looks for the `.so` beside the binary) into
  `tools_dir/mbincompiler/<tag>/`, `chmod 0755`, then verify by running
  `version` and checking the output contains the tag's numeric version. A failed
  verification removes the directory and returns the captured output. Flavor
  `auto`: `dotnet10` if `dotnet --list-runtimes` lists `Microsoft.NETCore.App 10.`,
  else `self-contained`; if the chosen flavor fails to run, try the other once
  and say so.
- R5.4 `mbin.Compiler` API: `Version(ctx) (string, error)`;
  `FileVersion(ctx, path) (string, error)` (the `version <file>` form);
  `Decompile(ctx, mbinPath, outDir) (mxmlPath string, err error)`;
  `Compile(ctx, mxmlPath, outDir) (mbinPath string, err error)`. Both conversions
  run `<bin> -y -q -d <outDir> <path>` with `WINEDEBUG`/`CLAUDECODE`-style
  environment leakage stripped (pass a minimal env: `PATH`, `HOME`,
  `DOTNET_*`, `LANG`, `TMPDIR`), honour `ctx` cancellation/timeout (kill the
  process), and on non-zero exit return an error carrying stderr plus the tail
  (last 40 lines) of `<bin>.log` when present. Output detection: the expected
  output file (`<base>.MXML` / `<base>.MBIN`) must exist in `outDir` after a
  successful exit; anything else is an error (`ErrNoOutput`).
- R5.5 A process-wide semaphore limits concurrent MBINCompiler processes to
  `config.parallel`; `Compile`/`Decompile` block on it.
- R5.6 `mbin.Locate(toolsDir, pin) (*Compiler, error)` finds an installed
  compiler without network: the pinned tag, else the highest installed tag.

### R6 — Game data version

- R6.1 `core.GameDataVersion`: extract `gcgameplayglobals.global.mbin` from
  `NMSARC.globals.pak` via R4 into a temp file and run `FileVersion` on it. Parse
  the first `M.m.p` triple in the output; report it as the game data version.
  If no compiler is installed yet, the version is `unknown` and selection (R5.2)
  falls back to the highest release.
- R6.2 Compatibility status: `match` (same major.minor), `compiler-older`
  (advise updating; building is allowed but flagged), `compiler-newer`
  (informational), `unknown`.

### R7 — Core operations (`internal/core`)

- R7.1 Every CLI command below is a function `core.X(ctx, XRequest) (XResult, error)`
  with plain-data request/result structs; the CLI only formats results. Provide
  `core.Events{Log func(level Level, msg string); Progress func(p Progress)}`
  with a nil-safe no-op default; `Progress{Step, Total int; What string}`.
- R7.2 Operations: `Status` (config paths, install summary incl. `ModsState` and
  `DisableAllMods`, installed compiler + game data version + compatibility,
  library dir mod count, pak index freshness), `Detect` (all candidates examined
  and why they were rejected), `EnsureTools` (R5.1–R5.3, returns what was
  already present vs installed), `ListTools`, `PakList{Pak, Glob}`,
  `PakFind{Glob}` (across the index), `PakExtract{Name, OutDir}`,
  `ConfigShow`, `ConfigSet{Key, Value}` (dotted keys, typed validation).

### R8 — CLI (`cmd/nmsbonker`, `internal/cli`)

- R8.1 Commands: `status [--json]`, `detect [--json]`, `tools ensure|list|pin <tag>|unpin`,
  `pak list [--pak NAME] [GLOB]`, `pak find GLOB [--json]`,
  `pak extract INTERNAL_PATH [-o DIR]`, `config show|set KEY VALUE`, `version`.
  Global flags: `--config PATH`, `--game-dir PATH`, `-v/--verbose` (debug log to
  stderr), `--no-network`.
- R8.2 Human output is plain text, one fact per line or a fixed-width table; no
  ANSI colour. Errors go to stderr with exit code 1; usage errors exit 2.
- R8.3 `nmsbonker` with no arguments prints help and exits 0.

### R9 — Tests

- R9.1 Unit, headless, no network: VDF/ACF parsing from small fixture strings
  (incl. Windows-style escaped paths `\\`); config load/save round-trip with
  unknown-key preservation and `~` expansion; HGPAK round-trip against a
  **synthetic pak built in the test** (a tiny writer that produces both a
  compressed and an uncompressed v2 pak with 3–4 files spanning chunk
  boundaries — the writer may live in `internal/hgpak/hgpaktest`); md5 path hash
  vector; release selection over a fixture JSON served by `httptest`; version
  parsing (`v7.02.0-pre1`, `v6.45.0`, garbage); MBINCompiler runner against a
  fake executable script that mimics `version`/convert behaviour.
- R9.2 Integration (skip unless `NMSBONKER_GAME_DIR` is set): open every pak in
  PCBANKS and read the manifest; extract `gcgameplayglobals.global.mbin` and
  assert the MBIN magic (`CC CC CC CC CC CC CC CC` or `DD…`) and, when
  `NMSBONKER_LEGACY_DIR` is set, byte-equality with
  `<legacy>/extract/GLOBALS/gcgameplayglobals.global.MBIN` (an hgpaktool
  extraction); with a compiler installed, `Decompile` it and assert the MXML
  begins with `<?xml` and contains `template="GcGameplayGlobals"`.
- R9.3 Test names are sentences describing the invariant; `require`, not
  `assert`; every test states the bug it prevents in a comment (angou idiom).

## Acceptance Criteria

- [ ] AC1 `make lint`, `make test`, `make build` pass from a clean checkout with
  `NMSBONKER_*` unset; `./nmsbonker version` prints the VERSION file value and
  the commit.
- [ ] AC2 `./nmsbonker status` on this machine reports the game dir under
  `~/.local/share/Steam`, buildid `25233815`, `ModsState=symlink` (current
  legacy setup), `DisableAllMods=false`, and the compiler state.
- [ ] AC3 `./nmsbonker tools ensure` downloads and verifies a MBINCompiler
  release into the tools dir on first run and is a no-op (says "already
  installed") on the second; `--no-network` with a populated tools dir succeeds.
- [ ] AC4 `./nmsbonker pak find '*rewardtable*'` lists
  `metadata/reality/tables/rewardtable.mbin` in `NMSARC.Precache.pak`;
  `./nmsbonker pak extract metadata/reality/tables/rewardtable.mbin -o /tmp/x`
  writes a file whose first 8 bytes are the MBIN magic, and `Decompile` of it
  succeeds.
- [ ] AC5 `./nmsbonker pak extract gcgameplayglobals.global.mbin` is
  byte-identical to the legacy hgpaktool extraction in
  `~/Games/nms-modding/extract/GLOBALS/`.
- [ ] AC6 Cold pak index over the real PCBANKS completes in < 5 s; warm load
  < 200 ms; numbers recorded in the spec's Status notes.
- [ ] AC7 Integration tests (R9.2) pass with `NMSBONKER_GAME_DIR` and
  `NMSBONKER_LEGACY_DIR=~/Games/nms-modding` set, and are skipped (not failed)
  without them.
- [ ] AC8 No file under `internal/`, `cmd/`, or `testdata/` contains game data,
  a Nexus script, or a personal absolute path (grep for `/home/`, `nverenin`,
  `Data2`).
- [ ] AC9 `go vet` and `golangci-lint` report nothing; `govulncheck ./...` clean
  or findings documented.

## Risks & Assumptions

- HGPAK raw-chunk fallback (zstd failure + exact 0x10000 length) is copied from
  the reference implementation; real PC paks observed so far are all compressed.
- MBINCompiler's `version <file>` output format is not documented; parsing is
  lenient (first `M.m.p`) and failure degrades to `unknown`, never blocks.
- GitHub unauthenticated rate limit (60/h) is ample for a release listing;
  ETag caching keeps repeat calls free.
- The framework-dependent `-dotnet10` binary needs `Microsoft.NETCore.App 10.x`;
  the "self-contained" `MBINCompiler-linux` asset's actual runtime needs are
  unverified — the runner treats "fails to execute" as a normal, reported outcome.
- Rollback: `git revert`; nothing here writes to the game directory.

## Alternatives Considered

- Shelling out to `hgpaktool` (pip) instead of a native reader: rejected — adds a
  Python dependency to a Go desktop app and the format is ~300 lines.
- Parsing MBIN in Go: rejected — libMBIN's struct definitions change every game
  release; MBINCompiler is the maintained converter.
- Fyne `Preferences` for config: rejected — the CLI has no Fyne app; a JSON file
  keeps CLI and GUI on one source of truth (angou does the same).
