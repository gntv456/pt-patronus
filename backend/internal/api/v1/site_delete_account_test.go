package v1

import (
	"net/http"
	"strings"
	"testing"
)

// 破坏性删除的「成功」必须对应真实发生的删除。
// 旧实现直接 oper.Delete(id)：GORM 对未命中主键不报错，于是删一个不存在的站点
// 也回 200 {"deleted":id}，界面弹「已删除」，而站点其实还在（凭据仍被签到/抓取用）。
func TestSiteDeleteReportsMissing(t *testing.T) {
	r, db := setupSiteServer(t)

	// 1) 库里没有这条：404 site_not_found，不是 200
	rec := doSite(t, r, http.MethodDelete, "/sites/999999", `{"confirm_risk":true}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("删不存在的站点 code=%d body=%s，应为 404", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "site_not_found") {
		t.Fatalf("body=%s 应带 site_not_found", rec.Body.String())
	}

	// 2) 未确认风险仍是 400（存在性检查排在确认之后，顺序别写反）
	rec = doSite(t, r, http.MethodDelete, "/sites/999999", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 confirm_risk code=%d body=%s，应为 400", rec.Code, rec.Body.String())
	}

	// 3) 真存在的那条：200 且行数真的减少；再删同一条转 404
	if rec := doSite(t, r, http.MethodPost, "/sites", siteBody); rec.Code != http.StatusOK {
		t.Fatalf("造站点失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doSite(t, r, http.MethodDelete, "/sites/1", `{"confirm_risk":true}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deleted":1`) {
		t.Fatalf("首次删除 code=%d body=%s，应为 200 {\"deleted\":1}", rec.Code, rec.Body.String())
	}
	var count int64
	if err := db.Table("sites").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("删除后剩余 %d 行，应为 0", count)
	}
	rec = doSite(t, r, http.MethodDelete, "/sites/1", `{"confirm_risk":true}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("重复删除 code=%d body=%s，应为 404", rec.Code, rec.Body.String())
	}
}
