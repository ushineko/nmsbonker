# nmsbonker

**For No Man's Sky on Linux (Steam + Proton).** The community mod builder,
AMUMSS, is Windows batch tooling and does not run on Linux, under Wine or
otherwise; the usual advice is to keep a Windows VM around just to rebuild
mods after each game update. nmsbonker exists so you do not have to: it does
the same job natively against the Steam/Proton install on the machine you
play on.

Rebuilds AMUMSS-format `.lua` mod scripts against the No Man's Sky files you
actually have installed, merges every enabled mod into one collision-free mod
folder, and deploys it. Native Go: no Wine, no Windows VM, no Python, no
`hgpaktool.exe`.

*Nothing from the game lives in this repository. It reads your install at run
time and writes its output under your XDG directories.*

**Version**: 0.1.0 (phases 1 to 3 of 4 — the mod pipeline builds and deploys and
the desktop window is up; tweaks, rollback and packaging are spec 004)

> The specs are the design of record, including the alternatives that were
> rejected and why:
> [`specs/001`](specs/001-foundations-steam-hgpak-mbincompiler.md) for the
> scaffold, Steam detection, the pak reader and MBINCompiler;
> [`specs/002`](specs/002-mod-pipeline-engine-build-report.md) for the mod
> pipeline; [`specs/003`](specs/003-fyne-gui.md) for the GUI;
> [`specs/004`](specs/004-tweaks-deploy-rollback-migration-packaging.md) for
> tweaks, deploy and packaging.

## What it does today

**Finds your game.** Reads Steam's own `libraryfolders.vdf` and
`appmanifest_275850.acf` across every library — including a Flatpak or Snap
Steam, and a second drive — and validates the result by the presence of
`GAMEDATA/PCBANKS`. `nmsbonker detect` shows every place it looked and why each
was rejected.

**Reads the game's `.pak` archives natively.** HGPAK version 2, the
zstd-chunked format PC builds ship, read in Go. This replaces running AMUMSS's
`hgpaktool.exe` under Wine; the bytes it produces are identical to hgpaktool's.

**Indexes all 97 archives.** One index over ~195,000 internal paths, cached and
keyed per archive by size and modification time, so a game update re-reads only
the archives it actually changed.

**Acquires MBINCompiler.** Picks the release that matches your game, downloads
the Linux binary and its `libMBIN` together, and verifies the pair by running
it before calling it installed.

**Proves the compiler matches your install.** `nmsbonker tools check`
decompiles two known game files, recompiles them, and compares the bytes
outside the header. That answers the question a version string cannot: the
game's own MBINs carry no libMBIN version, so whether this compiler can read
and rewrite your install's data has to be measured rather than looked up.

**Runs your `.lua` mod scripts.** An embedded Lua interpreter, sandboxed to
`base`, `table`, `string` and `math` — no `os`, no `io`, no way to load more
code — so a script downloaded from Nexus is data, not a program.

**Merges every enabled mod into one mod folder.** Two mods that edit the same
game file cannot each ship their own copy; the game loads one and the other's
edits vanish. So the unit of work is the game file, and every mod's edits to it
are applied in your configured order into one merged document.

**Never ships a file the compiler rejected.** Each merged file is gated on a
clean recompile. If it fails and some of the edits added or removed whole
entries, it is retried with only the value edits and ships "degraded", with the
report naming what was left out. If that fails too, the file is dropped and
every contributing mod is told.

**Tells you what happened.** `BUILD_REPORT.md` and `report.json` per build: a
verdict per mod, the keys a game update renamed out from under it, the files
that shipped degraded, and what to do about each.

**Has a window, if you want one.** `nmsbonker-gui` is the same operations with
the facts ranked and the build watchable: the install and the compiler with a
verdict beside each, the mod library as a reorderable table with each mod's
author and last verdict, and a Build section that streams the build's output
live with a step list and a Cancel that puts the previous mod folder back. It
reads and writes the same `config.json` the command line does, so the two never
disagree about where your game is.

## Requirements

- **Linux.** macOS is a secondary target and untested here.
- **No Man's Sky, installed through Steam.** Steam itself does not need to be
  running; only its on-disk manifests are read.
- **A .NET runtime**, for MBINCompiler. `nmsbonker tools ensure` prefers the
  framework-dependent build when `dotnet --list-runtimes` reports
  `Microsoft.NETCore.App 10.x`, and falls back to the self-contained build
  otherwise.
- **Go 1.25 or newer**, to build it. The CLI builds with `CGO_ENABLED=0`.
- **For the window only**: CGO, OpenGL, and X11 or Wayland development headers,
  which is what Fyne needs. The command line needs none of them and never links
  them — everything the tool does is reachable without a display.

## Building

```
make build          # ./nmsbonker, CGO-free, version and commit baked in
make build-gui      # ./nmsbonker-gui, needs CGO and the toolkit's headers
make build-all      # static CLI for linux/darwin x amd64/arm64, plus this host's GUI
make test           # go test -race -tags parity ./...
make lint           # golangci-lint, pinned to v2.12.2
make help           # every target
```

The window is built for the host only. It needs CGO, so cross-compiling it would
mean a C toolchain per target, and nothing about the tool's job depends on it.

## Using it

```
nmsbonker status                    # game, tools, caches, compatibility
nmsbonker detect                    # where the game was looked for, and why not
nmsbonker tools ensure              # install the MBINCompiler this game needs
nmsbonker tools list                # what is installed
nmsbonker tools pin v7.02.0-pre1    # always use this release
nmsbonker tools unpin
nmsbonker tools releases            # what GitHub offers, and what would be installed
nmsbonker tools remove v7.01.0-pre1 # delete an installed release that is not in use
nmsbonker pak list                  # the archives, with file counts
nmsbonker pak list --pak globals    # what is inside one of them
nmsbonker pak find '*rewardtable*'  # across every archive
nmsbonker pak extract metadata/reality/tables/rewardtable.mbin -o /tmp/x
nmsbonker pak reindex               # read every archive again, ignoring the cache
nmsbonker tools check               # does this compiler match this install?

nmsbonker cache show                # what the pristine cache and the index hold
nmsbonker cache clear               # throw the decompiled game files away

nmsbonker mods import ~/Downloads/nms-mods   # copy a folder of .lua in
nmsbonker mods add SomeMod.lua               # or one at a time
nmsbonker mods list                          # the build order
nmsbonker mods move SomeMod 1                # later wins on a conflict
nmsbonker mods disable SomeMod
nmsbonker mods check                         # load them all, report what they edit

nmsbonker build                      # merge, recompile, write the report
nmsbonker build --recache            # re-extract the game files first
nmsbonker build --deploy             # and install it when it succeeds
nmsbonker deploy                     # install the last build
nmsbonker report                     # the last build's verdicts

nmsbonker config show
nmsbonker config set mod_name "COSMOS COMBINE"
nmsbonker version
```

And the window:

```
nmsbonker-gui                                # opens on Overview
nmsbonker-gui --config /path/to/config.json  # the same flag the CLI takes
nmsbonker-gui --section build                # open straight on a section
nmsbonker-gui --scheme "Breeze Light"        # for this run only; not saved
nmsbonker-gui --version
```

Everything the window does is a `nmsbonker` command underneath, and a test fails
the build if either front end grows an operation the other lacks. The colour
scheme, the font and the text size are the only things it keeps to itself, in
Fyne's own preference store; every other setting is in `config.json`.

Global flags: `--config PATH`, `--game-dir PATH`, `-v/--verbose`,
`--no-network`. `status`, `detect`, `pak find`, `tools check`, `mods list`,
`mods check` and `report` also take `--json`.

`build` exits 1 if any game file had to be dropped, and 0 otherwise — a mod
that applied nothing is a warning in the report, not a build failure.

`deploy` refuses a symlinked `GAMEDATA/MODS`: the game reads mods through the
link, so installing "into the game" would silently write somewhere else.
`--replace-symlink` removes the link, never its target. Whatever was installed
before is archived under `$XDG_DATA_HOME/nmsbonker/archive/` first.

`pak extract` keeps the file's internal path under the output directory, so an
extraction tree can be compared with the archive that produced it.

`cache clear` deletes only derived data — the game files a build has already
extracted and decompiled. The next build rebuilds them, at the cost of the few
seconds the cache normally saves. Your mod library, the build output and the
game itself are not touched.

## Configuration

Settings live in `$XDG_CONFIG_HOME/nmsbonker/config.json`
(`~/.config/nmsbonker/config.json` by default), overridable with
`$NMSBONKER_CONFIG` or `--config`. A missing file means defaults; keys written
by a newer build are preserved when an older one saves.

| Key | Default | What it is |
| --- | --- | --- |
| `game_dir` | *(empty)* | The install; empty means detect it. `$NMSBONKER_GAME_DIR` and `--game-dir` outrank it, and `status` says when they do |
| `library_dir` | `$XDG_DATA_HOME/nmsbonker/library` | Your `.lua` mod scripts |
| `tools_dir` | `$XDG_DATA_HOME/nmsbonker/tools` | Where MBINCompiler is installed |
| `cache_dir` | `$XDG_CACHE_HOME/nmsbonker` | Pak index, release listing and the decompiled game files a build starts from |
| `workspace_dir` | `$XDG_DATA_HOME/nmsbonker/build` | Build output |
| `mod_name` | `COSMOS COMBINE` | The folder created under `GAMEDATA/MODS` |
| `mbincompiler.pin` | *(empty)* | A release tag to always use |
| `mbincompiler.flavor` | `auto` | `auto`, `dotnet10` or `self-contained` |
| `mods` | `[]` | The build order: `{"name", "enabled"}` per script, managed by `nmsbonker mods` |
| `parallel` | `0` | Concurrent MBINCompiler processes; 0 means half the CPUs |

The game directory is read-only to this tool except during `deploy`, which
writes one folder under `GAMEDATA/MODS` and archives whatever it replaced
first. Nothing else — including `GCMODSETTINGS.MXML`, which is read and
reported but never written.

## Testing

`make test` runs everything that needs no game and no network. The tests that
read a real install skip unless you point them at one:

```
NMSBONKER_GAME_DIR="$HOME/.local/share/Steam/steamapps/common/No Man's Sky" \
NMSBONKER_REFERENCE_DIR="/path/to/the/reference/pipeline" \
  go test -race ./...
```

`NMSBONKER_REFERENCE_DIR` points at the reference Python/Lua pipeline this tool
is a rewrite of, and enables the byte-for-byte comparison against an `hgpaktool`
extraction in that pipeline's build cache. Adding `NMSBONKER_GOLDEN_DIR` enables
the golden parity suite, which checks the edit engine against fixtures captured
from the reference Python builder: every script decoded the same way, every
merged file byte-identical, every report line identical, and the same per-mod
verdicts end to end. The fixtures are game-derived and are generated locally by
`tools/reference/make_golden.py`; they are never committed.

## Project layout

| Path | What is in it |
| --- | --- |
| `cmd/nmsbonker` | The CLI entry point |
| `cmd/nmsbonker-gui` | The desktop entry point; four flags and nothing else |
| `internal/cli` | The cobra command tree; renders, decides nothing |
| `internal/gui` | The Fyne window; renders, decides nothing |
| `internal/core` | Every operation, headless, as request/result structs |
| `internal/config` | Settings and directory resolution |
| `internal/steam` | Steam manifest parsing and game detection |
| `internal/hgpak` | The native HGPAK v2 reader and the pak index |
| `internal/mbin` | MBINCompiler acquisition, the process runner and the round-trip check |
| `internal/modscript` | The sandboxed Lua loader and the change-table model |
| `internal/mxml` | The line-based MXML edit engine |
| `internal/build` | The target plan, the merge and recompile gate, the report |
| `internal/buildinfo` | Version and commit, injected at build time |
| `tests/parity` | The guard that the CLI and the window expose the same operations |
| `packaging` | The application icon |

## Credit

The window's design system — the colour schemes, the font scanner, the flash
slot, the busy strip and the small shared widgets — is copied from
[angou](https://github.com/ushineko/angou) (MIT, same author). The copied files
say so at the top; they are kept in step by hand.

The HGPAK v2 read path is a port of
[HGPAKtool](https://github.com/monkeyman192/HGPAKtool) (MIT). The edit engine is
a port of the author's own Python builder, which was in turn written against
[AMUMSS](https://github.com/HolySpiritus/AMUMSS)'s script format; Lua runs
through [gopher-lua](https://github.com/yuin/gopher-lua) (MIT).
[MBINCompiler](https://github.com/monkeyman192/MBINCompiler) is downloaded and
run, not vendored; it remains the only maintained MBIN↔MXML converter.

## License

MIT. See [LICENSE](LICENSE).
