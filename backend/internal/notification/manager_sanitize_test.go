package notification

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/errtext"
)

// 逐通道错误会原样进「测试通知」结果表：传输层失败必须是 errtext 的中文口径，
// 同时 URL 里的 bot token/密钥绝不能出现在结果里。
func TestSanitizeNotifyErrTranslatesTransport(t *testing.T) {
	dial := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connectex: No connection could be made")}
	if got := sanitizeNotifyErr(dial); got != errtext.Unreachable {
		t.Fatalf("裸拨号失败应翻成中文可操作提示, got %q", got)
	}

	ue := &url.Error{Op: "Post", URL: "https://api.telegram.org/botSECRET/sendMessage", Err: dial}
	got := sanitizeNotifyErr(ue)
	if strings.Contains(got, "SECRET") {
		t.Fatalf("URL 里的 token 泄漏到结果里: %q", got)
	}
	if !strings.Contains(got, "api.telegram.org") || !strings.Contains(got, "连不上") {
		t.Fatalf("应保留 host 且用中文原因, got %q", got)
	}

	// 被 fmt.Errorf 包过一层的 *url.Error 也要认出来（errors.As）。
	wrapped := fmt.Errorf("推送失败: %w", ue)
	if got := sanitizeNotifyErr(wrapped); strings.Contains(got, "SECRET") || !strings.Contains(got, "连不上") {
		t.Fatalf("包装后的 url.Error 处理不对: %q", got)
	}

	// 上游自产文案（Telegram 业务失败等）原样透传，不二次加工。
	if got := sanitizeNotifyErr(errors.New("telegram: Unauthorized")); got != "telegram: Unauthorized" {
		t.Fatalf("未识别的上游文案应透传, got %q", got)
	}
	if got := sanitizeNotifyErr(nil); got != "" {
		t.Fatalf("nil 应回空串, got %q", got)
	}
}
