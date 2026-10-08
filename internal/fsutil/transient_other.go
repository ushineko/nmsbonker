//go:build !windows

package fsutil

// transient is false away from Windows: a rename that fails on Linux fails
// for a reason that waiting does not change.
func transient(error) bool { return false }
