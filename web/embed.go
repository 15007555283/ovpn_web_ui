package web

import "embed"

// 构建时嵌入页面及依赖，部署无需 Node 或外部 CDN。
//
//go:embed all:dist
var Files embed.FS
