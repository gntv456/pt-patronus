package chain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestRunSelectAndSweepRejectDisabledTask 手动触发一条「未启用」的刷流任务必须失败。
//
// 修复前两处都 `if !task.Enabled { return nil }`：HTTP 面于是回 {ok:true}，
// 界面 toast「已触发」，而 cron 也不会跑它 —— 用户以为这批种子在被选取/扫删，
// 实际一个都不会动。工具面（agent/plugin）当初是在调用方包里补的拒绝，链层这个
// 根因一直留着；收回链层后所有调用方（HTTP/工具/未来入口）共用同一判据。
func TestRunSelectAndSweepRejectDisabledTask(t *testing.T) {
	svc, id, rec := newBrushSweepHarness(t, map[string]map[string]any{}, 1)

	task, err := svc.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.Enabled = false
	if err := svc.op.UpdateTask(task); err != nil {
		t.Fatalf("disable task: %v", err)
	}

	if err := svc.RunSweep(context.Background(), id); !errors.Is(err, ErrTaskDisabled) {
		t.Fatalf("未启用任务的手动扫删应报 ErrTaskDisabled，got %v", err)
	}
	if err := svc.RunSelect(context.Background(), id); !errors.Is(err, ErrTaskDisabled) {
		t.Fatalf("未启用任务的手动选种应报 ErrTaskDisabled，got %v", err)
	}
	if len(rec.deleted) != 0 {
		t.Fatalf("未启用却发生了删种：%v", rec.deleted)
	}
	// 错误文案要说明「为什么没跑」，否则用户只会看到一句失败。
	err = svc.RunSweep(context.Background(), id)
	if !errors.Is(err, ErrTaskDisabled) {
		t.Fatalf("重复调用应仍然拒绝，got %v", err)
	}
	if !strings.Contains(err.Error(), "未启用") {
		t.Fatalf("文案需点明未启用，got %v", err)
	}
}
