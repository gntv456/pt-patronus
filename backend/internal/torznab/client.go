// Package torznab 提供 Jackett/Prowlarr Torznab 搜索客户端（辅种候选源）。
// 协议：GET {base}/api?t=search&q=...&apikey=... 返回 XML <item> 列表。
package torznab

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Result 是一条 Torznab 搜索结果。
type Result struct {
	Title       string
	Size        int64
	Link        string // 下载链
	Guid        string
	ImdbID      string
	TmdbID      string
	Seeders     int
	Indexer     string
	PublishDate time.Time
}

// Client 调 Torznab 端点。
type Client struct {
	HTTP    *http.Client
	BaseURL string
	APIKey  string
}

// Search 按关键字搜索；imdbID/tmdbID 非空时附带对应参数（Jackett/Prowlarr 支持）。
func (c *Client) Search(ctx context.Context, query, imdbID, tmdbID string) ([]Result, error) {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" {
		return nil, fmt.Errorf("torznab base url empty")
	}
	base := strings.TrimRight(c.BaseURL, "/")
	u, err := url.Parse(base + "/api")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("t", "search")
	if c.APIKey != "" {
		q.Set("apikey", c.APIKey)
	}
	if imdbID != "" {
		id := strings.TrimPrefix(strings.TrimSpace(imdbID), "tt")
		q.Set("imdbid", id)
	}
	if tmdbID != "" {
		id := strings.TrimSpace(tmdbID)
		for _, pfx := range []string{"movie:", "tv:", "tmdb:"} {
			id = strings.TrimPrefix(strings.ToLower(id), pfx)
		}
		// 仅数字
		digits := make([]rune, 0, len(id))
		for _, r := range id {
			if r >= '0' && r <= '9' {
				digits = append(digits, r)
			}
		}
		if len(digits) > 0 {
			q.Set("tmdbid", string(digits))
		}
	}
	if query != "" {
		q.Set("q", query)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 25 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("torznab http %d: %s", resp.StatusCode, string(b))
	}
	return parseRSS(resp.Body)
}

type rss struct {
	Channel channel `xml:"channel"`
}
type channel struct {
	Items []item `xml:"item"`
}
type item struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	Guid    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
	// Torznab attrs
	Attrs []attr `xml:"http://torznab.com/schemas/2015/feed attr"`
	// enclosure
	Enclosure *struct {
		URL  string `xml:"url,attr"`
		Len  string `xml:"length,attr"`
		Type string `xml:"type,attr"`
	} `xml:"enclosure"`
}
type attr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

func parseRSS(r io.Reader) ([]Result, error) {
	var doc rss
	dec := xml.NewDecoder(r)
	// Torznab often uses default namespace; Decoder handles most feeds without Strict.
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(doc.Channel.Items))
	for _, it := range doc.Channel.Items {
		res := Result{
			Title: strings.TrimSpace(it.Title),
			Guid:  strings.TrimSpace(it.Guid),
			Link:  strings.TrimSpace(it.Link),
		}
		if it.Enclosure != nil {
			if it.Enclosure.URL != "" {
				res.Link = it.Enclosure.URL
			}
			if n, err := strconv.ParseInt(it.Enclosure.Len, 10, 64); err == nil {
				res.Size = n
			}
		}
		for _, a := range it.Attrs {
			switch strings.ToLower(a.Name) {
			case "size":
				if n, err := strconv.ParseInt(a.Value, 10, 64); err == nil {
					res.Size = n
				}
			case "seeders":
				if n, err := strconv.Atoi(a.Value); err == nil {
					res.Seeders = n
				}
			case "imdb", "imdbid":
				res.ImdbID = a.Value
			case "tmdb", "tmdbid":
				res.TmdbID = a.Value
			case "magneturl":
				if res.Link == "" {
					res.Link = a.Value
				}
			}
		}
		if res.Title == "" || res.Link == "" {
			continue
		}
		out = append(out, res)
	}
	return out, nil
}

// ExtractImdbID 从标题中粗提取 tt\d{7,8}。
func ExtractImdbID(title string) string {
	low := strings.ToLower(title)
	i := strings.Index(low, "tt")
	for i >= 0 && i+9 <= len(low) {
		cand := low[i : i+9]
		ok := true
		for j := 2; j < 9; j++ {
			if cand[j] < '0' || cand[j] > '9' {
				ok = false
				break
			}
		}
		if ok {
			return cand
		}
		next := strings.Index(low[i+2:], "tt")
		if next < 0 {
			break
		}
		i = i + 2 + next
	}
	return ""
}
