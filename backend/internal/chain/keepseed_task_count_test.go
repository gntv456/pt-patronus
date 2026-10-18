package chain

import (
	"fmt"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// TestReseedTaskListCapStillReportsTotal 回归：候选列表有 500 条上限，但接口只回 tasks，
// 界面把头书写成「候选任务（500）」，把 1300 条渲染成 500 条；清完这 500 条后还谎报「没有待处理候选」。
// 上限本身可以保留（一次拉太多确实卡），但真实条数必须一起交出去。
func TestReseedTaskListCapStillReportsTotal(t *testing.T) {
	db := newReseedDB(t)
	n := KeepseedTaskLimit + 3
	base := time.Now().Add(-time.Duration(n) * time.Minute)
	rows := make([]models.CrossSeedTask, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, models.CrossSeedTask{
			DownloaderID: 1, InfoHash: fmt.Sprintf("H%05d", i), CandidateSite: "site-x",
			Status: "pending", CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	if err := db.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatalf("seed %d tasks: %v", n, err)
	}

	ch := &KeepSeedChain{db: db}
	list, err := ch.ListTasks("pending", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != KeepseedTaskLimit {
		t.Fatalf("列表应停在 %d 条上限，got %d", KeepseedTaskLimit, len(list))
	}
	total, err := ch.CountTasks("pending", 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != int64(n) {
		t.Fatalf("CountTasks 必须给出真实条数 %d，got %d（少了 %d 条候选被界面当成不存在）",
			n, total, int64(n)-total)
	}
	// 取的是「最近」若干条：上限截掉的最老那些不能是列表尾部
	if list[0].InfoHash != fmt.Sprintf("H%05d", n-1) {
		t.Errorf("列表应按创建时间倒序（首条=%s），got %s", fmt.Sprintf("H%05d", n-1), list[0].InfoHash)
	}
}

// TestCountTasksSharesListFilters 筛选口径必须与列表一致：status / site_id 两边同一套条件，
// 否则「一共多少条」会随筛选条件说谎。
func TestCountTasksSharesListFilters(t *testing.T) {
	db := newReseedDB(t)
	rows := []models.CrossSeedTask{
		{DownloaderID: 1, InfoHash: "A", CandidateSite: "s1", Status: "pending", SiteID: 7},
		{DownloaderID: 1, InfoHash: "B", CandidateSite: "s1", Status: "pending", SiteID: 7},
		{DownloaderID: 1, InfoHash: "C", CandidateSite: "s2", Status: "added", SiteID: 8},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	ch := &KeepSeedChain{db: db}

	for _, tc := range []struct {
		status   string
		siteID   uint
		wantList int
		wantCnt  int64
	}{
		{"", 0, 3, 3},
		{"pending", 0, 2, 2},
		{"", 7, 2, 2},
		{"added", 8, 1, 1},
		{"pending", 8, 0, 0},
	} {
		list, err := ch.ListTasks(tc.status, tc.siteID)
		if err != nil {
			t.Fatal(err)
		}
		cnt, err := ch.CountTasks(tc.status, tc.siteID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != tc.wantList || cnt != tc.wantCnt {
			t.Errorf("筛选(%q,site=%d)：list=%d want %d，count=%d want %d",
				tc.status, tc.siteID, len(list), tc.wantList, cnt, tc.wantCnt)
		}
	}
}
