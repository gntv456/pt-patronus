package chain

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// 手动「检查」的失败原因原样进界面 toast（200+{ok:false,error}）：
// 从前这里回英文 `no default downloader`，中文界面用户既看不懂也不知道去哪配。
func TestSubscribeCheckNoDownloaderReasonIsChinese(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sub-nodefault.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Subscription{}, &models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	ch := &SubscribeChain{
		dlOp: oper.NewDownloaderOper(db, nil),
		op:   oper.NewSubscriptionOper(db),
	}
	sub := &models.Subscription{ID: 1, Title: "无下载器订阅", Category: "movie"}
	_, err = ch.checkWithDefaults(context.Background(), sub, GlobalDefaults{})
	if err == nil {
		t.Fatal("没有下载器时应报错")
	}
	msg := err.Error()
	if !strings.Contains(msg, "默认下载器") || !strings.Contains(msg, "下载器」页") {
		t.Fatalf("原因是写给用户看的中文并指明去哪里配, got %q", msg)
	}
	if strings.Contains(msg, "no default downloader") {
		t.Fatalf("不许把英文原文回给界面, got %q", msg)
	}
}
