package transfer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/media"
)

// [P1] #1 电影目录名仅 Title (Year)：1080P/2160P 必须落同目录，洗版才能识别旧版。
func TestDestPathForMovie_FolderIsTitleYearOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TypeFolder = false
	cfg.CategoryFolder = false
	base := filepath.Join(t.TempDir(), "movies")
	// 文件名仍带分辨率/发布组，但目录名必须只是 "Inception (2010)"。
	dst, dir := destPathForMovie(cfg, base, "", "Inception (2010)", "Inception (2010) - 1080P - ESPA", ".mkv")
	if filepath.Base(dir) != "Inception (2010)" {
		t.Fatalf("movie folder = %q, want \"Inception (2010)\"", filepath.Base(dir))
	}
	if !strings.HasSuffix(filepath.Base(dst), " - 1080P - ESPA.mkv") {
		t.Fatalf("file name lost resolution/group: %q", filepath.Base(dst))
	}

	// 2160P 必须落到同一目录（洗版可识别旧版的核心）。
	_, dir2 := destPathForMovie(cfg, base, "", "Inception (2010)", "Inception (2010) - 2160P - XXX", ".mkv")
	if dir != dir2 {
		t.Fatalf("1080P/2160P 落不同目录：1080P=%s 2160P=%s", dir, dir2)
	}
}

// [P1] #1 电影洗版端到端：先入 1080P，再入 2160P → 同目录、删旧、Replaced 非空。
func TestEngine_MovieBestVersion_SameFolderReplace(t *testing.T) {
	tmp := t.TempDir()
	savePath := filepath.Join(tmp, "dl")
	_ = os.MkdirAll(savePath, 0o755)
	movieDir := filepath.Join(tmp, "movies")

	// 先手动放一个旧 1080P 版到库目录（模拟历史入库）。
	oldSrc := filepath.Join(savePath, "Inception.2010.1080p.BluRay.mkv")
	_ = os.WriteFile(oldSrc, []byte("OLD-1080P"), 0o644)

	kv := &memKV{m: map[string]string{}}
	cfg := DefaultConfig()
	cfg.MovieDir = movieDir
	cfg.BestVersion = true
	_ = Save(kv, cfg)

	mi := media.MediaInfo{Source: "tmdb", SourceID: "movie:1", Title: "Inception", Year: 2010, Type: media.TypeMovie}
	eng := NewEngine(fakeRecognizer{mi: mi}, nil, kv, func(context.Context, string) ([]byte, error) { return nil, nil }, nil)

	// 首次：1080P 入库（首版）。
	res := eng.Process(context.Background(), ProcessInput{
		Hash: "h1", Name: "Inception.2010.1080p.BluRay.mkv", SavePath: savePath,
		Files: []FileInput{{Path: oldSrc, Size: 8}},
	})
	if !res.Success || len(res.Replaced) != 0 {
		t.Fatalf("first organize success expected (no replace), got %+v", res)
	}
	firstDir := res.DestDir
	firstFile := res.Files[0]

	// 再来一个 2160P 新版。
	newSrc := filepath.Join(savePath, "Inception.2010.2160p.UHD.BluRay.mkv")
	_ = os.WriteFile(newSrc, []byte("NEW-2160P"), 0o644)

	res2 := eng.Process(context.Background(), ProcessInput{
		Hash: "h2", Name: "Inception.2010.2160p.UHD.BluRay.mkv", SavePath: savePath,
		Files: []FileInput{{Path: newSrc, Size: 8}},
	})
	if !res2.Success {
		t.Fatalf("best_version upgrade should succeed, got err=%q", res2.ErrMsg)
	}
	// 洗版：旧文件已被替换。
	if len(res2.Replaced) == 0 || res2.Replaced[0] != firstFile {
		t.Fatalf("Replaced should record old file %q, got %+v", firstFile, res2.Replaced)
	}
	// 落同目录（修复前会落 "Inception (2010) - 1080P..." vs "...- 2160P..." 两个目录）。
	if res2.DestDir != firstDir {
		t.Fatalf("升级后应落同目录：first=%s second=%s", firstDir, res2.DestDir)
	}
	// 旧文件确实被删，新文件存在。
	if pathExists(firstFile) {
		t.Fatalf("old version file should be removed: %s", firstFile)
	}
	if !pathExists(res2.Files[0]) {
		t.Fatalf("new version file missing: %s", res2.Files[0])
	}
}

// [P2] #4 剧集部分集 link 失败时 ErrMsg 反映缺集，Success=false。
func TestEngine_TVPartialFailureReportsMissing(t *testing.T) {
	tmp := t.TempDir()
	savePath := filepath.Join(tmp, "dl")
	_ = os.MkdirAll(savePath, 0o755)
	tvDir := filepath.Join(tmp, "tv")

	ep1 := filepath.Join(savePath, "Show.S01E01.1080p.mkv")
	ep3 := filepath.Join(savePath, "Show.S01E03.1080p.mkv")
	_ = os.WriteFile(ep1, []byte("A"), 0o644)
	// ep3 路径未创建 → link Apply 时 Open 失败
	_ = os.WriteFile(ep3, []byte("C"), 0o644)
	badEp2 := filepath.Join(savePath, "Show.S01E02.1080p.mkv") // 不存在 → HardLink 失败

	kv := &memKV{m: map[string]string{}}
	cfg := DefaultConfig()
	cfg.TVDir = tvDir
	_ = Save(kv, cfg)

	mi := media.MediaInfo{Source: "tmdb", SourceID: "tv:1", Title: "Show", Year: 2020, Type: media.TypeTV}
	eng := NewEngine(fakeRecognizer{mi: mi}, nil, kv, func(context.Context, string) ([]byte, error) { return nil, nil }, nil)

	res := eng.Process(context.Background(), ProcessInput{
		Hash: "h", Name: "Show.S01", SavePath: savePath,
		Files: []FileInput{{Path: ep1, Size: 1}, {Path: badEp2, Size: 1}, {Path: ep3, Size: 1}},
	})
	// 成功整理了 2 集（E01/E03）。
	if len(res.Files) != 2 {
		t.Fatalf("expected 2 organized episodes, got %d (%+v)", len(res.Files), res)
	}
	// 但 Success=false 且 ErrMsg 标识缺集 S01E02。
	if res.Success {
		t.Fatal("partial failure should not be Success=true")
	}
	if !strings.Contains(res.ErrMsg, "S01E02") {
		t.Fatalf("ErrMsg should mention missing S01E02, got %q", res.ErrMsg)
	}
}

// [P2] #3 copyFile 在 src 不存在时返回错误且不残留 dst。
func TestCopyFile_NoResidueOnFailure(t *testing.T) {
	tmp := t.TempDir()
	dst := filepath.Join(tmp, "sub", "dst.bin")
	if err := copyFile(filepath.Join(tmp, "no-such-src"), dst); err == nil {
		t.Fatal("expected error for missing src")
	}
	if pathExists(dst) {
		t.Fatalf("partial dst should be cleaned, found: %s", dst)
	}
}

// [P2] #3 copyFile：dst 与 src 同 inode（已是硬链到自身）时跳过，不截断自毁源。
func TestCopyFile_SameInodeSkips(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "a.bin")
	_ = os.WriteFile(src, []byte("DATA"), 0o644)
	// 建立同 inode 的硬链。
	hard := filepath.Join(tmp, "b.bin")
	if err := os.Link(src, hard); err != nil {
		t.Fatal(err)
	}
	// hard 与 src 同 inode；对 hard 做 copyFile(src, hard) 应跳过，源内容不被截断。
	if err := copyFile(src, hard); err != nil {
		t.Fatalf("same-inode copy should skip: %v", err)
	}
	got, _ := os.ReadFile(src)
	if string(got) != "DATA" {
		t.Fatalf("source truncated! got %q", got)
	}
}

// [P2] #6 Move 删源失败但 dst 等大时幂等成功（不卡死重试）。
func TestMove_IdempotentWhenSourceUnlinkFails(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.bin")
	dst := filepath.Join(tmp, "dst.bin")
	_ = os.WriteFile(src, []byte("DATA"), 0o644)
	// 用 chmod 0500 让父目录不可写 → os.Remove(src) 失败；copyFile 已成功 → dst 完整。
	// 注意：恢复权限以保证后续清理。
	t.Cleanup(func() { _ = os.Chmod(tmp, 0o755) })
	if err := os.Chmod(tmp, 0o500); err != nil {
		t.Fatal(err)
	}
	err := (LinkOps{}).Move(src, dst)
	// 多数 OS 下删除受目录写权限限制；若恰好放行则跳过断言。
	if err != nil {
		t.Fatalf("move should be idempotent success when dst complete, got %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "DATA" {
		t.Fatalf("dst content mismatch: %q", got)
	}
}

// [P2] #2 replaceViaTemp 成功路径：链到 tmp → 删旧 → rename。
func TestReplaceViaTemp_Success(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "new.mkv")
	old := filepath.Join(tmp, "old.mkv")
	dst := filepath.Join(tmp, "final.mkv")
	_ = os.WriteFile(src, []byte("NEW"), 0o644)
	_ = os.WriteFile(old, []byte("OLD"), 0o644)

	var replaced []string
	if err := replaceViaTemp(ModeCopy, src, dst, old, &replaced); err != nil {
		t.Fatalf("replaceViaTemp: %v", err)
	}
	if len(replaced) != 1 || replaced[0] != old {
		t.Fatalf("replaced = %v, want [%s]", replaced, old)
	}
	if pathExists(old) {
		t.Fatal("old should be removed")
	}
	if pathExists(dst + ".new") {
		t.Fatal("temp file should be renamed away")
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "NEW" {
		t.Fatalf("dst content = %q", got)
	}
}

// [P2] #2 replaceViaTemp 链失败时旧库文件不丢、无残临时文件。
func TestReplaceViaTemp_LinkFailureKeepsOld(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "no-such.mkv") // src 不存在 → Apply 失败
	old := filepath.Join(tmp, "old.mkv")
	dst := filepath.Join(tmp, "final.mkv")
	_ = os.WriteFile(old, []byte("OLD"), 0o644)

	var replaced []string
	err := replaceViaTemp(ModeCopy, missing, dst, old, &replaced)
	if err == nil {
		t.Fatal("expected link failure")
	}
	if !pathExists(old) {
		t.Fatal("old should NOT be removed when link fails")
	}
	if len(replaced) != 0 {
		t.Fatalf("replaced should be empty on failure, got %v", replaced)
	}
	if pathExists(dst + ".new") {
		t.Fatal("temp file should be cleaned on link failure")
	}
}

// [P2] #7 sanitizePath 拒绝裸 .. / . 段（防路径穿越：Title 恰为 ".."/"." 时返回空）。
// 注：sanitizePath 已把 / \ 替换为空格，含分隔符的穿越串（如 "../etc/passwd"）会被消解为
// 单段非危险字符串；此处只校验字面量 ".."/"." 被拒绝。
func TestSanitizePath_RejectsTraversal(t *testing.T) {
	for _, c := range []string{"..", "."} {
		if got := sanitizePath(c); got != "" {
			t.Fatalf("sanitizePath(%q) = %q, want empty", c, got)
		}
	}
	// 正常标题不受影响。
	if got := sanitizePath("Inception (2010)"); got != "Inception (2010)" {
		t.Fatalf("normal title mangled: %q", got)
	}
	// 含分隔符的穿越串被替换为空格，不再构成路径穿越。
	if got := sanitizePath("../etc/passwd"); strings.Contains(got, "/") || strings.Contains(got, "\\") {
		t.Fatalf("separator should be stripped: %q", got)
	}
}

// [P2] #8 joinNonEmpty 多元素须用 sep 拼接（修复前恒返回首个元素，丢集号）。
func TestJoinNonEmpty_MultipleUsesSep(t *testing.T) {
	if got := joinNonEmpty([]string{"S01E01", "S01E02", "S01E03"}, ","); got != "S01E01,S01E02,S01E03" {
		t.Fatalf("multi join = %q", got)
	}
	if got := joinNonEmpty([]string{"S01E01", "", "S01E02"}, " "); got != "S01E01 S01E02" {
		t.Fatalf("skip empty join = %q", got)
	}
	if got := joinNonEmpty([]string{"", ""}, " "); got != "" {
		t.Fatalf("all empty = %q want empty", got)
	}
	if got := joinNonEmpty([]string{"S01E01"}, " "); got != "S01E01" {
		t.Fatalf("single = %q", got)
	}
}
