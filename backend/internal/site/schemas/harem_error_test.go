// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// 邀请库存抓取失败必须把错误交回去。从前这里返回「空 status + nil 错误」，
// 原因塞进 InviteStatus.Reason——而该字段全仓无人读：链层据此判定成功，
// 后宫页于是把「站点压根没抓到」渲染成「剩余 0、权限 ✗、无被邀请人」，
// 还把这次失败当成功结果写进 30min 缓存。
func TestNexusPHPInviteStatusPropagatesFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	n := NewNexusPHP(hc)
	st, err := n.FetchInviteStatus(context.Background(), &site.SiteMetadata{}, srv.URL, "uid=1; passkey=abc")
	if err == nil {
		t.Fatalf("5xx 抓取失败必须交回错误，got status=%+v err=%v", st, err)
	}
	if st != nil {
		t.Errorf("失败时不得返回一个看似可用的空 status，got %+v", st)
	}
}
