// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// #377 第三步（出口层）：凭据读不出来这条原因必须真的出现在聚合结果里，
// 而不是被压成「该站 0 条结果」。
//
// 第一步让 credentialWithErr 返回可判别的错误、第二步把 21 个调用方迁过去，
// 但如果出口把它吞了/换成通用话术，用户看到的仍是「搜索 0 结果」「签到失败」
// —— 与站外故障完全同形，等于前两步白做。
//
// 这一层要证的是：**每站的 Error 字段里能看到那句中文**。逐个接口去构造请求成本高，
// 这里直接驱动聚合面的内核（searchOneSite / checkinOneSite），因为它们是所有
// 聚合出口（SearchSites / CheckinAll / MedalAll / snapshot）的共同入口 ——
// 上层只是把它塞进 JSON。
//
// 判据：主密钥换过时，逐站结果的 Error 必须含「凭据」与站点名，
// 且**不含**「0 条」这类被误读成正常的说法。

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

func rotatedSearchChain(t *testing.T) (*SiteChain, *models.Site) {
	t.Helper()
	db := newChainDB(t)
	writer := oper.NewSiteOper(db, security.NewCrypto("old-key"))
	s, err := writer.Create(oper.SiteInput{
		Name: "馒头", Platform: "nexusphp", BaseURL: "https://x.test",
		Cookie: "uid=7; pass=secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	// checkinGuard 必须有值：Checkin 的第一件事就是 checkinGuard.acquire，
	// nil receiver 会 panic —— 而 checkinOneSite 的 recover 兜底会把 panic 写成
	// 「签到异常(panic)」，正好掩盖真正的凭据错误（我第一版就撞上了这个）。
	return &SiteChain{sites: oper.NewSiteOper(db, security.NewCrypto("rotated-key")), checkinGuard: newSpendGuard()}, s
}

// TestSearchOneSiteSurfacesCredentialError 搜索：凭据读不出来要出现在 Error 里。
func TestSearchOneSiteSurfacesCredentialError(t *testing.T) {
	c, s := rotatedSearchChain(t)
	r := c.searchOneSite(context.Background(), s, "关键字", "")
	if r.Error == "" {
		t.Fatal("凭据解不开时逐站结果必须有 Error（旧形状：空 Error + 0 条种子，界面读成「该站没搜到」）")
	}
	if !strings.Contains(r.Error, "凭据") {
		t.Errorf("该点明是凭据读不出来，got %q", r.Error)
	}
	if !strings.Contains(r.Error, "馒头") {
		t.Errorf("该点名站点（聚合结果按站成行，用户要知道去哪个站重存）：%q", r.Error)
	}
	if len(r.Torrents) != 0 {
		t.Errorf("失败时不该有种子，got %d", len(r.Torrents))
	}
}

// TestCheckinOneSiteSurfacesCredentialError 签到：同上。
func TestCheckinOneSiteSurfacesCredentialError(t *testing.T) {
	c, s := rotatedSearchChain(t)
	r := c.checkinOneSite(context.Background(), s)
	if r.Error == "" {
		t.Fatal("凭据解不开时签到结果必须有 Error（旧形状：与「签到成功但没奖励」同形）")
	}
	if !strings.Contains(r.Error, "凭据") || !strings.Contains(r.Error, "馒头") {
		t.Errorf("该点明是哪个站的凭据读不出来，got %q", r.Error)
	}
	if r.Success {
		t.Error("失败时不该报成功")
	}
}

// 反向划界：主密钥没换过时，这两个出口的失败模式不被这次修复影响。
func TestAggregatesUnaffectedWhenKeyIntact(t *testing.T) {
	db := newChainDB(t)
	op := oper.NewSiteOper(db, security.NewCrypto("k"))
	s, err := op.Create(oper.SiteInput{
		Name: "好站", Platform: "nexusphp", BaseURL: "https://z.test",
		Cookie: "uid=1; pass=x",
	})
	if err != nil {
		t.Fatal(err)
	}
	c := &SiteChain{sites: op, checkinGuard: newSpendGuard()}
	r := c.searchOneSite(context.Background(), s, "关键字", "")
	if strings.Contains(r.Error, "凭据") {
		t.Errorf("密钥没换时不该出现凭据读不出来（旧形状会误伤所有正常站点）：%q", r.Error)
	}
}
