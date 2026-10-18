package chain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"gorm.io/gorm"
)

// fakeSubtitleFixer 默认恒定命中一条 chs 字幕的补齐依赖；三个开关分别模拟
// 「字幕源查询失败」「真的没找到」「下载失败」，验补齐账面把三者区分开。
type fakeSubtitleFixer struct {
	hits        int
	searchErr   error // 非 nil：Search 返回该错误（字幕源全崩）
	noResult    bool  // true：Search 返回空列表（这片确实没字幕）
	downloadErr error // 非 nil：Download 返回该错误
}

func (f *fakeSubtitleFixer) Search(context.Context, string, string) ([]subtitleItem, error) {
	f.hits++
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	if f.noResult {
		return nil, nil
	}
	return []subtitleItem{{Source: "fake", SourceID: "1", Title: "sub", LangCode: "chs", Ext: ".srt"}}, nil
}

func (f *fakeSubtitleFixer) Download(context.Context, subtitleItem) ([]byte, string, error) {
	if f.downloadErr != nil {
		return nil, "", f.downloadErr
	}
	return []byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"), "sub.chs.srt", nil
}

// waitSubtitleFix 等后台补齐收尾（FixMissingSubtitles 立即返回受理快照，计数在 goroutine 里更新）。
func waitSubtitleFix(t *testing.T, c *MediaServerChain) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for c.subtitleFix.running.Load() {
		if time.Now().After(deadline) {
			t.Fatal("subtitle fix still running after 5s")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestFixMissingSubtitlesSkipsDirectoryPaths 「一键补齐」只接受媒体文件 Path：
// 剧集条目的 Path 是剧集目录（健康报告里由服务端写入，旧报告/客户端提交的 body 也可能带目录），
// 对其补齐会把字幕写到其父目录＝库根，留下「剧集名.chs.srt」幻影文件；该文件随后又被
// hasSubtitleNextTo 当成「本库已有字幕」，把全库缺字幕检测永久遮蔽。
func TestFixMissingSubtitlesSkipsDirectoryPaths(t *testing.T) {
	ch, _, _, _ := newMediaServerTestChain(t)
	fixer := &fakeSubtitleFixer{}
	ch.SetSubtitleFixer(fixer)

	libRoot := t.TempDir()
	movieDir := filepath.Join(libRoot, "Movie.2020")
	seriesDir := filepath.Join(libRoot, "Show (2008)")
	if err := os.MkdirAll(movieDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(seriesDir, "Season 01"), 0o755); err != nil {
		t.Fatal(err)
	}
	moviePath := filepath.Join(movieDir, "Movie.2020.1080p.mkv")
	if err := os.WriteFile(moviePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep := &HealthReport{Issues: []HealthIssue{
		{ServerID: 1, Type: "missing_subtitle", ItemID: "m1", Path: moviePath},
		{ServerID: 1, Type: "missing_subtitle", ItemID: "s1", Title: "Show", Path: seriesDir},
		{ServerID: 1, Type: "unmatched", ItemID: "s2", Title: "Other", Path: filepath.Join(libRoot, "Other")},
	}}
	out, err := ch.FixMissingSubtitles(context.Background(), rep, "chs")
	if err != nil {
		t.Fatal(err)
	}
	waitSubtitleFix(t, ch)

	// 电影字幕正常落盘（正控：过滤不能把该补的也拦掉）
	wantSub := filepath.Join(movieDir, "Movie.2020.1080p.chs.srt")
	if _, err := os.Stat(wantSub); err != nil {
		t.Errorf("movie subtitle not written: %v", err)
	}
	// 库根不得出现幻影字幕
	phantom := filepath.Join(libRoot, "Show (2008).chs.srt")
	if _, err := os.Stat(phantom); err == nil {
		t.Errorf("phantom subtitle written to library root: %s", phantom)
	}
	ch.subtitleFix.mu.Lock()
	fixed, skipped := out.Fixed, out.Skipped
	ch.subtitleFix.mu.Unlock()
	if fixed != 1 || skipped != 0 {
		t.Errorf("fixed=%d skipped=%d, want 1/0（目录条目应整条剔除，不计入）", fixed, skipped)
	}
	if fixer.hits != 1 {
		t.Errorf("search calls=%d want 1（目录条目不应发起搜索）", fixer.hits)
	}
	// 未挂载/不可访问的媒体路径不做本地判定：库根没有字幕目录残留
	if entries, err := os.ReadDir(libRoot); err == nil {
		for _, e := range entries {
			switch filepath.Ext(e.Name()) {
			case ".srt", ".ass", ".ssa", ".sub":
				t.Errorf("unexpected subtitle file at library root: %s", e.Name())
			}
		}
	}
}

// TestFixMissingSubtitlesNoTargetsIsNoop 全部目标被过滤时不启 goroutine（返回空报告、不占重入锁）。
func TestFixMissingSubtitlesNoTargetsIsNoop(t *testing.T) {
	ch, _, _, _ := newMediaServerTestChain(t)
	fixer := &fakeSubtitleFixer{}
	ch.SetSubtitleFixer(fixer)

	out, err := ch.FixMissingSubtitles(context.Background(), &HealthReport{
		Issues: []HealthIssue{{Type: "missing_subtitle", Title: "Show", Path: t.TempDir()}},
	}, "chs")
	if err != nil {
		t.Fatal(err)
	}
	if out.Fixed != 0 || out.Skipped != 0 {
		t.Errorf("fixed=%d skipped=%d want 0/0", out.Fixed, out.Skipped)
	}
	if ch.subtitleFix.running.Load() {
		t.Error("running flag should stay free when nothing to fix")
	}
	if fixer.hits != 0 {
		t.Errorf("search calls=%d want 0", fixer.hits)
	}
}

// newSubtitleFixNoticeChain 带通知中心的补齐链（后台补齐的结果只能靠通知到达用户——
// 前端只弹「已受理」，从不轮询这份报告）。通知落独立的库，便于直接查表断言正文。
func newSubtitleFixNoticeChain(t *testing.T) (*MediaServerChain, *gorm.DB) {
	t.Helper()
	msDB, err := gorm.Open(sqlite.Open(t.TempDir()+"/ms.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := msDB.AutoMigrate(&models.MediaServer{}); err != nil {
		t.Fatal(err)
	}
	noticeDB, err := gorm.Open(sqlite.Open(t.TempDir()+"/notice.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := noticeDB.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, d := range []*gorm.DB{msDB, noticeDB} {
			if sqlDB, e := d.DB(); e == nil {
				_ = sqlDB.Close()
			}
		}
	})
	op := oper.NewMediaServerOper(msDB, security.NewCrypto("subtitlefix-seed"))
	ns := notice.NewService(oper.NewNoticeOper(noticeDB), nil, nil)
	ch, err := NewMediaServerChain(NewBase(module.New(), event.New()), op, ns)
	if err != nil {
		t.Fatal(err)
	}
	return ch, noticeDB
}

// oneSubtitleTarget 一条可补齐的媒体文件条目（目录 Path 会被整条剔除，不参与补齐）。
func oneSubtitleTarget(t *testing.T) *HealthReport {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Movie.2020.1080p.mkv")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &HealthReport{Issues: []HealthIssue{{ServerID: 1, Type: "missing_subtitle", ItemID: "m1", Title: "Movie", Path: path}}}
}

// lastSubtitleFixNotice 取最后一条字幕补齐通知（正文是用户唯一能看到的账面）。
func lastSubtitleFixNotice(t *testing.T, noticeDB *gorm.DB) models.Notice {
	t.Helper()
	var n models.Notice
	if err := noticeDB.Where("source = ?", notice.SourceSubtitleFix).Order("id desc").First(&n).Error; err != nil {
		t.Fatalf("补齐结束必须发一条通知: %v", err)
	}
	return n
}

// TestFixMissingSubtitlesRecordsSearchFailure 字幕源查询失败必须进 Errors 并在通知正文点名。
// 修复前 `err != nil || len(subs) == 0` 合成同一个 Skipped++（只 Debug），于是
// 「4 个源全崩」和「这些片本来就没字幕」在通知里长得一模一样（#142 链层新加的中文原因也被这层抹平）。
func TestFixMissingSubtitlesRecordsSearchFailure(t *testing.T) {
	ch, noticeDB := newSubtitleFixNoticeChain(t)
	ch.SetSubtitleFixer(&fakeSubtitleFixer{searchErr: errors.New("全部 4 个字幕源查询失败")})
	out, err := ch.FixMissingSubtitles(context.Background(), oneSubtitleTarget(t), "chs")
	if err != nil {
		t.Fatal(err)
	}
	waitSubtitleFix(t, ch)

	ch.subtitleFix.mu.Lock()
	errCount, skipped, first := len(out.Errors), out.Skipped, ""
	if errCount > 0 {
		first = out.Errors[0]
	}
	ch.subtitleFix.mu.Unlock()
	if errCount != 1 {
		t.Fatalf("搜索失败要进 Errors，got errors=%d", errCount)
	}
	if skipped != 0 {
		t.Errorf("搜索失败不能计成跳过，got skipped=%d", skipped)
	}
	if !strings.Contains(first, "字幕搜索失败") || !strings.Contains(first, "全部 4 个字幕源查询失败") {
		t.Errorf("Errors 要带中文前缀与源原因，got %q", first)
	}
	n := lastSubtitleFixNotice(t, noticeDB)
	if !strings.Contains(n.Body, "失败 1") || !strings.Contains(n.Body, "字幕搜索失败") {
		t.Errorf("通知正文必须带失败数与原因，got body=%q", n.Body)
	}
	if n.Level != "warning" {
		t.Errorf("有失败时通知级别应为 warning，got %q", n.Level)
	}
}

// TestFixMissingSubtitlesRecordsDownloadFailure 下载失败同样进 Errors（修复前既计 Skipped
// 又一行日志都不留）。
func TestFixMissingSubtitlesRecordsDownloadFailure(t *testing.T) {
	ch, noticeDB := newSubtitleFixNoticeChain(t)
	ch.SetSubtitleFixer(&fakeSubtitleFixer{downloadErr: errors.New("字幕站返回的是登录页")})
	out, err := ch.FixMissingSubtitles(context.Background(), oneSubtitleTarget(t), "chs")
	if err != nil {
		t.Fatal(err)
	}
	waitSubtitleFix(t, ch)

	ch.subtitleFix.mu.Lock()
	errCount, skipped, first := len(out.Errors), out.Skipped, ""
	if errCount > 0 {
		first = out.Errors[0]
	}
	ch.subtitleFix.mu.Unlock()
	if errCount != 1 || skipped != 0 {
		t.Fatalf("下载失败应进 Errors 且不进 Skipped，got errors=%d skipped=%d", errCount, skipped)
	}
	if !strings.Contains(first, "字幕下载失败") || !strings.Contains(first, "登录页") {
		t.Errorf("Errors 要带中文前缀与原因，got %q", first)
	}
	if n := lastSubtitleFixNotice(t, noticeDB); !strings.Contains(n.Body, "失败 1") {
		t.Errorf("通知正文要带失败数，got body=%q", n.Body)
	}
}

// TestFixMissingSubtitlesNoResultIsSkipped 反向对照：真的没搜到字幕 → 计 Skipped、不进 Errors、
// 通知正文不出现「失败」（防我把「没找到」也报成故障）。
func TestFixMissingSubtitlesNoResultIsSkipped(t *testing.T) {
	ch, noticeDB := newSubtitleFixNoticeChain(t)
	ch.SetSubtitleFixer(&fakeSubtitleFixer{noResult: true})
	out, err := ch.FixMissingSubtitles(context.Background(), oneSubtitleTarget(t), "chs")
	if err != nil {
		t.Fatal(err)
	}
	waitSubtitleFix(t, ch)

	ch.subtitleFix.mu.Lock()
	errCount, skipped := len(out.Errors), out.Skipped
	ch.subtitleFix.mu.Unlock()
	if errCount != 0 || skipped != 1 {
		t.Fatalf("没搜到应只计跳过，got errors=%d skipped=%d", errCount, skipped)
	}
	if n := lastSubtitleFixNotice(t, noticeDB); strings.Contains(n.Body, "失败") {
		t.Errorf("「没找到」不该被写成失败，got body=%q", n.Body)
	}
}
