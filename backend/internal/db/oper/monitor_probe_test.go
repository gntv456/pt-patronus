// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"gorm.io/gorm"
)

// TestRecordProbePersistsStatusWithoutSnapshot 手动探活的落库口径：状态三件套要写
// （否则设备卡片永远「未检测」，用户看不出自己点过），但绝不能落一条 MetricSnapshot
// ——探活只回答通不通，CPU/内存/累计字节全是空的。空快照既是一个凭空的 0% 读数，
// 又会成为 computeNetRate 的上一条基准（累计字节 0 → 下次采集把开机以来的全部流量
// 除以几秒），网速被顶成天文数字。
func TestRecordProbePersistsStatusWithoutSnapshot(t *testing.T) {
	db := newMonitorDB(t)
	op := NewMonitorOper(db, security.NewCrypto("probe-persist-seed"))
	target, err := op.Create(TargetInput{Name: "nas-probe", Type: "host", Protocol: "ssh", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	// 先有一条真实采集快照，作为网络累计基准。
	if _, err := op.RecordSnapshot(target, monitor.Metrics{Online: true, NetRxBytes: 8000000}, nil); err != nil {
		t.Fatal(err)
	}
	base := snapshotCount(t, db)

	if err := op.RecordProbe(target, true, nil); err != nil {
		t.Fatalf("RecordProbe: %v", err)
	}

	var got models.MonitorTarget
	if err := db.First(&got, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastOnline == nil || !*got.LastOnline {
		t.Errorf("探活成功要写 last_online=true，got %+v", got.LastOnline)
	}
	if got.LastCheckedAt == nil {
		t.Error("探活要写 last_checked_at（否则卡片仍显示未检测）")
	}
	if after := snapshotCount(t, db); after != base {
		t.Errorf("探活不得新增度量快照，快照数 %d → %d", base, after)
	}

	// 失败探活：原因进 last_error、在线位跟着翻，同样不落快照。
	boom := errors.New("dial tcp 10.0.0.9:22: i/o timeout")
	if err := op.RecordProbe(target, false, boom); err != nil {
		t.Fatalf("RecordProbe(fail): %v", err)
	}
	if err := db.First(&got, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastOnline != nil && *got.LastOnline {
		t.Error("探活失败后 last_online 必须为 false")
	}
	if !strings.Contains(got.LastError, "i/o timeout") {
		t.Errorf("last_error 要带探活原因，got %q", got.LastError)
	}
	if after := snapshotCount(t, db); after != base {
		t.Errorf("失败探活同样不得新增度量快照，快照数 %d → %d", base, after)
	}
}

// snapshotCount 当前 MetricSnapshot 行数（探活前后必须一致）。
func snapshotCount(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var n int64
	if err := db.Model(&models.MetricSnapshot{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return int(n)
}
