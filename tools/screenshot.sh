#!/usr/bin/env bash
# Capture nmsbonker-gui for the README, on KDE/Wayland.
#
# Adapted from angou's tools/screenshot.sh (MIT, same author); the window-finding,
# focus-checking and aspect-ratio machinery is angou's and the reasons below are
# its comments, kept because they are still the reasons.
#
# Unlike a hand-driven capture, this one drives the window itself: nmsbonker-gui takes
# --section, --scheme and --config, so the script starts a fresh window on the section it
# wants, grabs it, and kills it. Refreshing the whole set is one command with nothing to
# click, which is the difference between screenshots that track the interface and
# screenshots that quietly go stale.
#
# Two things still make this less trivial than "take a screenshot":
#
#   1. The active window is almost never the one we want. Refreshing these usually means
#      an agent or a terminal driving the capture, so whatever has focus is the terminal.
#      The window is therefore raised first, and found by window *class* -- searching by
#      name also matches a browser sitting on the project's GitHub page.
#   2. When a dialog is open the dialog *is* the active window, so an active-window grab
#      returns the dialog alone on a transparent background. For those shots pass
#      --with-dialog: it captures the whole desktop and crops to the window's geometry.
#
# Nothing in these images comes from the person running the script. The window is pointed
# at a settings file this script writes and throws away, whose directories are all under
# one temporary root, so no home path, no Steam library and no mod library of anyone's can
# appear in a committed screenshot. That is a rule rather than a nicety: these images go
# into a public README.
#
# Requires kdotool (Wayland's xdotool), spectacle, and python3 with Pillow.
set -euo pipefail

CLASS="io.ushineko.nmsbonker"
DEMO=""
BIN="${NMSBONKER_GUI:-$(command -v nmsbonker-gui || echo ./nmsbonker-gui)}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
    cat <<'USAGE'
usage: tools/screenshot.sh [--with-dialog] [--scheme NAME] --section NAME <output.png>
       tools/screenshot.sh --all

  --section NAME  which section to open on (Overview, Mods, Tweaks, Build, Report, ...)
  --scheme NAME   colour scheme for this run; not saved over the user's choice
  --with-dialog   a dialog is open: capture the desktop and crop, rather than grabbing
                  the active window (which would be the dialog on its own)
  --all           refresh the README set into assets/, then print the alt-text reminder

The README set is the sections that show a reader something the text cannot. Settings,
Appearance and About are left out deliberately: two are forms and one is prose.

  assets/screenshot-overview.png    what this machine has, ranked
  assets/screenshot-mods.png        the build order, with each mod's verdict
  assets/screenshot-tweaks.png      the built-in mods and their parameters
  assets/screenshot-report.png      what the last build made of each mod

The alt text in README.md describes what is actually in each image. It is the only
description a screen-reader user gets, and a stale one is worse than none -- check it
still matches before committing a new capture.
USAGE
}

with_dialog=0
section=""
scheme=""
out=""
all=0
while [ $# -gt 0 ]; do
    case "$1" in
        --with-dialog) with_dialog=1 ;;
        --section) shift; section="${1:-}" ;;
        --scheme) shift; scheme="${1:-}" ;;
        --all) all=1 ;;
        -h|--help) usage; exit 0 ;;
        -*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
        *) out="$1" ;;
    esac
    shift
done

for tool in kdotool spectacle python3; do
    command -v "$tool" >/dev/null || { echo "$tool is not installed" >&2; exit 1; }
done
python3 -c "import PIL" 2>/dev/null || { echo "python3 Pillow is not installed" >&2; exit 1; }
[ -x "$BIN" ] || { echo "nmsbonker-gui not found (set NMSBONKER_GUI, or run make build-gui)" >&2; exit 1; }

# demo builds the settings document the captures are taken against.
#
# Everything it points at is inside one temporary directory, including the mod library, so
# the window has nothing of the developer's to draw. The library is seeded with a couple of
# obviously invented scripts so the Mods table is not empty; the built-in tweaks need no
# seeding, since they are in the binary.
demo() {
    DEMO=$(mktemp -d)
    mkdir -p "$DEMO/library" "$DEMO/build" "$DEMO/cache" "$DEMO/home"

    cat > "$DEMO/library/ExampleFasterMining.lua" <<'LUA'
MINING_SPEED = 4
NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "ExampleFasterMining.pak",
["MOD_AUTHOR"]   = "an example",
["MODIFICATIONS"] =
    {
        { ["MBIN_CHANGE_TABLE"] =
            { { ["MBIN_FILE_SOURCE"] = "GCGAMEPLAYGLOBALS.GLOBAL.MBIN",
                ["EXML_CHANGE_TABLE"] =
                  { { ["MATH_OPERATION"] = "*",
                      ["VALUE_CHANGE_TABLE"] = { {"MiningLaserHeatTime", MINING_SPEED} } } } } } }
    }
}
LUA
    cat > "$DEMO/library/ExampleQuieterScanner.lua" <<'LUA'
SCAN_COOLDOWN = 2
NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "ExampleQuieterScanner.pak",
["MOD_AUTHOR"]   = "an example",
["MODIFICATIONS"] =
    {
        { ["MBIN_CHANGE_TABLE"] =
            { { ["MBIN_FILE_SOURCE"] = "GCGAMEPLAYGLOBALS.GLOBAL.MBIN",
                ["EXML_CHANGE_TABLE"] =
                  { { ["VALUE_CHANGE_TABLE"] = { {"ScanTime", SCAN_COOLDOWN} } } } } } }
    }
}
LUA

    cat > "$DEMO/config.json" <<JSON
{
  "library_dir": "$DEMO/library",
  "workspace_dir": "$DEMO/build",
  "cache_dir": "$DEMO/cache",
  "tools_dir": "$DEMO/tools",
  "mod_name": "COSMOS COMBINE"
}
JSON
}

demo_cleanup() {
    [ -n "$DEMO" ] || return 0
    rm -rf "$DEMO"
}

# capture starts a window on the requested section, grabs it, and stops it again.
# Starting fresh per shot rather than reusing one window keeps each image independent
# of whatever the previous one left selected.
capture() {
    local sect="$1" dest="$2"

    # Wait for any previous instance to be gone before starting the next. Two windows of
    # the same class at once means `search --class | head -1` can return the one that is
    # on its way out, and every check downstream then refers to the wrong window.
    local gone=0
    while [ "$gone" -lt 40 ]; do
        [ -z "$(timeout 10 kdotool search --class "$CLASS" 2>/dev/null || true)" ] && break
        sleep 0.25
        gone=$((gone + 1))
    done

    # HOME and the XDG directories are redirected so the window cannot reach a remembered
    # setting. XDG_RUNTIME_DIR is deliberately NOT: that is where the Wayland display
    # socket lives, and pointing it at a temporary directory leaves the window unable to
    # reach the compositor at all, which looks exactly like the window failing to start.
    HOME="$DEMO/home" XDG_CONFIG_HOME="$DEMO/home/.config" \
        XDG_DATA_HOME="$DEMO/home/.local/share" XDG_CACHE_HOME="$DEMO/home/.cache" \
        "$BIN" --config "$DEMO/config.json" --section "$sect" \
        ${scheme:+--scheme "$scheme"} >/dev/null 2>&1 &
    local pid=$!
    # shellcheck disable=SC2064  # pid is captured deliberately, at trap-set time
    trap "kill $pid 2>/dev/null || true; wait $pid 2>/dev/null || true" RETURN

    # Poll rather than sleeping a fixed time: a cold start after a rebuild is much
    # slower than a warm one, and a fixed wait is either flaky or wasteful.
    local wid="" waited=0
    while [ "$waited" -lt 40 ]; do
        wid=$(timeout 10 kdotool search --class "$CLASS" 2>/dev/null | head -1 || true)
        [ -n "$wid" ] && break
        sleep 0.25
        waited=$((waited + 1))
    done
    [ -n "$wid" ] || { echo "the window never appeared (no window of class $CLASS)" >&2; return 1; }

    # Activating is asynchronous, and `spectacle -a` grabs whatever is active at the
    # moment it fires. If the raise has not landed yet it silently captures the terminal,
    # or another monitor's window, and writes a plausible-looking PNG of the wrong thing.
    # So: activate, then confirm we actually have focus before grabbing.
    local active="" tries=0
    while [ "$tries" -lt 12 ]; do
        timeout 10 kdotool windowactivate "$wid" >/dev/null 2>&1 || true
        sleep 0.5
        active=$(timeout 10 kdotool getactivewindow 2>/dev/null || true)
        [ "$active" = "$wid" ] && break
        tries=$((tries + 1))
    done
    [ "$active" = "$wid" ] || { echo "could not focus the window (active=$active want=$wid)" >&2; return 1; }
    sleep 1.5                   # let it repaint after the raise, and let its loads land

    rm -f "$dest"
    if [ "$with_dialog" -eq 0 ]; then
        # -S drops the compositor's drop shadow, which otherwise pads the image unevenly
        timeout 30 spectacle -a -b -n -S -o "$dest" >/dev/null 2>&1 || true
        sleep 1.5
    else
        local tmp; tmp=$(mktemp --suffix=.png)
        timeout 30 spectacle -f -b -n -o "$tmp" >/dev/null 2>&1 || true
        sleep 1.5
        local geo; geo=$(timeout 10 kdotool getwindowgeometry "$wid")
        python3 "${REPO_DIR}/tools/crop.py" "$tmp" "$dest" \
            "$(printf '%s' "$geo" | awk '/Position/{print $2}')" \
            "$(printf '%s' "$geo" | awk '/Geometry/{print $2}')"
        rm -f "$tmp"
    fi

    [ -s "$dest" ] || { echo "capture produced nothing" >&2; return 1; }

    # A last sanity check on the geometry. Even with the focus check above, a grab can
    # land on the wrong surface; an image wildly wider or taller than the window we asked
    # for is not a screenshot of it, and shipping it to the README unnoticed is worse
    # than failing here.
    local geo_check; geo_check=$(timeout 10 kdotool getwindowgeometry "$wid" 2>/dev/null || true)
    python3 - "$dest" "$(printf '%s' "$geo_check" | awk '/Geometry/{print $2}')" <<'PY'
import sys
from PIL import Image

path, dim = sys.argv[1], sys.argv[2] if len(sys.argv) > 2 else ""
im = Image.open(path)
if dim and "x" in dim:
    w, h = (float(v) for v in dim.split("x"))
    # Allow for output scaling (the capture is in device pixels) but reject an
    # aspect ratio that is not the window's.
    want, got = w / h, im.width / im.height
    if abs(want - got) / want > 0.05:
        sys.exit("captured %dx%d, but the window is %gx%g -- wrong window grabbed"
                 % (im.width, im.height, w, h))
PY
    python3 - "$dest" <<'PY'
import os, sys
from PIL import Image
p = sys.argv[1]
im = Image.open(p)
print("  %s  %dx%d  %.0fK" % (os.path.basename(p), im.width, im.height,
                              os.path.getsize(p) / 1024))
PY
}

if [ "$all" -eq 1 ]; then
    # Force a scheme unless one was asked for. Without this the set inherits whatever the
    # person running it last picked in Appearance, so two refreshes on two machines
    # produce differently-coloured images for no reason.
    : "${scheme:=Breeze Dark}"
    mkdir -p "${REPO_DIR}/assets"
    demo
    trap demo_cleanup EXIT
    for s in Overview Mods Tweaks Report; do
        low=$(printf '%s' "$s" | tr '[:upper:]' '[:lower:]')
        capture "$s" "${REPO_DIR}/assets/screenshot-${low}.png"
    done
    echo
    echo "Now check the alt text in README.md still describes what is in each image,"
    echo "and that no path in any of them belongs to anybody."
    exit 0
fi

[ -n "$out" ] || { usage >&2; exit 2; }
[ -n "$section" ] || { echo "--section is required (or use --all)" >&2; exit 2; }
demo
trap demo_cleanup EXIT
capture "$section" "$out"
