// Package auth → oidc.go：通用 OIDC（Authorization Code + PKCE）SSO，
// 兼容 Authelia / Keycloak / Casdoor 等任何标准 OIDC Provider。
// 配置落 SystemConfig（auth.oidc）；令牌交换/userinfo 走标准 discovery。
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// oidcSettingsKey 是 SystemConfig 中存储 OIDC 配置的 key（JSON）。
const oidcSettingsKey = "auth.oidc"

// SettingsStore 抽象 SystemConfigOper 的 Get/Set（避 auth→oper→monitor→auth 循环，
// 同 notice.Store 模式）。oper.SystemConfigOper 隐式实现本接口。
type SettingsStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// OIDCSettings 是管理员配置的 OIDC Provider 凭据与开关。
type OIDCSettings struct {
	Enabled          bool   `json:"enabled"`
	Issuer           string `json:"issuer"` // 如 https://auth.example.com（不含 /.well-known）
	ClientID         string `json:"client_id"`
	ClientSecret     string `json:"client_secret"`
	Scopes           string `json:"scopes"`            // 默认 "openid profile email"
	DisplayName      string `json:"display_name"`      // 登录按钮文案，默认 "OIDC / SSO"
	FrontendRedirect string `json:"frontend_redirect"` // 授权成功后把浏览器送去的前端路径，默认 /login
}

// defaults 补默认。
func defaultOIDCSettings() OIDCSettings {
	return OIDCSettings{Scopes: "openid profile email", DisplayName: "OIDC / SSO", FrontendRedirect: "/login"}
}

// UserInfo 是从 userinfo 端点解析出的身份（字段按 OIDC standard claims 取）。
type UserInfo struct {
	Sub      string `json:"sub"`
	Username string `json:"preferred_username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

// discovery 是 /.well-known/openid-configuration 的必要字段。
type discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// OIDCService 编排 OIDC 流程：发现 + 授权 URL + 令牌交换 + userinfo。
type OIDCService struct {
	store   SettingsStore
	hc      *http.Client
	mu      sync.Mutex
	disco   map[string]discovery // issuer → discovery（带 TTL 见 discoTTL）
	discoAt map[string]time.Time
}

// NewOIDCService 构造。store 为 SystemConfigOper（或任意 SettingsStore 实现）。
func NewOIDCService(store SettingsStore) *OIDCService {
	return &OIDCService{
		store:   store,
		hc:      &http.Client{Timeout: 30 * time.Second},
		disco:   make(map[string]discovery),
		discoAt: make(map[string]time.Time),
	}
}

// LoadSettings 读取配置（含密钥，仅管理员侧用）。
func (s *OIDCService) LoadSettings() OIDCSettings {
	out := defaultOIDCSettings()
	if s.store == nil {
		return out
	}
	raw, err := s.store.Get(oidcSettingsKey)
	if err != nil || raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	if out.Scopes == "" {
		out.Scopes = "openid profile email"
	}
	if out.DisplayName == "" {
		out.DisplayName = "OIDC / SSO"
	}
	if out.FrontendRedirect == "" {
		out.FrontendRedirect = "/login"
	}
	return out
}

// SaveSettings 落库。
func (s *OIDCService) SaveSettings(in OIDCSettings) error {
	if in.Scopes == "" {
		in.Scopes = "openid profile email"
	}
	if in.DisplayName == "" {
		in.DisplayName = "OIDC / SSO"
	}
	if in.FrontendRedirect == "" {
		in.FrontendRedirect = "/login"
	}
	raw, _ := json.Marshal(in)
	return s.store.Set(oidcSettingsKey, string(raw))
}

// discover 取（带 10min 缓存的）OIDC discovery 文档。
func (s *OIDCService) discover(ctx context.Context, issuer string) (discovery, error) {
	issuer = strings.TrimRight(issuer, "/")
	s.mu.Lock()
	if d, ok := s.disco[issuer]; ok {
		if t, ok := s.discoAt[issuer]; ok && time.Since(t) < 10*time.Minute {
			s.mu.Unlock()
			return d, nil
		}
	}
	s.mu.Unlock()

	wellKnown := issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return discovery{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.hc.Do(req)
	if err != nil {
		return discovery{}, fmt.Errorf("discovery 失败：%w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return discovery{}, fmt.Errorf("discovery %d: %s", resp.StatusCode, snippetStr(string(body)))
	}
	var d discovery
	if err := json.Unmarshal(body, &d); err != nil {
		return discovery{}, fmt.Errorf("discovery 解析失败：%w", err)
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" {
		return discovery{}, errors.New("discovery 缺少 authorization/token 端点")
	}
	s.mu.Lock()
	s.disco[issuer] = d
	s.discoAt[issuer] = time.Now()
	s.mu.Unlock()
	return d, nil
}

// FlowResult 是 StartFlow 的产出：浏览器要跳转的授权 URL + 写回浏览器的 cookie 键值。
type FlowResult struct {
	AuthorizeURL   string
	StateCookie    http.Cookie
	VerifierCookie http.Cookie
}

// StartFlow 生成授权 URL + PKCE verifier/state（以 cookie 形式由 handler 写回浏览器）。
func (s *OIDCService) StartFlow(ctx context.Context, redirectURI string) (FlowResult, error) {
	cfg := s.LoadSettings()
	if !cfg.Enabled || cfg.Issuer == "" || cfg.ClientID == "" {
		return FlowResult{}, errors.New("OIDC 未配置或未启用")
	}
	d, err := s.discover(ctx, cfg.Issuer)
	if err != nil {
		return FlowResult{}, err
	}
	state, err := randomToken(16)
	if err != nil {
		return FlowResult{}, err
	}
	verifier, err := randomToken(32)
	if err != nil {
		return FlowResult{}, err
	}
	challenge := pkceChallenge(verifier)
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", cfg.Scopes)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	authorizeURL := d.AuthorizationEndpoint + "?" + q.Encode()
	common := http.Cookie{Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600}
	sc := common
	sc.Name = "ptp_oidc_state"
	sc.Value = state
	vc := common
	vc.Name = "ptp_oidc_verifier"
	vc.Value = verifier
	return FlowResult{AuthorizeURL: authorizeURL, StateCookie: sc, VerifierCookie: vc}, nil
}

// CompleteFlow 用授权码换令牌并取 userinfo，返回用户身份。
// state/verifier 由 handler 从 cookie 取回传入校验。
func (s *OIDCService) CompleteFlow(ctx context.Context, code, state, verifier, redirectURI string) (UserInfo, error) {
	cfg := s.LoadSettings()
	if !cfg.Enabled {
		return UserInfo{}, errors.New("OIDC 未启用")
	}
	d, err := s.discover(ctx, cfg.Issuer)
	if err != nil {
		return UserInfo{}, err
	}
	// 令牌交换（含 PKCE verifier + client_secret）。
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)
	form.Set("code_verifier", verifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return UserInfo{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	tokResp, err := s.hc.Do(req)
	if err != nil {
		return UserInfo{}, fmt.Errorf("令牌交换失败：%w", err)
	}
	defer tokResp.Body.Close()
	tokBody, _ := io.ReadAll(io.LimitReader(tokResp.Body, 1<<20))
	if tokResp.StatusCode >= 400 {
		return UserInfo{}, fmt.Errorf("令牌交换 %d: %s", tokResp.StatusCode, snippetStr(string(tokBody)))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(tokBody, &tok); err != nil || tok.AccessToken == "" {
		return UserInfo{}, fmt.Errorf("令牌响应无效：%s", snippetStr(string(tokBody)))
	}
	// userinfo。
	if d.UserinfoEndpoint != "" {
		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, d.UserinfoEndpoint, nil)
		req2.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		req2.Header.Set("Accept", "application/json")
		uiResp, err := s.hc.Do(req2)
		if err == nil {
			defer uiResp.Body.Close()
			uiBody, _ := io.ReadAll(io.LimitReader(uiResp.Body, 1<<20))
			if uiResp.StatusCode < 400 {
				var ui UserInfo
				if json.Unmarshal(uiBody, &ui) == nil && ui.Sub != "" {
					return ui, nil
				}
			}
		}
	}
	// 回退：解码 id_token 的 payload（无签名校验——仅取身份，授权已由 code 交换保证）。
	if tok.IDToken != "" {
		if ui, ok := decodeIDTokenClaims(tok.IDToken); ok {
			return ui, nil
		}
	}
	return UserInfo{}, errors.New("无法获取用户信息")
}

// ===== 工具 =====

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// pkceChallenge 计算 S256 code_challenge。
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// decodeIDTokenClaims 解 id_token 第二段（payload）取标准 claims（无验签，仅 fallback）。
func decodeIDTokenClaims(idToken string) (UserInfo, bool) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return UserInfo{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return UserInfo{}, false
	}
	var ui UserInfo
	if json.Unmarshal(payload, &ui) != nil {
		return UserInfo{}, false
	}
	return ui, ui.Sub != ""
}

func snippetStr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
