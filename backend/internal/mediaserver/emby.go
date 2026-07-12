package mediaserver

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"net/url"
	"strings"
)

// embyJellyfinClient 兼容 Emby 与 Jellyfin（JF fork 自 Emby，REST 路径 / 鉴权一致）。
// 鉴权用 X-Emby-Token 头（API Key），两家均识别；封面直链同样带该头。
type embyJellyfinClient struct {
	server  Server
	hc      httpDoer
	apiRoot string
}

func (c *embyJellyfinClient) headers() map[string]string {
	return map[string]string{
		"X-Emby-Token": c.server.Token,
		"Accept":       "application/json",
	}
}

// Test: GET /System/Info → {ServerName, Version}。
func (c *embyJellyfinClient) Test(ctx context.Context) (string, error) {
	var info struct {
		ServerName string `json:"ServerName"`
		Version    string `json:"Version"`
	}
	if err := jsonGet(ctx, c.hc, c.apiRoot+"/System/Info", c.headers(), &info); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s", info.ServerName, info.Version), nil
}

// Libraries: GET /Library/MediaFolders → {Items:[{Name,Id,CollectionType}]}。
func (c *embyJellyfinClient) Libraries(ctx context.Context) ([]Library, error) {
	var res struct {
		Items []struct {
			Name           string `json:"Name"`
			ID             string `json:"Id"`
			CollectionType string `json:"CollectionType"`
		} `json:"Items"`
	}
	if err := jsonGet(ctx, c.hc, c.apiRoot+"/Library/MediaFolders", c.headers(), &res); err != nil {
		return nil, err
	}
	out := make([]Library, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, Library{ID: it.ID, Name: it.Name, Type: it.CollectionType})
	}
	return out, nil
}

// LibraryScan: POST /Items/{id}/Refresh?Recursive=true 触发整库扫描。
func (c *embyJellyfinClient) LibraryScan(ctx context.Context, libraryID string) error {
	u := fmt.Sprintf("%s/Items/%s/Refresh?Recursive=true&MetadataRefreshMode=Default&ImageRefreshMode=Default",
		c.apiRoot, esc(libraryID))
	return jsonPost(ctx, c.hc, u, nil, c.headers(), nil)
}

// Refresh: 与 LibraryScan 同义（Emby/JF 无独立「刷新元数据」端点，统一走 Refresh）。
func (c *embyJellyfinClient) Refresh(ctx context.Context, libraryID string) error {
	return c.LibraryScan(ctx, libraryID)
}

// Notify: POST /Notifications/Admin {Name,Description}。
func (c *embyJellyfinClient) Notify(ctx context.Context, title, body string) error {
	payload, _ := json.Marshal(map[string]string{"Name": title, "Description": body})
	return jsonPost(ctx, c.hc, c.apiRoot+"/Notifications/Admin", payload, c.headers(), nil)
}

// RecentItems: GET /Items?ParentId=<lib>&Recursive=true&Limit=N&SortBy=DateCreated&SortOrder=Descending。
// 封面 URL 预拼为 /Items/<id>/Images/Primary?fillWidth=400（鉴权走 X-Emby-Token 头）。
func (c *embyJellyfinClient) RecentItems(ctx context.Context, libraryID string, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 9
	}
	u := fmt.Sprintf("%s/Items?ParentId=%s&Recursive=true&Limit=%d&SortBy=DateCreated&SortOrder=Descending&IncludeItemTypes=Movie,Series",
		c.apiRoot, esc(libraryID), limit)
	var res struct {
		Items []struct {
			ID   string `json:"Id"`
			Name string `json:"Name"`
		} `json:"Items"`
	}
	if err := jsonGet(ctx, c.hc, u, c.headers(), &res); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, Item{
			ID:       it.ID,
			Name:     it.Name,
			ImageURL: fmt.Sprintf("%s/Items/%s/Images/Primary?fillWidth=400", c.apiRoot, esc(it.ID)),
		})
	}
	return out, nil
}

// FetchImage 拉 Primary 图（带 X-Emby-Token 头），解码为 image.Image。
func (c *embyJellyfinClient) FetchImage(ctx context.Context, item Item) (image.Image, error) {
	return decodeImage(ctx, c.hc, item.ImageURL, c.headers())
}

// Users: GET /Users → [{Id,Name}]。Emby/JF 无单端点聚合 PlayCount，留 0（由观影报告聚合）。
func (c *embyJellyfinClient) Users(ctx context.Context) ([]User, error) {
	var res []struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	}
	if err := jsonGet(ctx, c.hc, c.apiRoot+"/Users", c.headers(), &res); err != nil {
		return nil, err
	}
	out := make([]User, 0, len(res))
	for _, u := range res {
		if u.ID == "" {
			continue
		}
		out = append(out, User{ID: u.ID, Name: u.Name})
	}
	return out, nil
}

// PlayedItems: GET /Users/{uid}/Items?Filters=IsPlayed&SortBy=DatePlayed&SortOrder=Descending
// &Fields=ProductionYear,MediaSourceCount&IncludeItemTypes=Movie,Series,Episode。
// UserData 携带 PlayCount/LastPlayedDate/PlaybackPositionTicks。
func (c *embyJellyfinClient) PlayedItems(ctx context.Context, userID string, limit int) ([]PlayItem, error) {
	if limit <= 0 {
		limit = 50
	}
	u := fmt.Sprintf("%s/Users/%s/Items?Filters=IsPlayed&SortBy=DatePlayed&SortOrder=Descending"+
		"&Limit=%d&Fields=ProductionYear,MediaSourceCount&IncludeItemTypes=Movie,Series,Episode",
		c.apiRoot, esc(userID), limit)
	var res struct {
		Items []struct {
			ID    string `json:"Id"`
			Name  string `json:"Name"`
			Type  string `json:"Type"`
			Year  int    `json:"ProductionYear"`
			User  struct {
				PlayCount    int    `json:"PlayCount"`
				LastPlayed   string `json:"LastPlayedDate"`
				RuntimeTicks int64  `json:"RuntimeTicks"` // 10μs 单位
			} `json:"UserData"`
		} `json:"Items"`
	}
	if err := jsonGet(ctx, c.hc, u, c.headers(), &res); err != nil {
		return nil, err
	}
	out := make([]PlayItem, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, PlayItem{
			ItemID: it.ID, Name: it.Name, Type: embyItemType(it.Type), Year: it.Year,
			UserID: userID, LastPlayed: it.User.LastPlayed,
			PlayCount: it.User.PlayCount, Duration: int(it.User.RuntimeTicks / 10000000),
		})
	}
	return out, nil
}

// LibraryItems: GET /Items?ParentId=<lib>&Recursive=true&IncludeItemTypes=Movie,Series
// &Fields=ProviderIds,ProductionYear,Path。ProviderIds 形如 {"Tmdb":"123","Imdb":"tt123"}。
func (c *embyJellyfinClient) LibraryItems(ctx context.Context, libraryID string) ([]DetailedItem, error) {
	u := fmt.Sprintf("%s/Items?ParentId=%s&Recursive=true&IncludeItemTypes=Movie,Series"+
		"&Fields=ProviderIds,ProductionYear,Path", c.apiRoot, esc(libraryID))
	var res struct {
		Items []struct {
			ID   string `json:"Id"`
			Name string `json:"Name"`
			Type string `json:"Type"`
			Year int    `json:"ProductionYear"`
			Path string `json:"Path"`
			Prov struct {
				Tmdb string `json:"Tmdb"`
				Imdb string `json:"Imdb"`
			} `json:"ProviderIds"`
		} `json:"Items"`
	}
	if err := jsonGet(ctx, c.hc, u, c.headers(), &res); err != nil {
		return nil, err
	}
	out := make([]DetailedItem, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, DetailedItem{
			ItemID: it.ID, Name: it.Name, Type: embyItemType(it.Type),
			Year: it.Year, TMDBID: it.Prov.Tmdb, IMDbID: it.Prov.Imdb, Path: it.Path,
		})
	}
	return out, nil
}

// embyItemType 把 Emby 的 Type（Movie/Series/Episode/...）归一为 movie|series|episode。
func embyItemType(t string) string {
	switch t {
	case "Movie":
		return "movie"
	case "Series":
		return "series"
	case "Episode":
		return "episode"
	}
	return strings.ToLower(t)
}

// esc 转义路径段（库 id / 条目 id 通常是数字，但防御性转义）。
func esc(s string) string { return url.PathEscape(s) }

var _ Client = (*embyJellyfinClient)(nil)
