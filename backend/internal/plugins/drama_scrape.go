// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// drama-scrape / playlet-category 共享的视频扩展名与小工具。两个短剧插件从 Python
// external-http 移植为 Go builtin：编译期编入二进制，前端「已安装」天然出现，不再依赖市场源。
const defaultVideoExts = ".mp4,.mkv,.avi,.ts,.mov,.wmv,.flv,.m4v,.rmvb,.mpg,.mpeg,.webm"

const dramaUA = "PTPatronusPlugin/1.0 (drama-scrape)"

// parseVideoExts 把 "mp4,.mkv,avi" 之类解析为带点小写扩展名集合；空值用默认列表。
func parseVideoExts(value string) map[string]bool {
	if strings.TrimSpace(value) == "" {
		value = defaultVideoExts
	}
	m := map[string]bool{}
	for _, p := range strings.Split(value, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, ".") {
			p = "." + p
		}
		m[p] = true
	}
	return m
}

// listMediaFiles 递归遍历 folder，返回扩展名命中的文件绝对路径（对标 Python os.walk 过滤）。
func listMediaFiles(folder string, exts map[string]bool) []string {
	var out []string
	_ = filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if exts[strings.ToLower(filepath.Ext(d.Name()))] {
			out = append(out, path)
		}
		return nil
	})
	return out
}

// copyFileData 复制单文件并保留权限位（link/move 跨卷失败时的 fallback）。
func copyFileData(src, dst string) error {
	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, si.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func matchesAny(keywords []string, s string) bool {
	for _, k := range keywords {
		if k != "" && strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// ---------------- 寸光集（ptang.top）client ----------------
// 与 internal/media/cunguangji.go 同源（/api/explore），但 drama-scrape 走寸光集 ptang.top
// 需要 Bearer API key 鉴权，且要保留 totalEpisodes/region/casts 全字段写进 NFO，故独立实现。

type xgItem struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	AltTitle      string  `json:"altTitle"`
	Cover         string  `json:"cover"`
	Description   string  `json:"description"`
	ReleaseYear   int     `json:"releaseYear"`
	Rating        float64 `json:"rating"`
	Tags          string  `json:"tags"`
	Region        string  `json:"region"`
	TotalEpisodes int     `json:"totalEpisodes"`
	Status        string  `json:"status"`
}

type xgCast struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Avatar string `json:"avatar"`
}

type xgDetailItem struct {
	xgItem
	Casts []xgCast `json:"casts"`
}

func resolveDramaURL(base, u string) string {
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if strings.HasPrefix(u, "/") {
		return strings.TrimRight(base, "/") + u
	}
	return u
}

// xgGet 发 GET 并解码到 out；硬 20s deadline（context.WithTimeout 取消 in-flight 请求）。
func xgGet(ctx context.Context, base, key, path string, params map[string]string, out any) error {
	u := strings.TrimRight(base, "/") + path
	if len(params) > 0 {
		v := url.Values{}
		for k, val := range params {
			v.Set(k, val)
		}
		u += "?" + v.Encode()
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", dramaUA)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cunguangji %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func xgSearch(ctx context.Context, base, key, name string) ([]xgItem, error) {
	var r struct {
		Items []xgItem `json:"items"`
	}
	if err := xgGet(ctx, base, key, "/api/explore", map[string]string{"q": name, "page": "1", "pageSize": "10"}, &r); err != nil {
		return nil, err
	}
	return r.Items, nil
}

func xgDetail(ctx context.Context, base, key, idStr string) (*xgDetailItem, error) {
	var d xgDetailItem
	if err := xgGet(ctx, base, key, "/api/explore/"+url.PathEscape(idStr), nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// ---------------- 名称清洗 + 匹配（逐字照 Python）----------------

var (
	dramaBracketRE = regexp.MustCompile(`【.*?】|\[.*?\]|\(.*?\)|（.*?）|「.*?」`)
	dramaNoiseRE   = regexp.MustCompile(`(?i)(S\d{1,2}E?\d{0,3}|[Ee][Pp]?\d{1,3}|\d{3,4}[pP]|[48][Kk]|HEVC|H\.?26[45]|h264|AVC|10bit|GB|fps|X264|[Xx]26[45]|AAC|FLAC|WEB-DL|BluRay|BDRip|mp4|mkv)`)
	dramaSepRE     = regexp.MustCompile(`[._]+`)
	dramaSpaceRE   = regexp.MustCompile(`\s+`)
)

func dramaCleanName(raw string) string {
	s := dramaBracketRE.ReplaceAllString(raw, " ")
	s = dramaNoiseRE.ReplaceAllString(s, " ")
	s = dramaSepRE.ReplaceAllString(s, " ")
	s = dramaSpaceRE.ReplaceAllString(s, " ")
	return strings.Trim(s, " -—")
}

// dramaPickMatch 评分选最佳：100 精确 / 50 包含 / 10 兜底；空结果返回 nil。
func dramaPickMatch(name string, items []xgItem) *xgItem {
	if len(items) == 0 {
		return nil
	}
	target := strings.ToLower(name)
	var best *xgItem
	bestScore := -1
	for i := range items {
		it := &items[i]
		title := strings.ToLower(it.Title)
		alt := strings.ToLower(it.AltTitle)
		score := -1
		switch {
		case target != "" && (target == title || target == alt):
			score = 100
		case target != "" && (strings.Contains(title, target) || strings.Contains(alt, target) || strings.Contains(target, title)):
			score = 50
		case title != "":
			score = 10
		}
		if score > bestScore {
			best = it
			bestScore = score
		}
	}
	return best
}

// ---------------- tvshow.nfo（1:1 复刻 Python ET 输出）----------------

func nfoEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func nfoLine(b *strings.Builder, tag, val string) {
	fmt.Fprintf(b, "  <%s>%s</%s>\n", tag, nfoEscape(val), tag)
}

func nfoSplitTags(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// writeTVShowNFO 写 Kodi/Jellyfin 兼容的 tvshow.nfo；字段与条件逐项对齐 Python write_tvshow_nfo。
func writeTVShowNFO(path string, d *xgDetailItem) error {
	var b strings.Builder
	b.WriteString("<?xml version='1.0' encoding='utf-8'?>\n<tvshow>\n")
	nfoLine(&b, "title", d.Title)
	if d.AltTitle != "" {
		nfoLine(&b, "originaltitle", d.AltTitle)
	}
	nfoLine(&b, "sorttitle", d.Title)
	if d.Description != "" {
		nfoLine(&b, "plot", d.Description)
	}
	if d.ReleaseYear != 0 {
		nfoLine(&b, "year", strconv.Itoa(d.ReleaseYear))
	}
	for _, g := range nfoSplitTags(d.Tags) {
		nfoLine(&b, "genre", g)
	}
	if d.Region != "" {
		nfoLine(&b, "country", d.Region)
	}
	nfoLine(&b, "studio", "寸光集")
	if d.Rating > 0 {
		b.WriteString("  <ratings>\n")
		b.WriteString(`    <rating name="cunguangji" max="10" default="true">` + "\n")
		fmt.Fprintf(&b, "      <value>%s</value>\n", strconv.FormatFloat(d.Rating, 'f', -1, 64))
		b.WriteString("      <votes>0</votes>\n")
		b.WriteString("    </rating>\n  </ratings>\n")
	}
	fmt.Fprintf(&b, `  <uniqueid type="cunguangji" default="true">%s</uniqueid>`+"\n", nfoEscape(strconv.Itoa(d.ID)))
	for i, c := range d.Casts {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		b.WriteString("  <actor>\n")
		fmt.Fprintf(&b, "    <name>%s</name>\n", nfoEscape(c.Name))
		if c.Role != "" {
			fmt.Fprintf(&b, "    <role>%s</role>\n", nfoEscape(c.Role))
		}
		fmt.Fprintf(&b, "    <order>%d</order>\n", i)
		b.WriteString("  </actor>\n")
	}
	b.WriteString("  <season>1</season>\n")
	if d.TotalEpisodes != 0 {
		nfoLine(&b, "episode", strconv.Itoa(d.TotalEpisodes))
	}
	b.WriteString("</tvshow>")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// ---------------- 图片下载 + 文件整理 ----------------

func downloadDramaImage(ctx context.Context, imgURL, dest string) error {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", dramaUA)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("image status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

// transferDramaFile：link=硬链接(失败 fallback copy) / copy / move(Rename 失败 fallback copy+delete)。
func transferDramaFile(src, dst, mode string) error {
	switch mode {
	case "link":
		if err := os.Link(src, dst); err != nil {
			return copyFileData(src, dst)
		}
		return nil
	case "copy":
		return copyFileData(src, dst)
	case "move":
		if err := os.Rename(src, dst); err == nil {
			return nil
		}
		if err := copyFileData(src, dst); err != nil {
			return err
		}
		return os.Remove(src)
	}
	return nil
}
