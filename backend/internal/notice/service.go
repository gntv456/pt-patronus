// Package notice 实现全局通知中心：聚合 13 类业务事件为站内通知。
// NoticeService 统一入口：去重 → 持久化 → WS 实时推送 → 外发渠道（bark/tg/webhook）。
package notice

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notification"
)

// Store 抽象 oper.NoticeOper 的方法（避免 notice→oper 循环：oper→monitor→notice）。
// oper.NoticeOper 隐式实现本接口。
type Store interface {
	Create(n *models.Notice) error
	List(unread bool, limit int) ([]models.Notice, error)
	MarkRead(id uint) error
	MarkAllRead() error
	Delete(id uint) error
	ClearAll() (int64, error)
	ExistsSince(source, key string, since time.Time) (bool, error)
	CountUnread() (int64, error)
	Cleanup(before time.Time) (int64, error)
}

// Broadcaster 把消息字节广播给所有在线 WS 客户（app.go 适配 ws.Hub.Broadcast）。nil 跳过。
// 用回调而非直接依赖 ws 包，避免 notice→ws→oper→monitor→notice 循环。
type Broadcaster func(payload []byte)

// noticeMsg 是推送给前端通知中心的 WS 消息体（type:"notice"，对齐前端 useWs 解析）。
type noticeMsg struct {
	Type string     `json:"type"`
	Data noticeData `json:"data"`
}

type noticeData struct {
	ID        uint      `json:"id"`
	Source    string    `json:"source"`
	Title     string    `json:"title"`
	Body      string    `json:"body,omitempty"`
	Level     string    `json:"level"`
	CreatedAt time.Time `json:"created_at"`
}

// 13 类通知 Source 枚举（事件 tag）。
const (
	SourceSiteAnniv    = "site_anniv"    // 今日站庆
	SourceNewMail      = "new_mail"      // 新邮件
	SourceAssessPass   = "assess_pass"   // 考核完成
	SourceAssessFail   = "assess_fail"   // 考核失败
	SourceAnnounce     = "announce"      // 新公告
	SourceHostOffline  = "host_offline"  // 服务器掉线
	SourceNASOffline   = "nas_offline"   // NAS 掉线
	SourceReseedDone   = "reseed_done"   // 辅种完成
	SourceDLDone       = "dl_done"       // 下载完成
	SourceSiteOffline  = "site_offline"  // 站点掉线（cookie 失效）
	SourceBackupDone   = "backup_done"   // 备份完成
	SourceBackupImport = "backup_import" // 备份导入成功
	SourceFreeleech    = "freeleech"     // 今日站免
	SourceSubscribeHit = "subscribe_hit" // 订阅命中（自动下载）
	SourceWorkflow     = "workflow"      // 工作流执行（手动/定时/事件）
	SourceRssHit       = "rss_hit"       // RSS 订阅命中（自动下载）
	SourceWebhook      = "webhook"       // 进站 Webhook 触发
	SourceTransferDone = "transfer_done" // 媒体整理完成
	SourceTransferFail = "transfer_fail" // 媒体整理失败
	SourceSubtitleFix  = "subtitle_fix"  // 字幕补齐完成（健康中心一键补齐）
	// 勋章墙价格/状态变动（定时刷新 diff 触发）
	SourceMedalNew        = "medal_new"          // 新勋章可购买
	SourceMedalAvailable  = "medal_available"    // 上架可购买
	SourceMedalPriceDrop  = "medal_price_drop"   // 降价
	SourceMedalPriceRise  = "medal_price_rise"   // 涨价
	SourceMedalOutOfStock = "medal_out_of_stock" // 售罄
)

// Input 是发通知的入参。
type Input struct {
	Source string
	Key    string // 去重键（周期事件填，如 "anniv:站名:2026-06-28"；一次性事件留空）
	Title  string
	Body   string
	Level  string // info | success | warning | danger
}

// Service 站内通知服务：去重 → 写库 → WS 广播 → 外发渠道。
type Service struct {
	store  Store
	bcast  Broadcaster           // WS 实时广播（app.go 适配 ws.Hub.Broadcast）；nil 跳过
	outbox *notification.Manager // 外发渠道（bark/tg/webhook）；nil 跳过外发
}

// NewService 构造。store 由 oper.NoticeOper 实现；bcast/outbox 可为 nil。
func NewService(store Store, bcast Broadcaster, outbox *notification.Manager) *Service {
	return &Service{store: store, bcast: bcast, outbox: outbox}
}

// Notice 发一条通知。dedup=true 时按 Source+Key 当天去重（周期事件，避免 cron 刷屏）；
// false 时一次性（备份完成/辅种完成等）。始终写库 + WS 广播 + 外发。
func (s *Service) Notice(ctx context.Context, in Input, dedup bool) error {
	if in.Source == "" || in.Title == "" {
		return nil
	}
	if dedup {
		ok, err := s.store.ExistsSince(in.Source, in.Key, startOfToday())
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	n := &models.Notice{
		Source:    in.Source,
		Key:       in.Key,
		Title:     in.Title,
		Body:      in.Body,
		Level:     orDefault(in.Level, "info"),
		CreatedAt: time.Now(),
	}
	if err := s.store.Create(n); err != nil {
		return err
	}
	// WS 实时推送给在线前端（无连接时为空操作）
	if s.bcast != nil {
		if payload, err := json.Marshal(noticeMsg{Type: "notice", Data: noticeData{
			ID: n.ID, Source: n.Source, Title: n.Title, Body: n.Body, Level: n.Level, CreatedAt: n.CreatedAt,
		}}); err == nil {
			s.bcast(payload)
		}
	}
	// 外发渠道（bark/telegram/webhook）。
	// 二轮 P2-2/P2-3：info 级不外发（站内+WS 已可见；跨站批量 info 曾造成
	// 50 条 TG 风暴——汇总条走 notif.Notify 直发不受此限）；danger/warning 外发
	// 失败时进程内退避重试一次（一次性事件如备份结果丢失不可恢复）。
	if s.outbox != nil && shouldSendExternal(n.Level) {
		msg := notification.Message{Title: in.Title, Body: in.Body, Level: mapLevel(in.Level)}
		go func() {
			if results := s.outbox.Notify(context.WithoutCancel(ctx), msg); hasSendFailure(results) {
				time.Sleep(30 * time.Second)
				s.outbox.Notify(context.Background(), msg)
			}
		}()
	}
	return nil
}

// shouldSendExternal 外发级别门槛：danger/warning 外发；info 仅站内+WS（防风暴）。
func shouldSendExternal(level string) bool {
	return level == "danger" || level == "warning" || level == "success"
}

// hasSendFailure 任一渠道失败即 true（触发重试）。
func hasSendFailure(results []notification.SendResult) bool {
	for _, r := range results {
		if !r.Ok {
			return true
		}
	}
	return false
}

// List 列通知（按创建时间倒序）。unread=true 仅未读。
func (s *Service) List(unread bool, limit int) ([]models.Notice, error) {
	return s.store.List(unread, limit)
}

// MarkRead 标单条已读。
func (s *Service) MarkRead(id uint) error { return s.store.MarkRead(id) }

// MarkAllRead 全部已读。
func (s *Service) MarkAllRead() error { return s.store.MarkAllRead() }

// Delete 删单条。
func (s *Service) Delete(id uint) error { return s.store.Delete(id) }

// ClearAll 一键清空全部通知。返回删除条数。
func (s *Service) ClearAll() (int64, error) { return s.store.ClearAll() }

// CountUnread 未读数（铃铛角标）。
func (s *Service) CountUnread() (int64, error) { return s.store.CountUnread() }

// startOfToday 本地时区今日 0 点（去重「当天」基准）。
func startOfToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// mapLevel 站内 level → 外发 Message.Level。
func mapLevel(level string) string {
	switch level {
	case "danger", "warning", "info", "success":
		return level
	default:
		return "info"
	}
}
