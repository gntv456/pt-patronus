package dl

import (
	"context"
	"errors"
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

func TestDLAdvancedOps(t *testing.T) {
	c, f := newDLClient(t)
	ctx := context.Background()

	if err := c.Recheck(ctx, []string{"h1"}); err != nil {
		t.Fatal(err)
	}
	if f.method != "core.force_recheck" {
		t.Errorf("recheck: %s", f.method)
	}
	if err := c.Reannounce(ctx, []string{"h1"}); err != nil {
		t.Fatal(err)
	}
	if f.method != "core.force_reannounce" {
		t.Errorf("reannounce: %s", f.method)
	}
	if err := c.SetLocation(ctx, []string{"h1"}, "/new"); err != nil {
		t.Fatal(err)
	}
	if f.method != "core.move_storage" {
		t.Errorf("SetLocation: %s", f.method)
	}
	for _, a := range []struct{ in, want string }{
		{"top", "core.queue_top"}, {"up", "core.queue_up"}, {"down", "core.queue_down"}, {"bottom", "core.queue_bottom"},
	} {
		if err := c.SetQueuePriority(ctx, []string{"h1"}, a.in); err != nil {
			t.Fatalf("queue %s: %v", a.in, err)
		}
		if f.method != a.want {
			t.Errorf("queue %s: %s want %s", a.in, f.method, a.want)
		}
	}
	if err := c.SetQueuePriority(ctx, nil, "bogus"); err == nil {
		t.Error("invalid queue action should error")
	}

	if err := c.SetShareLimits(ctx, []string{"h1"}, downloader.ShareLimits{RatioLimit: 2.0}); err != nil {
		t.Fatal(err)
	}
	if f.method != "core.set_torrent" {
		t.Errorf("sharelimits: %s", f.method)
	}

	lim, err := c.GetSpeedLimits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lim.UploadLimit != 102400 || lim.DownloadLimit != 204800 { // 100/200 KB → bytes
		t.Errorf("speedlimits: %+v", lim)
	}

	files, err := c.ListFiles(ctx, "h1")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "a.mkv" || files[0].Priority != 1 {
		t.Errorf("files: %+v", files)
	}
	if err := c.SetFilePriority(ctx, "h1", []int{0}, 3); err != nil {
		t.Fatal(err)
	}
	if f.method != "core.set_torrent_file_priorities" {
		t.Errorf("setprio: %s", f.method)
	}
	tr, err := c.ListTrackers(ctx, "h1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tr) != 1 || tr[0].URL != "http://t" {
		t.Errorf("trackers: %+v", tr)
	}
	peers, err := c.ListPeers(ctx, "h1")
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].IP != "1.2.3.4" || peers[0].Client != "ClientX" {
		t.Errorf("peers: %+v", peers)
	}
}

func TestDLNotImplemented(t *testing.T) {
	c, _ := newDLClient(t)
	ctx := context.Background()
	checks := map[string]func() error{
		"SetForceStart":    func() error { return c.SetForceStart(ctx, nil, true) },
		"ListTags":         func() error { _, e := c.ListTags(ctx); return e },
		"SetTags":          func() error { return c.SetTags(ctx, nil, nil) },
		"AddTags":          func() error { return c.AddTags(ctx, nil, nil) },
		"RemoveTags":       func() error { return c.RemoveTags(ctx, nil, nil) },
		"CreateCategory":   func() error { return c.CreateCategory(ctx, "n", "") },
		"EditCategory":     func() error { return c.EditCategory(ctx, "n", "") },
		"DeleteCategories": func() error { return c.DeleteCategories(ctx, nil) },
		"ToggleSequential": func() error { return c.ToggleSequentialDownload(ctx, nil) },
		"ToggleFirstLast":  func() error { return c.ToggleFirstLastPiece(ctx, nil) },
	}
	for name, fn := range checks {
		if err := fn(); !errors.Is(err, downloader.ErrNotImplemented) {
			t.Errorf("%s: want ErrNotImplemented, got %v", name, err)
		}
	}
}
