package subtitle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
)

// assrtProvider — ASSRT 字幕站官方 API（token 鉴权，JSON）。
// 文档：api.assrt.net/v1/sub/search + /v1/sub/detail。下载文件常为 zip（由 chain 解压）。
type assrtProvider struct {
	token string
	hc    HTTPDoer
	base  string
}

// NewAssrt 构造；token 空时 Search/Download 返 ErrNotConfigured（chain 跳过）。
func NewAssrt(token string, hc HTTPDoer) *assrtProvider {
	return &assrtProvider{token: token, hc: hc, base: "https://api.assrt.net"}
}

func (p *assrtProvider) Name() string { return "assrt" }

func (p *assrtProvider) configured() error {
	if p.token == "" {
		return ErrNotConfigured
	}
	return nil
}

func (p *assrtProvider) Search(ctx context.Context, query string, opts SearchOpts) ([]Subtitle, error) {
	if err := p.configured(); err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/v1/sub/search?token=%s&q=%s", p.base, url.QueryEscape(p.token), url.QueryEscape(query))
	resp, err := p.hc.Do(ctx, "GET", u, nil, jsonHeaders(), "", true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("assrt search: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Status int    `json:"status"`
		Err    string `json:"msg"`
		Sub    struct {
			Subs []struct {
				ID         json.Number `json:"id"`
				NativeName string      `json:"native_name"`
				VideoName  string      `json:"videoname"`
				Lang       struct {
					Desc string `json:"langlistdesc"`
				} `json:"lang"`
			} `json:"subs"`
		} `json:"sub"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, fmt.Errorf("assrt parse: %w", err)
	}
	if body.Status != 0 {
		return nil, fmt.Errorf("assrt search 失败: %s", body.Err)
	}
	out := make([]Subtitle, 0, len(body.Sub.Subs))
	for _, s := range body.Sub.Subs {
		title := firstNonEmptyStr(s.NativeName, s.VideoName)
		out = append(out, Subtitle{
			Source:   "assrt",
			SourceID: s.ID.String(),
			Title:    title,
			Language: s.Lang.Desc,
			LangCode: normalizeLang(s.Lang.Desc),
			FileName: title,
			Ext:      guessExt(title),
		})
		if opts.Limit > 0 && len(out) >= opts.Limit {
			break
		}
	}
	return out, nil
}

func (p *assrtProvider) Download(ctx context.Context, sub Subtitle) ([]byte, string, error) {
	if err := p.configured(); err != nil {
		return nil, "", err
	}
	// 取详情拿 filelist[0].url
	u := fmt.Sprintf("%s/v1/sub/detail?token=%s&id=%s", p.base, url.QueryEscape(p.token), url.QueryEscape(sub.SourceID))
	resp, err := p.hc.Do(ctx, "GET", u, nil, jsonHeaders(), "", true)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("assrt detail: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Status int `json:"status"`
		Sub    struct {
			Subs []struct {
				FileList []struct {
					URL string `json:"url"`
					F   string `json:"f"`
				} `json:"filelist"`
			} `json:"subs"`
		} `json:"sub"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, "", fmt.Errorf("assrt detail parse: %w", err)
	}
	if len(body.Sub.Subs) == 0 || len(body.Sub.Subs[0].FileList) == 0 {
		return nil, "", fmt.Errorf("assrt: 无可下载文件")
	}
	fileURL := body.Sub.Subs[0].FileList[0].URL
	name := body.Sub.Subs[0].FileList[0].F
	if !strings.HasPrefix(fileURL, "http") {
		fileURL = p.base + "/" + strings.TrimPrefix(fileURL, "/")
	}
	dl, err := p.hc.Do(ctx, "GET", fileURL, nil, nil, "", true)
	if err != nil {
		return nil, "", err
	}
	if dl.StatusCode >= 300 {
		return nil, "", fmt.Errorf("assrt download: HTTP %d", dl.StatusCode)
	}
	if name == "" {
		name = sub.FileName
	}
	return dl.Body, name, nil
}

// guessExt 从文件名/标题猜字幕扩展名（默认 srt）。
func guessExt(name string) string {
	if e := path.Ext(name); e != "" {
		return strings.ToLower(e)
	}
	return ".srt"
}

func firstNonEmptyStr(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

var _ Provider = (*assrtProvider)(nil)
