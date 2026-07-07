package schemas

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

var (
	zhixingIDRe       = regexp.MustCompile(`/user/([^/?#]+)`)
	zhixingDateTimeRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}(?:[ T]\d{2}:\d{2}:\d{2})?`)
)

func fetchZhixingUserInfo(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
	u := &site.UserInfo{}
	root, resp, err := fetchTorrentLeechDoc(ctx, hc, baseURL, "/", cookie)
	if err != nil {
		return u, nil
	}
	if loggedOut(resp, root, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	link := root.Find(`strong.vwmy > a[href*="/user/"]`).First()
	u.Name = strings.TrimSpace(link.Text())
	if href, ok := link.Attr("href"); ok {
		if m := zhixingIDRe.FindStringSubmatch(href); len(m) > 1 {
			u.ID = m[1]
		}
	}
	if u.ID == "" {
		return u, nil
	}
	if doc, _, err := fetchTorrentLeechDoc(ctx, hc, baseURL, "/user/"+url.PathEscape(u.ID), cookie); err == nil {
		parseZhixingProfile(doc, u)
	}
	if c, sz := fetchZhixingTorrentTotals(ctx, hc, def, baseURL, cookie, u.ID, "seeding"); c > 0 || sz > 0 {
		u.Seeding = c
		u.SeedingSize = sz
	}
	if c, _ := fetchZhixingTorrentTotals(ctx, hc, def, baseURL, cookie, u.ID, "uploaded"); c > 0 {
		u.Uploads = c
	}
	return u, nil
}

func parseZhixingProfile(doc *goquery.Document, u *site.UserInfo) {
	doc.Find("p").Each(func(_ int, p *goquery.Selection) {
		text := strings.TrimSpace(p.Text())
		switch {
		case containsAny(text, "uploaded", "\u4e0a\u4f20\u6d41\u91cf"):
			u.Uploaded = site.ParseBytes(text)
		case containsAny(text, "downloaded", "\u4e0b\u8f7d\u6d41\u91cf"):
			u.Downloaded = site.ParseBytes(text)
		case containsAny(text, "ratio", "\u5171\u4eab\u7387"):
			u.Ratio = site.ToFloat64(text)
		case containsAny(text, "user group", "\u7528\u6237\u7ec4"):
			u.Level = strings.TrimSpace(afterAny(text, ":", "\uff1a"))
		case containsAny(text, "total bonus", "\u603b\u79ef\u5206"):
			u.Bonus = site.ToFloat64(text)
		case containsAny(text, "seeding bonus", "\u4fdd\u79cd\u79ef\u5206"):
			u.SeedingBonus = site.ToFloat64(text)
			if u.Bonus == 0 {
				u.Bonus = site.ToFloat64(text)
			}
		case containsAny(text, "join", "\u6ce8\u518c\u65f6\u95f4"):
			u.JoinDate = firstDate(afterAny(text, ":", "\uff1a"))
		case containsAny(text, "last ipv6 access", "last access", "\u4e0a\u6b21IPv6\u8bbf\u95ee\u65f6\u95f4", "\u4e0a\u6b21\u8bbf\u95ee\u65f6\u95f4"):
			u.LastAccessAt = firstDateTime(afterAny(text, ":", "\uff1a"))
		case containsAny(text, "current seeding count", "\u5f53\u524d\u4fdd\u79cd\u6570\u91cf"):
			u.Seeding = site.ToInt(text)
		case containsAny(text, "current seeding size", "\u5f53\u524d\u4fdd\u79cd\u5bb9\u91cf"):
			u.SeedingSize = site.ParseBytes(text)
		}
	})
}

func fetchZhixingTorrentTotals(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie, uid, typ string) (count int, size int64) {
	for page := 1; page <= maxSeedingPages; page++ {
		path := "/user/" + url.PathEscape(uid) + "/" + typ + "/p" + strconv.Itoa(page)
		doc, _, err := fetchTorrentLeechDoc(ctx, hc, baseURL, path, cookie)
		if err != nil {
			break
		}
		if def.Selectors.Rows.Selector != "" && doc.Find(def.Selectors.Rows.Selector).Length() == 0 {
			break
		}
		torrents := site.ParseSearch(doc, def, baseURL)
		if len(torrents) == 0 {
			break
		}
		count += len(torrents)
		for _, t := range torrents {
			size += site.ParseBytes(t.Size)
		}
	}
	return count, size
}

func afterAny(text string, seps ...string) string {
	for _, sep := range seps {
		if i := strings.Index(text, sep); i >= 0 {
			return text[i+len(sep):]
		}
	}
	return text
}

func firstDateTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if m := zhixingDateTimeRe.FindString(raw); m != "" {
		return strings.ReplaceAll(m, "T", " ")
	}
	return raw
}
