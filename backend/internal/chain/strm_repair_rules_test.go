package chain

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// #291：规则表读不出来时，规则型 .strm 会被保守跳过（拿不到 mount/双地址就不改写）。
// 从前这件事只留一行日志，账面里那几条和「登记型没有文件（无事可做）」共用同一个 skipped，
// 于是界面一句「修复 12 / 跳过 3」把「这批 .strm 仍然是坏的」说成已经处理完了。
func repairRulesDB(t *testing.T) (*CloudChain, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/repair.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.CloudDrive{}, &models.StrmRule{}, &models.StrmRecord{},
		&models.Subscription{}, &models.SubscriptionHit{}, &models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	crypto := security.NewCrypto("repair-seed")
	driveOp := oper.NewCloudDriveOper(db, crypto)
	if _, err := driveOp.Create(oper.CloudDriveInput{Name: "115", Type: "115", Token: "t", RootPath: "/"}); err != nil {
		t.Fatal(err)
	}
	subCh := NewSubscribeChain(nil, nil, nil, oper.NewSubscriptionOper(db), nil, nil, nil, nil)
	ruleOp := oper.NewStrmRuleOper(db)
	ch, err := NewCloudChain(NewBase(module.New(), event.New()), driveOp, ruleOp, subCh, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ch, db
}

// 两条规则型记录（带 .strm 落盘路径）。
func seedStrmRecords(t *testing.T, db *gorm.DB, out string) {
	t.Helper()
	ruleOp := oper.NewStrmRuleOper(db)
	for i, p := range []string{"/电影/a.mkv", "/剧集/b.mp4"} {
		_ = i
		if _, err := ruleOp.UpsertRecord(1, p, out+"/x"+p[len(p)-1:]+".strm", 10, 0); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRepairStrmReportsRulesUnreadable(t *testing.T) {
	out := t.TempDir()
	ch, db := repairRulesDB(t)
	seedStrmRecords(t, db, out)

	// 只让「读规则」这一步失败（记录表还在）：删掉规则表，List() 必然报错。
	if err := db.Migrator().DropTable(&models.StrmRule{}); err != nil {
		t.Fatal(err)
	}

	sum, err := ch.RepairStrm(context.Background(), 1)
	if err != nil {
		t.Fatalf("读不出规则不该整批失败（应降级并记账）：%v", err)
	}
	if !sum.RulesUnavailable {
		t.Errorf("必须标出「规则这次读不出来」：%+v", sum)
	}
	if sum.RulesSkipped != 2 || sum.Skipped != 2 || sum.Repaired != 0 {
		t.Errorf("跳过口径 = repaired %d / skipped %d / rulesSkipped %d, want 0/2/2",
			sum.Repaired, sum.Skipped, sum.RulesSkipped)
	}
}

// 正控：规则表正常时不许冒出「读不出来」的旗标（否则每次修复都在报假故障）。
func TestRepairStrmWithReadableRules(t *testing.T) {
	out := t.TempDir()
	ch, db := repairRulesDB(t)
	seedStrmRecords(t, db, out)

	sum, err := ch.RepairStrm(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.RulesUnavailable || sum.RulesSkipped != 0 {
		t.Errorf("规则读得出时不该记账成「读不出来」：%+v", sum)
	}
	if sum.Skipped+sum.Repaired+sum.Failed != 2 {
		t.Errorf("三项之和应等于记录数 2：%+v", sum)
	}
}
