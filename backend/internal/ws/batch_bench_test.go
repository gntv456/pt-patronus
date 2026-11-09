package ws

import (
	"sort"
	"testing"
	"time"
)

// BenchmarkBroadcastFlush 测「一次 tick 合并广播」的 P99 耗时（ws_broadcast_p99_ms 口径）。
// 构造 200 个连接、每次合并 10 条消息推给全部连接——纯内存 channel 投递，不含网络。
// 对应 docs/perf-baseline.md 的 ws-burst 场景：主线程长任务 + 后端广播 P99 < 5ms。
func BenchmarkBroadcastFlush(b *testing.B) {
	const clients = 200
	const msgs = 10
	h := NewHub()
	for i := 0; i < clients; i++ {
		h.register(&Client{send: make(chan []byte, 64)})
	}
	payloads := make([][]byte, msgs)
	for i := range payloads {
		payloads[i] = []byte("payload-0123456789abcdef")
	}
	samples := make([]int64, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch := NewBatch(h)
		for _, p := range payloads {
			batch.Add(p)
		}
		start := time.Now()
		batch.Flush()
		samples = append(samples, time.Since(start).Nanoseconds())
	}
	b.StopTimer()
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	idx := int(float64(len(samples))*0.99) - 1
	if idx < 0 {
		idx = 0
	}
	b.ReportMetric(float64(samples[idx])/1e6, "p99-ms")
}
