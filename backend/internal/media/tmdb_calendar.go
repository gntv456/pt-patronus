// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package media

// TMDB 日历：按天聚合剧集播出（最近日子/订阅追更两条路径）。
// 从 tmdb.go 迁出。

import (
	"context"
	"strconv"
	"time"
)

// calendarWindow 日历取片的起止日（YYYY-MM-DD）。
// 按本地日历而不是 UTC：全站「今天」的口径都是本地时间（签到/通知按日去重、工作台的今日/昨日），
// 日历若改用 UTC，UTC+8 的实例每天 00:00–08:00 之间窗口起点仍停在昨天，
// 用户早上打开「新片日历」看到的是一整天前的片单，且与工作台对不上。
func calendarWindow(now time.Time, days int) (string, string) {
	return now.Format("2006-01-02"), now.AddDate(0, 0, days).Format("2006-01-02")
}

// 首播/上映」；对已开播剧集的后续单集更新，订阅追更链路（SubscribeChain）已覆盖。
func (t *TMDB) Calendar(ctx context.Context, kind string, days int) (map[string][]CalendarEpisode, error) {
	if t.apiKey == "" {
		return nil, ErrNotConfigured
	}
	if days <= 0 || days > 30 {
		days = 14
	}
	today, end := calendarWindow(time.Now(), days)
	params := map[string]string{
		"sort_by": "popularity.desc",
	}
	path := "/discover/movie"
	if kind == "tv" {
		path = "/discover/tv"
		params["first_air_date.gte"] = today
		params["first_air_date.lte"] = end
	} else {
		params["primary_release_date.gte"] = today
		params["primary_release_date.lte"] = end
	}
	params["page"] = "1"
	var r tmdbMultiResult
	if err := t.get(ctx, path, params, &r); err != nil {
		return nil, err
	}
	out := map[string][]CalendarEpisode{}
	for _, it := range r.Results {
		date := it.Release
		title := it.Title
		if kind == "tv" {
			date = it.FirstAir
			title = it.Name
		}
		if len(date) < 10 {
			continue
		}
		id := strconv.FormatInt(it.ID, 10)
		prefix := "movie"
		if kind == "tv" {
			prefix = "tv"
		}
		out[date[:10]] = append(out[date[:10]], CalendarEpisode{
			TMDBID: prefix + ":" + id, Title: title,
			AirDate: date[:10], Overview: it.Overview,
			PosterURL: tmdbImage(it.PosterPath),
		})
	}
	return out, nil
}

// CalendarRange 订阅日历：给定订阅的 TMDB tv id 集合，返回未来 N 天有单集开播的日程。
// 走 /tv/{id} detail 的 next_episode_to_air + last_episode_to_air（单集粒度），
// 供「我的订阅日历」视图（区别于全量 discover 的作品级日历）。
func (t *TMDB) CalendarForSubscriptions(ctx context.Context, tvIDs []string, days int) (map[string][]CalendarEpisode, error) {
	if t.apiKey == "" {
		return nil, ErrNotConfigured
	}
	if days <= 0 || days > 60 {
		days = 30
	}
	_, limit := calendarWindow(time.Now(), days)
	out := map[string][]CalendarEpisode{}
	type detailRes struct {
		Name       string `json:"name"`
		PosterPath string `json:"poster_path"`
		Next       struct {
			SeasonNumber int    `json:"season_number"`
			EpisodeNum   int    `json:"episode_number"`
			AirDate      string `json:"air_date"`
			Overview     string `json:"overview"`
			StillPath    string `json:"still_path"`
		} `json:"next_episode_to_air"`
	}
	for _, id := range tvIDs {
		if id == "" {
			continue
		}
		var d detailRes
		if err := t.get(ctx, "/tv/"+id, nil, &d); err != nil {
			continue // 单个失败跳过
		}
		e := d.Next
		if len(e.AirDate) < 10 || e.AirDate > limit {
			continue
		}
		// 海报兜底：剧集主海报优先，缺失退单集剧照（TMDB 竖版/横版都能展示）。
		img := tmdbImage(d.PosterPath)
		if img == "" {
			img = tmdbImage(e.StillPath)
		}
		out[e.AirDate[:10]] = append(out[e.AirDate[:10]], CalendarEpisode{
			TMDBID: "tv:" + id, Title: d.Name,
			Season: e.SeasonNumber, Episode: e.EpisodeNum,
			AirDate: e.AirDate[:10], Overview: e.Overview,
			PosterURL: img,
		})
	}
	return out, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
