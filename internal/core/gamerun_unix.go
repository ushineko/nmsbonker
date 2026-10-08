//go:build !windows

package core

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// procRoot is where running processes are looked up. A variable so a test can
// point it at a fixture (spec 007 AC6).
//
//nolint:gochecknoglobals // test seam
var procRoot = "/proc"

// modChangesNeedGameClosed: see refuseModChangeWhileRunning.
const modChangesNeedGameClosed = false

/*
detectGame looks for the game's process under Proton (spec 007 R6.2).

/proc is read directly rather than through a process library: the check is one
directory listing and a short file per process, and the name Proton gives the
game is stable. `comm` is truncated to fifteen characters, which NMS.exe fits
inside; `cmdline` is checked too for the case of a launcher whose comm differs.
*/
func detectGame() bool {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		dir := filepath.Join(procRoot, e.Name())
		if comm, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil &&
			strings.TrimSpace(string(comm)) == gameProcess {
			return true
		}
		if cmd, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			for _, arg := range strings.Split(string(cmd), "\x00") {
				if filepath.Base(strings.ReplaceAll(arg, `\`, "/")) == gameProcess {
					return true
				}
			}
		}
	}
	return false
}
