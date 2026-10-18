package oper

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// TestNoticeCountMatchesListFilter 收件箱的账面口径：Count 必须与 List 用同一套筛选。
// 少了 unread 过滤，「未读 2 条」的抽屉会显示「共 960 条」——
// 把未读账面顶成全量账面，用户以为未读比实际多，「全部已读」的破坏面也被夸大。
func TestNoticeCountMatchesListFilter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/notice.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	o := NewNoticeOper(db)
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 8; i++ {
		n := &models.Notice{
			Source: "cron", Key: "k" + string(rune('a'+i)), Title: "t", Level: "info",
			Read: i >= 6, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := o.Create(n); err != nil {
			t.Fatal(err)
		}
	}

	all, err := o.Count(false)
	if err != nil {
		t.Fatal(err)
	}
	if all != 8 {
		t.Errorf("全量应为 8，got %d", all)
	}
	unread, err := o.Count(true)
	if err != nil {
		t.Fatal(err)
	}
	if unread != 6 {
		t.Errorf("未读应为 6（Read=true 的 2 条不算），got %d", unread)
	}
	// 角标沿用旧入口，必须与新窄口同数（两处判定一旦漂移，铃铛与抽屉会互相打脸）。
	badge, err := o.CountUnread()
	if err != nil {
		t.Fatal(err)
	}
	if badge != unread {
		t.Errorf("CountUnread 应与 Count(true) 同数，got %d vs %d", badge, unread)
	}
	if list, err := o.List(true, 3); err != nil || len(list) != 3 {
		t.Errorf("未读清单 limit=3 应 3 条，got %d err=%v", len(list), err)
	}
	for _, n := range list2(t, o) {
		if n.Read {
			t.Error("List(unread=true) 里出现了已读通知")
		}
	}
}

func list2(t *testing.T, o *NoticeOper) []models.Notice {
	t.Helper()
	ns, err := o.List(true, 0)
	if err != nil {
		t.Fatal(err)
	}
	return ns
}

// TestNormalizeNoticeLimit 归一函数：超上限取上限而不是退回默认值。
func TestNormalizeNoticeLimit(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, NoticeListDefault},
		{-1, NoticeListDefault},
		{50, 50},
		{NoticeListLimit, NoticeListLimit},
		{999999, NoticeListLimit},
	}
	for _, c := range cases {
		if got := NormalizeNoticeLimit(c.in); got != c.want {
			t.Errorf("NormalizeNoticeLimit(%d)=%d，期望 %d", c.in, got, c.want)
		}
	}
}
