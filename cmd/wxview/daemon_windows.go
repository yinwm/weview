//go:build windows

package main

import (
	"os"
	"syscall"
)

func daemonSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func daemonSysProcAttr() *syscall.SysProcAttr {
	return nil
}
