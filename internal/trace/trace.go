/**
 * [INPUT]: 依赖 crypto/rand、encoding/hex、sync
 * [OUTPUT]: 对外提供 TraceID() / Traceparent() —— W3C Trace Context 出站头生成
 * [POS]: internal/trace 唯一成员，进程级 trace-id 的单一真相源；被 internal/api 的请求咽喉点消费
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package trace

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// ---------------------------------- W3C Trace Context ----------------------------------
//
// traceparent 格式（W3C v00，格式已冻结，故零依赖手写而非引 otel）:
//
//	00-{trace-id:32hex}-{parent-id:16hex}-{flags:2hex}
//
// 设计取舍：trace-id 进程内全局唯一且稳定（每次 CLI 调用一个），让一条命令
// 下发的所有请求在后端被串成同一棵 trace 树；parent-id 每个请求新生成，标识
// 树上的一个节点。X-Log-Id 即 trace-id 段，供日志关联检索。

// traceID 是进程级 trace-id，懒初始化一次：首个出站请求触发，此后全程复用。
// sync.OnceValue 让"每次 CLI 调用一个 trace-id"成为单一真相源，无需跨 Client 传参。
var traceID = sync.OnceValue(func() string { return randHex(16) })

// TraceID 返回本次 CLI 调用稳定的 trace-id（即 X-Log-Id 的值，32 个 hex 字符）。
func TraceID() string { return traceID() }

// Traceparent 返回 W3C traceparent：复用稳定的 trace-id，每次新生成 parent-id，
// flags 固定 01（sampled）。
func Traceparent() string {
	return "00-" + traceID() + "-" + randHex(8) + "-01"
}

// randHex 读取 n 字节 crypto/rand 并 hex 编码为 2n 个字符。
// 与 otel randomIDGenerator 同源（crypto/rand）；全零概率 2^-8n 可忽略，故不设回退分支。
func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
