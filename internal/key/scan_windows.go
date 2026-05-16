//go:build windows

package key

import (
	"fmt"
	"strings"
	"unsafe"

	"wxview/internal/decrypt"
)

const (
	processVMRead           = 0x0010
	processQueryInformation = 0x0400
	memCommit               = 0x1000
	pageNoAccess            = 0x01
	pageReadOnly            = 0x02
	pageReadWrite           = 0x04
	pageWriteCopy           = 0x08
	pageExecuteRead         = 0x20
	pageExecuteReadWrite    = 0x40
	pageExecuteWriteCopy    = 0x80
	pageGuard               = 0x100
	maxRegionSize           = 500 * 1024 * 1024
	windowsScanChunkSize    = 2 * 1024 * 1024
	maxSQLCipherHexLen      = 192
	windowsScanOverlap      = maxSQLCipherHexLen + 3
)

var (
	procOpenProcess       = kernel32DLL.NewProc("OpenProcess")
	procVirtualQueryEx    = kernel32DLL.NewProc("VirtualQueryEx")
	procReadProcessMemory = kernel32DLL.NewProc("ReadProcessMemory")
)

type memoryBasicInformation struct {
	BaseAddress       uintptr
	AllocationBase    uintptr
	AllocationProtect uint32
	PartitionID       uint16
	_                 uint16
	RegionSize        uintptr
	State             uint32
	Protect           uint32
	Type              uint32
}

func scanSQLCipherPragmaKey(pid int, saltHex string, page1 []byte) (string, error) {
	handle, _, err := procOpenProcess.Call(processVMRead|processQueryInformation, 0, uintptr(pid))
	if handle == 0 {
		return "", fmt.Errorf("OpenProcess failed for pid %d: %w", pid, err)
	}
	defer closeWindowsHandle(handle)

	saltHex = strings.ToLower(saltHex)
	for addr := uintptr(0); ; {
		var mbi memoryBasicInformation
		r1, _, _ := procVirtualQueryEx.Call(handle, addr, uintptr(unsafe.Pointer(&mbi)), unsafe.Sizeof(mbi))
		if r1 == 0 {
			break
		}
		if mbi.State == memCommit && readableProtect(mbi.Protect) && mbi.RegionSize > 0 && mbi.RegionSize < maxRegionSize {
			if keyHex := scanWindowsMemoryRegion(handle, mbi.BaseAddress, mbi.RegionSize, saltHex, page1); keyHex != "" {
				return keyHex, nil
			}
		}
		next := mbi.BaseAddress + mbi.RegionSize
		if next <= addr {
			break
		}
		addr = next
	}
	return "", nil
}

func readableProtect(protect uint32) bool {
	if protect&pageGuard != 0 || protect&pageNoAccess != 0 {
		return false
	}
	switch protect & 0xff {
	case pageReadOnly, pageReadWrite, pageWriteCopy, pageExecuteRead, pageExecuteReadWrite, pageExecuteWriteCopy:
		return true
	default:
		return false
	}
}

func scanWindowsMemoryRegion(handle uintptr, base uintptr, size uintptr, saltHex string, page1 []byte) string {
	var overlap []byte
	for offset := uintptr(0); offset < size; {
		chunkSize := uintptr(windowsScanChunkSize)
		if remaining := size - offset; remaining < chunkSize {
			chunkSize = remaining
		}
		data, ok := readWindowsProcessMemory(handle, base+offset, chunkSize)
		if ok {
			scanData := data
			if len(overlap) > 0 {
				combined := make([]byte, 0, len(overlap)+len(data))
				combined = append(combined, overlap...)
				combined = append(combined, data...)
				scanData = combined
			}
			if keyHex := scanSQLCipherBuffer(scanData, saltHex, page1); keyHex != "" {
				return keyHex
			}
			if len(data) > windowsScanOverlap {
				overlap = append(overlap[:0], data[len(data)-windowsScanOverlap:]...)
			} else {
				overlap = append(overlap[:0], data...)
			}
		} else {
			overlap = nil
		}
		offset += chunkSize
	}
	return ""
}

func readWindowsProcessMemory(handle uintptr, addr uintptr, size uintptr) ([]byte, bool) {
	if size == 0 {
		return nil, false
	}
	buf := make([]byte, int(size))
	var read uintptr
	r1, _, _ := procReadProcessMemory.Call(
		handle,
		addr,
		uintptr(unsafe.Pointer(&buf[0])),
		size,
		uintptr(unsafe.Pointer(&read)),
	)
	if r1 == 0 || read == 0 {
		return nil, false
	}
	return buf[:int(read)], true
}

func scanSQLCipherBuffer(data []byte, saltHex string, page1 []byte) string {
	for i := 0; i+3 < len(data); i++ {
		if data[i] != 'x' || data[i+1] != '\'' {
			continue
		}
		start := i + 2
		end := start
		for end < len(data) && end-start < maxSQLCipherHexLen && isHexByte(data[end]) {
			end++
		}
		hexLen := end - start
		if hexLen < 64 || end >= len(data) || data[end] != '\'' {
			continue
		}
		if keyHex := keyFromSQLCipherHex(string(data[start:end]), saltHex, page1); keyHex != "" {
			return keyHex
		}
		i = end
	}
	return ""
}

func keyFromSQLCipherHex(candidate string, saltHex string, page1 []byte) string {
	candidate = strings.ToLower(candidate)
	switch {
	case len(candidate) == 64:
		if decrypt.ValidateRawHexKey(page1, candidate) {
			return candidate
		}
	case len(candidate) == 96:
		keyHex := candidate[:64]
		if candidate[64:] == saltHex && decrypt.ValidateRawHexKey(page1, keyHex) {
			return keyHex
		}
	case len(candidate) > 96 && len(candidate)%2 == 0:
		keyHex := candidate[:64]
		if candidate[len(candidate)-32:] == saltHex && decrypt.ValidateRawHexKey(page1, keyHex) {
			return keyHex
		}
	}
	return ""
}

func isHexByte(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
