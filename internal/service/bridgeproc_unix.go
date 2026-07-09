//go:build !windows

package service

import "syscall"

// detachAttr starts the backend in its own session so it survives this CLI
// process exiting instead of receiving its signals (the Unix equivalent of
// `node backend/dist/index.js &` disowned from the shell).
func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
