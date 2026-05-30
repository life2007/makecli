/**
 * [INPUT]: 依赖 runtime/debug、testing
 * [OUTPUT]: 单元测试，无导出
 * [POS]: internal/build 的版本/日期派生测试，覆盖 vcs.time 兜底
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package build

import (
	"runtime/debug"
	"testing"
)

func TestDeriveDate(t *testing.T) {
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{
			name:     "vcs.time 截取为 YYYY-MM-DD",
			settings: []debug.BuildSetting{{Key: "vcs.time", Value: "2026-05-30T06:20:22Z"}},
			want:     "2026-05-30",
		},
		{
			name:     "无 vcs.time 返回空",
			settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}},
			want:     "",
		},
		{
			name:     "空 settings 返回空",
			settings: nil,
			want:     "",
		},
		{
			name:     "异常短值原样保留（不越界）",
			settings: []debug.BuildSetting{{Key: "vcs.time", Value: "2026"}},
			want:     "2026",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveDate(tt.settings); got != tt.want {
				t.Errorf("deriveDate = %q, want %q", got, tt.want)
			}
		})
	}
}
