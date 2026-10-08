//go:build windows

package mbin

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

/*
envKeep is the allow-list minimalEnv passes through on Windows (spec 023 R6.1).

A Windows process with only PATH and HOME does not get far: without SystemRoot
the .NET runtime cannot load its own crypto and socket providers, and without
TEMP, USERPROFILE and the AppData pair it has nowhere to put a temporary file
or find its per-user state. These are the variables the system itself sets for
every session; none of them carries anything of this program's.
*/
//
//nolint:gochecknoglobals // a fixed list, read-only
var envKeep = []string{
	"PATH", "HOME", "LANG", "TMPDIR",
	"SystemRoot", "windir", "SystemDrive", "ComSpec", "PATHEXT",
	"TEMP", "TMP", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "ProgramData",
	"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432",
	"NUMBER_OF_PROCESSORS", "PROCESSOR_ARCHITECTURE",
}

// envPrefix matches an environment entry's name the way Windows does: without
// case.
func envPrefix(kv, prefix string) bool {
	return len(kv) >= len(prefix) && strings.EqualFold(kv[:len(prefix)], prefix)
}

/*
setProcessGroup starts the compiler without a console window (spec 023 R6.3).

There is no process group to kill on Windows: exec.CommandContext's own kill,
plus the WaitDelay in run(), bounds a cancelled run, and MBINCompiler is one
process with no children (R6.2). What Windows does need is CREATE_NO_WINDOW --
the output is piped either way, and without it a GUI build opens and closes
a console for every file it converts.
*/
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
}

/*
findDotnet returns the dotnet host the way a framework-dependent .exe finds
the runtime: DOTNET_ROOT, then PATH, then the default install location (spec
023 R3.2). The last matters on a machine where the runtime was installed after
the terminal was opened, or by an installer that did not touch PATH: the
compiler would run, and a PATH-only check would say it cannot.
*/
func findDotnet() string {
	var candidates []string
	if root := os.Getenv("DOTNET_ROOT"); root != "" {
		candidates = append(candidates, filepath.Join(root, "dotnet.exe"))
	}
	if p, err := exec.LookPath("dotnet"); err == nil {
		candidates = append(candidates, p)
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		candidates = append(candidates, filepath.Join(pf, "dotnet", "dotnet.exe"))
	}
	for _, c := range candidates {
		//nolint:gosec // a stat of the places the .NET host itself looks; nothing is opened
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// hostFrameworkMissing is the .NET host's exit code for "the framework this
// program needs is not installed" (FrameworkMissingFailure).
const hostFrameworkMissing = 0x80008096

/*
runtimeHint says what to install when a compiler would not start for want of a
.NET runtime (spec 023 R3).

Neither Windows build is self-contained: MBINCompiler-dotnet10.exe needs .NET
10 and MBINCompiler.exe needs .NET 8, and the host's own message goes to a
console nobody sees. One runtime is enough, and .NET 10 serves the build that
is tried first.
*/
func runtimeHint(err error) string {
	var exit *exec.ExitError
	if errors.As(err, &exit) && uint32(exit.ExitCode()) == hostFrameworkMissing { //nolint:gosec // an exit code is a uint32 on Windows
		return " (the .NET runtime it needs is not installed: install the .NET 10 runtime, " +
			"e.g. winget install Microsoft.DotNet.Runtime.10)"
	}
	return ""
}
