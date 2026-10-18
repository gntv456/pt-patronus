package chain

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"gorm.io/gorm"
)

// newMutableRssChain 假 feed 可在两次 Check 之间换内容（模拟站点轮换凭据后重发 RSS）。
func newMutableRssChain(t *testing.T, body string) (*RssChain, *oper.RssItemOper, string, func(string)) {
	t.Helper()
	cur := atomic.Value{}
	cur.Store(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, cur.Load().(string))
	}))
	t.Cleanup(srv.Close)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/rss_dedupe.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RssSubscription{}, &models.RssItem{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	op := oper.NewRssOper(db)
	itemOp := oper.NewRssItemOper(db)
	ch := NewRssChain(NewBase(module.New(), event.New()), op, itemOp, nil, nil, nil, nil, hc, nil)
	return ch, itemOp, srv.URL, func(s string) { cur.Store(s) }
}

func rssFeed(items ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>F</title>` +
		strings.Join(items, "") + `</channel></rss>`
}

// TestRssCheckDedupeStableAcrossCredentialRotation guid 是带 passkey 的下载链时，
// 凭据轮换不能让整条 feed 的历史条目重新「变新」。
// 回归：rssItemKey 直接哈希原 guid——用户在站点后台重置 passkey 后，下一轮 RSS 里
// 每条 guid 都变了 → 全部记为新条目；开了自动下载就把已下过的种子整条重推一遍。
func TestRssCheckDedupeStableAcrossCredentialRotation(t *testing.T) {
	item := func(id, pk string) string {
		return fmt.Sprintf(
			`<item><title>Show.S01E%s</title><link>https://site/details.php?id=%s</link>`+
				`<guid>https://site/download.php?id=%s&amp;passkey=%s</guid></item>`,
			id, id, id, pk)
	}
	v1 := rssFeed(item("1", "OLDKEY"), item("2", "OLDKEY"))
	ch, itemOp, feedURL, setFeed := newMutableRssChain(t, v1)
	sub := &models.RssSubscription{Name: "cred-rotation", URL: feedURL, Enabled: true}
	if _, err := ch.Create(sub); err != nil {
		t.Fatal(err)
	}

	first, err := ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if first.New != 2 {
		t.Fatalf("首轮应发现 2 条，got %d", first.New)
	}

	// 同两条目、只换 passkey：必须仍判为已见过。
	setFeed(rssFeed(item("1", "NEWKEY"), item("2", "NEWKEY")))
	second, err := ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("Check2: %v", err)
	}
	if second.New != 0 {
		t.Fatalf("凭据轮换后不应重新算新条目，got %d", second.New)
	}
	items, _ := itemOp.ListBySubscription(sub.ID, 10)
	if len(items) != 2 {
		t.Fatalf("库里应只有 2 条，got %d %+v", len(items), items)
	}
}

// TestRssCheckKeepsDistinctIdentityParams 「不删空」护栏的另一面：
// 有些站的下载链把身份就放在 hash 这一个参数里（download.php?hash=<infohash>）。
// 若把它当鉴权参数删掉，同 feed 的每条都会塌成同一个光杆地址，第二条起被误判为
// 重复而永远不再入库/下载——静默丢种比多推一次严重得多。
func TestRssCheckKeepsDistinctIdentityParams(t *testing.T) {
	item := func(h string) string {
		return fmt.Sprintf(
			`<item><title>Movie.%s</title><link>https://site/details.php?h=%s</link>`+
				`<guid>https://site/download.php?hash=%s</guid></item>`, h, h, h)
	}
	ch, itemOp, feedURL, _ := newMutableRssChain(t, rssFeed(item("H1"), item("H2")))
	sub := &models.RssSubscription{Name: "hash-only", URL: feedURL, Enabled: true}
	if _, err := ch.Create(sub); err != nil {
		t.Fatal(err)
	}
	res, err := ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.New != 2 {
		t.Fatalf("两条 hash 各异的种子必须都算新，got %d", res.New)
	}
	items, _ := itemOp.ListBySubscription(sub.ID, 10)
	if len(items) != 2 {
		t.Fatalf("库里应 2 条，got %d", len(items))
	}
}

// TestNormalizeDownloadURLGuardCases 纯函数层的边界清单（配合上面两条链级测试逐出口验）。
func TestNormalizeDownloadURLGuardCases(t *testing.T) {
	same := func(a, b string) bool { return normalizeDownloadURL(a) == normalizeDownloadURL(b) }
	diff := func(a, b string) bool { return normalizeDownloadURL(a) != normalizeDownloadURL(b) }

	// 凭据轮换 → 归一后相同（有 id 兜着身份）
	if !same("https://s/download.php?id=7&passkey=A", "https://s/download.php?id=7&passkey=B") {
		t.Error("passkey 轮换应得同一键")
	}
	// Gazelle 每次签发的 auth 也是凭据
	if !same("https://s/t.php?action=download&torrentid=9&authkey=K1&auth=A1",
		"https://s/t.php?action=download&torrentid=9&authkey=K2&auth=A2") {
		t.Error("auth/authkey 轮换应得同一键")
	}
	// 身份参数不同 → 必须不同键
	if !diff("https://s/download.php?id=7&passkey=A", "https://s/download.php?id=8&passkey=A") {
		t.Error("不同 id 绝不能同键")
	}
	// query 会被删空时保留原样 → 只靠 hash 认身份的站不会被合并
	if !diff("https://s/download.php?hash=H1", "https://s/download.php?hash=H2") {
		t.Error("hash 作唯一身份时不得删空合并")
	}
	// magnet 的 infohash 是身份，authkey 是凭据
	if !same("magnet:?xt=urn:btih:AAAA&authkey=OLD", "magnet:?xt=urn:btih:AAAA&authkey=NEW") {
		t.Error("magnet 轮换 authkey 应得同一键")
	}
	if !diff("magnet:?xt=urn:btih:AAAA", "magnet:?xt=urn:btih:BBBB") {
		t.Error("不同 infohash 绝不能同键")
	}
	// 非 URL 的纯文本 guid（有些站给数字/UUID）原样可比
	if !diff("g1", "g2") || !same("g1", "g1") {
		t.Error("非 URL guid 应按原样区分")
	}
}
