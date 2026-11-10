// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// newEmbyScanFake 起一个 Emby/JF 假服务器：列出给定库，并对 fail 集合里的库让 Refresh 返回 500。
// 同时计数「实际收到几次 Refresh」，用来正控 scanned 是否等于真实扫成次数。
func newEmbyScanFake(t *testing.T, libsJSON string, fail map[string]bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var scans atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/Library/MediaFolders":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(libsJSON))
		case strings.HasPrefix(r.URL.Path, "/Items/") && strings.Contains(r.URL.Path, "/Refresh"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/Items/"), "/Refresh")
			scans.Add(1)
			if fail[id] {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &scans
}

// TestLibraryScanAllCountsRealScans 回归：工具与接口描述都写「empty library_id = scan all」，
// 但链层没有这条分支 —— 直接 LibraryScan(id, "") 会拼出 /Items//Refresh 的无效请求，
// 而回执写死 scanned:1，把「什么都没扫成」渲染成「扫好了一个库」。
func TestLibraryScanAllCountsRealScans(t *testing.T) {
	const twoLibs = `{"Items":[{"Name":"电影","Id":"L1","CollectionType":"movies"},{"Name":"剧集","Id":"L2","CollectionType":"tvshows"}]}`
	cases := []struct {
		name      string
		libs      string
		fail      map[string]bool
		wantScan  int
		wantErrIn string
		wantCalls int
	}{
		{name: "全部成功", libs: twoLibs, wantScan: 2, wantCalls: 2},
		{name: "部分失败仍报真实个数", libs: twoLibs, fail: map[string]bool{"L2": true}, wantScan: 1, wantCalls: 2},
		{name: "全部失败是错误", libs: twoLibs, fail: map[string]bool{"L1": true, "L2": true}, wantScan: 0, wantErrIn: "全部扫描失败", wantCalls: 2},
		{name: "没有库不是成功", libs: `{"Items":[]}`, wantScan: 0, wantErrIn: "没有可扫描的库", wantCalls: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, calls := newEmbyScanFake(t, tc.libs, tc.fail)
			ch, op, _, _ := newMediaServerTestChain(t)
			m, err := op.Create(oper.MediaServerInput{Name: "emby", Type: "emby", URL: srv.URL, Token: "t"})
			if err != nil {
				t.Fatalf("create media server: %v", err)
			}
			got, err := ch.LibraryScanAll(context.Background(), m.ID)
			switch {
			case tc.wantErrIn != "" && err == nil:
				t.Fatalf("want error containing %q, got nil（scanned=%d）", tc.wantErrIn, got)
			case tc.wantErrIn != "" && !strings.Contains(err.Error(), tc.wantErrIn):
				t.Fatalf("error = %v, want containing %q", err, tc.wantErrIn)
			case tc.wantErrIn == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantScan {
				t.Errorf("scanned = %d, want %d（假服务器实际收到 %d 次 Refresh）", got, tc.wantScan, calls.Load())
			}
			if calls.Load() != int32(tc.wantCalls) {
				t.Errorf("Refresh 调用次数 = %d, want %d", calls.Load(), tc.wantCalls)
			}
		})
	}
}
