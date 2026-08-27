package cloud

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// ---- 115 开放平台 PKCE 设备码授权 ----
//
// 契约（xhofe/115-sdk-go auth.go）：
//   POST https://passportapi.115.com/open/authDeviceCode
//        form: client_id, code_challenge(=b64(sha256(verifier))), code_challenge_method=sha256
//   → {uid, time, qrcode, sign}   qrcode=扫码 URL
//   GET https://qrcodeapi.115.com/get/status/?uid=&time=&sign= → {msg,status,version}
//   POST https://passportapi.115.com/open/deviceCodeToToken form {uid, code_verifier}
//   → {access_token, refresh_token, expires_in}
//   POST https://passportapi.115.com/open/refreshToken form {refresh_token}（轮换，两 token 都回写）
//
// status 数值语义官方文档未编码（社区语义：1=待扫码 2=已扫待确认 3=确认成功 -2=过期/取消）。

const api115Passport = "https://passportapi.115.com"
const api115QRStatus = "https://qrcodeapi.115.com/get/status/"

// PKCEVerifier 生成 code_verifier（43-128 位随机 URL 安全串）与 code_challenge。
func PKCEVerifier() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// DeviceCode115 authDeviceCode 响应。
type DeviceCode115 struct {
	UID    json.Number `json:"uid"`
	Time   json.Number `json:"time"`
	QRCode string      `json:"qrcode"`
	Sign   string      `json:"sign"`
	Err    string      `json:"error"`
}

// StartDeviceCode115 发起设备码授权（返回扫码 URL + 轮询参数 + verifier 供 Poll 用）。
// appID 为 115 开放平台应用 client_id。
func StartDeviceCode115(ctx context.Context, hc HTTPDoer, appID string) (code *DeviceCode115, verifier string, err error) {
	verifier, challenge, err := PKCEVerifier()
	if err != nil {
		return nil, "", err
	}
	form := url.Values{
		"client_id":             {appID},
		"code_challenge":        {challenge},
		"code_challenge_method": {"sha256"},
	}
	resp, err := hc.Do(ctx, "POST", api115Passport+"/open/authDeviceCode",
		[]byte(form.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "", true)
	if err != nil {
		return nil, "", fmt.Errorf("115 authDeviceCode: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("115 authDeviceCode: HTTP %d", resp.StatusCode)
	}
	var dc DeviceCode115
	if err := json.Unmarshal(resp.Body, &dc); err != nil {
		return nil, "", fmt.Errorf("115 authDeviceCode parse: %w", err)
	}
	if dc.QRCode == "" {
		return nil, "", fmt.Errorf("115 authDeviceCode 失败: %s", dc.Err)
	}
	return &dc, verifier, nil
}

// PollDeviceCode115 轮询扫码状态；confirmed=true 时 tokens 已取得。
func PollDeviceCode115(ctx context.Context, hc HTTPDoer, code *DeviceCode115, verifier string) (confirmed bool, status int, tokens *TokenPair, err error) {
	u := fmt.Sprintf("%s?uid=%s&time=%s&sign=%s", api115QRStatus,
		url.QueryEscape(code.UID.String()), url.QueryEscape(code.Time.String()), url.QueryEscape(code.Sign))
	resp, err := hc.Do(ctx, "GET", u, nil, jsonHeaders(), "", true)
	if err != nil {
		return false, 0, nil, err
	}
	if resp.StatusCode >= 300 {
		return false, 0, nil, fmt.Errorf("115 qr status: HTTP %d", resp.StatusCode)
	}
	var st struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(resp.Body, &st); err != nil {
		return false, 0, nil, fmt.Errorf("115 qr status parse: %w", err)
	}
	if st.Status != 3 { // 1/2=等待；-2=过期
		return false, st.Status, nil, nil
	}
	// 确认成功 → 换 token
	form := url.Values{"uid": {code.UID.String()}, "code_verifier": {verifier}}
	tresp, err := hc.Do(ctx, "POST", api115Passport+"/open/deviceCodeToToken",
		[]byte(form.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "", true)
	if err != nil {
		return false, st.Status, nil, fmt.Errorf("115 deviceCodeToToken: %w", err)
	}
	if tresp.StatusCode >= 300 {
		return false, st.Status, nil, fmt.Errorf("115 deviceCodeToToken: HTTP %d", tresp.StatusCode)
	}
	var tp TokenPair
	if err := json.Unmarshal(tresp.Body, &tp); err != nil {
		return false, st.Status, nil, fmt.Errorf("115 token parse: %w", err)
	}
	if tp.AccessToken == "" {
		return false, st.Status, nil, fmt.Errorf("115 token: 响应无 access_token")
	}
	return true, st.Status, &tp, nil
}

// TokenPair 115 开放平台令牌对（存储格式：JSON 序列化后整体加密，兼容裸 access_token）。
type TokenPair struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	// 阿里云盘 refresh 所需的 client 凭据（随 token 一并持久化；115 不用）
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// EncodeTokenPair 打包存储串。
func EncodeTokenPair(tp TokenPair) string {
	b, err := json.Marshal(tp)
	if err != nil {
		return tp.AccessToken
	}
	return string(b)
}

// DecodeTokenPair 解析存储串：JSON 双 token 或裸 access_token 均可。
func DecodeTokenPair(s string) TokenPair {
	s = strings.TrimSpace(s)
	if s == "" {
		return TokenPair{}
	}
	if strings.HasPrefix(s, "{") {
		var tp TokenPair
		if json.Unmarshal([]byte(s), &tp) == nil {
			return tp
		}
	}
	return TokenPair{AccessToken: s}
}

// HTTPDoer httpclient.Client 最小行为接口（授权流复用；httpDoer 的导出版）。
type HTTPDoer = httpDoer

// Refresh115Open 用 refresh_token 换新令牌（轮换；返回值应整体回写盘凭据）。
func Refresh115Open(ctx context.Context, hc HTTPDoer, refreshToken string) (*TokenPair, error) {
	form := url.Values{"refresh_token": {refreshToken}}
	resp, err := hc.Do(ctx, "POST", api115Passport+"/open/refreshToken",
		[]byte(form.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "", true)
	if err != nil {
		return nil, fmt.Errorf("115 refreshToken: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("115 refreshToken: HTTP %d", resp.StatusCode)
	}
	var tp TokenPair
	if err := json.Unmarshal(resp.Body, &tp); err != nil {
		return nil, fmt.Errorf("115 refreshToken parse: %w", err)
	}
	if tp.AccessToken == "" {
		return nil, fmt.Errorf("115 refreshToken: 响应无 access_token")
	}
	return &tp, nil
}
