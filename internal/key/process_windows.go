//go:build windows

package key

import (
	"context"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	th32csSnapProcess = 0x00000002
	invalidHandle     = ^uintptr(0)
)

var (
	procCreateToolhelp32Snapshot = kernel32DLL.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32DLL.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32DLL.NewProc("Process32NextW")
)

type processEntry32 struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [syscall.MAX_PATH]uint16
}

func wechatPIDs(ctx context.Context) ([]int, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	snapshot, _, err := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snapshot == invalidHandle {
		return nil, fmt.Errorf("find WeChat process: %w", err)
	}
	defer closeWindowsHandle(snapshot)

	var entry processEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	ok, _, err := procProcess32FirstW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	if ok == 0 {
		return nil, fmt.Errorf("find WeChat process: %w", err)
	}

	var pids []int
	for {
		name := syscall.UTF16ToString(entry.ExeFile[:])
		if isWindowsWeChatProcess(name) && entry.ProcessID > 0 {
			pids = append(pids, int(entry.ProcessID))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		ok, _, _ = procProcess32NextW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
		if ok == 0 {
			break
		}
	}
	return pids, nil
}

func isWindowsWeChatProcess(name string) bool {
	name = strings.TrimSpace(name)
	return strings.EqualFold(name, "Weixin.exe") || strings.EqualFold(name, "WeChat.exe")
}
