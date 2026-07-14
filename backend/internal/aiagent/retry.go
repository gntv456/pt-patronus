package aiagent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/llm"
)

// provider 重试参数：对可重试错误（429/5xx/网络瞬时）按指数退避重试，应对限流与上游抖动。
// 仅重试 ChatStream 的「立即错误」（建连/首响）；流内中途错误（EventError）不重试——
// 无法干净重放已部分下发的流，且中途错误多为上游内容问题而非可重试的传输问题。
const (
	defaultMaxRetries  = 3                          // 额外重试次数（首次 + 3 次 = 最多 4 次请求）
	defaultBaseBackoff = 800 * time.Millisecond    // 指数退避基数：800ms → 1.6s → 3.2s
)

// retryProvider 包装一个 provider，对可重试错误按指数退避重试。
type retryProvider struct {
	inner llm.Provider
	max   int           // 额外重试次数
	base  time.Duration // 退避基数
}

// withRetry 包装 provider 启用重试（默认参数）；nil 透传 nil。
func withRetry(p llm.Provider) llm.Provider {
	return withRetryCfg(p, defaultMaxRetries, defaultBaseBackoff)
}

// withRetryCfg 可配置重试参数（测试用小退避加速）。
func withRetryCfg(p llm.Provider, max int, base time.Duration) llm.Provider {
	if p == nil {
		return nil
	}
	return &retryProvider{inner: p, max: max, base: base}
}

func (p *retryProvider) ChatStream(ctx context.Context, req llm.Req) (<-chan llm.Event, error) {
	var lastErr error
	for attempt := 0; attempt <= p.max; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(p.base * time.Duration(1<<(attempt-1))): // base, 2×base, 4×base…
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		stream, err := p.inner.ChatStream(ctx, req)
		if err == nil {
			return stream, nil
		}
		lastErr = err
		// 客户端已取消或错误不可重试（4xx 非法请求、JSON 错误）→ 立即返回，不浪费退避时间。
		if ctx.Err() != nil || !isRetryableLLMError(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("LLM 调用重试 %d 次仍失败: %w", p.max, lastErr)
}

// isRetryableLLMError 判定是否值得重试：
//   - 429（限流）/ 5xx（上游临时故障）→ 重试
//   - 网络瞬时错误（client.Do 超时/连接失败等 net.Error）→ 重试
//   - 4xx 非法请求、JSON 序列化错误、ctx 取消/超时 → 不重试
func isRetryableLLMError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// provider 返回的 HTTP 状态错误："openai upstream HTTP 429" / "anthropic upstream HTTP 503"
	for _, pfx := range []string{"openai upstream HTTP ", "anthropic upstream HTTP "} {
		if strings.HasPrefix(err.Error(), pfx) {
			var status int
			if _, e := fmt.Sscanf(strings.TrimPrefix(err.Error(), pfx), "%d", &status); e == nil {
				return status == 429 || status >= 500
			}
		}
	}
	// 网络错误（url.Error 包裹的 net.OpError/timeout）→ 可重试
	var netErr net.Error
	return errors.As(err, &netErr)
}
