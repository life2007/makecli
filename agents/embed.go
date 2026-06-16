/**
 * [INPUT]: 依赖 embed 包，引用同目录 CLAUDE.md.tmpl / AGENTS.md.tmpl / gitignore.tmpl 模板文件（.tmpl 后缀避免与 GEB 的 CLAUDE.md L2 约定撞名）
 * [OUTPUT]: 对外提供 Templates embed.FS，包含 app create 脚手架所需的模板文件（键名带 .tmpl，写出时去后缀）
 * [POS]: agents 模块的 embed 文件，将模板编译进二进制供 cmd/app_create 脚手架写出
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

package agents

import "embed"

//go:embed CLAUDE.md.tmpl AGENTS.md.tmpl gitignore.tmpl
var Templates embed.FS
