// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// legacyKVTestRT 满足 Runtime 的最小桩，KV 存内存 map（handleKV 只用 KVGet/KVSet/KVDelete）。
type legacyKVTestRT struct{ kv map[string]any }

func (r *legacyKVTestRT) Config() map[string]any                          { return nil }
func (r *legacyKVTestRT) Log(string, string, string, map[string]any)      {}
func (r *legacyKVTestRT) Notice(context.Context, NoticeInput) error       { return nil }
func (r *legacyKVTestRT) Publish(context.Context, string, map[string]any) {}
func (r *legacyKVTestRT) KVGet(key string, out any) error {
	v, ok := r.kv[key]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	if o, isPtr := out.(*any); isPtr {
		*o = v
	}
	return nil
}
func (r *legacyKVTestRT) KVSet(key string, val any) error    { r.kv[key] = val; return nil }
func (r *legacyKVTestRT) KVDelete(key string) error          { delete(r.kv, key); return nil }
func (r *legacyKVTestRT) SiteList() []SiteRef                { return nil }
func (r *legacyKVTestRT) SiteCookie(uint) (string, error)    { return "", nil }
func (r *legacyKVTestRT) SetRuntimeConfig(string, any) error { return nil }

func TestHandleKVLegacyShim(t *testing.T) {
	rt := &legacyKVTestRT{kv: map[string]any{}}
	h := &externalHostServer{plugin: &externalHTTPPlugin{manifest: Manifest{ID: "legacy-test", Permissions: []string{"kv:read", "kv:write", "kv:delete"}}}}

	// legacy 写：POST /kv/set {"key","value"} → 应落到 "mykey"（而不是字面 "set"）。
	body := `{"key":"mykey","value":{"n":1}}`
	req := httptest.NewRequest(http.MethodPost, "/kv/set", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.handleKV(rec, req, rt)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy set: code %d body %s", rec.Code, rec.Body.String())
	}
	if _, ok := rt.kv["mykey"]; !ok {
		t.Fatalf("legacy set should write key 'mykey', got keys %v", rt.kv)
	}
	if _, wrong := rt.kv["set"]; wrong {
		t.Fatal("legacy set must NOT write literal key 'set'")
	}

	// legacy 读：POST /kv/get {"key"} → 改写成 GET，返回 found/value。
	req = httptest.NewRequest(http.MethodPost, "/kv/get", strings.NewReader(`{"key":"mykey"}`))
	rec = httptest.NewRecorder()
	h.handleKV(rec, req, rt)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"found":true`) {
		t.Fatalf("legacy get: code %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"n":1`) {
		t.Errorf("legacy get should return stored value, got %s", rec.Body.String())
	}

	// 契约写法不受影响：PUT /kv/contract 直写。
	req = httptest.NewRequest(http.MethodPut, "/kv/contract", strings.NewReader(`{"value":"v2"}`))
	rec = httptest.NewRecorder()
	h.handleKV(rec, req, rt)
	if rec.Code != http.StatusOK {
		t.Fatalf("contract put: code %d", rec.Code)
	}
	if rt.kv["contract"] != "v2" {
		t.Fatalf("contract put stored %v", rt.kv["contract"])
	}

	// legacy 删除：POST /kv/delete {"key"}。
	req = httptest.NewRequest(http.MethodPost, "/kv/delete", strings.NewReader(`{"key":"mykey"}`))
	rec = httptest.NewRecorder()
	h.handleKV(rec, req, rt)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy delete: code %d", rec.Code)
	}
	if _, still := rt.kv["mykey"]; still {
		t.Fatal("legacy delete should remove key")
	}
}

// TestLegacyKVRewrite_ReservedKeys 契约路径优先：键名恰为 set/get/delete 的契约写法不受影响
// （legacy 改写只认 POST + 保留字路径；PUT/GET/DELETE + /kv/{key} 天然不命中）。
func TestLegacyKVRewrite_ReservedKeys(t *testing.T) {
	h := &externalHostServer{plugin: &externalHTTPPlugin{manifest: Manifest{ID: "legacy-test", Permissions: []string{"kv:read", "kv:write", "kv:delete"}}}}
	rt := &legacyKVTestRT{kv: map[string]any{}}

	// PUT /kv/set 是契约写法（键名就是 "set"）。
	req := httptest.NewRequest(http.MethodPut, "/kv/set", strings.NewReader(`{"value":"kept"}`))
	rec := httptest.NewRecorder()
	h.handleKV(rec, req, rt)
	if rt.kv["set"] != "kept" {
		t.Fatalf("contract PUT /kv/set should store under key 'set', got %v", rt.kv)
	}

	// POST /kv/get 仍被识别为 legacy（改写成 GET）。
	req = httptest.NewRequest(http.MethodPost, "/kv/get", strings.NewReader(`{"key":"set"}`))
	rec = httptest.NewRecorder()
	h.handleKV(rec, req, rt)
	if !strings.Contains(rec.Body.String(), `"found":true`) {
		t.Fatalf("legacy POST /kv/get should read key 'set': %s", rec.Body.String())
	}
	_ = errors.New // 保持 import 对齐
}
