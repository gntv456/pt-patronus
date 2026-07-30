// Package trakt 对接 Trakt.tv REST API：Device OAuth 授权 + WatchList 拉取。
// 仅依赖标准库 + 内部 httpclient，便于 httptest 端到端覆盖。
// 端点契约来自 Trakt API v2（trakt.tv/oauth，trakt.tv/users/me/watchlist）。
package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// BaseURL 是 Trakt API 根（可被测试覆盖）。
const BaseURL = "https://api.trakt.tv"

// APIVersion 头（Trakt 要求 trakt-api-version: 2）。
const APIVersion = "2"

// Credentials 是创建 Trakt 应用得到的 client_id/secret。
type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// DeviceCode 是 POST /oauth/device/code 的响应。
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// Token 是 OAuth 令牌（access/refresh/expires）。
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	CreatedAt    int64  `json:"created_at"` // unix 秒，自填
}

// WatchlistItem 是一条 watchlist 条目（movie/show）。
type WatchlistItem struct {
	Type    string `json:"type"` // movie | show | season | episode
	Title   string `json:"title"`
	Year    int    `json:"year"`
	TMDBID  int    `json:"tmdb"`
	IMDbID  string `json:"imdb"`
	TraktID int    `json:"trakt"`
}

// rawWatchlistItem 对应 Trakt 返回结构：{type, movie:{title,year,ids:{...}}}
type rawWatchlistItem struct {
	Type  string `json:"type"`
	Movie *struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
		IDs   struct {
			Trakt int    `json:"trakt"`
			TMDB  int    `json:"tmdb"`
			IMDb  string `json:"imdb"`
		} `json:"ids"`
	} `json:"movie"`
	Show *struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
		IDs   struct {
			Trakt int    `json:"trakt"`
			TMDB  int    `json:"tmdb"`
			IMDb  string `json:"imdb"`
		} `json:"ids"`
	} `json:"show"`
}

// Client 是 Trakt API 客户端。baseURL 可覆盖以便测试。
type Client struct {
	baseURL string
	hc      *http.Client
}

// NewClient 构造。默认用 30s 超时的标准 http.Client（Trakt 公网，无需 SSRF）。
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = BaseURL
	}
	return &Client{baseURL: baseURL, hc: &http.Client{Timeout: 30 * time.Second}}
}

// DeviceCode 请求设备授权码。
func (c *Client) DeviceCode(ctx context.Context, creds Credentials) (*DeviceCode, error) {
	body := map[string]string{"client_id": creds.ClientID}
	var dc DeviceCode
	if err := c.postJSON(ctx, c.baseURL+"/oauth/device/code", body, "", creds.ClientID, &dc); err != nil {
		return nil, err
	}
	return &dc, nil
}

// DeviceTokenError 表示设备授权轮询的预期「未完成」状态（非致命错误）。
type DeviceTokenError struct {
	Code string // authorization_pending | slow_down | expired_token | access_denied
}

func (e *DeviceTokenError) Error() string { return "device token: " + e.Code }

// PollDeviceToken 用 device_code 换 access token。未完成（用户尚未授权）返 *DeviceTokenError。
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string, creds Credentials) (*Token, error) {
	body := map[string]string{
		"code":          deviceCode,
		"client_id":     creds.ClientID,
		"client_secret": creds.ClientSecret,
		"grant_type":    "urn:ietf:params:oauth:grant-type:device_code",
	}
	// 注意：Trakt 在「未完成授权」时返 400 + {error:"authorization_pending"/"slow_down"}，
	// 这是预期的轮询状态而非传输错误——必须读 body 判 error 字段，不能按状态码直接失败。
	raw, err := c.postJSONRawLenient(ctx, c.baseURL+"/oauth/device/token", body)
	if err != nil {
		return nil, err
	}
	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(raw, &errBody)
	if errBody.Error != "" {
		return nil, &DeviceTokenError{Code: errBody.Error}
	}
	var tok Token
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, err
	}
	tok.CreatedAt = time.Now().Unix()
	return &tok, nil
}

// RefreshToken 用 refresh_token 换新令牌。
func (c *Client) RefreshToken(ctx context.Context, refresh, redirectURI string, creds Credentials) (*Token, error) {
	body := map[string]string{
		"refresh_token": refresh,
		"client_id":     creds.ClientID,
		"client_secret": creds.ClientSecret,
		"grant_type":    "refresh_token",
		"redirect_uri":  redirectURI,
	}
	var tok Token
	raw, err := c.postJSONRaw(ctx, c.baseURL+"/oauth/token", body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, err
	}
	tok.CreatedAt = time.Now().Unix()
	return &tok, nil
}

// Watchlist 拉取指定类型（movies/shows）的 watchlist。
func (c *Client) Watchlist(ctx context.Context, accessToken, clientID, kind string) ([]WatchlistItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/users/me/watchlist/%s", c.baseURL, kind), nil)
	if err != nil {
		return nil, err
	}
	c.attachAuth(req, accessToken, clientID)
	var raw []rawWatchlistItem
	if err := c.do(req, &raw); err != nil {
		return nil, err
	}
	out := make([]WatchlistItem, 0, len(raw))
	for _, r := range raw {
		if r.Type == "movie" && r.Movie != nil {
			out = append(out, WatchlistItem{Type: "movie", Title: r.Movie.Title, Year: r.Movie.Year,
				TMDBID: r.Movie.IDs.TMDB, IMDbID: r.Movie.IDs.IMDb, TraktID: r.Movie.IDs.Trakt})
		} else if r.Type == "show" && r.Show != nil {
			out = append(out, WatchlistItem{Type: "show", Title: r.Show.Title, Year: r.Show.Year,
				TMDBID: r.Show.IDs.TMDB, IMDbID: r.Show.IDs.IMDb, TraktID: r.Show.IDs.Trakt})
		}
	}
	return out, nil
}

// attachAuth 注入 Trakt 必需头：Bearer + trakt-api-version + trakt-api-key。
func (c *Client) attachAuth(req *http.Request, accessToken, clientID string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", APIVersion)
	if clientID != "" {
		req.Header.Set("trakt-api-key", clientID)
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
}

// ===== 内部 HTTP =====

func (c *Client) postJSON(ctx context.Context, u string, body any, token, clientID string, out any) error {
	raw, err := c.postJSONRaw(ctx, u, body)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) postJSONRaw(ctx context.Context, u string, body any) ([]byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", APIVersion)
	return c.raw(req)
}

// postJSONRawLenient 同 postJSONRaw 但不按状态码拒绝（用于 device/token 端点：400 是预期轮询态）。
func (c *Client) postJSONRawLenient(ctx context.Context, u string, body any) ([]byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", APIVersion)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readBody(resp), nil
}

func (c *Client) do(req *http.Request, out any) error {
	raw, err := c.raw(req)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) raw(req *http.Request) ([]byte, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data := readBody(resp)
	if resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, &UnauthorizedError{Body: snippet(data)}
		}
		return nil, fmt.Errorf("trakt %d: %s", resp.StatusCode, snippet(data))
	}
	return data, nil
}

// UnauthorizedError 表示访问令牌被服务端拒绝（401）——通常令牌被提前吊销或失效。
// 调用方可据此强制刷新 refresh_token 后重试（见 chain.TraktChain.watchlistWithRetry），
// 弥补仅按 ExpiresAt 主动刷新、无法应对提前失效的短板。
type UnauthorizedError struct{ Body string }

func (e *UnauthorizedError) Error() string { return "trakt 401 unauthorized: " + e.Body }

// snippet 取错误响应摘要。
func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
