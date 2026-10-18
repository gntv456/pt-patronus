package v1

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// 网关的流式失败只剩日志这一个说话的地方（响应已经流出去，状态码改不了）。
// 从前两处 `_ = llmgateway.Stream…` 把它整个丢掉 ⇒ 运维侧看不到「谁的哪一次请求失败了」，
// 只能听用户说「模型不回话」。这里钉两层：helper 真写日志 + 两个出口真在用它。

func TestLogStreamFailure(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	logStreamFailure("anthropic", 42, nil)
	if buf.Len() != 0 {
		t.Errorf("没有失败就不该留痕（否则正常请求刷屏），got %q", buf.String())
	}

	buf.Reset()
	logStreamFailure("openai", 7, errors.New("上游 429：配额已用尽"))
	line := buf.String()
	for _, want := range []string{"level=WARN", "api=openai", "conversation_id=7", "配额已用尽"} {
		if !strings.Contains(line, want) {
			t.Errorf("日志缺 %s，got %q", want, line)
		}
	}
}

// TestGatewayStreamOutletsKeepError 两个出口都必须把 Stream 的返回值交给日志，
// 不许再退回 `_ =`（负控实测：把其中一处改回 `_ =` 只有这条红，helper 用例仍绿）。
// 只扫真源码，先剥 // 注释——免得我自己写的说明文字造出假红（同 docker_local_list_test 的护栏）。
func TestGatewayStreamOutletsKeepError(t *testing.T) {
	raw, err := os.ReadFile("llm_gateway.go")
	if err != nil {
		t.Fatalf("读不到被测源文件: %v", err)
	}
	var code strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		code.WriteString(line)
		code.WriteByte('\n')
	}
	src := code.String()
	if n := strings.Count(src, "_ = llmgateway.Stream"); n != 0 {
		t.Errorf("仍有 %d 处丢弃网关流式错误（_ = llmgateway.Stream…）", n)
	}
	if n := strings.Count(src, "logStreamFailure("); n != 3 { // 定义 1 + 两处出口调用
		t.Errorf("logStreamFailure 出现 %d 次，应为 3（定义 1 + 调用 2）", n)
	}
}
