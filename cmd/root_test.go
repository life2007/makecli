/**
 * [INPUT]: 依赖 cmd 包内 commandName（白盒）、github.com/spf13/cobra
 * [OUTPUT]: 覆盖 commandName 顶级命令解析的单元测试
 * [POS]: cmd 模块 root.go 的配套测试
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandName(t *testing.T) {
	root := &cobra.Command{Use: "makecli"}
	version := &cobra.Command{Use: "version"}
	version.AddCommand(&cobra.Command{Use: "list"})
	app := &cobra.Command{Use: "app"}
	app.AddCommand(&cobra.Command{Use: "create"})
	root.AddCommand(version, app, &cobra.Command{Use: "update"})

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"version"}, "version"},
		{[]string{"version", "list"}, "version"},
		{[]string{"update"}, "update"},
		{[]string{"app", "create", "foo"}, "app"},
		{[]string{}, ""},
		{[]string{"nonsense"}, ""},
	}
	for _, c := range cases {
		if got := commandName(root, c.args); got != c.want {
			t.Errorf("commandName(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}
