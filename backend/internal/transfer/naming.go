package transfer

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
)

// namingCtx 是命名模板的上下文（对齐 MP 的 RENAME_FORMAT 变量集，精简）。
type namingCtx struct {
	Title        string
	Year         int
	Resolution   string
	ReleaseGroup string
	Season       int
	Episode      int
	SeasonEpisode string
}

// renderName 渲染命名模板（Go text/template）。失败回退 "Title (Year)"。
func renderName(tplText string, c namingCtx) string {
	tpl, err := template.New("rename").Parse(tplText)
	if err != nil {
		return fallbackName(c)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, c); err != nil {
		return fallbackName(c)
	}
	name := sanitizePath(buf.String())
	name = strings.Trim(name, " -.")
	if name == "" {
		return fallbackName(c)
	}
	return name
}

func fallbackName(c namingCtx) string {
	if c.Year > 0 {
		return sanitizePath(fmt.Sprintf("%s (%d)", c.Title, c.Year))
	}
	return sanitizePath(c.Title)
}

// destPathForMovie 计算电影目标：库/[type]/[category]/Title (Year)/<name>.ext + nfo/图。
// category 非空且 cfg.CategoryFolder 时插入分类子目录。返回 (目标文件完整路径, 目标目录, 命名串)。
func destPathForMovie(cfg Config, baseDir, category, name, ext string) (string, string) {
	dir := libraryDir(cfg, baseDir, "movie", category)
	movieFolder := sanitizePath(name)
	targetDir := filepath.Join(dir, movieFolder)
	return filepath.Join(targetDir, name+ext), targetDir
}

// destPathForEpisode 计算剧集单集目标：库/[type]/[category]/Title (Year)/Season NN/<name>.ext。
// showDir = 库/.../Title (Year)；返回 (集文件路径, season目录, show目录)。
func destPathForEpisode(cfg Config, baseDir, category, showFolder, seasonFolder, name, ext string) (string, string, string) {
	dir := libraryDir(cfg, baseDir, "tv", category)
	showDir := filepath.Join(dir, showFolder)
	seasonDir := filepath.Join(showDir, seasonFolder)
	return filepath.Join(seasonDir, name+ext), seasonDir, showDir
}

// libraryDir 库根 + 可选 type 子目录（movie/tv）+ 可选 category 子目录。
func libraryDir(cfg Config, baseDir, typeKey, category string) string {
	dir := baseDir
	if cfg.TypeFolder {
		dir = filepath.Join(dir, typeKey)
	}
	if cfg.CategoryFolder && strings.TrimSpace(category) != "" {
		dir = filepath.Join(dir, sanitizePath(category))
	}
	return dir
}

// showFolderName 剧集库目录名：Title (Year)。
func showFolderName(title string, year int) string {
	return sanitizePath(folderTitleYear(title, year))
}

func folderTitleYear(title string, year int) string {
	if year > 0 {
		return fmt.Sprintf("%s (%d)", title, year)
	}
	return title
}

// seasonFolderName "Season 01"（Kodi/Emby/JF 标准）。
func seasonFolderName(season int) string {
	if season <= 0 {
		return "Season 00" // 特殊集惯例
	}
	return fmt.Sprintf("Season %s", pad2(season))
}

// sanitizePath 去除路径非法字符（Windows/Linux 通用）：\ / : * ? " < > |。
func sanitizePath(s string) string {
	r := strings.NewReplacer(
		"/", " ", "\\", " ", ":", " ", "*", " ",
		"?", " ", "\"", "'", "<", "(", ">", ")", "|", "-")
	s = r.Replace(s)
	return collapseSpaces(s)
}
