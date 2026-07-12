package mediaserver

import (
	"context"
	"errors"
	"fmt"
	"image"
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
