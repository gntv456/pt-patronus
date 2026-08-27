package transfer

import (
	"os"
	"path/filepath"
	"testing"
)

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
	got := LoadDirSyncTasks(kv)
	if len(got) != 1 || got[0].Mode != "link" {
		t.Errorf("roundtrip: %+v", got)
	}
}
