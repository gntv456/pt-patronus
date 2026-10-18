package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
)

func (g *GenericJSON) FetchTorrentDetail(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, detailURL string) (*site.TorrentDetail, error) {

	if def.ID != "sunnypt" {
		return nil, fmt.Errorf("sunnypt: detail not supported for site %s", def.ID)
	}
	m := sunnyTorrentIDRe.FindStringSubmatch(detailURL)
	if len(m) < 2 {
		return nil, fmt.Errorf("sunnypt: 无法从 %s 提取种子 id", detailURL)
	}
	apiBase := sunnyAPIBase(baseURL)
	body, st, err := g.sunnyGet(ctx, apiBase, cookie, "/api/v1/torrent/detail?id="+m[1])
	if err != nil {
		return nil, err
	}
	rb := string(body)
	if len(rb) > 12288 {
		rb = rb[:12288]
	}
	slog.Debug("sunnypt detail raw", "id", m[1], "len", len(body), "body", rb)

	data, err := sunnyData(body, st)
	if err != nil {
		return nil, err
	}
	var d map[string]any
	if json.Unmarshal(data, &d) != nil {
		return &site.TorrentDetail{DetailURL: detailURL}, nil
	}
	td := &site.TorrentDetail{
		DetailURL:   detailURL,
		Title:       sunnyMapStr(d, "name", "title", "subject", "fullname"),
		SubTitle:    sunnyMapStr(d, "subName", "subtitle", "small_desc", "smallDescr"),
		Description: sunnyMapStr(d, "description", "body", "content", "desc"),
		MediaInfo:   sunnyMapStr(d, "technical_info", "technicalInfo", "mediaInfo"),
		PosterURL:   sunnyAbsURL(sunnyMapStr(d, "cover", "poster", "poster_url")),
		Category:    sunnyMapStr(d, "category", "cat"),
		Codec:       sunnyMapStr(d, "video_codec", "codec"),
		Resolution:  sunnyMapStr(d, "resolution"),
		Source:      sunnyMapStr(d, "medium", "source"),
	}
	td.IMDBID = sunnyIDFromURL(sunnyMapStr(d, "imdb_url", "imdb"))
	td.DoubanID = sunnyIDFromURL(sunnyMapStr(d, "douban_url", "douban"))
	td.TMDBID = sunnyIDFromURL(sunnyMapStr(d, "tmdb_url", "tmdb"))
	if sz := sunnyMapStr(d, "size", "total_size", "file_size"); sz != "" {
		td.Size = sz
	} else if n, ok := sunnyMapNum(d, "size", "total_size", "file_size"); ok {
		td.Size = site.FormatBytes(int64(n))
	}
	if ss := sunnyMapStrSlice(d, "screenshot", "screenshots", "images"); len(ss) > 0 {
		td.Screenshots = sunnyAbsURLs(ss)
	} else if one := sunnyMapStr(d, "screenshot", "screenshots"); one != "" {
		// sunnypt 的 screenshot 是逗号分隔的字符串,拆成多条。
		td.Screenshots = sunnyAbsURLs(strings.Split(one, ","))
	}
	if n, ok := sunnyMapNum(d, "seeders"); ok {
		td.Seeders = int(n)
	}
	if n, ok := sunnyMapNum(d, "leechers"); ok {
		td.Leechers = int(n)
	}
	return td, nil
}
