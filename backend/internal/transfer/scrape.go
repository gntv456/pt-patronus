package transfer

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/hxpt/ptpatronus/internal/media"
)

// ImageFetcher 取图片字节（chain 注入 httpclient 实现，避免 transfer→httpclient 依赖）。
type ImageFetcher func(ctx context.Context, url string) ([]byte, error)

// NFOWriter 抽象 NFO 生成（media.NFO 的签名），便于解耦与替换（如 v2 富 NFO）。
type NFOWriter func(mi *media.MediaInfo) string

// writeScrape 按 policy 把 NFO + 海报 + 背景写到 dir，返回已写入文件相对名。
// nfoName：movie.nfo / tvshow.nfo / <stem>.nfo。
func writeScrape(ctx context.Context, dir, nfoName string, mi *media.MediaInfo, policy ScrapePolicy, nfoWrite NFOWriter, fetch ImageFetcher) []string {
	if policy == ScrapeSkip || mi == nil {
		return nil
	}
	var written []string

	// NFO
	nfoPath := filepath.Join(dir, nfoName)
	if shouldWrite(nfoPath, policy) {
		if nfoWrite != nil {
			if err := os.WriteFile(nfoPath, []byte(nfoWrite(mi)), 0o644); err == nil {
				written = append(written, nfoName)
			} else {
				slog.Debug("write nfo failed", "path", nfoPath, "err", err)
			}
		}
	}

	// 海报 poster.jpg
	if mi.PosterURL != "" {
		if name, ok := downloadImageTo(ctx, dir, "poster.jpg", mi.PosterURL, policy, fetch); ok {
			written = append(written, name)
		}
	}
	// 背景 fanart.jpg
	if mi.BackdropURL != "" {
		if name, ok := downloadImageTo(ctx, dir, "fanart.jpg", mi.BackdropURL, policy, fetch); ok {
			written = append(written, name)
		}
	}
	return written
}

// downloadImageTo 下载单图到 dir/name，按 policy 决定是否覆盖。返回 (文件名, 是否写入)。
func downloadImageTo(ctx context.Context, dir, name, url string, policy ScrapePolicy, fetch ImageFetcher) (string, bool) {
	dest := filepath.Join(dir, name)
	if !shouldWrite(dest, policy) {
		return name, false
	}
	if fetch == nil {
		return name, false
	}
	b, err := fetch(ctx, url)
	if err != nil || len(b) == 0 {
		slog.Debug("download image failed", "url", url, "err", err)
		return name, false
	}
	if err := os.WriteFile(dest, b, 0o644); err != nil {
		return name, false
	}
	return name, true
}

// shouldWrite 按 policy + 文件是否已存在决定是否写。
func shouldWrite(path string, policy ScrapePolicy) bool {
	if policy == ScrapeOverwrite {
		return true
	}
	// missing_only：仅不存在时写
	return !pathExists(path)
}

// videoExts 可整理的视频扩展名（小写）。
var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".ts": true, ".mov": true,
	".wmv": true, ".flv": true, ".m4v": true, ".iso": true, ".rmvb": true, ".rm": true,
}

// isVideoFile 是否视频文件（按扩展名）。
func isVideoFile(name string) bool {
	return videoExts[strings.ToLower(filepath.Ext(name))]
}

// isSample 样本/花絮/广告文件（跳过）。
func isSample(name string) bool {
	low := strings.ToLower(name)
	return strings.Contains(low, "sample") || strings.Contains(low, "extra") ||
		strings.Contains(low, "trailer") || strings.Contains(low, "花絮")
}
