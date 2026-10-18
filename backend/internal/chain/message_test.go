package chain

import (
	"context"
	"strings"
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
// atomic 计数 + 可选 proceed 阻塞（验 singleflight）+ 可选 err（验未登录）+
// 可选 panicHost（baseURL 命中时现场 panic，模拟适配器解析消息页崩溃，验 per-site 隔离）。
type fakeMsgAdapter struct {
	*fakeAdapter
	byMailbox map[site.MailboxType][]site.Message
	calls     int32
	proceed   chan struct{} // 非 nil 时 FetchMessages 阻塞至 close
	err       error
	panicHost string
}

func (f *fakeMsgAdapter) FetchMessages(ctx context.Context, _ *site.SiteMetadata, baseURL, _ string, mailbox site.MailboxType) ([]site.Message, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.panicHost != "" && strings.Contains(baseURL, f.panicHost) {
		panic("nil pointer dereference on malformed message page")
	}
	if f.proceed != nil {
		// 阻塞期间调用方 ctx 结束要像真实 HTTP 客户端一样带错误返回（区分中断 vs 超时）。
		select {
		case <-f.proceed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
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

// TestMessagesCallerCancelNotCached 调用方中断（切信箱/关页面/断连 → context.Canceled）
// 不得写进消息负缓存：否则一次中断就把该站伪装成「故障」钉满 TTL，期间所有人（含 cron/MCP
// 的普通读）都看到同一个假错误，且 SWR 要等过期才重试。
func TestMessagesCallerCancelNotCached(t *testing.T) {
	src := &fakeMsgAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		byMailbox:   map[site.MailboxType][]site.Message{site.MailboxInbox: {{Title: "M"}}},
		proceed:     make(chan struct{}),
	}
	ch, _ := newMsgChain(t, src)

	caller, cancel := context.WithCancel(context.Background())
	done := make(chan []MessageResult, 1)
	go func() { done <- ch.MessagesAll(caller, site.MailboxInbox, true) }()

	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&src.calls) < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if atomic.LoadInt32(&src.calls) < 1 {
		close(src.proceed)
		<-done
		t.Fatal("前提：回源尚未开始")
	}
	cancel()
	res := <-done
	if len(res) != 1 || !strings.Contains(res[0].Error, "context canceled") {
		t.Fatalf("前提：中断应作为本次结果的错误暴露，got %+v", res)
	}
	close(src.proceed)

	// 中断没进缓存 → 下一次正常读必须重新回源并拿到真实消息
	res2 := ch.MessagesAll(context.Background(), site.MailboxInbox, false)
	if got := atomic.LoadInt32(&src.calls); got != 2 {
		t.Errorf("调用方中断被钉进负缓存：calls=%d want 2（应重新回源）", got)
	}
	if len(res2) != 1 || res2[0].Error != "" || len(res2[0].Messages) != 1 || res2[0].Messages[0].Title != "M" {
		t.Errorf("正常读应拿到真实消息，got %+v", res2)
	}
}

// TestMessagesTimeoutStaysCached 正控：真实抓取超时（DeadlineExceeded）仍按原设计负缓存
// ——慢站短期不再重试，否则每次请求都被拖到超时。修复不能把这条一起关掉。
func TestMessagesTimeoutStaysCached(t *testing.T) {
	src := &fakeMsgAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		byMailbox:   map[site.MailboxType][]site.Message{site.MailboxInbox: {{Title: "M"}}},
		proceed:     make(chan struct{}),
	}
	ch, _ := newMsgChain(t, src)
	t.Cleanup(func() { close(src.proceed) })

	caller, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	res := ch.MessagesAll(caller, site.MailboxInbox, true)
	// 断言按语义（「超时」）而不是英文原文：逐条结果面的文案已收口到 errtext
	// 单一来源（面向用户是中文），本例要守的是「超时必须作为错误暴露 + 仍被负缓存」。
	if len(res) != 1 || !strings.Contains(res[0].Error, "超时") {
		t.Fatalf("前提：超时应作为错误暴露，got %+v", res)
	}

	before := atomic.LoadInt32(&src.calls)
	res2 := ch.MessagesAll(context.Background(), site.MailboxInbox, false)
	if got := atomic.LoadInt32(&src.calls); got != before {
		t.Errorf("超时结果应仍被负缓存（不再回源）：calls %d→%d", before, got)
	}
	if len(res2) != 1 || !strings.Contains(res2[0].Error, "超时") {
		t.Errorf("普通读应回读钉住的超时错误，got %+v", res2)
	}
}

// newMsgPanicChain 两站同 schema（消息能力只对 nexusphp 默认开启；混用 mteam 会在派发到适配器
// 之前就短路，panic 通道压根走不到）：good 正常出消息，crash 的 baseURL 命中 panicHost ⇒
// FetchMessages 现场 panic。
func newMsgPanicChain(t *testing.T) *SiteChain {
	t.Helper()
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	for _, name := range []string{"good", "crash"} {
		if _, err := siteOp.Create(oper.SiteInput{
			Name: name, Platform: "nexusphp", BaseURL: "https://" + name + ".example", IsActive: true,
		}); err != nil {
			t.Fatalf("创建站点 %s: %v", name, err)
		}
	}
	mm := module.New()
	mm.Register(&fakeMsgAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		byMailbox:   map[site.MailboxType][]site.Message{site.MailboxInbox: {{Title: "M"}}},
		panicHost:   "crash",
	})
	return NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())
}

// TestMessagesAllReportsPanickingSite 崩站必须留下一行带站名与 panic 原因的记录。修复前
// results[i] 停在零值：site_id=0、site_name=""、messages=nil（序列化成 JSON null），
// 「这站崩了」被渲染成「这站没有消息」，而且前端对 null 取 .length 会崩。
func TestMessagesAllReportsPanickingSite(t *testing.T) {
	ch := newMsgPanicChain(t)

	results := ch.MessagesAll(context.Background(), site.MailboxInbox, true)
	if len(results) != 2 {
		t.Fatalf("results len=%d want 2", len(results))
	}
	var crash *MessageResult
	for i := range results {
		if results[i].SiteName == "crash" {
			crash = &results[i]
		}
	}
	if crash == nil {
		t.Fatalf("崩站必须带站名出现在结果里，got %+v", results)
	}
	if crash.SiteID == 0 {
		t.Error("崩站行的 site_id 不应为零值")
	}
	if crash.Messages == nil {
		t.Error("崩站行的 messages 必须是空数组（nil 会序列化成 JSON null）")
	}
	if !strings.Contains(crash.Error, "消息抓取异常") || !strings.Contains(crash.Error, "malformed message page") {
		t.Errorf("崩站行要带中文前缀与 panic 载荷（后者证明真的走到了适配器），got error=%q", crash.Error)
	}
	for _, r := range results {
		if r.SiteName == "good" && (r.Error != "" || len(r.Messages) != 1) {
			t.Errorf("正常站应照常出消息，got %+v", r)
		}
	}
}

// TestMessagesStreamEmitsFrameForPanickingSite 流式（NDJSON/SSE）面同一轴：崩站一帧都不推，
// 前端只看到「少一个站」，而 listErr 只覆盖清单读失败，帮不上这一路。
func TestMessagesStreamEmitsFrameForPanickingSite(t *testing.T) {
	ch := newMsgPanicChain(t)

	var mu sync.Mutex
	var got []MessageResult
	if listErr := ch.MessagesStream(context.Background(), site.MailboxInbox, true, func(r MessageResult) {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
	}); listErr != "" {
		t.Fatalf("listErr=%q", listErr)
	}
	if len(got) != 2 {
		t.Fatalf("每站都该有一帧（含崩站），got %d 帧 %+v", len(got), got)
	}
	var crash *MessageResult
	for i := range got {
		if got[i].SiteName == "crash" {
			crash = &got[i]
		}
	}
	if crash == nil || !strings.Contains(crash.Error, "消息抓取异常") || !strings.Contains(crash.Error, "malformed message page") {
		t.Fatalf("崩站帧要带中文前缀与 panic 载荷（后者证明真的走到了适配器），got %+v", got)
	}
}
