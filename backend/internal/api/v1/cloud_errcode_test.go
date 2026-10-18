package v1

import (
	"net/http"
	"strings"
	"testing"
)

// PR-5 首批信封断言：cloud/transfer/plugin 透传收口后，
// 4xx/5xx 错误统一走 errcode 信封（{code, message, error}），底层细节只留日志链。
// 复用 assertEnvelope（三字段齐全 + error==message）。

// TestEnvelope_CloudInvalidType 网盘类型非法 → 400 + invalid_input（原 err.Error() 透传收口）。
func TestEnvelope_CloudInvalidType(t *testing.T) {
	r, _ := setupCloudServer(t)
	// type 不在白名单 → oper.ErrInvalidInput → InvalidInput 信封
	// （白名单与 cloud.NewClient/前端下拉一致：115/115_open/quark/123/aliyun_open/onedrive/baidu/tianyi/mobile）
	rec := doCloud(t, r, http.MethodPost, "/cloud/drives", `{"name":"x","type":"foobar","token":"t"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("应 400，got %d，body=%s", rec.Code, rec.Body.String())
	}
	assertEnvelope(t, rec, "invalid_input")
}

// TestEnvelope_CloudUpdateMissing 更新一条已不存在的网盘记录 → 404 + cloud_drive_not_found。
// 从前钉的是 500「chain 层不区分 not-found」：用户删掉云盘后另一个浏览器标签还在编辑，
// 看到的是「服务内部错误」，以为程序坏了，其实是记录已经没了。
// 不泄漏底层文本这条约束保留（响应体里不得出现 record not found）。
func TestEnvelope_CloudUpdateMissing(t *testing.T) {
	r, _ := setupCloudServer(t)
	rec := doCloud(t, r, http.MethodPut, "/cloud/drives/999", `{"name":"x","type":"115","token":"t"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("应 404，got %d，body=%s", rec.Code, rec.Body.String())
	}
	assertEnvelope(t, rec, "cloud_drive_not_found")
	if !assertNoLeak(rec.Body.String()) {
		t.Fatal("不应泄露底层错误细节")
	}
}

// assertNoLeak 确认响应体不含底层 Go 错误文本（record not found 等）。
func assertNoLeak(body string) bool {
	for _, leak := range []string{"record not found", "sql:", "gorm"} {
		if strings.Contains(body, leak) {
			return false
		}
	}
	return true
}
