package chain

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// 消息抓取并发与超时。
const (
	msgConcurrency = 8
	msgTimeout     = 15 * time.Second
)

// MessageResult 是单站消息聚合结果。
type MessageResult struct {
	SiteID   uint            `json:"site_id"`
	SiteName string          `json:"site_name"`
	Messages []site.Message  `json:"messages"`
	Error    string          `json:"error,omitempty"`
}

// Messages 抓单站消息（capability dispatch + 盖章 siteID/name/ID）。
func (c *SiteChain) Messages(ctx context.Context, s *models.Site, mailbox site.MailboxType) ([]site.Message, error) {
	def := c.defFor(s)
	if !def.Messages.Enabled {
		return []site.Message{}, nil
	}
	token := c.credential(s, def)
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

// MessagesAll 跨站并发聚合消息（无缓存，每次实时；并发 + 每站超时 + 容错）。
func (c *SiteChain) MessagesAll(ctx context.Context, mailbox site.MailboxType) []MessageResult {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("messages list sites failed", "err", err)
		return nil
	}
	results := make([]MessageResult, len(sites))
	sem := make(chan struct{}, msgConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		go func(i int, s models.Site) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			siteCtx, cancel := context.WithTimeout(ctx, msgTimeout)
			defer cancel()
			msgs, e := c.Messages(siteCtx, &s, mailbox)
			if msgs == nil {
				msgs = []site.Message{} // nil → [] 防 JSON null 致前端 .length 崩
			}
			r := MessageResult{SiteID: s.ID, SiteName: s.Name}
			if e != nil {
				if !errors.Is(e, site.ErrNotLoggedIn) {
					r.Error = e.Error()
					slog.Debug("messages fetch failed", "site", s.Name, "err", e)
				} else {
					r.Error = "未登录"
				}
			}
			r.Messages = msgs
			results[i] = r
		}(i, s)
	}
	wg.Wait()
	return results
}

// MessageContent 抓单站消息正文。
func (c *SiteChain) MessageContent(ctx context.Context, s *models.Site, msgURL string) (string, error) {
	def := c.defFor(s)
	token := c.credential(s, def)
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
	token := c.credential(s, def)
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
	token := c.credential(s, def)
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
	token := c.credential(s, def)
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
