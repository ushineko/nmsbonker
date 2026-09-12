//go:build unix

package mbin

import (
	"os/exec"
	"syscall"
)

/*
setProcessGroup makes a cancelled conversion kill the whole subtree.

exec.CommandContext's default is to signal the direct child only. That is enough
for MBINCompiler itself, which is one process, but not for anything it starts,
and not for the shell wrappers the tests use: the child dies, the grandchild
keeps the pipes open, and Wait blocks until the grandchild finishes -- exactly
the hang cancellation was supposed to prevent. Putting the child in its own
process group and signalling the group closes that gap. See also the WaitDelay
in run(), which bounds the wait even if a signal is ignored.
*/
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
