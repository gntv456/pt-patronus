package chain

import (
	"context"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// TestHistoryReportsTruncation 单站历史快照有返回上限：只回数组时，「该区间共 507 个快照」
// 会被界面当成「只有 500 个」，起点那段看起来像站点没数据。真实条数必须一起交出去。
func TestHistoryReportsTruncation(t *testing.T) {
	db := newTrendDB(t)
	base := time.Now().Add(-time.Hour)
	n := HistoryRowLimit + 7
	rows := make([]models.SiteStat, 0, n+3)
	for i := 0; i < n; i++ {
		rows = append(rows, models.SiteStat{
			SiteID: 1, Uploaded: int64(i), Downloaded: 1, SnapAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	// 另一站少量数据：不该被同站的截断口径波及。
	for i := 0; i < 3; i++ {
		rows = append(rows, models.SiteStat{
			SiteID: 2, Uploaded: int64(i), Downloaded: 1, SnapAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	if err := db.CreateInBatches(rows, 200).Error; err != nil {
		t.Fatal(err)
	}
	ch := &DashboardChain{db: db}

	res, err := ch.History(context.Background(), 1, 30)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != int64(n) {
		t.Errorf("total 应为真实条数 %d，got %d", n, res.Total)
	}
	if len(res.Points) != HistoryRowLimit {
		t.Fatalf("列表应停在 %d 上限，got %d", HistoryRowLimit, len(res.Points))
	}
	if !res.Truncated {
		t.Errorf("确实截断了却报 truncated=false：total=%d shown=%d", res.Total, len(res.Points))
	}
	if res.Points[0].Uploaded != int64(n-1) {
		t.Errorf("截断要保留最近的（首条 uploaded=%d），got %d", n-1, res.Points[0].Uploaded)
	}
	if res.Days != 30 {
		t.Errorf("回话要把实际生效的天数交出去，got %d", res.Days)
	}

	other, err := ch.History(context.Background(), 2, 30)
	if err != nil {
		t.Fatal(err)
	}
	if other.Truncated || other.Total != 3 || len(other.Points) != 3 {
		t.Errorf("未超限的站点不能谎报截断，got %+v", other)
	}
}
