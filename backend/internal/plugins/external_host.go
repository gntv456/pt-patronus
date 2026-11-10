// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
)

type externalHostServer struct {
	plugin  *externalHTTPPlugin
	server  *http.Server
	baseURL string
	token   string
}

func newExternalHostServer(plugin *externalHTTPPlugin) (*externalHostServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token, err := randomToken()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	h := &externalHostServer{
		plugin:  plugin,
		baseURL: "http://" + ln.Addr().String(),
		token:   token,
	}
	// 回调服务仅 loopback，但仍施加读超时，避免恶意/卡死插件慢速读取占用连接。
	h.server = &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second, // 没有写超时，慢速/卡死插件可让 host goroutine 长期占用；与读超时对称
	}
	safego.Go("external:656", func() {
		// Serve 总会返回非 nil 错误；ErrServerClosed 为正常关闭，其余异常因 host 回调仅 loopback 可达而无法外暴。
		_ = h.server.Serve(ln)
	})
	return h, nil
}

func (h *externalHostServer) stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return h.server.Shutdown(ctx)
}

func (h *externalHostServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" {
		writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if !h.authorized(r) {
		writeHostJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	rt := h.plugin.currentRuntime()
	if rt == nil {
		writeHostJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "plugin runtime unavailable"})
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/capabilities":
		writeHostJSON(w, http.StatusOK, map[string]any{
			"api_version":       CurrentPluginAPIVersion,
			"plugin_id":         h.plugin.manifest.ID,
			"permissions":       h.plugin.manifest.Permissions,
			"host_capabilities": supportedHostCapabilities(),
			"endpoints": []string{
				"GET /capabilities",
				"GET /config",
				"POST /runtime/config",
				"POST /log",
				"POST /notice",
				"POST /events",
				"GET /kv/{key}",
				"PUT /kv/{key}",
				"DELETE /kv/{key}",
				"GET /sites",
				"GET /sites/{id}/cookie",
				"POST /media/search",
				"GET /tools",
				"POST /tools/{name}",
			},
		})
	case r.Method == http.MethodGet && r.URL.Path == "/config":
		if !h.requirePermission(w, "config:read") {
			return
		}
		writeHostJSON(w, http.StatusOK, map[string]any{"config": rt.Config()})
	case r.Method == http.MethodPost && r.URL.Path == "/runtime/config":
		if !h.requirePermission(w, "config:write") {
			return
		}
		h.handleRuntimeConfig(w, r, rt)
	case r.Method == http.MethodPost && r.URL.Path == "/log":
		if !h.requirePermission(w, "log:write") {
			return
		}
		h.handleLog(w, r, rt)
	case r.Method == http.MethodPost && r.URL.Path == "/notice":
		if !h.requirePermission(w, "notice:write") {
			return
		}
		h.handleNotice(w, r, rt)
	case r.Method == http.MethodPost && r.URL.Path == "/events":
		if !h.requirePermission(w, "event:publish") {
			return
		}
		h.handlePublishEvent(w, r, rt)
	case strings.HasPrefix(r.URL.Path, "/kv/"):
		h.handleKV(w, r, rt)
	case r.Method == http.MethodGet && r.URL.Path == "/sites":
		if !h.requirePermission(w, "site:read") {
			return
		}
		writeHostJSON(w, http.StatusOK, map[string]any{"sites": rt.SiteList()})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/sites/"):
		if !h.requirePermission(w, "site:cookie") {
			return
		}
		h.handleSiteCookie(w, r, rt)
	case r.Method == http.MethodPost && r.URL.Path == "/media/search":
		if !h.requirePermission(w, "media:search") {
			return
		}
		h.handleMediaSearch(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/tools":
		h.handleToolsList(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/tools/"):
		h.handleToolCall(w, r)
	default:
		writeHostJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func (h *externalHostServer) handleMediaSearch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Keyword string `json:"keyword"`
		Year    int    `json:"year"`
	}
	// 与 decodeHostJSON 一致施加 1MB 上限，防恶意/失控插件 body 撑爆内存。
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
		return
	}
	results, err := h.plugin.manager.MediaSearch(r.Context(), strings.TrimSpace(body.Keyword), body.Year)
	if err != nil {
		writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"results": results})
}

// handleToolsList 返回当前插件权限可见的宿主工具清单（GET /tools）。
// 自描述：每条含 name/description/input_schema/permission/category/destructive，
// 让插件能动态发现宿主能力而无需查文档。未注入 registry 时返回空数组。
func (h *externalHostServer) handleToolsList(w http.ResponseWriter, r *http.Request) {
	reg := h.plugin.manager.HostTools()
	if reg == nil {
		writeHostJSON(w, http.StatusOK, map[string]any{"tools": []HostTool{}})
		return
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"tools": reg.VisibleTools(h.plugin.manifest.Permissions)})
}

// handleToolCall 执行一个宿主工具（POST /tools/{name}）。
// body: {"input": {...}}；权限/超时/未找到统一经 registry.Dispatch 处理。
// 写操作（:write/:action/:send）成功落审计日志（plugin_logs 表），失败落 warning。
func (h *externalHostServer) handleToolCall(w http.ResponseWriter, r *http.Request) {
	name, err := hostPathValue(r.URL.Path, "/tools/")
	if err != nil || name == "" {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "tool name is required"})
		return
	}
	reg := h.plugin.manager.HostTools()
	if reg == nil {
		writeHostJSON(w, http.StatusNotFound, map[string]any{"error": "tool not found", "tool": name})
		return
	}
	var in struct {
		Input map[string]any `json:"input"`
	}
	if !decodeHostJSON(w, r, &in) {
		return
	}
	pluginID := h.plugin.manifest.ID
	out, perm, notFound, derr := reg.Dispatch(r.Context(), h.plugin.manifest.Permissions, name, in.Input)
	switch {
	case notFound:
		writeHostJSON(w, http.StatusNotFound, map[string]any{"error": "tool not found", "tool": name})
	case derr == ErrPermissionDenied:
		writeHostJSON(w, http.StatusForbidden, map[string]any{"error": "permission denied", "permission": perm, "tool": name})
	case derr != nil:
		h.plugin.manager.log(pluginID, logWarning, "host.tool", fmt.Sprintf("%s failed: %v", name, derr), map[string]any{"input": in.Input})
		writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": derr.Error(), "tool": name})
	default:
		// 写操作审计（成功）：read 类跳过避免噪音。
		if isWritePermission(perm) {
			h.plugin.manager.log(pluginID, logInfo, "host.tool", "tool called: "+name, map[string]any{"input": in.Input})
		}
		raw, merr := json.Marshal(out) // json.RawMessage 透传；struct/map 正常序列化
		if merr != nil {
			writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": "result encode failed"})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	}
}

// isWritePermission 粗判是否为写操作（用于审计日志降噪）。
// 域级 :write、站点 :action、通知 :send 视为有副作用；其余（含空权限/只读）不计审计。
func isWritePermission(perm string) bool {
	if perm == "" {
		return false
	}
	return strings.HasSuffix(perm, ":write") || strings.HasSuffix(perm, ":action") || perm == "notify:send"
}

func (h *externalHostServer) authorized(r *http.Request) bool {
	token := strings.TrimSpace(h.token)
	if token == "" {
		return true
	}
	if r.Header.Get("Authorization") == "Bearer "+token {
		return true
	}
	return r.Header.Get("X-PTP-Host-Token") == token
}

func (h *externalHostServer) requirePermission(w http.ResponseWriter, required string) bool {
	if hasPluginPermission(h.plugin.manifest.Permissions, required) {
		return true
	}
	writeHostJSON(w, http.StatusForbidden, map[string]any{
		"error":      "permission denied",
		"permission": required,
	})
	return false
}

func hasPluginPermission(perms []string, required string) bool {
	required = strings.TrimSpace(required)
	for _, perm := range perms {
		perm = strings.TrimSpace(perm)
		switch perm {
		case "*", "host:*", "plugin:*", required:
			return true
		}
		if strings.HasSuffix(perm, ":*") {
			prefix := strings.TrimSuffix(perm, "*")
			if strings.HasPrefix(required, prefix) {
				return true
			}
		}
	}
	return false
}
