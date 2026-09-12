#!/usr/bin/env bash
#
# Installs the nmsbonker command line and, unless told otherwise, the desktop
# window with its launcher entry and icon. Idempotent: safe to re-run.
#
# Nothing here touches the game, your mod library, or any settings nmsbonker has
# written. It builds from this checkout and copies four files into ~/.local.

set -euo pipefail

BIN_DIR="${HOME}/.local/bin"
APP_DIR="${HOME}/.local/share/applications"
ICON_DIR="${HOME}/.local/share/icons/hicolor/scalable/apps"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

APP_ID="io.ushineko.nmsbonker"

DRY_RUN=0
WITH_GUI=1
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=1 ;;
        --with-gui) WITH_GUI=1 ;;
        --no-gui) WITH_GUI=0 ;;
        -h|--help)
            cat <<'USAGE'
Usage: install.sh [--dry-run] [--no-gui]

  --dry-run   Show what would be installed, change nothing
  --no-gui    Install the command line only, skipping nmsbonker-gui

Installs:
  ~/.local/bin/nmsbonker                                      the command line
  ~/.local/bin/nmsbonker-gui                                  the window
  ~/.local/share/applications/io.ushineko.nmsbonker.desktop   the launcher entry
  ~/.local/share/icons/hicolor/scalable/apps/nmsbonker.svg    its icon

The window is installed by default. It needs CGO and a C toolchain; if it will
not build, the command line is still installed and the window is skipped with a
note naming the packages to install. The command line is static, needs neither,
and does everything the window does.

Nothing you have made is touched, on install or on uninstall: your settings,
your mod library, the compiler nmsbonker downloaded, the build output, the
deploy archive and the save backups all stay where they are. uninstall.sh
prints where they live.
USAGE
            exit 0
            ;;
        *) echo "Unknown option: $arg" >&2; exit 1 ;;
    esac
done

run() {
    if [ "$DRY_RUN" -eq 1 ]; then
        echo "  would run: $*"
    else
        "$@"
    fi
}

echo "Installing nmsbonker from ${REPO_DIR} ..."

if ! command -v go >/dev/null 2>&1; then
    echo "Error: go is not installed. nmsbonker needs Go 1.25 or newer to build." >&2
    echo "       Arch: pacman -S go   Debian/Ubuntu: apt install golang-go" >&2
    exit 1
fi

echo "Building the command line ..."
run make -C "$REPO_DIR" build

echo "Installing the command line to ${BIN_DIR} ..."
run install -Dm755 "${REPO_DIR}/nmsbonker" "${BIN_DIR}/nmsbonker"

# The window is installed by default, but a failure to build it must not take
# the command line down with it. The CLI is the artifact everything depends on
# -- it is what builds and deploys mods -- and a missing C toolchain is a reason
# to skip the window, not a reason to leave the machine without nmsbonker.
if [ "$WITH_GUI" -eq 1 ]; then
    echo "Building the desktop window ..."
    if [ "$DRY_RUN" -eq 1 ] || make -C "$REPO_DIR" build-gui; then
        run install -Dm755 "${REPO_DIR}/nmsbonker-gui" "${BIN_DIR}/nmsbonker-gui"
        run install -Dm644 "${REPO_DIR}/packaging/${APP_ID}.desktop" "${APP_DIR}/${APP_ID}.desktop"
        run install -Dm644 "${REPO_DIR}/packaging/nmsbonker.svg" "${ICON_DIR}/nmsbonker.svg"
    else
        WITH_GUI=0
        echo
        echo "The window did not build, so it was skipped. The command line is unaffected" >&2
        echo "and does everything the window does. Building it needs CGO, OpenGL and the" >&2
        echo "X11 or Wayland development headers:" >&2
        echo "    Arch:          base-devel libgl libxi libxcursor libxrandr libxinerama" >&2
        echo "    Debian/Ubuntu: build-essential libgl1-mesa-dev xorg-dev" >&2
        echo "    Fedora:        gcc mesa-libGL-devel libXi-devel libXcursor-devel libXrandr-devel libXinerama-devel" >&2
        echo "Re-run with --no-gui to skip it without this message." >&2
        echo
    fi
fi

if [ "$WITH_GUI" -eq 1 ] && command -v update-desktop-database >/dev/null 2>&1; then
    run update-desktop-database "${APP_DIR}"
fi
# The icon cache is per theme directory and only some desktops need it poked;
# a failure here costs nothing but a stale icon until the next login.
if [ "$WITH_GUI" -eq 1 ] && command -v gtk-update-icon-cache >/dev/null 2>&1; then
    if [ "$DRY_RUN" -eq 1 ]; then
        echo "  would run: gtk-update-icon-cache -f -t ${HOME}/.local/share/icons/hicolor"
    else
        gtk-update-icon-cache -f -t "${HOME}/.local/share/icons/hicolor" >/dev/null 2>&1 || true
    fi
fi

echo
echo "Done."
case ":${PATH}:" in
    *":${BIN_DIR}:"*) ;;
    *) echo "Note: ${BIN_DIR} is not on your PATH." ;;
esac
echo
echo "Next:"
echo
echo "  nmsbonker status          # what this machine has: the game, the compiler, the caches"
echo "  nmsbonker tools ensure    # install the MBINCompiler this game version needs"
echo "  nmsbonker tweaks list     # the built-in mods, and what each one can be set to"
echo "  nmsbonker build           # merge every enabled mod into one folder"
echo "  nmsbonker deploy          # install it into the game"
if [ "$WITH_GUI" -eq 1 ]; then
    echo
    echo "Or open the window: nmsbonker-gui, or \"nmsbonker\" in your application launcher."
fi
