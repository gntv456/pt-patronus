package subtitle

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// zimukuProvider — 字幕库（zimuku/zmk.pw）网页抓取。goquery 解析搜索/详情页。
//
// TODO 联调：域名/选择器基于社区逆向（zimuku 域名常变：zimuku.org / zmk.pw），下载需
// 详情页→下载页多步跳转 + cookie。单测用 mock HTML 覆盖搜索解析；真实联调留用户。
type zimukuProvider struct {
	cookie string
	hc     HTTPDoer
	base   string
}

func NewZimuku(cookie string, hc HTTPDoer) *zimukuProvider {
	return &zimukuProvider{cookie: cookie, hc: hc, base: "https://zmk.pw"}
}

func (p *zimukuProvider) Name() string { return "zimuku" }

func (p *zimukuProvider) Search(ctx context.Context, query string, opts SearchOpts) ([]Subtitle, error) {
	u := fmt.Sprintf("%s/search?q=%s", p.base, url.QueryEscape(query))
	resp, err := p.hc.Do(ctx, "GET", u, nil, nil, p.cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("zimuku search: HTTP %d", resp.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("zimuku parse: %w", err)
	}
	seen := map[string]bool{}
	var out []Subtitle
	// 搜索结果页：电影/剧集条目链接到 /detail/{slug}.html
	doc.Find(`a[href*="/detail/"]`).Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		id := extractZimukuID(href)
		if id == "" || seen[id] {
			return
		}
		title := strings.TrimSpace(s.Text())
		if len(title) < 2 || seen[id] {
			return
		}
		seen[id] = true
		lang := guessLangFromTitle(title)
		out = append(out, Subtitle{
			Source: "zimuku", SourceID: id, Title: title,
			Language: lang, LangCode: normalizeLang(lang),
			FileName: title, Ext: guessExt(title),
			Extra: href,
		})
	})
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

// extractZimukuID 从 /detail/abc-123.html 取 slug。
func extractZimukuID(href string) string {
	i := strings.Index(href, "/detail/")
	if i < 0 {
		return ""
	}
	rest := href[i+8:]
	if j := strings.IndexAny(rest, "?#"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSuffix(strings.TrimSpace(rest), ".html")
}

func (p *zimukuProvider) Download(ctx context.Context, sub Subtitle) ([]byte, string, error) {
	// TODO 联调：zimuku 下载为详情页→下载页→文件多步（含 cookie/ref 校验）。
	// best-effort：直接 GET 详情页，解析其中的 .download_a / 下载链接取文件；失败返明确错误。
	u := fmt.Sprintf("%s/detail/%s.html", p.base, url.PathEscape(sub.SourceID))
	resp, err := p.hc.Do(ctx, "GET", u, nil, nil, p.cookie, true)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("zimuku detail: HTTP %d", resp.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, "", fmt.Errorf("zimuku detail parse: %w", err)
	}
	// 取首个下载链接（详情页“下载按钮”）
	dlURL, exists := doc.Find(`a.download_a, a[href*="/dld/"]`).First().Attr("href")
	if !exists || dlURL == "" {
		return nil, "", fmt.Errorf("zimuku: 详情页未找到下载链接（需 cookie/联调）")
	}
	if strings.HasPrefix(dlURL, "/") {
		dlURL = p.base + dlURL
	}
	file, err := p.hc.Do(ctx, "GET", dlURL, nil, nil, p.cookie, true)
	if err != nil {
		return nil, "", err
	}
	if file.StatusCode >= 300 {
		return nil, "", fmt.Errorf("zimuku download: HTTP %d", file.StatusCode)
	}
	name := sub.FileName
	if name == "" {
		name = sub.SourceID
	}
	return file.Body, name, nil
}

var _ Provider = (*zimukuProvider)(nil)
