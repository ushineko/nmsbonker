# nmsbonker

**For No Man's Sky installed through Steam and played under Proton on Linux.**
The community's mod builder, [AMUMSS](https://github.com/HolySpiritus/AMUMSS),
is Windows batch tooling wrapped around Windows executables. It does not run on
Linux, and it does not run under Wine either — the usual advice is to keep a
Windows VM around purely to rebuild your mods after every game update. This is
that job, done natively on the machine you actually play on: no Wine, no VM, no
Python, no `hgpaktool.exe`.

Rebuilds AMUMSS-format `.lua` mod scripts against the game files you have
installed, merges every enabled mod into one collision-free mod folder, and
deploys it. It also ships ten mods of its own — the "tweaks" — with a slider
for every number they change, so a fresh install has something to build without
downloading anything first.

*Nothing from the game lives in this repository. It reads your install at run
time and writes its output under your XDG directories.*

**Version**: 0.1.0

![The Overview section. A Game card: the directory, "Found by config game_dir",
a green tick against Steam buildid 25233815, "97 .pak in …/GAMEDATA/PCBANKS", a
green tick against "GAMEDATA/MODS dir" and another against "DisableAllMods
false". A Tools card below it: MBINCompiler v7.02.0-pre1 (dotnet10) ticked,
"Reports MBINCompiler v7.02.0-pre1", and a green Compatibility line reading
"compatible — game files round-trip through this compiler unchanged (2 file(s)
round-tripped byte-identical outside the header)", with ".NET 10 runtime
present". Along the bottom, Build and a red "Build and deploy…" beside Refresh,
and under them Disable all mods, Back up saves and a red "Remove deployed
mod…". The status bar reads: game 25233815, compiler v7.02.0-pre1, mods 0/12,
output COSMOS COMBINE.](assets/screenshot-overview.png)

## How the game loads mods, and why that shapes this tool

No Man's Sky loads loose files from `GAMEDATA/MODS/<any folder>/`, and a loose
file wins over the same path inside the game's own `.pak` archives. So a mod is
a tree of `.MBIN` files that shadow the game's.

That has one consequence that decides everything else here: **two mods that
edit the same game file cannot each ship their own copy of it.** The game loads
one of them, and the other mod's edits vanish with no error and no warning. The
unit of work is therefore the *game file*, not the mod — every enabled mod's
edits to a file are applied, in your configured order, into one merged document,
and the result goes into **one** mod folder.

An AMUMSS `.lua` script does not contain a mod. It contains a description of
edits: which game files to open, which lines to find, and what to do to the
numbers on them. nmsbonker runs those scripts in a sandboxed Lua interpreter,
applies their edits to the decompiled game data, and recompiles. A file the
compiler will not accept is never shipped.

## What it does

**Finds your game.** Reads Steam's own `libraryfolders.vdf` and
`appmanifest_275850.acf` across every library — including a Flatpak or Snap
Steam, and a second drive — and validates the result by the presence of
`GAMEDATA/PCBANKS`. `nmsbonker detect` shows every place it looked and why each
was rejected.

**Reads the game's `.pak` archives natively.** HGPAK version 2, the
zstd-chunked format PC builds ship, read in Go. This replaces running AMUMSS's
`hgpaktool.exe` under Wine; the bytes it produces are identical to hgpaktool's.
One index over all 97 archives and ~195,000 internal paths, cached and keyed per
archive by size and modification time, so a game update re-reads only the
archives it changed.

**Acquires MBINCompiler and proves it matches your install.** It picks the
release that matches your game, downloads the Linux binary and its `libMBIN`
together, and verifies the pair by running it. `nmsbonker tools check` then
decompiles two known game files, recompiles them, and compares the bytes outside
the header — which answers the question a version string cannot, because the
game's own MBINs carry no libMBIN version.

**Runs your `.lua` mod scripts in a sandbox.** An embedded Lua interpreter
limited to `base`, `table`, `string` and `math` — no `os`, no `io`, no way to
load more code — so a script downloaded from Nexus is data, not a program.

**Merges every enabled mod into one mod folder,** in your order. Later wins on a
conflict, and the order is the only control you have over two mods that change
the same value, so it is the first column of the mod table and one command on
the terminal.

**Never ships a file the compiler rejected.** Each merged file is gated on a
clean recompile. If it fails and some of the edits added or removed whole
entries, it is retried with only the value edits and ships "degraded", with the
report naming what was left out. If that fails too, the file is dropped and
every contributing mod is told.

**Comes with ten mods of its own,** each with declared parameters: material
yield, chest and loot amounts, units and nanites, nanites again on top, mission
standing, inventory stack limits, scan payouts, asteroid yield, item value, and
words learned per interaction. They are ordinary AMUMSS scripts, MIT-licensed
with the rest of this repository, and every number in them is a slider.

![The Tweaks section. A Mining card holding two tweaks: "Material yield" with
its checkbox ticked and "#1" beside it, described as multiplying the substance
amounts a rock, plant, crystal or asteroid gives up when mined, with a "Mined
amount multiplier" slider set to 20 against a dim "default 10" and a Reset
button; and "Space mining" unticked at #6, with an "Asteroid resource
multiplier" of 20 and an "Asteroid yield chance" of 1.0. Below them a Loot card
begins with "Chest and loot materials", ticked at #2, and its "Reward amount
multiplier" of 10. A line above the buttons reads "The last build, 2026-09-11
21:28, used these values", and the buttons are "Apply and build", "Reset all to
defaults" and Refresh.](assets/screenshot-tweaks.png)

**Tells you what happened.** `BUILD_REPORT.md` and `report.json` per build: a
verdict per mod, the keys a game update renamed out from under it, the files
that shipped degraded, and what to do about each.

**Finishes the deploy.** Installing the folder is not enough on its own: the
game keeps its own list of mods in `GCMODSETTINGS.MXML`, and a mod with no entry
there, or with the entry switched off, is installed and inert. Deploy writes
that file — an entry created if there is none, `Enabled` and `EnabledVR` on, and
`DisableAllMods` turned back off with a warning, since the game sets it after a
crash. The file is edited line by line, so the byte-order mark, the CRLF
endings, the tabs and every property this build has never heard of survive
untouched.

**Is undoable.** Every deploy leaves an archive entry holding the mod folder it
displaced and the settings as they were, and the newest five are kept.
`rollback` swaps an archived folder back in — and archives what was installed,
so the rollback is itself reversible. `undeploy` takes the folder out.
`mods-off` leaves everything in place and tells the game to load none of it,
which is the cheapest thing to try when the game stops starting.

**Copies your saves first.** The first deploy of each run copies every `st_*`
save profile out of the Proton prefix into a timestamped folder, retention ten.
Copy only, one direction: nothing here ever writes into the prefix.

**Has a window, if you want one.** `nmsbonker-gui` is the same operations with
the facts ranked and the build watchable. It reads and writes the same
`config.json` the command line does, so the two never disagree, and a test fails
the build if either front end grows an operation the other lacks.

## Requirements

- **Linux.** macOS is a secondary target: the CLI cross-compiles for it and is
  untested there.
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

## Installing

```
git clone https://github.com/ushineko/nmsbonker
cd nmsbonker
./install.sh
```

That builds both binaries and places four files:

| Path | What |
| --- | --- |
| `~/.local/bin/nmsbonker` | the command line |
| `~/.local/bin/nmsbonker-gui` | the window |
| `~/.local/share/applications/io.ushineko.nmsbonker.desktop` | the launcher entry |
| `~/.local/share/icons/hicolor/scalable/apps/nmsbonker.svg` | its icon |

`--no-gui` installs the command line alone; `--dry-run` shows what would happen
and changes nothing. If the window will not build — it needs a C toolchain and
the OpenGL headers — the installer names the packages and installs the command
line anyway, which does everything the window does.

`./uninstall.sh` removes exactly those four files and prints where everything
you made still is. `--dry-run` works there too.

Or build without installing:

```
make build          # ./nmsbonker, CGO-free, version and commit baked in
make build-gui      # ./nmsbonker-gui, needs CGO and the toolkit's headers
make build-all      # static CLI for linux/darwin x amd64/arm64, plus this host's GUI
make release        # tar.gz per target in dist/, with SHA256SUMS
make test           # go test -race -tags parity ./...
make lint           # golangci-lint, pinned to v2.12.2
make help           # every target
```

The window is built for the host only. It needs CGO, so cross-compiling it would
mean a C toolchain per target, and nothing about the tool's job depends on it.

## First run

```
nmsbonker status                # is the game here, is there a compiler, what would build
nmsbonker tools ensure          # download the MBINCompiler this game version needs
nmsbonker tools check           # prove it can read this install's files

nmsbonker tweaks list           # the ten built-in mods and every number in them
nmsbonker tweaks enable MaterialYield10x ItemValueBoost
nmsbonker tweaks set MaterialYield10x MATERIAL_MULTIPLIER 20

nmsbonker mods import ~/Downloads/nms-mods    # or bring your own scripts
nmsbonker mods list                           # the build order

nmsbonker build                 # merge, recompile, write the report
nmsbonker report                # what it made of each mod
nmsbonker deploy                # install it into the game
```

Nothing is enabled on a fresh install, including the built-in tweaks: installing
a mod builder is not consent to change your game. Nothing touches the game until
`deploy`.

The window is the same sequence with the facts in front of you:

```
nmsbonker-gui                                # opens on Overview
nmsbonker-gui --config /path/to/config.json  # the same flag the CLI takes
nmsbonker-gui --section tweaks               # open straight on a section
nmsbonker-gui --scheme "Breeze Light"        # for this run only; not saved
nmsbonker-gui --version
```

![The Mods section. A table with columns #, On, Name, Source, Author, Files and
Last verdict, holding the ten built-in tweaks: MaterialYield10x ticked with 69
files and an orange WORKING~, ChestAndLootMaterials10x, MoneyAndNanites5x,
ItemValueBoost, LearnMoreWords and MissionStandingBuff ticked and green
WORKING, and BigStacks, ScanValue50x, SpaceMiningBoost and NaniteRewardBuff
switched off with no verdict. Every row's Source reads "builtin" and its Author
"nmsbonker". Above the table: Add…, Import folder…, Check, Open library folder.
Below it, "8 of 12 enabled · build order is table order; lower rows apply later
and win on conflicts", and the row actions Enable, Disable, Move up, Move down,
Details…, Open script and a red Remove….](assets/screenshot-mods.png)

## Using it

```
nmsbonker status                    # game, tools, caches, compatibility
nmsbonker detect                    # where the game was looked for, and why not
nmsbonker version

nmsbonker tools ensure              # install the MBINCompiler this game needs
nmsbonker tools list                # what is installed
nmsbonker tools pin v7.02.0-pre1    # always use this release
nmsbonker tools unpin
nmsbonker tools releases            # what GitHub offers, and what would be installed
nmsbonker tools remove v7.01.0-pre1 # delete an installed release that is not in use
nmsbonker tools check               # does this compiler match this install?

nmsbonker pak list                  # the archives, with file counts
nmsbonker pak list --pak globals    # what is inside one of them
nmsbonker pak find '*rewardtable*'  # across every archive
nmsbonker pak extract metadata/reality/tables/rewardtable.mbin -o /tmp/x
nmsbonker pak reindex               # read every archive again, ignoring the cache

nmsbonker cache show                # what the pristine cache and the index hold
nmsbonker cache clear               # throw the decompiled game files away

nmsbonker mods import ~/Downloads/nms-mods   # copy a folder of .lua in
nmsbonker mods add SomeMod.lua               # or one at a time
nmsbonker mods list                          # the build order
nmsbonker mods move SomeMod 1                # later wins on a conflict
nmsbonker mods enable SomeMod
nmsbonker mods disable SomeMod
nmsbonker mods remove SomeMod --delete
nmsbonker mods check                         # load them all, report what they edit

nmsbonker tweaks list                        # the built-ins and their parameters
nmsbonker tweaks enable MaterialYield10x
nmsbonker tweaks disable MaterialYield10x
nmsbonker tweaks set BigStacks SUBSTANCE 500000
nmsbonker tweaks reset BigStacks             # or `... BigStacks SUBSTANCE`

nmsbonker build                      # merge, recompile, write the report
nmsbonker build --recache            # re-extract the game files first
nmsbonker build --deploy             # and install it when it succeeds
nmsbonker report                     # the last build's verdicts

nmsbonker deploy                     # install the last build
nmsbonker undeploy                   # take it back out, keeping a copy
nmsbonker archive list               # what deploy displaced, newest first
nmsbonker rollback                   # put the newest archived deployment back
nmsbonker rollback 20260911-204500Z  # or a particular one
nmsbonker mods-off                   # stop the game loading any mod
nmsbonker mods-on                    # let it again

nmsbonker saves backup               # copy the save profiles out of the prefix
nmsbonker saves list

nmsbonker config show
nmsbonker config set mod_name "COSMOS COMBINE"
nmsbonker config set save_backup false
```

Global flags: `--config PATH`, `--game-dir PATH`, `-v/--verbose`,
`--no-network`. `status`, `detect`, `pak find`, `tools check`, `mods list`,
`mods check`, `tweaks list`, `archive list`, `saves list` and `report` also take
`--json`.

`build` exits 1 if any game file had to be dropped, and 0 otherwise — a mod that
applied nothing is a warning in the report, not a build failure.

`deploy` refuses a symlinked `GAMEDATA/MODS`: the game reads mods through the
link, so installing "into the game" would silently write somewhere else.
`--replace-symlink` removes the link, never its target.

`tweaks set` never edits the script. The value goes into `config.json` and is
substituted into an in-memory copy at build time, so upgrading nmsbonker brings
the new script with your numbers still in it, and `tweaks reset` has something
to reset *to*. The same command works on a library script's own top-level
constants, unbounded, since nothing declares a range for those.

`cache clear` deletes only derived data — the game files a build has already
extracted and decompiled. Your mod library, the build output and the game itself
are not touched.

![The Report section. A table of the last build's verdicts: ChestAndLootMaterials10x,
ExampleAsteroidYield, ExampleRicherChests, ItemValueBoost, LearnMoreWords,
MissionStandingBuff and MoneyAndNanites5x all green WORKING with their edit
counts and no skips, and MaterialYield10x in orange as WORKING~ with 106 edits,
32 skipped and the note "keys not found: AmountMin, AmountMax". Under the table
the verdict legend, then a "Last build" card beginning "Generated 2026-09-11
21:28 · just now" and "Output folder COSMOS COMBINE". Along the bottom: Open
report folder, Open output folder, a red Deploy… and Roll back…, the last of
them greyed out because nothing has been deployed
yet.](assets/screenshot-report.png)

## Where things live

| Path | What is in it |
| --- | --- |
| `$XDG_CONFIG_HOME/nmsbonker/config.json` | your settings, the build order, and every tweak parameter you have changed |
| `$XDG_DATA_HOME/nmsbonker/library/` | your `.lua` mod scripts |
| `$XDG_DATA_HOME/nmsbonker/tools/` | MBINCompiler, as downloaded |
| `$XDG_DATA_HOME/nmsbonker/build/` | the merged mod folder and `reports/latest/` |
| `$XDG_DATA_HOME/nmsbonker/archive/` | what each deploy displaced, newest five |
| `$XDG_DATA_HOME/nmsbonker/save-backup/` | copies of your save profiles, newest ten |
| `$XDG_CACHE_HOME/nmsbonker/` | the pak index, the release listing, the decompiled game files |
| `$XDG_CONFIG_HOME/fyne/io.ushineko.nmsbonker/` | the window's colour scheme, font and text size |
| `<game>/GAMEDATA/MODS/<mod_name>/` | the only place in the game this tool writes |
| `<game>/Binaries/SETTINGS/GCMODSETTINGS.MXML` | the game's own mod list, which deploy edits |

The defaults are `~/.config`, `~/.local/share` and `~/.cache`; every one of the
first four is a setting, and `nmsbonker config show` prints where they resolved
to. The archive and the save backups are deliberately *not* configurable: they
are the undo button, and a user who has pointed them at a directory they later
clean out has lost it.

## Configuration

Settings live in `$XDG_CONFIG_HOME/nmsbonker/config.json`, overridable with
`$NMSBONKER_CONFIG` or `--config`. A missing file means defaults; keys written by
a newer build are preserved when an older one saves.

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
| `params` | `{}` | Tweak parameters you have changed: `{"<mod>": {"<GLOBAL>": <number>}}`, managed by `nmsbonker tweaks` |
| `save_backup` | `true` | Copy the game's saves before the first deploy of each run |
| `parallel` | `0` | Concurrent MBINCompiler processes; 0 means half the CPUs |

## Limitations

- **Linux.** The CLI cross-compiles for macOS and nobody has run it there; the
  window is built for the host only.
- **Mods must be rebuilt after every game update,** and MBINCompiler must have a
  release that matches. That is not a limitation of this tool — it is how
  loose-file MBIN mods work — but it is the reason the tool exists, so it is
  worth saying plainly. `nmsbonker tools ensure && nmsbonker build --deploy` is
  the whole ritual.
- **The ADD and REMOVE edits are heuristic.** The engine finds the block to add
  to or remove by matching keywords and counting braces, the way the pipeline
  this is a rewrite of did. A file whose structure a game update has changed can
  produce an edit that recompiles and is not what the mod's author meant, which
  is why any mod carrying one is reported as WORKING\* — verify it in game.
- **Some AMUMSS script keys are ignored:** `FSKWG`, `LINE_OFFSET`,
  `SECTION_ACTIVE`, `VALUE_MATCH`, `VALUE_MATCH_OPTIONS`, `VALUE_MATCH_TYPE`.
  The build report names them and names the mods relying on them; the edits
  those keys asked for do not happen.
- **Not a save editor.** Save backup is a copy, in one direction, and restoring
  is a copy you do yourself.
- **Not a Nexus client.** Third-party scripts are yours to download; `mods
  import` brings a folder of them in.
- **A parameter override is one substitution.** It rewrites the *first*
  top-level assignment of that global. A script that assigns the same global
  twice would ignore the override, and `mods check` warns when one does.

## Troubleshooting

**The game starts and none of my mods are on.** The game sets
`DisableAllMods=true` in `GCMODSETTINGS.MXML` after a crash, which silently
switches every mod off. `nmsbonker status` reports the switch, `nmsbonker
mods-on` clears it, and a deploy clears it with a warning.

**The game will not start at all.** Try `nmsbonker mods-off` first: it changes
nothing that has to be rebuilt, and `nmsbonker mods-on` puts it back exactly as
it was. If that fixes it, the mod is the problem; `nmsbonker rollback` puts the
previous deployment back, and the build report says which mods changed what.

**`deploy` refuses because GAMEDATA/MODS is a symlink.** The game reads mods
through the link, so installing there would write into the link's target instead
of into the game. `deploy --replace-symlink` removes the link — the link only,
never what it points at — and creates a real directory. The window offers the
same thing as "Replace symlink and deploy…". `ln -sfn` puts the link back if you
want it.

**`tools check` says "mismatch".** The compiler and this install disagree about
a file format, so what a build produces with it is worth doubting. Usually the
game updated and MBINCompiler has not caught up yet: `nmsbonker tools releases`
shows what exists, `nmsbonker tools ensure` takes the newest match, and
`nmsbonker tools pin <tag>` fixes a choice that works.

**"MBINCompiler will not start."** The `dotnet10` flavor is
framework-dependent and needs a .NET 10 runtime on `PATH`; `nmsbonker status`
reports whether one is there. Either install it, or
`nmsbonker config set mbincompiler.flavor self-contained` and
`nmsbonker tools ensure` again for a build that carries its own.

**A mod says WORKING~ with "keys not found".** A game update renamed or removed
the fields it was editing. The rest of its edits applied; that one did not, and
the mod needs a new version from its author.

**A build takes minutes the first time.** It decompiles every game file the
enabled mods touch, one MBINCompiler process per file. The results are cached
per game buildid, so the second build is seconds — until the game updates, which
starts a new cache.

## Testing

`make test` runs everything that needs no game and no network. The tests that
read a real install skip unless you point them at one:

```
NMSBONKER_GAME_DIR="$HOME/.local/share/Steam/steamapps/common/No Man's Sky" \
NMSBONKER_REFERENCE_DIR="/path/to/the/reference/pipeline" \
  go test -race ./...
```

`NMSBONKER_REFERENCE_DIR` points at the reference Python/Lua pipeline this tool
is a rewrite of. It enables the byte-for-byte comparison against an `hgpaktool`
extraction in that pipeline's build cache, and the check that the ten embedded
tweak scripts still decode to exactly what their reference copies decode to.
Adding `NMSBONKER_GOLDEN_DIR` enables the golden parity suite, which checks the
edit engine against fixtures captured from the reference builder: every script
decoded the same way, every merged file byte-identical, every report line
identical, and the same per-mod verdicts end to end. The fixtures are
game-derived and are generated locally by `tools/reference/make_golden.py`; they
are never committed.

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
| `internal/modscript` | The sandboxed Lua loader, the change-table model and the parameter parser |
| `internal/modsettings` | The game's `GCMODSETTINGS.MXML`, read and written line by line |
| `internal/mxml` | The line-based MXML edit engine |
| `internal/build` | The target plan, the merge and recompile gate, the report |
| `internal/tweaks` | The ten built-in mod scripts, embedded |
| `internal/buildinfo` | Version and commit, injected at build time |
| `tests/parity` | The guard that the CLI and the window expose the same operations |
| `packaging` | The desktop entry and the application icon |
| `tools` | The screenshot harness and the reference fixture generator |

[`docs/architecture.md`](docs/architecture.md) is the package map and the data
flow in more detail, including how the golden fixtures are regenerated.

> The specs are the design of record, including the alternatives that were
> rejected and why:
> [`specs/001`](specs/001-foundations-steam-hgpak-mbincompiler.md) for the
> scaffold, Steam detection, the pak reader and MBINCompiler;
> [`specs/002`](specs/002-mod-pipeline-engine-build-report.md) for the mod
> pipeline; [`specs/003`](specs/003-fyne-gui.md) for the window;
> [`specs/004`](specs/004-tweaks-deploy-rollback-packaging.md) for the tweaks,
> the deploy story and the packaging.

## Changelog

### 0.1.0

The first release: the whole pipeline, both front ends, and the packaging.

- **Finds the game and reads its archives natively.** Steam library and
  manifest parsing, including Flatpak and Snap installs and a second drive; a
  native HGPAK v2 reader; one cached index over all 97 archives.
- **Acquires MBINCompiler and measures whether it fits.** Release selection,
  download and verification, plus a round-trip check that decompiles and
  recompiles two known game files rather than comparing version strings, since
  the game's own MBINs carry no version to compare.
- **Builds mods from AMUMSS `.lua` scripts** in an embedded, sandboxed Lua
  interpreter, merges every enabled mod's edits per game file in your order, and
  gates every output on a clean recompile. Parity with the Python pipeline this
  replaces is enforced by a golden suite: same decoded scripts, byte-identical
  merged documents, same report lines, same per-mod verdicts.
- **Reports what happened**, per mod and per file, in Markdown and JSON.
- **Ten built-in tweaks**, with a declared range and a slider for every number
  they change. Overrides live in the settings and are substituted into an
  in-memory copy of the script at build time, so the scripts themselves are
  never edited.
- **A finished deploy**: the mod folder, the game's own `GCMODSETTINGS.MXML`,
  and an archive of what was displaced. `rollback`, `undeploy`, `archive list`,
  and `mods-off` / `mods-on` as a non-destructive kill switch.
- **Save backup** before the first deploy of each run, copy-only, retention ten.
- **A desktop window** — nine sections over the same operations, with the build
  streamed live and cancellable — and a parity test that fails the build if
  either front end grows an operation the other lacks.
- **`install.sh`, `uninstall.sh`,** a desktop entry and an icon.

## Credit

The window's design system — the colour schemes, the font scanner, the flash
slot, the busy strip, the small shared widgets, the installer's shape and the
screenshot harness — is copied from
[angou](https://github.com/ushineko/angou) (MIT, same author). The copied files
say so at the top; they are kept in step by hand.

The HGPAK v2 read path is a port of
[HGPAKtool](https://github.com/monkeyman192/HGPAKtool) (MIT). The edit engine is
a port of the author's own Python builder, which was in turn written against
[AMUMSS](https://github.com/HolySpiritus/AMUMSS)'s script format; AMUMSS is the
reason that format exists and is what everyone's mod scripts are written for.
Lua runs through [gopher-lua](https://github.com/yuin/gopher-lua) (MIT).
[MBINCompiler](https://github.com/monkeyman192/MBINCompiler) is downloaded and
run, not vendored; it remains the only maintained MBIN↔MXML converter, and none
of this would exist without it.

## License

MIT. See [LICENSE](LICENSE).
