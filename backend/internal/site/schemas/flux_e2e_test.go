package schemas

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// TestFluxTorrentLiveE2E 真实实例端到端冒烟(本机 Docker flux-api :8080)。
// FLUX_E2E_TOKEN 未设时 skip;FLUX_E2E_BASE 可覆盖地址。
func TestFluxTorrentLiveE2E(t *testing.T) {
	token := e2eEnv(t, "FLUX_E2E_TOKEN")
	base := e2eEnvOr(t, "FLUX_E2E_BASE", "http://127.0.0.1:8080")
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatalf("httpclient: %v", err)
	}
	f := NewFluxTorrent(hc)
	def := &site.SiteMetadata{ID: "fluxtorrent", Schema: "fluxtorrent"}
	cred := `{"token":"` + token + `"}`

	// 1. 搜索
	ts, err := f.Search(context.Background(), def, base, cred, "demo", "", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ts) == 0 {
		t.Fatalf("search returned 0 torrents")
	}
	t.Logf("search ok: %d torrents, first=%q size=%s free=%v dvf=%v", len(ts), ts[0].Title, ts[0].Size, ts[0].Free, ts[0].DownloadVolumeFactor)
	if !strings.Contains(ts[0].DetailURL, "/torrent/") {
		t.Fatalf("detail url = %q", ts[0].DetailURL)
	}

	// 2. 用户信息 + passkey 握手
	u, err := f.FetchUserInfo(context.Background(), def, base, cred)
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	t.Logf("userinfo ok: id=%s name=%s level=%s up=%d down=%d bonus=%v", u.ID, u.Name, u.Level, u.Uploaded, u.Downloaded, u.Bonus)
	if u.Name == "" || u.Level == "" {
		t.Fatalf("userinfo empty identity")
	}
	pk := f.TakePasskey()
	if len(pk) != 32 {
		t.Fatalf("passkey len = %d, want 32", len(pk))
	}

	// 3. 能力端点
	hrs, err := f.FetchHR(context.Background(), def, base, cred)
	if err != nil {
		t.Fatalf("FetchHR: %v", err)
	}
	items, err := f.FetchSeedingList(context.Background(), def, base, cred)
	if err != nil {
		t.Fatalf("FetchSeedingList: %v", err)
	}
	msgs, err := f.FetchMessages(context.Background(), def, base, cred, site.MailboxInbox)
	if err != nil {
		t.Fatalf("FetchMessages: %v", err)
	}
	t.Logf("capabilities ok: hr=%d seeding=%d messages=%d", len(hrs), len(items), len(msgs))

	// 4. 带 passkey 的下载直链构造 + 真实下载 .torrent
	cred2 := `{"token":"` + token + `","passkey":"` + pk + `"}`
	ts2, err := f.Search(context.Background(), def, base, cred2, "demo", "", 1)
	if err != nil || len(ts2) == 0 {
		t.Fatalf("search with passkey: %v %d", err, len(ts2))
	}
	if !strings.Contains(ts2[0].DownloadURL, "passkey=") {
		t.Fatalf("download url without passkey: %q", ts2[0].DownloadURL)
	}
	t.Logf("download url ok: %s", strings.ReplaceAll(ts2[0].DownloadURL, pk, "<passkey>"))
}

func e2eEnv(t *testing.T, key string) string {
	t.Helper()
	v := strings.TrimSpace(osGetenv(key))
	if v == "" {
		t.Skipf("%s not set; skipping live e2e", key)
	}
	return v
}

func e2eEnvOr(t *testing.T, key, def string) string {
	t.Helper()
	if v := strings.TrimSpace(osGetenv(key)); v != "" {
		return v
	}
	return def
}
