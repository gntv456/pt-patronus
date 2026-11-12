// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package mediaserver

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"
)

// plexClient 对接 Plex Media Server。鉴权用 X-Plex-Token 头（或 query ?X-Plex-Token=），
// 端点前缀 /library/sections；注意 Plex 多用 GET 触发扫描。
type plexClient struct {
	server  Server
	hc      httpDoer
	apiRoot string
}

func (c *plexClient) headers() map[string]string {
	return map[string]string{
		"X-Plex-Token": c.server.Token,
		"Accept":       "application/json",
	}
}

// Test: GET / → {MediaContainer:{friendlyName, version}}。
func (c *plexClient) Test(ctx context.Context) (string, error) {
	var res struct {
		MediaContainer struct {
			FriendlyName string `json:"friendlyName"`
			Version      string `json:"version"`
		} `json:"MediaContainer"`
	}
	if err := jsonGet(ctx, c.hc, c.apiRoot+"/", c.headers(), &res); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s", res.MediaContainer.FriendlyName, res.MediaContainer.Version), nil
}

// Libraries: GET /library/sections → {MediaContainer:{Directory:[{Key,Title,Type}]}}。
// Plex 的 Key 形如 "/library/sections/2"，库 id 取末段数字作为统一标识。
func (c *plexClient) Libraries(ctx context.Context) ([]Library, error) {
	var res struct {
		MediaContainer struct {
			Directory []struct {
				Key   string `json:"key"`
				Title string `json:"title"`
				Type  string `json:"type"`
			} `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := jsonGet(ctx, c.hc, c.apiRoot+"/library/sections", c.headers(), &res); err != nil {
		return nil, err
	}
	out := make([]Library, 0, len(res.MediaContainer.Directory))
	for _, d := range res.MediaContainer.Directory {
		out = append(out, Library{ID: plexSectionID(d.Key), Name: d.Title, Type: plexLibType(d.Type)})
	}
	return out, nil
}

// LibraryScan: GET /library/sections/<id>/refresh（Plex 用 GET 触发扫描）。
func (c *plexClient) LibraryScan(ctx context.Context, libraryID string) error {
	u := fmt.Sprintf("%s/library/sections/%s/refresh", c.apiRoot, esc(libraryID))
	return jsonGet(ctx, c.hc, u, c.headers(), nil)
}

// Refresh: 与 LibraryScan 同义（Plex 整库刷新即扫描）。
func (c *plexClient) Refresh(ctx context.Context, libraryID string) error {
	return c.LibraryScan(ctx, libraryID)
}

// Notify: Plex 无公开的管理员通知 REST（需 Plex Pass 且走通知代理），明确返回不支持。
func (c *plexClient) Notify(ctx context.Context, title, body string) error {
	return errors.New("Plex 暂不支持服务端通知（无对应 REST 端点）")
}

// RecentItems: GET /library/sections/<id>/all?sort=addedAt:desc。
// 条目 id=RatingKey；封面 URL 预拼为 /library/metadata/<ratingkey>/poster（带 token 头）。
func (c *plexClient) RecentItems(ctx context.Context, libraryID string, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 9
	}
	u := fmt.Sprintf("%s/library/sections/%s/all?sort=addedAt:desc&X-Plex-Container-Start=0&X-Plex-Container-Size=%d",
		c.apiRoot, esc(libraryID), limit)
	var res struct {
		MediaContainer struct {
			Metadata []struct {
				RatingKey string `json:"ratingKey"`
				Title     string `json:"title"`
			} `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	if err := jsonGet(ctx, c.hc, u, c.headers(), &res); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(res.MediaContainer.Metadata))
	for _, m := range res.MediaContainer.Metadata {
		out = append(out, Item{
			ID:       m.RatingKey,
			Name:     m.Title,
			ImageURL: fmt.Sprintf("%s/library/metadata/%s/poster", c.apiRoot, esc(m.RatingKey)),
		})
	}
	return out, nil
}

// FetchImage 拉海报（带 X-Plex-Token 头），解码。
func (c *plexClient) FetchImage(ctx context.Context, item Item) (image.Image, error) {
	return decodeImage(ctx, c.hc, item.ImageURL, c.headers())
}

// Users: GET /accounts → {MediaContainer:{Account:[{id,name]}]}。Plex 账户即用户。
// 完整播放历史需 Plex Pass（/history 端点），此处仅返回账户列表，PlayCount 留 0。
func (c *plexClient) Users(ctx context.Context) ([]User, error) {
	var res struct {
		MediaContainer struct {
			Account []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"Account"`
		} `json:"MediaContainer"`
	}
	if err := jsonGet(ctx, c.hc, c.apiRoot+"/accounts", c.headers(), &res); err != nil {
		return nil, err
	}
	out := make([]User, 0, len(res.MediaContainer.Account))
	for _, a := range res.MediaContainer.Account {
		if a.Name == "" {
			continue
		}
		out = append(out, User{ID: plexAccountKey(a.ID), Name: a.Name})
	}
	return out, nil
}

// PlayedItems: GET /status/sessions → 当前活跃播放，按 userID 过滤（Plex 无公开历史端点，需 Plex Pass）。
// 返回该用户当前在线会话作为「最近播放」的近似；完整历史留 TODO 联调（Plex Pass /history）。
//
// 注意：必须按 userID 过滤——否则每个用户的调用都拿到全量活跃会话，观影报告会把同一会话
// 记给所有用户（U×S 重复计数）。活跃会话无 lastPlayed，统一置为当下，便于按时间排序与窗口过滤。
func (c *plexClient) PlayedItems(ctx context.Context, userID string, limit int) ([]PlayItem, error) {
	if limit <= 0 {
		limit = 50
	}
	var res struct {
		MediaContainer struct {
			Video []struct {
				GrandparentTitle string `json:"grandparentTitle"`
				Title            string `json:"title"`
				Year             int    `json:"year"`
				ViewOffset       int64  `json:"viewOffset"` // ms
				Duration         int64  `json:"duration"`   // ms
				User             struct {
					ID    int    `json:"id"`
					Title string `json:"title"`
				} `json:"User"`
				RatingKey string `json:"ratingKey"`
			} `json:"Video"`
		} `json:"MediaContainer"`
	}
	if err := jsonGet(ctx, c.hc, c.apiRoot+"/status/sessions", c.headers(), &res); err != nil {
		return nil, err
	}
	now := time.Now().Format(time.RFC3339)
	out := make([]PlayItem, 0, len(res.MediaContainer.Video))
	for _, v := range res.MediaContainer.Video {
		// 仅归属该用户的活跃会话（防止跨用户重复计数）。
		if plexAccountKey(v.User.ID) != userID {
			continue
		}
		name := v.GrandparentTitle
		if name == "" {
			name = v.Title
		}
		out = append(out, PlayItem{
			ItemID: v.RatingKey, Name: name, Type: "episode", Year: v.Year,
			UserID: userID, UserName: v.User.Title,
			Duration: int(v.Duration / 1000), PlayCount: 1, LastPlayed: now,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// LibraryItems: GET /library/sections/<id>/all → 解析 Metadata 的 Guid（tmdb/imdb）、year、title。
// Plex Guid 形如 "tmdb:123"、"imdb:tt1234567"，逐项提取。
func (c *plexClient) LibraryItems(ctx context.Context, libraryID string) ([]DetailedItem, error) {
	u := fmt.Sprintf("%s/library/sections/%s/all", c.apiRoot, esc(libraryID))
	var res struct {
		MediaContainer struct {
			Metadata []struct {
				RatingKey string `json:"ratingKey"`
				Title     string `json:"title"`
				Type      string `json:"type"`
				Year      int    `json:"year"`
				Guids     []struct {
					ID string `json:"id"`
				} `json:"Guid"`
			} `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	if err := jsonGet(ctx, c.hc, u, c.headers(), &res); err != nil {
		return nil, err
	}
	out := make([]DetailedItem, 0, len(res.MediaContainer.Metadata))
	for _, m := range res.MediaContainer.Metadata {
		di := DetailedItem{
			ItemID: m.RatingKey, Name: m.Title,
			Type: plexItemType(m.Type), Year: m.Year,
		}
		for _, g := range m.Guids {
			// Plex 新版 Guid 形如 "tmdb://438631"（旧版 "tmdb:438631"）；统一剥前缀与多余的 "//"。
			switch {
			case strings.HasPrefix(g.ID, "tmdb"):
				di.TMDBID = strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(g.ID, "tmdb:"), "tmdb://"), "/")
			case strings.HasPrefix(g.ID, "imdb"):
				di.IMDbID = strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(g.ID, "imdb:"), "imdb://"), "/")
			}
		}
		out = append(out, di)
	}
	return out, nil
}

// plexAccountKey 把数字账户 id 转字符串（统一 ID 类型为 string）。
func plexAccountKey(id int) string { return strconv.Itoa(id) }

// LibraryItemCount: GET /library/sections/<id>/all
// ?X-Plex-Container-Start=0&X-Plex-Container-Size=0
// —— Size=0 时 Plex 不返回 Metadata、MediaContainer 仍有 totalSize，比 LibraryItems
// 整库拉条目轻几个数量级（工作台 60s 轮询在用）。Plex 的 section 计数按库类型整体
// 给出（movie 库全是电影、show 库全是剧集），按 lib.Type 归类；mixed 归 Movies。
func (c *plexClient) LibraryItemCount(
	ctx context.Context, lib Library,
) (LibraryCount, error) {
	u := fmt.Sprintf(
		"%s/library/sections/%s/all?X-Plex-Container-Start=0&X-Plex-Container-Size=0",
		c.apiRoot, esc(lib.ID))
	var res struct {
		MediaContainer struct {
			TotalSize int `json:"totalSize"`
		} `json:"MediaContainer"`
	}
	if err := jsonGet(ctx, c.hc, u, c.headers(), &res); err != nil {
		return LibraryCount{}, err
	}
	if lib.Type == "tvshows" {
		return LibraryCount{Series: res.MediaContainer.TotalSize}, nil
	}
	return LibraryCount{Movies: res.MediaContainer.TotalSize}, nil
}

// EpisodeCount 剧集已入库集数（健康中心缺集判定用）。
// GET /library/metadata/<ratingkey>/allChildren → leafCount 即全部集数；
// 兜底再拉 children 计数（leafCount 缺失的旧版本）。
func (c *plexClient) EpisodeCount(ctx context.Context, seriesID string) (int, error) {
	u := fmt.Sprintf("%s/library/metadata/%s/allChildren?includeChildren=1", c.apiRoot, esc(seriesID))
	var res struct {
		MediaContainer struct {
			LeafCount int `json:"leafCount"`
			Size      int `json:"size"`
		} `json:"MediaContainer"`
	}
	if err := jsonGet(ctx, c.hc, u, c.headers(), &res); err != nil {
		return 0, err
	}
	if res.MediaContainer.LeafCount > 0 {
		return res.MediaContainer.LeafCount, nil
	}
	return res.MediaContainer.Size, nil
}

// plexItemType 把 Plex metadata type（movie/show/episode）归一。
func plexItemType(t string) string {
	switch t {
	case "movie":
		return "movie"
	case "show":
		return "series"
	case "episode":
		return "episode"
	}
	return t
}

// plexSectionID 从 Plex Key（"/library/sections/2"）取末段作统一库 id。
func plexSectionID(key string) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '/' {
			return key[i+1:]
		}
	}
	return key
}

// plexLibType 把 Plex section type 映射到统一类型名。
func plexLibType(t string) string {
	switch t {
	case "movie":
		return "movies"
	case "show":
		return "tvshows"
	case "artist":
		return "music"
	}
	return t
}

var _ Client = (*plexClient)(nil)
