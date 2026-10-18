package chain

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/transfer"
	"gorm.io/gorm"
)

// Sweep 此前返回 void：cron/API/工具三面只能回执「已触发」，而「整理未启用」「下载器一个都列不出」
// 「连不上下载器」这三种什么都没跑的情况在界面上和真跑了长得一样。
// 现在必须交出账面，且 String() 不能说成成功。

func newSweepDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sweep.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Downloader{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func newSweepChain(db *gorm.DB) *TransferChain {
	dlOp := oper.NewDownloaderOper(db, security.NewCrypto("sweep-test-seed"))
	return &TransferChain{cfg: oper.NewSystemConfigOper(db), dlOp: dlOp, dlMgr: manager.NewManager(dlOp)}
}

func TestTransferSweepReportsDisabled(t *testing.T) {
	db := newSweepDB(t)
	ch := newSweepChain(db)
	if err := transfer.Save(oper.NewSystemConfigOper(db), transfer.Config{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if ch.SweepEnabled() {
		t.Fatal("配置已关，SweepEnabled 仍报开")
	}
	sum := ch.Sweep(context.Background())
	if sum.Enabled {
		t.Fatalf("未启用时 summary.Enabled 必须为 false，got %+v", sum)
	}
	if sum.Downloaders != 0 || sum.Candidates != 0 {
		t.Fatalf("未启用时不该碰任何下载器，got %+v", sum)
	}
	if s := sum.String(); !strings.Contains(s, "未启用") {
		t.Errorf("一句话结论要说明未启用，got %q", s)
	}
}

func TestTransferSweepReportsNoDownloaders(t *testing.T) {
	db := newSweepDB(t)
	ch := newSweepChain(db)
	sum := ch.Sweep(context.Background())
	if !sum.Enabled {
		t.Fatalf("默认配置应视为启用，got %+v", sum)
	}
	if sum.Downloaders != 0 || sum.Scanned != 0 {
		t.Fatalf("一个下载器都没配置时应如实报 0，got %+v", sum)
	}
	if s := sum.String(); strings.Contains(s, "完成") || strings.Contains(s, "成功") {
		t.Errorf("无事可做却把结论写成成功： %q", s)
	}
}

func TestTransferSweepRecordsPerDownloaderFailure(t *testing.T) {
	db := newSweepDB(t)
	// 一台建不出客户端（类型不认识）、一台能建但列种子必失败（URL 非法）：
	// 旧代码前者静默 continue、后者只打 Debug 日志，两笔账都不交。
	rows := []models.Downloader{
		{Name: "ghost", Type: "nosuchtype", URL: "http://127.0.0.1:1"},
		{Name: "broken", Type: "dl", URL: "://invalid-url"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	ch := newSweepChain(db)
	sum := ch.Sweep(context.Background())
	if sum.Downloaders != 2 {
		t.Fatalf("应计入 2 台下载器，got %+v", sum)
	}
	if sum.Scanned != 0 {
		t.Fatalf("两台都失败，Scanned 必须为 0，got %+v", sum)
	}
	if len(sum.Failed) != 2 {
		t.Fatalf("每台失败都要留名，got %v", sum.Failed)
	}
	for _, want := range []string{"ghost", "broken"} {
		found := false
		for _, f := range sum.Failed {
			if strings.Contains(f, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("失败清单缺少 %s：%v", want, sum.Failed)
		}
	}
	if s := sum.String(); !strings.Contains(s, "扫描失败") {
		t.Errorf("全部失败的一句话结论应是扫描失败，got %q", s)
	}
}

func TestTransferSweepRecordsListFailure(t *testing.T) {
	db := newSweepDB(t)
	ch := newSweepChain(db)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	sum := ch.Sweep(context.Background())
	if sum.Scanned != 0 || sum.Downloaders != 0 {
		t.Fatalf("读库失败时不该有账面，got %+v", sum)
	}
	if len(sum.Failed) == 0 || !strings.Contains(sum.Failed[0], "列出下载器失败") {
		t.Fatalf("读库失败要记进 Failed，got %v", sum.Failed)
	}
}
