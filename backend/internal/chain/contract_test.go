package chain

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestEvalBreaches(t *testing.T) {
	ui := &site.UserInfo{Ratio: 1.2, Seeding: 5, Uploaded: 2 << 30} // 分享率1.2 / 做种5 / 上传2GiB

	// 全达标
	r := &models.ContractRule{MinRatio: 1.0, MinSeeding: 3, MinUploadGB: 1}
	if b := evalBreaches(r, ui); len(b) != 0 {
		t.Errorf("全达标应无违例, got %v", b)
	}

	// 仅分享率不达标
	b := evalBreaches(&models.ContractRule{MinRatio: 1.5}, ui)
	if len(b) != 1 {
		t.Fatalf("分享率违例应 1 条, got %v", b)
	}

	// 三项全不达标
	b = evalBreaches(&models.ContractRule{MinRatio: 2, MinSeeding: 10, MinUploadGB: 10}, ui)
	if len(b) != 3 {
		t.Fatalf("三项违例应 3 条, got %v", b)
	}

	// 全 0 = 永不违例
	if b := evalBreaches(&models.ContractRule{}, ui); len(b) != 0 {
		t.Errorf("零规则应无违例, got %v", b)
	}

	// 上传量边界：正好等于下限算达标（< 才违例）
	eq := &site.UserInfo{Uploaded: 1 << 30}
	if b := evalBreaches(&models.ContractRule{MinUploadGB: 1}, eq); len(b) != 0 {
		t.Errorf("上传等于下限应达标, got %v", b)
	}
}
