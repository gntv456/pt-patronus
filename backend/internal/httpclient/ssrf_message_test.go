package httpclient

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// ErrBlocked 的文本自带 "ssrf:" 前缀，而 Do/PostWithCookie 从前又包一层
// fmt.Errorf("ssrf: %w") ⇒ 用户在勋章墙/站点面读到的是
// 「ssrf: ssrf: blocked private/internal address: 127.0.0.1」这种重复前缀的英文。
// 这里钉住：一次拦截只出现一个前缀，且哨兵判定仍然可用。
func TestSSRFBlockedMessageHasSinglePrefix(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:8080/x",
		"http://10.1.2.3/",
		"http://[::1]/",
	} {
		err := AssertPublicURL(raw)
		if err == nil {
			t.Fatalf("%q: 私有地址必须被拦", raw)
		}
		if !errors.Is(err, ErrBlocked) {
			t.Errorf("%q: 必须是确定的 ErrBlocked（供『绝不重试』与中文提示判定），got %v", raw, err)
		}
		if n := strings.Count(err.Error(), "ssrf:"); n != 1 {
			t.Errorf("%q: 前缀应只出现 1 次，实际 %d 次：%v", raw, n, err)
		}
	}

	// utls 拨号那侧同样是拼哨兵（从前它返回字面相似但不包哨兵的错误，
	// errors.Is 判不出来 ⇒ 确定的拒绝被当成临时故障重试）。
	joined := fmt.Errorf("%w: %s", ErrBlocked, "127.0.0.1")
	if !errors.Is(joined, ErrBlocked) {
		t.Errorf("拼接后的拦截错误必须仍可 errors.Is 判定，got %v", joined)
	}
	if n := strings.Count(joined.Error(), "ssrf:"); n != 1 {
		t.Errorf("拼接后前缀应只出现 1 次，实际 %d 次：%v", n, joined)
	}
}
