// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ---- 云端整理（CloudOrganize） ----
//
// 对「已在云盘上的文件」（转存/离线下载来的散乱结构）做识别→重命名→移动到云端
// 分类目录——复用 MediaChain 识别（经接口
// 注入，断 cloud→chain 依赖），目标从本地路径换成网盘路径，经 FileManager 执行。

// Organizer 媒体识别能力（chain.MediaChain 结构化满足）。
type Organizer interface {
	// Recognize 按名称识别媒体，返回 标题/年份/类型(movie|tv)/季集。
	Recognize(ctx context.Context, name string) (*OrganizeMeta, error)
}

// OrganizeMeta 识别结果（云端整理所需子集）。
type OrganizeMeta struct {
	Title    string
	Year     int
	Type     string // movie | tv
	Season   string // S01（剧集）
	Episodes []string
}

// OrganizeOptions 云端整理参数。
type OrganizeOptions struct {
	DriveID       uint
	SrcDir        string // 云盘待整理目录
	DstMovieDir   string // 云盘电影分类目录
	DstTVDir      string // 云盘剧集分类目录
	MediaOnly     bool   // 仅媒体文件（默认 true）
	KeepStructure bool   // 保持原目录结构（默认 false：按识别结果归位）
	Rename        bool   // 识别重命名（默认 true）
	Max           int    // 单次处理上限（防失控；默认 200）
	OnFile        func(done int, cloudPath, action string)
}

// OrganizeResult 整理汇总。
type OrganizeResult struct {
	Scanned int      `json:"scanned"`
	Moved   int      `json:"moved"`
	Renamed int      `json:"renamed"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

// Organize 执行云端整理：列目录 → 逐文件识别 → 计算目标路径 →（可选）重命名 → 移动。
// 不可识别文件留在原处（记 skipped），单文件失败不阻断。
func Organize(ctx context.Context, cli Client, org Organizer, opts OrganizeOptions) (*OrganizeResult, error) {
	fm, ok := cli.(FileManager)
	if !ok {
		return nil, fmt.Errorf("网盘类型不支持文件管理操作")
	}
	if opts.Max <= 0 {
		opts.Max = 200
	}
	res := &OrganizeResult{}
	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		if depth > 6 {
			return nil // 深度上限：防环形结构
		}
		files, err := cli.List(ctx, dir)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("列目录失败 %s: %v", dir, err))
			return nil
		}
		for _, f := range files {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if res.Scanned >= opts.Max {
				return nil
			}
			if f.IsDir {
				// 单文件模式下目录仅作遍历；保持结构模式下目录整体跳过（不迁移目录）
				if err := walk(f.Path, depth+1); err != nil {
					return err
				}
				continue
			}
			res.Scanned++
			if opts.MediaOnly && !IsMediaFile(f.Name) {
				res.Skipped++
				continue
			}
			// 识别
			meta, err := org.Recognize(ctx, f.Name)
			if err != nil || meta == nil || meta.Title == "" {
				res.Skipped++
				continue
			}
			// 目标目录 + 文件名
			dstDir, newName := organizeTarget(opts, meta, f.Name)
			if dstDir == "" {
				res.Skipped++
				continue
			}
			action := "moved"
			// 重命名（先 rename 后 move，避免网盘端一次操作两个语义）
			if opts.Rename && newName != f.Name {
				if err := fm.Rename(ctx, f.Path, newName); err != nil {
					res.Errors = append(res.Errors, fmt.Sprintf("重命名失败 %s: %v", f.Path, err))
					res.Skipped++
					continue
				}
				res.Renamed++
				f.Path = cloudDirOf(f.Path) + "/" + newName
				action = "renamed+moved"
			}
			if err := fm.Move(ctx, f.Path, dstDir); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("移动失败 %s → %s: %v", f.Path, dstDir, err))
				res.Skipped++
				continue
			}
			res.Moved++
			if opts.OnFile != nil {
				opts.OnFile(res.Moved, f.Path, action)
			}
		}
		return nil
	}
	if err := walk(opts.SrcDir, 0); err != nil {
		return res, err
	}
	sort.Strings(res.Errors)
	return res, nil
}

// organizeTarget 计算目标目录与文件名（电影→DstMovieDir/标题 (年份)/，
// 剧集→DstTVDir/标题/季/；未配置对应目录返回空跳过）。
func organizeTarget(opts OrganizeOptions, meta *OrganizeMeta, filename string) (dstDir, newName string) {
	// 剥离识别不相关的前缀噪声（发布组/站点名）——识别器已尽力，文件名保持原名即可；
	// 重命名仅做「目录化归位」，文件本体名保留（PT 场景文件名含音视频信息，改了不利洗版比对）。
	if meta.Type == "tv" {
		if opts.DstTVDir == "" {
			return "", ""
		}
		dstDir = strings.TrimRight(opts.DstTVDir, "/") + "/" + sanitizeName(meta.Title)
		if meta.Season != "" {
			dstDir += "/" + meta.Season
		}
		return dstDir, filename
	}
	if opts.DstMovieDir == "" {
		return "", ""
	}
	dir := sanitizeName(meta.Title)
	if meta.Year > 0 {
		dir = fmt.Sprintf("%s (%d)", dir, meta.Year)
	}
	return strings.TrimRight(opts.DstMovieDir, "/") + "/" + dir, filename
}

// sanitizeName 标题清洗为安全目录名（去路径分隔符与 Windows 非法字符）。
func sanitizeName(s string) string {
	repl := strings.NewReplacer("/", " ", "\\", " ", ":", " ", "*", " ", "?", " ",
		"\"", " ", "<", " ", ">", " ", "|", " ")
	out := strings.TrimSpace(repl.Replace(s))
	if out == "" {
		out = "未命名"
	}
	return out
}
