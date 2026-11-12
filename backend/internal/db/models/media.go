// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Code split from models.go. Package models defines GORM models for the persistence layer.
package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// Subscription — 媒体订阅。cron 定时跨站搜种子，命中即推送到下载器 + 通知。
// 单 admin 应用，无 user_id。Status: subscribed(订阅中) | completed(电影已下，不再查) | paused(暂停)。
type Subscription struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	Source       string `gorm:"size:16" json:"source"`          // tmdb | douban | bangumi
	SourceID     string `gorm:"size:64;index" json:"source_id"` // movie:123
	Title        string `gorm:"size:255;not null" json:"title"`
	Year         int    `json:"year"`
	Type         string `gorm:"size:16" json:"type"`     // movie | tv
	Category     string `gorm:"size:16" json:"category"` // 传给 SearchSites 的 category（movie/tv）
	PosterURL    string `gorm:"size:512" json:"poster_url"`
	Resolution   string `gorm:"size:32" json:"resolution"`  // 可选：1080p/2160p，空=不过滤
	DownloaderID uint   `gorm:"index" json:"downloader_id"` // 0=默认下载器
	SavePath     string `gorm:"size:255" json:"save_path"`
	// 多评分闸门：开启后命中种子前先查聚合评分，低于全局阈值（媒体质量配置）则跳过推送 + 通知。
	// 与全局 MediaQualityConfig.Enabled 取并集（任一为真即启用该订阅的评分闸门）。
	// *bool 三态：nil=未设置（Check 时回退全局订阅默认 SubscribeGlobalDefaults）；true/false=订阅自定义。
	EnableRatingGuard *bool `json:"enable_rating_guard"`
	// FilterGroups 绑定的过滤规则组名（管道分隔，空=用全局 SubscribeFilterRuleGroups）。
	FilterGroups string `gorm:"size:255" json:"filter_groups"`
	// PromoTypes 接受的促销类型多选（free/2xfree/2x/discount/2xdiscount，见 site.PromoType）；
	// 空=不过滤（任何促销都接受，含非促销）。替代早期的 OnlyFree 单一开关。
	PromoTypes StringSlice `gorm:"type:text" json:"promo_types"`
	// SiteIDs 参与本订阅搜索的站点 id 列表；空=全部活跃站点（类型见文件末 IntSlice）。
	SiteIDs IntSlice `gorm:"type:text" json:"site_ids"`
	Status  string   `gorm:"size:16;default:subscribed;index" json:"status"`
	// 洗版（best_version）：开启后持续寻找更高画质版本替换库中旧版。
	//   电影：候选 PriOrder 须严格 > CurrentPriority 才推送，达 100 视为洗版完成；
	//   剧集：按 SxxExx 跟踪每集当前最优 PriOrder（EpisodePriority JSON），候选至少改善一集才推送；
	//   BestVersionFull：整季包优先模式（仅剧集订阅生效）。开启后 checkWithDefaults 候选排序把整季包
	//     (Season Pack) 置顶，配合洗版门控优先用整季包提升整季进度；单集作为回退（整季包未改善时才考虑）。
	//     电影订阅与 false 时行为不变。
	// *bool 三态：nil=未设置（Check 时回退全局订阅默认 SubscribeGlobalDefaults）；true/false=订阅自定义。
	BestVersion *bool `json:"best_version"`
	// OfficialOnly 仅官种：*bool 三态(nil 回退全局默认)；true=只留 site.IsOfficial 命中者。复刻 BestVersion 模式。
	OfficialOnly    *bool      `json:"official_only"`
	BestVersionFull bool       `gorm:"default:false" json:"best_version_full"`
	CurrentPriority int        `gorm:"default:0" json:"current_priority"`
	EpisodePriority string     `gorm:"type:text" json:"episode_priority"`
	LastCheckedAt   *time.Time `json:"last_checked_at"`
	// LastCheckError 上一轮检查里「读不出来的站」留下的原因（空=整轮干净）。
	// 只有 last_checked_at 时，「3/10 个站 cookie 失效所以什么都没搜到」和「搜了确实没新命中」
	// 在界面上长得一模一样（订阅逐站失败此前是 `if sr.Error != "" { continue }` 静默丢）。
	LastCheckError string    `gorm:"type:text" json:"last_check_error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// IntSlice — uint 切片自定义类型：DB 存 JSON 文本（"[]" / "[1,2]"），JSON/API 暴露为 number 数组。
// 用于 Subscription.SiteIDs 等数字 id 列表字段（项目此前无 JSON int-slice 先例；FilterGroups 那种管道
// string 不适合数字数组）。MarshalJSON 保证空值序列化为 [] 而非 null（对齐 go-nil-slice-json-null 教训，
// 免去前端逐处 ?? [] 防御）。
type IntSlice []uint

// MarshalJSON 保证 nil/空切片对 API 均输出 []，避免前端数组字段出现 null。
func (s IntSlice) MarshalJSON() ([]byte, error) {
	if len(s) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal([]uint(s))
}

// Value 写入 DB：空切片存 "[]"，否则 JSON 文本。
func (s IntSlice) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	return string(b), err
}

// Scan 从 DB 读取：NULL/空文本 → nil；JSON 文本 → 反序列化。
func (s *IntSlice) Scan(v interface{}) error {
	switch x := v.(type) {
	case nil:
		*s = nil
		return nil
	case []byte:
		return json.Unmarshal(x, s)
	case string:
		return json.Unmarshal([]byte(x), s)
	}
	return fmt.Errorf("IntSlice.Scan: unsupported type %T", v)
}

// StringSlice — string 切片自定义类型：DB 存 JSON 文本（"[]"/"[\"free\",\"2x\"]"），JSON/API
// 暴露为 string 数组。用于 Subscription.PromoTypes 等枚举多选字段。空值序列化为 []（同 IntSlice）。
type StringSlice []string

// MarshalJSON 保证 nil/空切片对 API 均输出 []，避免前端数组字段出现 null。
func (s StringSlice) MarshalJSON() ([]byte, error) {
	if len(s) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(s))
}

// Value 写入 DB：空切片存 "[]"，否则 JSON 文本。
func (s StringSlice) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	return string(b), err
}

// Scan 从 DB 读取：NULL/空文本 → nil；JSON 文本 → 反序列化。
func (s *StringSlice) Scan(v interface{}) error {
	switch x := v.(type) {
	case nil:
		*s = nil
		return nil
	case []byte:
		return json.Unmarshal(x, s)
	case string:
		return json.Unmarshal([]byte(x), s)
	}
	return fmt.Errorf("StringSlice.Scan: unsupported type %T", v)
}

// SubscriptionHit — 订阅命中的种子去重记录（同一下载链接不重复推送）。
type SubscriptionHit struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	SubscriptionID uint      `gorm:"index:idx_sub_hit,priority:1;uniqueIndex:uk_sub_hit,priority:1" json:"subscription_id"`
	TorrentKey     string    `gorm:"size:64;index:idx_sub_hit,priority:2;uniqueIndex:uk_sub_hit,priority:2" json:"torrent_key"` // 下载链接的哈希
	Title          string    `gorm:"size:255" json:"title"`
	SiteName       string    `gorm:"size:64" json:"site_name"`
	CreatedAt      time.Time `json:"created_at"`
}

// TransferHistory — 媒体整理历史：下载完成 → 识别 → 重命名 → 硬链/移动 → 刮削 NFO/海报 的结果记录。
// download_hash 索引供 transfer-sweep 去重（同一种子不重复整理）。
type TransferHistory struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Src          string    `gorm:"size:512;index" json:"src"` // 源文件路径
	Dest         string    `gorm:"size:512" json:"dest"`      // 目标路径（媒体库）
	Mode         string    `gorm:"size:16" json:"mode"`       // link | move | copy | softlink
	Type         string    `gorm:"size:16" json:"type"`       // movie | tv
	Title        string    `gorm:"size:255;index" json:"title"`
	Year         int       `json:"year"`
	Source       string    `gorm:"size:16" json:"source"`    // tmdb | douban | bangumi
	SourceID     string    `gorm:"size:64" json:"source_id"` // movie:123 / tv:456
	Seasons      string    `gorm:"size:32" json:"seasons"`   // S01（剧集）
	Episodes     string    `gorm:"size:64" json:"episodes"`  // E01E02（剧集）
	PosterURL    string    `gorm:"size:512" json:"poster_url"`
	DownloaderID uint      `gorm:"index" json:"downloader_id"`
	DownloadHash string    `gorm:"size:64;index" json:"download_hash"`
	Status       string    `gorm:"size:16;index" json:"status"` // success | failed | skipped（skipped=未下完等前置跳过）
	ErrorMsg     string    `gorm:"type:text" json:"error_msg,omitempty"`
	Files        string    `gorm:"type:text" json:"files,omitempty"` // JSON：已整理文件列表
	Manual       bool      `json:"manual"`                           // true=手动整理/重做；自动失败冷却不统计手动失败
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// DownloadMediaLink — 下载推送 → 媒体身份映射：subscribe/rss/workflow 推送种子时落库，
// 整理时按 hash 秒级命中媒体身份（tmdbid/doubanid 索引路径），免再搜索。
// download_hash 唯一：同一种子多次推送以最新身份为准（Upsert）。
type DownloadMediaLink struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	DownloadHash string    `gorm:"size:64;uniqueIndex" json:"download_hash"`
	Source       string    `gorm:"size:16" json:"source"`    // tmdb | douban | bangumi
	SourceID     string    `gorm:"size:64" json:"source_id"` // movie:123
	Title        string    `gorm:"size:255" json:"title"`
	Year         int       `json:"year"`
	Type         string    `gorm:"size:16" json:"type"` // movie | tv
	CreatedAt    time.Time `json:"created_at"`
}

// MediaServer — 媒体服务器（Emby / Jellyfin / Plex）。多实例 CRUD（同 Downloader 范式）：
// 凭据（API Key / X-Plex-Token）加密存 TokenEnc；事件开关 per-server（下载完成→刷新、
// 播放 webhook→通知）。MediaServerChain 按需构建 client 调 REST API（库扫描/刷新/通知）。
type MediaServer struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Name      string `gorm:"size:64;not null" json:"name"`
	Type      string `gorm:"size:16;not null" json:"type"` // emby | jellyfin | plex
	URL       string `gorm:"size:255" json:"url"`
	Username  string `gorm:"size:64" json:"username"` // Emby/JF 管理员（通知目标 / 演职员推送用）；Plex 可空
	TokenEnc  string `gorm:"type:text" json:"-"`      // 加密的 API Key(Emby/JF) 或 X-Plex-Token
	IsDefault bool   `gorm:"default:false" json:"is_default"`
	// 事件开关
	RefreshOnDownload bool `gorm:"default:true" json:"refresh_on_download"` // 种子下载完成→触发该服务器媒体库刷新
	NotifyOnPlay      bool `gorm:"default:false" json:"notify_on_play"`     // 收到播放 webhook→通知中心
	// 直链 302 反代：开启后 /mediaserver/:id/direct/*path 把 Emby 文件路径改写到 DirectLinkBase，
	// 让播放器直连高带宽 CDN（如网盘直链）而非经 Emby 服务器中转，省本机带宽。base 为空则视为关闭。
	DirectLinkEnabled bool      `gorm:"default:false" json:"direct_link_enabled"`
	DirectLinkBase    string    `gorm:"size:255" json:"direct_link_base"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// CloudDrive — 网盘实例（115 / 夸克 / 123云盘）。多实例 CRUD（同 MediaServer 范式）：
// 凭据（cookie(115/夸克) 或 accessToken(123)）加密存 TokenEnc；RootPath 为浏览/STRM 扫描起点。
// CloudChain 按需构建 client 调网盘 API（列目录/配额/探活）；STRM 规则与订阅追更复用此实例。
type CloudDrive struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Type      string    `gorm:"size:16;not null" json:"type"` // 115 | quark | 123
	TokenEnc  string    `gorm:"type:text" json:"-"`           // 加密的 cookie(115/夸克) 或 accessToken(123)
	RootPath  string    `gorm:"size:255" json:"root_path"`    // 云盘根目录（浏览/扫描起点；"/"=网盘根）
	IsDefault bool      `gorm:"default:false" json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StrmRule — STRM 生成规则：从某网盘的 CloudPath 扫描媒体文件，按模板生成本地 .strm。
// 一条规则 = 一个 CloudDrive + 云盘源目录 + 本地输出目录 + 路径模板。GenerateAll 遍历 enabled 规则。
type StrmRule struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `gorm:"size:64;not null" json:"name"`
	DriveID     uint   `gorm:"index;not null" json:"drive_id"`
	CloudPath   string `gorm:"size:255;not null" json:"cloud_path"` // 云盘源目录（扫描起点）
	OutputDir   string `gorm:"size:255;not null" json:"output_dir"` // 本地输出目录（.strm 落地根）
	MountRoot   string `gorm:"size:255" json:"mount_root"`          // 挂载根（模板 {mount} 替换，如 /CloudDrive2/115）
	Template    string `gorm:"size:255" json:"template"`            // 路径模板，默认 {mount}{cloudpath}
	Recursive   bool   `gorm:"default:true" json:"recursive"`       // 递归扫描子目录
	KeepExt     bool   `gorm:"default:true" json:"keep_ext"`        // .strm 文件名保留源扩展名（a.mkv→a.mkv.strm）否则 a.strm
	Incremental bool   `gorm:"default:false" json:"incremental"`    // 增量模式：本地 .strm 已存在即跳过（大库定期生成）
	Enabled     bool   `gorm:"default:true" json:"enabled"`
	// 输出目录绑定目录标签（transfer.dir_tags，local 型）：填 OutputTag 时解析标签路径为
	// 实际输出目录并覆盖 OutputDir——改标签映射全局生效。
	OutputTag string `gorm:"size:64;default:''" json:"output_tag"`
	// 过滤：IncludeExts 命中才生成（空=全部媒体扩展名）；ExcludeKeywords 命中文件名即跳过
	// （排除 sample/预告/广告等）。逗号分隔存一列，避免加两张关联表。
	IncludeExts     string `gorm:"size:255;default:''" json:"include_exts"`     // 如 mkv,mp4；空=不过滤
	ExcludeKeywords string `gorm:"size:512;default:''" json:"exclude_keywords"` // 如 sample,trailer；空=不过滤
	// LocalBaseURL/PublicBaseURL 内外网双地址：模板 {direct} 渲染为 LocalBaseURL 优先、
	// 空 则回退全局 AppBaseURL；PublicBaseURL 非空时模板可用 {directPublic}（外网直链，
	// 供外网播放场景）。均空=保持相对路径（同机播放器场景）。
	LocalBaseURL  string    `gorm:"size:255;default:''" json:"local_base_url"`
	PublicBaseURL string    `gorm:"size:255;default:''" json:"public_base_url"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// StrmRecord — 已生成的 STRM 记录（去重 + 展示 + 订阅追更 linkage）。同 DriveID+CloudPath 唯一。
type StrmRecord struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	DriveID        uint      `gorm:"index;uniqueIndex:uk_strm_rec,priority:1" json:"drive_id"`
	CloudPath      string    `gorm:"size:512;uniqueIndex:uk_strm_rec,priority:2" json:"cloud_path"`
	StrmPath       string    `gorm:"size:512" json:"strm_path"`
	Size           int64     `json:"size"`
	SubscriptionID uint      `gorm:"index" json:"subscription_id"` // 0=规则生成；>0=订阅追更生成
	CreatedAt      time.Time `json:"created_at"`
}

// CloudTask — 云端任务队列（上传/STRM 生成/目录树导入）：进度可视 + 失败重试 + 历史回看。
// 由自动上传链路与手动操作入队；executor 串行消费（per-drive 顺序、跨 drive 并行）。
type CloudTask struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	DriveID   uint      `gorm:"index" json:"drive_id"`
	Kind      string    `gorm:"size:16;index" json:"kind"`   // upload | strm | import_tree
	Status    string    `gorm:"size:16;index" json:"status"` // pending | running | success | failed
	CloudPath string    `gorm:"size:512" json:"cloud_path"`  // 目标云盘路径（upload/strm）
	LocalPath string    `gorm:"size:512" json:"local_path"`  // 源本地文件（upload）
	Size      int64     `json:"size"`                        // 字节数（upload 源大小）
	Progress  int       `json:"progress"`                    // 0-100（阶段粗粒度）
	Error     string    `gorm:"type:text" json:"error,omitempty"`
	Attempts  int       `json:"attempts"`                           // 重试次数（executor 失败自动重入队 ≤3）
	Payload   string    `gorm:"type:text" json:"payload,omitempty"` // kind 附加 JSON（如 import_tree 的规则参数）
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
