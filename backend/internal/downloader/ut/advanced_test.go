package ut

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

func TestUTAdvancedOps(t *testing.T) {
	c, f := newUTClient(t)
	ctx := context.Background()

	if err := c.Recheck(ctx, []string{"h1"}); err != nil {
		t.Fatalf("recheck: %v", err)
	}
	if f.action != "recheck" {
		t.Errorf("recheck action: %s", f.action)
	}

	if err := c.SetForceStart(ctx, []string{"h1"}, true); err != nil {
		t.Fatalf("forcestart: %v", err)
	}
	if f.action != "forcestart" || !strings.Contains(f.raw, "v=1") {
		t.Errorf("forcestart: action=%s raw=%s", f.action, f.raw)
	}

	for _, a := range []struct{ in, want string }{
		{"top", "queuetop"}, {"up", "queueup"}, {"down", "queuedown"}, {"bottom", "queuebottom"},
	} {
		if err := c.SetQueuePriority(ctx, []string{"h1"}, a.in); err != nil {
			t.Fatalf("queue %s: %v", a.in, err)
		}
		if f.action != a.want {
			t.Errorf("queue %s: action=%s want %s", a.in, f.action, a.want)
		}
	}
	if err := c.SetQueuePriority(ctx, []string{"h1"}, "bogus"); err == nil {
		t.Errorf("invalid queue action should error")
	}

	if err := c.SetShareLimits(ctx, []string{"h1"}, downloader.ShareLimits{RatioLimit: 2.0, SeedingTimeLimit: 60}); err != nil {
		t.Fatalf("sharelimits: %v", err)
	}
	if f.action != "setprops" || !strings.Contains(f.raw, "seed_ratio") || !strings.Contains(f.raw, "seed_time") {
		t.Errorf("sharelimits raw: action=%s raw=%s", f.action, f.raw)
	}

	if err := c.SetLimits(ctx, []string{"h1"}, downloader.Limits{UploadLimit: 100, DownloadLimit: 200}); err != nil {
		t.Fatalf("limits: %v", err)
	}
	if !strings.Contains(f.raw, "rate_upload_max") || !strings.Contains(f.raw, "rate_download_max") {
		t.Errorf("limits raw: %s", f.raw)
	}

	if err := c.SetFilePriority(ctx, "h1", []int{0, 2}, 3); err != nil {
		t.Fatalf("setprio: %v", err)
	}
	if f.action != "setprio" || !strings.Contains(f.raw, "p=3") {
		t.Errorf("setprio raw: action=%s raw=%s", f.action, f.raw)
	}
}

func TestUTNotImplemented(t *testing.T) {
	c, _ := newUTClient(t)
	ctx := context.Background()
	checks := map[string]func() error{
		"Reannounce":       func() error { return c.Reannounce(ctx, nil) },
		"SetLocation":      func() error { return c.SetLocation(ctx, nil, "/x") },
		"ListTags":         func() error { _, e := c.ListTags(ctx); return e },
		"SetTags":          func() error { return c.SetTags(ctx, nil, nil) },
		"AddTags":          func() error { return c.AddTags(ctx, nil, nil) },
		"RemoveTags":       func() error { return c.RemoveTags(ctx, nil, nil) },
		"CreateCategory":   func() error { return c.CreateCategory(ctx, "n", "") },
		"EditCategory":     func() error { return c.EditCategory(ctx, "n", "") },
		"DeleteCategories": func() error { return c.DeleteCategories(ctx, nil) },
		"ToggleSequential": func() error { return c.ToggleSequentialDownload(ctx, nil) },
		"ToggleFirstLast":  func() error { return c.ToggleFirstLastPiece(ctx, nil) },
		"ListTrackers":     func() error { _, e := c.ListTrackers(ctx, "h"); return e },
		"AddTrackers":      func() error { return c.AddTrackers(ctx, "h", nil) },
		"RemoveTrackers":   func() error { return c.RemoveTrackers(ctx, "h", nil) },
	}
	for name, fn := range checks {
		if err := fn(); !errors.Is(err, downloader.ErrNotImplemented) {
			t.Errorf("%s: want ErrNotImplemented, got %v", name, err)
		}
	}
}
