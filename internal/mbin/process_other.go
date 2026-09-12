//go:build !unix

package mbin

import "os/exec"

// setProcessGroup is a no-op away from unix; exec.CommandContext's own
// cancellation, plus the WaitDelay in run(), is what bounds a run there.
func setProcessGroup(_ *exec.Cmd) {}
