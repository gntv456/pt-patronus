// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// 手动探活从前只把结果放进一条 3 秒就消失的 toast：设备没开轮询时（enabled=false，
// 采集循环永远不做）卡片一直显示「未检测」，这一次探测在库里留不下任何痕迹
// （last_online / last_error / last_checked_at 全空），告警面也无从参考。
// 这里钉住：ProbeNow 必须落状态账，但落的是「探活状态」而不是度量快照。

type recordingStore struct {
	fakeAgentStore
	lastOnline  bool
	lastErr     error
	probes      int
	snapRecords int
	alerts      int
}

func (r *recordingStore) RecordSnapshot(_ *models.MonitorTarget, m Metrics, err error) (Metrics, error) {
	r.snapRecords++
	r.lastErr = err
	return m, nil
}

func (r *recordingStore) RecordProbe(_ *models.MonitorTarget, online bool, err error) error {
	r.probes++
	r.lastOnline = online
	r.lastErr = err
	return nil
}

func (r *recordingStore) CreateAlert(*models.MonitorAlert) error {
	r.alerts++
	return nil
}

type probeCollector struct {
	online bool
	err    error
}

func (p probeCollector) String() string { return "probe-collector" }
func (p probeCollector) Collect(context.Context, *models.MonitorTarget, Credentials) (Metrics, error) {
	return Metrics{}, p.err
}
func (p probeCollector) Probe(context.Context, *models.MonitorTarget, Credentials) (bool, error) {
	return p.online, p.err
}

func TestProbeNowPersistsResult(t *testing.T) {
	refused := errors.New("dial tcp 127.0.0.1:22: connectex: No connection could be made")
	store := &recordingStore{}
	store.targets = []models.MonitorTarget{{ID: 1, Name: "smoke-host", Type: "host", Protocol: "ssh"}}
	reg := NewRegistry()
	reg.Register("host", "ssh", probeCollector{online: false, err: refused})
	svc := NewService(store, reg, noopNotifier, time.Second, nil)

	online, err := svc.ProbeNow(context.Background(), 1)
	if online || !errors.Is(err, refused) {
		t.Fatalf("探活结果要原样回给调用方，got online=%v err=%v", online, err)
	}
	if store.probes != 1 {
		t.Fatalf("手动探活必须落一次状态账（RecordProbe），got %d 次", store.probes)
	}
	if store.lastOnline {
		t.Errorf("落库的在线位要跟着探测结果，got true")
	}
	if !errors.Is(store.lastErr, refused) {
		t.Errorf("落库要带上失败原因（供 last_error 与详情面板显示），got %v", store.lastErr)
	}
	// 手动点一下不该越级生成告警（评估归轮询），否则用户点一次就攒一条。
	if store.alerts != 0 {
		t.Errorf("手动探活不应直接建告警，got %d 条", store.alerts)
	}
}

// 探活不是度量：不能冒充一条快照。Online=true 而 CPU/内存/累计字节全空的行落进
// MetricSnapshot 后，既是界面上凭空的 0% 读数，又成了 computeNetRate 的上一条基准
// （prev 累计字节变成 0 → 下次采集把开机以来的全部流量除以几秒，网速爆表）。
func TestProbeNowDoesNotWriteMetricSnapshot(t *testing.T) {
	store := &recordingStore{}
	store.targets = []models.MonitorTarget{{ID: 3, Name: "nas", Type: "host", Protocol: "ssh"}}
	reg := NewRegistry()
	reg.Register("host", "ssh", probeCollector{online: true})
	svc := NewService(store, reg, noopNotifier, time.Second, nil)

	if online, err := svc.ProbeNow(context.Background(), 3); !online || err != nil {
		t.Fatalf("探活成功路径应回 online=true：%v %v", online, err)
	}
	if store.probes != 1 {
		t.Fatalf("探活状态没落账，got %d 次", store.probes)
	}
	if store.snapRecords != 0 {
		t.Errorf("探活不该写度量快照（会造出 0%% 读数并顶掉网速基准），got %d 条", store.snapRecords)
	}
}

// 手动探活是 targetLocks 写者清单里的第四位（前三个：pull tick / CollectNow / agent push）。
// 不加锁时它能与到点的 tick 同时读写同一目标的最后状态与同一条 prev 基准。
func TestProbeNowSerializesWithCollection(t *testing.T) {
	store := &recordingStore{}
	store.targets = []models.MonitorTarget{{ID: 7, Name: "nas", Type: "host", Protocol: "ssh"}}
	reg := NewRegistry()
	reg.Register("host", "ssh", probeCollector{online: true})
	svc := NewService(store, reg, noopNotifier, time.Second, nil)

	unlock := lockTarget(7) // 模拟轮询正握着这台设备的采集
	done := make(chan struct{})
	go func() {
		svc.ProbeNow(context.Background(), 7) //nolint:errcheck // 本例只关心它是否被串行化
		close(done)
	}()
	select {
	case <-done:
		unlock()
		t.Fatal("轮询持锁期间手动探活已经完成——两写者未串行")
	case <-time.After(150 * time.Millisecond):
	}
	unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("释放锁后探活应完成")
	}
	if store.probes != 1 {
		t.Errorf("串行化后仍应只落一次状态账，got %d", store.probes)
	}
}

// agent 型设备的状态由它自己 push 上来，手动探活只回读现状、不该伪造一次快照。
func TestProbeNowAgentTargetDoesNotRecord(t *testing.T) {
	store := &recordingStore{}
	store.targets = []models.MonitorTarget{{ID: 2, Name: "nas-agent", Type: "host", Protocol: "agent"}}
	svc := NewService(store, NewRegistry(), noopNotifier, time.Second, nil)

	if _, err := svc.ProbeNow(context.Background(), 2); err != nil {
		t.Fatalf("agent target 探活应直接回读现状：%v", err)
	}
	if store.probes != 0 || store.snapRecords != 0 {
		t.Errorf("agent target 不该被 pull 侧落账，got probe=%d snapshot=%d", store.probes, store.snapRecords)
	}
}
