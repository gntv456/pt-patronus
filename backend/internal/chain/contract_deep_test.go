// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestEvalBreachesBoundary(t *testing.T) {
	// 恰好等于下限 → 不违例（严格 <）
	r := &models.ContractRule{MinRatio: 1.0, MinSeeding: 10, MinUploadGB: 5}
	ui := &site.UserInfo{Ratio: 1.0, Seeding: 10, Uploaded: int64(5) << 30}
	if b := evalBreaches(r, ui, seedingBase{Count: ui.Seeding}); len(b) != 0 {
		t.Errorf("恰好等于下限不应违例, got %v", b)
	}

	// 全 0 规则 → 不检（任何 UserInfo/base 都达标）
	if b := evalBreaches(&models.ContractRule{}, &site.UserInfo{}, seedingBase{}); len(b) != 0 {
		t.Errorf("全0规则不应违例, got %v", b)
	}

	// 三项全低于 → 3 条违例
	r2 := &models.ContractRule{MinRatio: 2.0, MinSeeding: 100, MinUploadGB: 100}
	ui2 := &site.UserInfo{Ratio: 0.5, Seeding: 3, Uploaded: 1 << 30}
	if b := evalBreaches(r2, ui2, seedingBase{Count: ui2.Seeding}); len(b) != 3 {
		t.Errorf("应 3 项违例, got %d: %v", len(b), b)
	}

	// 仅上传量违例
	r3 := &models.ContractRule{MinRatio: 1.0, MinSeeding: 5, MinUploadGB: 100}
	ui3 := &site.UserInfo{Ratio: 1.5, Seeding: 10, Uploaded: 1 << 30}
	b := evalBreaches(r3, ui3, seedingBase{Count: ui3.Seeding})
	if len(b) != 1 || !strings.Contains(b[0], "上传") {
		t.Errorf("应仅上传违例, got %v", b)
	}
}

// 上传量下限精确边界（字节级，恰好等于不违例）。
func TestEvalBreachesUploadThreshold(t *testing.T) {
	r := &models.ContractRule{MinUploadGB: 10}
	if b := evalBreaches(r, &site.UserInfo{Uploaded: int64(10) << 30}, seedingBase{}); len(b) != 0 {
		t.Errorf("恰好 10GiB 不应违例, got %v", b)
	}
	if b := evalBreaches(r, &site.UserInfo{Uploaded: int64(10)<<30 - 1}, seedingBase{}); len(b) != 1 {
		t.Errorf("差 1 字节应违例, got %d", len(b))
	}
}
