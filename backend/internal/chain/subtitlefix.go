package chain

import (
	"context"

	"fmt"
	"github.com/hxpt/ptpatronus/internal/subtitle"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ---- 字幕库级补齐（媒体健康中心联动） ----
//
// 健康中心发现 missing_subtitle → 对每个条目按标题搜字幕（语言优先级 chs>cht>eng>bi）
// → 下载落盘到媒体同目录（文件名 = 媒体名 + 语言后缀 + .ext）。批量异步执行，
// 结果发通知；重复调用幂等（已有字幕跳过——健康中心天然过滤）。

// SubtitleFixer 字幕补齐能力依赖（*SubtitleChain 结构化满足）。
type SubtitleFixer interface {
	Search(ctx context.Context, query, language string) ([]subtitleItem, error)
	Download(ctx context.Context, sub subtitleItem) ([]byte, string, error)
}

// subtitleItem SubtitleChain 子项镜像（避免 chain→subtitle 类型耦合；适配在装配层）。
type subtitleItem struct {
	Source   string
	SourceID string
	Title    string
	LangCode string
	Ext      string
}

// SubtitleFixReport 补齐结果。
type SubtitleFixReport struct {
	Fixed   int      `json:"fixed"`   // 成功落盘数
	Skipped int      `json:"skipped"` // 搜索无结果/下载失败
	Errors  []string `json:"errors,omitempty"`
}

// FixMissingSubtitles 对健康报告的 missing_subtitle 条目批量补字幕。
// language 为偏好语言（默认 chs）；异步执行（后台 goroutine），立即返回受理数。
func (c *MediaServerChain) FixMissingSubtitles(ctx context.Context, rep *HealthReport, language string) (*SubtitleFixReport, error) {
	fixer := c.subtitleFixer
	if fixer == nil {
		return nil, fmt.Errorf("字幕服务未配置")
	}
	if language == "" {
		language = "chs"
	}
	var targets []HealthIssue
	for _, i := range rep.Issues {
		if i.Type == "missing_subtitle" && i.Path != "" {
			targets = append(targets, i)
		}
	}
	if len(targets) == 0 {
		return &SubtitleFixReport{}, nil
	}
	out := &SubtitleFixReport{}
	go func() {
		actx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		for _, t := range targets {
			query := subtitleQueryFromPath(t.Path, t.Title)
			if query == "" {
				out.Skipped++
				continue
			}
			subs, err := fixer.Search(actx, query, language)
			if err != nil || len(subs) == 0 {
				out.Skipped++
				slog.Debug("subtitle fix: no result", "title", t.Title, "err", err)
				continue
			}
			// 语言优先级：精确匹配偏好 → chs → bi → eng → 任意
			pick := pickSubtitle(subs, language)
			if pick == nil {
				out.Skipped++
				continue
			}
			data, fname, err := fixer.Download(actx, *pick)
			if err != nil || len(data) == 0 {
				out.Skipped++
				continue
			}
			dst := subtitleDestPath(t.Path, pick.LangCode, fname)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				out.Errors = append(out.Errors, t.Title+": "+err.Error())
				continue
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				out.Errors = append(out.Errors, t.Title+": "+err.Error())
				continue
			}
			out.Fixed++
			slog.Info("subtitle fix: saved", "file", filepath.Base(dst))
		}
	}()
	return out, nil
}

// SetSubtitleFixer 注入字幕补齐能力（app 装配）。
func (c *MediaServerChain) SetSubtitleFixer(f SubtitleFixer) { c.subtitleFixer = f }

// pickSubtitle 按语言优先级挑字幕。
func pickSubtitle(subs []subtitleItem, prefer string) *subtitleItem {
	order := []string{prefer, "chs", "bi", "cht", "eng"}
	for _, lang := range order {
		for i := range subs {
			if subs[i].LangCode == lang {
				return &subs[i]
			}
		}
	}
	if len(subs) > 0 {
		return &subs[0]
	}
	return nil
}

// subtitleQueryFromPath 搜索词：文件名去扩展（ParseMeta 清洗在 subtitle 源内做）。
func subtitleQueryFromPath(path, title string) string {
	if title != "" {
		return title
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// subtitleDestPath 字幕落盘路径：媒体同目录 + 媒体名（去扩展）+ 语言 + 源扩展名。
func subtitleDestPath(mediaPath, lang, downloadedName string) string {
	dir := filepath.Dir(mediaPath)
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	ext := filepath.Ext(downloadedName)
	if ext == "" || len(ext) > 5 {
		ext = ".srt"
	}
	if lang != "" {
		return filepath.Join(dir, base+"."+lang+ext)
	}
	return filepath.Join(dir, base+ext)
}

// SubtitleChainAdapter 把 *SubtitleChain 适配为 SubtitleFixer（subtitle.Subtitle ↔ subtitleItem）。
type SubtitleChainAdapter struct{ Ch *SubtitleChain }

func (a SubtitleChainAdapter) Search(ctx context.Context, query, language string) ([]subtitleItem, error) {
	subs, err := a.Ch.Search(ctx, query, language)
	if err != nil {
		return nil, err
	}
	out := make([]subtitleItem, 0, len(subs))
	for _, s := range subs {
		out = append(out, subtitleItem{Source: s.Source, SourceID: s.SourceID, Title: s.Title, LangCode: s.LangCode, Ext: s.Ext})
	}
	return out, nil
}

func (a SubtitleChainAdapter) Download(ctx context.Context, s subtitleItem) ([]byte, string, error) {
	return a.Ch.Download(ctx, subtitleFromItem(s))
}

func subtitleFromItem(s subtitleItem) subtitle.Subtitle {
	return subtitle.Subtitle{Source: s.Source, SourceID: s.SourceID, Title: s.Title, LangCode: s.LangCode, Ext: s.Ext}
}
