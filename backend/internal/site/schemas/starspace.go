// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

var starSpaceLevelMap = map[string]string{
	"class1":  "User",
	"class2":  "Power User",
	"class3":  "Elite User",
	"class4":  "Crazy User",
	"class5":  "Insane User",
	"class6":  "Veteran User",
	"class7":  "Extreme User",
	"class8":  "Ultimate User",
	"class9":  "Master User",
	"class10": "Star User",
	"class11": "God User",
}

func fetchStarSpaceUserInfo(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
	u := &site.UserInfo{}
	index, resp, err := fetchHTMLDoc(ctx, hc, baseURL, "/p_index/index.php", cookie)
	if err != nil {
		// 首页都没读到就返空结构当成功，等于把站点故障报成「该号 0 上传 0 做种」。
		return nil, err
	}
	if loggedOut(resp, index, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	link := index.Find(`span#user_info a[href*="user_detail.php"]`).First()
	u.Name = strings.TrimSpace(link.Text())
	if href, ok := link.Attr("href"); ok {
		if parsed, err := url.Parse(href); err == nil {
			u.ID = parsed.Query().Get("uid")
		}
	}
	if u.ID != "" {
		if doc, _, err := fetchHTMLDoc(ctx, hc, baseURL, "/p_user/user_detail.php?uid="+url.QueryEscape(u.ID), cookie); err == nil {
			parseStarSpaceDetail(doc, u)
		}
		if doc, _, err := fetchHTMLDoc(ctx, hc, baseURL, "/p_torrent/torrent_user.php?uid="+url.QueryEscape(u.ID)+"&pop=8", cookie); err == nil {
			u.Seeding, u.SeedingSize = parseStarSpaceTorrentTable(doc)
		}
		if doc, _, err := fetchHTMLDoc(ctx, hc, baseURL, "/p_torrent/torrent_user.php?uid="+url.QueryEscape(u.ID)+"&pop=7", cookie); err == nil {
			u.Uploads = parseStarSpaceUploads(doc)
		}
	}
	if doc, _, err := fetchHTMLDoc(ctx, hc, baseURL, "/p_bonus/bonus_hour.php", cookie); err == nil {
		u.BonusPerHour = site.ToFloat64(doc.Find("tr#tr_list td").Last().Text())
	}
	if userInfoParsesEmpty(u) {
		return nil, emptyUserInfoErr(baseURL, "首页与资料页")
	}
	return u, nil
}

func parseStarSpaceDetail(doc *goquery.Document, u *site.UserInfo) {
	doc.Find("tr").Each(func(_ int, row *goquery.Selection) {
		cells := row.ChildrenFiltered("td")
		if cells.Length() < 2 {
			return
		}
		label := strings.ToLower(strings.TrimSpace(cells.First().Text()))
		value := cells.Eq(1)
		switch {
		case containsAny(label, "level", "\u7528\u6237\u7b49\u7ea7", "\u7528\u6236\u7b49\u7d1a"):
			level := strings.TrimSpace(value.Find("img").First().AttrOr("alt", value.Text()))
			if mapped := starSpaceLevelMap[level]; mapped != "" {
				level = mapped
			}
			u.Level = level
		case containsAny(label, "join", "\u52a0\u5165\u65e5\u671f"):
			u.JoinDate = firstDate(value.Text())
		case containsAny(label, "\u6700\u8fd1\u8bbf\u95ee", "\u6700\u8fd1\u8a2a\u554f", "\u6700\u8fd1\u52a8\u5411", "\u6700\u8fd1\u52d5\u5411", "last access", "last action"):
			u.LastAccessAt = strings.TrimSpace(value.Text())
		case containsAny(label, "\u4fdd\u79cd\u79ef\u5206", "\u4fdd\u7a2e\u7a4d\u5206", "\u505a\u79cd\u79ef\u5206", "\u505a\u7a2e\u7a4d\u5206", "seeding bonus", "seed bonus"):
			u.SeedingBonus = site.ToFloat64(value.Text())
		case containsAny(label, "bonus", "\u9b54\u529b"):
			u.Bonus = site.ToFloat64(value.Text())
		case containsAny(label, "transfer", "\u4f20\u8f93", "\u50b3\u8f38"):
			stats := value.Find("tr#tr_item_min td")
			if stats.Length() >= 4 {
				u.Uploaded = starSpaceParseSize(stats.Eq(0).Text())
				u.Downloaded = starSpaceParseSize(stats.Eq(2).Text())
				u.Ratio = site.ToFloat64(stats.Eq(3).Text())
			}
			if stats.Length() >= 7 {
				u.TrueUploaded = starSpaceParseSize(stats.Eq(4).Text())
				u.TrueDownloaded = starSpaceParseSize(stats.Eq(5).Text())
				u.TrueRatio = site.ToFloat64(stats.Eq(6).Text())
			}
		}
	})
}

func parseStarSpaceTorrentTable(doc *goquery.Document) (count int, size int64) {
	doc.Find("tr[class^='tm_tr'], tr[class*='tm_tr']").Each(func(_ int, row *goquery.Selection) {
		cell := row.Find("td#tm_td_size").First()
		if cell.Length() == 0 {
			return
		}
		if sz := starSpaceParseSize(cell.Text()); sz > 0 {
			count++
			size += sz
		}
	})
	return count, size
}

func parseStarSpaceUploads(doc *goquery.Document) int {
	if n := site.ToInt(doc.Find("#table_pager span.pager_current").First().Text()); n > 0 {
		return n
	}
	return doc.Find("tr[class^='tm_tr'], tr[class*='tm_tr']").Length()
}

func starSpaceParseSize(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if sizeTokenRe.MatchString(raw) {
		return site.ParseBytes(raw)
	}
	return site.ParseBytes(raw + "B")
}

func containsAny(s string, needles ...string) bool {
	s = strings.ToLower(s)
	for _, needle := range needles {
		if strings.Contains(s, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func firstDate(raw string) string {
	raw = strings.TrimSpace(raw)
	for _, token := range strings.Fields(raw) {
		if len(token) >= 10 && token[4] == '-' && token[7] == '-' {
			return token[:10]
		}
	}
	return raw
}
