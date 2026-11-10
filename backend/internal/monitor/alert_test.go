// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// fakeStore 仅记录告警创建/恢复/通知，用于验证状态转换去重逻辑。
type fakeStore struct {
	mu       sync.Mutex
	open     map[string]*models.MonitorAlert
	all      []*models.MonitorAlert
	created  int
	resolved []string
}

func newFakeStore() *fakeStore { return &fakeStore{open: map[string]*models.MonitorAlert{}} }

func (f *fakeStore) ListEnabled() ([]models.MonitorTarget, error) { return nil, nil }
func (f *fakeStore) Get(uint) (*models.MonitorTarget, error)      { return nil, nil }
func (f *fakeStore) Credentials(*models.MonitorTarget) (Credentials, error) {
	return Credentials{}, nil
}
func (f *fakeStore) RecordSnapshot(_ *models.MonitorTarget, m Metrics, _ error) (Metrics, error) {
	return m, nil
}
func (f *fakeStore) RecordProbe(*models.MonitorTarget, bool, error) error { return nil }
func (f *fakeStore) UpdateRTSPURL(uint, string) error                     { return nil }
func (f *fakeStore) CreateAlert(a *models.MonitorAlert) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created++
	f.all = append(f.all, a)
	f.open[a.Metric] = a
	return nil
}
func (f *fakeStore) LastOpenAlert(_ uint, metric string) (*models.MonitorAlert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.open[metric], nil
}
func (f *fakeStore) ActiveSilenceUntil(targetID uint, metric string, now time.Time) (*time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *time.Time
	for _, a := range f.all {
		if a.TargetID != targetID || a.Metric != metric || a.SilencedUntil == nil {
			continue
		}
		if a.SilencedUntil.After(now) && (best == nil || a.SilencedUntil.After(*best)) {
			v := *a.SilencedUntil
			best = &v
		}
	}
	return best, nil
}

func (f *fakeStore) ResolveOpenAlerts(_ uint, metric string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolved = append(f.resolved, metric)
	delete(f.open, metric)
	return nil
}

func TestAlertStateTransitions(t *testing.T) {
	store := newFakeStore()
	notifCount := 0
	a := NewAlerter(store, Notifier(func(context.Context, string, string, string) { notifCount++ }), nil)
	tg := mkTarget("host", "", "", "")
	tg.Name = "srv1"
	tg.Thresholds = `{"cpu_usage":80}`

	high := Metrics{Online: true, CPUUsage: 92}
	a.Evaluate(ctxBG(), tg, high, nil)
	if store.created != 1 || notifCount != 1 {
		t.Fatalf("first trigger: created=%d notif=%d, want 1/1", store.created, notifCount)
	}
	// 仍超阈值：不重复
	a.Evaluate(ctxBG(), tg, high, nil)
	if store.created != 1 || notifCount != 1 {
		t.Errorf("still triggered: created=%d notif=%d, want 1/1", store.created, notifCount)
	}
	// 恢复：cpu_usage 被 resolve（每次 Evaluate 会 resolve 所有未触发指标，故看是否包含）
	a.Evaluate(ctxBG(), tg, Metrics{Online: true, CPUUsage: 30}, nil)
	if !contains(store.resolved, "cpu_usage") {
		t.Errorf("should resolve cpu_usage, got %v", store.resolved)
	}
	// 再次超阈值：又新建一条
	a.Evaluate(ctxBG(), tg, high, nil)
	if store.created != 2 {
		t.Errorf("re-trigger: created=%d, want 2", store.created)
	}
}

func TestAlertOffline(t *testing.T) {
	store := newFakeStore()
	called := false
	a := NewAlerter(store, Notifier(func(context.Context, string, string, string) { called = true }), nil)
	tg := mkTarget("nas", "nas1", "", "")
	tg.Name = "nas1"
	a.Evaluate(ctxBG(), tg, Metrics{Online: false}, context.DeadlineExceeded)
	if store.created != 1 || !called {
		t.Fatalf("offline: created=%d notif=%v, want 1/true", store.created, called)
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// TestSilenceSurvivesFlapping 用户点「静默 30 分钟」后，设备抖动（恢复→再触发）不该把静默丢掉。
// 静默记在具体告警行上，而再触发会新建一行：不继承过来，静默就只挡得住当前这一行——
// 抖动型告警（offline 反复）过了通知冷却期照样吵，新行在界面上也显示成「未静默」。
func TestSilenceSurvivesFlapping(t *testing.T) {
	store := newFakeStore()
	var notes []string
	alerter := NewAlerter(store, func(_ context.Context, title, body, level string) {
		notes = append(notes, title)
	}, nil)
	tgt := &models.MonitorTarget{ID: 7, Name: "edge-nas", Type: "nas"}
	broken := errors.New("connection refused")

	alerter.Evaluate(context.Background(), tgt, Metrics{}, broken)
	if len(notes) != 1 {
		t.Fatalf("首次触发应通知一次，got %v", notes)
	}
	opened, _ := store.LastOpenAlert(tgt.ID, "offline")
	if opened == nil {
		t.Fatal("用例前提失效：首条 offline 告警不在 open 表里")
	}
	until := time.Now().Add(30 * time.Minute)
	opened.SilencedUntil = &until

	// 恢复 → 再触发：新建的那一行必须带上还没到期的静默。
	// 恢复这一步直接走 store（等价于 Evaluate 的 resolve 分支，那条路径已有 TestAlertStateTransitions 覆盖），
	// 免得这里还要拼出「健康」的 Metrics 形状来触发同一段代码。
	_ = store.ResolveOpenAlerts(tgt.ID, "offline")
	alerter.Evaluate(context.Background(), tgt, Metrics{}, broken)
	if store.created != 2 {
		t.Fatalf("告警记录要留痕（审计完整），created=%d", store.created)
	}
	latest, _ := store.LastOpenAlert(tgt.ID, "offline")
	if latest == nil || latest.SilencedUntil == nil {
		t.Fatalf("新行必须继承未过期的静默，否则冷却期一过就照旧吵：%+v", latest)
	}
	if !latest.SilencedUntil.After(time.Now()) {
		t.Errorf("继承到的静默截止时间应在未来，got %v", *latest.SilencedUntil)
	}

	// 反证：静默已过期时不得凭空造静默，否则「静默」会永久化、告警再也不响。
	// 通知次数在这里不可断言（Alerter 有 5 分钟/设备的冷却），只断言建行与继承两件事。
	expired := time.Now().Add(-time.Minute)
	for _, row := range store.all {
		row.SilencedUntil = &expired
	}
	store.open = map[string]*models.MonitorAlert{}
	alerter.Evaluate(context.Background(), tgt, Metrics{}, broken)
	if store.created != 3 {
		t.Fatalf("过期静默不该阻止新告警建行，created=%d", store.created)
	}
	fresh, _ := store.LastOpenAlert(tgt.ID, "offline")
	if fresh == nil || fresh.SilencedUntil != nil {
		t.Errorf("过期静默不应被继承到新行：%+v", fresh)
	}
}

// TestAcknowledgeStopsEscalation 「确认」必须在提醒链上有效。
// 此前持续提醒只看「告警还开着 + 超过 alertEscalateAfter」，完全无视 AcknowledgedAt：
// 用户点了确认（表示我在处理），系统仍每过冷却期推一条「设备告警持续」，确认按钮等于没做。
func TestAcknowledgeStopsEscalation(t *testing.T) {
	store := newFakeStore()
	target := &models.MonitorTarget{ID: 11, Name: "core-host", Type: "host"}
	broken := errors.New("connection refused")
	// 每个场景各用一个新的 Alerter：通知冷却（5 分钟/设备）记在 Alerter 里，
	// 复用同一个会把「该不该发」的差异掩盖成「刚发过」。
	run := func() []string {
		var notes []string
		a := NewAlerter(store, func(_ context.Context, title, _, _ string) {
			notes = append(notes, title)
		}, nil)
		a.Evaluate(context.Background(), target, Metrics{}, broken)
		return notes
	}

	if got := run(); len(got) != 1 {
		t.Fatalf("首次触发应通知一次，got %v", got)
	}
	opened, _ := store.LastOpenAlert(target.ID, "offline")
	if opened == nil {
		t.Fatal("用例前提失效：offline 告警不在 open 表里")
	}
	opened.CreatedAt = time.Now().Add(-30 * time.Minute)

	// 未确认的老告警：持续提醒要发（证明这条路径本身是活的，而不是被别处挡掉）。
	if got := run(); len(got) != 1 {
		t.Fatalf("未确认的老告警应持续提醒一次，got %v", got)
	}

	// 确认之后：同一告警、同一状态，不该再推。
	ack := time.Now().Add(-time.Minute)
	opened.AcknowledgedAt = &ack
	if got := run(); len(got) != 0 {
		t.Fatalf("已确认的告警不该再推持续提醒，got %v", got)
	}

	// 反例：确认后恢复再触发是「新一次事件」（新建一行、没有确认状态），提醒必须回来，
	// 否则一次确认会变成整台设备的永久静音。
	_ = store.ResolveOpenAlerts(target.ID, "offline")
	if got := run(); len(got) != 1 {
		t.Fatalf("新一次触发应正常提醒，got %v", got)
	}
}
