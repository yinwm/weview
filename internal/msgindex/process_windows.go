//go:build windows

package msgindex

import "os"

func processAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}
