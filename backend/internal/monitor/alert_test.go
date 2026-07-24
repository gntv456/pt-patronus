package monitor

import (
	"context"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// fakeStore 仅记录告警创建/恢复/通知，用于验证状态转换去重逻辑。
type fakeStore struct {
	mu       sync.Mutex
	open     map[string]*models.MonitorAlert
	created  int
	resolved []string
}

func newFakeStore() *fakeStore { return &fakeStore{open: map[string]*models.MonitorAlert{}} }

func (f *fakeStore) ListEnabled() ([]models.MonitorTarget, error)                  { return nil, nil }
func (f *fakeStore) Get(uint) (*models.MonitorTarget, error)                       { return nil, nil }
func (f *fakeStore) Credentials(*models.MonitorTarget) (Credentials, error)        { return Credentials{}, nil }
func (f *fakeStore) RecordSnapshot(_ *models.MonitorTarget, m Metrics, _ error) (Metrics, error) {
	return m, nil
}
func (f *fakeStore) UpdateRTSPURL(uint, string) error                              { return nil }
func (f *fakeStore) CreateAlert(a *models.MonitorAlert) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created++
	f.open[a.Metric] = a
	return nil
}
func (f *fakeStore) LastOpenAlert(_ uint, metric string) (*models.MonitorAlert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.open[metric], nil
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
