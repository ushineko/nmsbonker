# Architecture

How nmsbonker is put together, what each package is responsible for, and where
the correctness of the whole thing is actually established.

This is the map. The specs under [`specs/`](../specs/) are the design of record
and carry the alternatives that were rejected and why; the [README](../README.md)
is what the tool does. This is for someone about to change it.

---

## The one rule

**Every user-facing operation is a headless function in `internal/core` taking a
request struct and returning a result struct. The front ends render and decide
nothing.**

```
cmd/nmsbonker  ──►  internal/cli  ──┐
                                    ├──►  internal/core  ──►  everything else
cmd/nmsbonker-gui  ──►  internal/gui ┘
```

Neither front end reaches past `core` into `hgpak`, `steam`, `mbin` or `build`.
A new operation lands in `core` first and in both front ends in the same commit.
This is enforced rather than remembered: `tests/parity` walks the cobra command
tree and `gui.Actions()` and fails the build when either side holds an operation
the other does not, with a documented allow-list of three entries (`version`,
`completion`, `help`).

Two consequences that are easy to lose:

- **The CLI must not link the GUI.** `cmd/nmsbonker` builds with `CGO_ENABLED=0`
  and `go list -deps ./cmd/nmsbonker` names neither `internal/gui` nor Fyne. A
  machine with no display, no OpenGL and no C toolchain can build and run
  everything the tool does.
- **Nothing user-visible may live only in a front end.** A verdict, a piece of
  wording, a default: if both front ends need it, it belongs in `core` or in the
  package `core` got it from. What the GUI does own is presentation — which
  colour ranks a verdict, what a card is called — and that lives in
  `internal/gui/model.go`, deliberately apart.

---

## Package map

| Package | Responsibility |
| --- | --- |
| `cmd/nmsbonker` | CLI entry point. Parses nothing; hands off to `internal/cli` and maps error kinds to exit codes |
| `cmd/nmsbonker-gui` | Window entry point. Four flags: `--config`, `--section`, `--scheme`, `--version` |
| `internal/cli` | The cobra tree. Builds a `core.Request` from flags, calls one operation, prints the result |
| `internal/gui` | The Fyne window. Nine sections, every core call off the UI thread and back through `fyne.Do` |
| `internal/core` | Every operation, as request/result structs. The only package both front ends import |
| `internal/config` | The settings document, the XDG directory resolution, and the parameter overrides |
| `internal/steam` | `libraryfolders.vdf` and `appmanifest_275850.acf` parsing; game detection; the state of `GAMEDATA/MODS` |
| `internal/hgpak` | The native HGPAK v2 reader and the cached index over every archive |
| `internal/mbin` | MBINCompiler release selection, download, the process runner, and the round-trip compatibility check |
| `internal/modscript` | The sandboxed Lua loader, the decoded change-table model, and the `@param` parser |
| `internal/mxml` | The line-based MXML edit engine: the thing that actually changes values |
| `internal/build` | The target plan, the per-file merge, the recompile gate, and the report |
| `internal/build/cache` | The pristine cache: game files extracted and decompiled once per game buildid |
| `internal/build/report` | The result model and its two renderings, `BUILD_REPORT.md` and `report.json` |
| `internal/modsettings` | The game's `GCMODSETTINGS.MXML`, read and written line by line |
| `internal/tweaks` | The ten built-in mod scripts, embedded, with their headers parsed |
| `internal/buildinfo` | Version and commit, injected by the linker |
| `tests/parity` | The CLI/GUI parity guard, behind a build tag `make test` always passes |
| `tools/reference` | `make_golden.py`, which regenerates the golden fixtures from the reference builder |
| `tools/screenshot.sh` | The README capture harness |

---

## Data flow: one build

```
  config.json                     the game install
  ├─ mods[]  (order, enabled)     ├─ GAMEDATA/PCBANKS/*.pak
  └─ params{} (overrides)         └─ GAMEDATA/MODS/          ← written only by deploy
        │                                │
        ▼                                ▼
  core.loadScripts                  hgpak.BuildIndex ────────► cache/pak-index.json
        │                                │                     (keyed per pak by
        │  for each enabled mod:         │                      size + mtime)
        │   tweaks.Source() or the .lua  │
        │   modscript.Override(…)  ◄─────┼── params: textual substitution of the
        │   modscript.LoadSource()       │   first `NAME = <number>` line, in memory
        ▼                                │
  []modscript.Definition                 │
        │                                │
        ▼                                ▼
  build.NewPlan ──────────────────► build/cache.Ensure
   groups every mod's edits          extract each source .MBIN from its pak,
   by TARGET FILE, in order          decompile it with MBINCompiler, keep both
        │                            under cache/game/<buildid>/
        │                                │
        └──────────────┬─────────────────┘
                       ▼
                  build.Run  (one worker per target, up to `parallel`)
                       │
                       ├─ mxml.Apply     every mod's blocks, in build order,
                       │                 into one merged document
                       ├─ recompile      MBINCompiler MXML → MBIN
                       │   ├─ clean      → ship
                       │   ├─ failed     → retry without the ADD/REMOVE blocks
                       │   │                → clean: ship "degraded"
                       │   └─ failed     → drop the file, tell every mod
                       ▼
        workspace/<mod_name>/**.MBIN        workspace/reports/latest/
                       │                     ├─ BUILD_REPORT.md
                       │                     └─ report.json  (params included)
                       ▼
                  core.Deploy
                       ├─ copy the saves out of the Proton prefix (first deploy per run)
                       ├─ stage into GAMEDATA/MODS/<name>.tmp-<pid>, then rename
                       ├─ archive/<mod_name>-<UTC>/mod + GCMODSETTINGS.MXML
                       ├─ modsettings: entry Enabled=true, DisableAllMods=false
                       └─ prune the archive to the newest five
```

Three properties of that path are load-bearing and easy to break:

- **The game directory is read-only except during deploy.** Everything else
  writes under the workspace and the cache. `deploy`, `undeploy`, `rollback` and
  `mods-off` are the only operations that write inside the install, and each of
  them archives what it displaces first.
- **The merge is per target file, not per mod.** Two mods that edit the same file
  cannot each ship a copy of it; the game would load one and lose the other. So
  the plan is keyed by `cache.Key(source)` and every mod's blocks for that key
  are applied in build order into one document.
- **Nothing ships that the compiler rejected.** The recompile gate is the last
  word, and a file that fails twice is dropped and reported rather than shipped
  broken.

---

## Where correctness is established

Three layers, answering three different questions.

### Unit tests

The ordinary kind, in every package. The engine ones are worth singling out:
`internal/mxml` is at 94% and the tests there are the specification of what an
edit *means* — what `MATH_OPERATION` does to an integer, what `REPLACE_TYPE`
changes, how a `SECTION_UP` walk terminates.

### Golden parity with the reference builder

nmsbonker is a rewrite of a Python/Lua pipeline that worked. That pipeline is
the correctness oracle for the edit engine, and the golden suite is how the
claim "this produces the same mod" is checked rather than asserted.

Three stages, in `internal/build/golden_test.go`:

| Stage | Question | Compared against |
| --- | --- | --- |
| A | Does the embedded Lua interpreter decode the same change tables? | `golden/dumped/*.json`, from the reference `dump_mod.lua` |
| B | Does the edit engine produce the same merged documents? | `golden/merged/**`, byte for byte, and `golden/report_lines.txt`, line for line |
| C | Does the whole pipeline reach the same verdicts? | `golden/BUILD_REPORT.md`, per mod |

A fourth check lives in `internal/tweaks/golden_test.go`: the ten built-in
scripts, which were copied out of the reference pipeline and given `@tweak` and
`@param` header comments, must still decode to exactly the same `MODIFICATIONS`
as their reference copies. `MOD_AUTHOR` is compared separately because it was
changed deliberately.

**The fixtures are game-derived and are never committed.** They are generated
locally, and the suite skips when it cannot find them.

#### Regenerating the golden fixtures

`tools/reference/make_golden.py` is this project's own code — a harness around
the reference builder — and contains no game data. It imports that builder's
merge stage, runs it, and writes down what it produced. It needs the reference
pipeline checked out, with its own cache already populated from a game.

```
python3 tools/reference/make_golden.py "$NMSBONKER_REFERENCE_DIR"
```

The argument defaults to `$NMSBONKER_REFERENCE_DIR`, and the fixtures land in
`<that tree>/golden/`, which is what `$NMSBONKER_GOLDEN_DIR` should point at. It
writes `mods.conf` (the build order the fixtures were captured in), `dumped/`
(stage A), `merged/` and `report_lines.txt` (stage B), and a copy of the
reference `BUILD_REPORT.md` (stage C), plus an `index.json` summarising what was
captured and a copy of the reference cache's `manifest.json` saying which game
build it was captured against.

It runs the merge stage only — no MBINCompiler — so it is seconds rather than
minutes, and stage C's compile is nmsbonker's own.

Then:

```
NMSBONKER_GAME_DIR="…/steamapps/common/No Man's Sky" \
NMSBONKER_REFERENCE_DIR=… \
NMSBONKER_GOLDEN_DIR=… \
  go test -race -count=1 ./...
```

**The verdicts depend on the build order**, which is a property of the pipeline
rather than a defect: one of the reference scripts carries a `REMOVE` with no
`SPECIAL_KEY_WORDS` that empties a whole reward table, so a mod sorted before it
and a mod sorted after it get different answers. Stage C therefore sets the
order from `golden/mods.conf` explicitly rather than taking filename order.

If you change engine semantics on purpose, regenerate the fixtures and say so in
the spec. If you change them by accident, stage B tells you which of a hundred
merged files moved and by which byte.

### The parity guard

`tests/parity` is not about correctness of output; it is about the two front
ends not drifting. It carries a build tag so that a plain `go test ./...` does
not need it and `make test` always passes it — a guard that runs only when
someone remembers to ask for it is not a guard.

---

## Conventions worth knowing before changing anything

**Long work is cancellable and reports progress.** Every operation that can take
more than a moment takes a `context.Context` and talks back through
`core.Events{Log, Progress}`. Both fields may be nil. Cancelling a build kills
the MBINCompiler processes it started *and* puts the previous mod folder back —
`build.Run` moves the old output aside and restores it on the way out, which is
a bug that was found by pressing Cancel in the window and not by any test.

**The window never blocks its render thread.** Every core call in
`internal/gui` goes through `perform` or `startBuild`, which run on a goroutine
and hop back with `fyne.Do`. A raw `go func()` reaching into core is a window
that sits still with no explanation.

**Nothing transient may reflow the interface.** Result banners, the busy strip
and the build's step markers live in regions that keep their size whether or not
anything is in them. A card that grows a row when a slider moves takes the next
card out from under the mouse.

**A settings file this tool did not write is edited, not regenerated.**
`config.json` keeps keys it does not understand and writes them back.
`GCMODSETTINGS.MXML` is the game's, and is edited line by line so the byte-order
mark, the CRLF endings, the tabs and every unknown property survive; the tests
assert that an unedited document round-trips byte for byte and that a flipped
switch changes exactly one line.

**No game data, no third-party scripts, no personal paths in the repository.**
Tests that need real data read the user's install at run time and skip when the
environment variables are unset. The screenshot harness builds a fake
Steam-shaped install under `/tmp` whose `PCBANKS` is a symlink to the real one,
so the images show real numbers and no path belonging to anybody.

---

## Adding an operation

1. Write it in `internal/core` as `XxxRequest` / `XxxResult` / `func Xxx(ctx, req)`.
   Embed `core.Request`. Return facts, not formatted text.
2. Add a cobra command in `internal/cli` that calls it and prints the result.
   Add `--json` if a script is a likely reader.
3. Add the affordance in `internal/gui` and its name to `gui.Actions()`.
4. `make test` — the parity guard fails until step 3 is done, which is the point.
5. If it writes anywhere outside the workspace, it needs a confirmation in the
   window that says what it does *and what it leaves alone*, and an archive entry
   if it displaces something.
