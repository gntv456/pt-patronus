package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/subtitle"
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
	// Queued 本轮真正排进后台队列的条目数：在受理那一刻确定，之后不再变动。
	// Fixed/Skipped/Errors 由 goroutine 逐条累加（全程持 subtitleFix.mu），调用方在受理时
	// 读它们只能拿到全 0 的假快照，而且是并发读——最终账面写进通知（SourceSubtitleFix）。
	Queued  int      `json:"queued"`
	Fixed   int      `json:"fixed"`   // 成功落盘数
	Skipped int      `json:"skipped"` // 搜索无结果/下载失败
	Errors  []string `json:"errors,omitempty"`
}

// ErrSubtitleFixRunning 上一次补齐仍在执行（防重入：并发触发会对同一 dst
// 非原子并发写，产生截断/交错的坏字幕且健康中心不再复检，无自愈路径）。
var ErrSubtitleFixRunning = errors.New("字幕补齐正在执行中，请稍候")

// subtitleFixState 防重入 + 报告计数的同步原语（per-chain，挂 Base 旁）。
type subtitleFixState struct {
	running atomic.Bool
	mu      sync.Mutex // 保护后台 goroutine 对 out 计数字段的递增
}

// FixMissingSubtitles 对健康报告的 missing_subtitle 条目批量补字幕。
// language 为偏好语言（默认 chs）；异步执行（后台 goroutine），立即返回受理数。
// 返回的 report 指针会在后台持续更新（读侧有锁），前端可轮询/忽略——
// 立即返回的 JSON 只是受理快照（Accepted=0 计数），真实结果走通知中心。
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
		// 只接受媒体文件路径：Series 目录 Path（或客户端手写报告）会算出库根落盘路径，
		// 写出「剧集名.chs.srt」幻影文件并反过来遮蔽后续全库检测。
		if i.Type == "missing_subtitle" && subtitleCheckablePath(i.Path) {
			targets = append(targets, i)
		}
	}
	if len(targets) == 0 {
		// 一部都没排进队列：不起 goroutine，也不会发通知。调用方必须把 queued=0 当
		// 终态报出去，否则界面只能回一句「已受理」，用户等一条永远不会来的通知。
		return &SubtitleFixReport{}, nil
	}
	if !c.subtitleFix.running.CompareAndSwap(false, true) {
		return nil, ErrSubtitleFixRunning
	}
	out := &SubtitleFixReport{Queued: len(targets)}
	go func() {
		defer c.subtitleFix.running.Store(false)
		actx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		for _, t := range targets {
			query := subtitleQueryFromPath(t.Path, t.Title)
			if query == "" {
				c.subtitleFix.mu.Lock()
				out.Skipped++
				c.subtitleFix.mu.Unlock()
				continue
			}
			subs, err := fixer.Search(actx, query, language)
			if err != nil {
				// 搜索失败 ≠ 「这部片没字幕」：以前和空结果合成同一个 Skipped++，通知正文只报
				// 「成功 N，跳过 M」，字幕源全崩听起来像库里本来就没字幕（同 #75 写失败混进 skipped）。
				c.subtitleFix.mu.Lock()
				out.Errors = append(out.Errors, t.Title+": 字幕搜索失败: "+errtext.Text(err))
				c.subtitleFix.mu.Unlock()
				slog.Warn("subtitle fix: search failed", "title", t.Title, "err", err)
				continue
			}
			if len(subs) == 0 {
				c.subtitleFix.mu.Lock()
				out.Skipped++
				c.subtitleFix.mu.Unlock()
				continue
			}
			// 语言优先级：精确匹配偏好 → chs → bi → eng → 任意
			pick := pickSubtitle(subs, language)
			if pick == nil {
				c.subtitleFix.mu.Lock()
				out.Skipped++
				c.subtitleFix.mu.Unlock()
				continue
			}
			data, fname, err := fixer.Download(actx, *pick)
			if err != nil || len(data) == 0 {
				if err == nil {
					err = errors.New("下载内容为空")
				}
				c.subtitleFix.mu.Lock()
				out.Errors = append(out.Errors, t.Title+": 字幕下载失败: "+errtext.Text(err))
				c.subtitleFix.mu.Unlock()
				slog.Warn("subtitle fix: download failed", "title", t.Title, "err", err)
				continue
			}
			dst := subtitleDestPath(t.Path, pick.LangCode, fname)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				c.subtitleFix.mu.Lock()
				out.Errors = append(out.Errors, t.Title+": "+err.Error())
				c.subtitleFix.mu.Unlock()
				continue
			}
			if err := writeFileAtomic(dst, data, 0o644); err != nil {
				c.subtitleFix.mu.Lock()
				out.Errors = append(out.Errors, t.Title+": "+err.Error())
				c.subtitleFix.mu.Unlock()
				continue
			}
			c.subtitleFix.mu.Lock()
			out.Fixed++
			c.subtitleFix.mu.Unlock()
			slog.Info("subtitle fix: saved", "file", filepath.Base(dst))
		}
		// 结果发通知（后台异步，调用方只拿受理快照；前端不轮询这份报告，账面必须写进正文）。
		if c.notice != nil {
			level := "success"
			title := "字幕补齐完成"
			body := fmt.Sprintf("成功 %d，跳过 %d", out.Fixed, out.Skipped)
			if len(out.Errors) > 0 {
				level = "warning"
				title = "字幕补齐结束（有失败）"
				reasons := out.Errors
				if len(reasons) > 2 {
					reasons = append(append([]string{}, reasons[:2]...), fmt.Sprintf("…另 %d 条", len(reasons)-2))
				}
				body = fmt.Sprintf("成功 %d，跳过 %d，失败 %d：%s", out.Fixed, out.Skipped, len(out.Errors), strings.Join(reasons, "；"))
			} else if out.Fixed == 0 {
				level = "warning"
			}
			_ = c.notice.Notice(context.Background(), notice.Input{
				Source: notice.SourceSubtitleFix,
				Title:  title,
				Body:   body,
				Level:  level,
			}, false)
		}
	}()
	return out, nil
}

// writeFileAtomic tmp+rename 原子写（崩溃/并发不产生半截文件）。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
