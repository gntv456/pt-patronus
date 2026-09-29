package v1

import (
	"net/http"
	"strings"
	"testing"
)

// TestBrush_UpdateKeepsStats 编辑任务不得重置服务端统计字段（total_added/last_error 等）。
func TestBrush_UpdateKeepsStats(t *testing.T) {
	r, db := setupBrushServer(t)
	doBrush(t, r, http.MethodPost, "/brush/tasks", `{"name":"t1","site_id":1,"downloader_id":1,"max_seeds":3}`)
	if err := db.Exec("UPDATE brush_flow_tasks SET total_added = 42, last_error = 'boom' WHERE id = 1").Error; err != nil {
		t.Fatal(err)
	}
	rec := doBrush(t, r, http.MethodPut, "/brush/tasks/1", `{"name":"t2","site_id":1,"downloader_id":1,"enabled":false,"total_added":0}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update failed: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"total_added":0`) || !strings.Contains(rec.Body.String(), `"total_added":42`) {
		t.Fatalf("total_added 应保留统计值 42: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"last_error":"boom"`) {
		t.Fatalf("last_error 应保留: %s", rec.Body.String())
	}
	var got int64
	db.Raw("SELECT total_added FROM brush_flow_tasks WHERE id = 1").Scan(&got)
	if got != 42 {
		t.Fatalf("DB total_added 被重置: %d", got)
	}
}
