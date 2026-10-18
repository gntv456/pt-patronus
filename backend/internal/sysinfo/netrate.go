package sysinfo

import (
	"sync"
	"time"
)

// 本机网络速率：gopsutil 只给累计字节，而运维中心「网络 ↓/↑ B/s」要的是速率。
// 从前 /api/v1/system/metrics 从不填 rate 字段（`json:"...,omitempty"` 让它连 0 都不出现），
// 前端 `metrics.net_rx_rate ?? 0` 兜底 ⇒ 卡片永远显示 0 B/s，
// 「机器空闲」和「压根没人算过」在界面上长得一模一样。
//
// 这里按「与上次采样的差值」补上速率：本包只服务本机一条采集链（运维中心轮询、
// local 采集器共用），单进程内一份基线即可；agent 上报路径同样受益（服务端
// RecordSnapshot 仍会按快照基线覆写，两处口径一致）。
var (
	netBaseMu sync.Mutex
	netBase   netBaseline
)

type netBaseline struct {
	rx, tx uint64
	at     time.Time
}

// fillNetRates 用上次采样的累计字节求平均速率并写回 m；同时把本次采样记成新基线。
// 无基线（进程刚起）、间隔过短、或计数器回绕/重启清零时不填——宁缺勿假。
func fillNetRates(m *Metrics, now time.Time) {
	netBaseMu.Lock()
	defer netBaseMu.Unlock()

	prev := netBase
	netBase = netBaseline{rx: m.NetRxBytes, tx: m.NetTxBytes, at: now}

	if prev.at.IsZero() {
		return
	}
	dt := now.Sub(prev.at).Seconds()
	if rx, ok := netRate(prev.rx, m.NetRxBytes, dt); ok {
		m.NetRxRate = rx
	}
	if tx, ok := netRate(prev.tx, m.NetTxBytes, dt); ok {
		m.NetTxRate = tx
	}
}

// netRate 两次累计字节求速率；间隔 <0.5s 或计数器变小（重启/回绕）判不出可信值。
func netRate(prevBytes, curBytes uint64, dt float64) (uint64, bool) {
	if dt < 0.5 || curBytes < prevBytes {
		return 0, false
	}
	return uint64(float64(curBytes-prevBytes) / dt), true
}
