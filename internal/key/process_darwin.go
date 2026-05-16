//go:build darwin

package key

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func wechatPIDs(ctx context.Context) ([]int, error) {
	cmd := exec.CommandContext(ctx, "pgrep", "-x", "WeChat")
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("find WeChat process: %w", err)
	}
	lines := strings.Fields(string(out))
	pids := make([]int, 0, len(lines))
	for _, line := range lines {
		pid, err := strconv.Atoi(line)
		if err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
