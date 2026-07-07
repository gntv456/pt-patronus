package schemas

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/site"
)

const skyeySnowMaxPages = 100

func fetchSkyeySnowUserInfo(ctx context.Context, a *BaseHTMLAdapter, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
	u := &site.UserInfo{}
	if a == nil {
		return u, nil
	}
	if len(def.UserInfo.Pages) > 0 {
		if err := fetchUserInfoPages(ctx, a, def, baseURL, cookie, u); err != nil {
			return nil, err
		}
	}
	if u.ID == "" {
		return u, nil
	}
	if seeding, seedingSize := fetchSkyeySnowSeeding(ctx, a, def, baseURL, cookie); seeding > 0 || seedingSize > 0 {
		u.Seeding = seeding
		u.SeedingSize = seedingSize
	}
	if uploads := fetchSkyeySnowUploads(ctx, a, def, baseURL, cookie, u.ID); uploads > 0 {
		u.Uploads = uploads
	}
	return u, nil
}

func fetchSkyeySnowSeeding(ctx context.Context, a *BaseHTMLAdapter, def *site.SiteMetadata, baseURL, cookie string) (count int, size int64) {
	maxPage := 0
	for page := 0; page <= maxPage && page <= skyeySnowMaxPages; page++ {
		doc, _, err := a.fetchDoc(ctx, baseURL, skyeySnowForumPath(url.Values{
			"mod":     {"torrents"},
			"cat_5up": {"on"},
			"page":    {strconv.Itoa(page)},
		}), cookie)
		if err != nil {
			break
		}
		if maxPage == 0 {
			maxPage = skyeySnowMaxPage(doc)
		}
		torrents := site.ParseSearch(doc, def, baseURL)
		if len(torrents) == 0 {
			break
		}
		count += len(torrents)
		for _, torrent := range torrents {
			size += site.ParseBytes(torrent.Size)
		}
	}
	return count, size
}

func fetchSkyeySnowUploads(ctx context.Context, a *BaseHTMLAdapter, def *site.SiteMetadata, baseURL, cookie, uid string) int {
	doc, _, err := a.fetchDoc(ctx, baseURL, skyeySnowForumPath(url.Values{
		"mod":    {"torrents"},
		"search": {"#" + uid},
	}), cookie)
	if err != nil {
		return 0
	}
	if uploads := skyeySnowUploadCount(doc); uploads > 0 {
		return uploads
	}
	return len(site.ParseSearch(doc, def, baseURL))
}

func skyeySnowForumPath(q url.Values) string {
	return "/forum.php?" + q.Encode()
}

func skyeySnowMaxPage(doc *goquery.Document) int {
	maxPage := 0
	doc.Find(`a[onclick*="page="]`).Each(func(_ int, a *goquery.Selection) {
		if !strings.Contains(a.Text(), "-") {
			return
		}
		if onclick, ok := a.Attr("onclick"); ok {
			if n := pageNumberFromText(onclick); n > maxPage {
				maxPage = n
			}
		}
	})
	return maxPage
}

func skyeySnowUploadCount(doc *goquery.Document) int {
	best := 0
	doc.Find(`a[onclick*="page="]`).Each(func(_ int, a *goquery.Selection) {
		text := strings.TrimSpace(a.Text())
		if !strings.Contains(text, "-") {
			return
		}
		parts := strings.Split(text, "-")
		if len(parts) >= 2 {
			if n, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1])); err == nil && n > best {
				best = n
			}
		}
	})
	return best
}

func pageNumberFromText(raw string) int {
	idx := strings.LastIndex(raw, "page=")
	if idx < 0 {
		return 0
	}
	s := raw[idx+len("page="):]
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}
