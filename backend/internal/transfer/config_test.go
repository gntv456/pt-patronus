package transfer

import (
	"path/filepath"
	"testing"
)

func TestConfigMapPathUsesDownloaderAndLongestPrefix(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PathMappings = []PathMapping{
		{RemotePath: "/video", LocalPath: filepath.Join(t.TempDir(), "all")},
		{DownloaderID: 7, RemotePath: "/video/movies", LocalPath: filepath.Join(t.TempDir(), "movies")},
	}

	got, ok := cfg.MapPath(7, "/video/movies/Let.Movie.mkv")
	if !ok || got != filepath.Join(cfg.PathMappings[1].LocalPath, "Let.Movie.mkv") {
		t.Fatalf("MapPath = %q, %v", got, ok)
	}
	got, ok = cfg.MapPath(8, `/video/Let.Movie.mkv`)
	if !ok || got != filepath.Join(cfg.PathMappings[0].LocalPath, "Let.Movie.mkv") {
		t.Fatalf("generic MapPath = %q, %v", got, ok)
	}
	if _, ok := cfg.MapPath(7, "/videos/Let.Movie.mkv"); ok {
		t.Fatal("mapping should respect path segment boundaries")
	}
}

func TestConfigDefaultsFailureRetryAndMappings(t *testing.T) {
	cfg := (Config{}).withDefaults()
	if cfg.FailedRetryMinutes != DefaultConfig().FailedRetryMinutes {
		t.Fatalf("FailedRetryMinutes = %d", cfg.FailedRetryMinutes)
	}
	if cfg.PathMappings == nil {
		t.Fatal("PathMappings should be a non-nil JSON array")
	}
}
