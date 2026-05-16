//go:build !darwin && !windows

package key

import "context"

func wechatPermissionHint(ctx context.Context) string {
	return " Automatic process-memory key scanning is only implemented for macOS or Windows WeChat 4.x in V1."
}
