// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestEvalBreaches(t *testing.T) {
	ui := &site.UserInfo{Ratio: 1.2, Seeding: 5, Uploaded: 2 << 30} // 分享率1.2 / 做种5 / 上传2GiB
	base := seedingBase{Count: ui.Seeding}

	// 全达标
	r := &models.ContractRule{MinRatio: 1.0, MinSeeding: 3, MinUploadGB: 1}
	if b := evalBreaches(r, ui, base); len(b) != 0 {
		t.Errorf("全达标应无违例, got %v", b)
	}

	// 仅分享率不达标
	b := evalBreaches(&models.ContractRule{MinRatio: 1.5}, ui, base)
	if len(b) != 1 {
		t.Fatalf("分享率违例应 1 条, got %v", b)
	}

	// 三项全不达标
	b = evalBreaches(&models.ContractRule{MinRatio: 2, MinSeeding: 10, MinUploadGB: 10}, ui, base)
	if len(b) != 3 {
		t.Fatalf("三项违例应 3 条, got %v", b)
	}

	// 全 0 = 永不违例
	if b := evalBreaches(&models.ContractRule{}, ui, base); len(b) != 0 {
		t.Errorf("零规则应无违例, got %v", b)
	}

	// 上传量边界：正好等于下限算达标（< 才违例）
	eq := &site.UserInfo{Uploaded: 1 << 30}
	if b := evalBreaches(&models.ContractRule{MinUploadGB: 1}, eq, seedingBase{}); len(b) != 0 {
		t.Errorf("上传等于下限应达标, got %v", b)
	}
}

// 官种维度：OfficialOnly 时做种基数切到官种子集。
func TestEvalBreachesOfficialBase(t *testing.T) {
	ui := &site.UserInfo{Ratio: 2.0, Seeding: 100, Uploaded: 1 << 40}
	// 全部做种 100，官种仅 3；契约要求做种 ≥5 → 用官种基数时违例
	b := evalBreaches(&models.ContractRule{MinSeeding: 5}, ui, seedingBase{Count: 3})
	if len(b) != 1 {
		t.Fatalf("官种基数 3<5 应 1 条违例, got %v", b)
	}
	// 全量基数 100 ≥5 → 不违例
	if b := evalBreaches(&models.ContractRule{MinSeeding: 5}, ui, seedingBase{Count: 100}); len(b) != 0 {
		t.Errorf("全量基数 100≥5 应不违例, got %v", b)
	}
}

// 体积下限边界（GiB，字节级比较）。
func TestEvalBreachesSeedingSize(t *testing.T) {
	ui := &site.UserInfo{}
	// 恰好等于下限 → 不违例
	if b := evalBreaches(&models.ContractRule{MinSeedingSizeGB: 10}, ui, seedingBase{Size: int64(10) << 30}); len(b) != 0 {
		t.Errorf("做种体积恰好 10GiB 应达标, got %v", b)
	}
	// 差 1 字节 → 违例
	if b := evalBreaches(&models.ContractRule{MinSeedingSizeGB: 10}, ui, seedingBase{Size: int64(10)<<30 - 1}); len(b) != 1 {
		t.Errorf("做种体积差 1 字节应违例, got %d", len(b))
	}
}

// contractPeriodKey 周期去重 key：周/月边界。
func TestContractPeriodKey(t *testing.T) {
	// Period="" → 当日
	k := contractPeriodKey(&models.ContractRule{}, time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC))
	if k != "2026-07-14" {
		t.Errorf("daily key = %q", k)
	}
	// monthly 无 start → 年月
	k = contractPeriodKey(&models.ContractRule{Period: "monthly"}, time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC))
	if k != "2026-07" {
		t.Errorf("monthly key = %q", k)
	}
	// weekly + start → 序号（同一周内不变，跨周变化）
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	r := &models.ContractRule{Period: "weekly", StartTime: &start}
	k1 := contractPeriodKey(r, time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC))
	k2 := contractPeriodKey(r, time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC))
	if k1 != k2 {
		t.Errorf("同周应共享 key: %q vs %q", k1, k2)
	}
	k3 := contractPeriodKey(r, time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC))
	if k1 == k3 {
		t.Errorf("跨周 key 应不同: %q", k3)
	}
}
