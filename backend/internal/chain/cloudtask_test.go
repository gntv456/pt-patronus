package chain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// newCloudTaskTestChain 内存 DB + 云盘/STRM/任务三表。
func newCloudTaskTestChain(t *testing.T) (*CloudChain, *oper.CloudTaskOper, *oper.CloudDriveOper) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/tasks.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.CloudDrive{}, &models.StrmRule{}, &models.StrmRecord{}, &models.CloudTask{}); err != nil {
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
	tOp := oper.NewCloudTaskOper(db)
	base := NewBase(module.New(), event.New())
	ch, err := NewCloudChain(base, dOp, rOp, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ch, tOp, dOp
}

// TestCloudTaskEnqueueClaimRetry 任务生命周期：入队→抢占→失败重试→终态。
func TestCloudTaskEnqueueClaimRetry(t *testing.T) {
	_, op, _ := newCloudTaskTestChain(t)
	task := &models.CloudTask{DriveID: 1, Kind: "upload", CloudPath: "/a.mkv", LocalPath: "x", Size: 5}
	if err := op.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	if task.Status != "pending" || task.Progress != 0 {
		t.Errorf("enqueue state wrong: %+v", task)
	}
	got, err := op.ClaimPending()
	if err != nil || got == nil || got.ID != task.ID {
		t.Fatalf("claim failed: %v %v", got, err)
	}
	// 再抢：无 pending
	if again, _ := op.ClaimPending(); again != nil {
		t.Errorf("second claim should be nil, got %+v", again)
	}
	// 失败 1：重入队（attempts<max）
	if err := op.Fail(got.ID, "boom", 3); err != nil {
		t.Fatal(err)
	}
	ts, _ := op.List("pending", 10)
	if len(ts) != 1 || ts[0].Attempts != 1 {
		t.Errorf("after fail should requeue, got %+v", ts)
	}
	// 连续失败到上限 → failed
	got2, _ := op.ClaimPending()
	op.Fail(got2.ID, "boom", 3)
	got3, _ := op.ClaimPending()
	op.Fail(got3.ID, "boom", 3)
	got4, _ := op.ClaimPending()
	op.Fail(got4.ID, "boom", 3)
	ts, _ = op.List("failed", 10)
	if len(ts) != 1 {
		t.Errorf("should be terminal failed, got %+v", ts)
	}
	// 手动重试 → pending
	if err := op.Retry(got4.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := op.PendingCount(); n != 1 {
		t.Errorf("retry should requeue, pending=%d", n)
	}
	// 成功
	got5, _ := op.ClaimPending()
	if err := op.Succeed(got5.ID); err != nil {
		t.Fatal(err)
	}
	ts, _ = op.List("success", 10)
	if len(ts) != 1 || ts[0].Progress != 100 {
		t.Errorf("success state wrong: %+v", ts)
	}
}

// TestImportTreeStrm 目录树导入：媒体行生成 .strm、非媒体/分隔行跳过、目录树内容正确。
func TestImportTreeStrm(t *testing.T) {
	ch, _, _ := newCloudTaskTestChain(t)
	out := t.TempDir()
	tree := "电影/Inception (2010)/Inception.2010.1080p.mkv\n" +
		"电影/Inception (2010)/poster.jpg\n" +
		"---- 剧集 ----\n" +
		"剧集/庆余年/S02E01.mp4\n"
	payload, _ := json.Marshal(map[string]string{
		"tree": tree, "base": "/媒体库", "output_dir": out, "template": "{direct}{cloudpath}",
	})
	task := &models.CloudTask{DriveID: 9, Kind: "import_tree", Payload: string(payload)}
	if err := ch.ImportTreeStrm(nil, task); err != nil {
		t.Fatal(err)
	}
	// 两个媒体文件 → 两份 .strm
	b1, err := os.ReadFile(filepath.Join(out, "电影", "Inception (2010)", "Inception.2019.1080p.strm"))
	if err == nil {
		t.Logf("unexpected alt file: %s", b1)
	}
	b1, err = os.ReadFile(filepath.Join(out, "电影", "Inception (2010)", "Inception.2010.1080p.mkv.strm"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != "/api/v1/cloud/direct/9/媒体库/电影/Inception (2010)/Inception.2010.1080p.mkv\n" {
		t.Errorf("strm content wrong: %q", b1)
	}
	b2, err := os.ReadFile(filepath.Join(out, "剧集", "庆余年", "S02E01.mp4.strm"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(b2), "/api/v1/cloud/direct/9/媒体库/剧集/庆余年/S02E01.mp4") {
		t.Errorf("strm2 wrong: %q", b2)
	}
	// 海报不应生成
	if _, err := os.Stat(filepath.Join(out, "电影", "Inception (2010)", "poster.jpg.strm")); err == nil {
		t.Error("poster should be skipped")
	}
}

// TestImportTreeStrmFailuresNotSilent 逐行落盘失败不得静默 success：
// output_dir 为相对路径（非法）→ 每行 UpsertStrm 必失败 → 必须返回错误交任务
// 标失败可见（此前逐行仅 Debug+continue、全失败仍 return nil → 任务假 success）。
func TestImportTreeStrmFailuresNotSilent(t *testing.T) {
	ch, _, _ := newCloudTaskTestChain(t)
	payload, _ := json.Marshal(map[string]string{
		"tree":       "电影/A.mkv\n剧集/B.mp4\n",
		"base":       "/媒体库",
		"output_dir": "relative/not-absolute",
	})
	task := &models.CloudTask{DriveID: 9, Kind: "import_tree", Payload: string(payload)}
	if err := ch.ImportTreeStrm(nil, task); err == nil {
		t.Fatal("全行失败必须返回错误（否则任务静默 success，用户以为已导入）")
	}
}

// TestImportTreeStrmNoMediaNoFail 树中没有媒体文件 → 无操作、不报错（跳过≠失败）。
func TestImportTreeStrmNoMediaNoFail(t *testing.T) {
	ch, _, _ := newCloudTaskTestChain(t)
	payload, _ := json.Marshal(map[string]string{
		"tree":       "电影/A/poster.jpg\n---\n",
		"base":       "/媒体库",
		"output_dir": "relative/not-absolute", // 无媒体行 → 不会触到该路径
	})
	task := &models.CloudTask{DriveID: 9, Kind: "import_tree", Payload: string(payload)}
	if err := ch.ImportTreeStrm(nil, task); err != nil {
		t.Fatalf("无媒体行不应报错: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
