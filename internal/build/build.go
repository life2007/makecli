/**
 * [INPUT]: 依赖 runtime/debug 的 ReadBuildInfo
 * [OUTPUT]: 对外提供 Version、Date 变量（可通过 ldflags 在构建时注入）
 * [POS]: internal/build 的版本元数据持有者，被 cmd/version.go 消费
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package build

import "runtime/debug"

// Version 在构建时通过 ldflags 注入，默认为 DEV
var Version = "DEV"

// Date 在构建时通过 ldflags 注入，格式 YYYY-MM-DD
var Date = ""

func init() {
	// 若未通过 ldflags 注入，尝试从 go module 构建信息中兜底
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if Version == "DEV" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
	if Date == "" {
		Date = deriveDate(info.Settings)
	}
}

// deriveDate 从构建信息的 vcs.time 派生日期，截取为 YYYY-MM-DD（best-effort）。
// 无 vcs.time 或值过短时分别返回空 / 原值，绝不越界。
func deriveDate(settings []debug.BuildSetting) string {
	for _, s := range settings {
		if s.Key != "vcs.time" {
			continue
		}
		if len(s.Value) >= 10 {
			return s.Value[:10]
		}
		return s.Value
	}
	return ""
}
