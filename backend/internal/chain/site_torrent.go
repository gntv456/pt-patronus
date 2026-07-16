package chain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 下载 .torrent 时的哨兵错误。
var (
	// ErrDelegatedToDownloader 表示种子 URL 自带凭证 / 无凭证可补 / 非 http(s) / 未装配 hc，
	// 应让下载器沿用旧路径直接用 URL 添加——不视为失败，DownloadChain.Add 据此跳过 cookie 下载。
	ErrDelegatedToDownloader = errors.New("torrent url is self-authenticating or no credential; delegate to downloader")
	// ErrCookieInvalid 表示站点 cookie 失效（download 跳转/返回登录页）。调用方应上抛让用户/cron 重试。
	ErrCookieInvalid = errors.New("site cookie invalid or expired (download redirected to login)")
	// ErrNotTorrent 表示响应不是合法 .torrent（HTML 伪装/未知页面），防把 HTML 当种子推下载器。
	ErrNotTorrent = errors.New("download response is not a valid torrent file")
)

// FetchTorrentFile 用站点凭据（cookie 为主，必要时补 passkey）把 downloadURL 指向的
// .torrent 下载成字节。对标 MoviePilot TorrentHelper.download_torrent：MP 自己用站点 cookie
// 下载 .torrent 再以文件方式提交下载器，而非让下载器直连需登录态的 PT 站 URL。
//
// 返回约定：
//   - 成功：content 为 .torrent 字节，fileName 为建议文件名。
//   - ErrDelegatedToDownloader：URL 自带凭证 / 无凭证可补 / 非 http(s) / 未装配 hc → 调用方走旧 URL 路径。
//   - ErrCookieInvalid / ErrNotTorrent / 其他：调用方上抛（禁止静默假成功——这正是原 bug 根源）。
func (c *SiteChain) FetchTorrentFile(ctx context.Context, s *models.Site, downloadURL string) (content []byte, fileName string, err error) {
	if c == nil || c.hc == nil {
		return nil, "", ErrDelegatedToDownloader
	}
	if !isHTTPURL(downloadURL) {
		return nil, "", ErrDelegatedToDownloader // magnet 等交下载器直处理
	}
	// 同站守卫：downloadURL 必须落在站点 baseURL 同注册域，防 A 站 cookie 打 B 站（SSRF 双重防线）。
	if !sameHost(s.BaseURL, downloadURL) {
		return nil, "", ErrDelegatedToDownloader
	}
	def := c.defFor(s)
	// schema 分流：mteam/rousi 的 download URL 自带临时 token/passkey，下载器可直连；
	// 且 mteam 用 cookie 下载反致错（对标 MP 注释「涉及解析地址的不使用Cookie下载种子，否则MT会出错」）。
	if def != nil && (def.Schema == "mteam" || def.Schema == "rousi") {
		return nil, "", ErrDelegatedToDownloader
	}
	cookie := c.credential(s, def)
	passkey := c.sites.DecryptedPasskey(s)
	if cookie == "" && passkey == "" {
		return nil, "", ErrDelegatedToDownloader // 无任何凭据，让下载器试原 URL
	}
	// passkey 补全：URL 不含 passkey 且站点配了 passkey → append（实测 NexusPHP download.php 支持 ?passkey=）。
	dlURL := downloadURL
	if passkey != "" && !strings.Contains(strings.ToLower(dlURL), "passkey=") {
		dlURL = appendQuery(dlURL, "passkey", passkey)
	}
	resp, gerr := c.hc.Get(ctx, dlURL, cookie, true)
	if gerr != nil {
		return nil, "", fmt.Errorf("fetch torrent: %w", gerr)
	}
	content, err = c.classifyAndConfirm(ctx, resp, dlURL, cookie)
	if err != nil {
		return nil, "", err
	}
	return content, torrentFileName(resp, downloadURL), nil
}

// FetchTorrentFileByID 按 siteID 取站点后委托 FetchTorrentFile（供 DownloadChain 调，免其碰 oper）。
func (c *SiteChain) FetchTorrentFileByID(ctx context.Context, siteID uint, downloadURL string) ([]byte, string, error) {
	if c == nil {
		return nil, "", ErrDelegatedToDownloader
	}
	s, err := c.sites.Get(siteID)
	if err != nil {
		return nil, "", fmt.Errorf("get site %d: %w", siteID, err)
	}
	return c.FetchTorrentFile(ctx, s, downloadURL)
}

// classifyAndConfirm 对 download 响应分类：合法 .torrent 直接返回；NexusPHP 首次下载确认页
// 则 POST 确认后重判；登录页/HTML 报错。对标 MP 处理 download.php 确认表单。
func (c *SiteChain) classifyAndConfirm(ctx context.Context, resp *httpclient.Response, dlURL, cookie string) ([]byte, error) {
	if resp == nil {
		return nil, ErrNotTorrent
	}
	if isTorrentBytes(resp.Body) {
		return resp.Body, nil
	}
	body := string(resp.Body)
	// NexusPHP 首次下载确认页：form action 含 download + hidden 字段（cookie 有效才出现，优先判）。
	if action, form := parseDownloadConfirmForm(body, resp.FinalURL); action != "" {
		slog.Info("torrent download confirm page detected, posting", "url", dlURL)
		postResp, perr := c.hc.PostFormAjaxWith(ctx, action, cookie, form, dlURL, nil, true)
		if perr != nil {
			return nil, fmt.Errorf("post confirm: %w", perr)
		}
		if isTorrentBytes(postResp.Body) {
			return postResp.Body, nil
		}
		if looksLikeLogin(string(postResp.Body), postResp.StatusCode) {
			return nil, ErrCookieInvalid
		}
		return nil, ErrNotTorrent
	}
	// cookie 失效：登录页特征。
	if looksLikeLogin(body, resp.StatusCode) {
		return nil, ErrCookieInvalid
	}
	return nil, ErrNotTorrent
}

// isTorrentBytes 轻量 bencode sniff：.torrent 是 bencode dict，首字节 'd'(0x64)、末字节 'e'(0x65)，
// 且必含 info / announce 键。区分 bencode 与 HTML/JSON，不引外部依赖。
func isTorrentBytes(b []byte) bool {
	return len(b) > 64 && b[0] == 'd' && b[len(b)-1] == 'e' &&
		(bytes.Contains(b, []byte("4:info")) || bytes.Contains(b, []byte("8:announce")))
}

// looksLikeLogin 判定响应是否为登录页（cookie 失效）。FollowRedirect:true 已把 302→login 跟随成
// 200+HTML，这里靠 body 关键词（含中英文 + NexusPHP 变体）+ 状态码双信号判定。
func looksLikeLogin(body string, status int) bool {
	if status == 401 || status == 403 {
		return true
	}
	low := strings.ToLower(body)
	return strings.Contains(low, "takelogin") || strings.Contains(low, "login.php") ||
		strings.Contains(low, "returnto=") || strings.Contains(low, "登录") || strings.Contains(low, "请登录")
}

var (
	reFormAction = regexp.MustCompile(`(?i)<form[^>]*action=["']([^"']*)["']`)
	// hidden input：name/value 顺序兼容（NexusPHP 模板 value 在 name 前/后均可能出现）。
	reHiddenNameFirst  = regexp.MustCompile(`(?i)<input[^>]*type=["']hidden["'][^>]*name=["']([^"']*)["'][^>]*value=["']([^"']*)["']`)
	reHiddenValueFirst = regexp.MustCompile(`(?i)<input[^>]*value=["']([^"']*)["'][^>]*name=["']([^"']*)["']`)
)

// parseDownloadConfirmForm 检测 NexusPHP 首次下载确认页并提取确认表单。
// 返回 (action 绝对 URL, form-urlencoded body)；非确认页返回 ("", "")。
func parseDownloadConfirmForm(body, finalURL string) (action, form string) {
	m := reFormAction.FindStringSubmatch(body)
	if m == nil {
		return "", ""
	}
	if !strings.Contains(strings.ToLower(m[1]), "download") {
		return "", "" // 登录页等 form action 不含 download
	}
	action = resolveActionURL(finalURL, m[1])
	vals := url.Values{}
	for _, hm := range reHiddenNameFirst.FindAllStringSubmatch(body, -1) {
		vals.Set(hm[1], hm[2])
	}
	for _, hm := range reHiddenValueFirst.FindAllStringSubmatch(body, -1) {
		vals.Set(hm[2], hm[1])
	}
	if len(vals) == 0 {
		return "", ""
	}
	return action, vals.Encode()
}

// resolveActionURL 把 form action（可能相对）解析为绝对 URL（相对 finalURL，即跟随后终态 URL）。
func resolveActionURL(finalURL, action string) string {
	if action == "" {
		return ""
	}
	if strings.HasPrefix(action, "http://") || strings.HasPrefix(action, "https://") {
		return action
	}
	base, err := url.Parse(finalURL)
	if err != nil {
		return action
	}
	ref, err := url.Parse(action)
	if err != nil {
		return action
	}
	return base.ResolveReference(ref).String()
}

// torrentFileName 从 Content-Disposition 取文件名，否则按 download.php?id=XXX 回退 XXX.torrent。
func torrentFileName(resp *httpclient.Response, dlURL string) string {
	if resp != nil {
		if name := filenameFromContentDisposition(resp.Header.Get("Content-Disposition")); name != "" {
			return name
		}
	}
	if u, err := url.Parse(dlURL); err == nil {
		if id := u.Query().Get("id"); id != "" {
			return id + ".torrent"
		}
	}
	return "file.torrent"
}

// filenameFromContentDisposition 解析 Content-Disposition 头的 filename（兼容 RFC 5987 filename*）。
func filenameFromContentDisposition(cd string) string {
	if cd == "" {
		return ""
	}
	low := strings.ToLower(cd)
	if idx := strings.Index(low, "filename*="); idx >= 0 {
		rest := cd[idx+len("filename*="):]
		if eq := strings.Index(rest, "''"); eq >= 0 {
			val := rest[eq+2:]
			if semi := strings.Index(val, ";"); semi >= 0 {
				val = val[:semi]
			}
			if dec, err := url.QueryUnescape(strings.Trim(val, `"`)); err == nil && dec != "" {
				return dec
			}
		}
	}
	if idx := strings.Index(low, "filename="); idx >= 0 {
		rest := strings.TrimLeft(cd[idx+len("filename="):], " ")
		if strings.HasPrefix(rest, `"`) {
			rest = rest[1:]
			if q := strings.Index(rest, `"`); q >= 0 {
				return rest[:q]
			}
		}
		if semi := strings.Index(rest, ";"); semi >= 0 {
			rest = rest[:semi]
		}
		return strings.TrimSpace(rest)
	}
	return ""
}

// isHTTPURL 判断是否 http(s) 链接（非磁力等）。
func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// appendQuery 给 URL 追加一个 query 参数（已存在则覆盖）。
func appendQuery(rawURL, key, val string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Set(key, val)
	u.RawQuery = q.Encode()
	return u.String()
}
