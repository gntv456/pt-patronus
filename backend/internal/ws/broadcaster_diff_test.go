package ws

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

func snapOf(dlID uint, hashes ...string) []TorrentSnapshot {
	ts := make([]downloader.Torrent, 0, len(hashes))
	for _, h := range hashes {
		ts = append(ts, downloader.Torrent{Hash: h, Name: h, State: "completed"})
	}
	return []TorrentSnapshot{{DownloaderID: dlID, Name: "dl", Torrents: ts}}
}

// 首轮（无 prev）即便 forceFull=false 也返回 nil diff（走全量路径）+ 填充 next 缓存。
func TestBuildDiff_FirstRoundIsFull(t *testing.T) {
	b := &Broadcaster{}
	diff, next := b.buildDiff(snapOf(1, "h1"), nil, false)
	if diff != nil {
		t.Fatalf("first round should be full (nil diff), got %+v", diff)
	}
	if len(next) != 1 || next[1] == nil || len(next[1].torrents) != 1 {
		t.Fatalf("next cache should contain dl 1 with h1, got %+v", next)
	}
}

// forceFull=true 时返回 nil diff。
func TestBuildDiff_ForceFull(t *testing.T) {
	b := &Broadcaster{}
	_, prev := b.buildDiff(snapOf(1, "h1"), nil, false)
	diff, _ := b.buildDiff(snapOf(1, "h1"), prev, true)
	if diff != nil {
		t.Fatalf("forceFull should yield nil diff, got %+v", diff)
	}
}

// 稳态（快照全同）→ diff 非空对象但 Added/Removed/Changed 全空。
func TestBuildDiff_SteadyStateNoChange(t *testing.T) {
	b := &Broadcaster{}
	_, prev := b.buildDiff(snapOf(1, "h1", "h2"), nil, false)
	diff, _ := b.buildDiff(snapOf(1, "h1", "h2"), prev, false)
	if diff == nil {
		t.Fatal("expected non-nil diff object")
	}
	if len(diff.Added) != 0 || len(diff.Removed) != 0 || len(diff.Changed) != 0 {
		t.Fatalf("steady state should be empty diff, got %+v", diff)
	}
}

// 单下载器内：种子新增 / 删除 / 字段变化 同轮并存。
func TestBuildDiff_AddRemoveChangeTorrent(t *testing.T) {
	b := &Broadcaster{}
	_, prev := b.buildDiff(snapOf(1, "h1", "h2"), nil, false)
	newSnap := []TorrentSnapshot{{DownloaderID: 1, Name: "dl", Torrents: []downloader.Torrent{
		{Hash: "h1", Name: "h1-renamed", State: "completed"}, // changed
		{Hash: "h3", Name: "h3", State: "completed"},         // added
	}}}
	diff, _ := b.buildDiff(newSnap, prev, false)
	if diff == nil || len(diff.Changed) != 1 {
		t.Fatalf("expected one downloader changed, got %+v", diff)
	}
	dd := diff.Changed[0]
	if len(dd.Added) != 1 || dd.Added[0].Hash != "h3" {
		t.Fatalf("added: %+v", dd.Added)
	}
	if len(dd.Removed) != 1 || dd.Removed[0] != "h2" {
		t.Fatalf("removed: %+v", dd.Removed)
	}
	if len(dd.Changed) != 1 || dd.Changed[0].Hash != "h1" || dd.Changed[0].Name != "h1-renamed" {
		t.Fatalf("changed: %+v", dd.Changed)
	}
}

// 下载器级新增 / 删除。
func TestBuildDiff_AddRemoveDownloader(t *testing.T) {
	b := &Broadcaster{}
	_, prev := b.buildDiff(append(snapOf(1, "h1"), snapOf(2, "h2")...), nil, false)
	// 只剩下载器 1，新增下载器 3
	newSnap := append(snapOf(1, "h1"), snapOf(3, "h3")...)
	diff, _ := b.buildDiff(newSnap, prev, false)
	if diff == nil {
		t.Fatal("expected diff")
	}
	if len(diff.Added) != 1 || diff.Added[0].DownloaderID != 3 {
		t.Fatalf("added downloader: %+v", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0] != 2 {
		t.Fatalf("removed downloader: %+v", diff.Removed)
	}
}

// stats 变化 → 该下载器进 Changed 且带新 stats。
func TestBuildDiff_StatsChange(t *testing.T) {
	b := &Broadcaster{}
	_, prev := b.buildDiff([]TorrentSnapshot{{DownloaderID: 1, Name: "dl", Stats: &downloader.GlobalStats{DLRate: 100}}}, nil, false)
	diff, _ := b.buildDiff([]TorrentSnapshot{{DownloaderID: 1, Name: "dl", Stats: &downloader.GlobalStats{DLRate: 200}}}, prev, false)
	if diff == nil || len(diff.Changed) != 1 {
		t.Fatalf("expected stats change, got %+v", diff)
	}
	if diff.Changed[0].Stats == nil || diff.Changed[0].Stats.DLRate != 200 {
		t.Fatalf("stats: %+v", diff.Changed[0].Stats)
	}
}
