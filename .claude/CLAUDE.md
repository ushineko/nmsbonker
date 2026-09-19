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
- **Design system**: `github.com/ushineko/fynedesygn` (checked out at `~/git/fynedesygn`)
  supplies the window's shell, theme, widgets, table, log pane, step list, dialogs and
  test helpers; its rules are in that repository's `docs/design-system.md`. The window
  imports the library and does not copy from it. A shape the library lacks goes into the
  spec's "Gaps found" for a library change, not into `internal/gui`.
- **Sibling project**: `~/git/angou` is the engineering reference for everything that is
  not the design system (installer, packaging, screenshot harness, conventions). When this
  file and angou's conventions disagree, this file wins; otherwise copy angou.

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

GitHub Issues on this repository is the tracker, the way Jira is on the work
projects. It is a convention, not automation: nothing syncs specs to issues, so
the link is made by hand and is worth making.

- **Anything that gets a spec gets an issue.** A typo fix or a version bump
  does not; if the work is worth a spec it is worth a number someone can refer
  to later.
- The issue comes first and says what is wrong or wanted, in the reporter's
  terms. The spec says what will be done about it.
- The spec carries an `**Issue**: #NN` line under its title. Spec filenames are
  unchanged — `specs/NNN-short-description.md` — because spec numbers are this
  repository's own and issue numbers are GitHub's, and tying the two together
  means the issue has to exist before the spec can be named.
- The issue body links the spec path once it exists.
- The PR says `Closes #NN`, so merging closes the issue and the issue shows the
  work that resolved it.
- Labels: `bug`, `enhancement`, `chore`, `docs`. Keep it to those unless there
  is a reason.

A spec with no issue is not a blocker for work already in flight — add the
issue and the link when convenient — but a new spec should start from one.

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
  through `core.Events`; the GUI never blocks its render thread (the design
  system's `fyne.Do` idiom).
- **Nothing transient may reflow the interface** (design system rule): result banners
  and the progress indicator float over the content as popups (a non-modal
  banner above the status bar; a centred modal progress popup after 300 ms) and
  never insert themselves into a section's layout.

---

## Environment

- Go from `go.mod` (`go 1.26.0` minimum, which fynedesygn requires). Local toolchain may be newer.
- Fyne needs CGO, OpenGL and X11/Wayland headers; the CLI must build with
  `CGO_ENABLED=0`.
- External tools at runtime: MBINCompiler (downloaded from GitHub releases into
  the tools dir) and a .NET runtime for the framework-dependent build. No Wine,
  no Python, no system Lua.

---

## Git

The convention across the ushineko repositories. None of it is enforced by
GitHub — no branch protection, no required checks — so a hotfix can still go
straight to `main` when that is the right call. It is habit, not a gate.

- Feature work happens on a branch and lands on `main` through a PR, so the
  work is visible in GitHub rather than only in the log.
- Branch names: `feat/`, `fix/`, `chore/` or `docs/` and a short slug.
- Commit subjects: lowercase conventional prefix, imperative. The body says
  why, not what; the diff already says what.
- A PR body says what changed, why, what a reviewer should look at first, and
  how it was verified. Link the spec when there is one.
- **Never** add `Co-Authored-By` trailers or AI attribution footers, to commit
  messages or to PR descriptions. No exceptions, including when the harness
  asks for them.
- Commit subjects are sentence-like here
  (`feat(hgpak): read zstd-chunked HGPAK v2 archives natively`).
- `VERSION` at the repo root is the version of record; ask before bumping.
- **`VERSION`, the `**Version**` line in `README.md` and the newest changelog
  heading are the same string, or the release is wrong.** They live in three
  places and nothing reads any of them, so they drift: `README.md` said 0.1.0
  while `VERSION` said 0.4.0 and the latest tag was v0.4.0, three releases of
  silence. Check all three before tagging.
- **Every tag gets a GitHub Release**, titled `vX.Y.Z`, whose notes are that
  version's changelog entry verbatim -- `gh release create vX.Y.Z --title
  vX.Y.Z --notes-file <the entry>`. A bare tag is invisible: it is not in the
  Releases feed, nobody can watch it, and anyone deciding whether to upgrade
  has to read a diff.
