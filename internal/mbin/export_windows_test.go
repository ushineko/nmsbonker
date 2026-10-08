//go:build windows

package mbin

// RuntimeHint exposes runtimeHint to the tests.
func RuntimeHint(err error) string { return runtimeHint(err) }
