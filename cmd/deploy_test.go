/**
 * [INPUT]: 依赖 cmd 包内的 runDeploy / gitDeploy / gitDeployFunc（包内白盒）、enterAppDir(写 apps/dsl/app.yaml + chdir)，encoding/json、errors、fmt、net/http、net/http/httptest、os、path/filepath、strings、testing、github.com/go-git/go-git/v5（及 plumbing/object 子包）
 * [OUTPUT]: 覆盖 deploy 子命令核心逻辑的单元测试（runDeploy 编排：stub 隔离 git；gitDeploy 真 go-git：init+commit+gitignore+push 到本地裸仓库）
 * [POS]: cmd 模块 deploy.go 的配套测试，用 httptest 隔离网络、gitDeployFunc 打桩隔离文件系统/推送、临时裸仓库做本地 remote 验证真实 go-git 行为
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// deployCall 打桩 gitDeployFunc：记录 runDeploy 传入的参数，按 err 返回。
type deployCall struct {
	cloneURL string
	token    string
	env      string
	force    bool
	called   bool
	err      error
}

func (d *deployCall) install(t *testing.T) {
	t.Helper()
	old := gitDeployFunc
	gitDeployFunc = func(cloneURL, token, env string, force bool) error {
		d.called = true
		d.cloneURL, d.token, d.env, d.force = cloneURL, token, env, force
		return d.err
	}
	t.Cleanup(func() { gitDeployFunc = old })
}

// enterAppDir 切到一个含 apps/dsl/app.yaml（key=<key>）的临时工程根目录。
// deploy 的 app 身份取自 DSL 文件而非目录名——临时目录名是随机的，证明 key 来自 app.yaml。
func enterAppDir(t *testing.T, key string) {
	t.Helper()
	dir := t.TempDir()
	chdir(t, dir)
	dslDir := filepath.Join(dir, "apps", "dsl")
	if err := os.MkdirAll(dslDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("key: %s\nname: %s\ntype: Make.App\nmeta:\n  version: 1.0.0\nproperties: {}\n", key, key)
	writeTestFile(t, filepath.Join(dslDir, "app.yaml"), []byte(content))
}

// newMockRepoServer 启动返回双环境仓库响应的代码仓库服务 mock
func newMockRepoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200, "msg": "repositories are ready",
			"data": map[string]any{
				"appKey": "myapp", "type": "Make.Code.Repository",
				"properties": map[string]any{
					"env": map[string]any{
						"preview":    map[string]any{"repository": map[string]any{"cloneUrl": "https://repo.example/org/myapp-preview.git"}},
						"production": map[string]any{"repository": map[string]any{"cloneUrl": "https://repo.example/org/myapp-production.git"}},
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// setupDeployEnv 准备工程目录(app.yaml key=myapp) + 凭证 + repo server 指向，返回安装好的 gitDeploy 桩
func setupDeployEnv(t *testing.T) *deployCall {
	t.Helper()
	enterAppDir(t, "myapp")
	t.Setenv("HOME", t.TempDir())
	saveDefaultToken(t)
	RepoServerURL = newMockRepoServer(t).URL
	t.Cleanup(func() { RepoServerURL = "" })
	d := &deployCall{}
	d.install(t)
	return d
}

// ---------------------------------- runDeploy 编排（stub 隔离 git） ----------------------------------

func TestRunDeploy(t *testing.T) {
	t.Run("deploys to preview", func(t *testing.T) {
		d := setupDeployEnv(t)

		out := captureStdout(t, func() {
			if err := runDeploy("preview", false); err != nil {
				t.Errorf("runDeploy: %v", err)
			}
		})

		if d.cloneURL != "https://repo.example/org/myapp-preview.git" {
			t.Errorf("clone url = %q, want preview repo", d.cloneURL)
		}
		if d.env != "preview" || d.force {
			t.Errorf("env=%q force=%v, want preview/false", d.env, d.force)
		}
		if d.token == "" {
			t.Error("token should not be empty")
		}
		if !strings.Contains(out, "Deployed 'myapp' to preview") {
			t.Errorf("output missing success line: %q", out)
		}
	})

	t.Run("passes production env and force", func(t *testing.T) {
		d := setupDeployEnv(t)

		_ = captureStdout(t, func() {
			if err := runDeploy("production", true); err != nil {
				t.Errorf("runDeploy: %v", err)
			}
		})

		if d.cloneURL != "https://repo.example/org/myapp-production.git" {
			t.Errorf("clone url = %q, want production repo", d.cloneURL)
		}
		if d.env != "production" || !d.force {
			t.Errorf("env=%q force=%v, want production/true", d.env, d.force)
		}
	})

	t.Run("reads app key from app.yaml", func(t *testing.T) {
		// 工程目录名是随机临时名，部署 key 取自 app.yaml 的 fromdsl
		enterAppDir(t, "fromdsl")
		t.Setenv("HOME", t.TempDir())
		saveDefaultToken(t)
		RepoServerURL = newMockRepoServer(t).URL
		t.Cleanup(func() { RepoServerURL = "" })
		d := &deployCall{}
		d.install(t)

		out := captureStdout(t, func() {
			if err := runDeploy("preview", false); err != nil {
				t.Errorf("runDeploy: %v", err)
			}
		})

		if !d.called {
			t.Error("expected gitDeploy to be called")
		}
		if !strings.Contains(out, "Deployed 'fromdsl' to preview") {
			t.Errorf("expected app key from app.yaml in output, got: %q", out)
		}
	})

	t.Run("rejects invalid env", func(t *testing.T) {
		d := setupDeployEnv(t)

		if err := runDeploy("staging", false); err == nil {
			t.Fatal("expected error for invalid env")
		}
		if d.called {
			t.Error("gitDeploy should not run on invalid env")
		}
	})

	t.Run("fails when app.yaml missing", func(t *testing.T) {
		chdir(t, t.TempDir()) // 干净目录，无 apps/dsl/app.yaml

		if err := runDeploy("preview", false); err == nil {
			t.Fatal("expected error when app.yaml is missing")
		}
	})

	t.Run("fails when app.yaml has invalid key", func(t *testing.T) {
		enterAppDir(t, "_bad") // 下划线开头，validResourceKey 拒绝

		if err := runDeploy("preview", false); err == nil {
			t.Fatal("expected error for invalid key in app.yaml")
		}
	})

	t.Run("fails without credentials", func(t *testing.T) {
		enterAppDir(t, "myapp")
		t.Setenv("HOME", t.TempDir())
		d := &deployCall{}
		d.install(t)

		if err := runDeploy("preview", false); err == nil {
			t.Fatal("expected error for missing credentials")
		}
		if d.called {
			t.Error("gitDeploy should not run without credentials")
		}
	})

	t.Run("fails on repository API error", func(t *testing.T) {
		enterAppDir(t, "myapp")
		t.Setenv("HOME", t.TempDir())
		saveDefaultToken(t)
		srv := newMockMeta(t, 500, "repository could not be prepared")
		t.Cleanup(srv.Close)
		RepoServerURL = srv.URL
		t.Cleanup(func() { RepoServerURL = "" })
		(&deployCall{}).install(t)

		if err := runDeploy("preview", false); err == nil {
			t.Fatal("expected error on API failure")
		}
	})

	t.Run("fails when env clone url missing", func(t *testing.T) {
		enterAppDir(t, "myapp")
		t.Setenv("HOME", t.TempDir())
		saveDefaultToken(t)
		srv := newMockMeta(t, 200, "ok") // data 为空 → 无 cloneUrl
		t.Cleanup(srv.Close)
		RepoServerURL = srv.URL
		t.Cleanup(func() { RepoServerURL = "" })
		(&deployCall{}).install(t)

		if err := runDeploy("preview", false); err == nil {
			t.Fatal("expected error when clone url missing")
		}
	})

	t.Run("propagates git deploy error", func(t *testing.T) {
		d := setupDeployEnv(t)
		d.err = errors.New("push rejected")

		var err error
		_ = captureStdout(t, func() { err = runDeploy("preview", false) })
		if err == nil {
			t.Fatal("expected gitDeploy error to propagate")
		}
	})
}

// ---------------------------------- gitDeploy 真实 go-git（本地裸仓库做 remote） ----------------------------------

func TestGitDeploy(t *testing.T) {
	t.Run("inits, commits and pushes to dev branch", func(t *testing.T) {
		work := t.TempDir()
		chdir(t, work)
		writeTestFile(t, filepath.Join(work, "code.txt"), []byte("v1"))
		bare := newBareRemote(t)

		if err := gitDeploy(bare, "", "preview", false); err != nil {
			t.Fatalf("gitDeploy: %v", err)
		}

		tree := devTree(t, bare)
		if _, err := tree.File("code.txt"); err != nil {
			t.Errorf("code.txt not pushed to dev: %v", err)
		}
	})

	t.Run("respects .gitignore", func(t *testing.T) {
		work := t.TempDir()
		chdir(t, work)
		writeTestFile(t, filepath.Join(work, ".gitignore"), []byte("secret.txt\nnode_modules/\n"))
		writeTestFile(t, filepath.Join(work, "keep.txt"), []byte("keep"))
		writeTestFile(t, filepath.Join(work, "secret.txt"), []byte("token=abc"))
		if err := os.MkdirAll(filepath.Join(work, "node_modules", "pkg"), 0755); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(work, "node_modules", "pkg", "index.js"), []byte("junk"))
		bare := newBareRemote(t)

		if err := gitDeploy(bare, "", "preview", false); err != nil {
			t.Fatalf("gitDeploy: %v", err)
		}

		tree := devTree(t, bare)
		if _, err := tree.File("keep.txt"); err != nil {
			t.Errorf("keep.txt should be pushed: %v", err)
		}
		if _, err := tree.File("secret.txt"); !errors.Is(err, object.ErrFileNotFound) {
			t.Errorf("secret.txt should be gitignored, got err=%v", err)
		}
		if _, err := tree.File("node_modules/pkg/index.js"); !errors.Is(err, object.ErrFileNotFound) {
			t.Errorf("node_modules should be gitignored, got err=%v", err)
		}
	})

	t.Run("redeploy pushes the new working-tree state", func(t *testing.T) {
		work := t.TempDir()
		chdir(t, work)
		codePath := filepath.Join(work, "code.txt")
		writeTestFile(t, codePath, []byte("v1"))
		bare := newBareRemote(t)

		if err := gitDeploy(bare, "", "preview", false); err != nil {
			t.Fatalf("first gitDeploy: %v", err)
		}
		writeTestFile(t, codePath, []byte("v2")) // 编辑后重新部署
		if err := gitDeploy(bare, "", "preview", false); err != nil {
			t.Fatalf("second gitDeploy: %v", err)
		}

		f, err := devTree(t, bare).File("code.txt")
		if err != nil {
			t.Fatalf("code.txt missing on dev: %v", err)
		}
		content, err := f.Contents()
		if err != nil {
			t.Fatal(err)
		}
		if content != "v2" {
			t.Errorf("dev has %q, want v2 (redeploy must push current tree, not stale HEAD)", content)
		}
	})

	t.Run("clean redeploy is a no-op success", func(t *testing.T) {
		work := t.TempDir()
		chdir(t, work)
		writeTestFile(t, filepath.Join(work, "code.txt"), []byte("v1"))
		bare := newBareRemote(t)

		if err := gitDeploy(bare, "", "preview", false); err != nil {
			t.Fatalf("first gitDeploy: %v", err)
		}
		// 无任何改动，再次部署应成功（远端已是该提交 → up-to-date）
		if err := gitDeploy(bare, "", "preview", false); err != nil {
			t.Errorf("clean redeploy should succeed, got: %v", err)
		}
	})
}

// newBareRemote 建一个临时裸仓库作为本地 push 目标（file transport，无需网络）
func newBareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatal(err)
	}
	return dir
}

// devTree 取裸仓库 deployBranch 分支最新提交的文件树
func devTree(t *testing.T, bareDir string) *object.Tree {
	t.Helper()
	r, err := git.PlainOpen(bareDir)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := r.Reference(plumbing.NewBranchReferenceName(deployBranch), true)
	if err != nil {
		t.Fatalf("dev branch missing on remote: %v", err)
	}
	c, err := r.CommitObject(ref.Hash())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := c.Tree()
	if err != nil {
		t.Fatal(err)
	}
	return tree
}
