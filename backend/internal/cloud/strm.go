package cloud

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// STRM 文本文件：内容单行，指向真实播放路径（云盘挂载路径或直链）。
// 媒体服务器（Emby/JF/Plex）读取 .strm 后按该路径直链播放云盘资源，无需本地落地。

const defaultStrmTemplate = "{mount}{cloudpath}"

// GenerateOptions STRM 生成参数（由 StrmRule / 订阅追更映射而来）。
type GenerateOptions struct {
	DriveID   uint
	CloudPath string // 云盘源目录（扫描起点，以 / 起）
	OutputDir string // 本地输出目录（.strm 落地根）
	MountRoot string // 挂载根（模板 {mount} 替换，如 /CloudDrive2/115）
	Template  string // 路径模板，空=默认 {mount}{cloudpath}
	Recursive bool   // 递归扫描子目录
	KeepExt   bool   // .strm 文件名保留源扩展名（a.mkv→a.mkv.strm）否则 a.strm
	SubID     uint   // 订阅追更传入（规则生成=0）；透传给 OnFile 供记录 linkage
	// OnFile 每个媒体文件回调：written=是否新建/更新（false=本地已存在且内容相同）。
	OnFile func(written bool, cloudPath, strmPath string, size int64)
}

// GenerateResult STRM 生成汇总。
type GenerateResult struct {
	Generated int      `json:"generated"` // 新建/更新的 .strm 数
	Skipped   int      `json:"skipped"`   // 本地已存在且内容相同，跳过
	Errors    []string `json:"errors"`    // 单文件失败（不阻断整体）
}

// Generate 遍历 cli 在 CloudPath 下的媒体文件，按模板生成本地 .strm。
// 本地幂等：目标 .strm 已存在且内容相同则跳过（written=false）。单文件失败收集到 Errors 不阻断。
func Generate(ctx context.Context, cli Client, opts GenerateOptions) (GenerateResult, error) {
	opts.CloudPath = cleanCloudPath(opts.CloudPath)
	if opts.Template == "" {
		opts.Template = defaultStrmTemplate
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return GenerateResult{}, fmt.Errorf("创建输出目录失败: %w", err)
	}

	var res GenerateResult
	const maxFiles = 20000 // 软上限：防止巨型网盘失控（超出记录到 Errors 后停止）
	count := 0

	var walk func(dir string) error
	walk = func(dir string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		files, err := cli.List(ctx, dir)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("列目录失败 %s: %v", dir, err))
			return nil // 单目录失败不阻断
		}
		for _, f := range files {
			if f.IsDir {
				if opts.Recursive {
					if err := walk(f.Path); err != nil {
						return err
					}
				}
				continue
			}
			if !IsMediaFile(f.Name) {
				continue
			}
			count++
			if count > maxFiles {
				res.Errors = append(res.Errors, fmt.Sprintf("已达单次生成上限 %d，剩余文件未处理", maxFiles))
				return errStopWalk
			}
			written, strmPath, err := writeStrm(opts, f)
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("写入失败 %s: %v", f.Path, err))
				continue
			}
			if written {
				res.Generated++
			} else {
				res.Skipped++
			}
			if opts.OnFile != nil {
				opts.OnFile(written, f.Path, strmPath, f.Size)
			}
		}
		return nil
	}

	if err := walk(opts.CloudPath); err != nil && err != errStopWalk {
		return res, err
	}
	return res, nil
}

// errStopWalk 软上限触发时中止递归（非真实错误）。
var errStopWalk = fmt.Errorf("stop walk")

// isWithinDir 判断 path（清理后）是否位于 dir 之内或等于 dir。防 STRM 写入逃出 OutputDir。
func isWithinDir(path, dir string) bool {
	cp := filepath.Clean(path)
	cd := filepath.Clean(dir)
	if cp == cd {
		return true
	}
	return strings.HasPrefix(cp, cd+string(os.PathSeparator))
}

// writeStrm 生成单个 .strm：计算本地路径 + 渲染内容 + 幂等写入。返回 (是否写入, strm路径, err)。
func writeStrm(opts GenerateOptions, f File) (bool, string, error) {
	rel := strings.TrimPrefix(f.Path, opts.CloudPath) // CloudPath 下相对路径，含前导 /
	relDir := filepath.FromSlash(filepath.Dir(strings.TrimPrefix(rel, "/")))
	strmName := filepath.Base(f.Name)
	if opts.KeepExt {
		strmName += ".strm"
	} else {
		strmName = strings.TrimSuffix(strmName, filepath.Ext(strmName)) + ".strm"
	}
	outPath := filepath.Join(opts.OutputDir, relDir, strmName)
	// 路径穿越防御：若云条目名含 ".." 等被 Join 折叠后逃出 OutputDir，拒绝写入（防御纵深；
	// 当前 115/夸克/123 API 禁 "/" 和 ".." 故不可利用，但写点是安全边界）。
	if !isWithinDir(outPath, opts.OutputDir) {
		return false, outPath, fmt.Errorf("strm 输出路径逃出 OutputDir: %s", outPath)
	}

	content := RenderTemplate(opts.Template, opts.MountRoot, f.Path, rel, f.Name) + "\n"

	// 幂等：已存在且内容相同则跳过。
	if old, err := os.ReadFile(outPath); err == nil && string(old) == content {
		return false, outPath, nil
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return false, outPath, err
	}
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return false, outPath, err
	}
	return true, outPath, nil
}

// RenderTemplate 渲染 STRM 内容路径。可用变量：
//   {mount}     挂载根（MountRoot）
//   {cloudpath} 媒体在云盘的完整路径（/电影/X/a.mkv）
//   {relpath}   扫描根下相对路径（含前导 /）
//   {filename}  文件名（a.mkv）
func RenderTemplate(tmpl, mount, cloudpath, relpath, filename string) string {
	out := tmpl
	out = strings.ReplaceAll(out, "{mount}", mount)
	out = strings.ReplaceAll(out, "{cloudpath}", cloudpath)
	out = strings.ReplaceAll(out, "{relpath}", relpath)
	out = strings.ReplaceAll(out, "{filename}", filename)
	return out
}
