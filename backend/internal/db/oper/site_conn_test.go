package oper

import (
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// TestUpsertConnectivityAtomicSQL 验证原子 SQL 累加/滑动平均（四轮 P2-4 改造的回归钉）：
// success/fail 走 gorm.Expr 列级自增，avg_seconds 在 SQL 端算新均 = (旧均*n + 本次)/(n+1)。
func TestUpsertConnectivityAtomicSQL(t *testing.T) {
	db := openIsolatedDB(t)
	if err := db.AutoMigrate(&models.SiteConnectivity{}); err != nil {
		t.Fatal(err)
	}
	op := &SiteOper{db: db}

	// 首次 connected（插入行）：success=1, avg=12.0
	if err := op.UpsertConnectivity(1, "connected", "ok", "", 12.0); err != nil {
		t.Fatal(err)
	}
	var rec models.SiteConnectivity
	if err := db.Where("site_id = ?", 1).First(&rec).Error; err != nil {
		t.Fatal(err)
	}
	if rec.Success != 1 || rec.AvgSeconds != 12.0 {
		t.Errorf("first: success=%d avg=%v, want 1/12.0", rec.Success, rec.AvgSeconds)
	}
	// 第二次 connected 6s：success=2, avg=(12*1+6)/2=9
	if err := op.UpsertConnectivity(1, "connected", "ok", "", 6.0); err != nil {
		t.Fatal(err)
	}
	db.Where("site_id = ?", 1).First(&rec)
	if rec.Success != 2 || rec.AvgSeconds != 9.0 {
		t.Errorf("second: success=%d avg=%v, want 2/9.0", rec.Success, rec.AvgSeconds)
	}
	// 第三次 connected（seconds=0 不更新 avg）：success=3, avg 仍 9
	if err := op.UpsertConnectivity(1, "connected", "ok", "", 0); err != nil {
		t.Fatal(err)
	}
	db.Where("site_id = ?", 1).First(&rec)
	if rec.Success != 3 || rec.AvgSeconds != 9.0 {
		t.Errorf("third: success=%d avg=%v, want 3/9.0", rec.Success, rec.AvgSeconds)
	}
	// disconnected：fail=1，success/avg 不动
	if err := op.UpsertConnectivity(1, "disconnected", "err", "timeout", 0); err != nil {
		t.Fatal(err)
	}
	db.Where("site_id = ?", 1).First(&rec)
	if rec.Fail != 1 || rec.Success != 3 || rec.AvgSeconds != 9.0 {
		t.Errorf("fail: fail=%d success=%d avg=%v, want 1/3/9.0", rec.Fail, rec.Success, rec.AvgSeconds)
	}
}

// TestSiteCreateConcurrentSameDomain 并发建同域站点：唯一索引兜底下最多一条落库
// （四轮 P2-5 迁移 v3/v4 + 冲突映射 ErrDuplicateSite 的回归钉）。
func TestSiteCreateConcurrentSameDomain(t *testing.T) {
	db := openIsolatedDB(t)
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	op := NewSiteOper(db, nil)
	var wg sync.WaitGroup
	var okCount, dupCount int
	var mu sync.Mutex
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := op.Create(SiteInput{Name: "RaceSite", BaseURL: "https://racesite.example.org"})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				okCount++
			} else {
				dupCount++
			}
		}()
	}
	wg.Wait()
	var cnt int64
	db.Model(&models.Site{}).Where("base_url LIKE ?", "%racesite.example.org%").Count(&cnt)
	if cnt != 1 {
		t.Errorf("concurrent same-domain create: rows=%d want 1 (ok=%d dup=%d)", cnt, okCount, dupCount)
	}
	if okCount != 1 || dupCount != 7 {
		t.Errorf("ok=%d dup=%d, want 1/7", okCount, dupCount)
	}
}
