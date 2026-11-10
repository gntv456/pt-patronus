// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

// TestMessagesAllMarksUnwiredSite 站点定义没开私信（unit3d / gazelle / tcg / 小 horse 等平台的
// 默认值就是这样）时，这一站必须被标成「未接线」，而不是和「抓到了，这站没私信」同形。
// 刻意不塞进 Error：Error 会被当作站点故障计数，未接线是配置事实不是故障。
func TestMessagesAllMarksUnwiredSite(t *testing.T) {
	ch, siteOp := newSearchChain(t, &fakeMsgAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		byMailbox:   map[site.MailboxType][]site.Message{site.MailboxInbox: {{Title: "M"}}},
	})
	createActive(t, siteOp, "nexus", "nexusphp")
	createActive(t, siteOp, "unit", "unit3d")

	results := ch.MessagesAll(context.Background(), site.MailboxInbox, true)
	if len(results) != 2 {
		t.Fatalf("results len=%d want 2", len(results))
	}
	var unwired, wired *MessageResult
	for i := range results {
		switch results[i].SiteName {
		case "unit":
			unwired = &results[i]
		case "nexus":
			wired = &results[i]
		}
	}
	if unwired == nil {
		t.Fatalf("未接线站没出现在结果里: %+v", results)
	}
	if !unwired.Unsupported {
		t.Errorf("未接线站必须标 Unsupported，got %+v", unwired)
	}
	if unwired.Error != "" {
		t.Errorf("未接线不是故障，不该写进 Error，got %q", unwired.Error)
	}
	if unwired.Messages == nil {
		t.Error("未接线行的 messages 必须是空数组")
	}
	if wired == nil || wired.Unsupported || len(wired.Messages) != 1 {
		t.Errorf("已接线站不受影响，got %+v", wired)
	}
}

// rowByName 按站名取一行（聚合结果顺序不保证，靠站名定位）。
func rowByName(t *testing.T, results []MessageResult, name string) *MessageResult {
	t.Helper()
	for i := range results {
		if results[i].SiteName == name {
			return &results[i]
		}
	}
	t.Fatalf("结果里没有站 %q: %+v", name, results)
	return nil
}

// TestMessagesCacheHitMarksUnwiredSite 缓存命中那一条出口以前自己拼结构体，绕过了 msgResult。
// Unsupported 是新字段：只要有一条出口忘了接上，用户第二轮读（命中缓存）就会看到未适配的站
// 退化成「这站没私信」——首轮还是对的，更难发现。
func TestMessagesCacheHitMarksUnwiredSite(t *testing.T) {
	ch, siteOp := newSearchChain(t, &fakeMsgAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		byMailbox:   map[site.MailboxType][]site.Message{site.MailboxInbox: {{Title: "M"}}},
	})
	createActive(t, siteOp, "nexus", "nexusphp")
	createActive(t, siteOp, "unit", "unit3d")

	// 首轮 force 回源并写缓存。
	if got := ch.MessagesAll(context.Background(), site.MailboxInbox, true); len(got) != 2 {
		t.Fatalf("首轮应有 2 行，got %+v", got)
	}
	// 次轮非 force：两站都应命中缓存（TTL 2 分钟，刚写入必不过期）。
	second := ch.MessagesAll(context.Background(), site.MailboxInbox, false)
	if len(second) != 2 {
		t.Fatalf("次轮应有 2 行，got %+v", second)
	}
	if u := rowByName(t, second, "unit"); !u.Unsupported {
		t.Errorf("缓存命中路径也要标 Unsupported，got %+v", u)
	}
	if n := rowByName(t, second, "nexus"); n.Unsupported || len(n.Messages) != 1 {
		t.Errorf("已接线站缓存行不该受影响，got %+v", n)
	}
}

// TestMessagesCacheHitNormalizesNilMessages 缓存命中路径的 messages 必须归一成空数组。
// 现有写入方（fetchMessages / writeMsgCache）都会先把 nil 变成 []，所以这条是纵深防御：
// 直接投放一条 nil 缓存，证明「以后有写入方漏归一」时 null 不会发给前端（页面在
// r.messages.length 崩）。
func TestMessagesCacheHitNormalizesNilMessages(t *testing.T) {
	ch, s := newMsgChain(t, &fakeMsgAdapter{fakeAdapter: &fakeAdapter{schema: "nexusphp"}})
	ch.msgMu.Lock()
	ch.msgCache[msgCacheKey(s.ID, site.MailboxInbox)] = msgCacheEntry{messages: nil, at: time.Now()}
	ch.msgMu.Unlock()

	got := ch.MessagesAll(context.Background(), site.MailboxInbox, false)
	if len(got) != 1 {
		t.Fatalf("应命中缓存并返回 1 行，got %+v", got)
	}
	if got[0].Messages == nil {
		t.Error("缓存命中路径的 messages 也必须是空数组（走 msgResult 归一）")
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"messages":null`) {
		t.Errorf("不得把 null 发给前端，got %s", raw)
	}
}
