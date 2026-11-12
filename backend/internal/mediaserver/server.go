// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package mediaserver 统一封装 Emby / Jellyfin / Plex 的 REST 客户端子集：
// 库扫描 / 元数据刷新 / 管理员通知 / 近期条目（封面生成用）。
//
// 设计：Emby 与 Jellyfin 同源（JF fork 自 Emby），REST 路径与 X-Emby-Token 鉴权高度一致，
// 共用一个 embyJellyfinClient；Plex 协议独立，单独 plexClient。两者实现同一 Client 接口，
// chain 层按 Server.Type dispatch，上层无需感知差异。
package mediaserver

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// Server 是构建 client 所需的连接信息（oper 解密 Token 后填入）。
type Server struct {
	ID       uint
	Type     string // emby | jellyfin | plex
	URL      string
	Username string
	Token    string // API Key(Emby/JF) 或 X-Plex-Token
}

// Library 媒体库（虚拟目录）。
type Library struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // movies | tvshows | music | mixed | ...
}

// Item 媒体条目。ImageURL 由 client 预拼好鉴权参数，封面生成器直接拉取。
type Item struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}

// User 媒体服务器用户（观影报告用）。PlayCount 由各端点尽力填充（Emby 聚合自 UserData，
// Plex 取自 history/sessions，无则 0）。
type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PlayCount int    `json:"play_count"`
}

// PlayItem 一条播放记录（观影报告）。LastPlayed 为 ISO 时间串；Duration 单位秒（未知=0）。
type PlayItem struct {
	ItemID     string `json:"item_id"`
	Name       string `json:"name"`
	Type       string `json:"type"` // movie | series | episode
	Year       int    `json:"year"`
	UserID     string `json:"user_id"`
	UserName   string `json:"user_name"`
	LastPlayed string `json:"last_played"`
	PlayCount  int    `json:"play_count"`
	Duration   int    `json:"duration"`
}

// DetailedItem 库内条目（重复检测用，带年份与 Provider ID）。TMDBID/IMDbID 为空则按标题+年份归组。
type DetailedItem struct {
	ItemID string `json:"item_id"`
	Name   string `json:"name"`
	Type   string `json:"type"` // movie | series
	Year   int    `json:"year"`
	TMDBID string `json:"tmdb_id"`
	IMDbID string `json:"imdb_id"`
	Path   string `json:"path"`
}

// Client 统一三家媒体服务器的操作子集。
type Client interface {
	// Test 连接 + 鉴权探活（拉服务信息）。
	Test(ctx context.Context) (info string, err error)
	// Libraries 列出全部媒体库。
	Libraries(ctx context.Context) ([]Library, error)
	// LibraryScan 触发整库扫描（扫文件入索引）。
	LibraryScan(ctx context.Context, libraryID string) error
	// Refresh 刷新库元数据（重扫 + 拉元数据）。
	Refresh(ctx context.Context, libraryID string) error
	// Notify 发管理员通知；不支持的服务器返回明显错误（不 panic）。
	Notify(ctx context.Context, title, body string) error
	// RecentItems 取某库最近 N 条带封面的条目（封面生成用）。
	RecentItems(ctx context.Context, libraryID string, limit int) ([]Item, error)
	// FetchImage 拉取并解码条目封面为 image.Image。
	FetchImage(ctx context.Context, item Item) (image.Image, error)
	// Users 列出服务器用户（观影报告用）。
	Users(ctx context.Context) ([]User, error)
	// PlayedItems 取某用户最近播放的 N 条（观影报告）。limit<=0 用默认。
	PlayedItems(ctx context.Context, userID string, limit int) ([]PlayItem, error)
	// LibraryItems 枚举某库全部条目（重复检测用；带年份与 Provider ID）。
	LibraryItems(ctx context.Context, libraryID string) ([]DetailedItem, error)
	// LibraryItemCount 取某库 movie/series 条目计数（工作台媒体库态势）。
	// 与 LibraryItems 的区别：只拿计数不拉条目——上万条的 Emby/Plex 库走
	// LibraryItems 会整库 JSON 拉回只为 ++，工作台首屏被拖到数秒。
	// 传 Library（含归一后的 type）而非裸 ID：Plex 的 section 计数按库类型
	// 整体给出，需要知道这是 movie 库还是 show 库才能正确归类。
	LibraryItemCount(ctx context.Context, lib Library) (LibraryCount, error)
}

// LibraryCount 单库按类型聚合的条目计数（LibraryItemCount 的返回）。
type LibraryCount struct {
	Movies int `json:"movies"`
	Series int `json:"series"`
}

// NewClient 按 Server.Type 构建对应客户端。type 不在白名单内返回错误。
// ugos（绿影视）/ fnos（飞牛影视）/ zspace（极空间极影视）均为 Emby/Jellyfin 兼容内核
// 或提供 Emby 兼容网关，复用 embyJellyfinClient（apiRoot 已含各家的 /emby 前缀差异处理）。
func NewClient(s Server, hc *httpclient.Client) (Client, error) {
	switch s.Type {
	case "emby", "jellyfin":
		return &embyJellyfinClient{server: s, hc: hc, apiRoot: apiRoot(s)}, nil
	case "plex":
		return &plexClient{server: s, hc: hc, apiRoot: apiRoot(s)}, nil
	case "ugos", "fnos", "zspace":
		return &embyCompatClient{embyJellyfinClient{server: s, hc: hc, apiRoot: compatAPIRoot(s)}}, nil
	}
	return nil, fmt.Errorf("不支持的媒体服务器类型: %s", s.Type)
}

// compatAPIRoot 国产 NAS 影视服务的 Emby 兼容层前缀：
//   - 绿影视（UGOS）：{url}/index.php/emby （Emby 兼容插件路径）
//   - 飞牛影视（FNOS）：{url}/index.php/emby （fnOS 官方 Emby 兼容网关）
//   - 极空间极影视：{url}/emby （极影视内建 Emby 兼容 API）
//
// 用户在 UI 填服务根 URL 即可，此处自动补兼容前缀（已带前缀则不重复补）。
func compatAPIRoot(s Server) string {
	u := apiRoot(s)
	switch s.Type {
	case "ugos", "fnos":
		if !strings.Contains(u, "/emby") {
			return u + "/index.php/emby"
		}
	case "zspace":
		if !strings.Contains(u, "/emby") {
			return u + "/emby"
		}
	}
	return u
}

// apiRoot 规范化服务器根 URL（去尾斜杠）。
func apiRoot(s Server) string {
	u := s.URL
	for len(u) > 0 && u[len(u)-1] == '/' {
		u = u[:len(u)-1]
	}
	return u
}

// httpDoer 是 httpclient.Client 的最小行为接口（便于单测注入 httptest）。
type httpDoer interface {
	Do(ctx context.Context, method, rawURL string, body []byte,
		headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error)
}

// jsonGet / jsonPost 是三家客户端共用的薄封装：发请求 + 校验状态码 + 解析 JSON。
func jsonGet(ctx context.Context, hc httpDoer, url string, headers map[string]string, out any) error {
	return jsonCall(ctx, hc, "GET", url, nil, headers, out)
}

func jsonPost(ctx context.Context, hc httpDoer, url string, body []byte, headers map[string]string, out any) error {
	return jsonCall(ctx, hc, "POST", url, body, headers, out)
}

// jsonCall 发请求、校验 2xx、解码 JSON 到 out（out 为 nil 时跳过解码）。
func jsonCall(ctx context.Context, hc httpDoer, method, url string, body []byte, headers map[string]string, out any) error {
	// 媒体服务器多为内网部署：ssrf=false 跳过公网守卫（端点由管理员配置，同下载器范式）。
	resp, err := hc.Do(ctx, method, url, body, withJSON(headers), "", false)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d", method, redactURL(url), resp.StatusCode)
	}
	if out == nil || len(resp.Body) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Body, out)
}

// withJSON 确保 headers 含 Accept: application/json（不覆盖调用方已设的键）。
func withJSON(headers map[string]string) map[string]string {
	if headers == nil {
		headers = map[string]string{}
	}
	if _, ok := headers["Accept"]; !ok {
		headers["Accept"] = "application/json"
	}
	return headers
}

// fetchImageBytes 拉取图片字节（GET，带鉴权头；ssrf=false）。
func fetchImageBytes(ctx context.Context, hc httpDoer, url string, headers map[string]string) (*httpclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := hc.Do(ctx, "GET", url, nil, headers, "", false)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch image: HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// redactURL 隐藏 query 中的 token，避免错误信息泄露凭据。
func redactURL(rawURL string) string {
	for i := 0; i < len(rawURL); i++ {
		if rawURL[i] == '?' {
			return rawURL[:i] + "?…"
		}
	}
	return rawURL
}
