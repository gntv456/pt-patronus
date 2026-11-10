// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package notice 实现全局通知中心：聚合 13 类业务事件为站内通知。
// NoticeService 统一入口：去重 → 持久化 → WS 实时推送 → 外发渠道（bark/tg/webhook）。
package notice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notification"
)

// Store 抽象 oper.NoticeOper 的方法（避免 notice→oper 循环：oper→monitor→notice）。
// oper.NoticeOper 隐式实现本接口。
type Store interface {
	Create(n *models.Notice) error
	List(unread bool, limit int) ([]models.Notice, error)
	Count(unread bool) (int64, error)
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
	SourceSiteAnniv     = "site_anniv"     // 今日站庆
	SourceNewMail       = "new_mail"       // 新邮件
	SourceAssessPass    = "assess_pass"    // 考核完成
	SourceAssessFail    = "assess_fail"    // 考核失败
	SourceAnnounce      = "announce"       // 新公告
	SourceHostOffline   = "host_offline"   // 服务器掉线
	SourceNASOffline    = "nas_offline"    // NAS 掉线
	SourceCameraOffline = "camera_offline" // 摄像头掉线（小米等 P2P 设备经 RTSP 探活失败）
	SourceReseedDone    = "reseed_done"    // 辅种完成
	SourceReseedFail    = "reseed_fail"    // 辅种 Torznab 搜索失败（端点配置坏但搜索静默返回空）
	SourceDLDone        = "dl_done"        // 下载完成
	SourceSiteOffline   = "site_offline"   // 站点掉线（cookie 失效）
	SourceBackupDone    = "backup_done"    // 备份完成
	SourceBackupImport  = "backup_import"  // 备份导入成功
	SourceFreeleech     = "freeleech"      // 今日站免
	SourceSubscribeHit  = "subscribe_hit"  // 订阅命中（自动下载）
	// 订阅本轮被拦下/跳过（未配置下载器/评分闸门/站点不可用）
	SourceSubscribeCheckFail = "subscribe_check_fail"
	SourceSubscribeListFail  = "subscribe_list_fail" // 订阅整轮检查未开始（订阅列表读失败）
	SourceRssListFail        = "rss_list_fail"       // RSS 整轮扫描未开始（启用订阅读失败）
	SourceRssCheckFail       = "rss_check_fail"      // RSS 单个订阅本轮抓取/解析失败（cron 只留日志＝界面上永远"健康"）
	SourceRssDownloadFail    = "rss_download_fail"   // RSS 自动下载推种失败
	// 条目已回滚、下轮重试；cron 只留日志时条目列表永远为空（静默）。
	SourceStrmListFail      = "strm_list_fail"      // STRM 后台维护未开始（续签/追更读库失败）
	SourceBrushflowListFail = "brushflow_list_fail" // 刷流整轮未开始（任务清单读失败）
	SourceContractListFail  = "contract_list_fail"  // 保种契约整轮未检查（规则清单读失败）
	SourceDirSyncListFail   = "dirsync_list_fail"   // 目录同步整轮未开始（任务清单读失败/配置损坏）
	SourceBrushflow         = "brushflow"           // 刷流删种等运行事件
	SourceCloud             = "cloud"               // 网盘任务 / 订阅追更事件
	SourceContract          = "contract"            // 保种契约违约告警
	SourceMediaServer       = "mediaserver"         // 媒体库事件（刷新/扫描）
	SourceWorkflow          = "workflow"            // 工作流执行（手动/定时/事件）
	SourceRssHit            = "rss_hit"             // RSS 订阅命中（自动下载）
	SourceWebhook           = "webhook"             // 进站 Webhook 触发
	SourceTransferDone      = "transfer_done"       // 媒体整理完成
	SourceTransferFail      = "transfer_fail"       // 媒体整理失败
	SourceSubtitleFix       = "subtitle_fix"        // 字幕补齐完成（健康中心一键补齐）
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

// noticeSeq 为空 Key 合成唯一键的进程内序号（UnixNano 在 Windows 分辨率粗，单靠它可能撞）。
var noticeSeq atomic.Uint64

// Notice 发一条通知。dedup=true 时按 Source+Key 当天去重（周期事件，避免 cron 刷屏）；
// false 时一次性（备份完成/辅种完成等）。始终写库 + WS 广播 + 外发。
func (s *Service) Notice(ctx context.Context, in Input, dedup bool) error {
	if in.Source == "" || in.Title == "" {
		return nil
	}
	if dedup {
		ok, err := s.store.ExistsSince(in.Source, in.Key, startOfToday())
		if err != nil {
			// 调用点几乎全是 `_ = svc.Notice(...)`（本仓 20+ 处，且多半是「读不出来要出声」
			// 那类自曝通知）。错误只回不记就等于什么都没发生：提醒丢了，日志里也找不到。
			// 判据修在这一处，胜过逐个调用点补日志。
			slog.Warn("通知去重查询失败，这条提醒没有发出",
				"source", in.Source, "key", in.Key, "title", in.Title, "err", err)
			return err
		}
		if ok {
			return nil
		}
	}
	// 空 Key 是「无去重键」语义（ExistsSince 同样按空 Key 跳过 key 过滤），但唯一索引
	// uk_notice_src_key(source,key) 把 (source,"") 当同一键：NoticeOper.Create 的
	// OnConflict{DoNothing} 从第二条起静默吞掉且不报错——用户永远只见第一条「辅种完成/
	// 备份完成」等一次性通知。入库前补合成唯一键；带 Key 通知仍受唯一索引并发去重保护。
	key := in.Key
	if key == "" {
		key = fmt.Sprintf("oneoff:%d:%d", time.Now().UnixNano(), noticeSeq.Add(1))
	}
	n := &models.Notice{
		Source:    in.Source,
		Key:       key,
		Title:     in.Title,
		Body:      in.Body,
		Level:     orDefault(in.Level, "info"),
		CreatedAt: time.Now(),
	}
	if err := s.store.Create(n); err != nil {
		slog.Error("通知落库失败：收件箱里看不到这条提醒",
			"source", n.Source, "key", n.Key, "title", n.Title, "level", n.Level, "err", err)
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
			results := s.outbox.Notify(context.WithoutCancel(ctx), msg)
			failed := failedChannels(results)
			if len(failed) == 0 {
				return
			}
			// 渠道级失败本身在 notification/manager.go:119 已有一条 Warn("notify failed")，
			// 第一次与重试各打一条——但那两行同形，只说「某渠道发不出去」，既没有这条通知是谁
			// （notice_id/source/title），也没有说出终局：**重试也没救回来，这条 danger/warning
			// 告警不会再有下一次机会**。上面注释写明重试是给「备份结果这类一次性事件丢失不可恢复」
			// 兜底的，兜底失败却要靠运维自己数两行同形日志才能推断出来，等于没有结论。
			time.Sleep(noticeRetryDelay)
			// 仅重发失败渠道：整体重发会让已成功渠道（如 TG 成功、Bark 失败）重复收到推送。
			if retried := s.outbox.NotifyChannels(context.Background(), failed, msg); hasSendFailure(retried) {
				slog.Error("通知外发重试仍失败：这条告警没有送达任何外部渠道（渠道级明细见前一条 notify failed）",
					"notice_id", n.ID, "source", n.Source, "title", n.Title, "level", n.Level,
					"channels", strings.Join(failedChannels(retried), ","), "err", firstSendError(retried))
			}
		}()
	}
	return nil
}

// firstSendError 取第一条失败渠道的原因（渠道侧已脱敏，见 notification.sanitizeNotifyErr）。
func firstSendError(results []notification.SendResult) string {
	for _, r := range results {
		if !r.Ok {
			return r.Channel + ": " + r.Error
		}
	}
	return ""
}

// noticeRetryDelay 外发失败重试等待（包级变量便于测试注入小值）。
var noticeRetryDelay = 30 * time.Second

// shouldSendExternal 外发级别门槛：danger/warning 外发；info 仅站内+WS（防风暴）。
func shouldSendExternal(level string) bool {
	return level == "danger" || level == "warning" || level == "success"
}

// hasSendFailure 任一渠道失败即 true（触发重试）。
func hasSendFailure(results []notification.SendResult) bool {
	return len(failedChannels(results)) > 0
}

// failedChannels 提取发送失败的渠道名（重试只打这些渠道）。
func failedChannels(results []notification.SendResult) []string {
	var out []string
	for _, r := range results {
		if !r.Ok {
			out = append(out, r.Channel)
		}
	}
	return out
}

// List 列通知（按创建时间倒序）。unread=true 仅未读。
func (s *Service) List(unread bool, limit int) ([]models.Notice, error) {
	return s.store.List(unread, limit)
}

// Count 与 List 同一套筛选的条数：列表有单页上限，账面不同行给出就把一页读成全部。
func (s *Service) Count(unread bool) (int64, error) { return s.store.Count(unread) }

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
