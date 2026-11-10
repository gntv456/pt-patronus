// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package web 用 go:embed 把前端构建产物（dist/）打进后端二进制，
// 使 Docker 镜像成为单文件完整产品（后端 API + Web UI）。
//
// 本地开发：dist/ 为占位 index.html，前端走 vite dev server（:5173 代理 /api）。
// 生产/Docker：Dockerfile 先 npm build 前端，把 dist/ 覆盖为真实产物再 go build。
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// FS 返回以 dist 为根的嵌入文件系统。
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist 一定存在（编译期 embed 保证），不会到这里
		panic(err)
	}
	return sub
}

// Handler 返回一个 SPA 静态处理器：文件存在则直出，否则回退 index.html
// （前端路由由 vue-router 接管）。
func Handler() http.Handler {
	fsys := FS()
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/")
		if rel != "" {
			if _, err := fs.Stat(fsys, rel); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback：所有未命中文件的路径都返回 index.html
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
