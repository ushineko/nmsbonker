#!/usr/bin/env bash
#
# Removes exactly what install.sh puts in place: two binaries, a launcher entry
# and an icon. Everything nmsbonker has made for you stays, and this prints
# where it is. Idempotent.

set -euo pipefail

BIN_DIR="${HOME}/.local/bin"
APP_DIR="${HOME}/.local/share/applications"
ICON_DIR="${HOME}/.local/share/icons/hicolor/scalable/apps"

APP_ID="io.ushineko.nmsbonker"

DRY_RUN=0
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=1 ;;
        -h|--help)
            cat <<'USAGE'
Usage: uninstall.sh [--dry-run]

  --dry-run   List what would be removed, change nothing

Removes only the four files install.sh placed. Your settings, your mod library,
the MBINCompiler it downloaded, the build output, the deploy archive and the
save backups are left alone, and their locations are printed so you can remove
them by hand if you want to.
USAGE
            exit 0
            ;;
        *) echo "Unknown option: $arg" >&2; exit 1 ;;
    esac
done

echo "Removing nmsbonker ..."

removed=0
for f in "${BIN_DIR}/nmsbonker" "${BIN_DIR}/nmsbonker-gui" \
         "${APP_DIR}/${APP_ID}.desktop" \
         "${ICON_DIR}/nmsbonker.svg"; do
    if [ -e "$f" ]; then
        removed=$((removed + 1))
        if [ "$DRY_RUN" -eq 1 ]; then
            echo "  would remove $f"
        else
            echo "  removing $f"
            rm -f "$f"
        fi
    fi
done
if [ "$removed" -eq 0 ]; then
    echo "  nothing to remove; install.sh has not run, or has already been undone"
fi

if [ "$DRY_RUN" -eq 0 ]; then
    command -v update-desktop-database >/dev/null 2>&1 && \
        update-desktop-database "${APP_DIR}" >/dev/null 2>&1 || true
    command -v gtk-update-icon-cache >/dev/null 2>&1 && \
        gtk-update-icon-cache -f -t "${HOME}/.local/share/icons/hicolor" >/dev/null 2>&1 || true
fi

# Every one of these is either the user's own work or something that took time
# to produce, so none of it is removed automatically. The window's appearance
# preferences are written by the application rather than by the installer, and
# hold a colour scheme, a font name and a text size and nothing else.
DATA_HOME="${XDG_DATA_HOME:-${HOME}/.local/share}"
CONFIG_HOME="${XDG_CONFIG_HOME:-${HOME}/.config}"
CACHE_HOME="${XDG_CACHE_HOME:-${HOME}/.cache}"

echo
echo "Done."
echo
echo "Nothing you made was removed. It is in:"
echo
# The paths are of wildly different lengths, so the column is measured rather
# than guessed; a listing whose second column does not line up is a listing
# nobody reads to the end.
kept=(
    "${CONFIG_HOME}/nmsbonker/config.json|your settings and build order"
    "${DATA_HOME}/nmsbonker/library|your .lua mod scripts"
    "${DATA_HOME}/nmsbonker/tools|the MBINCompiler it downloaded"
    "${DATA_HOME}/nmsbonker/build|the last build and its report"
    "${DATA_HOME}/nmsbonker/archive|what deploy displaced, for rollback"
    "${DATA_HOME}/nmsbonker/save-backup|copies of your game saves"
    "${CACHE_HOME}/nmsbonker|the pak index and decompiled game files"
    "${CONFIG_HOME}/fyne/${APP_ID}|the window's colour scheme and font"
)
width=0
for entry in "${kept[@]}"; do
    path="${entry%%|*}"
    if [ "${#path}" -gt "$width" ]; then
        width="${#path}"
    fi
done
for entry in "${kept[@]}"; do
    printf '    %-*s  %s\n' "$width" "${entry%%|*}" "${entry#*|}"
done
echo
echo "A mod already installed in the game is also still there. To take it out"
echo "before removing nmsbonker, run: nmsbonker undeploy"
