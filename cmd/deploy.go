/**
 * [INPUT]: 依赖 cmd/client（newRepoClientFromProfile）、cmd/app（loadAppManifestFromFile/validResourceKey）、cmd/app_create（appDSLPath）、errors、fmt、os、slices、strings、time、github.com/go-git/go-git/v5（及 config/plumbing/object/transport/http 子包）、github.com/spf13/cobra
 * [OUTPUT]: 对外提供 newDeployCmd 函数；包级 gitDeployFunc 可打桩变量（测试替换，参照 update.go applyFunc 模式）
 * [POS]: cmd 模块 app 命令组的 deploy 子命令：从 apps/dsl/app.yaml（app 身份单一真相源）读取 app key，调用代码仓库服务幂等准备
 *        preview/production 双环境仓库（MakeService.CreateResource），按 --env 选取 cloneUrl 后用 go-git（纯 Go，不再 shell-out git）
 *        把当前工作树快照(必要时 git init + commit)推送到固定分支（deployBranch，webhook 约定）触发构建；token 走 HTTP BasicAuth(make:<token>)
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package cmd

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/cobra"
)

// deployEnvs 是合法的部署环境集合，与服务端双仓库约定一一对应
var deployEnvs = []string{"preview", "production"}

// deployBranch 是构建流水线 webhook 监听的固定远端分支。
// 部署只推送到此分支——分支名是服务端约定，不是用户可调旋钮。
const deployBranch = "dev"

// anonymousRemote 是 go-git 临时 remote 的固定名（CreateRemoteAnonymous 约定值），
// 仅存在于内存、不写进 .git/config，用完即弃——cloneUrl 每次部署才解析，不该污染用户仓库配置。
const anonymousRemote = "anonymous"

// gitDeployFunc 为包级可打桩变量，单测替换以隔离真实文件系统与网络推送
var gitDeployFunc = gitDeploy

func newDeployCmd() *cobra.Command {
	var env string
	var force bool

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy an app to Make Platform",
		Example: `  makecli app deploy --env preview
  makecli app deploy --env production`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(env, force)
		},
	}

	cmd.Flags().StringVar(&env, "env", "", "target environment: preview | production (required)")
	_ = cmd.MarkFlagRequired("env")
	cmd.Flags().BoolVar(&force, "force", false, "force push")
	return cmd
}

func runDeploy(env string, force bool) error {
	if !slices.Contains(deployEnvs, env) {
		return fmt.Errorf("invalid --env %q: must be one of %s", env, strings.Join(deployEnvs, " | "))
	}

	appKey, err := appKeyFromDSL()
	if err != nil {
		return err
	}

	client, token, err := newRepoClientFromProfile()
	if err != nil {
		return err
	}

	// CreateResource 幂等：组织/仓库不存在则创建，存在则复用，成功即可推送
	repo, err := client.CreateRepository(appKey)
	if err != nil {
		return fmt.Errorf("准备代码仓库失败: %w", err)
	}

	cloneURL := repo.CloneURLFor(env)
	if cloneURL == "" {
		return fmt.Errorf("服务端未返回 %s 环境的仓库地址", env)
	}

	fmt.Printf("%-12s %s\n", "App:", appKey)
	fmt.Printf("%-12s %s\n", "Environment:", env)
	fmt.Printf("%-12s %s\n", "Repository:", cloneURL)

	if err := gitDeployFunc(cloneURL, token, env, force); err != nil {
		return err
	}

	fmt.Printf("Deployed '%s' to %s\n", appKey, env)
	return nil
}

// appKeyFromDSL 从工程内 apps/dsl/app.yaml 读取 app key。
// app.yaml 是 app 身份的单一真相源（create 写出、apply/diff 读回），
// deploy 据此定位部署目标——目录可随意改名而部署仓库稳定，无需 --app 旋钮。
// 文件缺失给可操作错误：要么不在 app 工程根目录，要么尚未 makecli app create。
func appKeyFromDSL() (string, error) {
	if _, err := os.Stat(appDSLPath); err != nil {
		return "", fmt.Errorf("%s not found: run deploy from the app project root (or create it with `makecli app create`)", appDSLPath)
	}
	manifest, err := loadAppManifestFromFile(appDSLPath)
	if err != nil {
		return "", err
	}
	if err := validResourceKey(manifest.Key); err != nil {
		return "", fmt.Errorf("invalid app key in %s: %w", appDSLPath, err)
	}
	return manifest.Key, nil
}

// gitDeploy 用 go-git（纯 Go）把当前工作树快照并推送到 cloneURL 的部署分支。
// 语义是「快照即部署」：打开/初始化仓库 → 暂存全部改动（尊重 .gitignore）→ 有变更就自动提交 → 推送。
// 每次都提交，确保 deploy 推的永远是当前工作树，而非停在旧 HEAD（否则二次 deploy 会悄悄推旧代码）。
func gitDeploy(cloneURL, token, env string, force bool) error {
	repo, err := openOrInitRepo()
	if err != nil {
		return err
	}

	if err := snapshotWorktree(repo, env); err != nil {
		return err
	}

	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("仓库无可推送的提交: %w", err)
	}

	fmt.Printf("Pushing %s -> %s ...\n", head.Hash().String()[:7], deployBranch)
	return pushHead(repo, head, cloneURL, token, force)
}

// openOrInitRepo 打开当前目录所属的 git 仓库；不存在则就地初始化。
// DetectDotGit 让子目录里执行 deploy 也能找到仓库根（对齐 git 命令行的向上探测）。
func openOrInitRepo() (*git.Repository, error) {
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err == nil {
		return repo, nil
	}
	if errors.Is(err, git.ErrRepositoryNotExists) {
		return git.PlainInit(".", false)
	}
	return nil, fmt.Errorf("打开 git 仓库失败: %w", err)
}

// snapshotWorktree 暂存全部改动并在有变更时提交。
// AddWithOptions{All} 等价 git add -A（含新增/删除，且尊重 .gitignore）；
// 工作树干净时跳过提交，直接复用现有 HEAD——避免空提交。
func snapshotWorktree(repo *git.Repository, env string) error {
	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("读取工作树失败: %w", err)
	}
	if err := w.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("暂存改动失败: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return fmt.Errorf("读取工作树状态失败: %w", err)
	}
	if status.IsClean() {
		return nil
	}

	msg := fmt.Sprintf("Deploy to %s", env)
	if _, err := w.Commit(msg, &git.CommitOptions{Author: gitSignature(repo)}); err != nil {
		return fmt.Errorf("提交失败: %w", err)
	}
	return nil
}

// gitSignature 解析提交署名：优先用用户 git 配置(user.name/email，含全局)，
// 缺失则回退 makecli 身份——deploy 不该因为用户没配 git 身份就失败。
func gitSignature(repo *git.Repository) *object.Signature {
	name, email := "makecli", "makecli@make.local"
	if cfg, err := repo.ConfigScoped(config.SystemScope); err == nil {
		if cfg.User.Name != "" {
			name = cfg.User.Name
		}
		if cfg.User.Email != "" {
			email = cfg.User.Email
		}
	}
	return &object.Signature{Name: name, Email: email, When: time.Now()}
}

// pushHead 把 head 指向的提交推送到临时 remote 的固定部署分支。
// 用匿名 remote 承载 cloneUrl（不落 .git/config）；token 走 HTTP BasicAuth(make:<token>)；
// up-to-date（远端已是该提交）视为成功，不当错误。
func pushHead(repo *git.Repository, head *plumbing.Reference, cloneURL, token string, force bool) error {
	remote, err := repo.CreateRemoteAnonymous(&config.RemoteConfig{
		Name: anonymousRemote,
		URLs: []string{cloneURL},
	})
	if err != nil {
		return fmt.Errorf("准备推送目标失败: %w", err)
	}

	refspec := config.RefSpec(fmt.Sprintf("%s:refs/heads/%s", head.Name().String(), deployBranch))
	err = remote.Push(&git.PushOptions{
		RemoteName: anonymousRemote,
		RefSpecs:   []config.RefSpec{refspec},
		Auth:       &http.BasicAuth{Username: "make", Password: token},
		Force:      force,
		Progress:   os.Stdout,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("git push 失败: %w", err)
	}
	return nil
}
