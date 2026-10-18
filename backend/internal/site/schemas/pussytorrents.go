package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

func fetchPussyTorrentsUserInfo(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
	u := &site.UserInfo{}
	root, resp, err := fetchHTMLDoc(ctx, hc, baseURL, "/", cookie)
	if err != nil {
		// 首页都没读到就返空结构当成功，等于把站点故障报成「该号 0 上传 0 做种」。
		return nil, err
	}
	if loggedOut(resp, root, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	site.ParseUserInfoFields(root, def.UserInfo.Fields, u)
	profileKey := u.ID
	if profileKey == "" {
		profileKey = u.Name
	}
	if profileKey == "" {
		if userInfoParsesEmpty(u) {
			return nil, emptyUserInfoErr(baseURL, "首页")
		}
		return u, nil
	}
	profile, _, err := fetchHTMLDoc(ctx, hc, baseURL, "/profile/"+url.PathEscape(profileKey), cookie)
	if err == nil {
		site.ParseUserInfoFields(profile, def.UserInfo.Fields, u)
		userID := strings.TrimSpace(profile.Find(`a[href="#snatchlist"][data-ajax="/user/account/snatchlist"]`).First().AttrOr("data-userid", ""))
		if userID != "" {
			if c, sz := fetchPussyTorrentsSeeding(ctx, hc, baseURL, cookie, userID); sz > 0 {
				u.Seeding = c
				u.SeedingSize = sz
			}
			if uploads := fetchPussyTorrentsUploads(ctx, hc, baseURL, cookie, userID); uploads > 0 {
				u.Uploads = uploads
			}
		}
	}
	if userInfoParsesEmpty(u) {
		return nil, emptyUserInfoErr(baseURL, "首页与资料页")
	}
	return u, nil
}

func fetchPussyTorrentsSeeding(ctx context.Context, hc *httpclient.Client, baseURL, cookie, userID string) (int, int64) {
	form := pussyTorrentsDataTablesForm(userID, 9, -1)
	resp, err := hc.PostFormAjax(ctx, joinURL(baseURL, "/user/account/snatchlist"), cookie, form.Encode(), joinURL(baseURL, "/user/account/snatchlist"), true)
	if err != nil || resp.IsCloudflare() {
		return 0, 0
	}
	var body struct {
		Rows [][]any `json:"aaData"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return 0, 0
	}
	var count int
	var size int64
	for _, row := range body.Rows {
		if len(row) < 9 || !pussyTorrentsCompletedAndActive(row[7], row[8]) {
			continue
		}
		count++
		if s, ok := row[1].(string); ok {
			size += site.ParseBytes(s)
		}
	}
	return count, size
}

func fetchPussyTorrentsUploads(ctx context.Context, hc *httpclient.Client, baseURL, cookie, userID string) int {
	form := pussyTorrentsDataTablesForm(userID, 5, 50)
	resp, err := hc.PostFormAjax(ctx, joinURL(baseURL, "/user/account/uploadedtorrents"), cookie, form.Encode(), joinURL(baseURL, "/user/account/uploadedtorrents"), true)
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

func pussyTorrentsDataTablesForm(userID string, columns, length int) url.Values {
	v := url.Values{
		"sEcho":          {"1"},
		"iColumns":       {strconv.Itoa(columns)},
		"sColumns":       {""},
		"iDisplayStart":  {"0"},
		"iDisplayLength": {strconv.Itoa(length)},
		"sSearch":        {""},
		"bRegex":         {"false"},
		"iSortingCols":   {"1"},
		"iSortCol_0":     {"0"},
		"sSortDir_0":     {"desc"},
		"userID":         {userID},
	}
	for i := 0; i < columns; i++ {
		idx := strconv.Itoa(i)
		v.Set("sSearch_"+idx, "")
		v.Set("bRegex_"+idx, "false")
		v.Set("bSearchable_"+idx, "true")
		v.Set("bSortable_"+idx, "true")
	}
	return v
}

func pussyTorrentsCompletedAndActive(completed, active any) bool {
	okCompleted := false
	switch v := completed.(type) {
	case float64:
		okCompleted = v == 1
	case string:
		okCompleted = strings.TrimSpace(v) == "1"
	}
	return okCompleted && strings.EqualFold(strings.TrimSpace(toString(active)), "yes")
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
