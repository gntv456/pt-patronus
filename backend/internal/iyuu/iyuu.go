// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package iyuu 对接 IYUU（api.iyuu.cn）精确辅种接口：按 infohash 跨站匹配 + 站点目录。
// 协议参考 Go 实现 goreseed：sign 字段即 IYUU token（无额外签名）；下载 URL 客户端按平台规则拼 passkey。
package iyuu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

const DefaultBase = "https://api.iyuu.cn"

// SiteInfo 是 IYUU 站点目录的一项（App.Api.Sites）。
type SiteInfo struct {
	ID           int    `json:"id"`
	Site         string `json:"site"`          // IYUU 站点键，如 "m-team"/"ourbits"
	BaseURL      string `json:"base_url"`      // 不含 scheme，如 "xp.m-team.cc"
	DownloadPage string `json:"download_page"` // 含 {} 占位，如 "download.php?id={}"
	IsHTTPS      int    `json:"is_https"`      // 1=https 0=http
}

// MatchItem 是某 hash 的一个跨站匹配（App.Api.Hash 返回）。
type MatchItem struct {
	Sid       int    `json:"sid"`        // 命中的站点 ID（对应 SiteInfo.ID）
	TorrentID int    `json:"torrent_id"` // 该站点的种子 id（填入 download_page 占位）
	InfoHash  string `json:"info_hash"`  // 该站点资源的 infohash
}

// HashMatch 是一个源 hash 的全部跨站匹配。
type HashMatch struct {
	Hash     string      `json:"hash"`
	Torrents []MatchItem `json:"torrent"`
}

// Client 调用 IYUU 接口。
type Client struct {
	hc   *httpclient.Client
	base string
}

// New 构造；base 为空走 DefaultBase。
func New(hc *httpclient.Client, base string) *Client {
	if base == "" {
		base = DefaultBase
	}
	return &Client{hc: hc, base: base}
}

// HashIndex 提交一批 infohash，返回每个 hash 的跨站精确匹配。
// 空 hashes 直接返回 nil（不发请求）。
func (c *Client) HashIndex(ctx context.Context, token string, hashes []string) ([]HashMatch, error) {
	if token == "" {
		return nil, fmt.Errorf("iyuu: empty token")
	}
	if len(hashes) == 0 {
		return nil, nil
	}
	// hash 字段是 JSON 字符串数组：["h1","h2"]
	hashJSON, err := json.Marshal(hashes)
	if err != nil {
		return nil, err
	}
	body, err := c.postForm(ctx, c.base+"/?service=App.Api.Hash", token, map[string]string{
		"timestamp": "0",
		"version":   "1.0.0",
		"hash":      string(hashJSON),
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Ret  int         `json:"ret"`
		Data []HashMatch `json:"data"`
		Msg  string      `json:"msg"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("iyuu hash: bad response: %w", err)
	}
	if resp.Ret != 200 {
		return nil, fmt.Errorf("iyuu hash: %s", resp.Msg)
	}
	return resp.Data, nil
}

// Sites 返回 IYUU 站点目录（sid → 站点信息）。
func (c *Client) Sites(ctx context.Context, token string) ([]SiteInfo, error) {
	if token == "" {
		return nil, fmt.Errorf("iyuu: empty token")
	}
	body, err := c.postForm(ctx, c.base+"/?service=App.Api.Sites", token, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Ret  int `json:"ret"`
		Data struct {
			Sites []SiteInfo `json:"sites"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("iyuu sites: bad response: %w", err)
	}
	if resp.Ret != 200 {
		return nil, fmt.Errorf("iyuu sites: %s", resp.Msg)
	}
	return resp.Data.Sites, nil
}

// postForm 以 multipart/form-data 发送（IYUU 按 goreseed 接 multipart；PHP $_POST 兼容）。
// sign 固定为 token；extra 为额外字段。
func (c *Client) postForm(ctx context.Context, rawURL, token string, extra map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("sign", token)
	for k, v := range extra {
		_ = w.WriteField(k, v)
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	headers := map[string]string{"Content-Type": w.FormDataContentType()}
	resp, err := c.hc.Do(ctx, http.MethodPost, rawURL, buf.Bytes(), headers, "", true)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// BuildDownloadURL 按 IYUU 站点目录 + torrent_id + 本地 passkey 拼出可直接添加的下载链接。
// 移植 goreseed getTorrentUrl 的平台分支（默认 NexusPHP download.php?id=&passkey=）。
func BuildDownloadURL(s SiteInfo, torrentID int, passkey string) string {
	scheme := "http://"
	if s.IsHTTPS == 1 {
		scheme = "https://"
	}
	page := strings.ReplaceAll(s.DownloadPage, "{}", strconv.Itoa(torrentID))
	down := scheme + s.BaseURL + "/" + page
	switch s.Site {
	case "ttg":
		return down + "/" + passkey
	case "m-team", "moecat":
		return down + "&passkey=" + passkey + "&https=1"
	case "ccfbits":
		return strings.ReplaceAll(down, "{passkey}", passkey)
	default:
		return down + "&passkey=" + passkey
	}
}
