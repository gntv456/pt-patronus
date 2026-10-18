package transfer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRunDueDirSyncs 定时 tick：仅执行「启用 + interval≥5 + 已到期」的任务并回写 last_run_at。
func TestRunDueDirSyncs(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.mkv"), []byte("video"), 0o644)

	kv := &memKV{m: map[string]string{}}
	now := time.Now()
	if err := SaveDirSyncTasks(kv, []DirSyncTask{
		{Name: "due", SrcDir: src, DstDir: dst, Mode: "copy", Enabled: true, IntervalMinutes: 5},
		{Name: "disabled", SrcDir: src, DstDir: dst, Mode: "copy", Enabled: false, IntervalMinutes: 5},
		{Name: "manual", SrcDir: src, DstDir: dst, Mode: "copy", Enabled: true},
		{Name: "notdue", SrcDir: src, DstDir: dst, Mode: "copy", Enabled: true, IntervalMinutes: 60, LastRunAt: now.Add(-5 * time.Minute).Unix()},
	}); err != nil {
		t.Fatal(err)
	}

	runs, err := RunDueDirSyncs(kv, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Name != "due" {
		t.Fatalf("runs = %+v, want only [due]", runs)
	}
	if runs[0].Result == nil || runs[0].Result.Synced != 1 {
		t.Fatalf("due result = %+v", runs[0].Result)
	}
	if _, err := os.Stat(filepath.Join(dst, "a.mkv")); err != nil {
		t.Fatalf("文件应已同步: %v", err)
	}

	// last_run_at 已回写：紧接的第二次 tick 不应重复执行
	got, err := LoadDirSyncTasks(kv)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range got {
		if task.Name == "due" && task.LastRunAt == 0 {
			t.Error("due 的 last_run_at 未回写")
		}
	}
	if again, err := RunDueDirSyncs(kv, now); err != nil || len(again) != 0 {
		t.Fatalf("到期判定失败，重复执行: %+v err=%v", again, err)
	}

	// 到期后再次执行
	if r, err := RunDueDirSyncs(kv, now.Add(6*time.Minute)); err != nil || len(r) != 1 {
		t.Fatalf("到期后应再次执行: %+v err=%v", r, err)
	}
}

// errKV 模拟 kv 读失败（真实场景：DB 关闭/存储故障）。
type errKV struct{ err error }

func (e errKV) Get(string) (string, error) { return "", e.err }
func (e errKV) Set(string, string) error   { return e.err }

// TestLoadDirSyncTasksSurfacesReadErrors：读失败/配置损坏必须与「空清单」区分——
// 此前两者都静默变 nil：cron 整轮不跑、界面空列表，与「真的没配置」同形。
func TestLoadDirSyncTasksSurfacesReadErrors(t *testing.T) {
	if ts, err := LoadDirSyncTasks(nil); err != nil || len(ts) != 0 {
		t.Fatalf("无 KV 应视为空清单（特性未配置），got %v, %v", ts, err)
	}
	if ts, err := LoadDirSyncTasks(&memKV{m: map[string]string{}}); err != nil || len(ts) != 0 {
		t.Fatalf("从未保存过应视为空清单，got %v, %v", ts, err)
	}
	if _, err := LoadDirSyncTasks(errKV{err: errors.New("db closed")}); err == nil {
		t.Error("kv 读失败必须返回错误（否则被渲染成「没有任务」）")
	}
	bad := &memKV{m: map[string]string{dirSyncKey: "{not json"}}
	if _, err := LoadDirSyncTasks(bad); err == nil {
		t.Error("配置损坏必须返回错误（否则被渲染成「没有任务」）")
	}

	// cron tick 面同口径：整轮跳过必须能报出来，不能静默当「没有到期任务」。
	runs, err := RunDueDirSyncs(errKV{err: errors.New("db closed")}, time.Now())
	if err == nil {
		t.Error("RunDueDirSyncs：清单读失败必须返回 err 供调用方自曝")
	}
	if len(runs) != 0 {
		t.Errorf("读失败时不应有执行记录，got %+v", runs)
	}
}

func TestDirSyncCopyAndStrm(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.mkv"), []byte("video"), 0o644)
	os.WriteFile(filepath.Join(src, "b.txt"), []byte("skip"), 0o644)

	// copy：媒体过滤（b.txt 跳过）
	res := RunDirSync(DirSyncTask{Name: "t", SrcDir: src, DstDir: dst, Mode: "copy"})
	if res.Scanned != 2 || res.Synced != 1 || res.Skipped != 1 {
		t.Errorf("copy result: %+v", res)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "a.mkv")); err != nil || string(b) != "video" {
		t.Errorf("copied file wrong: %q err=%v", b, err)
	}
	// 再跑一遍：幂等跳过
	res = RunDirSync(DirSyncTask{Name: "t", SrcDir: src, DstDir: dst, Mode: "copy"})
	if res.Synced != 0 || res.Skipped != 2 {
		t.Errorf("idempotent copy: %+v", res)
	}

	// strm：生成指向绝对路径的 .strm
	dst2 := t.TempDir()
	res = RunDirSync(DirSyncTask{Name: "s", SrcDir: src, DstDir: dst2, Mode: "strm"})
	if res.Synced != 1 {
		t.Fatalf("strm result: %+v", res)
	}
	abs, _ := filepath.Abs(filepath.Join(src, "a.mkv"))
	b, err := os.ReadFile(filepath.Join(dst2, "a.mkv.strm"))
	if err != nil || string(b) != abs+"\n" {
		t.Errorf("strm content: %q err=%v", b, err)
	}
}

func TestDirSyncMove(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	p := filepath.Join(src, "m.mkv")
	os.WriteFile(p, []byte("x"), 0o644)
	res := RunDirSync(DirSyncTask{Name: "mv", SrcDir: src, DstDir: dst, Mode: "move"})
	if res.Synced != 1 {
		t.Fatalf("move: %+v", res)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("source should be removed after move")
	}
	if _, err := os.Stat(filepath.Join(dst, "m.mkv")); err != nil {
		t.Errorf("dest missing: %v", err)
	}
}

func TestDirSyncValidation(t *testing.T) {
	kv := kvTag{}
	if err := SaveDirSyncTasks(kv, []DirSyncTask{{Name: "x", Mode: "bad"}}); err == nil {
		t.Error("bad mode should fail")
	}
	if err := SaveDirSyncTasks(kv, []DirSyncTask{{Name: "x", Mode: "copy"}, {Name: "x", Mode: "copy"}}); err == nil {
		t.Error("dup name should fail")
	}
	ok := []DirSyncTask{{Name: "x", SrcDir: "/a", DstDir: "/b", Mode: "link", Enabled: true}}
	if err := SaveDirSyncTasks(kv, ok); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDirSyncTasks(kv)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Mode != "link" {
		t.Errorf("roundtrip: %+v", got)
	}
}
