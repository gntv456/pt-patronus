// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package medalwall

import (
	"context"
	"errors"
	"testing"
)

// FetchOwned 必须区分「回源失败」与「成功但为空」：失败要返回错误，chain 才能据此保留
// 旧 OWNED 快照（BatchUpsert 删后插）；静默 nil,nil 会把一次瞬时故障放大成
// 「已拥有勋章变可购买」在 DB TTL 内持续误导。
func TestNexusFetchOwnedSurfacesFailures(t *testing.T) {
	if _, err := (NexusPHPAdapter{}).FetchOwned(context.Background(), shopHTTPStub{}, Def{}, "https://tracker.test", "", ""); !errors.Is(err, ErrOwnedUnavailable) {
		t.Fatalf("uid 缺失应返回 ErrOwnedUnavailable, got %v", err)
	}
	if _, err := (NexusPHPAdapter{}).FetchOwned(context.Background(), shopHTTPStub{err: errors.New("boom")}, Def{}, "https://tracker.test", "", "42"); err == nil {
		t.Fatal("传输错误应上报而非静默空")
	}
	if _, err := (NexusPHPAdapter{}).FetchOwned(context.Background(), shopHTTPStub{resp: &Response{StatusCode: 500}}, Def{}, "https://tracker.test", "", "42"); err == nil {
		t.Fatal("HTTP 500 应上报而非静默空")
	}
	owned, err := (NexusPHPAdapter{}).FetchOwned(context.Background(),
		shopHTTPStub{resp: &Response{StatusCode: 200, Body: []byte("<html></html>")}}, Def{}, "https://tracker.test", "", "42")
	if err != nil || owned != nil {
		t.Fatalf("2xx 空结果应为 (nil, nil), got %v, %v", owned, err)
	}
}

func TestTNodeFetchOwnedSurfacesFailures(t *testing.T) {
	if _, err := (TNodeAdapter{}).FetchOwned(context.Background(), shopHTTPStub{}, Def{}, "https://tracker.test", "", ""); !errors.Is(err, ErrOwnedUnavailable) {
		t.Fatalf("uid 缺失应返回 ErrOwnedUnavailable, got %v", err)
	}
	if _, err := (TNodeAdapter{}).FetchOwned(context.Background(), shopHTTPStub{resp: &Response{StatusCode: 503}}, Def{}, "https://tracker.test", "", "42"); err == nil {
		t.Fatal("全部端点均为非 2xx 应上报而非静默空")
	}
	owned, err := (TNodeAdapter{}).FetchOwned(context.Background(),
		shopHTTPStub{resp: &Response{StatusCode: 200, Body: []byte("<html></html>")}}, Def{}, "https://tracker.test", "", "42")
	if err != nil || owned != nil {
		t.Fatalf("2xx 空结果应为 (nil, nil), got %v, %v", owned, err)
	}
}

func TestUnit3DFetchOwnedSurfacesFailures(t *testing.T) {
	if _, err := (Unit3DAdapter{}).FetchOwned(context.Background(), shopHTTPStub{}, Def{}, "https://tracker.test", "", ""); !errors.Is(err, ErrOwnedUnavailable) {
		t.Fatalf("uid/用户名缺失应返回 ErrOwnedUnavailable, got %v", err)
	}
	if _, err := (Unit3DAdapter{}).FetchOwned(context.Background(), shopHTTPStub{err: errors.New("timeout")}, Def{}, "https://tracker.test", "", "alice"); err == nil {
		t.Fatal("传输错误应上报而非静默空")
	}
	owned, err := (Unit3DAdapter{}).FetchOwned(context.Background(),
		shopHTTPStub{resp: &Response{StatusCode: 200, Body: []byte("<html></html>")}}, Def{}, "https://tracker.test", "", "alice")
	if err != nil || owned != nil {
		t.Fatalf("2xx 空结果应为 (nil, nil), got %v, %v", owned, err)
	}
}
