// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

// Piggo（猪猪，piggo.me）适配器 —— Laravel + Vite SPA 自研引擎，全 JSON API。
// 契约来源：2026-11-07 用站点会话实测（登录态 token+sign 采样全部端点，见 CHANGELOG）。
//
// 鉴权（三件套，缺一即 401）：
//   - Authorization: Bearer <token>（SRP 登录签发，绑定登录时的 visitorId）
//   - HMAC 请求签名：X-Spa-Nonce + X-Spa-Hmac = HMAC-SHA256(key=sign 字符串原文,
//     msg="<METHOD>\n<path>\n<ts>\n<nonce>")；path 规范化（去 query、去首尾斜杠、小写）；
//     sign 由 GET /v1/web/auth/sign 的 data.sign 下发，登录会话内有效
//   - 固定头 visitorId（32 hex，登录时与 token 绑定）/ ts / version=1.0.0 / webversion=1
//
// 端点（实测）：
//   搜索   GET /v1/web/torrents?page=N&keyword=...（section 路由 /torrents/movie 等）
//   详情   GET /v1/web/torrents/{id} → data.item
//   资料   GET /v1/web/me → data.user（stats: bonus/ratio/uploaded[格式化串]/seeding...）
//   下载   POST /v1/web/torrents/{id}/download-link（无 body）→ data.url（签名直链，
//          匿名 GET 即得 .torrent；等价 sunnypt 模式）
// 注意：新号有「首次登录须读 Wiki」闸门（403 wiki_unread），API 层无法代答，
// 需用户在网页点一次「确认已阅读」；闸门未清时种子端点全 403，错误信息已透出。

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

type Piggo struct {
	hc *httpclient.Client

	mu      sync.Mutex
	session map[string]*piggoSession // key = baseURL + "|" + username
}

// piggoSession 是一次登录的凭据态（token+sign+visitorId 绑定为一组）。
type piggoSession struct {
	Token     string
	Sign      string
	VisitorID string
	CreatedAt time.Time
}

func NewPiggo(hc *httpclient.Client) *Piggo {
	return &Piggo{hc: hc, session: map[string]*piggoSession{}}
}

func (p *Piggo) Schema() string { return "piggo" }
func (p *Piggo) Name() string   { return "site.piggo" }
func (p *Piggo) Priority() int  { return 100 }

// ── 会话管理 ──

// piggoCred 是 chain.credential 打包的账密凭据（chain/site_credential.go piggo 分支）。
type piggoCred struct {
	Username string
	Password string
}

func parsePiggoCred(raw string) piggoCred {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return piggoCred{}
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		return piggoCred{Username: strings.TrimSpace(m["username"]), Password: m["password"]}
	}
	return piggoCred{}
}

// sessionFor 取缓存会话（12h 刷新）；未登录则 SRP 登录并缓存。
// 适配器实例为进程单例：token 仅存内存，重启后按站点账密重登（账密在站点凭据密文里）。
func (p *Piggo) sessionFor(ctx context.Context, baseURL string, cred piggoCred) (*piggoSession, error) {
	// 凭据缺账密：若进程内已有该站的活跃登录态（如外部会话注入的测试场景）直接复用；
	// 生产路径凭据永远带账密，不会走到这里。
	if cred.Username == "" || cred.Password == "" {
		p.mu.Lock()
		for k, s := range p.session {
			if strings.HasPrefix(k, strings.TrimRight(baseURL, "/")+"|") &&
				time.Since(s.CreatedAt) < 12*time.Hour {
				p.mu.Unlock()
				return s, nil
			}
		}
		p.mu.Unlock()
		return nil, fmt.Errorf("piggo 需要站点账号密码：请在站点凭据里填写用户名与密码（站点用 SRP 加密登录，不支持 Cookie 接入）")
	}
	key := strings.TrimRight(baseURL, "/") + "|" + cred.Username
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.session[key]; ok && time.Since(s.CreatedAt) < 12*time.Hour {
		return s, nil
	}
	token, err := pgSrpLogin(ctx, p.hc, baseURL, cred.Username, cred.Password)
	if err != nil {
		return nil, err
	}
	s := &piggoSession{Token: token, VisitorID: pgVisitorID(), CreatedAt: time.Now()}
	// sign key：登录后立即取（每会话不同；丢失/失效时 401 会触发重登）。
	var sg struct {
		Code int `json:"code"`
		Data struct {
			Sign string `json:"sign"`
		} `json:"data"`
	}
	if err := p.signedJSON(ctx, s, baseURL, "GET", "/v1/web/auth/sign", nil, &sg); err == nil &&
		sg.Code == 0 && sg.Data.Sign != "" {
		s.Sign = sg.Data.Sign
	}
	p.session[key] = s
	return s, nil
}

// invalidate 丢弃会话（401 时调用，下次请求重登）。
func (p *Piggo) invalidate(baseURL string, cred piggoCred) {
	key := strings.TrimRight(baseURL, "/") + "|" + cred.Username
	p.mu.Lock()
	delete(p.session, key)
	p.mu.Unlock()
}

// signedJSON 发带 Bearer + HMAC 签名的请求并解包 {code,data} 信封。
// 401 返回 site.ErrNotLoggedIn（调用方按凭据失效口径处理）。
func (p *Piggo) signedJSON(ctx context.Context, s *piggoSession, baseURL, method, path string, body any, out any) error {
	payload := []byte{}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = b
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := pgVisitorID() // 32 hex 随机串（前端 G9：16 随机字节 hex）
	full := strings.TrimRight(baseURL, "/") + path
	// path 规范化（前端 r5）：去 query、去首尾斜杠、小写
	norm := strings.ToLower(strings.Trim(strings.SplitN(path, "?", 2)[0], "/"))
	mac := hmac.New(sha256.New, []byte(s.Sign))
	fmt.Fprintf(mac, "%s\n%s\n%s\n%s", strings.ToUpper(method), norm, ts, nonce)
	headers := map[string]string{
		"Authorization":    "Bearer " + s.Token,
		"visitorId":        s.VisitorID,
		"ts":               ts,
		"version":          "1.0.0",
		"webversion":       "1",
		"X-Spa-Nonce":      nonce,
		"X-Spa-Hmac":       fmt.Sprintf("%x", mac.Sum(nil)),
		"Accept":           "application/json",
		"X-Requested-With": "XMLHttpRequest",
	}
	if body != nil {
		headers["Content-Type"] = "application/json"
	}
	resp, err := p.hc.Do(ctx, method, full, payload, headers, "", true)
	if err != nil {
		return err
	}
	if resp.StatusCode == 401 {
		return site.ErrNotLoggedIn
	}
	var env struct {
		Code    int             `json:"code"`
		Data    json.RawMessage `json:"data"`
		Message string          `json:"msg"`
		DataMsg string          `json:"message"`
	}
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return fmt.Errorf("parse piggo response: %w", err)
	}
	// 业务闸门（wiki_unread 等）：HTTP 200/403 + code 字段为字符串错误码。
	if len(env.Data) > 0 && env.Data[0] == '{' {
		var gate struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(env.Data, &gate) == nil && gate.Code != "" && gate.Code != "0" {
			return fmt.Errorf("piggo 业务拒绝（%s）：%s", gate.Code, env.Message)
		}
	}
	if env.Code != 0 {
		return fmt.Errorf("piggo api error: %s", env.Message)
	}
	if out == nil || len(env.Data) == 0 {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// ── 搜索 ──

func (p *Piggo) Search(ctx context.Context, def *site.SiteMetadata, baseURL, token, keyword, category string, page int) ([]site.Torrent, error) {
	if page < 1 {
		page = 1
	}
	cred := parsePiggoCred(token)
	s, err := p.sessionFor(ctx, baseURL, cred)
	if err != nil {
		return nil, err
	}
	// category → section 路由（电影/剧集有独立分区；其余走综合 + keyword）
	path := fmt.Sprintf("/v1/web/torrents?page=%d", page)
	switch category {
	case "movie":
		path = fmt.Sprintf("/v1/web/torrents/movie?page=%d", page)
	case "tv":
		path = fmt.Sprintf("/v1/web/torrents/tv?page=%d", page)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		path += "&keyword=" + url.QueryEscape(kw)
	}
	var data struct {
		Items []map[string]any `json:"items"`
	}
	if err := p.signedJSON(ctx, s, baseURL, "GET", path, nil, &data); err != nil {
		if errors.Is(err, site.ErrNotLoggedIn) {
			p.invalidate(baseURL, cred)
		}
		return nil, err
	}
	out := make([]site.Torrent, 0, len(data.Items))
	for _, it := range data.Items {
		out = append(out, parsePiggoTorrent(it, baseURL))
	}
	return out, nil
}

// parsePiggoTorrent 列表项 → site.Torrent。
// promotion.label「免费」→ FREE（up/down 因子：up=1/down=0 即免费档；倍数字段站点未给数值档）。
func parsePiggoTorrent(it map[string]any, baseURL string) site.Torrent {
	id := fmt.Sprint(it["id"])
	t := site.Torrent{
		ID:        id,
		Title:     pgStr(it, "name"),
		SubTitle:  pgStr(it, "small_descr"),
		Size:      formatMTBytes(pgInt(it, "size")),
		Seeders:   int(pgInt(it, "seeders")),
		Leechers:  int(pgInt(it, "leechers")),
		Completed: int(pgInt(it, "times_completed")),
	}
	if id != "" {
		web := strings.TrimRight(baseURL, "/")
		t.DetailURL = web + "/detail/" + id
		// 下载走 POST download-link 换签名直链（FetchTorrentFile piggo 分支按此 URL 抽 id），
		// 这里合成带标记的下载占位 URL（同注册域，满足同站守卫）。
		t.DownloadURL = web + "/v1/web/torrents/" + id + "/dl"
	}
	// 促销：promotion.label 含「免费」→ FREE；全局促销同判（新窝首免等）。
	if pgIsFree(pgMap(it, "promotion")) || pgIsFree(pgMap(it, "global_promotion")) {
		t.Free = true
		t.DownloadVolumeFactor = 0
	} else {
		t.DownloadVolumeFactor = 1
	}
	if pr := pgMap(it, "promotion"); pr != nil {
		if up := pgInt(pr, "up"); up > 1 {
			t.UploadVolumeFactor = float64(up)
		}
	}
	if sec := pgMap(it, "section"); sec != nil {
		t.Category = pgStr(sec, "name")
	}
	return t
}

func pgIsFree(m map[string]any) bool {
	if m == nil {
		return false
	}
	return strings.Contains(pgStr(m, "label"), "免费") || strings.Contains(strings.ToUpper(pgStr(m, "label")), "FREE")
}

// ── 用户资料 ──

func (p *Piggo) FetchUserInfo(ctx context.Context, def *site.SiteMetadata, baseURL, token string) (*site.UserInfo, error) {
	cred := parsePiggoCred(token)
	s, err := p.sessionFor(ctx, baseURL, cred)
	if err != nil {
		return nil, err
	}
	var data struct {
		User struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			Email     string `json:"email"`
			Avatar    string `json:"avatar"`
			CreatedAt string `json:"created_at"`
			Class     struct {
				Name string `json:"name"`
			} `json:"class"`
			Stats struct {
				Bonus             any    `json:"bonus"`
				Ratio             any    `json:"ratio"`
				Uploaded          string `json:"uploaded"`   // 格式化串（"80.53 GB"）
				Downloaded        string `json:"downloaded"` // 格式化串
				Seeding           int64  `json:"seeding"`
				Leeching          int64  `json:"leeching"`
				SeedbonusPerHour  any    `json:"seedbonus_per_hour"`
				SeedPointsPerHour any    `json:"seed_points_per_hour"`
			} `json:"stats"`
		} `json:"user"`
	}
	if err := p.signedJSON(ctx, s, baseURL, "GET", "/v1/web/me", nil, &data); err != nil {
		if errors.Is(err, site.ErrNotLoggedIn) {
			p.invalidate(baseURL, cred)
		}
		return nil, err
	}
	u := data.User
	return &site.UserInfo{
		ID:           strconv.FormatInt(u.ID, 10),
		Name:         u.Username,
		Email:        u.Email,
		Avatar:       u.Avatar,
		Level:        u.Class.Name,
		JoinDate:     u.CreatedAt,
		Uploaded:     pgParseSize(u.Stats.Uploaded),
		Downloaded:   pgParseSize(u.Stats.Downloaded),
		Ratio:        pgFloat(u.Stats.Ratio),
		Bonus:        pgFloat(u.Stats.Bonus),
		BonusPerHour: pgFloat(u.Stats.SeedbonusPerHour),
		Seeding:      int(u.Stats.Seeding),
		Leeching:     int(u.Stats.Leeching),
	}, nil
}

// ── 下载 ──

// DownloadTorrentFile 按 id 下载 .torrent：POST download-link 换签名直链 → GET 直链。
// 由 chain/site_torrent.go 的 piggo 分支调用（凭据打包为账密 JSON）。
func (p *Piggo) DownloadTorrentFile(ctx context.Context, baseURL, credRaw, torrentID string) ([]byte, string, error) {
	cred := parsePiggoCred(credRaw)
	s, err := p.sessionFor(ctx, baseURL, cred)
	if err != nil {
		return nil, "", err
	}
	var data struct {
		URL string `json:"url"`
	}
	path := "/v1/web/torrents/" + torrentID + "/download-link"
	if err := p.signedJSON(ctx, s, baseURL, "POST", path, nil, &data); err != nil {
		if errors.Is(err, site.ErrNotLoggedIn) {
			p.invalidate(baseURL, cred)
		}
		return nil, "", err
	}
	if !strings.HasPrefix(data.URL, "http") {
		return nil, "", fmt.Errorf("piggo download-link 返回非法 url")
	}
	resp, err := p.hc.Get(ctx, data.URL, "", true) // 签名直链，匿名 GET
	if err != nil {
		return nil, "", fmt.Errorf("piggo fetch torrent: %w", err)
	}
	if !pgIsTorrentBytes(resp.Body) {
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return nil, "", site.ErrNotLoggedIn
		}
		return nil, "", fmt.Errorf("piggo 下载响应不是 .torrent 文件（HTTP %d）", resp.StatusCode)
	}
	return resp.Body, "piggo-" + torrentID + ".torrent", nil
}

// ── 容错取值（列表项字段类型不稳定：int/float/string 混用）──

func pgStr(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	switch v := m[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	}
	return ""
}

func pgInt(m map[string]any, k string) int64 { return toInt64(m[k]) }

func pgFloat(v any) float64 { return toFloat64(v) }

func pgMap(m map[string]any, k string) map[string]any {
	if m == nil {
		return nil
	}
	mm, _ := m[k].(map[string]any)
	return mm
}

// pgParseSize 解析站点格式化大小串（"80.53 GB" / "898.18 MB"）→ 字节。
func pgParseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	fields := strings.Fields(s)
	if len(fields) != 2 {
		return 0
	}
	num, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	unit := strings.ToUpper(strings.TrimSpace(fields[1]))
	mult := 1.0
	switch {
	case strings.HasPrefix(unit, "T"):
		mult = 1 << 40
	case strings.HasPrefix(unit, "G"):
		mult = 1 << 30
	case strings.HasPrefix(unit, "M"):
		mult = 1 << 20
	case strings.HasPrefix(unit, "K"):
		mult = 1 << 10
	}
	return int64(num * mult)
}

// pgIsTorrentBytes 轻量 bencode sniff（与 chain.isTorrentBytes 同判据：首 'd' 末 'e'
// 且含 info/announce 键；该函数在 chain 包不可直接复用，此处按同口径内联）。
func pgIsTorrentBytes(b []byte) bool {
	return len(b) > 64 && b[0] == 'd' && b[len(b)-1] == 'e' &&
		(bytes.Contains(b, []byte("4:info")) || bytes.Contains(b, []byte("8:announce")))
}
