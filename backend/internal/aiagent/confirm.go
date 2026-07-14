package aiagent

import (
	"sync"
	"time"
)

// confirmTimeout 危险工具等待用户确认的最长时间；超时视为拒绝（不执行）。
const confirmTimeout = 180 * time.Second //nolint:gochecknoglobals // 与本包其它超时常量同级

// confirmRegistry 管理 pending 的危险工具确认。
//
// 每个 pending 确认对应一个 size-1 响应 channel：true=批准，false=拒绝。
// 设计要点（并发安全）：
//   - 只有 register 创建 channel，只有 resolve 向其发送（size-1 buffer，非阻塞）。
//   - 全程不 close channel → 不存在「close 后再 send」panic。
//   - resolve/cancel 都在锁内先 delete 再操作，确保同一 token 只有一方胜出：
//     已被 resolve 的 token，cancel 找不到、no-op；反之亦然。
//   - 等待方持有 register 返回的 channel 引用，靠 select 的 ctx/timeout 分支退出，
//     不依赖 registry 是否仍持有该 entry。
//
// token 由调用方保证唯一（convID:toolCallID）；同 conv 单线程不并发，天然无碰撞。
type confirmRegistry struct {
	mu      sync.Mutex
	pending map[string]chan bool
}

func newConfirmRegistry() *confirmRegistry {
	return &confirmRegistry{pending: make(map[string]chan bool)}
}

// register 登记一个 pending 确认并返回其响应 channel（size 1）。
// 若 token 已存在（理论不会发生），返回 nil——调用方据此直接判否，避免覆盖在途确认。
func (r *confirmRegistry) register(token string) chan bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.pending[token]; exists {
		return nil
	}
	ch := make(chan bool, 1)
	r.pending[token] = ch
	return ch
}

// resolve 送达用户决断。返回是否有 pending 确认被命中（false=已过期/已取消/未知）。
func (r *confirmRegistry) resolve(token string, approved bool) bool {
	r.mu.Lock()
	ch, ok := r.pending[token]
	if ok {
		delete(r.pending, token)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	ch <- approved // size-1 buffer，非阻塞
	return true
}

// cancel 在等待方退出（ctx 取消/超时）后移除 pending 条目做 housekeeping；
// 已被 resolve 的 token 找不到、no-op。不向 channel 发值。
func (r *confirmRegistry) cancel(token string) {
	r.mu.Lock()
	delete(r.pending, token)
	r.mu.Unlock()
}
