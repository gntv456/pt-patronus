package chain

// 刷流选种（RunSelect）推送失败的可观测性回归。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// bfFlakyQB 假 qB：failAdds 为真时 /torrents/add 返回 500（模拟密码过期、保存路径非法、配额满）。
func bfFlakyQB(t *testing.T) (*httptest.Server, *atomic.Int64, *atomic.Bool) {
	t.Helper()
	var adds atomic.Int64
	var failAdds atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/v2/auth/login"):
			_, _ = w.Write([]byte("Ok."))
		case strings.HasSuffix(r.URL.Path, "/api/v2/sync/maindata"):
			_, _ = w.Write([]byte(`{"rid":1,"full_update":true,"torrents":{},"server_state":{}}`))
		case strings.HasSuffix(r.URL.Path, "/api/v2/torrents/add"):
			if failAdds.Load() {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			adds.Add(1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &adds, &failAdds
}

// TestRunSelectAddFailureSurfaced 候选全部推送失败时必须在任务页留诊断。
// 旧实现只 slog.Warn，末尾又无条件 ClearTaskError("select")：added=0、last_select_at 照刷、
// 上一轮错误被抹掉——用户看到「任务正常运行」，实际一条都没进下载器。
func TestRunSelectAddFailureSurfaced(t *testing.T) {
	srv, adds, fail := bfFlakyQB(t)
	svc, db, taskID, adapter, _ := newBrushSelectHarnessAt(t, srv.URL, adds)
	op := oper.NewBrushFlowOper(db)
	adapter.torrents = []site.Torrent{
		{ID: "9", Title: "cand-9", DownloadURL: "http://fakesite.local/dl/9", Size: "1 GB", Seeders: 3},
	}
	ctx := context.Background()
	fail.Store(true)

	if err := svc.RunSelect(ctx, taskID); err != nil {
		t.Fatalf("推送失败应是软错误（上抛会让 cron 记成运行失败）: %v", err)
	}
	got, err := op.GetTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalAdded != 0 || adds.Load() != 0 {
		t.Fatalf("前提：失败轮不该计数, total_added=%d adds=%d", got.TotalAdded, adds.Load())
	}
	if !strings.Contains(got.LastError, "推送下载器失败") {
		t.Errorf("推送失败必须写进任务页诊断, got last_error=%q（旧行为：只 Warn + 末尾清空）", got.LastError)
	}

	// 连续失败：第二轮不得把上一轮诊断清空。
	if err := svc.RunSelect(ctx, taskID); err != nil {
		t.Fatal(err)
	}
	got2, err := op.GetTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got2.LastError, "推送下载器失败") {
		t.Errorf("连续失败轮诊断被清掉了, got last_error=%q", got2.LastError)
	}

	// 正控：解除故障后成功一轮 → 诊断清空、计数前进。
	fail.Store(false)
	if err := svc.RunSelect(ctx, taskID); err != nil {
		t.Fatal(err)
	}
	got3, err := op.GetTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if got3.LastError != "" {
		t.Errorf("成功轮应清空同源诊断, got last_error=%q", got3.LastError)
	}
	if got3.TotalAdded != 1 || adds.Load() != 1 {
		t.Errorf("成功轮应 total_added=1/adds=1, got total_added=%d adds=%d", got3.TotalAdded, adds.Load())
	}
}
