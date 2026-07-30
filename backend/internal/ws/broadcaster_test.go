package ws

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

func TestEncodeHRAlert(t *testing.T) {
	payload, err := EncodeHRAlert([]HRAlert{
		{Site: "好学", Title: "Some Torrent", Status: "未达标", Deadline: "10:00:00"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var m Message
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Type != "hr-alert" {
		t.Fatalf("type=%s, want hr-alert", m.Type)
	}
	// Data 是 any（json 解出为 []interface{}），重新序列化后解码到 []HRAlert。
	raw, _ := json.Marshal(m.Data)
	var alerts []HRAlert
	if err := json.Unmarshal(raw, &alerts); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Site != "好学" || alerts[0].Status != "未达标" {
		t.Fatalf("unexpected: %+v", alerts)
	}
}

// Message.Data 改 any 后，torrents 消息仍正常编码。
func TestTorrentsMessageEncodesAfterAnyData(t *testing.T) {
	payload, err := json.Marshal(Message{Type: "torrents", Data: []TorrentSnapshot{}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m Message
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Type != "torrents" {
		t.Fatalf("type=%s, want torrents", m.Type)
	}
}

func TestEncodeCookieAlert(t *testing.T) {
	payload, err := EncodeCookieAlert([]string{"好学", "馒头"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var m Message
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Type != "cookie-alert" {
		t.Fatalf("type=%s, want cookie-alert", m.Type)
	}
	raw, _ := json.Marshal(m.Data)
	var sites []string
	if err := json.Unmarshal(raw, &sites); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(sites) != 2 || sites[0] != "好学" {
		t.Fatalf("unexpected: %+v", sites)
	}
}

// detectDone 应 fan-out 调用全部 DoneHook（媒体库刷新 + 媒体整理并存）。
// 钩子经 hookQ 异步投递，drainHooks 同步消费以验证「投递 + 执行」全链路。
func TestDetectDone_FanOutHooks(t *testing.T) {
	b := &Broadcaster{hookQ: make(chan hookJob, hookQueueCap)}
	calls := &[]string{}
	b.AddDownloadDoneHook(func(context.Context, uint, string, string) { *calls = append(*calls, "a") })
	b.AddDownloadDoneHook(func(context.Context, uint, string, string) { *calls = append(*calls, "b") })

	completed := []TorrentSnapshot{{DownloaderID: 1, Torrents: []downloader.Torrent{{Hash: "h1", Name: "t1", State: "completed"}}}}
	b.detectDone(context.Background(), completed) // 第一轮仅 prime（primed=false 不触发）
	drainHooks(b)
	*calls = nil
	b.detectDone(context.Background(), completed) // 同 hash 已知 → 不重触
	drainHooks(b)
	if len(*calls) != 0 {
		t.Fatalf("known hash should not re-fire, got %v", *calls)
	}
	// 新完成 hash → 两个钩子都应投递并执行
	b.detectDone(context.Background(), []TorrentSnapshot{{DownloaderID: 1, Torrents: []downloader.Torrent{{Hash: "h2", Name: "t2", State: "completed"}}}})
	drainHooks(b)
	if len(*calls) != 2 {
		t.Fatalf("expected 2 hook calls (fan-out), got %d %v", len(*calls), *calls)
	}
}

// 单个钩子 panic 不应阻断其余钩子（worker/drain 内 per-job recover 隔离）。
func TestDetectDone_PanicHookIsolated(t *testing.T) {
	b := &Broadcaster{hookQ: make(chan hookJob, hookQueueCap)}
	calls := &[]string{}
	b.AddDownloadDoneHook(func(context.Context, uint, string, string) { panic("boom") })
	b.AddDownloadDoneHook(func(context.Context, uint, string, string) { *calls = append(*calls, "ok") })

	b.detectDone(context.Background(), []TorrentSnapshot{{DownloaderID: 1, Torrents: []downloader.Torrent{{Hash: "p1", Name: "t", State: "completed"}}}})
	drainHooks(b)
	*calls = nil
	b.detectDone(context.Background(), []TorrentSnapshot{{DownloaderID: 1, Torrents: []downloader.Torrent{{Hash: "p2", Name: "t", State: "completed"}}}})
	drainHooks(b)
	if len(*calls) != 1 {
		t.Fatalf("second hook must run despite first panicking, got %d", len(*calls))
	}
}

// drainHooks 同步消费 hookQ 中已投递的钩子任务（测试用：模拟 worker 执行，每任务独立 recover）。
func drainHooks(b *Broadcaster) {
	for {
		select {
		case job := <-b.hookQ:
			func() {
				defer func() { _ = recover() }()
				job.hk(context.Background(), job.dlID, job.name, job.hash)
			}()
		default:
			return
		}
	}
}
