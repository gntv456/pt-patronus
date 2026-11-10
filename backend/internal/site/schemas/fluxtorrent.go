// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// FluxTorrent 是 FluxTorrent 建站系统（Rust + Next.js，非 NexusPHP）的适配器。
// 页面为 Next.js RSC 渲染、无 NP 形状 HTML，全部走 /api/v1/compat JSON 兼容层（契约见
// FluxTorrent _doc/工具维护者对接页.md）：
//   - 搜索 GET /api/v1/compat/nexusphp/torrents.json?keyword=&page=&pagesize=（NP 字段口径）
//   - 用户 GET /api/v1/plugins/ptppUserInfo（PT-Plugin-Plus 聚合端点，含 passkey）
//   - 下载 GET /api/v1/compat/nexusphp/download.php?id=&passkey=（见 chain/site_torrent 通用补全）
//
// 鉴权：Authorization: Token <fxo_ 开放 API Token>（注意前缀是 Token 不是 Bearer）；
// cookie 对兼容层不可用。凭据包由 chain.credential 打包（token+passkey+cookie JSON），
// parseFluxTorrentCredential 解析。所有响应为 {code,message,data} 信封，code!=0 视为业务失败。
type FluxTorrent struct {
	hc *httpclient.Client
}

func NewFluxTorrent(hc *httpclient.Client) *FluxTorrent {
	return &FluxTorrent{hc: hc}
}

func (f *FluxTorrent) Schema() string { return "fluxtorrent" }
func (f *FluxTorrent) Name() string   { return "site.fluxtorrent" }
func (f *FluxTorrent) Priority() int  { return 200 }

// fluxtorrentCredential 是 chain.credential 打包的凭据包。token 必填；passkey 供下载链
// （fetch 时可能尚未获取，留空则 DownloadURL 不带 passkey，由 chain 通用补全兜底）。
type fluxtorrentCredential struct {
	Token   string
	Passkey string
}

func parseFluxTorrentCredential(raw string) fluxtorrentCredential {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fluxtorrentCredential{}
	}
	if strings.HasPrefix(raw, "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			return fluxtorrentCredential{
				Token:   firstNonEmptyF3NIX(m["token"], m["api_key"], m["apikey"], m["apiKey"]),
				Passkey: firstNonEmptyF3NIX(m["passkey"], m["rsskey"]),
			}
		}
	}
	return fluxtorrentCredential{Token: raw}
}

// withFluxTorrentAuth 注入 Token 鉴权头 + JSON Accept。
func withFluxTorrentAuth(headers map[string]string, token string) map[string]string {
	out := map[string]string{}
	for k, v := range headers {
		out[k] = v
	}
	if token != "" {
		out["Authorization"] = "Token " + token
	}
	out["Accept"] = "application/json"
	return out
}

// fluxEnvelope 是统一响应信封。
type fluxEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// fluxNPItem 是 torrents.json 的单条（NP 字段口径：size 字节、added Unix 秒、completed 完成数）。
type fluxNPItem struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	SmallDescr   string `json:"small_descr"`
	Seeders      int    `json:"seeders"`
	Leechers     int    `json:"leechers"`
	Completed    int    `json:"completed"`
	Size         int64  `json:"size"`
	Added        int64  `json:"added"`
	CategoryName string `json:"category_name"`
	Promotion    string `json:"promotion"`
	Free         bool   `json:"free"`
	InfoHash     string `json:"info_hash"`
}

type fluxNPList struct {
	Items []fluxNPItem `json:"items"`
}

// Search 走 NP 兼容层 torrents.json（NP 搜索参数口径：keyword/page/pagesize）。
func (f *FluxTorrent) Search(ctx context.Context, def *site.SiteMetadata,
	baseURL, credRaw, keyword string, category string, page int) ([]site.Torrent, error) {
	cred := parseFluxTorrentCredential(credRaw)
	if cred.Token == "" {
		return nil, fmt.Errorf("fluxtorrent api token required（网页端 我的 → API Token 签发，fxo_ 开头）")
	}
	if page < 1 {
		page = 1
	}
	q := url.Values{}
	q.Set("keyword", keyword)
	q.Set("page", strconv.Itoa(page))
	q.Set("pagesize", "50")
	target := joinURL(baseURL, "/api/v1/compat/nexusphp/torrents.json") + "?" + q.Encode()
	resp, err := f.hc.GetWithHeaders(ctx, target, "", withFluxTorrentAuth(nil, cred.Token), true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, site.ErrNotLoggedIn
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fluxtorrent search: status %d", resp.StatusCode)
	}
	var env fluxEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("fluxtorrent search: parse envelope: %w", err)
	}
	if env.Code != 0 {
		return nil, fluxBizErr("search", env)
	}
	var list fluxNPList
	// data 兼容两种形状：{"items":[...]}（现行）或直接 [...]（宽松兜底）
	if err := json.Unmarshal(env.Data, &list); err != nil {
		var direct []fluxNPItem
		if derr := json.Unmarshal(env.Data, &direct); derr != nil {
			return nil, fmt.Errorf("fluxtorrent search: parse items: %w", err)
		}
		list.Items = direct
	}
	out := make([]site.Torrent, 0, len(list.Items))
	for _, it := range list.Items {
		t := site.Torrent{
			ID:        strconv.Itoa(it.ID),
			Title:     it.Name,
			SubTitle:  it.SmallDescr,
			DetailURL: joinURL(baseURL, fmt.Sprintf("/torrent/%d", it.ID)),
			Size:      site.FormatBytes(it.Size),
			Seeders:   it.Seeders,
			Leechers:  it.Leechers,
			Completed: it.Completed,
			Category:  it.CategoryName,
			Labels:    fluxPromotionLabels(it),
		}
		if it.Added > 0 {
			t.Time = time.Unix(it.Added, 0).Format("2006-01-02 15:04:05")
			t.PubDate = time.Unix(it.Added, 0)
		}
		t.DownloadVolumeFactor, t.UploadVolumeFactor = fluxPromotionFactors(it.Promotion)
		t.Free = it.Free || t.DownloadVolumeFactor == 0
		if pk := strings.TrimSpace(cred.Passkey); pk != "" {
			t.DownloadURL = joinURL(baseURL, fmt.Sprintf("/api/v1/compat/nexusphp/download.php?id=%d&passkey=%s", it.ID, url.QueryEscape(pk)))
		} else {
			// 无 passkey：给无凭证模板链，chain FetchTorrentFile 通用 passkey 补全会替换。
			t.DownloadURL = joinURL(baseURL, fmt.Sprintf("/api/v1/compat/nexusphp/download.php?id=%d", it.ID))
		}
		out = append(out, t)
	}
	return out, nil
}

// fluxPTPPData 是 ptppUserInfo 端点的 data（字段名即 PT-Plugin-Plus 习惯）。
// id 数字或字符串均收（json.Number 兼容两边口径）；joinTime 同理——实测见过 Unix 秒与
// ISO8601 字符串两种口径，用自定义类型 fluxTime 归一为 time.Time。
type fluxPTPPData struct {
	ID           json.Number `json:"id"`
	Name         string      `json:"name"`
	Bonus        float64     `json:"bonus"`
	Uploaded     int64       `json:"uploaded"`
	Downloaded   int64       `json:"downloaded"`
	Seeding      int         `json:"seeding"`
	Leeching     int         `json:"leeching"`
	SeedingSize  int64       `json:"seedingSize"`
	Invites      int         `json:"invites"`
	LevelName    string      `json:"levelName"`
	JoinTime     fluxTime    `json:"joinTime"`
	MessageCount int         `json:"messageCount"`
	Passkey      string      `json:"passkey"`
}

// fluxTime 兼容 Unix 秒（数字）与 ISO8601（字符串）两种时间口径。
type fluxTime time.Time

func (t *fluxTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		return nil
	}
	if sec, err := strconv.ParseInt(s, 10, 64); err == nil {
		if sec > 0 {
			*t = fluxTime(time.Unix(sec, 0))
		}
		return nil
	}
	if pt, err := time.Parse(time.RFC3339, s); err == nil {
		*t = fluxTime(pt)
		return nil
	}
	// 容错：截前 10 位日期（YYYY-MM-DD）
	if len(s) >= 10 {
		if pt, err := time.Parse("2006-01-02", s[:10]); err == nil {
			*t = fluxTime(pt)
		}
	}
	return nil
}

// FetchUserInfo 走 PT-Plugin-Plus 聚合端点（一次请求拿全量用户数据 + passkey）。
// 发现的 passkey 挂在返回值 UserInfo.DiscoveredPasskey 上随本次调用交给 chain 落库
// （见 chain.SiteChain.maybeAdoptPasskey）——不经实例字段中转，否则多站并发会串站。
func (f *FluxTorrent) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, credRaw string) (*site.UserInfo, error) {
	cred := parseFluxTorrentCredential(credRaw)
	if cred.Token == "" {
		return nil, fmt.Errorf("fluxtorrent api token required（网页端 我的 → API Token 签发，fxo_ 开头）")
	}
	resp, err := f.hc.GetWithHeaders(ctx, joinURL(baseURL, "/api/v1/plugins/ptppUserInfo"), "",
		withFluxTorrentAuth(nil, cred.Token), true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, site.ErrNotLoggedIn
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fluxtorrent userinfo: status %d", resp.StatusCode)
	}
	var env fluxEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("fluxtorrent userinfo: parse envelope: %w", err)
	}
	if env.Code != 0 {
		return nil, fluxBizErr("userinfo", env)
	}
	var d fluxPTPPData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return nil, fmt.Errorf("fluxtorrent userinfo: parse data: %w", err)
	}
	u := &site.UserInfo{
		ID:          d.ID.String(),
		Name:        d.Name,
		Level:       d.LevelName,
		Uploaded:    d.Uploaded,
		Downloaded:  d.Downloaded,
		Seeding:     d.Seeding,
		Leeching:    d.Leeching,
		SeedingSize: d.SeedingSize,
		Bonus:       d.Bonus,
		Invited:     d.Invites,
	}
	if u.ID == "" {
		u.ID = d.Name // 兜底：聚合端点偶发缺 id 时用用户名，避免 UID 空串中断快照链
	}
	if d.Downloaded > 0 {
		u.Ratio = float64(d.Uploaded) / float64(d.Downloaded)
	}
	if jt := time.Time(d.JoinTime); !jt.IsZero() {
		u.JoinDate = jt.Format("2006-01-02")
	}
	u.DiscoveredPasskey = strings.TrimSpace(d.Passkey)
	return u, nil
}

// fluxBizErr 把信封业务错误转为 error（保留服务端 message）。
func fluxBizErr(op string, env fluxEnvelope) error {
	if env.Message == "" {
		return fmt.Errorf("fluxtorrent %s: code %d", op, env.Code)
	}
	return fmt.Errorf("fluxtorrent %s: %s", op, env.Message)
}

// fluxPromotionFactors 把 promotion 映射为（下载因子, 上传因子）。
// 与 nexusPromotionFactor 同口径：0=FREE 1=正常 0.5=半价 2=双倍上传；upload 留 0 由
// 通用层（site/promo.go）归一为 1，表示「非上传倍率促销」。
func fluxPromotionFactors(promotion string) (download, upload float64) {
	download, upload = 1, 0
	switch strings.ToLower(strings.TrimSpace(promotion)) {
	case "free":
		download = 0
	case "x2free":
		download, upload = 0, 2
	case "half":
		download = 0.5
	case "2x50":
		download, upload = 0.5, 2
	case "2x":
		upload = 2
	}
	return download, upload
}

// fluxPromotionLabels 促销标签（非空 promotion 时附 promotion_name 语义的原始值）。
func fluxPromotionLabels(it fluxNPItem) []string {
	p := strings.TrimSpace(it.Promotion)
	if p == "" || strings.EqualFold(p, "none") || strings.EqualFold(p, "normal") {
		return nil
	}
	return []string{p}
}
