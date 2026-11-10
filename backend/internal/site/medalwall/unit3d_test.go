// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package medalwall

import (
	"context"
	"testing"
)

// TestParseUnit3DJSON JSON API 响应解析（递归找 name+cost 对象，货币 BP）。
func TestParseUnit3DJSON(t *testing.T) {
	body := []byte(`{"data":[{"id":12,"name":"上传达人","cost":500,"image":"/m/12.png","stock":10},{"id":13,"title":"分享之星","cost":800}]}`)
	items := parseUnit3DJSON(body, "https://unit3d.test")
	if len(items) != 2 {
		t.Fatalf("items: got %d want 2", len(items))
	}
	if items[0].Name != "上传达人" || items[0].Price != 500 || items[0].Currency != "BP" {
		t.Errorf("item0: name=%q price=%v currency=%q", items[0].Name, items[0].Price, items[0].Currency)
	}
	if items[0].ImageSmall != "https://unit3d.test/m/12.png" {
		t.Errorf("item0 image not absolute: %q", items[0].ImageSmall)
	}
	if items[0].Stock != 10 {
		t.Errorf("item0 stock: got %d want 10", items[0].Stock)
	}
	if items[1].Name != "分享之星" {
		t.Errorf("item1 name: got %q want 分享之星", items[1].Name)
	}
}

// TestParseUnit3DJSON_NoCost 无 cost 的对象不产出（防误识非勋章条目）。
func TestParseUnit3DJSON_NoCost(t *testing.T) {
	body := []byte(`{"data":[{"name":"无价格勋章","description":"x"}]}`)
	if items := parseUnit3DJSON(body, "https://u.test"); len(items) != 0 {
		t.Errorf("no-cost item should be skipped, got %d", len(items))
	}
}

// TestParseUnit3DHTML Bootstrap 卡片解析。
func TestParseUnit3DHTML(t *testing.T) {
	html := `<html><body>
<div class="bonus-card"><img src="/b/1.png"/><span class="card-title">上传勋章</span><span class="bp">300</span></div>
<div class="bonus-card"><span class="card-title">魔力勋章</span><span class="bp">600</span></div>
<div class="bonus-card"><span class="card-title">无价格</span></div>
</body></html>`
	items := parseUnit3DHTML(html, "https://u.test")
	if len(items) != 2 {
		t.Fatalf("items: got %d want 2 (no-price card skipped)", len(items))
	}
	if items[0].Name != "上传勋章" || items[0].Price != 300 || items[0].Currency != "BP" {
		t.Errorf("item0: name=%q price=%v currency=%q", items[0].Name, items[0].Price, items[0].Currency)
	}
}

// TestTNodeAdapter TNode 平台标识 + 支持开关。
func TestTNodeAdapter(t *testing.T) {
	var a Adapter = TNodeAdapter{}
	if !a.Supported() {
		t.Error("TNode should be supported")
	}
	if a.Platform() != "tnode" {
		t.Errorf("platform: got %q want tnode", a.Platform())
	}
}

// TestUnit3DAdapter Unit3D 平台标识 + Wear 不支持。
func TestUnit3DAdapter(t *testing.T) {
	var a Adapter = Unit3DAdapter{}
	if !a.Supported() {
		t.Error("Unit3D should be supported")
	}
	if a.Platform() != "unit3d" {
		t.Errorf("platform: got %q want unit3d", a.Platform())
	}
	r, _ := a.Wear(context.Background(), nil, Def{}, "", "", "")
	if r.Success {
		t.Error("Unit3D Wear should be unsupported")
	}
}
