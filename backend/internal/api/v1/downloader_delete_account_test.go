// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"strings"
	"testing"
)

// 「删不存在的下载器」不能报成功：GORM 的 Delete 对未命中主键不报错，
// 旧实现一律 200 {"deleted":id}，界面弹「已删除」——那一条其实根本没动过（或压根不存在）。
func TestDownloaderDeleteReportsMissing(t *testing.T) {
	r, db := setupDownloaderServer(t)

	// 1) 库里一条都没有：必须 404 downloader_not_found，而不是 200
	rec := doDL(t, r, http.MethodDelete, "/downloaders/999999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("删不存在的下载器 code=%d body=%s，应为 404", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "downloader_not_found") {
		t.Fatalf("body=%s 应带 downloader_not_found", rec.Body.String())
	}

	// 2) 真存在的那条：删除回 200，第二次删同一条要转 404（证明第一次是真删，而不是恒 200）
	// 下载器类型的取值就是后端列出的那几个短码（qb/rt/tr/dl/ut/ar/fl）。
	created := doDL(t, r, http.MethodPost, "/downloaders",
		`{"name":"本地 qB","type":"qb","url":"http://127.0.0.1:58888","username":"u","password":"p"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("造数据失败 code=%d body=%s", created.Code, created.Body.String())
	}
	first := doDL(t, r, http.MethodDelete, "/downloaders/1", "")
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"deleted":1`) {
		t.Fatalf("首次删除 code=%d body=%s，应为 200 {\"deleted\":1}", first.Code, first.Body.String())
	}
	second := doDL(t, r, http.MethodDelete, "/downloaders/1", "")
	if second.Code != http.StatusNotFound {
		t.Fatalf("重复删除 code=%d body=%s，应为 404", second.Code, second.Body.String())
	}
	var left int64
	if err := db.Table("downloaders").Count(&left).Error; err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("首次删除没真的删掉：还剩 %d 条", left)
	}
}
