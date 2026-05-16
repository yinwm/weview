//go:build windows

package key

import "context"

func wechatPermissionHint(ctx context.Context) string {
	_ = ctx
	return " Run the terminal as Administrator, keep Windows WeChat running, and ensure the process name is Weixin.exe or WeChat.exe."
}
