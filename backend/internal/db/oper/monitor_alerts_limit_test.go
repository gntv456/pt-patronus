package oper

import (
	"fmt"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// TestListAlertsReportsTrueTotal 回归：ListAlerts 过去硬编码 Limit(200) 且不报总数，
// 于是面板把「取到的 200 条」当成「全部告警」；未解决的告警不会被 30 天清理删掉，
// 积压超过上限时最旧（往往最没人处理）的那批直接不可见，也没有一句话说「还有更多」。
func TestListAlertsReportsTrueTotal(t *testing.T) {
	db := newMonitorDB(t)
	op := NewMonitorOper(db, security.NewCrypto("alert-limit-seed"))
	a, err := op.Create(TargetInput{Name: "host-a", Type: "host", Protocol: "local", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := op.Create(TargetInput{Name: "host-b", Type: "host", Protocol: "local", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	seed := func(targetID uint, n int, read bool, tag string) {
		for i := 0; i < n; i++ {
			if err := db.Create(&models.MonitorAlert{
				TargetID: targetID, Level: "warning", Metric: tag, Message: fmt.Sprintf("%s-%d", tag, i), Read: read,
			}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	seed(a.ID, MonitorAlertListLimit+3, false, "未读") // 203 条未读
	seed(a.ID, 5, true, "已读")                        // 5 条已读
	seed(b.ID, 10, false, "别机")                      // 另一台设备，不该混进账面

	unread, totalUnread, err := op.ListAlerts(a.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != MonitorAlertListLimit {
		t.Errorf("列表应被截到上限 %d，got %d", MonitorAlertListLimit, len(unread))
	}
	if totalUnread != int64(MonitorAlertListLimit+3) {
		t.Errorf("总数要按同口径算（未读 203），got %d", totalUnread)
	}
	if int64(len(unread)) >= totalUnread {
		t.Error("用例前提不成立：没真的截断，就证明不了 total 承重")
	}

	all, totalAll, err := op.ListAlerts(a.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != MonitorAlertListLimit || totalAll != int64(MonitorAlertListLimit+8) {
		t.Errorf("不筛已读时应含 208 条总数、列表仍截断，got len=%d total=%d", len(all), totalAll)
	}

	// 别的设备不能借同一台的上限漏计（filters 两处口径必须一致）
	other, totalOther, err := op.ListAlerts(b.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 10 || totalOther != 10 {
		t.Errorf("host-b 应只有自己 10 条，got len=%d total=%d", len(other), totalOther)
	}
}
