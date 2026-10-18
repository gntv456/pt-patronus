package plugins

import (
	"context"
	"time"
)

const RuntimeBuiltin = "builtin"
const RuntimeExternalHTTP = "external-http"

type RuntimeEntry struct {
	Command  string            `json:"command,omitempty"`
	Args     []string          `json:"args,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	WorkDir  string            `json:"work_dir,omitempty"`
	BaseURL  string            `json:"base_url,omitempty"`
	Health   string            `json:"health,omitempty"`
	Shutdown string            `json:"shutdown,omitempty"`
}

type ConfigOption struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

type ConfigField struct {
	Key      string         `json:"key"`
	Label    string         `json:"label"`
	Type     string         `json:"type"`
	Required bool           `json:"required,omitempty"`
	Default  any            `json:"default,omitempty"`
	Options  []ConfigOption `json:"options,omitempty"`
	Help     string         `json:"help,omitempty"`
	// Group 把字段归到配置表单的一个分组里（如"订阅"/"网络"/"通知"/"定时"）。
	// 留空归入默认（无标题）分组。仅用于 UI 组织，不影响存储。
	Group string `json:"group,omitempty"`
	// OptionsSource 动态选项源（如 "sites"=用户已配置站点）。配合 type="multiselect" 使用：
	// 前端据此从系统 API 拉取选项填充多选下拉，让用户勾选而不是手敲 ID。
	OptionsSource string `json:"options_source,omitempty"`
}

// ScheduleSpec 声明插件的一条定时调度：到点自动调用某个 action。
// 对标 MoviePilot 的 get_service()——让插件能"自调度"，而不必手搓工作流 timer。
type ScheduleSpec struct {
	ID     string `json:"id"`              // 稳定键，参与 cron job 命名（plugin:<pluginID>:<ID>）
	Label  string `json:"label,omitempty"` // 人类可读名，如"每日拉取订阅"
	Action string `json:"action"`          // 要调用的 action 名（必须在 Manifest.Actions 里）
	Cron   string `json:"cron"`            // 默认 5 段 cron 表达式
	// CronConfigKey 非空时，运行时从 config[此键] 读取 cron 覆盖 Cron；
	// 配置值为空串则该条调度停用（用户在前端清空 cron 字段即停），无需额外开关。
	CronConfigKey string         `json:"cron_config_key,omitempty"`
	Input         map[string]any `json:"input,omitempty"` // 传给 action 的静态入参
}

type EventSpec struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type ActionSpec struct {
	Name        string        `json:"name"`
	Label       string        `json:"label"`
	Description string        `json:"description,omitempty"`
	InputSchema []ConfigField `json:"input_schema,omitempty"`
}

type ContributionSpec struct {
	Commands []CommandContribution `json:"commands,omitempty"`
	Views    []ViewContribution    `json:"views,omitempty"`
	Menus    []MenuContribution    `json:"menus,omitempty"`
}

type CommandContribution struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Action      string `json:"action,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

type ViewContribution struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Location string `json:"location,omitempty"`
	Path     string `json:"path,omitempty"`
	Icon     string `json:"icon,omitempty"`
}

type MenuContribution struct {
	Location string `json:"location"`
	Command  string `json:"command"`
	Group    string `json:"group,omitempty"`
	When     string `json:"when,omitempty"`
}

type PluginDependency struct {
	ID       string `json:"id"`
	Version  string `json:"version,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// ResourceAccessSpec 是插件对外部资源访问的**声明**（声明而非强制——external-http 插件是无限制
// 子进程，没有 OS sandbox 可强制）。声明后会在安装确认页向管理员展示，让"这个插件会碰哪些
// 目录/跑哪些二进制/连哪些主机"可见。诚实优先：不假装能 gate，只 surface。
type ResourceAccessSpec struct {
	FsPaths      []string `json:"fs_paths,omitempty"`      // 会读写的目录/路径前缀
	ExecBinaries []string `json:"exec_binaries,omitempty"` // 会调用的外部二进制（如 ffmpeg）
	NetHosts     []string `json:"net_hosts,omitempty"`     // 会出站连接的主机/域名（如 ptang.top）
}

// MediaHit 是 Host API /media/search 返回给插件的一条媒体元数据（镜像 media.MediaInfo 的子集，
// 故意不复用 media 包以避免 plugins→media 循环依赖；由 app.go 的适配器转换）。
type MediaHit struct {
	Source        string   `json:"source"`
	SourceID      string   `json:"source_id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title,omitempty"`
	Year          int      `json:"year,omitempty"`
	Type          string   `json:"type,omitempty"`
	Overview      string   `json:"overview,omitempty"`
	PosterURL     string   `json:"poster_url,omitempty"`
	BackdropURL   string   `json:"backdrop_url,omitempty"`
	ReleaseDate   string   `json:"release_date,omitempty"`
	Rating        float64  `json:"rating,omitempty"`
	Genres        []string `json:"genres,omitempty"`
}

// MediaSearcher 由宿主注入：把插件的关键词检索转发到 PTPatronus 已有的媒体链（douban/tmdb/寸光集…），
// 让刮削类插件复用单一元数据源，而不必各自内嵌 TMDB client + key。
type MediaSearcher interface {
	Search(ctx context.Context, keyword string, year int) ([]MediaHit, error)
}

type DependencyStatus struct {
	ID               string `json:"id"`
	Version          string `json:"version,omitempty"`
	Optional         bool   `json:"optional,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Installed        bool   `json:"installed"`
	Enabled          bool   `json:"enabled"`
	InstalledVersion string `json:"installed_version,omitempty"`
	Satisfied        bool   `json:"satisfied"`
	Message          string `json:"message,omitempty"`
}

type InstallPlanStep struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	Action           string   `json:"action"`
	Installed        bool     `json:"installed"`
	Enabled          bool     `json:"enabled"`
	InstalledVersion string   `json:"installed_version,omitempty"`
	Permissions      []string `json:"permissions,omitempty"`
}

type Manifest struct {
	ID                       string             `json:"id"`
	Name                     string             `json:"name"`
	Version                  string             `json:"version"`
	APIVersion               string             `json:"api_version,omitempty"`
	AppVersion               string             `json:"app_version,omitempty"`
	HostCapabilities         []string           `json:"host_capabilities,omitempty"`
	OptionalHostCapabilities []string           `json:"optional_host_capabilities,omitempty"`
	Dependencies             []PluginDependency `json:"dependencies,omitempty"`
	Author                   string             `json:"author"`
	Description              string             `json:"description"`
	Homepage                 string             `json:"homepage,omitempty"`
	Repository               string             `json:"repository,omitempty"`
	License                  string             `json:"license,omitempty"`
	Keywords                 []string           `json:"keywords,omitempty"`
	Runtime                  string             `json:"runtime"`
	Builtin                  bool               `json:"built_in"`
	Entry                    *RuntimeEntry      `json:"entry,omitempty"`
	Permissions              []string           `json:"permissions"`
	Events                   []EventSpec        `json:"events"`
	Actions                  []ActionSpec       `json:"actions"`
	ConfigSchema             []ConfigField      `json:"config_schema"`
	Schedule                 []ScheduleSpec     `json:"schedule,omitempty"`
	Contributes              *ContributionSpec  `json:"contributes,omitempty"`
	// ResourceAccess 声明插件会访问的目录/二进制/主机（见 ResourceAccessSpec）。
	ResourceAccess *ResourceAccessSpec `json:"resource_access,omitempty"`
	// Requirements 是 Python pip 需求行（如 "clouddrive==1.0"、"Pillow"）。
	// 安装时 host 用 pip install --target {插件目录}/.deps 装好，并把 .deps 注入 PYTHONPATH。
	Requirements []string `json:"requirements,omitempty"`
}

type Event struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}

type NoticeInput struct {
	Title string
	Body  string
	Level string
}

// SiteRef 是给插件用的站点只读视图（不含密钥）。插件通过 Runtime.SiteList 枚举已配置站点，
// 再用 Runtime.SiteCookie(id) 取解密后的 cookie——复用站点系统的 AES-GCM 解密管线。
type SiteRef struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	Username string `json:"username,omitempty"` // 站点账号(喊话 feedback 匹配 @username 用)
}

type Runtime interface {
	Config() map[string]any
	Log(level, eventName, message string, data map[string]any)
	Notice(ctx context.Context, in NoticeInput) error
	Publish(ctx context.Context, eventType string, data map[string]any)
	// KV 持久化（失败队列、调度锚点等运行时状态，存 plugin_kv 表）。
	KVGet(key string, out any) error
	KVSet(key string, val any) error
	KVDelete(key string) error
	// 站点访问（只读视图 + 解密 cookie）。
	SiteList() []SiteRef
	SiteCookie(id uint) (string, error)
	// SetRuntimeConfig 合并写回 ConfigJSON 的单个键并触发调度重排
	// （动态 cron 用，如织梦按邮件时间+24h 算下次 cron 写回 zm_cron）。
	SetRuntimeConfig(key string, val any) error
}

// HostCaller 可选的宿主工具直调能力（Runtime 附加接口，经类型断言使用：
// pluginRuntime 实现；外部/测试桩可不实现）。
type HostCaller interface {
	CallHostTool(ctx context.Context, name string, input map[string]any) (any, error)
}

type Plugin interface {
	Manifest() Manifest
	DefaultConfig() map[string]any
	HandleEvent(ctx context.Context, rt Runtime, e Event) error
	ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error)
}

type RuntimeStatus struct {
	State         string     `json:"state"`
	Managed       bool       `json:"managed"`
	PID           int        `json:"pid,omitempty"`
	BaseURL       string     `json:"base_url,omitempty"`
	HostBaseURL   string     `json:"host_base_url,omitempty"`
	RestartCount  int        `json:"restart_count,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	StoppedAt     *time.Time `json:"stopped_at,omitempty"`
	LastHealthAt  *time.Time `json:"last_health_at,omitempty"`
	LastRestartAt *time.Time `json:"last_restart_at,omitempty"`
	NextRestartAt *time.Time `json:"next_restart_at,omitempty"`
	LastExit      string     `json:"last_exit,omitempty"`
}

type View struct {
	Manifest
	AvailableOptionalHostCapabilities []string           `json:"available_optional_host_capabilities,omitempty"`
	MissingOptionalHostCapabilities   []string           `json:"missing_optional_host_capabilities,omitempty"`
	DependenciesStatus                []DependencyStatus `json:"dependencies_status,omitempty"`
	CanEnable                         bool               `json:"can_enable"`
	CanEnableReason                   string             `json:"can_enable_reason,omitempty"`
	Enabled                           bool               `json:"enabled"`
	Installed                         bool               `json:"installed"`
	Config                            map[string]any     `json:"config"`
	RuntimeStatus                     *RuntimeStatus     `json:"runtime_status,omitempty"`
	LastError                         string             `json:"last_error,omitempty"`
	LastEventAt                       *time.Time         `json:"last_event_at,omitempty"`
	CreatedAt                         time.Time          `json:"created_at"`
	UpdatedAt                         time.Time          `json:"updated_at"`
}

type LogView struct {
	ID        uint           `json:"id"`
	PluginID  string         `json:"plugin_id"`
	Level     string         `json:"level"`
	Event     string         `json:"event"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type ReleaseView struct {
	ID          uint      `json:"id"`
	PluginID    string    `json:"plugin_id"`
	Version     string    `json:"version"`
	Action      string    `json:"action"`
	Source      string    `json:"source,omitempty"`
	Archive     string    `json:"archive,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
	SnapshotDir string    `json:"snapshot_dir"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}

type MarketIndex struct {
	Version          int            `json:"version"`
	Name             string         `json:"name,omitempty"`
	PublicKey        string         `json:"public_key,omitempty"`
	APIVersion       string         `json:"api_version,omitempty"`
	AppVersion       string         `json:"app_version,omitempty"`
	HostCapabilities []string       `json:"host_capabilities,omitempty"`
	Plugins          []MarketPlugin `json:"plugins"`
	// 聚合多源的账面：取到几个源、哪些源没取到及原因。缺这批字段，「配了 3 个源挂了 1 个」
	// 与「仓库里就这些插件」在界面上同形，用户不会知道自己看到的是残缺清单。
	SourcesOK     int                 `json:"sources_ok,omitempty"`
	SourcesFailed []MarketSourceError `json:"sources_failed,omitempty"`
}

// MarketSourceError 聚合市场时单个源的失败记录（原因已走 errtext 中文口径）。
type MarketSourceError struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type MarketPublisher struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Website  string `json:"website,omitempty"`
	Verified bool   `json:"verified,omitempty"`
}

type MarketPlugin struct {
	ID                       string             `json:"id"`
	Name                     string             `json:"name"`
	Version                  string             `json:"version"`
	Source                   string             `json:"source,omitempty"`
	APIVersion               string             `json:"api_version,omitempty"`
	AppVersion               string             `json:"app_version,omitempty"`
	HostCapabilities         []string           `json:"host_capabilities,omitempty"`
	OptionalHostCapabilities []string           `json:"optional_host_capabilities,omitempty"`
	Dependencies             []PluginDependency `json:"dependencies,omitempty"`
	Author                   string             `json:"author,omitempty"`
	Description              string             `json:"description,omitempty"`
	Homepage                 string             `json:"homepage,omitempty"`
	Repository               string             `json:"repository,omitempty"`
	License                  string             `json:"license,omitempty"`
	Keywords                 []string           `json:"keywords,omitempty"`
	// Category 是市场展示用的分类标签（如「媒体库」「短剧」「网盘」「工具」），仅供前端分组/筛选，
	// 不进运行时 Manifest、不参与签名校验（签名只盖 archive 字节）。
	Category                          string             `json:"category,omitempty"`
	Publisher                         *MarketPublisher   `json:"publisher,omitempty"`
	PublishedAt                       string             `json:"published_at,omitempty"`
	Changelog                         []string           `json:"changelog,omitempty"`
	Runtime                           string             `json:"runtime"`
	Archive                           string             `json:"archive"`
	SHA256                            string             `json:"sha256"`
	Signature                         string             `json:"signature,omitempty"`
	TrustLevel                        string             `json:"trust_level,omitempty"`
	Permissions                       []string           `json:"permissions,omitempty"`
	Events                            []EventSpec        `json:"events,omitempty"`
	Actions                           []ActionSpec       `json:"actions,omitempty"`
	ConfigSchema                      []ConfigField      `json:"config_schema,omitempty"`
	Contributes                       *ContributionSpec  `json:"contributes,omitempty"`
	AvailableOptionalHostCapabilities []string           `json:"available_optional_host_capabilities,omitempty"`
	MissingOptionalHostCapabilities   []string           `json:"missing_optional_host_capabilities,omitempty"`
	DependenciesStatus                []DependencyStatus `json:"dependencies_status,omitempty"`
	DependenciesSatisfied             bool               `json:"dependencies_satisfied"`
	InstallPlan                       []InstallPlanStep  `json:"install_plan,omitempty"`
	Installable                       bool               `json:"installable"`
	InstallReason                     string             `json:"install_reason,omitempty"`
	Compatible                        bool               `json:"compatible"`
	CompatibilityReason               string             `json:"compatibility_reason,omitempty"`
	Installed                         bool               `json:"installed"`
	InstalledVersion                  string             `json:"installed_version,omitempty"`
	UpdateAvailable                   bool               `json:"update_available"`
	// ResourceAccess / Requirements 透传到市场浏览/安装确认页，向管理员声明资源访问与依赖。
	ResourceAccess *ResourceAccessSpec `json:"resource_access,omitempty"`
	Requirements   []string            `json:"requirements,omitempty"`
	// MarketPlugin 运营层附加字段（百鸟朝凤/百宝袋；finalizeIndex 填充，索引原文不含）。
	InstallCount int64 `json:"install_count"` // 本站累计安装次数（老树洞）
	Favorited    bool  `json:"favorited"`     // 当前用户已收藏（百宝袋）
}

type InstallResult struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	InstalledDir  string            `json:"installed_dir"`
	RestartNeeded bool              `json:"restart_needed"`
	Items         []InstallPlanStep `json:"items,omitempty"`
}
