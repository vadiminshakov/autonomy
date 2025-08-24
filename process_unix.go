//go:build !windows

package main

import "syscall"

func checkProcessWithKill(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil
}
