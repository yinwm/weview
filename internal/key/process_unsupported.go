//go:build !darwin && !windows

package key

import (
	"context"
	"fmt"
)

func wechatPIDs(ctx context.Context) ([]int, error) {
	_ = ctx
	return nil, fmt.Errorf("WeChat process discovery is only implemented for macOS or Windows WeChat 4.x in V1")
}
