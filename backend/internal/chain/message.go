package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/site"
)

// 消息缓存 TTL（平衡档：私信时效 vs 体验；过期走 SWR 后台刷新，故可较短）。
// 并发/超时复用 site_info.go 的 infoFetchConcurrency(8)/infoFetchTimeout(15s)，与公告/站免同值。
const messageCacheTTL = 2 * time.Minute

// MessageResult 是单站消息聚合结果。
type MessageResult struct {
	SiteID   uint           `json:"site_id"`
	SiteName string         `json:"site_name"`
	Messages []site.Message `json:"messages"`
	Error    string         `json:"error,omitempty"`
	// Unsupported 表示这一站的私信压根没接线（站点定义未开启 / 该平台未适配）。
	// 它必须与「抓到了，这站没有私信」分开：前者是配置事实，后者是站点现状，
	// 混在一起用户会以为站点真的清空了私信。刻意不复用 Error——那会被当成故障计数。
	Unsupported bool `json:"unsupported,omitempty"`
}

// msgSFResult 是消息单次回源的 singleflight 结果（messages + 展示用 errMsg）。
type msgSFResult struct {
	messages []site.Message
	errMsg   string
}

// Messages 抓单站消息（capability dispatch + 盖章 siteID/name/ID）。
func (c *SiteChain) Messages(ctx context.Context, s *models.Site, mailbox site.MailboxType) ([]site.Message, error) {
	def := c.defFor(s)
	if !def.Messages.Enabled {
		return []site.Message{}, nil
	}
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
	var out []site.Message
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		mc, ok := a.(site.MessageCapable)
		if !ok {
			return ErrNotSupported
		}
		msgs, e := mc.FetchMessages(ctx, def, s.BaseURL, token, mailbox)
		if e != nil {
			return e
		}
		out = msgs
		return nil
	})
	// 盖章站点身份 + 合成 ID
	for i := range out {
		out[i].SiteID = s.ID
		out[i].SiteName = s.Name
		out[i].Mailbox = mailbox
		out[i].FetchedAt = time.Now()
		if out[i].AdminActions != nil && out[i].AdminActions.ItemID != "" {
			if n, e := strconv.ParseUint(out[i].AdminActions.ItemID, 10, 64); e == nil {
				out[i].ID = n
			}
		}
		if out[i].ID == 0 {
			out[i].ID = site.SynthesizeMsgID(s.ID, mailbox, i)
		}
	}
	if out == nil {
		out = []site.Message{}
	}
	return out, err
}

// msgCacheKey 消息缓存 key：siteID + mailbox（信箱 6 类，防切信箱串数据）。
func msgCacheKey(siteID uint, mailbox site.MailboxType) string {
	return strconv.Itoa(int(siteID)) + "|" + string(mailbox)
}

// MessagesWired 该站的私信是否接了线（站点定义开启 messages capability）。
// 判定只写在这一处：聚合面与单站同步面都要用它，抄第二遍必漂移。
func (c *SiteChain) MessagesWired(s *models.Site) bool { return c.defFor(s).Messages.Enabled }

// resolveMessage 解析单站消息结果（缓存命中/SWR/miss/force 同聚合逻辑），供聚合与流式复用。
func (c *SiteChain) resolveMessage(ctx context.Context, sem chan struct{}, s models.Site, mailbox site.MailboxType, force bool) MessageResult {
	key := msgCacheKey(s.ID, mailbox)
	// 未接线判定 + 两条出口统一走 msgResult：缓存命中那一条以前自己拼结构体，
	// 新字段只要漏接在一处出口，命中缓存的第二轮读就退化（首轮还正常，最难发现）；
	// nil→[] 归一顺带覆盖，写入方目前都归一了，这里是纵深防御。
	unsupported := !c.MessagesWired(&s)
	row := func(res msgSFResult) MessageResult {
		r := msgResult(s.ID, s.Name, res)
		r.Unsupported = unsupported
		return r
	}
	// 快速路径：缓存存在（含已过期）且非 force → 立即用旧值；过期则后台 SWR 刷新。
	if !force {
		c.msgMu.Lock()
		e, ok := c.msgCache[key]
		c.msgMu.Unlock()
		if ok {
			if time.Since(e.at) > messageCacheTTL {
				c.refreshMessagesAsync(s, mailbox)
			}
			return row(msgSFResult{messages: e.messages, errMsg: e.errMsg})
		}
	}
	// miss 或 force：同步抓取（force 直连回源；miss 走 singleflight 合并防击穿）。
	var res msgSFResult
	if force {
		res = c.fetchMessagesOnce(ctx, sem, &s, mailbox)
	} else {
		res, _ = sfDo(&c.msgSF, key, func() (msgSFResult, error) {
			// double-check：leader 完成后紧随 caller 重跑 fn，缓存可能已满。
			c.msgMu.Lock()
			ce, ok := c.msgCache[key]
			c.msgMu.Unlock()
			if ok {
				return msgSFResult{messages: ce.messages, errMsg: ce.errMsg}, nil
			}
			return c.fetchMessagesOnce(ctx, sem, &s, mailbox), nil
		})
	}
	return row(res)
}

// msgPanicResult 崩站的兜底行（JSON 面与流式面共用，防两处口径漂移）：messages 给空数组
// 而非 nil——零值行的 messages 序列化成 JSON null，前端 .length 直接崩（同 msgResult 归一口径）。
func msgPanicResult(s models.Site, r any) MessageResult {
	return MessageResult{
		SiteID:   s.ID,
		SiteName: s.Name,
		Messages: []site.Message{},
		Error:    fmt.Sprintf("消息抓取异常: %v", r),
	}
}

// MessagesAll 跨站并发聚合消息（per-(site|mailbox) 缓存 + SWR；force=true 跳过缓存强抓）。
// 等全部站完成后一次性返回（MCP 工具/cron 用）；前端页面要"加载一个显示一个"用 MessagesStream。
func (c *SiteChain) MessagesAll(ctx context.Context, mailbox site.MailboxType, force bool) []MessageResult {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("messages list sites failed", "err", err)
		return nil
	}
	results := make([]MessageResult, len(sites))
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		safego.Go("message:126", func() {
			defer wg.Done()
			// per-site 隔离：safego 只记日志，崩掉那一站否则会留下一行匿名空槽
			// （site_id=0 + messages=nil），与「这站没有消息」同形。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("messages site panic", "site", s.Name, "err", r)
					results[i] = msgPanicResult(s, r)
				}
			}()
			results[i] = c.resolveMessage(ctx, sem, s, mailbox, force)
		})
	}
	wg.Wait()
	return results
}

// MessagesStream 流式聚合：每站解析完立即通过 emit 推送（不等全部），供前端"加载一个显示一个"。
// 缓存命中的站秒级 emit，慢站（context deadline 15s）最后到达；缓存/SWR/负缓存逻辑同聚合。
// emit 由调用方保证并发安全（多 goroutine 同时回调）。
// 返回 listErr（""=正常）：站点清单读失败时交回给 handler 的末尾 done 行带出，前端可留痕。
func (c *SiteChain) MessagesStream(ctx context.Context, mailbox site.MailboxType, force bool, emit func(MessageResult)) string {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("messages stream list sites failed", "err", err)
		return err.Error()
	}
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for _, s := range sites {
		wg.Add(1)
		safego.Go("message:148", func() {
			defer wg.Done()
			// per-site 隔离：崩站也必须有一帧，否则前端只看到「少一站」而没有任何错误行。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("messages stream site panic", "site", s.Name, "err", r)
					emit(msgPanicResult(s, r))
				}
			}()
			emit(c.resolveMessage(ctx, sem, s, mailbox, force))
		})
	}
	wg.Wait()
	return ""
}

// fetchMessages 抓单站消息（盖章由 Messages 内部完成）；成功与失败结果都写缓存（负缓存）。
// 失败站（context deadline / SSRF no-host / CF 阻挡 / 未登录）短期不再重试——这些站单次抓取
// 就要等到 15s 超时，若不缓存每次请求都会被少数慢站拖到前端超时。配合 SWR（过期返旧值+
// 后台刷），失败站也仅在缓存过期后由后台异步重试，不阻塞用户。
// ctx 超时由调用方控制：同步路径用 infoFetchTimeout，SWR 后台用独立 Background ctx。
// 例外：调用方中断（切信箱/关页面 → context.Canceled）不写缓存，见 writeMsgCache——
// singleflight 下 leader 被中断时同批 waiter 也会拿到这个结果，但缓存不落盘，下次请求即自愈。
func (c *SiteChain) fetchMessages(ctx context.Context, s *models.Site, mailbox site.MailboxType) msgSFResult {
	msgs, e := c.Messages(ctx, s, mailbox)
	var errMsg string
	if e != nil {
		switch {
		case errors.Is(e, site.ErrNotLoggedIn):
			errMsg = "未登录"
		default:
			errMsg = errtext.Text(e)
			slog.Debug("messages fetch failed", "site", s.Name, "err", e)
		}
	}
	if msgs == nil {
		msgs = []site.Message{}
	}
	c.writeMsgCache(s, mailbox, msgs, errMsg, e)
	return msgSFResult{messages: msgs, errMsg: errMsg}
}

// fetchMessagesOnce 占一个 sem 槽 + 独立超时后抓取（同步路径限流，对标 fetchAnnouncementOnce）。
func (c *SiteChain) fetchMessagesOnce(ctx context.Context, sem chan struct{}, s *models.Site, mailbox site.MailboxType) msgSFResult {
	sem <- struct{}{}
	defer func() { <-sem }()
	siteCtx, cancel := context.WithTimeout(ctx, infoFetchTimeout)
	defer cancel()
	return c.fetchMessages(siteCtx, s, mailbox)
}

// refreshMessagesAsync SWR 后台刷新：缓存过期时立即返回旧值后，异步拉最新写缓存。
// 独立 ctx（不绑请求生命周期，请求结束不中断刷新）+ sfDo 合并同 key（防刷新风暴）+ recover 防 panic。
func (c *SiteChain) refreshMessagesAsync(s models.Site, mailbox site.MailboxType) {
	key := msgCacheKey(s.ID, mailbox)
	go func() {
		defer func() { _ = recover() }()
		_, _ = sfDo(&c.msgSF, key, func() (msgSFResult, error) {
			c.msgMu.Lock()
			ce, ok := c.msgCache[key]
			c.msgMu.Unlock()
			if ok && time.Since(ce.at) <= messageCacheTTL { // double-check：可能已被并发回源填满
				return msgSFResult{messages: ce.messages, errMsg: ce.errMsg}, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), infoFetchTimeout)
			defer cancel()
			return c.fetchMessages(ctx, &s, mailbox), nil
		})
	}()
}

// msgResult 组装单站展示结果（nil → [] 防 JSON null 致前端 .length 崩）。
func msgResult(id uint, name string, res msgSFResult) MessageResult {
	msgs := res.messages
	if msgs == nil {
		msgs = []site.Message{}
	}
	return MessageResult{SiteID: id, SiteName: name, Messages: msgs, Error: res.errMsg}
}

// MessageContent 抓单站消息正文。
func (c *SiteChain) MessageContent(ctx context.Context, s *models.Site, msgURL string) (string, error) {
	def := c.defFor(s)
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return "", cerr
	}
	var out string
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		mc, ok := a.(site.MessageCapable)
		if !ok {
			return ErrNotSupported
		}
		content, e := mc.FetchMessageContent(ctx, def, s.BaseURL, token, msgURL)
		if e != nil {
			return e
		}
		out = content
		return nil
	})
	return out, err
}

// ReplyMessage 回复消息（透传 capability）。
// msgURL 必须落在站点 baseURL 的同注册域下（sameHost 守卫），防止借站点 cookie
// 向其它主机提交写操作。
func (c *SiteChain) ReplyMessage(ctx context.Context, s *models.Site, msgURL, content string) (*site.MessageReplyResult, error) {
	if !sameHost(s.BaseURL, msgURL) {
		return nil, errors.New("reply url host does not match site base url")
	}
	def := c.defFor(s)
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
	var out *site.MessageReplyResult
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		mc, ok := a.(site.MessageCapable)
		if !ok {
			return ErrNotSupported
		}
		r, e := mc.ReplyMessage(ctx, def, s.BaseURL, token, msgURL, content)
		if e != nil {
			return e
		}
		out = r
		return nil
	})
	return out, err
}

// DeleteMessage 删除消息。
// deleteURL 必须落在站点 baseURL 的同注册域下（sameHost 守卫），防止借站点 cookie
// 向其它主机提交删除写操作（URL 白名单）。
func (c *SiteChain) DeleteMessage(ctx context.Context, s *models.Site, deleteURL string) error {
	if !sameHost(s.BaseURL, deleteURL) {
		return errors.New("delete url host does not match site base url")
	}
	def := c.defFor(s)
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return cerr
	}
	return c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		mc, ok := a.(site.MessageCapable)
		if !ok {
			return ErrNotSupported
		}
		return mc.DeleteMessage(ctx, def, s.BaseURL, token, deleteURL)
	})
}

// MarkRead 批量标记已读。
func (c *SiteChain) MarkRead(ctx context.Context, s *models.Site, itemIDs []string) (*site.BatchResult, error) {
	def := c.defFor(s)
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
	var out *site.BatchResult
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		mc, ok := a.(site.MessageCapable)
		if !ok {
			return ErrNotSupported
		}
		r, e := mc.MarkRead(ctx, def, s.BaseURL, token, itemIDs)
		if e != nil {
			return e
		}
		out = r
		return nil
	})
	return out, err
}
