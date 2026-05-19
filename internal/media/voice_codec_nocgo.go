//go:build !cgo

package media

import "fmt"

func silkToWAV(_ []byte) ([]byte, error) {
	return nil, fmt.Errorf("voice wav decoding requires a cgo-enabled wxview build")
}
