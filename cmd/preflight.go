/**
 * [INPUT]: 依赖 errors、fmt、os、path/filepath、github.com/spf13/cobra
 * [OUTPUT]: 对外提供 newPreflightCmd 函数、errPreflightFailed 哨兵错误
 * [POS]: cmd 模块的顶层 preflight 命令，校验工作目录是否具备 Make app 必需工程骨架
 *        （apps/dsl 目录 + apps/service/package.json + apps/ui/package.json）；
 *        任一缺失返回 errPreflightFailed（由 main.go 转译为退出码 1），作 CI / deploy 前置门禁
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// ---------------------------------- 哨兵错误 ----------------------------------

// errPreflightFailed 表示工程骨架检查未通过。沿 cobra RunE 链向上返回，
// 由 main.go 转译为退出码 1，使 CI / deploy 能据此门禁；它不是执行失败，
// 故 preflight 命令静默其错误消息（SilenceErrors），避免污染 stderr。
var errPreflightFailed = errors.New("preflight: project layout check failed")

// ---------------------------------- 必需骨架 ----------------------------------

// layoutEntry 描述一项必需的工程结构条目：path 相对工程根，dir 区分目录/文件。
type layoutEntry struct {
	path string
	dir  bool
}

// requiredLayout 是 Make app 工程的必需骨架——deploy 前置门禁。
// dsl 目录承载 DSL 定义；service / ui 各自是带 package.json 的 Node 子工程。
var requiredLayout = []layoutEntry{
	{"apps/dsl", true},
	{"apps/service/package.json", false},
	{"apps/ui/package.json", false},
}

// ---------------------------------- 命令定义 ----------------------------------

func newPreflightCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preflight [dir]",
		Short: "Check the directory has a valid Make app project layout",
		Long: `Preflight verifies the directory contains the required Make app skeleton:

  apps/dsl/                  DSL definitions (directory)
  apps/service/package.json  backend service (Node project)
  apps/ui/package.json       frontend ui (Node project)

Any missing entry fails the check (exit code 1), so it can gate CI or deploy.
The directory defaults to the current working directory.`,
		Example: `  makecli preflight
  makecli preflight ./my-app`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true, // 检查未过返回 errPreflightFailed 仅作退出码信号，不打印 error: 行
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			return reportPreflightError(cmd, runPreflight(root))
		},
	}
	return cmd
}

// reportPreflightError 在命令开启 SilenceErrors 的前提下亲自打印真实错误到 stderr，
// 但放过 errPreflightFailed 哨兵——它仅用于把「检查未过」翻译成非零退出码。
func reportPreflightError(cmd *cobra.Command, err error) error {
	if err != nil && !errors.Is(err, errPreflightFailed) {
		cmd.PrintErrln(cmd.ErrPrefix(), err.Error())
	}
	return err
}

// runPreflight 逐项 stat requiredLayout，打印 ✓ / ✗ 清单。
// 任一项缺失或类型不符 → 返回 errPreflightFailed（退出码 1）。
func runPreflight(root string) error {
	display := root
	if abs, err := filepath.Abs(root); err == nil {
		display = abs
	}
	fmt.Printf("%-10s %s\n\n", "Project:", display)

	failed := 0
	for _, e := range requiredLayout {
		if err := checkLayoutEntry(root, e); err != nil {
			fmt.Printf("✗ %-26s %s\n", e.path, err)
			failed++
		} else {
			fmt.Printf("✓ %s\n", e.path)
		}
	}

	if failed > 0 {
		fmt.Printf("\nFAIL: %d/%d checks failed — not a valid Make app project\n", failed, len(requiredLayout))
		return errPreflightFailed
	}
	fmt.Printf("\nOK: project layout looks good\n")
	return nil
}

// checkLayoutEntry 校验单项；通过返回 nil，否则返回失败原因（直接用于输出）。
func checkLayoutEntry(root string, e layoutEntry) error {
	info, err := os.Stat(filepath.Join(root, e.path))
	if err != nil {
		return errors.New("missing")
	}
	if e.dir && !info.IsDir() {
		return errors.New("expected directory, found file")
	}
	if !e.dir && info.IsDir() {
		return errors.New("expected file, found directory")
	}
	return nil
}
