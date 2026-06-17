# implementation-notes — X-Log-Id + Traceparent

> 功能：给 makecli 所有出站 HTTP 请求注入 W3C `Traceparent` 与 `X-Log-Id`（X-Log-Id = traceparent 第二段 trace-id）。

## 共识决策（讨论后定稿）

1. **零依赖手写，不引 otel**
   - 评估了三档：全 otel SDK / otel-lite（仅 `otel/trace`+`propagation`）/ 零依赖手写。
   - traceparent v00 是**冻结的 W3C 格式**，otel 的"算法"本质只是 `crypto/rand` + 定长 hex 拼串，没有黑魔法。CLI 不导出 span，引 SDK 纯属仪式。
   - 选零依赖手写（`internal/trace`，~30 行），最契合 makecli「二进制自包含」。

2. **trace-id 作用域 = 每次 CLI 调用一个（per-invocation）**
   - 一条命令（如 `apply` 下发的 N 个请求）共享同一 trace-id，后端可串成同一棵 trace 树。
   - 用 `sync.OnceValue` 做进程级单一真相源，懒初始化一次、全程复用，无需跨 Client 传参。
   - `parent-id`（span 段）**每个 HTTP 请求新生成**，标识树上的节点。

## 规范外的实现选择 / 取舍

- **用户给的 gorilla/mux 链接用不上**：`otelmux` 是 server 端中间件（提取入站 traceparent），makecli 是客户端，方向相反。已向用户说明。
- **省略全零回退分支**：otel `randomIDGenerator` 会在 ID 全零时重试。全零概率 trace-id 2⁻¹²⁸ / span-id 2⁻⁶⁴，可忽略。按「能消失的分支永远比能写对的分支更优雅」刻意不写该分支。
- **注入点 = 2 个咽喉**：`internal/api/client.go:do()`（Meta/Data/Repo 全走这里）+ `integration.go:OCR()`（multipart）。两处都在原有 `c.headers` 注入旁加 2 行。
- **debug 输出同步**：trace 头在两个咽喉点都先生成一次（`traceparent, logID := ...`），同一对值既打进 `--debug` 的 curl 输出又设到真实请求，保证 debug 显示与实际发出一致。
- **OAuth 请求不注入**（`internal/oauth/*` 的 discovery/registration/token 三个请求）：属登录引导，与业务 trace 无关，刻意不挂。
- **`crypto/rand.Read` 忽略 error**（`_, _ =`）：现代 Go 该调用在受支持平台不返回错误，与 otel 同样处理；显式 `_` 满足 errcheck。

## 头部命名

- `Traceparent` / `X-Log-Id`：经 Go `http.Header.Set` 规范化；HTTP 头大小写不敏感，服务端按标准解析无碍。

## 验证

- `make vet` ✅ / `make test` ✅（新增 `internal/trace` 包测试 + api `TestTraceHeaders` 出站头断言）/ `golangci-lint run ./...` ✅ 0 issues。

## GEB 文档回环

- L3：`trace.go` / `trace_test.go` 新建带头部；`client.go` / `integration.go` / `client_test.go` 头部 INPUT/OUTPUT 已更新。
- L2：新建 `internal/trace/CLAUDE.md`；更新 `internal/api/CLAUDE.md`。
- L1：根 `CLAUDE.md` 目录树新增 `internal/trace/`，api 条目补注 trace 头注入。
