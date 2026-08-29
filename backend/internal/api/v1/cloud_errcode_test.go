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
	rec := doCloud(t, r, http.MethodPost, "/cloud/drives", `{"name":"x","type":"baidu","token":"t"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("应 400，got %d，body=%s", rec.Code, rec.Body.String())
	}
	assertEnvelope(t, rec, "invalid_input")
}

// TestEnvelope_CloudUpdateMissing 更新不存在的网盘 → 500 + internal（chain 层不区分 not-found，
// 统一 internal 收口；底层 record not found 细节保留在 WithErr 日志链，不进响应体）。
func TestEnvelope_CloudUpdateMissing(t *testing.T) {
	r, _ := setupCloudServer(t)
	rec := doCloud(t, r, http.MethodPut, "/cloud/drives/999", `{"name":"x","type":"115","token":"t"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("应 500，got %d，body=%s", rec.Code, rec.Body.String())
	}
	assertEnvelope(t, rec, "internal")
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
