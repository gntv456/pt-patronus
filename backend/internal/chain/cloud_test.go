package chain

import (
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

// newCloudTestChain 构建一条接内存 DB 的 CloudChain（sub=nil；CRUD/匹配测试不依赖真实网盘）。
func newCloudTestChain(t *testing.T) (*CloudChain, *oper.CloudDriveOper, *oper.StrmRuleOper) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cloud.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.CloudDrive{}, &models.StrmRule{}, &models.StrmRecord{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	crypto := security.NewCrypto("test-seed")
	dOp := oper.NewCloudDriveOper(db, crypto)
	rOp := oper.NewStrmRuleOper(db)
	base := NewBase(module.New(), event.New())
	ch, err := NewCloudChain(base, dOp, rOp, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ch, dOp, rOp
}

func TestCloudDriveCRUDOper(t *testing.T) {
	_, op, _ := newCloudTestChain(t)

	// 类型白名单：非法类型返 ErrInvalidInput
	if _, err := op.Create(oper.CloudDriveInput{Name: "bad", Type: "baidu"}); !errors.Is(err, oper.ErrInvalidInput) {
		t.Errorf("bad type want ErrInvalidInput, got %v", err)
	}

	// 创建 115 + token 加密往返
	d, err := op.Create(oper.CloudDriveInput{Name: "我的115", Type: "115", Token: "UID=1; CID=2", RootPath: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if d.TokenEnc == "" || d.TokenEnc == "UID=1; CID=2" {
		t.Error("token should be encrypted at rest")
	}
	tok, err := op.Token(d)
	if err != nil || tok != "UID=1; CID=2" {
		t.Errorf("token roundtrip: %q err=%v", tok, err)
	}
	if d.Name != "我的115" || d.Type != "115" {
		t.Errorf("drive fields wrong: %+v", d)
	}

	// 更新：空 token = 保留旧值
	upd, err := op.Update(d.ID, oper.CloudDriveInput{Name: "115-renamed", Type: "115", RootPath: "/电影"})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Name != "115-renamed" {
		t.Errorf("name not updated: %+v", upd)
	}
	if tok2, _ := op.Token(upd); tok2 != "UID=1; CID=2" {
		t.Errorf("empty token should preserve old, got %q", tok2)
	}

	// List
	ds, _ := op.List()
	if len(ds) != 1 {
		t.Errorf("list len=%d want 1", len(ds))
	}

	// Delete
	if err := op.Delete(d.ID); err != nil {
		t.Fatal(err)
	}
	ds, _ = op.List()
	if len(ds) != 0 {
		t.Errorf("after delete len=%d want 0", len(ds))
	}
}

func TestStrmRuleOper(t *testing.T) {
	_, _, rOp := newCloudTestChain(t)

	rule, err := rOp.Create(oper.StrmRuleInput{
		Name: "电影规则", DriveID: 1, CloudPath: "/电影", OutputDir: "/strm/电影", MountRoot: "/mnt/115",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rule.Recursive || !rule.KeepExt || !rule.Enabled {
		t.Errorf("defaults wrong: %+v", rule)
	}

	// ListEnabled
	rs, _ := rOp.ListEnabled()
	if len(rs) != 1 {
		t.Errorf("enabled len=%d want 1", len(rs))
	}

	// UpsertRecord：首次 created=true，再次 created=false
	created, err := rOp.UpsertRecord(1, "/电影/a.mkv", "/strm/电影/a.mkv.strm", 100, 0)
	if err != nil || !created {
		t.Errorf("first upsert created=%v err=%v", created, err)
	}
	created2, err := rOp.UpsertRecord(1, "/电影/a.mkv", "/strm/电影/a.mkv.strm", 100, 0)
	if err != nil || created2 {
		t.Errorf("second upsert created=%v want false, err=%v", created2, err)
	}

	// ListRecords + DeleteRecord
	recs, _ := rOp.ListRecords(1)
	if len(recs) != 1 {
		t.Errorf("records len=%d want 1", len(recs))
	}
	if err := rOp.DeleteRecord(recs[0].ID); err != nil {
		t.Fatal(err)
	}
	recs, _ = rOp.ListRecords(1)
	if len(recs) != 0 {
		t.Errorf("after delete records len=%d want 0", len(recs))
	}
}

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"Inception.2010.1080p.BluRay.x264.mkv": "inception",
		"Movie.1999.X265-HDC":                  "movie hdc", // 1999/year 与 x265 剥离，hdc 非 noise 保留
		"庆余年 第二季 S02E12.mkv":                "庆余年 第二季 s02e12",
		"a.b.c.mp4":                            "a b c",
	}
	for in, want := range cases {
		if got := normalizeName(in); got != want {
			t.Errorf("normalizeName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestMatchAnySubscription(t *testing.T) {
	subs := []models.Subscription{
		{ID: 1, Title: "Inception", Status: "subscribed", Type: "movie"},
		{ID: 2, Title: "庆余年", Status: "subscribed", Type: "tv"},
	}
	ch := &CloudChain{}
	if m := matchAnySubscription(subs, "Inception.2010.1080p.BluRay.mkv"); m == nil || m.ID != 1 {
		t.Errorf("should match Inception, got %+v", m)
	}
	if m := matchAnySubscription(subs, "庆余年 第二季 S02E12.mkv"); m == nil || m.ID != 2 {
		t.Errorf("should match 庆余年, got %+v", m)
	}
	if m := matchAnySubscription(subs, "Random.Movie.2020.mkv"); m != nil {
		t.Errorf("should not match, got %+v", m)
	}
	_ = ch // CloudChain 零值占位（matchAnySubscription 为包级函数，不依赖接收者）
	// 已完成/暂停的订阅不参与（调用方 SyncSubscriptions 已过滤 status；这里仅验匹配本身）
}

func TestCloudChainDeleteCascade(t *testing.T) {
	ch, op, rOp := newCloudTestChain(t)
	d, _ := op.Create(oper.CloudDriveInput{Name: "d", Type: "115", Token: "x"})
	rOp.Create(oper.StrmRuleInput{Name: "r", DriveID: d.ID, CloudPath: "/", OutputDir: "/o"})
	rOp.UpsertRecord(d.ID, "/a.mkv", "/o/a.strm", 1, 0)
	// 删网盘应级联清规则+记录
	if err := ch.Delete(d.ID); err != nil {
		t.Fatal(err)
	}
	if rs, _ := rOp.ListRecords(d.ID); len(rs) != 0 {
		t.Errorf("records not cascaded: %d", len(rs))
	}
	if rs, _ := rOp.List(); len(rs) != 0 {
		t.Errorf("rules not cascaded: %d", len(rs))
	}
}

// TestMatchAnySubscriptionTokenized 订阅匹配按独立 token，避免裸子串误匹配
// （"It" 不应命中 "The Comm**it**ments"、"Her" 不应命中 "Mot**her**"）。
func TestMatchAnySubscriptionTokenized(t *testing.T) {
	subs := []models.Subscription{
		{Title: "It"},
		{Title: "Dune"},
	}
	if m := matchAnySubscription(subs, "The.Commitments.1991.1080p.BluRay.mkv"); m != nil {
		t.Errorf("It should NOT match The Commitments, got %q", m.Title)
	}
	if m := matchAnySubscription(subs, "It.1990.1080p.BluRay.mkv"); m == nil || m.Title != "It" {
		t.Errorf("It should match It.1990, got %v", m)
	}
	if m := matchAnySubscription(subs, "Dune.2021.2160p.UHD.mkv"); m == nil || m.Title != "Dune" {
		t.Errorf("Dune should match Dune.2021, got %v", m)
	}
}
