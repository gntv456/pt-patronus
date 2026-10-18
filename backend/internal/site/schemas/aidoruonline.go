package schemas

import (
	"bytes"
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

var aidoruSeedPageRe = regexp.MustCompile(`Page\s+(\d+)\s*/\s*(\d+)`)

func fetchAidoruOnlineUserInfo(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
	u := &site.UserInfo{}
	cp, resp, err := fetchAidoruOnlineDoc(ctx, hc, baseURL, "/forum/usercp.php", cookie)
	if err == nil {
		if loggedOut(resp, cp, def.NoLogin) {
			return nil, site.ErrNotLoggedIn
		}
		site.ParseUserInfoFields(cp, def.UserInfo.Fields, u)
	}
	account, _, err := fetchAidoruOnlineDoc(ctx, hc, baseURL, "/account.php", cookie)
	if err != nil {
		if userInfoParsesEmpty(u) {
			return nil, err
		}
		return u, nil // usercp 已读到东西，account 页失败按部分数据返回
	}
	site.ParseUserInfoFields(account, def.UserInfo.Fields, u)
	u.Bonus = 0
	u.SeedingSize = 0
	pages := aidoruOnlineSeedPages(account)
	for p := 0; p < pages; p++ {
		if doc, _, err := fetchAidoruOnlineDoc(ctx, hc, baseURL, "/get_account_peers.php?uid=&p="+strconv.Itoa(p)+"&type=seed&searchstr=", cookie); err == nil {
			u.SeedingSize += aidoruOnlineSeedingSize(doc)
		}
	}
	if userInfoParsesEmpty(u) {
		return nil, emptyUserInfoErr(baseURL, "usercp 与 account 页")
	}
	return u, nil
}

func fetchAidoruOnlineDoc(ctx context.Context, hc *httpclient.Client, baseURL, path, cookie string) (*goquery.Document, *httpclient.Response, error) {
	full := joinURL(baseURL, path)
	resp, err := hc.Get(ctx, full, cookie, true)
	if err != nil {
		return nil, resp, err
	}
	// 与 fetchHTMLDoc 同两道判定：CF 盾页与 5xx 错误页都会解析成「选择器全落空」的空文档。
	if err := guardHTMLResp(resp, baseURL, full); err != nil {
		return nil, resp, err
	}
	body := resp.Body
	if strings.Contains(path, "get_account_peers.php") {
		body = []byte("<table><tbody>" + string(body) + "</tbody></table>")
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	return doc, resp, err
}

func aidoruOnlineSeedPages(doc *goquery.Document) int {
	raw := strings.TrimSpace(doc.Find("div#seed-search-stat").First().Text())
	if m := aidoruSeedPageRe.FindStringSubmatch(raw); len(m) > 2 {
		if total, err := strconv.Atoi(m[2]); err == nil && total > 0 {
			return total
		}
		if current, err := strconv.Atoi(m[1]); err == nil && current > 0 {
			return current
		}
	}
	return 0
}

func aidoruOnlineSeedingSize(doc *goquery.Document) int64 {
	var total int64
	doc.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		size := strings.TrimSpace(tr.Find("td").Eq(3).Text())
		total += site.ParseBytes(strings.ReplaceAll(size, ",", ""))
	})
	return total
}
