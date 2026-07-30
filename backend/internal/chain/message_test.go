package chain

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// fakeMsgAdapter 在 fakeAdapter 基础上加 MessageCapable：按 mailbox 返回不同数据，
// atomic 计数 + 可选 proceed 阻塞（验 singleflight）+ 可选 err（验未登录）。
type fakeMsgAdapter struct {
	*fakeAdapter
	byMailbox map[site.MailboxType][]site.Message
	calls     int32
	proceed   chan struct{} // 非 nil 时 FetchMessages 阻塞至 close
	err       error
}

func (f *fakeMsgAdapter) FetchMessages(_ context.Context, _ *site.SiteMetadata, _, _ string, mailbox site.MailboxType) ([]site.Message, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.proceed != nil {
		<-f.proceed
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.byMailbox[mailbox], nil
}

// MessageCapable 还要求另外 4 个方法；测试只关心 FetchMessages，其余给 no-op stub 满足接口。
func (f *fakeMsgAdapter) FetchMessageContent(context.Context, *site.SiteMetadata, string, string, string) (string, error) {
	return "", nil
}
func (f *fakeMsgAdapter) ReplyMessage(context.Context, *site.SiteMetadata, string, string, string, string) (*site.MessageReplyResult, error) {
	return nil, nil
}
func (f *fakeMsgAdapter) DeleteMessage(context.Context, *site.SiteMetadata, string, string, string) error {
	return nil
}
func (f *fakeMsgAdapter) MarkRead(context.Context, *site.SiteMetadata, string, string, []string) (*site.BatchResult, error) {
	return nil, nil
}

// newMsgChain 建一个单站 nexusphp chain + 注册给定适配器。
func newMsgChain(t *testing.T, src module.Module) (*SiteChain, *models.Site) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://x", IsActive: true})
	mm := module.New()
	mm.Register(src)
	return NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry()), s
}

// TestMessagesAll_MailboxIsolation 最关键回归点：切信箱不能串数据。
// 缓存 key 必须含 mailbox，否则 inbox 缓存被 sent 覆盖/误读。
func TestMessagesAll_MailboxIsolation(t *testing.T) {
	src := &fakeMsgAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		byMailbox: map[site.MailboxType][]site.Message{
			site.MailboxInbox: {{Title: "INBOX"}},
			site.MailboxSent:  {{Title: "SENT"}},
		},
	}
	ch, _ := newMsgChain(t, src)

	// force 预热两个信箱（写各自缓存）
	ch.MessagesAll(context.Background(), site.MailboxInbox, true)
	ch.MessagesAll(context.Background(), site.MailboxSent, true)
	if calls := atomic.LoadInt32(&src.calls); calls != 2 {
		t.Fatalf("after warming 2 mailboxes: calls=%d want 2", calls)
	}

	// force=false 读：应命中各自缓存，互不串
	inbox := ch.MessagesAll(context.Background(), site.MailboxInbox, false)
	sent := ch.MessagesAll(context.Background(), site.MailboxSent, false)
	if calls := atomic.LoadInt32(&src.calls); calls != 2 {
		t.Errorf("cached reads should not re-fetch: calls=%d want 2", calls)
	}
	if len(inbox) != 1 || len(inbox[0].Messages) != 1 || inbox[0].Messages[0].Title != "INBOX" {
		t.Errorf("inbox = %+v, want INBOX", inbox)
	}
	if len(sent) != 1 || len(sent[0].Messages) != 1 || sent[0].Messages[0].Title != "SENT" {
		t.Errorf("sent = %+v, want SENT (mailbox isolation broken)", sent)
	}
}

// TestMessagesAll_ForceBypassesCache force=true 绕过缓存强制回源。
func TestMessagesAll_ForceBypassesCache(t *testing.T) {
	src := &fakeMsgAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		byMailbox:   map[site.MailboxType][]site.Message{site.MailboxInbox: {{Title: "M"}}},
	}
	ch, _ := newMsgChain(t, src)

	ch.MessagesAll(context.Background(), site.MailboxInbox, true)  // 写缓存 calls=1
	ch.MessagesAll(context.Background(), site.MailboxInbox, false) // 命中 calls=1
	if calls := atomic.LoadInt32(&src.calls); calls != 1 {
		t.Fatalf("after cache hit: calls=%d want 1", calls)
	}
	ch.MessagesAll(context.Background(), site.MailboxInbox, true) // force 绕过 calls=2
	if calls := atomic.LoadInt32(&src.calls); calls != 2 {
		t.Errorf("force should bypass cache: calls=%d want 2", calls)
	}
}

// TestMessagesAll_LoggedOutSurfaced 未登录须作为 Error 暴露（否则误显「无消息」），且不钉缓存。
func TestMessagesAll_LoggedOutSurfaced(t *testing.T) {
	src := &fakeMsgAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		err:         site.ErrNotLoggedIn,
	}
	ch, _ := newMsgChain(t, src)

	res := ch.MessagesAll(context.Background(), site.MailboxInbox, true)
	if len(res) != 1 || res[0].Error == "" {
		t.Fatalf("ErrNotLoggedIn must surface as Error, got %+v", res)
	}
	// 未登录不钉缓存：再抓应再次回源（calls 递增）
	calls1 := atomic.LoadInt32(&src.calls)
	ch.MessagesAll(context.Background(), site.MailboxInbox, true)
	if calls := atomic.LoadInt32(&src.calls); calls != calls1+1 {
		t.Errorf("logged-out must not be cached: calls=%d want %d", calls, calls1+1)
	}
}

// TestMessagesAll_CronWarmsInbox cron(force=true)预热后，普通读(force=false)命中缓存零回源。
func TestMessagesAll_CronWarmsInbox(t *testing.T) {
	src := &fakeMsgAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		byMailbox:   map[site.MailboxType][]site.Message{site.MailboxInbox: {{Title: "M"}}},
	}
	ch, _ := newMsgChain(t, src)

	ch.MessagesAll(context.Background(), site.MailboxInbox, true)         // cron 预热 calls=1
	res := ch.MessagesAll(context.Background(), site.MailboxInbox, false) // 命中
	if calls := atomic.LoadInt32(&src.calls); calls != 1 {
		t.Errorf("cron-warmed cache should hit: calls=%d want 1", calls)
	}
	if len(res) != 1 || len(res[0].Messages) != 1 || res[0].Messages[0].Title != "M" {
		t.Errorf("result = %+v want M", res)
	}
}

// TestMessagesAll_SWR 缓存过期时：立即返回旧值（秒回）+ 后台异步刷新（calls 最终递增）。
func TestMessagesAll_SWR(t *testing.T) {
	src := &fakeMsgAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		byMailbox:   map[site.MailboxType][]site.Message{site.MailboxInbox: {{Title: "OLD"}}},
	}
	ch, s := newMsgChain(t, src)

	ch.MessagesAll(context.Background(), site.MailboxInbox, true) // 写缓存 calls=1
	if calls := atomic.LoadInt32(&src.calls); calls != 1 {
		t.Fatalf("warm: calls=%d want 1", calls)
	}

	// 手动把缓存标记为过期 → 触发 SWR
	key := msgCacheKey(s.ID, site.MailboxInbox)
	ch.msgMu.Lock()
	e := ch.msgCache[key]
	e.at = time.Now().Add(-1 * time.Hour)
	ch.msgCache[key] = e
	ch.msgMu.Unlock()

	// force=false：应立即返回旧值（OLD），不阻塞
	res := ch.MessagesAll(context.Background(), site.MailboxInbox, false)
	if len(res) != 1 || len(res[0].Messages) != 1 || res[0].Messages[0].Title != "OLD" {
		t.Errorf("SWR should return stale immediately: %+v want OLD", res)
	}

	// 后台刷新最终发生（calls 1→2）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&src.calls) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if calls := atomic.LoadInt32(&src.calls); calls < 2 {
		t.Errorf("SWR background refresh should re-fetch: calls=%d want >=2", calls)
	}
}

// TestMessagesAllConcurrentDedup singleflight：N 个并发（同 site+mailbox）FetchMessages 只调一次。
func TestMessagesAllConcurrentDedup(t *testing.T) {
	src := &fakeMsgAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		byMailbox:   map[site.MailboxType][]site.Message{site.MailboxInbox: {{Title: "M"}}},
		proceed:     make(chan struct{}),
	}
	ch, _ := newMsgChain(t, src)

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([][]MessageResult, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = ch.MessagesAll(context.Background(), site.MailboxInbox, false)
		}(i)
	}
	close(start)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&src.calls) >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Fatalf("expected 1 in-flight FetchMessages (singleflight coalesce), got %d", got)
	}
	close(src.proceed)
	wg.Wait()

	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("after completion: calls=%d want 1", got)
	}
	for i, r := range results {
		if len(r) != 1 || len(r[0].Messages) != 1 || r[0].Messages[0].Title != "M" {
			t.Errorf("result[%d] = %+v want one site with M", i, r)
		}
	}
}
