//go:build windows

package core

// ProcessRunning exposes the snapshot scan to the tests.
func ProcessRunning(name string) bool { return processRunning(name) }
