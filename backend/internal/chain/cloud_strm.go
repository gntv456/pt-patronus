package chain

// STRM 生成链路：按规则扫描网盘 → 写本地 .strm → 落库记录；含目录标签解析、
// 即时生成、全量生成与订阅追更同步。从 cloud.go 迁出。

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"strings"

	"github.com/hxpt/ptpatronus/internal/cloud"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// ---- STRM 生成 ----

// GenerateStrm 按单条规则扫描网盘生成本地 .strm。
// OutputTag 非空时优先解析目录标签（transfer.dir_tags，local 型）为输出目录——
// 标签不存在/类型不符时报错（显式失败优于静默落错目录）。
func (c *CloudChain) GenerateStrm(ctx context.Context, ruleID uint) (cloud.GenerateResult, error) {
	rule, err := c.ruleOp.Get(ruleID)
	if err != nil {
		return cloud.GenerateResult{}, err
	}
	cli, drive, err := c.client(rule.DriveID)
	if err != nil {
		return cloud.GenerateResult{}, err
	}
	outputDir := rule.OutputDir
	if rule.OutputTag != "" {
		tag, err := c.resolveLocalDirTag(rule.OutputTag)
		if err != nil {
			return cloud.GenerateResult{}, err
		}
		outputDir = tag
	}
	return cloud.Generate(ctx, cli, cloud.GenerateOptions{
		DriveID:         rule.DriveID,
		CloudPath:       clampCloudPath(drive.RootPath, rule.CloudPath),
		OutputDir:       outputDir,
		MountRoot:       rule.MountRoot,
		Template:        rule.Template,
		Recursive:       rule.Recursive,
		KeepExt:         rule.KeepExt,
		SkipExisting:    rule.Incremental,
		IncludeExts:     splitCSV(rule.IncludeExts),
		ExcludeKeywords: splitCSV(rule.ExcludeKeywords),
		LocalBaseURL:    rule.LocalBaseURL,
		PublicBaseURL:   rule.PublicBaseURL,
		FallbackBaseURL: c.appBaseURL,
		// 二轮 P1-1 修复：规则生成路径同样接入限时签名（此前只有上传路径有，
		// {direct} 模板的规则 .strm 在签名模式下全 404）。nil=兼容模式自动降级。
		SignFunc: c.SignDirectLink,
		OnFile: func(written bool, cloudPath, strmPath string, size int64) {
			if _, err := c.ruleOp.UpsertRecord(rule.DriveID, cloudPath, strmPath, size, 0); err != nil {
				slog.Warn("cloud: upsert strm record failed", "err", err)
			}
		},
	})
}

// resolveLocalDirTag 解析 local 型目录标签为本机路径；cloud 型/不存在报错。
func (c *CloudChain) resolveLocalDirTag(name string) (string, error) {
	if c.transferCfgGetter == nil {
		return "", fmt.Errorf("目录标签解析不可用（未注入配置读取器）: %s", name)
	}
	tags := LoadDirTagsFromKV(c.transferCfgGetter)
	for _, t := range tags {
		if t.Name == name {
			if t.Kind != "local" {
				return "", fmt.Errorf("目录标签 %s 是 %s 型，STRM 输出目录需 local 型", name, t.Kind)
			}
			return t.Path, nil
		}
	}
	return "", fmt.Errorf("目录标签不存在: %s", name)
}

// splitCSV 逗号分隔字符串拆片（去空白；空串返回 nil）。
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// dirTagItem 目录标签镜像（transfer.DirTag 的最小副本，避免 chain→transfer 依赖环；
// 字段与 transfer.dir_tags 存储格式严格一致）。
type dirTagItem struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	DriveID uint   `json:"drive_id,omitempty"`
	Path    string `json:"path"`
	Note    string `json:"note,omitempty"`
}

// LoadDirTagsFromKV 从 SystemConfig 读 transfer.dir_tags 标签表（读失败返回空）。
func LoadDirTagsFromKV(kv KVGetter) []dirTagItem {
	if kv == nil {
		return nil
	}
	raw, err := kv.Get("transfer.dir_tags")
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var ts []dirTagItem
	if json.Unmarshal([]byte(raw), &ts) != nil {
		return nil
	}
	return ts
}

// GenerateInput 即时 STRM 生成参数（文件页选中目录直接生成，不持久化为规则）。
type GenerateInput struct {
	DriveID         uint   `json:"drive_id"`
	CloudPath       string `json:"cloud_path"`
	OutputDir       string `json:"output_dir"`
	OutputTag       string `json:"output_tag"` // 输出目录标签（非空优先于 OutputDir）
	MountRoot       string `json:"mount_root"`
	Template        string `json:"template"`
	Recursive       bool   `json:"recursive"`
	KeepExt         bool   `json:"keep_ext"`
	IncludeExts     string `json:"include_exts"`     // 扩展名白名单（逗号分隔；空=全部）
	ExcludeKeywords string `json:"exclude_keywords"` // 文件名排除关键词（逗号分隔）
	LocalBaseURL    string `json:"local_base_url"`   // 内网基址（{direct} 用）
	PublicBaseURL   string `json:"public_base_url"`  // 外网基址（{directPublic} 用）
}

// GenerateInstant 即时生成 STRM（不持久化规则；仍 upsert 记录便于追溯删除）。
func (c *CloudChain) GenerateInstant(ctx context.Context, in GenerateInput) (cloud.GenerateResult, error) {
	cli, drive, err := c.client(in.DriveID)
	if err != nil {
		return cloud.GenerateResult{}, err
	}
	outputDir := in.OutputDir
	if in.OutputTag != "" {
		tag, err := c.resolveLocalDirTag(in.OutputTag)
		if err != nil {
			return cloud.GenerateResult{}, err
		}
		outputDir = tag
	}
	return cloud.Generate(ctx, cli, cloud.GenerateOptions{
		DriveID:         in.DriveID,
		CloudPath:       clampCloudPath(drive.RootPath, in.CloudPath),
		OutputDir:       outputDir,
		MountRoot:       in.MountRoot,
		Template:        in.Template,
		Recursive:       in.Recursive,
		KeepExt:         in.KeepExt,
		IncludeExts:     splitCSV(in.IncludeExts),
		ExcludeKeywords: splitCSV(in.ExcludeKeywords),
		LocalBaseURL:    in.LocalBaseURL,
		PublicBaseURL:   in.PublicBaseURL,
		FallbackBaseURL: c.appBaseURL,
		SignFunc:        c.SignDirectLink, // 二轮 P1-1：即时生成同口径接入签名
		OnFile: func(written bool, cloudPath, strmPath string, size int64) {
			if _, err := c.ruleOp.UpsertRecord(in.DriveID, cloudPath, strmPath, size, 0); err != nil {
				slog.Debug("cloud: upsert strm record failed", "err", err)
			}
		},
	})
}

// GenerateAll 遍历启用规则生成 STRM（cron 入口）。单规则失败不阻断其余。
func (c *CloudChain) GenerateAll(ctx context.Context) {
	rules, err := c.ruleOp.ListEnabled()
	if err != nil {
		slog.Warn("cloud: list rules failed", "err", err)
		return
	}
	for _, r := range rules {
		res, err := c.GenerateStrm(ctx, r.ID)
		if err != nil {
			slog.Warn("cloud: generate rule failed", "rule", r.Name, "err", err)
			continue
		}
		slog.Info("cloud: strm generated", "rule", r.Name, "generated", res.Generated, "skipped", res.Skipped, "errors", len(res.Errors))
		// 本轮有新落地内容 → 触发一次媒体库扫库（生成与扫描解耦，别让 Emby 等 6h 下一轮）。
		if res.Generated > 0 && c.libRefresher != nil {
			c.libRefresher("strm:" + r.Name)
		}
		c.Emit(ctx, "CloudStrmGenerated", map[string]any{"rule": r.Name, "generated": res.Generated, "skipped": res.Skipped})
	}
	// 二轮 P0-2 修复：签名续签——上传型 .strm（不在任何规则的 CloudPath 子树下）
	// 此前只在上传瞬间写一次签名，过期后唯一恢复是管理员手动 repair。每轮 cron
	// 对全部记录按盘重签（幂等：新 exp 内容不同才写盘）。规则未覆盖的盘也重签。
	c.ResignAllStrm(ctx)
}

// ResignAllStrm 按盘遍历 STRM 记录重写 .strm（刷新签名有效期）。幂等、best-effort。
func (c *CloudChain) ResignAllStrm(ctx context.Context) {
	drives, err := c.op.List()
	if err != nil {
		slog.Warn("cloud: resign list drives failed", "err", err)
		return
	}
	for _, d := range drives {
		repaired, skipped, err := c.RepairStrm(ctx, d.ID)
		if err != nil {
			slog.Warn("cloud: resign drive failed", "drive", d.ID, "err", err)
			continue
		}
		if repaired > 0 {
			slog.Info("cloud: strm resigned", "drive", d.ID, "repaired", repaired, "skipped", skipped)
		}
	}
}

// ---- 订阅追更 ----

// SyncSubscriptions 把已生成的 STRM 记录匹配到媒体订阅：命中即链接（记 SubscriptionID）+ 通知。
// 设计：追更复用「STRM 规则已扫描落库的 StrmRecord」（不重复扫盘），按文件名归一化匹配订阅标题。
// 即用户先用 STRM 规则覆盖媒体目录，追更自动把其中的资源链接到订阅并提醒。
func (c *CloudChain) SyncSubscriptions(ctx context.Context) {
	if c.sub == nil {
		return
	}
	subs, err := c.sub.List()
	if err != nil || len(subs) == 0 {
		return
	}
	active := make([]models.Subscription, 0, len(subs))
	for _, s := range subs {
		if s.Status == "subscribed" {
			active = append(active, s)
		}
	}
	if len(active) == 0 {
		return
	}
	recs, err := c.ruleOp.ListRecords(0) // driveID=0 = 全部
	if err != nil {
		slog.Warn("cloud: list strm records failed", "err", err)
		return
	}
	for _, rec := range recs {
		if rec.SubscriptionID != 0 {
			continue // 已链接
		}
		matched := matchAnySubscription(active, path.Base(rec.CloudPath))
		if matched == nil {
			continue
		}
		if err := c.ruleOp.LinkRecord(rec.ID, matched.ID); err != nil {
			slog.Warn("cloud: link record failed", "err", err)
			continue
		}
		if c.notice != nil {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: "cloud",
				Key:    fmt.Sprintf("cloud:%d:%s", rec.DriveID, rec.CloudPath),
				Title:  fmt.Sprintf("订阅《%s》网盘资源已就绪", matched.Title),
				Body:   fmt.Sprintf("%s（已生成 STRM）", path.Base(rec.CloudPath)),
				Level:  "info",
			}, true)
		}
	}
}
