// Package downloader 定义统一下载器抽象（qBittorrent / Transmission / uTorrent / Deluge / Aria2 / ruTorrent / Flood）与数据模型。
// 接口粒度参考 qBitController 的 Repository 层（已验证的业务方法集）。
package downloader

import (
	"context"
	"errors"
	"time"
)

// Torrent 是跨下载器统一的任务模型。
type Torrent struct {
	Hash       string    `json:"hash"`      // 下载器操作标识（Aria2 为 GID）
	InfoHash   string    `json:"info_hash,omitempty"` // BT info hash（Aria2 与操作标识不同）
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Downloaded int64     `json:"downloaded"`
	Uploaded   int64     `json:"uploaded"`
	Progress   float64   `json:"progress"` // 0..1
	Ratio      float64   `json:"ratio"`
	State      string    `json:"state"` // downloading/uploading/paused/stopped/completed/error/...
	DLRate     int64     `json:"dl_rate"`
	UPRate     int64     `json:"up_rate"`
	SavePath   string    `json:"save_path"`
	Category   string    `json:"category"`
	Tags       []string  `json:"tags"`
	AddedAt    time.Time `json:"added_at"`

	// —— 详情字段（qB 来自 sync/maindata 零额外请求；TR 来自 torrent-get 扩展 fields，部分留零值）——
	NumSeeds         int       `json:"num_seeds"`          // 当前连接做种数
	NumLeechs        int       `json:"num_leechs"`         // 当前连接下载数
	NumComplete      int       `json:"num_complete"`       // swarm 做种总数（qB）
	NumIncomplete    int       `json:"num_incomplete"`     // swarm 下载总数（qB）
	Availability     float64   `json:"availability"`       // 可用性 0..1（qB）
	RatioLimit       float64   `json:"ratio_limit"`        // 当前生效比率上限：-2=无限 -1=全局默认 >=0=值
	SeedingTimeLimit int       `json:"seeding_time_limit"` // 当前生效做种时长上限（分钟），同上语义
	ForceStart       bool      `json:"force_start"`        // 强制开始（qB）
	AutoTMM          bool      `json:"auto_tmm"`           // 自动 torrent 管理（qB）
	Sequential       bool      `json:"sequential"`         // 顺序下载（qB）
	FirstLastPiece   bool      `json:"first_last_piece"`   // 首尾优先（qB）
	Priority         int       `json:"priority"`           // 队列优先级（qB：0=最高）
	CompletedAt      time.Time `json:"completed_at"`       // 完成时间（零值=未完成）
}

// GlobalStats 是下载器全局速率统计。
type GlobalStats struct {
	DLRate     int64 `json:"dl_rate"`
	UPRate     int64 `json:"up_rate"`
	DLSpeedRaw int64 `json:"-"`
	UPSpeedRaw int64 `json:"-"`
	TotalSize  int64 `json:"total_size"`
}

// Limits 是限速设置（统一单位 bytes/s；0 表示不限）。
type Limits struct {
	UploadLimit   int64 `json:"upload_limit"`
	DownloadLimit int64 `json:"download_limit"`
}

// ShareLimits 是做种（分享）限制。
// RatioLimit / SeedingTimeLimit 统一语义：-2=无限，-1=全局默认，>=0=具体值。
// 注意：qB 的 SeedingTimeLimit 为「总做种时长」；TR 映射为「空闲时长 seedIdle」语义最近，UI 需注明。
type ShareLimits struct {
	RatioLimit       float64 `json:"ratio_limit"`
	SeedingTimeLimit int     `json:"seeding_time_limit"` // 分钟
}

// TorrentFile 是单种子内的文件信息。
// Priority 保留各后端原始取值（薄抽象，由前端按下载器类型分支渲染选项）：
//   - qB: 0=不下载, 1=高, 2=正常, 7=最高
//   - TR: -1=低, 0=正常, 1=高（TR 无 priority 式「不下载」，走 files-wanted）
type TorrentFile struct {
	Index    int     `json:"index"`
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"` // 0..1
	Priority int     `json:"priority"`
}

// Peer 是连接的对等体信息。
type Peer struct {
	IP       string  `json:"ip"`
	Client   string  `json:"client"`
	Progress float64 `json:"progress"` // 0..1
	DLRate   int64   `json:"dl_rate"`  // 该 peer 给我们的速率
	UPRate   int64   `json:"up_rate"`  // 我们给该 peer 的速率
	Flags    string  `json:"flags,omitempty"`
}

// Tracker 是种子的 tracker 信息。
type Tracker struct {
	URL    string `json:"url"`
	Tier   int    `json:"tier,omitempty"`
	Status string `json:"status,omitempty"`
}

// AddOptions 是添加种子的选项。
type AddOptions struct {
	URL string // magnet / 种子下载链接（与 FileContent 互斥）

	// FileContent 为 .torrent 原始字节（与 URL 互斥；优先级 URL > FileContent）。
	FileContent []byte
	FileName    string

	// SiteID 非 0 时，DownloadChain.Add 会先用该站点 cookie/passkey 把 URL 指向的
	// .torrent 下载成字节（FileContent）再推下载器——解决 PT 站 download.php 需登录态、
	// 下载器无站点 cookie 拿不到种子的问题（对标 MoviePilot）。0=沿用旧 URL 直推。
	SiteID uint

	Category      string
	Tags          []string
	SavePath      string
	Paused        bool
	UploadLimit   int64 // bytes/s，0 不限
	DownloadLimit int64 // bytes/s，0 不限
}

// Downloader 是统一下载器接口。
type Downloader interface {
	// Type 返回下载器类型（qb | tr | ut | dl | ar | rt | fl）。
	Type() string
	// TestConnection 探测连接与登录态。
	TestConnection(ctx context.Context) error
	// ListTorrents 列出全部任务（含一次同步）。
	ListTorrents(ctx context.Context) ([]Torrent, error)
	// AddTorrent 添加种子（magnet/链接 或 .torrent 字节，见 AddOptions）。
	AddTorrent(ctx context.Context, opts AddOptions) error
	// Pause 暂停任务（hashes 为空表示全部）。
	Pause(ctx context.Context, hashes []string) error
	// Resume 恢复任务。
	Resume(ctx context.Context, hashes []string) error
	// Delete 删除任务。
	Delete(ctx context.Context, hashes []string, deleteFiles bool) error
	// SetCategory 设置分类。
	SetCategory(ctx context.Context, hashes []string, category string) error
	// GlobalStats 全局速率统计。
	GlobalStats(ctx context.Context) (*GlobalStats, error)

	// —— 分类 / 标签 ——
	// ListCategories 返回当前可用分类名（qB 来自同步状态零请求；TR 无概念返回 ErrNotImplemented）。
	ListCategories(ctx context.Context) ([]string, error)
	// ListTags 返回当前可用标签名。
	ListTags(ctx context.Context) ([]string, error)
	// SetTags 覆盖式设置标签（qB 无原生覆盖接口，内部按 diff 实现）。
	SetTags(ctx context.Context, hashes []string, tags []string) error
	// AddTags 给指定任务追加标签。
	AddTags(ctx context.Context, hashes []string, tags []string) error
	// RemoveTags 从指定任务移除标签。
	RemoveTags(ctx context.Context, hashes []string, tags []string) error
	// CreateCategory 新建分类（可绑定保存路径；TR 返回 ErrNotImplemented）。
	CreateCategory(ctx context.Context, name, savePath string) error
	// EditCategory 编辑分类保存路径（TR 返回 ErrNotImplemented）。
	EditCategory(ctx context.Context, name, savePath string) error
	// DeleteCategories 删除分类（TR 返回 ErrNotImplemented）。
	DeleteCategories(ctx context.Context, names []string) error

	// —— 生命周期 ——
	// Recheck 强制重新校验。
	Recheck(ctx context.Context, hashes []string) error
	// Reannounce 重新汇报。
	Reannounce(ctx context.Context, hashes []string) error
	// SetLocation 移动保存路径。
	SetLocation(ctx context.Context, hashes []string, location string) error
	// SetForceStart 强制开始/取消强制（qB；TR 返回 ErrNotImplemented）。
	SetForceStart(ctx context.Context, hashes []string, enable bool) error

	// —— 做种限制 ——
	// SetShareLimits 设置分享比率/做种时长上限（语义见 ShareLimits）。
	SetShareLimits(ctx context.Context, hashes []string, limits ShareLimits) error

	// —— 队列 ——
	// SetQueuePriority 调整队列优先级，action ∈ top|up|down|bottom（qB 全支持；TR 仅 top/bottom）。
	SetQueuePriority(ctx context.Context, hashes []string, action string) error

	// —— 顺序下载 / 首尾优先（qB；TR 返回 ErrNotImplemented）——
	ToggleSequentialDownload(ctx context.Context, hashes []string) error
	ToggleFirstLastPiece(ctx context.Context, hashes []string) error

	// —— 限速（统一 bytes/s，0=不限）——
	// SetLimits 设置单个/多个任务的上下行限速。
	SetLimits(ctx context.Context, hashes []string, limits Limits) error
	// GetSpeedLimits 读取全局速度上限。
	GetSpeedLimits(ctx context.Context) (*Limits, error)
	// SetSpeedLimits 设置全局速度上限。
	SetSpeedLimits(ctx context.Context, limits Limits) error

	// —— 文件优先级 ——
	// ListFiles 列出某任务内文件（priority 保留后端原始取值，见 TorrentFile）。
	ListFiles(ctx context.Context, hash string) ([]TorrentFile, error)
	// SetFilePriority 设置文件优先级（priority 为后端原始取值）。
	SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error

	// —— Peers ——
	// ListPeers 列出某任务当前连接的对等体。
	ListPeers(ctx context.Context, hash string) ([]Peer, error)

	// —— Tracker ——
	// ListTrackers 列出某任务的 tracker。
	ListTrackers(ctx context.Context, hash string) ([]Tracker, error)
	// AddTrackers 给某任务追加 tracker。
	AddTrackers(ctx context.Context, hash string, urls []string) error
	// RemoveTrackers 从某任务移除 tracker。
	RemoveTrackers(ctx context.Context, hash string, urls []string) error
}

// StatsCacher 是可选能力：能零额外请求返回已缓存的全局速率（qb 满足，tr 不实现）。
// broadcaster 用类型断言调用，避免对 tr 每 2s 多发一次 session-get。
type StatsCacher interface {
	CachedStats() *GlobalStats
}

// 错误分类（参考 qBitController RequestResult）。
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrBanned             = errors.New("banned by downloader")
	ErrCannotConnect      = errors.New("cannot connect to downloader")
	ErrNotImplemented     = errors.New("not implemented")
)
