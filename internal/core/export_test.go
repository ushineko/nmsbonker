package core

// SetGameRunning makes the game-running check answer running until the
// returned undo is called (spec 007 AC6, spec 020 R4.3).
func SetGameRunning(running bool) func() {
	prev := gameRunning
	gameRunning = func() bool { return running }
	return func() { gameRunning = prev }
}
