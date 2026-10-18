package oper

import (
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/monitor"
)

// TestSnapshotListAccount 趋势图的账面口径：
// ① limit 归一化只有一份（handler 回显生效值），留空取默认、超上限取上限而不是退回默认；
// ② 列表被截断时 CountSnapshots 必须给出真实总数——否则「最近 120 次采集」在图上
//
//	被画成「这台设备的全部历史」（同辅种候选 500 条、NAS 告警 200 条那一族）。
func TestSnapshotListAccount(t *testing.T) {
	if got := NormalizeSnapshotLimit(0); got != SnapshotListDefault {
		t.Errorf("留空应取默认 %d，got %d", SnapshotListDefault, got)
	}
	if got := NormalizeSnapshotLimit(-5); got != SnapshotListDefault {
		t.Errorf("非法值应取默认 %d，got %d", SnapshotListDefault, got)
	}
	if got := NormalizeSnapshotLimit(999999); got != SnapshotListLimit {
		t.Errorf("超上限应取上限 %d（而不是退回默认），got %d", SnapshotListLimit, got)
	}
	if got := NormalizeSnapshotLimit(120); got != 120 {
		t.Errorf("正常值不得被改动，got %d", got)
	}

	db := newMonitorDB(t)
	op := NewMonitorOper(db, security.NewCrypto("snapshot-account-seed"))
	target, err := op.Create(TargetInput{Name: "nas-trend", Type: "host", Protocol: "local", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		snap := &models.MetricSnapshot{
			TargetID: target.ID,
			SnapAt:   base.Add(time.Duration(i) * time.Minute),
			Online:   true,
			Metrics:  monitor.MetricsJSON(monitor.Metrics{Online: true, CPUUsage: float64(i)}),
		}
		if err := db.Create(snap).Error; err != nil {
			t.Fatal(err)
		}
	}

	total, err := op.CountSnapshots(target.ID, time.Time{})
	if err != nil {
		t.Fatalf("CountSnapshots: %v", err)
	}
	if total != 5 {
		t.Fatalf("总数应为 5，got %d", total)
	}

	// 取 2 条时清单短于总数：truncated 判据的原料必须成立。
	snaps, err := op.ListSnapshots(target.ID, time.Time{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("limit=2 应只回 2 条，got %d", len(snaps))
	}
	if !(total > int64(len(snaps))) {
		t.Error("被截断时 total 必须大于已返回条数")
	}
	// 最新的一条必须在结果里（DESC 取最近 N 再反转，不是最旧 N 条）。
	if snaps[len(snaps)-1].SnapAt.Before(snaps[0].SnapAt) {
		t.Error("清单应按时间升序，末项为最新")
	}

	// 时间窗：只数窗口内的，否则「近 1 小时」的图会拿全量总数谎报被截断。
	late, err := op.CountSnapshots(target.ID, base.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if late != 2 {
		t.Errorf("since 过滤后应为 2，got %d", late)
	}

	// 别的设备不得串数。
	other, err := op.Create(TargetInput{Name: "nas-2", Type: "host", Protocol: "local", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := op.CountSnapshots(other.ID, time.Time{}); n != 0 {
		t.Errorf("新设备快照数应为 0，got %d", n)
	}
}
