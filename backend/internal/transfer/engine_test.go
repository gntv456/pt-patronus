package transfer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hxpt/ptpatronus/internal/media"
)

// memKV 内存 KVStore，用于测试。
type memKV struct{ m map[string]string }

func (m *memKV) Get(k string) (string, error) { return m.m[k], nil }
func (m *memKV) Set(k, v string) error        { m.m[k] = v; return nil }

type fakeRecognizer struct{ mi media.MediaInfo }

func (f fakeRecognizer) Search(_ context.Context, _ string, _ int, _ string) ([]media.MediaInfo, error) {
	return []media.MediaInfo{f.mi}, nil
}
func (f fakeRecognizer) Detail(_ context.Context, _, _ string) (*media.MediaInfo, error) {
	mi := f.mi
	return &mi, nil
}

func newTestEngine(t *testing.T, mi media.MediaInfo) (*Engine, *memKV) {
	t.Helper()
	kv := &memKV{m: map[string]string{}}
	rec := fakeRecognizer{mi: mi}
	fetch := func(_ context.Context, _ string) ([]byte, error) { return []byte("IMG"), nil }
	return NewEngine(rec, nil, kv, fetch, nil), kv
}

func TestEngine_ProcessMovie_HardlinkNFO(t *testing.T) {
	tmp := t.TempDir()
	savePath := filepath.Join(tmp, "downloads")
	if err := os.MkdirAll(savePath, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(savePath, "Inception.2010.1080p.BluRay.mkv")
	if err := os.WriteFile(src, []byte("MKV"), 0o644); err != nil {
		t.Fatal(err)
	}
	movieDir := filepath.Join(tmp, "movies")

	// 配置：开 TypeFolder，movie_dir 指向 tmp/movies。
	kv := &memKV{m: map[string]string{}}
	cfg := DefaultConfig()
	cfg.MovieDir = movieDir
	_ = Save(kv, cfg)

	mi := media.MediaInfo{Source: "tmdb", SourceID: "movie:123", Title: "Inception", Year: 2010, Type: media.TypeMovie, PosterURL: "http://x/p.jpg", Genres: []string{"Sci-Fi"}}
	eng := NewEngine(fakeRecognizer{mi: mi}, nil, kv, func(_ context.Context, _ string) ([]byte, error) { return []byte("IMG"), nil }, nil)

	res := eng.Process(context.Background(), ProcessInput{
		Hash: "h1", Name: "Inception.2010.1080p.BluRay.mkv", SavePath: savePath,
		Files: []FileInput{{Path: src, Size: 3}},
	})

	if !res.Success {
		t.Fatalf("expected success, got err=%q", res.ErrMsg)
	}
	// 目标文件存在
	if !pathExists(res.Files[0]) {
		t.Fatalf("dest file not found: %s", res.Files[0])
	}
	// 源仍存在（硬链不删源）
	if !pathExists(src) {
		t.Fatal("source removed in link mode")
	}
	// NFO + 海报写入
	if !pathExists(filepath.Join(res.DestDir, "movie.nfo")) {
		t.Fatal("movie.nfo not written")
	}
	if !pathExists(filepath.Join(res.DestDir, "poster.jpg")) {
		t.Fatal("poster.jpg not written")
	}
}

func TestEngine_ProcessTV_EpisodesAndShowNFO(t *testing.T) {
	tmp := t.TempDir()
	savePath := filepath.Join(tmp, "downloads")
	_ = os.MkdirAll(savePath, 0o755)
	ep1 := filepath.Join(savePath, "Breaking.Bad.S01E01.1080p.mkv")
	ep2 := filepath.Join(savePath, "Breaking.Bad.S01E02.1080p.mkv")
	_ = os.WriteFile(ep1, []byte("A"), 0o644)
	_ = os.WriteFile(ep2, []byte("B"), 0o644)
	tvDir := filepath.Join(tmp, "tv")

	kv := &memKV{m: map[string]string{}}
	cfg := DefaultConfig()
	cfg.TVDir = tvDir
	_ = Save(kv, cfg)

	mi := media.MediaInfo{Source: "tmdb", SourceID: "tv:123", Title: "Breaking Bad", Year: 2008, Type: media.TypeTV, PosterURL: "http://x/p.jpg"}
	eng := NewEngine(fakeRecognizer{mi: mi}, nil, kv, func(_ context.Context, _ string) ([]byte, error) { return []byte("IMG"), nil }, nil)

	res := eng.Process(context.Background(), ProcessInput{
		Hash: "h2", Name: "Breaking.Bad.S01", SavePath: savePath,
		Files: []FileInput{{Path: ep1, Size: 1}, {Path: ep2, Size: 1}},
	})
	if !res.Success {
		t.Fatalf("expected success, got err=%q", res.ErrMsg)
	}
	if len(res.Files) != 2 {
		t.Fatalf("expected 2 episodes organized, got %d", len(res.Files))
	}
	// 剧根有 tvshow.nfo + poster.jpg
	if !pathExists(filepath.Join(res.DestDir, "tvshow.nfo")) {
		t.Fatal("tvshow.nfo not written at show root")
	}
	if !pathExists(filepath.Join(res.DestDir, "poster.jpg")) {
		t.Fatal("show poster not written")
	}
	// 集文件落在 Season 01 子目录
	for _, f := range res.Files {
		if !contains(f, "Season 01") {
			t.Fatalf("episode not under Season 01: %s", f)
		}
	}
}

func TestEngine_NoVideoFiles(t *testing.T) {
	eng, _ := newTestEngine(t, media.MediaInfo{Title: "X", Type: media.TypeMovie})
	res := eng.Process(context.Background(), ProcessInput{
		Files: []FileInput{{Path: "/tmp/readme.txt", Size: 1}},
	})
	if res.Success {
		t.Fatal("should fail on no video")
	}
}

// TestEngine_ProcessMovie_M2TS .m2ts（BD remux 常见格式）必须可整理：
// 旧实现 videoExts 缺 .m2ts，纯 m2ts 目录被 dirwatch 反复发现却恒报
// "no video files to organize"，失败历史/通知循环刷屏且永不入库。
func TestEngine_ProcessMovie_M2TS(t *testing.T) {
	tmp := t.TempDir()
	savePath := filepath.Join(tmp, "downloads")
	if err := os.MkdirAll(savePath, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(savePath, "Interstellar.2014.2160p.BluRay.REMUX.HDR.m2ts")
	if err := os.WriteFile(src, []byte("M2TS"), 0o644); err != nil {
		t.Fatal(err)
	}
	movieDir := filepath.Join(tmp, "movies")

	kv := &memKV{m: map[string]string{}}
	cfg := DefaultConfig()
	cfg.MovieDir = movieDir
	_ = Save(kv, cfg)

	mi := media.MediaInfo{Source: "tmdb", SourceID: "movie:157336", Title: "Interstellar", Year: 2014, Type: media.TypeMovie, PosterURL: "http://x/p.jpg"}
	eng := NewEngine(fakeRecognizer{mi: mi}, nil, kv, func(_ context.Context, _ string) ([]byte, error) { return []byte("IMG"), nil }, nil)

	res := eng.Process(context.Background(), ProcessInput{
		Hash: "hm2ts", Name: "Interstellar.2014.2160p.BluRay.REMUX.HDR.m2ts", SavePath: savePath,
		Files: []FileInput{{Path: src, Size: 4}},
	})

	if !res.Success {
		t.Fatalf("m2ts 应可整理, got err=%q", res.ErrMsg)
	}
	if len(res.Files) != 1 || !pathExists(res.Files[0]) {
		t.Fatalf("m2ts 目标文件未生成: %v", res.Files)
	}
}

func TestLinkOps_HardlinkCrossDeviceFallback(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "a.bin")
	_ = os.WriteFile(src, []byte("DATA"), 0o644)
	dst := filepath.Join(tmp, "sub", "b.bin")
	if err := (LinkOps{}).HardLink(src, dst); err != nil {
		t.Fatalf("hardlink: %v", err)
	}
	if !pathExists(dst) {
		t.Fatal("dest not created")
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

// TestEngine_RerunAlreadyInLibraryIsSkipNotFailure 换源/换下载器重下同一部片（hash 变了，
// 成功记录不命中会重跑到引擎）：库里已有目标即「无事可做」，不该记成整理失败
// （failed 会按冷却每轮重试并反复发「整理失败」通知，而内容完好）。
func TestEngine_RerunAlreadyInLibraryIsSkipNotFailure(t *testing.T) {
	tmp := t.TempDir()
	savePath := filepath.Join(tmp, "downloads")
	_ = os.MkdirAll(savePath, 0o755)
	src := filepath.Join(savePath, "Interstellar.2014.1080p.BluRay.mkv")
	_ = os.WriteFile(src, []byte("MKV"), 0o644)

	kv := &memKV{m: map[string]string{}}
	cfg := DefaultConfig()
	cfg.MovieDir = filepath.Join(tmp, "movies")
	_ = Save(kv, cfg)
	mi := media.MediaInfo{Source: "tmdb", SourceID: "movie:9", Title: "Interstellar", Year: 2014, Type: media.TypeMovie}
	eng := NewEngine(fakeRecognizer{mi: mi}, nil, kv, func(_ context.Context, _ string) ([]byte, error) { return []byte("IMG"), nil }, nil)

	in := ProcessInput{Hash: "hash-A", Name: "Interstellar.2014.1080p.BluRay.mkv", SavePath: savePath, Files: []FileInput{{Path: src, Size: 3}}}
	first := eng.Process(context.Background(), in)
	if !first.Success || len(first.Files) != 1 {
		t.Fatalf("首轮应整理成功: success=%v files=%d err=%q", first.Success, len(first.Files), first.ErrMsg)
	}

	// 同一内容换个 hash 再来一遍（换源重下的典型形态）
	in.Hash = "hash-B"
	again := eng.Process(context.Background(), in)
	if again.Success {
		t.Error("重跑不该报成功（本轮没落任何文件）")
	}
	if again.ErrMsg != "" {
		t.Errorf("库里已有应记 Skipped 而非失败, got ErrMsg=%q（旧行为：target exists 当失败）", again.ErrMsg)
	}
	if !again.Skipped || !contains(again.SkipReason, "target exists") {
		t.Errorf("应标记为跳过并给出原因, got skipped=%v reason=%q", again.Skipped, again.SkipReason)
	}
}

// TestEngine_TVAlreadyInLibrarySkipButNewEpisodeStillOrganized 剧集同理：全部集已存在 →
// 跳过不是失败；但只要有一集是新的，就必须照常整理（跳过计数不能挡真活）。
func TestEngine_TVAlreadyInLibrarySkipButNewEpisodeStillOrganized(t *testing.T) {
	tmp := t.TempDir()
	savePath := filepath.Join(tmp, "downloads")
	_ = os.MkdirAll(savePath, 0o755)
	ep := func(n string) string {
		p := filepath.Join(savePath, n)
		_ = os.WriteFile(p, []byte("X"), 0o644)
		return p
	}
	e1, e2 := ep("Show.S01E01.1080p.mkv"), ep("Show.S01E02.1080p.mkv")

	kv := &memKV{m: map[string]string{}}
	cfg := DefaultConfig()
	cfg.TVDir = filepath.Join(tmp, "tv")
	_ = Save(kv, cfg)
	mi := media.MediaInfo{Source: "tmdb", SourceID: "tv:77", Title: "Show", Year: 2020, Type: media.TypeTV}
	eng := NewEngine(fakeRecognizer{mi: mi}, nil, kv, func(_ context.Context, _ string) ([]byte, error) { return []byte("IMG"), nil }, nil)

	first := eng.Process(context.Background(), ProcessInput{
		Hash: "h1", Name: "Show.S01", SavePath: savePath,
		Files: []FileInput{{Path: e1, Size: 1}, {Path: e2, Size: 1}},
	})
	if !first.Success || len(first.Files) != 2 {
		t.Fatalf("首轮应整理 2 集: success=%v files=%d err=%q", first.Success, len(first.Files), first.ErrMsg)
	}

	// 整季重复（换源重下）：应跳过而非失败
	repeat := eng.Process(context.Background(), ProcessInput{
		Hash: "h2", Name: "Show.S01", SavePath: savePath,
		Files: []FileInput{{Path: e1, Size: 1}, {Path: e2, Size: 1}},
	})
	if repeat.ErrMsg != "" || !repeat.Skipped {
		t.Errorf("整季已存在应记跳过, got ErrMsg=%q skipped=%v reason=%q", repeat.ErrMsg, repeat.Skipped, repeat.SkipReason)
	}

	// 混合场景：两集已有 + 一集新 → 必须把新集整理掉（正控，防「有跳过就不干活」）
	e3 := ep("Show.S01E03.1080p.mkv")
	mixed := eng.Process(context.Background(), ProcessInput{
		Hash: "h3", Name: "Show.S01", SavePath: savePath,
		Files: []FileInput{{Path: e1, Size: 1}, {Path: e2, Size: 1}, {Path: e3, Size: 1}},
	})
	if !mixed.Success {
		t.Fatalf("含新集的一轮必须成功: ErrMsg=%q skipped=%v", mixed.ErrMsg, mixed.Skipped)
	}
	if len(mixed.Files) != 1 || !contains(mixed.Files[0], "S01E03") {
		t.Errorf("应只整理新集 S01E03, got %v", mixed.Files)
	}
}
