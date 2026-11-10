// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

var torrentLeechUserIDRe = regexp.MustCompile(`userLogUserID\s*=\s*['"](\d+)['"]`)

func fetchTorrentLeechUserInfo(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
	u := &site.UserInfo{}
	root, resp, err := fetchHTMLDoc(ctx, hc, baseURL, "/", cookie)
	if err != nil {
		// 首页都没读到就返空结构当成功，等于把站点故障报成「该号 0 上传 0 做种」。
		return nil, err
	}
	if loggedOut(resp, root, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	top := root.Find("span.centerTopBar")
	u.Name = strings.TrimSpace(top.Find(`span[onclick*="/profile/"][onclick*="view"]`).First().Text())
	u.Uploaded = site.ParseBytes(top.Find(`div[title^="Uploaded"] span`).First().Text())
	u.Downloaded = site.ParseBytes(top.Find(`div[title^="Downloaded"] span`).First().Text())
	u.Bonus = site.ToFloat64(top.Find("span.total-TL-points").First().Text())

	if u.Name != "" {
		if doc, _, err := fetchHTMLDoc(ctx, hc, baseURL, "/profile/"+url.PathEscape(u.Name), cookie); err == nil {
			parseTorrentLeechProfile(doc, u)
		}
		if doc, _, err := fetchHTMLDoc(ctx, hc, baseURL, "/profile/"+url.PathEscape(u.Name)+"/seeding", cookie); err == nil {
			if c, sz := parseTorrentListSize(doc); sz > 0 {
				u.Seeding = c
				u.SeedingSize = sz
			}
		}
	}
	if u.ID != "" {
		if uploads := fetchTorrentLeechUploads(ctx, hc, baseURL, cookie, u.ID); uploads > 0 {
			u.Uploads = uploads
		}
	}
	if userInfoParsesEmpty(u) {
		return nil, emptyUserInfoErr(baseURL, "首页与资料页")
	}
	return u, nil
}

func parseTorrentLeechProfile(doc *goquery.Document, u *site.UserInfo) {
	if doc == nil || u == nil {
		return
	}
	if m := torrentLeechUserIDRe.FindStringSubmatch(doc.Text()); len(m) > 1 {
		u.ID = m[1]
	}
	u.Level = strings.TrimSpace(doc.Find("div.profile-details div.label-user-class").First().Text())
	joinRaw := strings.TrimSpace(doc.Find("table.profileViewTable td:contains('Registration date') + td").First().Text())
	u.JoinDate = parseTorrentLeechDate(joinRaw)
	lastRaw := strings.TrimSpace(doc.Find("table.profileViewTable td:contains('Last visit') + td").First().Text())
	u.LastAccessAt = parseTorrentLeechDateTime(lastRaw)
}

func fetchTorrentLeechUploads(ctx context.Context, hc *httpclient.Client, baseURL, cookie, userID string) int {
	form := url.Values{
		"sEcho":          {"1"},
		"iColumns":       {"6"},
		"sColumns":       {"categoryID,name,size,completed,seeders,leechers"},
		"iDisplayStart":  {"0"},
		"iDisplayLength": {"50"},
		"mDataProp_0":    {"0"},
		"mDataProp_1":    {"1"},
		"mDataProp_2":    {"2"},
		"mDataProp_3":    {"3"},
		"mDataProp_4":    {"4"},
		"mDataProp_5":    {"5"},
		"userID":         {userID},
	}
	resp, err := hc.PostFormAjax(ctx, joinURL(baseURL, "/user/account/uploadedtorrents"), cookie, form.Encode(), baseURL, true)
	if err != nil || resp.IsCloudflare() {
		return 0
	}
	var body struct {
		Total int `json:"iTotalRecords"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return 0
	}
	return body.Total
}

func parseTorrentLeechDate(raw string) string {
	raw = strings.TrimSpace(strings.Split(raw, " (")[0])
	if raw == "" {
		return ""
	}
	for _, layout := range []string{"Monday 2nd January 2006", "Monday 2 January 2006", "2 January 2006"} {
		if t, err := time.Parse(layout, removeOrdinalSuffix(raw)); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return raw
}

func parseTorrentLeechDateTime(raw string) string {
	raw = strings.TrimSpace(strings.Split(raw, " (")[0])
	if raw == "" {
		return ""
	}
	normalized := removeOrdinalSuffix(raw)
	for _, layout := range []string{
		"Monday 2 January 2006 03:04:05 PM",
		"Monday 2 January 2006 15:04:05",
		"2 January 2006 03:04:05 PM",
		"2 January 2006 15:04:05",
	} {
		if t, err := time.Parse(layout, normalized); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}
	if date := parseTorrentLeechDate(raw); date != raw {
		return date
	}
	return raw
}

func removeOrdinalSuffix(raw string) string {
	parts := strings.Fields(raw)
	for i, part := range parts {
		n := strings.TrimRight(part, "stndrh")
		if _, err := strconv.Atoi(n); err == nil {
			parts[i] = n
			break
		}
	}
	return strings.Join(parts, " ")
}
