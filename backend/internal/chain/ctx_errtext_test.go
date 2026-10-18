package chain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"gorm.io/gorm"
)

// ctxErrText 是后宫/求种/公告等 11 个跨站聚合面共用的逐条兜底文案口。
// 从前它对非截断错误直接 err.Error()，于是药单上出现
// 「ssrf: blocked private/internal address: 127.0.0.1」这种英文原文（#112 的单一来源漏了这一处）。
func TestCtxErrTextUsesSingleSource(t *testing.T) {
	ctx := context.Background()

	if got := ctxErrText(ctx, fmt.Errorf("%w: 127.0.0.1", httpclient.ErrBlocked)); got != errtext.SSRFHint {
		t.Errorf("SSRF 拦截应翻成可操作中文，got %q", got)
	}
	if got := ctxErrText(ctx, gorm.ErrRecordNotFound); got != errtext.NotFound {
		t.Errorf("记录不存在应翻成中文，got %q", got)
	}
	// 站点自己写的中文原因照旧原样透出（不能被泛化文案顶掉）。
	siteMsg := errors.New("站点返回：魔力值不足")
	if got := ctxErrText(ctx, siteMsg); got != siteMsg.Error() {
		t.Errorf("站点原文不得被改写，got %q", got)
	}
	// 真·超时保持「稍后重试」语义，且不出现开发者术语。
	dead, cancel := context.WithCancel(ctx)
	cancel()
	if got := ctxErrText(dead, context.Canceled); !strings.Contains(got, "稍后重试") {
		t.Errorf("截断路径应给「稍后重试」，got %q", got)
	}
}
