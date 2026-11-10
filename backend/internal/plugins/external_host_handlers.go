// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"bytes"
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

func (h *externalHostServer) handleKV(w http.ResponseWriter, r *http.Request, rt Runtime) {
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
