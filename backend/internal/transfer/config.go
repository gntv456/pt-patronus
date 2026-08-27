// Package transfer 实现媒体整理流水线引擎：下载完成 → 识别 → 重命名 → 硬链/移动 → 刮削 NFO/海报。
// 纯逻辑编排层，所有外部依赖（识别/存储/通知/配置/取图）经接口与函数注入，便于单测且不产生包循环。
// 对标 MoviePilot TransferChain，适配 PTPatronus 既有积木（MediaChain 识别 / media.NFO / 媒体服务器刷新）。
package transfer

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// Mode 整理（链接）模式。
type Mode string

const (
	ModeLink     Mode = "link"     // 硬链接（默认，保种——源与目标同盘 inode 共享）
	ModeSoftLink Mode = "softlink" // 软链接（符号链接）
	ModeMove     Mode = "move"     // 移动（删源，断种！UI 需二次确认）
	ModeCopy     Mode = "copy"     // 复制（双份占盘）
)

// ScrapePolicy 刮削 NFO/图片的覆盖策略。
type ScrapePolicy string

const (
	ScrapeMissingOnly ScrapePolicy = "missing_only" // 仅在缺失时写（默认）
	ScrapeOverwrite   ScrapePolicy = "overwrite"    // 覆盖已有
	ScrapeSkip        ScrapePolicy = "skip"         // 不刮削
)

// CloudUpload 整理后自动上传网盘配置：整理成功的媒体文件按 DestDir 相对路径
// 上传到网盘 CloudBase 下，随后（可选）生成指向直链端点的 .strm。
type CloudUpload struct {
	Enabled       bool   `json:"enabled"`
	DriveID       uint   `json:"drive_id"`        // 目标网盘（cloud drives 表 id）
	CloudBase     string `json:"cloud_base"`      // 网盘目标根目录（如 /media）
	UploadVideos  bool   `json:"upload_videos"`   // 上传视频本体（默认 true）
	StrmOutputDir string `json:"strm_output_dir"` // 非空=在本地该目录生成 .strm（内容指向直链端点）
	StrmTemplate  string `json:"strm_template"`   // .strm 内容模板，默认 {direct}{cloudpath}
	DeleteLocal   bool   `json:"delete_local"`    // 上传成功后删除本地整理文件（危险，默认 false）

	// 运行时填充（不入 JSON）：整理配置的库根目录，供云盘路径映射保留「标题/季」结构
	// （cloudPathFor 需要从文件绝对路径剥掉库前缀；为空则不参与匹配）。
	MovieDir string `json:"-"`
	TVDir    string `json:"-"`
}

// directPlaceholder 直链端点占位模板默认值。
const DefaultStrmDirectTemplate = "{direct}{cloudpath}"

// Config 媒体整理配置（存 SystemConfig key "transfer.config"，JSON）。
type Config struct {
	Enabled            bool          `json:"enabled"`
	Mode               Mode          `json:"mode"`
	MovieDir           string        `json:"movie_dir"`
	TVDir              string        `json:"tv_dir"`
	Categories         []string      `json:"categories"`           // 整理白名单分类（默认 subscribe/workflow）
	SkipTags           []string      `json:"skip_tags"`            // 跳过标签前缀（默认 brushflow:/keepseed/reseed）
	OrganizeTag        string        `json:"organize_tag"`         // 命中即强制整理（默认 organize），绕过分类白名单
	PathMappings       []PathMapping `json:"path_mappings"`        // 远程下载器路径到本机可访问路径的前缀映射
	FailedRetryMinutes int           `json:"failed_retry_minutes"` // 自动整理失败后的冷却时间；手动整理不受影响
	TypeFolder         bool          `json:"type_folder"`          // 库下加 movie/tv 子目录
	CategoryFolder     bool          `json:"category_folder"`      // 加分类子目录
	MovieRename        string        `json:"movie_rename"`         // 命名模板（Go text/template）
	TVRename           string        `json:"tv_rename"`            // 剧集集命名模板
	Scrape             ScrapePolicy  `json:"scrape"`
	RefreshLibrary     bool          `json:"refresh_library"` // 整理后触发媒体服务器扫库
	OverwriteFiles     bool          `json:"overwrite_files"` // 目标已存在时是否覆盖文件本体（默认 false）
	BestVersion        bool          `json:"best_version"`    // 洗版：目标库已有旧版本时，新版画质更高则删旧链新
	CloudUpload        CloudUpload   `json:"cloud_upload"`    // 整理后自动上传网盘 + STRM（默认关）
}

// PathMapping 将下载器返回的远程路径前缀映射到后端所在主机可访问的目录。
// DownloaderID 为 0 时匹配所有下载器；非 0 时只匹配指定下载器。
type PathMapping struct {
	DownloaderID uint   `json:"downloader_id"`
	RemotePath   string `json:"remote_path"`
	LocalPath    string `json:"local_path"`
}

const configKey = "transfer.config"

// DefaultConfig 返回默认配置（硬链接 + 影视分类 + 不覆盖）。
func DefaultConfig() Config {
	return Config{
		Enabled:            true,
		Mode:               ModeLink,
		MovieDir:           "/media/movies",
		TVDir:              "/media/tv",
		Categories:         []string{"subscribe", "workflow"},
		SkipTags:           []string{"brushflow:", "keepseed", "reseed"},
		OrganizeTag:        "organize",
		PathMappings:       []PathMapping{},
		FailedRetryMinutes: 360,
		TypeFolder:         true,
		CategoryFolder:     false,
		MovieRename:        `{{.Title}}{{if .Year}} ({{.Year}}){{end}}{{if .Resolution}} - {{.Resolution}}{{end}}{{if .ReleaseGroup}} - {{.ReleaseGroup}}{{end}}`,
		TVRename:           `{{.Title}}{{if .Year}} ({{.Year}}){{end}} - {{.SeasonEpisode}}{{if .Resolution}} - {{.Resolution}}{{end}}{{if .ReleaseGroup}} - {{.ReleaseGroup}}{{end}}`,
		Scrape:             ScrapeMissingOnly,
		RefreshLibrary:     true,
		OverwriteFiles:     false,
	}
}

// KVStore 抽象 oper.SystemConfigOper（Get/Set），避免 transfer→oper 依赖。
type KVStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Load 从 KVStore 读取配置；缺失或解析失败回退默认值（绝不因脏配置阻断整理）。
func Load(kv KVStore) Config {
	def := DefaultConfig()
	if kv == nil {
		return def
	}
	raw, err := kv.Get(configKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return def
	}
	// 反序列化到默认值副本：未提供的字段保留默认（JSON 缺字段不会覆盖默认）。
	c := def
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return def
	}
	return c.withDefaults()
}

// Save 写配置到 KVStore（JSON）。
func Save(kv KVStore, c Config) error {
	if kv == nil {
		return nil
	}
	b, err := json.Marshal(c.withDefaults())
	if err != nil {
		return err
	}
	return kv.Set(configKey, string(b))
}

// withDefaults 补空数组与空串为默认值，避免 UI 误清空导致整理停摆。
func (c Config) withDefaults() Config {
	out := c
	if len(out.Categories) == 0 {
		out.Categories = DefaultConfig().Categories
	}
	if len(out.SkipTags) == 0 {
		out.SkipTags = DefaultConfig().SkipTags
	}
	if out.OrganizeTag == "" {
		out.OrganizeTag = DefaultConfig().OrganizeTag
	}
	if out.PathMappings == nil {
		out.PathMappings = []PathMapping{}
	}
	if out.FailedRetryMinutes <= 0 {
		out.FailedRetryMinutes = DefaultConfig().FailedRetryMinutes
	}
	if out.Mode == "" {
		out.Mode = ModeLink
	}
	if out.Scrape == "" {
		out.Scrape = ScrapeMissingOnly
	}
	if out.MovieRename == "" {
		out.MovieRename = DefaultConfig().MovieRename
	}
	if out.TVRename == "" {
		out.TVRename = DefaultConfig().TVRename
	}
	if out.CloudUpload.StrmTemplate == "" {
		out.CloudUpload.StrmTemplate = DefaultStrmDirectTemplate
	}
	if out.CloudUpload.StrmOutputDir != "" && out.CloudUpload.CloudBase == "" {
		out.CloudUpload.CloudBase = "/"
	}
	return out
}

// MapPath 对下载器返回的绝对路径做最长前缀匹配。返回 false 表示没有可用映射。
func (c Config) MapPath(downloaderID uint, sourcePath string) (string, bool) {
	source := normalizeRemotePath(sourcePath)
	if source == "" {
		return sourcePath, false
	}
	bestLen := -1
	bestLocal := ""
	bestSuffix := ""
	for _, mapping := range c.PathMappings {
		if mapping.DownloaderID != 0 && mapping.DownloaderID != downloaderID {
			continue
		}
		remote := strings.TrimRight(normalizeRemotePath(mapping.RemotePath), "/")
		local := strings.TrimSpace(mapping.LocalPath)
		if remote == "" || local == "" || !pathPrefixMatch(source, remote) || len(remote) <= bestLen {
			continue
		}
		bestLen = len(remote)
		bestLocal = local
		bestSuffix = strings.TrimLeft(source[len(remote):], "/")
	}
	if bestLen < 0 {
		return sourcePath, false
	}
	if bestSuffix == "" {
		return filepath.Clean(bestLocal), true
	}
	return filepath.Join(filepath.Clean(bestLocal), filepath.FromSlash(bestSuffix)), true
}

func normalizeRemotePath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, `\`, "/"))
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return p
}

func pathPrefixMatch(source, prefix string) bool {
	if source == prefix {
		return true
	}
	return strings.HasPrefix(source, prefix) && len(source) > len(prefix) && source[len(prefix)] == '/'
}

// ShouldProcess 范围过滤：分类在白名单 或 命中 OrganizeTag，且不含任何 SkipTag 前缀。
func (c Config) ShouldProcess(category string, tags []string) bool {
	if !c.Enabled {
		return false
	}
	for _, tag := range tags {
		tl := strings.ToLower(tag)
		for _, skip := range c.SkipTags {
			if strings.HasPrefix(tl, strings.ToLower(skip)) {
				return false
			}
		}
	}
	for _, cat := range c.Categories {
		if strings.EqualFold(cat, category) {
			return true
		}
	}
	if c.OrganizeTag != "" {
		for _, tag := range tags {
			if strings.EqualFold(tag, c.OrganizeTag) {
				return true
			}
		}
	}
	return false
}
