/**
 * [INPUT]: 依赖 bytes、errors、fmt、strings、testing，依赖 internal/api 的 ErrAuthFailed；用 setProfile/setEnvFlag 隔离全局态
 * [OUTPUT]: 覆盖 reportExecuteError 各分支（nil/退出码哨兵静默/真实错误打印/鉴权升级）+ authFailedHint 文案
 * [POS]: cmd 模块错误呈现单一出口的测试，守护退出码哨兵静默、真实错误打印、鉴权引导升级三态
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/qfeius/makecli/internal/api"
)

// TestReportExecuteError 锁定错误呈现单一出口的契约：
// 退出码哨兵（errDiffFound/errPreflightFailed）静默、真实错误打印 error: 前缀、鉴权失败升级为引导。
func TestReportExecuteError(t *testing.T) {
	authErr := fmt.Errorf("%w [990300403]: token验证失败", api.ErrAuthFailed)

	tests := []struct {
		name      string
		err       error
		wantPrint bool
		wantParts []string // 若打印，输出须包含的片段
	}{
		{"nil silent", nil, false, nil},
		{"diff sentinel silent", errDiffFound, false, nil},
		{"preflight sentinel silent", errPreflightFailed, false, nil},
		{"real error prints", errors.New("connection refused"), true, []string{"error:", "connection refused"}},
		{"auth failed upgrades", authErr, true, []string{
			"error: 鉴权失败 [990300403]: token验证失败",
			"makecli login",
			"makecli configure verify",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			reportExecuteError(&buf, tt.err)
			if (buf.Len() > 0) != tt.wantPrint {
				t.Fatalf("printed=%v (%q), want printed=%v", buf.Len() > 0, buf.String(), tt.wantPrint)
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(buf.String(), part) {
					t.Errorf("output missing %q\ngot:\n%s", part, buf.String())
				}
			}
		})
	}
}

// TestAuthFailedHintShowsContext 守护鉴权引导回显当前 profile / env（用于识别环境串号）。
func TestAuthFailedHintShowsContext(t *testing.T) {
	setProfile(t, "work")
	setEnvFlag(t, "production")

	hint := authFailedHint(fmt.Errorf("%w [990300403]: token验证失败", api.ErrAuthFailed))

	for _, want := range []string{
		"鉴权失败 [990300403]: token验证失败",
		"当前 profile: work | env: production",
		"    makecli login",
	} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q\ngot:\n%s", want, hint)
		}
	}
}
