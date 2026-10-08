package core

import "fmt"

// gameProcess is the executable name the game runs as, natively on Windows
// and under Proton on Linux.
const gameProcess = "NMS.exe"

// gameRunning reports whether the game is open (spec 007 R6.2, spec 020 R4.3).
// A variable so a test can say either way without the game installed, let
// alone running.
//
//nolint:gochecknoglobals // test seam
var gameRunning = detectGame

// refuseModChangeWhileRunning stops deploy, rollback and undeploy while the game
// is open, where the platform needs it (spec 020 R5.1).
//
// Windows: the running game holds its mod files open, so moving the mod folder
// into the archive fails partway and leaves the archive and GAMEDATA/MODS out
// of step. Linux does not lock open files and keeps its existing behaviour.
func refuseModChangeWhileRunning() error {
	if modChangesNeedGameClosed && gameRunning() {
		return fmt.Errorf("%w; close it before changing its mods", ErrGameRunning)
	}
	return nil
}
