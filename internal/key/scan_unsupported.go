//go:build (!darwin && !windows) || (darwin && !cgo)

package key

import "fmt"

func scanSQLCipherPragmaKey(pid int, saltHex string, _ []byte) (string, error) {
	return "", fmt.Errorf("process memory key scanning is only implemented for macOS with cgo enabled or Windows")
}
