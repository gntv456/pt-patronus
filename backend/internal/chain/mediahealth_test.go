package chain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/oper"
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

// fakeEpCounter 固定集数的 episodeCounter（缺集判定与字幕判定解耦）。
type fakeEpCounter struct{ n int }

func (f fakeEpCounter) EpisodeCount(context.Context, string) (int, error) { return f.n, nil }

// TestAppendItemIssuesSubtitleScan 字幕扫描只对「Path 本身就是媒体文件」的条目生效：
// 电影条目 Path=媒体文件 → 无字幕时记 missing_subtitle、有字幕时不记；
// 剧集条目 Path=剧集目录 → 一律不记（旧行为会对目录做同目录判定，读到其父目录即库根，
// 于是整个剧库的每部剧集都刷出假阳性，且「一键补齐」会把字幕文件写到库根）。
func TestAppendItemIssuesSubtitleScan(t *testing.T) {
	libRoot := t.TempDir()

	movieDir := filepath.Join(libRoot, "Movie.2020")
	if err := os.MkdirAll(movieDir, 0o755); err != nil {
		t.Fatal(err)
	}
	moviePath := filepath.Join(movieDir, "Movie.2020.1080p.mkv")
	if err := os.WriteFile(moviePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	movie := mediaserver.DetailedItem{ItemID: "m1", Name: "Movie 2020", Type: "movie", TMDBID: "10", Path: moviePath}
	// 电影无字幕 → 报缺字幕（正控：修复不能把正常检测一起关掉）
	rep := &HealthReport{Issues: []HealthIssue{}}
	appendItemIssues(context.Background(), 1, "movies", "电影", movie, fakeEpCounter{n: 5}, true, rep)
	if countIssues(rep.Issues, "missing_subtitle") != 1 {
		t.Fatalf("movie without subtitle should report, got %+v", rep.Issues)
	}
	if rep.Issues[0].Path != moviePath {
		t.Errorf("issue path=%s want %s", rep.Issues[0].Path, moviePath)
	}

	// 补上同名字幕 → 不再报（正控：判定本身仍有效）
	if err := os.WriteFile(filepath.Join(movieDir, "Movie.2020.1080p.chs.srt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep = &HealthReport{Issues: []HealthIssue{}}
	appendItemIssues(context.Background(), 1, "movies", "电影", movie, fakeEpCounter{n: 5}, true, rep)
	if len(rep.Issues) != 0 {
		t.Errorf("movie with subtitle should be clean, got %+v", rep.Issues)
	}

	// 剧集条目：Path 是剧集目录（父目录=库根，库根无字幕文件）→ 不记缺字幕
	seriesDir := filepath.Join(libRoot, "Show (2008)")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(seriesDir, "Season 01"), 0o755); err != nil {
		t.Fatal(err)
	}
	series := mediaserver.DetailedItem{ItemID: "s1", Name: "Show", Type: "series", TMDBID: "20", Path: seriesDir}
	rep = &HealthReport{Issues: []HealthIssue{}}
	appendItemIssues(context.Background(), 1, "tvshows", "剧集", series, fakeEpCounter{n: 5}, true, rep)
	if countIssues(rep.Issues, "missing_subtitle") != 0 {
		t.Errorf("series dir path must not produce missing_subtitle, got %+v", rep.Issues)
	}
	if len(rep.Issues) != 0 {
		t.Errorf("series with 5 episodes should be clean, got %+v", rep.Issues)
	}

	// 字幕扫描关闭时不产出缺字幕（保持原有开关语义）
	rep = &HealthReport{Issues: []HealthIssue{}}
	appendItemIssues(context.Background(), 1, "movies", "电影", movie, fakeEpCounter{n: 5}, false, rep)
	if len(rep.Issues) != 0 {
		t.Errorf("subtitleScan off should not scan, got %+v", rep.Issues)
	}
}

// healthFakeServer 假 Emby：libsOK 时 /Library/MediaFolders 返回一个电影库，
// itemsStatus 控制 /Items 的响应码（用于构造「库读不出」）。
func healthFakeServer(t *testing.T, libsOK bool, itemsStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Library/MediaFolders":
			if !libsOK {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{
				{"Name": "电影", "Id": "lib1", "CollectionType": "movies"},
			}})
		case "/Items":
			if itemsStatus != http.StatusOK {
				w.WriteHeader(itemsStatus)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{
				{"Id": "m1", "Name": "未刮削的片", "Type": "Movie", "Path": "/media/a.mkv"},
			}})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

// addHealthServer 往测试链里注册一台指向 baseURL 的 Emby。
func addHealthServer(t *testing.T, op *oper.MediaServerOper, name, baseURL string) {
	t.Helper()
	if _, err := op.Create(oper.MediaServerInput{Name: name, Type: "emby", URL: baseURL, Token: "t"}); err != nil {
		t.Fatalf("Create %s: %v", name, err)
	}
}

// TestHealthCheckSurfacesUnreachableServer 服务器连不上时必须把原因回传：
// 旧实现静默跳过 → 报告 0 问题 → 前端绿色「一切正常」，用户以为库很干净而实际一台都没扫。
func TestHealthCheckSurfacesUnreachableServer(t *testing.T) {
	srv := healthFakeServer(t, false, http.StatusOK)
	defer srv.Close()
	ch, op, _, _ := newMediaServerTestChain(t)
	addHealthServer(t, op, "飞牛影视", srv.URL)

	rep := ch.HealthCheck(context.Background(), false, false)
	if len(rep.Issues) != 0 {
		t.Fatalf("前提：连不上的服务器不该产出问题，got %+v", rep.Issues)
	}
	if len(rep.Errors) == 0 {
		t.Fatal("不可达服务器必须回传失败原因（旧行为：静默全绿）")
	}
	if !strings.Contains(rep.Errors[0], "飞牛影视") {
		t.Errorf("失败原因里要带服务器名，got %q", rep.Errors[0])
	}
}

// TestHealthCheckSurfacesFailedLibrary 库列表能读但条目读不出（单库故障）同样要回传，
// 并带上库名——否则用户无法判断是哪一部分没扫到。
func TestHealthCheckSurfacesFailedLibrary(t *testing.T) {
	srv := healthFakeServer(t, true, http.StatusServiceUnavailable)
	defer srv.Close()
	ch, op, _, _ := newMediaServerTestChain(t)
	addHealthServer(t, op, "Emby", srv.URL)

	rep := ch.HealthCheck(context.Background(), false, false)
	if len(rep.Errors) == 0 {
		t.Fatal("库条目读取失败必须回传原因")
	}
	if !strings.Contains(rep.Errors[0], "电影") {
		t.Errorf("失败原因里要带库名，got %q", rep.Errors[0])
	}
}

// TestHealthCheckSurfacesServerListFailure 媒体服务器表读不出（DB 故障）也要回传：
// 这一层失败连「有几台」都不知道，更不能显示成健康。
func TestHealthCheckSurfacesServerListFailure(t *testing.T) {
	ch, op, db, _ := newMediaServerTestChain(t)
	addHealthServer(t, op, "Emby", "http://127.0.0.1:1")
	if err := db.Exec("DROP TABLE media_servers").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := op.List(); err == nil {
		t.Fatal("前提：DROP TABLE 后 List 应报错")
	}

	rep := ch.HealthCheck(context.Background(), false, false)
	if len(rep.Errors) == 0 {
		t.Error("服务器列表读取失败必须回传原因")
	}
}

// TestHealthCheckCleanServerHasNoErrors 正控：正常服务器的 errors 为空，
// 且真实问题照常算出（修复不能把成功路径一起标成失败）。
func TestHealthCheckCleanServerHasNoErrors(t *testing.T) {
	srv := healthFakeServer(t, true, http.StatusOK)
	defer srv.Close()
	ch, op, _, _ := newMediaServerTestChain(t)
	addHealthServer(t, op, "Emby", srv.URL)

	rep := ch.HealthCheck(context.Background(), false, false)
	if len(rep.Errors) != 0 {
		t.Errorf("正常巡检不该有 errors，got %v", rep.Errors)
	}
	if countIssues(rep.Issues, "unmatched") != 1 {
		t.Errorf("未匹配条目应被算出，got %+v", rep.Issues)
	}
}
