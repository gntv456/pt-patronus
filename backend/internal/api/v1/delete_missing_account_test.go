package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 删除类端点的「不存在」分支：GORM 的 Delete 对未命中主键不报错，
// 早期各域 handler 直接把它当成功回 200 {"deleted":id}，界面弹「已删除」
// 而记录其实还在（下载器/凭据/订阅仍在被后台任务使用）。
// 现由 requireExists 一处收口，这里逐域验它真的接上了每个 DELETE 端点。
func TestDeleteMissingReportsNotFound(t *testing.T) {
	cases := []struct {
		name string
		path string
		code string
	}{
		{"file-storages", "/file-storages/999999", "file_storage_not_found"},
		{"monitor", "/monitor/targets/999999", "monitor_target_not_found"},
		{"rss", "/rss/999999", "rss_subscription_not_found"},
		{"subscribe", "/subscribe/999999", "subscription_not_found"},
		{"cloud", "/cloud/drives/999999", "cloud_drive_not_found"},
		{"cloudRule", "/cloud/rules/999999", "strm_rule_not_found"},
		{"transfer", "/transfer/history/999999", "transfer_history_not_found"},
		{"alert", "/monitor/alerts/999999", "monitor_alert_not_found"},
		{"agentConv", "/agent/conversations/999999", "agent_conversation_not_found"},
		{"cloudRecord", "/cloud/records/999999", "strm_record_not_found"},
		{"brushTask", "/brushflow/999999", "brushflow_task_not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var r *gin.Engine
			switch tc.name {
			case "file-storages":
				r, _, _ = setupFileServer(t)
			case "monitor":
				r, _ = setupMonitorServer(t)
			case "rss":
				r, _ = setupRssServer(t)
			case "subscribe":
				r, _ = setupSubscribeServer(t)
			case "cloud", "cloudRule", "cloudRecord":
				r, _ = setupCloudServer(t)
			case "transfer":
				r, _ = setupTransferServer(t)
			case "alert":
				r, _ = setupMonitorServer(t)
			case "brushTask":
				r, _ = setupBrushServer(t)
			case "agentConv":
				r = setupAgentChatServer(t)
			}
			req, _ := http.NewRequest(http.MethodDelete, tc.path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			body := rec.Body.String()
			if rec.Code == http.StatusOK && strings.Contains(body, `"deleted":999999`) {
				t.Fatalf("%s 把「本来就没有」报成删除成功：%s", tc.name, body)
			}
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s code=%d body=%s，应为 404", tc.name, rec.Code, body)
			}
			if !strings.Contains(body, tc.code) {
				t.Fatalf("%s body=%s 应带本域码 %s", tc.name, body, tc.code)
			}
		})
	}
}
