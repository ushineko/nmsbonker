//go:build !windows

package core

// SetProcRoot points the /proc scan at a fixture directory laid out like /proc
// and returns the undo.
func SetProcRoot(dir string) func() {
	prev := procRoot
	procRoot = dir
	return func() { procRoot = prev }
}

// DetectGame runs the real check, not the seam.
func DetectGame() bool { return detectGame() }
