package schemas

import (
	"bytes"
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// fetchUploadsCount 抓 getusertorrentlistajax?type=uploaded 取「发布数」。
// 优先「<b>N</b>条记录」头部总数；缺失用汇总 div「N | SIZE」的 N；再缺失数种子详情行。best-effort。
func fetchUploadsCount(ctx context.Context, hc *httpclient.Client, baseURL, uid, cookie, listAPI string) int {
	for _, tpl := range userTorrentListTemplates(listAPI) {
		if n := fetchUploadsCountFromTemplate(ctx, hc, baseURL, uid, cookie, tpl); n > 0 {
			return n
		}
	}
	return 0
}

func fetchUploadsCountFromTemplate(ctx context.Context, hc *httpclient.Client, baseURL, uid, cookie, tpl string) int {
	u := userTorrentListURL(baseURL, tpl, uid, "uploaded", 0)
	resp, err := hc.GetWithHeaders(ctx, u, cookie, userTorrentListHeaders(baseURL, uid, tpl), true)
	if err != nil || resp.IsCloudflare() {
		return 0
	}
	body := string(resp.Body)
	if m := ajaxDataCountRe.FindStringSubmatch(body); len(m) > 1 {
		if n := site.ToInt(m[1]); n > 0 {
			return n
		}
	}
	if m := uploadsCountRe.FindStringSubmatch(body); len(m) > 1 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	// 回退 1：汇总 div「N | SIZE」的 N（数量在 | 前）
	if m := seedingSummaryRe.FindStringSubmatch(body); len(m) == 3 {
		if n := site.ToInt(m[1]); n > 0 {
			return n
		}
	}
	// 回退 2：数种子详情行
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return 0
	}
	return doc.Find(`table tr:has(a[href*="details.php"])`).Length()
}

func userTorrentListTemplates(listAPI string) []string {
	out := []string{}
	add := func(tpl string) {
		tpl = strings.TrimSpace(tpl)
		if tpl == "" {
			return
		}
		for _, old := range out {
			if old == tpl {
				return
			}
		}
		out = append(out, tpl)
	}
	add(listAPI)
	add("getusertorrentlistajax.php?userid={id}&type={type}")
	add("getusertorrentlist.php?userid={id}&type={type}")
	return out
}

func userTorrentListHeaders(baseURL, uid, tpl string) map[string]string {
	if uid == "" {
		return nil
	}
	param := "id"
	if strings.Contains(strings.ToLower(tpl), "useruuid") || strings.Contains(strings.ToLower(tpl), "uuid=") {
		param = "uuid"
	}
	return map[string]string{
		"Referer": joinURL(baseURL, "userdetails.php?"+param+"="+url.QueryEscape(uid)),
	}
}

func userTorrentListURL(baseURL, tpl, uid, typ string, page int) string {
	path := strings.ReplaceAll(tpl, "{id}", url.QueryEscape(uid))
	path = strings.ReplaceAll(path, "{uid}", url.QueryEscape(uid))
	path = strings.ReplaceAll(path, "{type}", url.QueryEscape(typ))
	if strings.Contains(path, "{page}") {
		if page <= 0 {
			page = 1
		}
		path = strings.ReplaceAll(path, "{page}", strconv.Itoa(page))
	} else if page > 1 {
		path = addQueryParam(path, "page", strconv.Itoa(page))
	}
	return joinURL(baseURL, path)
}

func addQueryParam(path, key, value string) string {
	u, err := url.Parse(path)
	if err != nil {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		return path + sep + url.QueryEscape(key) + "=" + url.QueryEscape(value)
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String()
}
