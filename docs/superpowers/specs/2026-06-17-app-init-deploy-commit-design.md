# Spec: `app init` 回归 + commit 责任反转

- **日期**: 2026-06-17
- **状态**: 已定稿（待实现）
- **影响命令**: `app init`（新增）、`app create`、`app deploy`
- **新增模块**: `cmd/git.go`（共享 go-git 原语）

---

## 1. 动机与本质

现状 `app deploy` 是「快照即部署」——`snapshotWorktree` 自动 `git add -A` + 自动 commit，推当前工作树。这把 git 的提交时机从用户手里夺走，HEAD 与「用户意图的提交点」之间产生歧义。

本次反转把 commit 责任交还用户：

- **deploy 退化为纯 push HEAD**：不再自动 commit，工作树脏 / 无 `.git` / 无 commit 一律报错并给可操作提示。
- **`app init` 回归**：负责 `git init` 幂等 + `.gitignore` 增量补齐，是 git 仓库形态的单一入口。
- **`app create` 串联 init**：create 末尾调用 init 内核，再做一次「initial scaffold」commit，使 create 产物即干净、可立即 deploy。

哲学：deploy 只做一件事——把**已提交**的状态推上去。提交点由用户决定，部署不制造隐式历史。

---

> **2026-06-17 修订**：`app init` 的职责从「仅 git + .gitignore」扩展为「完整本地项目脚手架」。
> `app init` 与 `app create` **共享同一脚手架内核**：`app create` = `app init` 本地核心 + 远端注册 + initial commit。
> 连带：`create` 去掉旧 `assertScaffoldClear` 硬拒，本地写改为幂等 skip-if-exists，使 `init`→`create` 可组合。下文 §2 / §3 已按此更新。

## 2. `app init`（新增 · cmd/app_init.go）

### 2.1 命令契约

```
makecli app init [appKey]
```

- 可选位置参数 `[appKey]`（兼目录名，`deriveAppKey` 推导：`shop`→./shop key=shop，缺省 `.`→cwd 名），`validResourceKey` 把关。
- 写出**完整本地项目骨架**：`CLAUDE.md` / `AGENTS.md` / `apps/dsl/app.yaml`（key 取推导值，name 回退 key，无 description）+ `git init` + `.gitignore`。
- 全程**幂等 skip-if-exists**：已存在文件原样保留（不覆盖用户编辑），已是仓库跳过 init，`.gitignore` 已全则不改。
- 刻意**不 commit**（提交时机交还用户），**不碰远端**（远端是 create 的事）。

### 2.1b 与 create 的共享内核

`runAppInit(target)` 复用 `deriveAppKey` / `newAppManifest` / `writeScaffold`（app_create.go）+ `initGitRepo` / `ensureGitignore`（git.go），逐项打印 created/exists · initialized/already · updated/complete 状态行。`runAppCreate` 调同一组函数，额外加 `CreateApp`（远端先行）+ `stageAndCommit` + `prepareCodeRepos`。`writeScaffold` 改为返回新建文件清单并 skip-if-exists。

### 2.2 内核（两个可复用函数）

```go
// initGitRepo 在 dir 就地建立 git 仓库；已是仓库根则跳过。
// 用 PlainOpen（不 DetectDotGit）——只问「这个目录自身是不是仓库根」，
// 不探测父仓库（app 目录应是独立仓库根）。
func initGitRepo(dir string) (created bool, err error)

// ensureGitignore 增量补齐 dir/.gitignore 的期望 ignore 条目。
func ensureGitignore(dir string) (changed bool, err error)
```

- `initGitRepo`：`git.PlainOpen(dir)` 成功 → `created=false`；`ErrRepositoryNotExists` → `git.PlainInit(dir, false)` → `created=true`；其他错误上抛。
- 输出（人类可读）：
  - `git: initialized` 或 `git: already a repository`
  - `.gitignore: added N entries` 或 `.gitignore: already complete`

### 2.3 `.gitignore` 单一真相源

`agents/gitignore.tmpl` 是**唯一**的「期望 ignore 内容」来源，不另起硬编码清单。

`ensureGitignore` 逻辑：

1. 解析模板中**有意义的行**（trim 后非空、非 `#` 注释）为期望条目集合，例如 `node_modules/`、`dist/`、`build/`、`.next/`、`out/`、`.env`、`.env.*`、`!.env.example`、`*.log`、`.DS_Store`、`.idea/`、`.vscode/`。
2. `dir/.gitignore` **不存在** → 原样写出模板全文（含注释分组），`changed=true`。
3. **已存在** → 读取现有行（trim 后建 set），逐条期望条目做 exact-match 检测；缺失的追加到文件尾，前置一行 `# added by makecli` 标记（仅当确有追加时写标记）。保留用户已有自定义行原样不动。无缺失则 `changed=false`，不触碰文件。

---

## 3. `app create` 改动（cmd/app_create.go）

### 3.1 新执行序

```
deriveAppKey → newAppManifest → newClientFromProfile
→ CreateApp(远端，先行——先于任何本地写)
→ writeScaffold                // 幂等 skip-if-exists：CLAUDE.md / AGENTS.md / apps/dsl/app.yaml（不含 .gitignore）
→ scaffoldGit(folder)          // initGitRepo + ensureGitignore + stageAndCommit("Initial scaffold for <key>")
→ prepareCodeRepos
```

### 3.2 去掉 `assertScaffoldClear`，改幂等 + 可组合

- `app create` 与 `app init` 共享 `writeScaffold`；为支持「先 init 本地、再 create 补远端」，`writeScaffold` 改为**幂等 skip-if-exists** 并返回新建文件清单。
- 删除旧 `assertScaffoldClear`「文件已存在即拒绝」硬护栏——已存在文件原样保留（不覆盖用户编辑）。
- 仍靠**远端先行**保证零残留：`CreateApp` 在任何本地写之前，坏 token / 冲突时**新目录**不落任何文件、重跑干净。
- `.gitignore` 不在 `scaffoldTemplates`，由 `scaffoldGit` 的 `ensureGitignore` 写出（git init 之后、commit 之前）。

### 3.3 initial commit（create 独有）

- `scaffoldGit` 在 init + ensureGitignore 后调用 `stageAndCommit(repo, "Initial scaffold for <appKey>")`，把全部脚手架文件纳入首个提交。
- 提交署名走修正后的 `gitSignature`（见 §5）。

### 3.4 失败处置：降级为 stderr 警告

git init / commit 失败**不**让 create 报全败——与 `prepareCodeRepos` 同档处理：远端 App 已建、本地脚手架已写，git 没起来属于可单独补救（`makecli app init` + 手动 commit）的局部问题。

- create 仍打印 `App 'X' created successfully`。
- git 失败时额外 `fmt.Fprintf(os.Stderr, "warning: git not initialized: %v (run 'makecli app init')\n", err)`。

---

## 4. `app deploy` 反转（cmd/deploy.go · 核心）

### 4.1 新执行序

```
env 校验 → appKeyFromDSL
→ openRepo()                    // 本地：PlainOpenWithOptions{DetectDotGit:true}，不再 init
→ assertDeployable(repo)        // 本地：有 HEAD + 工作树干净，否则可操作报错
   ↑ 以上全在网络调用之前（fail-fast）
→ newRepoClientFromProfile
→ CreateRepository(网络，幂等) → cloneURL
→ pushHead(repo, head, cloneURL, token, force)   // 不变
```

### 4.2 三种报错（可操作提示）

| 情况 | 检测 | 报错文案（要点） |
|---|---|---|
| 无 `.git` | `openRepo` 返回 `ErrRepositoryNotExists` | `no git repository; run 'makecli app init' first` |
| 零 commit（无 HEAD） | `repo.Head()` 报错 | `nothing committed yet; commit first: git add -A && git commit -m ...` |
| 工作树脏 | `worktree.Status()` 非空（含未跟踪，尊重 `.gitignore`） | `working tree has uncommitted changes; commit before deploy` + 列出脏文件（`status.String()` 风格） |

### 4.3 删除 `snapshotWorktree`

- `snapshotWorktree`（自动 add+commit）整个删除。
- 其「暂存全部 + 有变更才提交」逻辑搬进 `cmd/git.go` 的 `stageAndCommit`，供 create 复用（单一真相源）。
- `openOrInitRepo` 拆分：deploy 用「只开不 init」的 `openRepo`；`PlainInit` 能力下沉到 `initGitRepo`。

### 4.4 fail-fast 与测试 seam（需测试改造）

本地校验前置到 `CreateRepository` 网络之前，会动到 `deploy_test.go` 现有 `gitDeployFunc` 打桩结构（那些桩测试的临时目录没有真 `.git`）。处置：

- 把本地校验做成独立可打桩 seam（或在编排测试里用真 go-git 建仓库），保持「网络隔离」与「FS 隔离」两类测试各自清晰。
- **这是测试改造，不改变产品行为。**

---

## 5. ❗gitSignature 修正（cmd/git.go）

现状 `gitSignature` 读 `config.SystemScope`（= `/etc/gitconfig`），**读不到**用户全局 `~/.gitconfig`，注释「含全局」是错的——实际几乎永远 fallback 到 `makecli` 身份。

commit 现由用户掌控（create 的 initial commit + 用户自己的提交），署名正确性变重要。

**修正**：`config.SystemScope` → `config.LocalScope`。go-git 的 `LocalScope` 返回 system+global+local 合并视图，等价 `git commit` 看到的身份链。修正后真实 git 身份生效，缺失才回退 `makecli`。

---

## 6. 共享层 cmd/git.go（新增）

抽出三命令共用的 go-git 原语，消除 deploy.go 独占 git 逻辑的耦合：

```go
func initGitRepo(dir string) (created bool, err error)   // §2.2
func ensureGitignore(dir string) (changed bool, err error) // §2.3
func openRepo() (*git.Repository, error)                 // 只开不 init（DetectDotGit）
func gitSignature(repo *git.Repository) *object.Signature // 从 deploy.go 迁入 + §5 修正
func stageAndCommit(repo *git.Repository, msg string) (committed bool, err error) // add -A + 有变更才 commit
func assertDeployable(repo *git.Repository) error         // 有 HEAD + 工作树干净
func pushHead(...) error                                  // 从 deploy.go 迁入（或留 deploy.go，按内聚度定）
```

文件归属原则：`git.go` 持「与具体命令无关的 go-git 原语」；`pushHead` 含部署分支/匿名 remote 约定，更内聚于 deploy.go，可留原处——实现期按依赖收敛度定夺，不在 spec 强约束。

---

## 7. 落地清单

### 新增
- `cmd/app_init.go` — `newAppInitCmd` + `runAppInit`
- `cmd/app_init_test.go`
- `cmd/git.go` — 共享 go-git 原语
- `cmd/git_test.go`（按需，覆盖 `ensureGitignore` / `stageAndCommit` / `assertDeployable`）

### 修改
- `cmd/app.go` — `newAppCmd` 挂 `newAppInitCmd`（L3 头部已预写 init，回到同构）
- `cmd/app_create.go` — 去 `.gitignore`（scaffoldTemplates）、删 `assertScaffoldClear`、`writeScaffold` 改幂等 skip-if-exists 返回新建清单（+ `scaffoldOutputs`/`fileExists`）、加 `scaffoldGit`（init+ensureGitignore+commit）、失败降级警告
- `cmd/deploy.go` — 删 `snapshotWorktree`，`openOrInitRepo`→`openRepo`，加 `assertDeployable` 前置，`gitSignature` 迁出
- `cmd/app_create_test.go` — 适配新执行序（git 仓库 + initial commit 断言）
- `cmd/deploy_test.go` — 适配 fail-fast 与脏/无仓库/无 commit 报错路径

### 文档（GEB 回环）
- `cmd/CLAUDE.md` — 新成员 app_init.go / git.go；改 app_create.go / deploy.go 职责描述
- `agents/CLAUDE.md` — `gitignore.tmpl` 角色：从「create 写出」改为「app init 的 ignore 清单单一真相源」
- 根 `CLAUDE.md` — deploy 的「快照即部署」描述改为「纯 push 已提交状态」

---

## 8. 验证项（实现期）

1. **`Status()` 性能/准确性**：在含大 `node_modules`（被 ignore）的真实 app 目录实测脏检查，确认既快又准（go-git `.gitignore` 嵌套解析有已知毛刺）。
2. **`gitSignature` LocalScope**：在配了 `~/.gitconfig` 的环境验证 create 的 initial commit 署名为真实用户身份。
3. **门控验证**：`make vet && make test` + `golangci-lint run ./...` exit 0 才提交（项目纪律，血泪教训）。

---

## 9. 非目标（YAGNI）

- 不加 `app init --git`/`--no-git` 之类旋钮——init 行为单一。
- 不让 `app init` 做 commit（用户掌控提交点）。
- 不在 deploy 提供「自动 commit」回退开关——反转是彻底的，不留双语义。
- 不做不相关重构（仅抽 git.go 这一项服务于本目标的结构改进）。
