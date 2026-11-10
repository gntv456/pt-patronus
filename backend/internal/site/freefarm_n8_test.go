// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

// freefarm n8 改版 fixture 验证：testdata/freefarm_n8_index.html 为登录态
// index.php 静态 HTML（2026-11-08 实抓）。验证 n8-cell 新选择器与 parse_number
// 万/亿后缀乘算。

import (
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestFreefarmN8UserInfo(t *testing.T) {
	def, ok := mustRegistry(t).Definition("freefarm")
	if !ok {
		t.Fatal("freefarm definition missing")
	}
	data, err := os.ReadFile("testdata/freefarm_n8_index.html")
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	doc, err2 := goquery.NewDocumentFromReader(strings.NewReader(string(data)))
	if err2 != nil {
		t.Fatal(err2)
	}

	u := ParseUserInfo(doc, def)
	// 实测样本（gntv 登录态）：上传 111.45GB / 下载 99.22GB / 分享率 1.12 /
	// 魔力值 102.2万（=1022000）/ 当前活动 ↑14 ↓0
	t.Logf("parsed: uploaded=%d downloaded=%d ratio=%.2f bonus=%.0f seeding=%d leeching=%d",
		u.Uploaded, u.Downloaded, u.Ratio, u.Bonus, u.Seeding, u.Leeching)
	if u.Uploaded>>30 < 100 || u.Uploaded>>30 > 120 {
		t.Errorf("uploaded = %d bytes, want ~111.45GiB", u.Uploaded)
	}
	if u.Downloaded>>30 < 95 || u.Downloaded>>30 > 103 {
		t.Errorf("downloaded = %d bytes, want ~99.22GiB", u.Downloaded)
	}
	if u.Ratio < 1.0 || u.Ratio > 1.3 {
		t.Errorf("ratio = %v, want ~1.12", u.Ratio)
	}
	if u.Bonus != 1022000 {
		t.Errorf("bonus = %v, want 1022000（102.2万 ×1e4，parse_number 中文数量级）", u.Bonus)
	}
	if u.Seeding != 14 {
		t.Errorf("seeding = %d, want 14", u.Seeding)
	}
	if u.Leeching != 0 {
		t.Errorf("leeching = %d, want 0", u.Leeching)
	}
	if u.ID != "10725" {
		t.Errorf("id = %q, want 10725", u.ID)
	}
	if u.Name == "" {
		t.Error("name 为空（n8-hero 链接文本未取到）")
	}
}
