package core

// SetProcRoot points the game-running check at a fixture directory laid out
// like /proc (spec 007 AC6) and returns the undo.
func SetProcRoot(dir string) func() {
	prev := procRoot
	procRoot = dir
	return func() { procRoot = prev }
}
