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

func (m *memKV) Get(k string) (string, error)  { return m.m[k], nil }
func (m *memKV) Set(k, v string) error { m.m[k] = v; return nil }

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
