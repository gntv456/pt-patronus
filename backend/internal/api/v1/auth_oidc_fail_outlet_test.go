package v1

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/auth"
)

// memStore 是 auth.SettingsStore 的内存替身（SystemConfigOper 的最小替身）。
type memStore struct{ m map[string]string }

// errKeyMissing = 「这个 key 没存过」。
var errKeyMissing = errors.New("no such key")

func (s memStore) Get(k string) (string, error) {
	v, ok := s.m[k]
	if !ok {
		return "", errKeyMissing // 任意非 nil error = 「没有这条配置」
	}
	return v, nil
}

func (s memStore) Set(k, v string) error { s.m[k] = v; return nil }

func newOIDCTestHandler(t *testing.T, settings *auth.OIDCSettings) *AuthHandler {
	t.Helper()
	store := memStore{m: map[string]string{}}
	svc := auth.NewOIDCService(store)
	if settings != nil {
		if err := svc.SaveSettings(*settings); err != nil {
			t.Fatalf("保存 OIDC 配置失败: %v", err)
		}
	}
	return &AuthHandler{oidc: svc}
}

func oidcTestContext(accept string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/start", nil)
	c.Request.Host = "pt.example.com"
	if accept != "" {
		c.Request.Header.Set("Accept", accept)
	}
	return c, rec
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("响应体不是 JSON 信封: %v body=%q", err, rec.Body.String())
	}
	return m
}

// 修复前的形状（负控要能把它演回来）：StartFlow 失败一律 InvalidInput，
// 浏览器整页导航拿到 JSON。
func TestOIDCStartFailureIsNotParameterError(t *testing.T) {
	cases := []struct {
		name     string
		settings *auth.OIDCSettings
		wantCode string
		wantHTTP int
	}{
		{"未启用（配置没打开）", &auth.OIDCSettings{Enabled: false, Issuer: "https://auth.example.com", ClientID: "cid"}, "oidc_disabled", http.StatusServiceUnavailable},
		{"issuer 空", &auth.OIDCSettings{Enabled: true, Issuer: "", ClientID: "cid"}, "oidc_disabled", http.StatusServiceUnavailable},
		{"连不上 provider", &auth.OIDCSettings{Enabled: true, Issuer: "http://127.0.0.1:1", ClientID: "cid"}, "oidc_provider_unreachable", http.StatusBadGateway},
		{"discovery 返回非 JSON", &auth.OIDCSettings{Enabled: true, Issuer: "http://127.0.0.1:" + closedPort(t), ClientID: "cid"}, "oidc_provider_unreachable", http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newOIDCTestHandler(t, tc.settings)
			c, rec := oidcTestContext("application/json")
			h.OIDCStart(c)
			if rec.Code != tc.wantHTTP {
				t.Fatalf("状态码 = %d，期望 %d（body=%s）", rec.Code, tc.wantHTTP, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if env["code"] != tc.wantCode {
				t.Fatalf("错误码 = %v，期望 %s", env["code"], tc.wantCode)
			}
			msg, _ := env["message"].(string)
			if msg == "" {
				t.Fatalf("message 为空：用户读不到任何原因")
			}
			for _, bad := range []string{"参数有误", "请求参数"} {
				if strings.Contains(msg, bad) {
					t.Fatalf("message 仍把配置/网络问题说成用户传错参数: %q", msg)
				}
			}
		})
	}
}

// 浏览器整页导航（Accept: text/html）不得拿到 JSON：必须 302 回登录页带错误码。
func TestOIDCBrowserNavigationGetsRedirectNotJSON(t *testing.T) {
	cases := []struct{ redirect, want string }{
		// 目标只有 query 时仍用 # 起 fragment（route.hash 读得到）。
		{"/login?tab=sso", "/login?tab=sso#oidc_error=oidc_provider_unreachable"},
		// 目标自带 fragment 时必须 & 续接，否则第二个 # 会被当成 fragment 的一部分丢掉。
		{"/login#sso", "/login#sso&oidc_error=oidc_provider_unreachable"},
	}
	for _, tc := range cases {
		h := newOIDCTestHandler(t, &auth.OIDCSettings{
			Enabled: true, Issuer: "http://127.0.0.1:1", ClientID: "cid", FrontendRedirect: tc.redirect,
		})
		c, rec := oidcTestContext("text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		h.OIDCStart(c)
		if rec.Code != http.StatusFound {
			t.Fatalf("浏览器导航应被 302 送回登录页，实际 %d body=%q", rec.Code, rec.Body.String())
		}
		if loc := rec.Header().Get("Location"); loc != tc.want {
			t.Fatalf("回跳地址 = %q，期望 %q", loc, tc.want)
		}
	}
}

// 未装配（h.oidc == nil）也走同一条出路：界面拿得到码，客户端拿得到信封。
func TestOIDCNotAssembledBothOutlets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{}

	c, rec := oidcTestContext("application/json")
	h.OIDCStart(c)
	if rec.Code != http.StatusServiceUnavailable || decodeEnvelope(t, rec)["code"] != "oidc_not_configured" {
		t.Fatalf("start 未装配出口 = %d %s", rec.Code, rec.Body.String())
	}

	c2, rec2 := oidcTestContext("text/html")
	c2.Request.URL.Path = "/api/v1/auth/oidc/callback"
	h.OIDCCallback(c2)
	if rec2.Code != http.StatusFound || rec2.Header().Get("Location") != "/login#oidc_error=oidc_not_configured" {
		t.Fatalf("callback 未装配出口 = %d Location=%q", rec2.Code, rec2.Header().Get("Location"))
	}
}

// 回调侧：state 校验失败与换票失败都不是一闪而过的「上游不可用」，且浏览器不接 JSON。
func TestOIDCCallbackFailuresAreRedirectedWithCode(t *testing.T) {
	h := newOIDCTestHandler(t, &auth.OIDCSettings{Enabled: true, Issuer: "http://127.0.0.1:1", ClientID: "cid"})

	// state 不一致（浏览器最常见的「回来太晚 / cookie 没带上」）
	c, rec := oidcTestContext("text/html")
	c.Request.URL.Path = "/api/v1/auth/oidc/callback"
	c.Request.URL.RawQuery = "code=abc&state=s1"
	c.Request.AddCookie(&http.Cookie{Name: "ptp_oidc_state", Value: "other"})
	h.OIDCCallback(c)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login#oidc_error=oidc_state_mismatch" {
		t.Fatalf("state 失败出口 = %d Location=%q body=%q", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}

	// state 一致，但换票/discovery 失败 → oidc_login_failed（不再是通用「上游服务不可用」）
	c2, rec2 := oidcTestContext("application/json")
	c2.Request.URL.Path = "/api/v1/auth/oidc/callback"
	c2.Request.URL.RawQuery = "code=abc&state=s1"
	c2.Request.AddCookie(&http.Cookie{Name: "ptp_oidc_state", Value: "s1"})
	h.OIDCCallback(c2)
	if rec2.Code != http.StatusBadGateway {
		t.Fatalf("换票失败状态码 = %d，期望 502 body=%s", rec2.Code, rec2.Body.String())
	}
	env := decodeEnvelope(t, rec2)
	if env["code"] != "oidc_login_failed" {
		t.Fatalf("错误码 = %v，期望 oidc_login_failed", env["code"])
	}
	if msg, _ := env["message"].(string); !strings.Contains(msg, "client_secret") {
		t.Fatalf("message 没给出下一步（该核对 client_secret/issuer）: %q", msg)
	}
}

// closedPort 取一个确实没人监听的端口，避免依赖 1 号端口的环境差异。
func closedPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("取空闲端口失败: %v", err)
	}
	defer ln.Close()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}
