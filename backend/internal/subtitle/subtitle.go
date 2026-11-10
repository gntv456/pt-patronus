// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package subtitle 封装多源字幕搜索与下载（SubHD / Zimuku / ASSRT / OpenSubtitles）。
//
// 设计：各源实现 Provider 接口（Search 返回 []Subtitle，Download 解析为字节），
// chain 层并发跨源搜索 + 聚合 + 单源失败不阻断（同 media 包范式）。下载统一经
// extractSubtitle 解 zip + 编码转 UTF-8 + 可选繁简转换（OpenCC）。
//
// SubHD / Zimuku 为网页抓取（goquery），字段/选择器基于社区逆向，httptest mock 覆盖，
// 真实联调留用户（代码标 TODO 联调）。ASSRT / OpenSubtitles 走官方 API（token/api-key）。
package subtitle

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ErrNotConfigured 表示该源未配置（缺 token/api-key/cookie），Chain 会静默跳过它（同 media.ErrNotConfigured）。
var ErrNotConfigured = errors.New("subtitle source not configured")

// Subtitle 一条字幕搜索结果。Source+SourceID 唯一定位；Extra 存源私有下载引用（URL/详情 id），不序列化。
type Subtitle struct {
	Source   string `json:"source"`              // assrt | opensubtitles | subhd | zimuku
	SourceID string `json:"source_id"`           // 源内 id
	Title    string `json:"title"`               // 视频/发布名
	Language string `json:"language"`            // 源原始语言标签
	LangCode string `json:"lang_code"`           // 归一化：chs | cht | eng | bi | und
	FileName string `json:"file_name,omitempty"` // 预期文件名
	Ext      string `json:"ext,omitempty"`       // srt | ass | vtt（媒体扩展名）
	Extra    string `json:"-"`                   // 源私有下载引用（URL/详情页 id 等）
	Score    int    `json:"-"`                   // 排序权重（下载量/匹配度，越大越前）
}

// SearchOpts 搜索选项。
type SearchOpts struct {
	Language string // "" 任意 | chs | cht | eng
	Limit    int    // 单源返回上限（0=不限）
}

// Provider 字幕源接口。
type Provider interface {
	Name() string
	Search(ctx context.Context, query string, opts SearchOpts) ([]Subtitle, error)
	Download(ctx context.Context, sub Subtitle) (data []byte, filename string, err error)
}

// HTTPDoer 是 httpclient.Client 的最小行为接口（便于单测注入 fake）。各源以 ssrf=true 走公网守卫。
type HTTPDoer interface {
	Do(ctx context.Context, method, rawURL string, body []byte,
		headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error)
}

// jsonHeaders 返回 JSON 接收头（API 源用）。
func jsonHeaders() map[string]string {
	return map[string]string{"Accept": "application/json"}
}

// subtitleExts 支持的字幕文本扩展名。
var subtitleExts = map[string]bool{
	".srt": true, ".ass": true, ".ssa": true, ".vtt": true, ".sub": true, ".lrc": true,
}

// IsSubtitleExt 是否字幕扩展名。
func IsSubtitleExt(name string) bool {
	return subtitleExts[strings.ToLower(path.Ext(name))]
}

// normalizeLang 把源杂乱的语言标签归一为 chs/cht/eng/bi/und。
func normalizeLang(raw string) string {
	r := strings.ToLower(strings.TrimSpace(raw))
	if r == "" {
		return "und"
	}
	// 双语优先（「简繁双语」含「繁」字，须先判双语避免误判繁体）
	if strings.Contains(r, "双语") || strings.Contains(r, "bilingual") ||
		(strings.Contains(r, "中") && (strings.Contains(r, "英") || strings.Contains(r, "eng"))) {
		return "bi"
	}
	// 繁体特征
	if strings.Contains(r, "繁") || strings.Contains(r, "cht") || strings.Contains(r, "traditional") ||
		strings.Contains(r, "big5") || strings.Contains(r, "zh-tw") || strings.Contains(r, "zh-hk") {
		return "cht"
	}
	// 简体特征
	if strings.Contains(r, "简") || strings.Contains(r, "chs") || strings.Contains(r, "gb") ||
		strings.Contains(r, "simplified") || strings.Contains(r, "zh-cn") {
		return "chs"
	}
	// 中文兜底（含「中」「华」但无繁简特征，按简体处理——简体用户更常见）
	if strings.Contains(r, "中") || strings.Contains(r, "华") || strings.Contains(r, "chinese") || strings.Contains(r, "zh") {
		return "chs"
	}
	if strings.Contains(r, "eng") || strings.Contains(r, "english") || strings.Contains(r, "en") {
		return "eng"
	}
	return "und"
}

// Extract 处理下载结果：若是 zip 包则解压取其中的字幕文件；否则原样返回。
// 返回字幕字节 + 文件名。RAR/7z 不支持（返明确错误，让调用方透传）。
func Extract(data []byte, fallbackName string) ([]byte, string, error) {
	// zip 魔数 PK\x03\x04
	if len(data) > 3 && data[0] == 'P' && data[1] == 'K' && (data[2] == 0x03 || data[2] == 0x05 || data[2] == 0x07) {
		out, name, err := extractZip(data)
		if err != nil {
			return nil, "", err
		}
		if looksLikeWebPage(out) {
			return nil, "", errSubtitleIsWebPage(name)
		}
		return out, name, nil
	}
	// 原样（已是单文件字幕）
	if looksLikeWebPage(data) {
		return nil, "", errSubtitleIsWebPage(fallbackName)
	}
	name := fallbackName
	if name == "" {
		name = "subtitle.srt"
	}
	return data, name, nil
}

// looksLikeWebPage 判断「下载到的字幕」其实是一个网页：国内字幕站的登录墙/积分墙/反爬页
// 普遍返回 HTTP 200 + HTML（状态码看不出问题），此前会被原样写进媒体库当 .srt——
// 播放器没字幕，而健康中心按「字幕文件已存在」判定，从此不再提示缺失。
// 只认「开头就是 HTML 文档」这一形态：本项目接受的字幕格式（.srt/.ass/.ssa/.vtt/.sub/.lrc）
// 没有一个以 <!doctype html 或 <html 起始，字幕正文里出现这些词也不在开头，故不会误杀。
func looksLikeWebPage(data []byte) bool {
	s := strings.TrimLeft(string(data), "\ufeff \t\r\n")
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html")
}

// errSubtitleIsWebPage 明确错误：让调用方（手动下载/健康中心修复/追更自动补）都拿到原因，
// 而不是写一个垃圾 .srt 再报成功。
func errSubtitleIsWebPage(name string) error {
	if name == "" {
		name = "未知文件"
	}
	return fmt.Errorf("下载结果其实是网页（%s）：该字幕源多半需要登录/积分，或 cookie 已失效，未写入字幕", name)
}

// maxSubBytes 单个字幕解压上限：字幕源返声明巨大的 UncompressedSize64 条目（zip 炸弹）时，
// 避免几十 GB 分配拖垮进程。64MB 远超任何正常字幕，安全余量充足。
const maxSubBytes int64 = 64 << 20

// extractZip 从 zip 字节中取第一个字幕文件。
func extractZip(data []byte) ([]byte, string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, "", fmt.Errorf("解压 zip 失败: %w", err)
	}
	// 优先选字幕文件，否则选最大的文本类文件；声明体积超上限的条目一律跳过（防 zip 炸弹）。
	var best *zip.File
	for i := range zr.File {
		f := zr.File[i]
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > uint64(maxSubBytes) {
			continue
		}
		if IsSubtitleExt(f.Name) {
			best = f
			break
		}
		if best == nil || f.UncompressedSize64 > best.UncompressedSize64 {
			best = f
		}
	}
	if best == nil {
		return nil, "", errors.New("zip 内无可用字幕文件")
	}
	rc, err := best.Open()
	if err != nil {
		return nil, "", fmt.Errorf("打开 zip 条目失败: %w", err)
	}
	defer rc.Close()
	// 即便头部声明体积正常，解压实际字节仍以硬上限兜底（防声明造假/压缩比异常的炸弹）。
	b, err := io.ReadAll(io.LimitReader(rc, maxSubBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > maxSubBytes {
		return nil, "", fmt.Errorf("字幕解压超出 %d 字节上限（疑似 zip 炸弹）", maxSubBytes)
	}
	return b, path.Base(best.Name), nil
}

// DecodeUTF8 若字节非 UTF-8（多为 GBK/GB18030），转码为 UTF-8；已是 UTF-8 原样返回。
// 国内字幕站常发 GBK 编码，不转码播放器乱码。
func DecodeUTF8(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	// 尝试 GBK（GB18030 是 GBK 超集）解码
	dec := simplifiedchinese.GB18030.NewDecoder()
	out, err := io.ReadAll(transform.NewReader(bytes.NewReader(data), dec))
	if err != nil || len(out) == 0 {
		return data // 转码失败退回原字节
	}
	return out
}
