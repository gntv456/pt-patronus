package sysinfo

import (
	"testing"
	"time"
)

// 运维中心的「网络 ↓/↑ B/s」从前恒为 0：/system/metrics 只报累计字节，
// rate 字段带 omitempty 干脆不出现，前端 ?? 0 兜底 => 空闲与「没人算过」看不出区别。
func TestNetRatePure(t *testing.T) {
	cases := []struct {
		name      string
		prev, cur uint64
		dt        float64
		want      uint64
		wantOk    bool
	}{
		{"正常差值", 1000, 5000, 2, 2000, true},
		{"间隔过短不可信", 1000, 5000, 0.2, 0, false},
		{"计数器回绕或重启清零", 5000, 1000, 2, 0, false},
		{"零差值（真空闲）要报 0 而不是不填", 5000, 5000, 2, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := netRate(tc.prev, tc.cur, tc.dt)
			if ok != tc.wantOk || got != tc.want {
				t.Fatalf("netRate(%d,%d,%.1f) = (%d,%v), want (%d,%v)", tc.prev, tc.cur, tc.dt, got, ok, tc.want, tc.wantOk)
			}
		})
	}
}

// 接线证据：第一趟没有基线 ⇒ 不填；第二趟相对伪造的基线必须算出速率。
// （只测纯函数会漏掉「CollectSystemMetrics 到底调没调它」这一层。）
func TestCollectSystemMetricsFillsNetRates(t *testing.T) {
	resetBaseline(t)
	first := CollectSystemMetrics()
	if first.NetRxBytes == 0 && first.NetTxBytes == 0 {
		t.Skip("本机取不到网络累计字节，无法验证速率")
	}
	if first.NetRxRate != 0 || first.NetTxRate != 0 {
		t.Errorf("进程刚起没有基线，不该凭空报速率，got rx=%d tx=%d", first.NetRxRate, first.NetTxRate)
	}

	if first.NetRxBytes < 8_000_000 || first.NetTxBytes < 4_000_000 {
		t.Skipf("累计字节太小，减去伪造增量会下溢（rx=%d tx=%d）", first.NetRxBytes, first.NetTxBytes)
	}
	// 伪造「2 秒前收过 8MB、发出 4MB」的基线（真实计数器只会更大）。
	// 差值故意给大：CollectSystemMetrics 自身要跑 1~3s，dt 越大速率下限越稳。
	netBaseMu.Lock()
	netBase = netBaseline{
		rx: first.NetRxBytes - 8_000_000,
		tx: first.NetTxBytes - 4_000_000,
		at: time.Now().Add(-2 * time.Second),
	}
	netBaseMu.Unlock()

	second := CollectSystemMetrics()
	if second.NetRxRate == 0 {
		t.Errorf("第二次采样必须报出接收速率（运维中心的 B/s 就靠它），got 0")
	}
	// 8MB 摊在 ≥2s 上：下限按 1MB/s 卡，上限防 dt 算错（比如拿零基线除出天量）。
	if second.NetRxRate < 1_000_000 || second.NetRxRate > 1_000_000_000 {
		t.Errorf("接收速率应落在可信区间，got %d B/s", second.NetRxRate)
	}
	if second.NetTxRate < 500_000 || second.NetTxRate > 1_000_000_000 {
		t.Errorf("发送速率应落在可信区间，got %d B/s", second.NetTxRate)
	}
}

func resetBaseline(t *testing.T) {
	t.Helper()
	netBaseMu.Lock()
	netBase = netBaseline{}
	netBaseMu.Unlock()
	t.Cleanup(func() {
		netBaseMu.Lock()
		netBase = netBaseline{}
		netBaseMu.Unlock()
	})
}
