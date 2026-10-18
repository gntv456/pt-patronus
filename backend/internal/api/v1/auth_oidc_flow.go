package v1

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/auth"
)

// ===== OIDC / SSO（Authorization Code + PKCE） =====

// oidcRedirectURI 计算回调地址（本后端的 /api/v1/auth/oidc/callback）。
// 优先用请求的对外基址（X-Forwarded-Proto/Host），回退 Request.Host。
func oidcRedirectURI(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/v1/auth/oidc/callback", scheme, c.Request.Host)
}

// OIDCStart 发起 OIDC 授权：写 state/verifier cookie 并 302 跳转到 Provider 授权端点。
func (h *AuthHandler) OIDCStart(c *gin.Context) {
	if h.oidc == nil {
		h.oidcFail(c, errcode.OIDCNotConfigured)
		return
	}
	flow, err := h.oidc.StartFlow(c.Request.Context(), oidcRedirectURI(c))
	if err != nil {
		audit(c, "oidc_start", "fail", slog.String("err", err.Error()))
		// 这个 GET 没有任何入参（redirect_uri 由本端自己算）：此前一律 InvalidInput，
		// 用户看到「请求参数有误」却没有参数可核，真实处境是「SSO 没启用」或「连不上 Provider」。
		if errors.Is(err, auth.ErrOIDCUnconfigured) {
			h.oidcFail(c, errcode.OIDCDisabled.WithErr(err))
			return
		}
		h.oidcFail(c, errcode.OIDCProviderUnreachable.WithErr(err))
		return
	}
	http.SetCookie(c.Writer, &flow.StateCookie)
	http.SetCookie(c.Writer, &flow.VerifierCookie)
	c.Redirect(http.StatusFound, flow.AuthorizeURL)
}

// oidcFail 是 SSO 各失败出口的唯一出路。
// 两个 SSO 端点都由浏览器整页导航进入（登录按钮 window.location.href、Provider 的 302 回调），
// 直接 errcode.Respond 会让用户看到一屏 {"code":"oidc_state_mismatch",...} 的 JSON ——
// 真实文案在信封里，但没人能读。认 Accept: text/html 的顶层导航，把错误码放进 fragment
// 交回登录页由界面翻成人话；API 客户端（curl / 测试 / 移动端）不带该头，仍拿信封。
func (h *AuthHandler) oidcFail(c *gin.Context, e *errcode.Error) {
	if strings.Contains(c.GetHeader("Accept"), "text/html") {
		c.Redirect(http.StatusFound, h.oidcFrontendURL("oidc_error", e.Code))
		return
	}
	errcode.Respond(c, e)
}

// oidcFrontendURL 拼回前端登录页的 fragment 地址（成功带 oidc_token，失败带 oidc_error）。
// 回跳目标解析只留这一份：原先散在成功路径里，失败路径要复用时抄第二遍必漂移。
func (h *AuthHandler) oidcFrontendURL(key, value string) string {
	target := "/login"
	if h.oidc != nil {
		if t := strings.TrimSpace(h.oidc.LoadSettings().FrontendRedirect); t != "" {
			target = t
		}
	}
	sep := "#"
	if strings.Contains(target, "#") {
		sep = "&"
	}
	return target + sep + key + "=" + url.QueryEscape(value)
}

// OIDCCallback 处理 Provider 回调：校验 state + 换令牌 + 取 userinfo + 签发本地 JWT + 302 回前端。
func (h *AuthHandler) OIDCCallback(c *gin.Context) {
	if h.oidc == nil {
		h.oidcFail(c, errcode.OIDCNotConfigured)
		return
	}
	stateCookie, err := c.Cookie("ptp_oidc_state")
	verifier, _ := c.Cookie("ptp_oidc_verifier")
	code := c.Query("code")
	state := c.Query("state")
	if err != nil || code == "" || state == "" || state != stateCookie {
		audit(c, "oidc_callback", "fail", slog.String("reason", "state_mismatch"))
		h.oidcFail(c, errcode.OIDCStateMismatch)
		return
	}
	ui, err := h.oidc.CompleteFlow(c.Request.Context(), code, state, verifier, oidcRedirectURI(c))
	// 清掉一次性 cookie。
	clearOIDCCookie(c, "ptp_oidc_state")
	clearOIDCCookie(c, "ptp_oidc_verifier")
	if err != nil {
		audit(c, "oidc_callback", "fail", slog.String("err", err.Error()))
		// 「未启用/没填全」与「换票失败」分开：前者是配置问题（回设置页），
		// 后者是 Provider/凭据问题（核对 client_secret 与 issuer），都不是上游一闪而过。
		if errors.Is(err, auth.ErrOIDCUnconfigured) {
			h.oidcFail(c, errcode.OIDCDisabled.WithErr(err))
			return
		}
		h.oidcFail(c, errcode.OIDCLoginFailed.WithErr(err))
		return
	}
	// 映射用户名：preferred_username → name → email 前缀 → sub。
	username := ui.Username
	if username == "" {
		username = ui.Name
	}
	if username == "" && ui.Email != "" {
		username = strings.SplitN(ui.Email, "@", 2)[0]
	}
	if username == "" {
		username = ui.Sub
	}
	token, err := h.issueForOIDC("oidc:"+ui.Sub, username)
	if err != nil {
		// 单用户限制拒绝与签发失败分开呈现，两条都交回登录页说人话（浏览器导航看不到信封）。
		if errors.Is(err, errcode.AuthUserRejected) {
			audit(c, "oidc_callback", "reject", slog.String("sub", ui.Sub), slog.String("reason", "single_user_limit"))
			h.oidcFail(c, errcode.AuthUserRejected)
			return
		}
		audit(c, "oidc_callback", "fail", slog.String("sub", ui.Sub), slog.String("err", err.Error()))
		h.oidcFail(c, errcode.AuthTokenFailed.WithErr(err))
		return
	}
	audit(c, "oidc_callback", "ok", slog.String("sub", ui.Sub), slog.String("user", username))
	// 302 回前端：用 fragment(#oidc_token=) 传递短 TTL JWT。fragment 不进服务器访问日志、
	// 不随 Referrer 头外泄、不留在浏览器历史的 query 串里，比 query 更安全（仍建议生产走 HTTPS）。
	c.Redirect(http.StatusFound, h.oidcFrontendURL("oidc_token", token))
}

func clearOIDCCookie(c *gin.Context, name string) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1})
}
