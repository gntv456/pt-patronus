// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// TestBrushflowRejectsTaglessDownloader 不支持标签的下载器（如 uTorrent，caps.Tags=false）
// 不得建任务、历史任务也不得运行选种/扫删：Tag 被静默丢弃 → MaxSeeds 限流恒失效
// （每轮加满、无限膨胀）且扫删永匹配不到（永不删种）。qB（Tags=true）作为正控。
func TestBrushflowRejectsTaglessDownloader(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/bf.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, e := db.DB(); e == nil {
			_ = s.Close()
		}
	})
	if err := db.AutoMigrate(&models.Downloader{}, &models.BrushFlowTask{}); err != nil {
		t.Fatal(err)
	}
	crypto := security.NewCrypto("seed")
	dOp := oper.NewDownloaderOper(db, crypto)
	dUT, err := dOp.Create(oper.DownloaderInput{Name: "ut", Type: "ut", URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	dQB, err := dOp.Create(oper.DownloaderInput{Name: "qb", Type: "qb", URL: "http://127.0.0.1:2"})
	if err != nil {
		t.Fatal(err)
	}
	base := NewBase(module.New(), event.New())
	dlCh := NewDownloadChain(base, nil, dOp, nil)
	bOp := oper.NewBrushFlowOper(db)
	bf := NewBrushFlowChain(base, nil, nil, dlCh, bOp, nil, nil)

	if err := bf.CreateTask(&models.BrushFlowTask{Name: "t-ut", DownloaderID: dUT.ID, Enabled: true}); !errors.Is(err, ErrBrushflowNoTagSupport) {
		t.Fatalf("ut 建任务应被拒: %v", err)
	}
	// 历史任务（绕过 CreateTask 落库，如能力表收紧前的既有数据）→ 运行同样拒跑。
	hist := &models.BrushFlowTask{Name: "hist", DownloaderID: dUT.ID, Enabled: true}
	if err := bOp.CreateTask(hist); err != nil {
		t.Fatal(err)
	}
	if err := bf.RunSelect(context.Background(), hist.ID); !errors.Is(err, ErrBrushflowNoTagSupport) {
		t.Fatalf("ut 运行选种应被拒: %v", err)
	}
	if err := bf.RunSweep(context.Background(), hist.ID); !errors.Is(err, ErrBrushflowNoTagSupport) {
		t.Fatalf("ut 运行扫删应被拒: %v", err)
	}
	// 正控：能力表判定正确（qb Tags=true → 建任务顺利落库）。
	if err := bf.CreateTask(&models.BrushFlowTask{Name: "t-qb", DownloaderID: dQB.ID, Enabled: true}); err != nil {
		t.Fatalf("qb 建任务不应因类型被拒: %v", err)
	}
}
