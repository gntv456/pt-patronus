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
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/media"
	"github.com/hxpt/ptpatronus/internal/transfer"
	"gorm.io/gorm"
)

type transferDeepRecognizer struct {
	info media.MediaInfo
}

func (r transferDeepRecognizer) Search(context.Context, string, int, string) ([]media.MediaInfo, error) {
	return []media.MediaInfo{r.info}, nil
}

func (r transferDeepRecognizer) Detail(context.Context, string, string) (*media.MediaInfo, error) {
	info := r.info
	return &info, nil
}

type transferDeepDownloader struct {
	downloader.Downloader
	files     []downloader.TorrentFile
	filesErr  error
	listCalls int
}

func (d *transferDeepDownloader) ListFiles(context.Context, string) ([]downloader.TorrentFile, error) {
	d.listCalls++
	return d.files, d.filesErr
}

func newTransferDeepChain(t *testing.T, info media.MediaInfo) (*TransferChain, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "transfer.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.SystemConfig{}, &models.TransferHistory{}, &models.DownloadMediaLink{}); err != nil {
		t.Fatal(err)
	}
	cfgOp := oper.NewSystemConfigOper(db)
	cfg := transfer.DefaultConfig()
	cfg.MovieDir = filepath.Join(t.TempDir(), "movies")
	cfg.TVDir = filepath.Join(t.TempDir(), "tv")
	cfg.RefreshLibrary = false
	if err := transfer.Save(cfgOp, cfg); err != nil {
		t.Fatal(err)
	}
	links := oper.NewDownloadMediaLinkOper(db)
	engine := transfer.NewEngine(transferDeepRecognizer{info: info}, linkStoreAdapter{op: links}, cfgOp, nil, nil)
	chain := &TransferChain{
		engine: engine,
		hist:   oper.NewTransferHistoryOper(db),
		links:  links,
		cfg:    cfgOp,
	}
	return chain, db
}

func TestTransferProcessTorrentFiltersAndDeduplicates(t *testing.T) {
	chain, db := newTransferDeepChain(t, media.MediaInfo{
		Source: "tmdb", SourceID: "movie:1", Title: "Deep Test", Year: 2026, Type: media.TypeMovie,
	})
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "Deep.Test.2026.1080p.mkv")
	if err := os.WriteFile(src, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	dl := &transferDeepDownloader{files: []downloader.TorrentFile{{Name: filepath.Base(src), Size: 5}}}
	torrent := &downloader.Torrent{Hash: "hash-1", Name: filepath.Base(src), SavePath: srcDir, Category: "other", State: "completed", CompletedAt: time.Now()}

	if got := chain.processTorrent(context.Background(), 1, torrent, dl, false); got != nil {
		t.Fatalf("automatic organize should honor category filter, got %+v", got)
	}
	if dl.listCalls != 0 {
		t.Fatalf("filtered torrent should not list files, calls=%d", dl.listCalls)
	}

	torrent.Category = "subscribe"
	res := chain.processTorrent(context.Background(), 1, torrent, dl, false)
	if res == nil || !res.Success {
		t.Fatalf("expected successful organize, got %+v", res)
	}
	if got := chain.processTorrent(context.Background(), 1, torrent, dl, false); got != nil {
		t.Fatalf("successful hash should be deduplicated, got %+v", got)
	}
	if dl.listCalls != 1 {
		t.Fatalf("deduplicated torrent should not list files again, calls=%d", dl.listCalls)
	}
	var successes int64
	if err := db.Model(&models.TransferHistory{}).Where("download_hash = ? AND status = ?", torrent.Hash, "success").Count(&successes).Error; err != nil {
		t.Fatal(err)
	}
	if successes != 1 {
		t.Fatalf("success history count=%d, want 1", successes)
	}
}

func TestTransferProcessTorrentRecordsListFilesFailure(t *testing.T) {
	chain, db := newTransferDeepChain(t, media.MediaInfo{Title: "Unused", Type: media.TypeMovie})
	dl := &transferDeepDownloader{filesErr: errors.New("downloader offline")}
	torrent := &downloader.Torrent{Hash: "hash-error", Name: "Broken.mkv", Category: "subscribe", State: "completed", CompletedAt: time.Now()}

	res := chain.processTorrent(context.Background(), 9, torrent, dl, false)
	if res == nil || res.Success || !strings.Contains(res.ErrMsg, "downloader offline") {
		t.Fatalf("unexpected result: %+v", res)
	}
	var history models.TransferHistory
	if err := db.Where("download_hash = ?", torrent.Hash).First(&history).Error; err != nil {
		t.Fatal(err)
	}
	if history.Status != "failed" || !strings.Contains(history.ErrorMsg, "list torrent files") {
		t.Fatalf("unexpected history: %+v", history)
	}
}

func TestTransferProcessTorrentSkipsIncomplete(t *testing.T) {
	chain, db := newTransferDeepChain(t, media.MediaInfo{Title: "X", Type: media.TypeMovie})
	dl := &transferDeepDownloader{files: []downloader.TorrentFile{{Name: "v.mkv", Size: 5}}}
	// 未下完（stalledDL + 无 CompletedAt）：即便 manual 也跳过，且不调 ListFiles。
	torrent := &downloader.Torrent{Hash: "hash-inc", Name: "Incomplete.mkv", Category: "subscribe", State: "stalledDL", Progress: 0.4}

	res := chain.processTorrent(context.Background(), 1, torrent, dl, true)
	if res == nil || !res.Skipped {
		t.Fatalf("expected skipped result for incomplete torrent, got %+v", res)
	}
	if dl.listCalls != 0 {
		t.Fatalf("incomplete torrent should not list files, calls=%d", dl.listCalls)
	}
	var hist models.TransferHistory
	if err := db.Where("download_hash = ?", torrent.Hash).First(&hist).Error; err != nil {
		t.Fatalf("expected skipped history row, got err: %v", err)
	}
	if hist.Status != "skipped" || !strings.Contains(hist.ErrorMsg, "未完成") {
		t.Fatalf("unexpected skipped history: %+v", hist)
	}
}

func TestToFileInputsNormalizesRelativePaths(t *testing.T) {
	base := filepath.Join(t.TempDir(), "downloads")
	abs := filepath.Join(t.TempDir(), "absolute.mkv")
	got := toFileInputs(base, []downloader.TorrentFile{
		{Name: filepath.Join("show", "episode.mkv"), Size: 11},
		{Name: abs, Size: 22},
	})
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].Path != filepath.Join(base, "show", "episode.mkv") || got[0].Size != 11 {
		t.Fatalf("relative path not normalized: %+v", got[0])
	}
	if got[1].Path != abs || got[1].Size != 22 {
		t.Fatalf("absolute path changed: %+v", got[1])
	}
}
