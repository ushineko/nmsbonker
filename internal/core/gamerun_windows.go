//go:build windows

package core

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// modChangesNeedGameClosed: see refuseModChangeWhileRunning.
const modChangesNeedGameClosed = true

// detectGame looks for NMS.exe among the running processes (spec 020 R4.3).
func detectGame() bool { return processRunning(gameProcess) }

/*
processRunning reports whether any process's image name is name, ignoring
case as Windows does.

A Toolhelp snapshot rather than `tasklist`: no child process, and no
localised text to parse. A snapshot that cannot be taken reports "not
running", which is what the Linux check does when /proc is unreadable.
*/
func processRunning(name string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(snap) }()

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), name) {
			return true
		}
	}
	return false
}
