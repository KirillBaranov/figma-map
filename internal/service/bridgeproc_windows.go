//go:build windows

package service

import "syscall"

// detachAttr starts the backend in its own process group on Windows, the
// closest equivalent to Unix's Setsid for outliving this CLI process.
func detachAttr() *syscall.SysProcAttr {
	const createNewProcessGroup = 0x00000200
	return &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}
