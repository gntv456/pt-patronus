package cloud

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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
	// SkipExisting 增量模式：本地 .strm 已存在即跳过（不读内容比对）。
	// 存量巨大的库定期生成时省去全量读盘；首次全量后建议开启。
	SkipExisting bool
	// IncludeExts 扩展名白名单（小写、无点，如 ["mkv","mp4"]）。空=全部媒体扩展名。
	IncludeExts []string
	// ExcludeKeywords 文件名排除关键词（小写包含匹配，如 ["sample","trailer"]）。
	ExcludeKeywords []string
	// LocalBaseURL/PublicBaseURL 内外网双地址：{direct} 用 LocalBaseURL（空=回退 FallbackBaseURL
	// 再空=相对路径）；{directPublic} 用 PublicBaseURL（空=同 {direct}）。
	LocalBaseURL    string
	PublicBaseURL   string
	FallbackBaseURL string // LocalBaseURL 为空时的回退（注入的全局 AppBaseURL）
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
	opts.CloudPath = CleanPath(opts.CloudPath)
	if opts.Template == "" {
		opts.Template = defaultStrmTemplate
	}
	opts.IncludeExts = normalizeExtList(opts.IncludeExts)
	opts.ExcludeKeywords = normalizeKeywordList(opts.ExcludeKeywords)
	if err := validateStrmOutputDir(opts.OutputDir); err != nil {
		return GenerateResult{}, err
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
			if !matchStrmFilters(f, opts) {
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

// sensitiveStrmDirs STRM 输出禁止落入的系统/二进制敏感目录。即时生成（/cloud/generate，
// 登录用户可调）的 output_dir 若能任意指定，可在服务器关键目录投递 .strm；此黑名单堵住
// 明显的系统目录写入。理想方案是配置化白名单媒体库根，此处先做基本防线。
var sensitiveStrmDirs = []string{
	"/", "/etc", "/usr", "/bin", "/sbin", "/boot", "/dev", "/proc", "/sys",
	"/root", "/lib", "/lib64", "/var/log", "/var/spool", "/var/lib", "/opt",
	`C:\Windows`, `C:\Program Files`, `C:\Program Files (x86)`, `C:\ProgramData`,
}

// validateStrmOutputDir 校验 STRM 输出目录安全性：非空、绝对路径、Clean 后不含 ".."、
// 不落在系统敏感目录内。os.MkdirAll 之前调用，防 output_dir 被设为 /etc 等任意路径。
func validateStrmOutputDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("output_dir 不能为空")
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("output_dir 必须为绝对路径: %s", dir)
	}
	cleaned := filepath.Clean(dir)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("output_dir 非法（含 ..）: %s", dir)
	}
	sep := string(os.PathSeparator)
	if volume := filepath.VolumeName(cleaned); volume != "" && cleaned == volume+sep {
		return fmt.Errorf("output_dir 不允许为磁盘根目录: %s", cleaned)
	}
	for _, bad := range sensitiveStrmDirs {
		normBad := filepath.Clean(bad)
		candidate, blocked := cleaned, normBad
		if runtime.GOOS == "windows" {
			candidate, blocked = strings.ToLower(candidate), strings.ToLower(blocked)
		}
		if candidate == blocked || strings.HasPrefix(candidate, blocked+sep) {
			return fmt.Errorf("output_dir 落在系统敏感目录，已拒绝: %s", cleaned)
		}
	}
	return nil
}

// directEndpoints 由规则双地址解析 {direct}/{directPublic} 的基址：
// {direct} = LocalBaseURL > FallbackBaseURL(全局 AppBaseURL) > 相对路径；
// {directPublic} = PublicBaseURL > 同 {direct}。
func directEndpoints(opts GenerateOptions, direct WriteStrmOptions) (directBase, publicBase string) {
	fallback := strings.TrimRight(direct.AppBaseURL, "/")
	local := strings.TrimRight(opts.LocalBaseURL, "/")
	if local == "" {
		local = strings.TrimRight(opts.FallbackBaseURL, "/")
	}
	if local == "" {
		local = fallback
	}
	public := strings.TrimRight(opts.PublicBaseURL, "/")
	if public == "" {
		public = local
	}
	return local, public
}

// renderStrmContent 渲染 .strm 内容：先替换路径变量，再按规则双地址替换 {direct}/{directPublic}。
// Generate（规则生成）路径用：mount 型模板不含 direct 时行为与旧版一致。
func renderStrmContent(opts GenerateOptions, f File, rel string) string {
	content := RenderTemplate(opts.Template, opts.MountRoot, f.Path, rel, f.Name)
	if strings.Contains(content, "{direct}") || strings.Contains(content, "{directPublic}") {
		directBase, publicBase := directEndpoints(opts, WriteStrmOptions{})
		drive := strconv.FormatUint(uint64(opts.DriveID), 10)
		content = strings.ReplaceAll(content, "{direct}", directBase+"/api/v1/cloud/direct/"+drive)
		content = strings.ReplaceAll(content, "{directPublic}", publicBase+"/api/v1/cloud/direct/"+drive)
	}
	return content
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

	// 增量模式：文件已存在即跳过（不读内容）——大库定期生成省全量读盘比对。
	if opts.SkipExisting {
		if _, err := os.Stat(outPath); err == nil {
			return false, outPath, nil
		}
	}

	content := renderStrmContent(opts, f, rel) + "\n"

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

// normalizeExtList 规整扩展名白名单：去点、小写、去空白；空串剔除。
func normalizeExtList(exts []string) []string {
	out := make([]string, 0, len(exts))
	for _, e := range exts {
		e = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(e), ".")))
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

// normalizeKeywordList 规整排除关键词：小写、去空白。
func normalizeKeywordList(kws []string) []string {
	out := make([]string, 0, len(kws))
	for _, k := range kws {
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

// matchStrmFilters 规则级过滤：扩展名白名单（空=全过）+ 文件名排除关键词（小写包含）。
// 在 IsMediaFile 之上叠加，匹配失败的文件既不生成 .strm 也不计入 OnFile。
func matchStrmFilters(f File, opts GenerateOptions) bool {
	if len(opts.IncludeExts) > 0 {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(f.Name), "."))
		hit := false
		for _, want := range opts.IncludeExts {
			if ext == want {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if len(opts.ExcludeKeywords) > 0 {
		name := strings.ToLower(f.Name)
		for _, kw := range opts.ExcludeKeywords {
			if strings.Contains(name, kw) {
				return false
			}
		}
	}
	return true
}

// RenderTemplate 渲染 STRM 内容路径。可用变量：
//
//	{mount}     挂载根（MountRoot）
//	{cloudpath} 媒体在云盘的完整路径（/电影/X/a.mkv）
//	{relpath}   扫描根下相对路径（含前导 /）
//	{filename}  文件名（a.mkv）
//	{direct}    直链端点（自动上传链路专用：替换为 /api/v1/cloud/direct/:driveId）
func RenderTemplate(tmpl, mount, cloudpath, relpath, filename string) string {
	out := tmpl
	out = strings.ReplaceAll(out, "{mount}", mount)
	out = strings.ReplaceAll(out, "{cloudpath}", cloudpath)
	out = strings.ReplaceAll(out, "{relpath}", relpath)
	out = strings.ReplaceAll(out, "{filename}", filename)
	return out
}

// WriteStrmOptions 单文件 .strm 落地参数（自动上传链路用；不走网盘扫描）。
type WriteStrmOptions struct {
	OutputDir  string // 本地输出根目录
	CloudPath  string // 文件在云盘的完整路径（/{cloudpath} 变量与相对目录）
	Template   string // 内容模板（额外支持 {direct}）
	FileName   string // 源文件名（a.mkv）
	DriveID    uint   // {direct} → DirectBase/:driveId
	DirectBase string // 直链端点前缀，空=/api/v1/cloud/direct
	AppBaseURL string // 服务对外基址（含协议+主机+端口）；非空时 strm 内容渲染为绝对 URL
	// StructRoot 结构镜像根：非空时 .strm 落在 OutputDir + (CloudPath 去掉 StructRoot
	// 的相对段)，保留云盘目录结构；空则文件直落 OutputDir。
	StructRoot string
}

// WriteStrmFile 写单个 .strm 文件（幂等：内容相同跳过）。与规则生成的 .strm 结构一致，
// Emby 扫描 .strm 即入库；内容指向 302 直链端点时由后端实时解析网盘限时直链。
func WriteStrmFile(opts WriteStrmOptions) error {
	if err := validateStrmOutputDir(opts.OutputDir); err != nil {
		return err
	}
	if opts.Template == "" {
		opts.Template = defaultStrmTemplate
	}
	root := opts.StructRoot
	if root == "" {
		root = cloudDirOf(opts.CloudPath) // 默认父目录 → 文件直落 OutputDir
	}
	g := GenerateOptions{OutputDir: opts.OutputDir, CloudPath: root, Template: opts.Template, KeepExt: true}
	f := File{Name: opts.FileName, Path: opts.CloudPath}
	_, _, err := writeStrmDirect(g, f, opts)
	return err
}

// directBaseURL 直链端点基址。AppBaseURL 配置时渲染绝对 URL（Emby strm 播放必需：
// 播放器在远端解析内容，相对路径无宿主）；否则保留相对路径（同机场景）。
func directBaseURL(direct WriteStrmOptions) string {
	base := strings.TrimRight(direct.DirectBase, "/")
	if base == "" {
		base = "/api/v1/cloud/direct"
	}
	if direct.AppBaseURL != "" && strings.HasPrefix(base, "/") {
		return strings.TrimRight(direct.AppBaseURL, "/") + base
	}
	return base
}

// cloudDirOf 云盘路径父目录（根则 "/"）。
func cloudDirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	return "/"
}

// writeStrmDirect writeStrm 的直链变体：先替换既有变量，再把 {direct} 替换为
// 直链端点 + /:driveId，幂等写入（复用路径穿越防御与目录创建）。
func writeStrmDirect(opts GenerateOptions, f File, direct WriteStrmOptions) (bool, string, error) {
	rel := strings.TrimPrefix(f.Path, opts.CloudPath)
	relDir := filepath.FromSlash(filepath.Dir(strings.TrimPrefix(rel, "/")))
	strmName := filepath.Base(f.Name) + ".strm"
	outPath := filepath.Join(opts.OutputDir, relDir, strmName)
	if !isWithinDir(outPath, opts.OutputDir) {
		return false, outPath, fmt.Errorf("strm 输出路径逃出 OutputDir: %s", outPath)
	}
	content := RenderTemplate(opts.Template, "", f.Path, rel, f.Name)
	if strings.Contains(content, "{direct}") {
		content = strings.ReplaceAll(content, "{direct}", directBaseURL(direct)+"/"+strconv.FormatUint(uint64(direct.DriveID), 10))
	}
	content += "\n"
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
