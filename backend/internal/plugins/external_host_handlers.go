// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

func (h *externalHostServer) handleLog(w http.ResponseWriter, r *http.Request, rt Runtime) {
	var in struct {
		Level   string         `json:"level"`
		Event   string         `json:"event"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if !decodeHostJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Level) == "" {
		in.Level = logInfo
	}
	if strings.TrimSpace(in.Event) == "" {
		in.Event = "plugin.host"
	}
	rt.Log(in.Level, in.Event, in.Message, in.Data)
	writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *externalHostServer) handleNotice(w http.ResponseWriter, r *http.Request, rt Runtime) {
	var in NoticeInput
	if !decodeHostJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Level) == "" {
		in.Level = logInfo
	}
	if err := rt.Notice(r.Context(), in); err != nil {
		writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *externalHostServer) handlePublishEvent(w http.ResponseWriter, r *http.Request, rt Runtime) {
	var in Event
	if !decodeHostJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Type) == "" {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "type is required"})
		return
	}
	rt.Publish(r.Context(), in.Type, in.Data)
	writeHostJSON(w, http.StatusOK, map[string]any{"published": true, "type": in.Type})
}

func (h *externalHostServer) handleRuntimeConfig(w http.ResponseWriter, r *http.Request, rt Runtime) {
	var in struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	if !decodeHostJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Key) == "" {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "key is required"})
		return
	}
	if err := rt.SetRuntimeConfig(in.Key, in.Value); err != nil {
		writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// legacyKVValueKey 是 legacy 改写路径在 request context 里缓存 Value 的键。
type legacyKVValueKey struct{}

func legacyKVValueFrom(r *http.Request) (any, bool) {
	v := r.Context().Value(legacyKVValueKey{})
	if v == nil {
		return nil, false
	}
	return v, true
}

// legacyKVRewrite 识别 legacy KV 调用形状（键名在 Body 里）并返回真实键名。
// 返回 (key, ok, handled)：handled=true 表示命中 legacy 形状；ok=false 表示
// Body 解析失败且响应已写出，调用方应直接返回。
// legacyKVRewrite 识别 legacy KV 调用形状（键名在 Body 里）并返回真实键名。
func (h *externalHostServer) legacyKVRewrite(
	w http.ResponseWriter, r *http.Request,
) (string, bool, bool) {
	readKey := func() (string, bool) {
		var in struct {
			Key string `json:"key"`
		}
		if !decodeHostJSON(w, r, &in) {
			return "", false
		}
		return strings.TrimSpace(in.Key), true
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/kv/set":
		var in struct {
			Key   string `json:"key"`
			Value any    `json:"value"`
		}
		if !decodeHostJSON(w, r, &in) {
			return "", true, false
		}
		// Value 缓存进 context（Body 已被消费，不能二次读）。
		ctx := context.WithValue(r.Context(), legacyKVValueKey{}, in.Value)
		*r = *r.WithContext(ctx)
		h.logLegacyKV()
		return strings.TrimSpace(in.Key), true, true
	case r.Method == http.MethodPost && r.URL.Path == "/kv/get":
		key, ok := readKey()
		if !ok {
			return "", true, false
		}
		h.logLegacyKV()
		r.Method = http.MethodGet // legacy 用 POST 携带键名；契约读取是 GET
		return key, true, true
	case r.Method == http.MethodPost && r.URL.Path == "/kv/delete":
		key, ok := readKey()
		if !ok {
			return "", true, false
		}
		h.logLegacyKV()
		r.Method = http.MethodDelete
		return key, true, true
	}
	return "", false, false
}

// logLegacyKV 每 100 次至多打 1 条 deprecation 提示（落插件日志，作者能看到）。
func (h *externalHostServer) logLegacyKV() {
	if h.legacyKVCount.Add(1)%100 != 1 {
		return
	}
	h.plugin.manager.log(h.plugin.manifest.ID, logWarning, "host.api",
		"legacy /kv/set|get|delete 已兼容，下个 api_version 移除；"+
			"请改用 PUT/GET/DELETE /kv/{key}（SDK kv_get/kv_set 即契约写法）", nil)
}

func (h *externalHostServer) handleKV(w http.ResponseWriter, r *http.Request, rt Runtime) {
	// legacy 兼容：旧式插件用 POST /kv/set {"key","value"}、POST /kv/get {"key"}、
	// POST /kv/delete {"key"}（键名在 Body 里）。契约写法是 PUT/GET/DELETE /kv/{key}
	// （SDK client.py 同形）。识别三个保留字路径，改写成契约形状并打节流 deprecation
	// 日志引导迁移；api_version "1" 内不删除。
	// 改写命中时 legacyKVRewrite 已消费 Body，这里把 Value 存入 context 供写分支复用
	//（避免二次读 Body 得 EOF）。
	if key, ok, handled := h.legacyKVRewrite(w, r); handled {
		if !ok {
			return // 改写阶段已写过错误响应
		}
		r.URL.Path = "/kv/" + key
		if v, has := legacyKVValueFrom(r); has {
			r = r.WithContext(context.WithValue(r.Context(), legacyKVValueKey{}, v))
		}
	}
	key, err := hostPathValue(r.URL.Path, "/kv/")
	if err != nil || key == "" {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "key is required"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.requirePermission(w, "kv:read") {
			return
		}
		var value any
		err := rt.KVGet(key, &value)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeHostJSON(w, http.StatusOK, map[string]any{"found": false})
			return
		}
		if err != nil {
			writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeHostJSON(w, http.StatusOK, map[string]any{"found": true, "value": value})
	case http.MethodPut, http.MethodPost:
		if !h.requirePermission(w, "kv:write") {
			return
		}
		// legacy 改写路径：Value 已在改写时读出，从 context 取，不二次读 Body。
		if v, has := legacyKVValueFrom(r); has {
			if err := rt.KVSet(key, v); err != nil {
				writeHostJSON(w, http.StatusInternalServerError,
					map[string]any{"error": err.Error()})
				return
			}
			writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		var in struct {
			Value any `json:"value"`
		}
		if !decodeHostJSON(w, r, &in) {
			return
		}
		if err := rt.KVSet(key, in.Value); err != nil {
			writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		if !h.requirePermission(w, "kv:delete") {
			return
		}
		if err := rt.KVDelete(key); err != nil {
			writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeHostJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (h *externalHostServer) handleSiteCookie(w http.ResponseWriter, r *http.Request, rt Runtime) {
	rest, err := hostPathValue(r.URL.Path, "/sites/")
	if err != nil {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid site path"})
		return
	}
	if !strings.HasSuffix(rest, "/cookie") {
		writeHostJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	idText := strings.TrimSuffix(rest, "/cookie")
	id, err := strconv.ParseUint(strings.Trim(idText, "/"), 10, 64)
	if err != nil {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid site id"})
		return
	}
	cookie, err := rt.SiteCookie(uint(id))
	if err != nil {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"cookie": cookie})
}

func hostPathValue(path, prefix string) (string, error) {
	raw := strings.TrimPrefix(path, prefix)
	value, err := url.PathUnescape(raw)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func decodeHostJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	if r.Body == nil {
		return true
	}
	defer r.Body.Close()
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(out); err != nil {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return false
	}
	return true
}

func writeHostJSON(w http.ResponseWriter, status int, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		status = http.StatusInternalServerError
		raw = []byte(`{"error":"json encode failed"}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

type pluginOutputWriter struct {
	plugin    *externalHTTPPlugin
	level     string
	eventName string
	mu        sync.Mutex
	buf       []byte
}

func (w *pluginOutputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	data := append(w.buf, p...)
	lines := bytes.SplitAfter(data, []byte("\n"))
	w.buf = w.buf[:0]
	for i, line := range lines {
		if i == len(lines)-1 && !bytes.HasSuffix(line, []byte("\n")) {
			w.buf = append(w.buf, line...)
			continue
		}
		w.logLine(line)
	}
	if len(w.buf) > 4096 {
		w.logLine(w.buf)
		w.buf = w.buf[:0]
	}
	return len(p), nil
}

func (w *pluginOutputWriter) logLine(line []byte) {
	msg := strings.TrimSpace(string(line))
	if msg == "" {
		return
	}
	if len(msg) > 4000 {
		msg = msg[:4000]
	}
	if rt := w.plugin.currentRuntime(); rt != nil {
		rt.Log(w.level, w.eventName, msg, nil)
	}
}
