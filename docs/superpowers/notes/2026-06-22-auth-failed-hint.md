# 鉴权失败错误引导 — 实现笔记

> 目标：API 返回 `990300403 / token验证失败` 时，把裸错误升级为带 next-step 的引导
> （提示用户 `makecli login`），并显示当前 profile/env 帮助排查环境串号。

## 最终输出（用户定稿）
```
error: 鉴权失败 [990300403]: token验证失败
当前 profile: default | env: production

凭证无效或已过期,请重新登陆:
    makecli login

若已登陆仍报此错,请确认 --env 与 token 颁发环境一致,
可用 makecli configure verify 自检当前凭证。
```

## 非 spec 决策记录

### 1. 990300403 语义不确定 → 文案做笼统覆盖
makecli 代码 / API 文档（AgenticDSL/Design）均无后端错误码表，`990300403` 纯后端返回。
唯一书面证据（login 设计 spec）表明 `token验证失败` 是**广义鉴权拒绝**，至少含：过期 /
无效签名 / **环境串号**（dev 身份服务器颁发的 token 打到 test 后端被拒）。
→ 故文案不写死「已过期」，给双 next-step（login + 环境自检），并回显 profile/env。
→ **待办**：找后端要鉴权码表，确认是否还有其它鉴权码（见下「识别依据」）。

### 2. 识别依据：业务码，非 HTTP 401 / 非文本匹配
后端走 HTTP 200 包 `{code,msg}`，`do()` 不看 HTTP 状态 → 拿不到 401。
只能靠业务码。当前只认 `authFailedCode = 990300403`（单一常量，后端补码表后在此扩展）。
不用 msg 文本匹配（脆弱）。

### 3. 分层：api 报「事实」，cmd 管「呈现」
- `internal/api`：新增哨兵 `ErrAuthFailed`，探测到鉴权码即 `%w` 包裹返回（含原始 code/msg）。
  复刻既有 `ErrNotFound` 哨兵模式（对称，不发明新机制）。
- `cmd`：用 `errors.Is(err, api.ErrAuthFailed)` 翻译成带 `makecli login` 的文案。
  `makecli login` / profile / env 是 CLI 知识，**不下沉进 api 包**。

### 4. 根治：错误打印收口到单一出口（用户选定范围）
**根因**：makecli 错误打印无单一出口——多数命令靠 cobra 自动打印（`error:` 前缀），
`diff`/`preflight` 各自 `SilenceErrors` + 自打印。要统一加引导，必须先建单一出口。
- `rootCmd.SilenceErrors = true`（全局），`Execute()` 出口统一 `reportExecuteError`。
- `reportExecuteError`：退出码哨兵（errDiffFound/errPreflightFailed）静默；
  `ErrAuthFailed` → 引导文案；其余 → `error: <err>`（复刻 cobra 行为）。
- **顺带消除特例**：删 diff/preflight 的局部 `SilenceErrors` + `reportDiffError`/
  `reportPreflightError` 自打印，RunE 直接返回错误，由统一出口打印。

### 5. env 名解析：新增 fail-safe `envName()`
`config.Environment` preset 只有 URL 三件套，无 Name 字段。错误展示需要环境**名**。
- 新增 `envName()`：`--env > [settings] > 默认 dev`，**吞 LoadSettings 错误回退 default**
  （纯展示，不能因配置读取失败而显示不出环境名）。
- `resolveEnvironment()` 保持 fail-loud（LoadSettings 错误上抛、未知名报错）——职责不同，
  不强行合并（合并会耦合两种错误语义）。3 行解析链的「重复」是语义分歧的必要代价。

## 测试调整
- `diff_report_test.go` → `errors_test.go`：原测 `reportDiffError`（已删），改测 `reportExecuteError`
  各分支 + `authFailedHint` 文案。`TestDiffCommandPrintsRealError` 的「真实错误不被吞」契约
  迁移到 `reportExecuteError` 单测。
- `client_test.go` / `integration_test.go`：加 990300403 → `errors.Is(err, ErrAuthFailed)`。
