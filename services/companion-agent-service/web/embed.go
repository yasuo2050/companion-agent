// Package web 嵌入最小聊天演示页面。
package web

import "embed"

// Files 包含可由 Agent Service 直接提供的静态页面。
//
//go:embed index.html
var Files embed.FS
