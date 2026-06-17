/**
 * [INPUT]: 依赖 regexp、testing；被测对象为同包 TraceID / Traceparent
 * [OUTPUT]: 覆盖 trace-id 稳定性与格式、traceparent 格式、trace-id 跨请求复用、parent-id 每次新生成
 * [POS]: internal/trace 的单元测试，守护 W3C v00 格式与"每次调用一个 trace-id"语义
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package trace

import (
	"regexp"
	"testing"
)

// traceparentRe 锚定 W3C v00 格式: 00-{32hex}-{16hex}-01
var (
	traceparentRe = regexp.MustCompile(`^00-([0-9a-f]{32})-([0-9a-f]{16})-01$`)
	traceIDRe     = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

func TestTraceIDStableAndFormat(t *testing.T) {
	a, b := TraceID(), TraceID()
	if a != b {
		t.Fatalf("TraceID 应进程内稳定: %q != %q", a, b)
	}
	if !traceIDRe.MatchString(a) {
		t.Fatalf("TraceID 应为 32 个 hex 字符: %q", a)
	}
}

func TestTraceparentFormatAndStitch(t *testing.T) {
	m1 := traceparentRe.FindStringSubmatch(Traceparent())
	if m1 == nil {
		t.Fatal("traceparent 不符合 W3C v00 格式")
	}
	// trace-id 段必须等于 TraceID()，X-Log-Id 才能与 traceparent 关联
	if m1[1] != TraceID() {
		t.Fatalf("traceparent trace-id 段 %q != TraceID() %q", m1[1], TraceID())
	}

	m2 := traceparentRe.FindStringSubmatch(Traceparent())
	if m2 == nil {
		t.Fatal("traceparent 不符合 W3C v00 格式")
	}
	if m1[1] != m2[1] {
		t.Fatalf("trace-id 应跨请求稳定: %q != %q", m1[1], m2[1])
	}
	if m1[2] == m2[2] {
		t.Fatalf("parent-id 应每次新生成, 两次却同为 %q", m1[2])
	}
}
