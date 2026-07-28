package downloader

import "testing"

func TestCapabilities(t *testing.T) {
	// 已登记类型返回其能力位
	qb := Capabilities("qb")
	if !qb.ForceStart || !qb.Tags || len(qb.FilePrio) != 4 {
		t.Fatalf("qb caps mismatch: %+v", qb)
	}
	// tr 分类不支持（契约校验）
	if tr := Capabilities("tr"); tr.Category {
		t.Fatalf("tr should not support category")
	}
	// 未登记类型走 minimalCaps（非 qb 全开——治旧默认把未知类型当 qb 显示全部按钮的漂移）
	unk := Capabilities("unknown-client")
	if unk.ForceStart || unk.Tags || unk.Peers || !unk.Files {
		t.Fatalf("unknown type should fall back to minimal caps, got %+v", unk)
	}
}

func TestAllCapabilities(t *testing.T) {
	all := AllCapabilities()
	if len(all) < 7 {
		t.Fatalf("expected >=7 registered types, got %d", len(all))
	}
	// 返回须是副本：改不影响全局表
	all["qb"] = Caps{}
	if !Capabilities("qb").ForceStart {
		t.Fatal("mutating AllCapabilities result affected global typeCaps")
	}
}
