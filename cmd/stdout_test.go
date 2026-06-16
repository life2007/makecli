/**
 * [INPUT]: 依赖 os、bytes、io、testing；对全局变量 Profile 的引用
 * [OUTPUT]: 对外提供 captureStdout / captureStderr / setProfile 三个测试辅助函数
 * [POS]: cmd 模块的测试基础设施，被各子命令测试文件复用
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stdout = writer

	outputC := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		outputC <- buf.String()
	}()

	fn()

	_ = writer.Close()
	os.Stdout = originalStdout
	output := <-outputC
	_ = reader.Close()

	return output
}

// captureStderr 劫持 os.Stderr 捕获 fn 期间写入的内容（用于断言降级警告）
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	originalStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stderr = writer

	outputC := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		outputC <- buf.String()
	}()

	fn()

	_ = writer.Close()
	os.Stderr = originalStderr
	output := <-outputC
	_ = reader.Close()

	return output
}

// setProfile 在测试期间临时覆盖全局 Profile，结束自动还原。
// 替代旧的「runXxx(... "nonexistent" ...)」风格——profile 不再走参数。
func setProfile(t *testing.T, name string) {
	t.Helper()
	old := Profile
	Profile = name
	t.Cleanup(func() { Profile = old })
}
