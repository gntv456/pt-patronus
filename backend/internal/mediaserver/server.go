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
}

// NewClient 按 Server.Type 构建对应客户端。type 不在白名单内返回错误。
func NewClient(s Server, hc *httpclient.Client) (Client, error) {
	switch s.Type {
	case "emby", "jellyfin":
		return &embyJellyfinClient{server: s, hc: hc, apiRoot: apiRoot(s)}, nil
	case "plex":
		return &plexClient{server: s, hc: hc, apiRoot: apiRoot(s)}, nil
	}
	return nil, fmt.Errorf("不支持的媒体服务器类型: %s", s.Type)
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
