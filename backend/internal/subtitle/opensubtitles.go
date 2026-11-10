// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package subtitle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// opensubtitlesProvider — OpenSubtitles 官方 REST API（Api-Key 头鉴权）。
// 文档：api.opensubtitles.com/api/v1/subtitles（搜索）+ /api/v1/download（取直链）。
// 下载需先 POST file_id 换临时直链，再 GET 直链取字幕（多为 srt/ass）。
type opensubtitlesProvider struct {
	apiKey string
	hc     HTTPDoer
	base   string
}

func NewOpenSubtitles(apiKey string, hc HTTPDoer) *opensubtitlesProvider {
	return &opensubtitlesProvider{apiKey: apiKey, hc: hc, base: "https://api.opensubtitles.com"}
}

func (p *opensubtitlesProvider) Name() string { return "opensubtitles" }

func (p *opensubtitlesProvider) configured() error {
	if p.apiKey == "" {
		return ErrNotConfigured
	}
	return nil
}

func (p *opensubtitlesProvider) headers() map[string]string {
	h := jsonHeaders()
	h["Api-Key"] = p.apiKey
	h["Content-Type"] = "application/json"
	return h
}

func (p *opensubtitlesProvider) Search(ctx context.Context, query string, opts SearchOpts) ([]Subtitle, error) {
	if err := p.configured(); err != nil {
		return nil, err
	}
	params := url.Values{"query": {query}}
	if opts.Language != "" {
		params["languages"] = []string{osdLang(opts.Language)}
	}
	u := p.base + "/api/v1/subtitles?" + params.Encode()
	resp, err := p.hc.Do(ctx, "GET", u, nil, p.headers(), "", true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("opensubtitles search: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				FileName    string `json:"file_name"`
				Language    string `json:"language"`
				DownloadCnt int    `json:"download_count"`
				Ratings     int    `json:"ratings"`
				Release     string `json:"release"`
				Files       []struct {
					FileID   json.Number `json:"file_id"`
					FileName string      `json:"file_name"`
				} `json:"files"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, fmt.Errorf("opensubtitles parse: %w", err)
	}
	out := make([]Subtitle, 0, len(body.Data))
	for _, d := range body.Data {
		if len(d.Attributes.Files) == 0 {
			continue
		}
		f := d.Attributes.Files[0]
		name := firstNonEmptyStr(f.FileName, d.Attributes.FileName, d.Attributes.Release)
		out = append(out, Subtitle{
			Source:   "opensubtitles",
			SourceID: f.FileID.String(), // 下载用 file_id
			Title:    name,
			Language: d.Attributes.Language,
			LangCode: normalizeLang(d.Attributes.Language),
			FileName: name,
			Ext:      guessExt(name),
			Extra:    d.ID,
			Score:    d.Attributes.DownloadCnt,
		})
		if opts.Limit > 0 && len(out) >= opts.Limit {
			break
		}
	}
	return out, nil
}

func (p *opensubtitlesProvider) Download(ctx context.Context, sub Subtitle) ([]byte, string, error) {
	if err := p.configured(); err != nil {
		return nil, "", err
	}
	// POST file_id 换临时直链
	body, _ := json.Marshal(map[string]any{"file_id": sub.SourceID})
	resp, err := p.hc.Do(ctx, "POST", p.base+"/api/v1/download", body, p.headers(), "", true)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("opensubtitles download link: HTTP %d", resp.StatusCode)
	}
	var dl struct {
		Link     string `json:"link"`
		FileName string `json:"file_name"`
	}
	if err := json.Unmarshal(resp.Body, &dl); err != nil {
		return nil, "", fmt.Errorf("opensubtitles download parse: %w", err)
	}
	if dl.Link == "" {
		return nil, "", fmt.Errorf("opensubtitles: 未取到下载直链")
	}
	// GET 直链取字幕字节
	file, err := p.hc.Do(ctx, "GET", dl.Link, nil, nil, "", true)
	if err != nil {
		return nil, "", err
	}
	if file.StatusCode >= 300 {
		return nil, "", fmt.Errorf("opensubtitles fetch: HTTP %d", file.StatusCode)
	}
	name := firstNonEmptyStr(dl.FileName, sub.FileName)
	return file.Body, name, nil
}

// osdLang 把内部 lang code 转 OpenSubtitles 的语言代码（zh-CN/zh-TW/en）。
func osdLang(code string) string {
	switch code {
	case "chs":
		return "zh-CN"
	case "cht":
		return "zh-TW"
	case "eng":
		return "en"
	}
	return code
}

var _ Provider = (*opensubtitlesProvider)(nil)
