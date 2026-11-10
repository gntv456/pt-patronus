// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package errtext 把底层技术错误翻成用户看得懂、能照着做的中文。
//
// 它是「文案的单一来源」：HTTP 硬失败面（errcode 信封）与逐条结果里的 error 字段
// （一键签到、站点连通性、求种、搜索等每站一行的表格）都从这里取，避免同一个
// 判定在两处各写一份文案、改了这边漏了那边。
//
// 站点适配器 / 下载器客户端自己返回的原文一律透传：那是用户唯一的线索，不做二次加工。
// 例外是本仓库自己声明的下载器哨兵（连不上），它有明确的下一步动作，翻成中文提示。
package errtext

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"strings"
	"unicode/utf8"

	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"gorm.io/gorm"
)

const (
	// SSRFHint 站点建在内网/环回时的可操作提示（ALLOW_PRIVATE_SITES 开关就是为它存在的）。
	SSRFHint = "该站点地址是内网/环回地址，已被 SSRF 防护拦截：请改用可公网访问的地址，或设置环境变量 ALLOW_PRIVATE_SITES=true 后重启程序"
	// NotFound 按 id 定位记录却不存在（多半是列表里那一行已被删除）。
	NotFound = "目标不存在，可能已被删除，请刷新后重试"
	// FileMissing 文件管理器按路径操作，但那个路径在磁盘上已经没了
	// （别的程序或另一个标签页先删了）。Go 的 `remove C:\…: The system cannot find the file specified.`
	// 对用户没有任何意义，也不能拿来当「操作成功」的反面。
	FileMissing = "文件或目录不存在，可能已被删除，请刷新文件列表"
	// Timeout 上游请求超时。
	Timeout = "请求超时，请稍后重试"
	// DownloaderOffline 下载器连不上。哨兵是本仓库自己写的（不是客户端原文），
	// 从前它在各处被当成「请求参数有误」弹回用户，指错了方向——该去的是「下载器」页。
	DownloaderOffline = "下载器暂时连不上：请确认客户端已启动、地址与端口正确，可在「下载器」页用「测试连接」复核"
	// MissingScheme 地址里没有协议前缀。Go 的原文是 `unsupported protocol scheme ""`，
	// 用户看得懂才有救（漏写 https:// 是 RSS/监控地址最常见的笔误）。
	MissingScheme = "地址缺少协议前缀：请填写以 http:// 或 https:// 开头的完整地址"
	// #401 下载 .torrent 时的两类失败。哨兵定义在 chain/site_torrent.go，原文是英文，
	// 而下载推送这条路径的出口是 opResult → userErrText = errtext.Text。
	// Text 从前不认这两个哨兵 → 落到兜底原样返回 err.Error()，于是用户看到
	//
	//	下载 .torrent 失败: site cookie invalid or expired (download redirected to login)
	//
	// Cookie 失效是下载失败里最常见的一种（站点把 download 重定向到登录页），
	// 而英文既没点名字段也没给下一步：用户不知道要去「站点」页重登 cookie，
	// 只会反复点推送。
	SiteCookieInvalid = "站点 Cookie 已失效（下载时站点把请求转到了登录页）：请到「站点」页重新获取并保存该站点的 Cookie，再重推一次"
	NotTorrentFile    = "下载到的内容不是有效的种子文件（多半是登录页/错误页 HTML）：请确认该站点 Cookie 是否有效，或该种子是否已被删除"
	// Unreachable 连不上目标或域名解析不了。Go/os 原文（dial tcp…/connectex…/no such host）
	// 对界面没有任何信息量，原文一律留在日志里。
	Unreachable = "连不上该地址：请确认服务已启动、端口填写正确；域名解析不了时先核对拼写与本机 DNS"
)

// registry 允许上层包（chain 等）把自己的哨兵登记进来，配一句可操作的中文。
//
// #401 的由来：chain/site_torrent.go 的 ErrCookieInvalid / ErrNotTorrent 是下载失败里
// 最常见的两类，原文是英文。出口 opResult → userErrText(=errtext.Text) 从前不认它们，
// 落到兜底原样返回 err.Error()，用户看到
//
//	下载 .torrent 失败: site cookie invalid or expired (download redirected to login)
//
// ——英文、没点名字段、没给下一步（该去站点页重登 cookie）。
//
// 为什么用注册而不是直接 errors.Is 判：errtext 被 chain 依赖（chain → errtext），
// 反过来 errtext → chain 就是环。所以哨兵由 chain 在 init 时登记进来，
// 判据仍留在 errtext 这一处（单一来源），只是把「哪些哨兵」变成可注册而非硬编码。
var registry []struct {
	sentinel error
	text     string
}

// Register 把一个哨兵登记为可翻译的错误，配一句写给用户、能照着做的中文。
//
// 只许在 **init()** 里调用，不得在 handler / goroutine 里调：registry 是个包级 slice，
// append 与 lookupRegistry 的并发遍历构成 data race（-race 会当场报）。
// Go 保证全部 init() 跑完才进 main，所以「init 里写、之后只读」是安全的 ——
// 这也正是本仓现有两处登记（chain/site_torrent.go）安全的原因。
//
// 将来若需要在运行时登记（比如插件注册自己的哨兵），必须改成 sync.RWMutex 或
// copy-on-write（原子替换整个 slice），不能直接沿用现在这个 append。
func Register(sentinel error, text string) {
	if sentinel == nil || text == "" {
		return
	}
	registry = append(registry, struct {
		sentinel error
		text     string
	}{sentinel, text})
}

// lookupRegistry 查登记表。
func lookupRegistry(err error) (string, bool) {
	for _, r := range registry {
		if errors.Is(err, r.sentinel) {
			return r.text, true
		}
	}
	return "", false
}

// Text 返回给用户看的错误文本；err 为 nil 时返回空串。
func Text(err error) string {
	if err == nil {
		return ""
	}
	// 登记表优先于内置档：上层登记的哨兵都带可操作的下一步，比通用兜底有用。
	if text, ok := lookupRegistry(err); ok {
		return text
	}
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return NotFound
	case errors.Is(err, httpclient.ErrBlocked):
		return SSRFHint
	case errors.Is(err, context.DeadlineExceeded):
		return Timeout
	case errors.Is(err, downloader.ErrCannotConnect):
		return DownloaderOffline
	case errors.Is(err, fs.ErrNotExist):
		return FileMissing
	}
	// 传输层两类失败：Go 原文（unsupported protocol scheme / dial tcp / connectex / no such host）
	// 对界面零信息量，原文一律留在日志里，界面只给可操作的中文。
	// 位置在哨兵判定之后：ErrBlocked（SSRF）等已有更具体的下一步，不能被这里抢走。
	if msg := err.Error(); strings.Contains(msg, "unsupported protocol scheme") {
		return MissingScheme
	} else if unreachableErr(err, msg) {
		return Unreachable
	}
	return err.Error()
}

// unreachableErr 判「连不上/解析不了」。dial 层能 errors.As 到 *net.OpError；
// 各平台把 connection refused 包成不可 As 的 syscall 错误，故再兜一层文本匹配。
func unreachableErr(err error, msg string) bool {
	var oe *net.OpError
	if errors.As(err, &oe) {
		return true
	}
	for _, s := range []string{"connection refused", "connectex:", "no such host", "network is unreachable", "connection timed out"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// RespSnippet 截取上游响应原文用于错误提示定位，最长 160 字节——
// 只给用户看得懂的一小段，不把整个 HTML 页塞进 toast。
func RespSnippet(b []byte) string {
	const max = 160
	if len(b) <= max {
		return string(b)
	}
	// 收尾必须落在 rune 边界：中文响应按字节切到第 160 个字节时，往往正好砍在一个
	// 汉字的第 2/3 个字节上，半个 rune 进 JSON 会被编码成 U+FFFD（界面显示成乱码方块）。
	s := b[:max]
	for len(s) > 0 {
		if r, size := utf8.DecodeLastRune(s); r == utf8.RuneError && size <= 1 {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	if len(s) == 0 {
		// 整段都不含合法 rune（二进制垃圾响应）：回原始截断段，
		// 也不能把「唯一线索」清成空串。
		return string(b[:max]) + "\u2026"
	}
	return string(s) + "\u2026"
}
