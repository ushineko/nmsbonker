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

**Version**: 0.1.0 (phase 1 of 4 — foundations only; the mod pipeline lands in
spec 002)

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

The mod pipeline itself — reading `.lua` scripts, editing MXML, recompiling and
deploying — is spec 002 and is not in this build.

## Requirements

- **Linux.** macOS is a secondary target and untested here.
- **No Man's Sky, installed through Steam.** Steam itself does not need to be
  running; only its on-disk manifests are read.
- **A .NET runtime**, for MBINCompiler. `nmsbonker tools ensure` prefers the
  framework-dependent build when `dotnet --list-runtimes` reports
  `Microsoft.NETCore.App 10.x`, and falls back to the self-contained build
  otherwise.
- **Go 1.25 or newer**, to build it. The CLI builds with `CGO_ENABLED=0`.

## Building

```
make build          # ./nmsbonker, CGO-free, version and commit baked in
make test           # go test -race ./...
make lint           # golangci-lint, pinned to v2.12.2
make help           # every target
```

## Using it

```
nmsbonker status                    # game, tools, caches, compatibility
nmsbonker detect                    # where the game was looked for, and why not
nmsbonker tools ensure              # install the MBINCompiler this game needs
nmsbonker tools list                # what is installed
nmsbonker tools pin v7.02.0-pre1    # always use this release
nmsbonker tools unpin
nmsbonker pak list                  # the archives, with file counts
nmsbonker pak list --pak globals    # what is inside one of them
nmsbonker pak find '*rewardtable*'  # across every archive
nmsbonker pak extract metadata/reality/tables/rewardtable.mbin -o /tmp/x
nmsbonker config show
nmsbonker config set mod_name "COSMOS COMBINE"
nmsbonker version
```

Global flags: `--config PATH`, `--game-dir PATH`, `-v/--verbose`,
`--no-network`. `status`, `detect` and `pak find` also take `--json`.

`pak extract` keeps the file's internal path under the output directory, so an
extraction tree can be compared with the archive that produced it.

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
| `cache_dir` | `$XDG_CACHE_HOME/nmsbonker` | Pak index and release listing |
| `workspace_dir` | `$XDG_DATA_HOME/nmsbonker/build` | Build output |
| `mod_name` | `COSMOS COMBINE` | The folder created under `GAMEDATA/MODS` |
| `mbincompiler.pin` | *(empty)* | A release tag to always use |
| `mbincompiler.flavor` | `auto` | `auto`, `dotnet10` or `self-contained` |
| `parallel` | `0` | Concurrent MBINCompiler processes; 0 means half the CPUs |

The game directory is read-only to this tool. Only spec 004's deploy will write
under `GAMEDATA/MODS`, and it will archive what it replaces first.

## Testing

`make test` runs everything that needs no game and no network. The tests that
read a real install skip unless you point them at one:

```
NMSBONKER_GAME_DIR="$HOME/.local/share/Steam/steamapps/common/No Man's Sky" \
NMSBONKER_LEGACY_DIR="$HOME/Games/nms-modding" \
  go test -race ./...
```

`NMSBONKER_LEGACY_DIR` enables the byte-for-byte comparison against an
`hgpaktool` extraction in the legacy pipeline's build cache.

## Project layout

| Path | What is in it |
| --- | --- |
| `cmd/nmsbonker` | The CLI entry point |
| `internal/cli` | The cobra command tree; renders, decides nothing |
| `internal/core` | Every operation, headless, as request/result structs |
| `internal/config` | Settings and directory resolution |
| `internal/steam` | Steam manifest parsing and game detection |
| `internal/hgpak` | The native HGPAK v2 reader and the pak index |
| `internal/mbin` | MBINCompiler acquisition and the process runner |
| `internal/buildinfo` | Version and commit, injected at build time |

## Credit

The HGPAK v2 read path is a port of
[HGPAKtool](https://github.com/monkeyman192/HGPAKtool) (MIT).
[MBINCompiler](https://github.com/monkeyman192/MBINCompiler) is downloaded and
run, not vendored; it remains the only maintained MBIN↔MXML converter.

## License

MIT. See [LICENSE](LICENSE).
