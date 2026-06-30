package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
)

// runMessages 周期轮询站点收件箱，diff 出新邮件 → 通知中心。
// seen message ID 集合存 SystemConfig（key mail:seen:站名，JSON），新 ID = 当前 − seen。
func (s *Service) runMessages() {
	if s.notice == nil || s.cfgOp == nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.rootCtx, 5*time.Minute)
	defer cancel()
	results := s.siteCh.MessagesAll(ctx, site.MailboxInbox)
	for _, r := range results {
		if r.Error != "" || len(r.Messages) == 0 {
			continue
		}
		seen := loadSeenIDs(s.cfgOp, r.SiteName)
		cur := make(map[uint64]struct{}, len(r.Messages))
		fresh := 0
		for i := range r.Messages {
			m := &r.Messages[i]
			cur[m.ID] = struct{}{}
			if _, ok := seen[m.ID]; !ok {
				fresh++
			}
		}
		if fresh > 0 {
			s.notice.Notice(ctx, notice.Input{
				Source: notice.SourceNewMail,
				Key:    "mail:" + r.SiteName + ":" + time.Now().Format("2006-01-02"),
				Title:  "新邮件：" + r.SiteName,
				Body:   "收到 " + strconv.Itoa(fresh) + " 条新私信",
				Level:  "info",
			}, true)
		}
		saveSeenIDs(s.cfgOp, r.SiteName, cur)
	}
	slog.Debug("messages check done", "sites", len(results))
}

// shortHash 取文本 sha256 前 12 位 hex（公告 diff 基准）。
func shortHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:6])
}

// truncate 截断到 max 字符（通知 body 防过长）。
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// loadSeenIDs 读某站已见邮件 ID 集合（JSON 存 SystemConfig）。
func loadSeenIDs(cfgOp *oper.SystemConfigOper, siteName string) map[uint64]struct{} {
	raw, _ := cfgOp.Get("mail:seen:" + siteName)
	if raw == "" {
		return map[uint64]struct{}{}
	}
	var ids []uint64
	if json.Unmarshal([]byte(raw), &ids) != nil {
		return map[uint64]struct{}{}
	}
	m := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

// saveSeenIDs 写某站已见邮件 ID 集合。
func saveSeenIDs(cfgOp *oper.SystemConfigOper, siteName string, ids map[uint64]struct{}) {
	list := make([]uint64, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	raw, _ := json.Marshal(list)
	_ = cfgOp.Set("mail:seen:"+siteName, string(raw))
}
