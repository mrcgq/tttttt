package api

import "embed"

// WebUIFS 嵌入 Web UI 静态文件
// 编译时将 index.html 放在 pkg/api/webui/ 目录下
//
//go:embed webui/*
var WebUIFS embed.FS
