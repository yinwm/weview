//go:build windows

package key

import "syscall"

var kernel32DLL = syscall.NewLazyDLL("kernel32.dll")

var procCloseHandle = kernel32DLL.NewProc("CloseHandle")

func closeWindowsHandle(handle uintptr) {
	if handle != 0 {
		_, _, _ = procCloseHandle.Call(handle)
	}
}
