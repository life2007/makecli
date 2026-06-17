/**
 * [INPUT]: 依赖 cmd/app_create（deriveAppKey/newAppManifest/writeScaffold/scaffoldOutputs）、cmd/git（initGitRepo/ensureGitignore）、fmt、github.com/spf13/cobra
 * [OUTPUT]: 对外提供 newAppInitCmd 函数；包内 runAppInit
 * [POS]: cmd/app 的 init 子命令——把一个目录变成完整的本地 Make app 项目：写脚手架（CLAUDE.md/AGENTS.md/apps/dsl/app.yaml）+ git init + .gitignore。
 *        与 create 共享同一脚手架内核（writeScaffold + initGitRepo + ensureGitignore），create = init 核心 + 远端注册 + initial commit。
 *        可选位置参数 [appKey]（兼目录名，deriveAppKey 推导，缺省取 cwd 名）；全程幂等 skip-if-exists，刻意不 commit、不碰远端。
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAppInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [appKey]",
		Short: "Scaffold a local Make app project (files + git, idempotent, no remote)",
		Example: `  makecli app init
  makecli app init shop`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			return runAppInit(target)
		},
	}
	return cmd
}

// runAppInit 在 target 目录幂等长出完整本地项目骨架并初始化 git，逐项打印状态。
// appKey 由 target 推导（缺省=cwd 名，validResourceKey 把关），name 默认回退 key、无 description——
// 这些都可在生成的 app.yaml 里编辑。全程 skip-if-exists + 不 commit，重复运行安全。
func runAppInit(target string) error {
	appKey, err := deriveAppKey(target)
	if err != nil {
		return err
	}
	manifest := newAppManifest(appKey, appKey, "")

	created, err := writeScaffold(target, manifest)
	if err != nil {
		return err
	}
	madeNew := map[string]bool{}
	for _, f := range created {
		madeNew[f] = true
	}
	for _, out := range scaffoldOutputs() {
		fmt.Printf("%-22s %s\n", out, statusWord(madeNew[out], "created", "exists"))
	}

	gitCreated, err := initGitRepo(target)
	if err != nil {
		return err
	}
	fmt.Printf("%-22s %s\n", "git", statusWord(gitCreated, "initialized", "already a repository"))

	changed, err := ensureGitignore(target)
	if err != nil {
		return err
	}
	fmt.Printf("%-22s %s\n", ".gitignore", statusWord(changed, "updated", "already complete"))
	return nil
}

// statusWord 把布尔变更挑成「动作发生 / 已就绪」两个词，消除调用点的 if/else。
func statusWord(changed bool, yes, no string) string {
	if changed {
		return yes
	}
	return no
}
