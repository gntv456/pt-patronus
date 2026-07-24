package schemas

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// liveSkipOnAuth 在本地 cookie 过期（ErrNotLoggedIn）时 skip live 验证，而非 fail。
// live 测试依赖 D:/PTPatronus/tmp/sunny_cookie.txt，cookie 过期是正常情况，不应阻断测试。
func liveSkipOnAuth(t *testing.T, stage string, err error) {
	t.Helper()
	if errors.Is(err, site.ErrNotLoggedIn) {
		t.Skip(stage + ": sunny cookie expired (not logged in)")
	}
}

// 需本地 cookie 文件时才跑：D:/PTPatronus/tmp/sunny_cookie.txt
func TestSunnyptLiveInteract(t *testing.T) {
	ckPath := filepath.Clean(`D:/PTPatronus/tmp/sunny_cookie.txt`)
	b, err := os.ReadFile(ckPath)
	if err != nil || len(b) < 20 {
		t.Skip("no local sunny cookie")
	}
	ck := string(b)
	hc, _ := httpclient.New(httpclient.Options{Timeout: 25 * time.Second, AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	def := &site.SiteMetadata{ID: "sunnypt", Schema: "tnode"}
	base := "https://sunnypt.top"
	ctx := context.Background()

	cr, err := g.Checkin(ctx, def, base, ck)
	if err != nil {
		liveSkipOnAuth(t, "checkin", err)
		t.Fatalf("checkin: %v", err)
	}
	if cr == nil || !cr.Success {
		t.Fatalf("checkin not success: %+v", cr)
	}
	t.Logf("checkin: %s bonus=%.0f", cr.Message, cr.Bonus)

	ann, err := g.FetchAnnouncement(ctx, def, base, ck)
	if err != nil {
		liveSkipOnAuth(t, "ann", err)
		t.Fatalf("ann: %v", err)
	}
	if ann.Error != "" {
		t.Fatalf("ann error: %s", ann.Error)
	}
	if len(ann.Items) == 0 {
		t.Fatal("ann empty")
	}
	t.Logf("ann items=%d first=%s %s", len(ann.Items), ann.Items[0].Date, ann.Items[0].Title)

	sys, err := g.FetchMessages(ctx, def, base, ck, site.MailboxSystem)
	if err != nil {
		liveSkipOnAuth(t, "msg system", err)
		t.Fatalf("msg system: %v", err)
	}
	t.Logf("system msgs=%d", len(sys))

	inbox, err := g.FetchMessages(ctx, def, base, ck, site.MailboxInbox)
	if err != nil {
		liveSkipOnAuth(t, "msg inbox", err)
		t.Fatalf("msg inbox: %v", err)
	}
	t.Logf("inbox rooms=%d", len(inbox))
	if len(inbox) == 0 {
		t.Fatal("expected at least one private chat room")
	}
}
