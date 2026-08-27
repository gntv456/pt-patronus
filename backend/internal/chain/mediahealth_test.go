package chain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hxpt/ptpatronus/internal/mediaserver"
)

// TestHasSubtitleNextTo 字幕同目录判定：有 .srt→true；无→false；.strm→true（跳过判定）；
// 目录不可达→true（不误报）。
func TestHasSubtitleNextTo(t *testing.T) {
	dir := t.TempDir()
	// 无字幕
	if err := os.WriteFile(filepath.Join(dir, "a.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hasSubtitleNextTo(filepath.Join(dir, "a.mkv")) {
		t.Error("no subtitle should be false")
	}
	// 加 srt
	if err := os.WriteFile(filepath.Join(dir, "a.chs.srt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasSubtitleNextTo(filepath.Join(dir, "a.mkv")) {
		t.Error("with srt should be true")
	}
	// strm 跳过
	if !hasSubtitleNextTo("/media/strm/b.mkv.strm") {
		t.Error("strm should skip (true)")
	}
	// 目录不可达不误报
	if !hasSubtitleNextTo("/nonexistent-dir-xyz/c.mkv") {
		t.Error("unreadable dir should not report missing")
	}
}

// TestCountIssues 分类计数。
func TestCountIssues(t *testing.T) {
	issues := []HealthIssue{
		{Type: "unmatched"}, {Type: "unmatched"}, {Type: "missing_subtitle"},
	}
	if countIssues(issues, "unmatched") != 2 || countIssues(issues, "missing_subtitle") != 1 {
		t.Error("count wrong")
	}
}

// TestQualityRank 分辨率等级：2160/4K/UHD=4、1080=3、720=2、480/SD=1、不可识别=0。
func TestQualityRank(t *testing.T) {
	cases := map[string]int{
		"/media/Movie.2020.2160p.WEB-DL.mkv": 4,
		"/media/Movie.4K.BluRay.mkv":         4,
		"/media/Movie.UHD.mkv":               4,
		"/media/Movie.1080p.mkv":             3,
		"/media/Movie.720p.mkv":              2,
		"/media/Movie.480p.mkv":              1,
		"/media/Movie.SDTV.mkv":              1,
		"/media/Movie.mkv":                   0,
	}
	for p, want := range cases {
		if got := qualityRank(p); got != want {
			t.Errorf("qualityRank(%s)=%d want %d", p, got, want)
		}
	}
}

// TestDetectUpgradable 洗版检测：同 TMDB 组内低于最高版本的条目记 upgradeable；
// 无路径 / 无法判级 / 不同 TMDB 的不参与。
func TestDetectUpgradable(t *testing.T) {
	items := []mediaserver.DetailedItem{
		{ItemID: "1", Name: "A", TMDBID: "100", Path: "/media/A.720p.mkv"},
		{ItemID: "2", Name: "A", TMDBID: "100", Path: "/media/A.2160p.mkv"},
		{ItemID: "3", Name: "B", TMDBID: "200", Path: "/media/B.1080p.mkv"}, // 单条不报
		{ItemID: "4", Name: "C", TMDBID: "300"},                             // 无路径跳过
	}
	rep := &HealthReport{Issues: []HealthIssue{}}
	detectUpgradable(1, "电影", items, rep)
	if len(rep.Issues) != 1 {
		t.Fatalf("expect 1 upgradeable issue, got %d", len(rep.Issues))
	}
	iss := rep.Issues[0]
	if iss.ItemID != "1" || iss.Type != "upgradeable" {
		t.Errorf("expect item 1 upgradeable, got %+v", iss)
	}
}
