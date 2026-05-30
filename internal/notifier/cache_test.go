/**
 * [INPUT]: 依赖 notifier 包内 cacheData / readCache / writeCache / expired（白盒）
 * [OUTPUT]: 覆盖缓存原子读写与过期判定的单元测试
 * [POS]: internal/notifier 模块 cache.go 的配套测试
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package notifier

import (
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	t.Setenv("MAKE_CLI_CONFIG_DIR", t.TempDir())

	in := cacheData{
		CheckedAt:     time.Now().UTC().Truncate(time.Second),
		LatestVersion: "v1.2.3",
		HTMLURL:       "https://example.com/x",
	}
	if err := writeCache(in); err != nil {
		t.Fatalf("writeCache: %v", err)
	}
	out, err := readCache()
	if err != nil {
		t.Fatalf("readCache: %v", err)
	}
	if out.LatestVersion != in.LatestVersion || out.HTMLURL != in.HTMLURL {
		t.Errorf("roundtrip mismatch: got %+v want %+v", out, in)
	}
	if !out.CheckedAt.Equal(in.CheckedAt) {
		t.Errorf("CheckedAt mismatch: got %v want %v", out.CheckedAt, in.CheckedAt)
	}
}

func TestReadCache_Missing(t *testing.T) {
	t.Setenv("MAKE_CLI_CONFIG_DIR", t.TempDir())
	c, err := readCache()
	if err != nil {
		t.Fatalf("expected no error for missing cache, got %v", err)
	}
	if c.LatestVersion != "" {
		t.Errorf("expected zero cache, got %+v", c)
	}
}

func TestExpired(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	fresh := cacheData{CheckedAt: now.Add(-1 * time.Hour)}
	stale := cacheData{CheckedAt: now.Add(-25 * time.Hour)}
	zero := cacheData{}

	if fresh.expired(24*time.Hour, now) {
		t.Error("1h-old cache should be fresh under 24h interval")
	}
	if !stale.expired(24*time.Hour, now) {
		t.Error("25h-old cache should be expired under 24h interval")
	}
	if !zero.expired(24*time.Hour, now) {
		t.Error("zero cache should be expired")
	}
}
