// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// piggo（猪猪）.torrent 下载分流 —— 从 site_torrent.go 拆出（行数门禁 400）。
// 适配器契约见 internal/site/schemas/piggo.go；凭据为账密 JSON（site_credential.go piggo 分支）。

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
)

var piggoDLIDRe = regexp.MustCompile(`/torrents/(\d+)/dl`)

// fetchPiggoTorrentFile piggo 下载：从 downloadURL（适配器合成的
// <base>/v1/web/torrents/<id>/dl 占位）抽 id → Piggo.DownloadTorrentFile
// （SRP 会话 POST download-link 换签名直链 → GET 直链得 .torrent）。
func (c *SiteChain) fetchPiggoTorrentFile(ctx context.Context, downloadURL, cred string) ([]byte, string, error) {
	m := piggoDLIDRe.FindStringSubmatch(downloadURL)
	if len(m) < 2 {
		return nil, "", ErrDelegatedToDownloader
	}
	// 站点 baseURL 从 downloadURL 源头取（同站守卫已在上游校验过同注册域）。
	base := ""
	if u, err := url.Parse(downloadURL); err == nil {
		base = u.Scheme + "://" + u.Host
	}
	pg := c.piggoAdapter()
	if pg == nil {
		return nil, "", fmt.Errorf("piggo 适配器未装配")
	}
	return pg.DownloadTorrentFile(ctx, base, cred, m[1])
}
