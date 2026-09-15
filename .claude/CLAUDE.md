# nmsbonker Project Guidelines

Follows the Ralph Wiggum methodology (see `~/.claude/CLAUDE.md`) with the extensions
below.

---

## Project Overview

- **Type**: Go CLI + desktop GUI (Fyne)
- **Purpose**: No Man's Sky mod builder / trainer for Linux. Rebuilds AMUMSS-format
  `.lua` mod scripts against the installed game's data files into one merged,
  collision-free mod, and deploys it. Native Go pipeline, no Wine, no Windows VM.
- **Module**: `github.com/ushineko/nmsbonker`
- **Sibling project**: `~/git/angou` is the design system and engineering reference.
  When this file and angou's conventions disagree, this file wins; otherwise copy angou.

---

## Selected Policies

Load the following policy modules from `~/.claude/policies/`:

- `languages/go.md`
- `languages/bash.md`
- `git/standard.md`
- `release-safety/minimal.md`
- `security/owasp-review.md`
- `testing/philosophy.md`
- `communication/standards.md`

---

## Ralph Settings

```yaml
validation: milestones-only
```

---

## Issue Tracking

Personal public GitHub repository, no issue tracker. Spec files are named without
ticket IDs (`specs/NNN-short-description.md`). Do not prompt for ticket IDs.

---

## Public-repository rules (non-negotiable)

This repository is **public**. The following hold without exception:

- **No game assets.** Nothing extracted from the game's `.pak` files (MBIN, MXML,
  textures, tables) may be committed, not even as test fixtures. Tests that need
  game data read it from the user's install at test time and skip when it is
  absent (`NMSBONKER_GAME_DIR`, `NMSBONKER_GOLDEN_DIR`, `NMSBONKER_REFERENCE_DIR`).
  Synthetic fixtures are hand-written and small.
- **No third-party mod scripts.** AMUMSS `.lua` scripts downloaded from Nexus or
  elsewhere belong to their authors and stay in the user's library directory
  (`$XDG_DATA_HOME/nmsbonker/library/`). Only scripts authored for this project
  (the built-in tweaks under `internal/tweaks/scripts/`) are committed.
- **No personal paths, hostnames, or Steam identifiers** in committed code, docs,
  or fixtures. Paths are derived at runtime from Steam's own manifests.
- **No credentials.** The GitHub API is used unauthenticated; `GITHUB_TOKEN` is
  honoured from the environment only and never persisted or logged.

---

## Architecture rules

- **CLI/GUI parity**: every user-facing operation is a headless function in
  `internal/core` taking a request struct and returning a result struct. The
  cobra CLI (`cmd/nmsbonker`) and the Fyne GUI (`cmd/nmsbonker-gui`) render only.
  A new operation lands in core first, then in both front ends, in the same
  commit. Enforced by a parity test with a documented allow-list.
- **The game directory is read-only except during deploy and a save edit.** Build
  output goes to the workspace under `$XDG_DATA_HOME/nmsbonker/`; only `core.Deploy`
  writes under `GAMEDATA/MODS/` and `Binaries/SETTINGS/GCMODSETTINGS.MXML`, and it
  archives what it replaces first. Only `core.writeSave` (spec 007) writes under the
  Proton prefix's save folder, and it copies the whole profile to the save backup
  first, refuses while the game is running, and writes each file atomically.
- **Recompile gate**: a MBIN is shipped only if MBINCompiler recompiles the merged
  MXML cleanly. A failed structural edit is retried without it, then dropped and
  reported. Never ship a file the compiler rejected.
- **Golden parity with the reference Python builder** is the correctness oracle for
  the edit engine (spec 002). Do not "improve" engine semantics without updating
  the golden fixtures and saying so in the spec.
- **Long-running work is cancellable** (`context.Context`) and reports progress
  through `core.Events`; the GUI never blocks its render thread (angou's
  `fyne.Do` idiom).
- **Nothing transient may reflow the interface** (angou rule): banners, progress
  and the busy indicator live in fixed-height regions.

---

## Environment

- Go from `go.mod` (`go 1.25.0` minimum). Local toolchain may be newer.
- Fyne needs CGO, OpenGL and X11/Wayland headers; the CLI must build with
  `CGO_ENABLED=0`.
- External tools at runtime: MBINCompiler (downloaded from GitHub releases into
  the tools dir) and a .NET runtime for the framework-dependent build. No Wine,
  no Python, no system Lua.

---

## Git

- Work on `main` directly for this single-developer project unless a change is
  experimental; no PR flow.
- Never add `Co-Authored-By` trailers or AI attribution footers. No exceptions.
- Commit subjects: lowercase conventional prefix, imperative, sentence-like
  (`feat(hgpak): read zstd-chunked HGPAK v2 archives natively`).
- `VERSION` at the repo root is the version of record; ask before bumping.
