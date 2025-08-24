//go:build windows

package main

import "syscall"

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess = kernel32.NewProc("OpenProcess")
	procCloseHandle = kernel32.NewProc("CloseHandle")
)

func checkProcessWithKill(pid int) bool {
	handle, _, _ := procOpenProcess.Call(
		uintptr(0x1000), // PROCESS_QUERY_LIMITED_INFORMATION
		uintptr(0),      // bInheritHandle
		uintptr(pid))

	if handle == 0 {
		return false
	}

	procCloseHandle.Call(handle)
	return true
}
