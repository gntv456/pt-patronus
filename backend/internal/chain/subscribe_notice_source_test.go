package chain

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"gorm.io/gorm"
)

// 持续型状态（未配置下载器/评分拦截/站点不可用）不能挂「订阅命中」来源：
// 通知中心会贴「订阅命中」标签，正文却在说失败。命中通知仍用 subscribe_hit。
func TestSubscribeNoticeSourcesDistinguishHitAndFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sub-notice.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	noticeOp := oper.NewNoticeOper(db)
	ch := &SubscribeChain{notice: notice.NewService(noticeOp, nil, nil)}
	sub := &models.Subscription{ID: 9, Title: "订阅"}

	ch.noticeStable(context.Background(), sub, "no-downloader",
		"订阅检查："+sub.Title, "未配置默认下载器，已跳过自动下载", "warning")
	ch.noticeOnce(context.Background(), sub, "订阅命中："+sub.Title, "命中 1 条", "success")

	ns, err := noticeOp.List(false, 10)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, n := range ns {
		got[n.Source]++
	}
	if got[notice.SourceSubscribeCheckFail] != 1 {
		t.Fatalf("持续失败状态要用 subscribe_check_fail, got sources=%v", got)
	}
	if got[notice.SourceSubscribeHit] != 1 {
		t.Fatalf("真正命中仍要用 subscribe_hit, got sources=%v", got)
	}
}
