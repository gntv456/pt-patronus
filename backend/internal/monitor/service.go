package monitor

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// Notifier 发送告警通知的回调（由 app.go 适配到 NotificationChain，解耦 monitor 与 chain/notification）。
type Notifier func(ctx context.Context, title, body, level string)

// Store 抽象 Oper 中 service/alerter 需要的方法（避免 monitor ↔ oper 循环依赖；oper.MonitorOper 隐式实现）。
type Store interface {
	ListEnabled() ([]models.MonitorTarget, error)
	Get(id uint) (*models.MonitorTarget, error)
	Credentials(t *models.MonitorTarget) (Credentials, error)
	RecordSnapshot(t *models.MonitorTarget, m Metrics, err error) (Metrics, error)
	UpdateRTSPURL(id uint, url string) error
	CreateAlert(a *models.MonitorAlert) error
	ResolveOpenAlerts(targetID uint, metric string) error
	LastOpenAlert(targetID uint, metric string) (*models.MonitorAlert, error)
}

// Service 编排监控采集：单 ticker 扫表 → 到点并发采集（ctx 超时）→ 写快照 → 告警评估。
type Service struct {
	store   Store
	reg     *Registry
	alerter *Alerter
	tick    time.Duration
}

// NewService 构造。tick<=0 默认 10s。ns 为站内通知中心（离线告警发通知；nil 跳过）。
func NewService(store Store, reg *Registry, notif Notifier, tick time.Duration, ns *notice.Service) *Service {
	if tick <= 0 {
		tick = 10 * time.Second
	}
	return &Service{store: store, reg: reg, alerter: NewAlerter(store, notif, ns), tick: tick}
}

// Run 常驻采集循环（与 ws.Broadcaster.Run 同模式；由 app.go 起 goroutine）。
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scanAndCollect(ctx)
		}
	}
}

// scanAndCollect 遍历启用目标，到点的并发采集。
func (s *Service) scanAndCollect(ctx context.Context) {
	targets, err := s.store.ListEnabled()
	if err != nil {
		return
	}
	now := time.Now()
	// 并发采集上限，防大量不可达 target 同时烧满 SSH/RTSP 超时致 fd/socket 耗尽（参考 chain 包 semaphore 范式）。
	// wg.Wait 串行化轮次：采集慢时 ticker 自然降频，而非堆积 goroutine。
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := range targets {
		t := &targets[i]
		if t.LastCheckedAt != nil && now.Sub(*t.LastCheckedAt) < time.Duration(t.IntervalSec)*time.Second {
			continue // 未到点
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			s.collectOne(ctx, t)
		}()
	}
	wg.Wait()
}

// collectOne 采集单个目标（错误隔离 + 超时 + 回填摄像头 RTSPURL）。
func (s *Service) collectOne(parent context.Context, t *models.MonitorTarget) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("monitor collect panic", "target", t.Name, "panic", r)
		}
	}()
	// agent target 由 push 链路（AgentWSHub → IngestAgentReport）负责，跳过 pull 采集。
	if ResolveProtocol(t) == "agent" {
		return
	}
	timeout := time.Duration(t.TimeoutSec) * time.Second
	if timeout < 5*time.Second {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	c, ok := s.reg.CollectorFor(t.Type, ResolveProtocol(t))
	if !ok {
		_, _ = s.store.RecordSnapshot(t, Metrics{Online: false}, ErrUnsupported)
		s.alerter.Evaluate(ctx, t, Metrics{Online: false}, ErrUnsupported)
		return
	}
	creds, _ := s.store.Credentials(t)
	m, err := c.Collect(ctx, t, creds)
	m, _ = s.store.RecordSnapshot(t, m, err)
	if t.Type == "camera" && t.RTSPURL == "" {
		if u, ok := m.Extra["rtsp_url"].(string); ok && u != "" {
			_ = s.store.UpdateRTSPURL(t.ID, u)
		}
	}
	s.alerter.Evaluate(ctx, t, m, err)
}

// CollectNow 手动触发单次采集并返回结果（handler 用）。
func (s *Service) CollectNow(ctx context.Context, id uint) (Metrics, error) {
	t, err := s.store.Get(id)
	if err != nil {
		return Metrics{}, err
	}
	if ResolveProtocol(t) == "agent" {
		return Metrics{}, errors.New("该设备由 agent 主动上报，不支持手动采集")
	}
	c, ok := s.reg.CollectorFor(t.Type, ResolveProtocol(t))
	if !ok {
		return Metrics{}, ErrUnsupported
	}
	creds, _ := s.store.Credentials(t)
	m, err := c.Collect(ctx, t, creds)
	m, _ = s.store.RecordSnapshot(t, m, err)
	s.alerter.Evaluate(ctx, t, m, err)
	return m, err
}

// ProbeNow 手动探活（handler 用）。agent 设备的在线状态由 WS 连接维护，返回 DB 里的 last_online。
func (s *Service) ProbeNow(ctx context.Context, id uint) (bool, error) {
	t, err := s.store.Get(id)
	if err != nil {
		return false, err
	}
	if ResolveProtocol(t) == "agent" {
		return t.LastOnline != nil && *t.LastOnline, nil
	}
	c, ok := s.reg.CollectorFor(t.Type, ResolveProtocol(t))
	if !ok {
		return false, ErrUnsupported
	}
	creds, _ := s.store.Credentials(t)
	return c.Probe(ctx, t, creds)
}

// IngestAgentReport 接收 agent push 上报的指标，复用 pull 链路的存储+告警（绕过 Collector）。
// agent target 在 collectOne 中被跳过不会重复采集；网络速率由 RecordSnapshot 对比上次快照自动算。
func (s *Service) IngestAgentReport(ctx context.Context, targetID uint, m Metrics) error {
	t, err := s.store.Get(targetID)
	if err != nil {
		return err
	}
	if ResolveProtocol(t) != "agent" {
		return errors.New("target is not an agent target")
	}
	m, err = s.store.RecordSnapshot(t, m, nil)
	if err != nil {
		return err
	}
	s.alerter.Evaluate(ctx, t, m, nil)
	return nil
}
