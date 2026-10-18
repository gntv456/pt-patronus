package v1

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// TestRespondFetchErrorClassifiesSSRF 站点建在内网/环回时被 SSRF 守卫拦下，必须报
// 400 + 可操作的下一步（换地址或开 ALLOW_PRIVATE_SITES），不能是「服务内部错误，请
// 稍后重试」或「上游不可用」——重试和查网络都不会有用。同时钉住「其它错误仍按各自
// 兜底码返回」，别把真故障也洗成 400。
func TestRespondFetchErrorClassifiesSSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name     string
		err      error
		fallback *errcode.Error
		wantCode int
		wantBody string
	}{
		{"裸 sentinel", httpclient.ErrBlocked, errcode.Internal, http.StatusBadRequest, "ssrf_blocked"},
		{"包装过", fmt.Errorf("fetch: %w", httpclient.ErrBlocked), errcode.BadGateway, http.StatusBadRequest, "ssrf_blocked"},
		{"双层包装", fmt.Errorf("site: %w", fmt.Errorf("fetch: %w", httpclient.ErrBlocked)), errcode.BadGateway, http.StatusBadRequest, "ssrf_blocked"},
		{"超时不改判", context.DeadlineExceeded, errcode.BadGateway, http.StatusBadGateway, "bad_gateway"},
		{"读库失败不改判", errcode.Internal, errcode.Internal, http.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		respondFetchError(c, tc.err, tc.fallback)
		if w.Code != tc.wantCode {
			t.Fatalf("%s: code=%d want %d body=%s", tc.name, w.Code, tc.wantCode, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), tc.wantBody) {
			t.Fatalf("%s: 应含 %s，got %s", tc.name, tc.wantBody, w.Body.String())
		}
		if tc.wantBody == "ssrf_blocked" && !strings.Contains(w.Body.String(), "ALLOW_PRIVATE_SITES") {
			t.Fatalf("%s: 拦截文案要给出可操作的开关，got %s", tc.name, w.Body.String())
		}
	}
}
