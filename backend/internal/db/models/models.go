// Package models defines GORM models for the persistence layer.
package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// User — phase-0 single admin；登录改造后以好学外部身份(HxptUID)为准，本地密码登录弃用。
type User struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Username      string     `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash  string     `gorm:"size:128" json:"-"` // 本地密码登录弃用，保留列兼容老数据
	IsAdmin       bool       `gorm:"default:false" json:"is_admin"`
	Level         int        `gorm:"default:0" json:"level"`        // 与站点认证等级配合
	HxptUID       string     `gorm:"size:64;index" json:"hxpt_uid"` // 好学用户 ID（外部身份，登录 upsert 依据；非唯一索引避空值约束坑）
	HxptUsername  string     `gorm:"size:64" json:"hxpt_username"`
	VipLevel      int        `gorm:"default:0" json:"vip_level"` // 好学 APP 专属 VIP 等级快照
	VipVerifiedAt *time.Time `json:"vip_verified_at"`            // 上次 VIP 校验成功时间（nil=未验证）
	TokenVersion  uint       `gorm:"default:0" json:"-"`         // 登出 bump → 旧 JWT 失效
	HxptCookieEnc string     `gorm:"type:text" json:"-"`         // AES-GCM 加密的好学 cookie（VIP 重验用）
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Site — PT 站点配置。
type Site struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"size:64;not null" json:"name"`
	Platform      string    `gorm:"size:32;index" json:"platform"` // nexusphp | mteam | unit3d | gazelle | tnode | rousi
	Definition    string    `gorm:"size:64" json:"definition"`     // 引用 definitions/*.yaml 的 id
	BaseURL       string    `gorm:"size:255" json:"base_url"`
	CookieEnc     string    `gorm:"type:text" json:"-"` // AES-GCM 加密
	APIKeyEnc     string    `gorm:"type:text" json:"-"` // MTeam 等 API key（加密）
	PasskeyEnc    string    `gorm:"type:text" json:"-"`
	Username      string    `gorm:"size:64" json:"username"` // 站点账号（财神等 API 登录用）
	PasswordEnc   string    `gorm:"type:text" json:"-"`      // 站点密码（AES-GCM 加密）
	IsActive      bool      `gorm:"default:true" json:"is_active"`
	LimitInterval int       `gorm:"default:0" json:"limit_interval"` // 抓取限流（秒）
	Priority      int       `gorm:"default:0" json:"priority"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SiteStat — 站点统计快照。
type SiteStat struct {
	ID                        uint      `gorm:"primaryKey" json:"id"`
	SiteID                    uint      `gorm:"index;not null" json:"site_id"`
	Level                     string    `gorm:"size:64" json:"level"`    // 用户等级（展示用）
	UserName                  string    `gorm:"size:64" json:"username"` // 站点用户名（导出海报身份行，best-effort）
	UserID                    string    `gorm:"size:32" json:"user_id"`  // 站点用户 ID
	Email                     string    `gorm:"size:128" json:"email"`   // 账号邮箱（best-effort，部分隐藏原样保留）
	Uploaded                  int64     `gorm:"default:0" json:"uploaded"`
	Downloaded                int64     `gorm:"default:0" json:"downloaded"`
	Ratio                     float64   `gorm:"default:0" json:"ratio"`
	TrueUploaded              int64     `gorm:"default:0" json:"true_uploaded"`
	TrueDownloaded            int64     `gorm:"default:0" json:"true_downloaded"`
	TrueRatio                 float64   `gorm:"default:0" json:"true_ratio"`
	Seeding                   int       `gorm:"default:0" json:"seeding"`
	Leeching                  int       `gorm:"default:0" json:"leeching"`
	SeedingSize               int64     `gorm:"default:0" json:"seeding_size"`   // 做种总体积（字节）
	Uploads                   int       `gorm:"default:0" json:"uploads"`        // 发布数（用户上传的种子数）
	UniqueGroups              int       `gorm:"default:0" json:"unique_groups"`  // 唯一作品组数（Gazelle community.groups 等）
	PerfectFLAC               int       `gorm:"default:0" json:"perfect_flac"`   // Perfect FLAC 数
	Invited                   int       `gorm:"default:0" json:"invited"`        // 已邀请用户数
	Bonus                     float64   `gorm:"default:0" json:"bonus"`          // 魔力/火花/蟹币/元宝（站点别名）
	BonusPerHour              float64   `gorm:"default:0" json:"bonus_per_hour"` // 每小时积分产出
	SeedingBonus              float64   `gorm:"default:0" json:"seeding_bonus"`  // 做种/保种积分（与普通积分分开）
	SeedingBonusPerHour       float64   `gorm:"default:0" json:"seeding_bonus_per_hour"`
	RescueSeedingBonusPerHour float64   `gorm:"default:0" json:"rescue_seeding_bonus_per_hour"`
	ClaimCount                int       `gorm:"default:0" json:"claim_count"`  // 当前认领数
	MaxClaim                  int       `gorm:"default:0" json:"max_claim"`    // 可认领上限
	JoinDate                  string    `gorm:"size:32" json:"join_date"`      // 加入日期（原始文本）
	LastAccessAt              string    `gorm:"size:64" json:"last_access_at"` // 最近动向/访问时间（原始文本）
	SnapAt                    time.Time `json:"snap_at"`
	CreatedAt                 time.Time `json:"created_at"`
}

// Downloader — 下载器连接配置。
type Downloader struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:64;not null" json:"name"`
	Type        string    `gorm:"size:16;not null" json:"type"` // qb | tr
	URL         string    `gorm:"size:255" json:"url"`
	Username    string    `gorm:"size:64" json:"username"`
	PasswordEnc string    `gorm:"type:text" json:"-"`
	IsDefault   bool      `gorm:"default:false" json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ActivationCode — 好学 APP 专属 VIP 激活码。
type ActivationCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Code      string     `gorm:"uniqueIndex;size:128;not null" json:"code"`
	DeviceID  string     `gorm:"size:128;index" json:"device_id"` // 绑定设备（可空）
	VipLevel  int        `gorm:"default:1" json:"vip_level"`
	UsedAt    *time.Time `json:"used_at"`
	ExpiresAt *time.Time `json:"expires_at"`
	Revoked   bool       `gorm:"default:false" json:"revoked"`
	CreatedAt time.Time  `json:"created_at"`
}

// SystemConfig — 运行时可变配置（key/value）。
type SystemConfig struct {
	Key       string    `gorm:"primaryKey;size:128" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PluginState stores runtime state and user config for a registered plugin.
type PluginState struct {
	ID          string     `gorm:"primaryKey;size:96" json:"id"`
	Enabled     bool       `gorm:"default:false;index" json:"enabled"`
	Installed   bool       `gorm:"default:true;index" json:"installed"`
	ConfigJSON  string     `gorm:"type:text;column:config_json" json:"-"`
	LastError   string     `gorm:"type:text" json:"last_error,omitempty"`
	LastEventAt *time.Time `json:"last_event_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// PluginLog records plugin events, actions, and operational messages.
type PluginLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PluginID  string    `gorm:"size:96;index;not null" json:"plugin_id"`
	Level     string    `gorm:"size:16;index" json:"level"`
	Event     string    `gorm:"size:96;index" json:"event"`
	Message   string    `gorm:"type:text" json:"message"`
	DataJSON  string    `gorm:"type:text;column:data_json" json:"-"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// PluginKV stores per-plugin key-value state (失败消息队列、调度锚点等)。
// 撬动多插件的持久化需求——插件 Runtime 通过 KVGet/KVSet/KVDelete 读写自己的运行时状态，
// 不必把运行时数据塞进用户可见的 ConfigJSON。
type PluginKV struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PluginID  string    `gorm:"size:96;uniqueIndex:idx_plugin_kv,priority:1;not null" json:"plugin_id"`
	Key       string    `gorm:"size:128;uniqueIndex:idx_plugin_kv,priority:2;not null" json:"key"`
	ValueJSON string    `gorm:"type:text" json:"-"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PluginRelease stores install, upgrade, and rollback history for external plugins.
type PluginRelease struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	PluginID    string    `gorm:"size:96;index;not null" json:"plugin_id"`
	Version     string    `gorm:"size:64;index;not null" json:"version"`
	Action      string    `gorm:"size:16;index;not null" json:"action"`
	Source      string    `gorm:"size:512" json:"source,omitempty"`
	Archive     string    `gorm:"size:512" json:"archive,omitempty"`
	SHA256      string    `gorm:"size:128" json:"sha256,omitempty"`
	SnapshotDir string    `gorm:"size:512;not null" json:"snapshot_dir"`
	Active      bool      `gorm:"default:false;index" json:"active"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

// PosterCache — 海报/元数据缓存（避免重复打 TMDB/豆瓣/MusicBrainz）。
type PosterCache struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CacheKey  string    `gorm:"uniqueIndex;size:128;not null" json:"cache_key"` // source:tmdbid / title|year|type 哈希
	Source    string    `gorm:"size:32;index" json:"source"`                    // tmdb | douban | musicbrainz
	SourceID  string    `gorm:"size:64" json:"source_id"`
	Title     string    `gorm:"size:255" json:"title"`
	Year      int       `json:"year"`
	Type      string    `gorm:"size:16" json:"type"` // movie | tv | music
	PosterURL string    `gorm:"size:512" json:"poster_url"`
	Overview  string    `gorm:"type:text" json:"overview"`
	RawJSON   string    `gorm:"type:text" json:"-"` // 原始元数据 JSON
	HitCount  int       `gorm:"default:0" json:"hit_count"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
}

// CrossSeedSetting — 辅种配置（per-site / per-downloader）。
type CrossSeedSetting struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	SiteID        uint      `gorm:"index" json:"site_id"`
	DownloaderID  uint      `gorm:"index" json:"downloader_id"`
	Enabled       bool      `gorm:"default:false" json:"enabled"`
	SavePath      string    `gorm:"size:255" json:"save_path"`
	PauseAfterAdd bool      `gorm:"default:true" json:"pause_after_add"`
	AutoReseed    bool      `gorm:"default:false" json:"auto_reseed"`     // 启用定时自动辅种（该下载器）
	AutoReseedAdd bool      `gorm:"default:false" json:"auto_reseed_add"` // 自动添加新候选；否则仅入库 pending 待人工审
	UpdatedAt     time.Time `json:"updated_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// CrossSeedTask — 辅种候选记录。
type CrossSeedTask struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	DownloaderID  uint       `gorm:"index" json:"downloader_id"` // 源种子所在下载器
	SiteID        uint       `gorm:"index" json:"site_id"`       // 候选命中的本地站（MTeam genDlToken 现取 baseURL+api_key 用）
	InfoHash      string     `gorm:"index;size:40;uniqueIndex:uk_cross_seed,priority:1" json:"info_hash"`
	SourceSite    string     `gorm:"size:64" json:"source_site"`
	SourceName    string     `gorm:"size:255" json:"source_name"`
	CandidateSite string     `gorm:"size:64;uniqueIndex:uk_cross_seed,priority:2" json:"candidate_site"`
	CandidateURL  string     `gorm:"size:512" json:"candidate_url"`
	SavePath      string     `gorm:"size:255" json:"save_path"`             // 辅种目标保存路径（默认复用源种子路径）
	Status        string     `gorm:"size:16;default:pending" json:"status"` // pending | added | skipped | failed
	MatchScore    int        `json:"match_score"`
	CreatedAt     time.Time  `json:"created_at"`
	ResolvedAt    *time.Time `json:"resolved_at"`
}

// FileStorage — 网盘/存储后端配置。
type FileStorage struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:64;not null" json:"name"`
	Type        string    `gorm:"size:16;not null" json:"type"` // webdav | alist | smb | agent | local
	URL         string    `gorm:"size:255" json:"url"`
	Username    string    `gorm:"size:64" json:"username"`
	PasswordEnc string    `gorm:"type:text" json:"-"`
	BasePath    string    `gorm:"size:255" json:"base_path"`
	AgentID     uint      `gorm:"index" json:"agent_id"` // type=agent 时关联 Agent 表；其他类型忽略
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// OpsCredential — 运维凭据中心。密码/私钥加密存储；明文只在创建/更新入参中出现，不回传。
type OpsCredential struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"size:64;not null" json:"name"`
	Type          string    `gorm:"size:16;index;not null" json:"type"` // ssh | nas | generic
	UsernameEnc   string    `gorm:"type:text" json:"-"`
	PasswordEnc   string    `gorm:"type:text" json:"-"`
	PrivateKeyEnc string    `gorm:"type:text" json:"-"`
	Username      string    `gorm:"-" json:"username,omitempty"`
	HasPassword   bool      `gorm:"-" json:"has_password"`
	HasPrivateKey bool      `gorm:"-" json:"has_private_key"`
	Tags          string    `gorm:"size:255" json:"tags"`
	Note          string    `gorm:"type:text" json:"note"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// OpsConfig — 运维配置中心。用于保存可审计的 KV / 文本 / JSON 配置。
type OpsConfig struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"size:128;uniqueIndex;not null" json:"key"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Env       string    `gorm:"size:32;index" json:"env"`
	Format    string    `gorm:"size:16;index;not null" json:"format"` // kv | text | json
	Value     string    `gorm:"type:text" json:"value"`
	Tags      string    `gorm:"size:255" json:"tags"`
	Note      string    `gorm:"type:text" json:"note"`
	Version   int       `gorm:"default:1" json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OpsConfigHistory — 运维配置变更历史。每次创建/更新保留一份快照。
type OpsConfigHistory struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ConfigID  uint      `gorm:"index;not null" json:"config_id"`
	Key       string    `gorm:"size:128;index" json:"key"`
	Version   int       `json:"version"`
	Format    string    `gorm:"size:16" json:"format"`
	Value     string    `gorm:"type:text" json:"value"`
	Note      string    `gorm:"type:text" json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// RegistrationWatch — 注册开放检测的监控目标（用户尚无账号、蹲守开放抢注的站点）。
type RegistrationWatch struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Name          string     `gorm:"size:64;not null" json:"name"`
	URL           string     `gorm:"size:255;not null" json:"url"`
	SignupPath    string     `gorm:"size:128" json:"signup_path"`  // 默认 signup.php
	OpenText      string     `gorm:"size:255" json:"open_text"`    // 命中视为开放（可空）
	ClosedText    string     `gorm:"type:text" json:"closed_text"` // 命中视为关闭（可空）
	LastOpen      *bool      `json:"last_open"`                    // 上次检测结果（nil=从未检测）
	LastCheckedAt *time.Time `json:"last_checked_at"`
	IsActive      bool       `gorm:"default:true" json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// MonitorTarget — 设备监控目标（服务器 host / NAS / 摄像头 camera）。
// Protocol 决定采集器分发：host=local(本机 gopsutil)/ssh(远程)；nas=api(厂商)/snmp；camera=onvif/rtsp。
type MonitorTarget struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Name           string     `gorm:"size:64;not null" json:"name"`
	Type           string     `gorm:"size:16;index;not null" json:"type"` // host | nas | camera
	Host           string     `gorm:"size:255" json:"host"`               // 地址/IP；host 本机留空
	Port           int        `json:"port"`
	Vendor         string     `gorm:"size:32" json:"vendor"`              // nas: synology|truenas|snmp；camera: onvif|rtsp
	Protocol       string     `gorm:"size:16" json:"protocol"`            // local|ssh|api|snmp|onvif|rtsp
	CredentialID   uint       `gorm:"index" json:"credential_id"`         // 0=使用本设备内嵌凭据；非 0=引用 OpsCredential
	UsernameEnc    string     `gorm:"type:text" json:"-"`                 // AES-GCM 加密
	PasswordEnc    string     `gorm:"type:text" json:"-"`                 // AES-GCM 加密
	PrivateKeyEnc  string     `gorm:"type:text" json:"-"`                 // AES-GCM 加密的 SSH 私钥 PEM（可选）
	Username       string     `gorm:"-" json:"username,omitempty"`        // 回显用（解密自 UsernameEnc）；写时忽略
	CredentialName string     `gorm:"-" json:"credential_name,omitempty"` // 回显用（引用凭据名）
	HasPrivateKey  bool       `gorm:"-" json:"has_private_key,omitempty"` // 回显用（PrivateKeyEnc 非空）；明文私钥绝不回传
	RTSPURL        string     `gorm:"size:512" json:"rtsp_url"`           // camera 拉流地址（预览用，ONVIF 可回填）
	IntervalSec    int        `gorm:"default:60" json:"interval_sec"`     // 采集间隔（秒）
	TimeoutSec     int        `gorm:"default:15" json:"timeout_sec"`      // 单次采集超时
	Enabled        bool       `gorm:"default:true" json:"enabled"`
	Thresholds     string     `gorm:"type:text" json:"thresholds"` // 阈值规则 JSON（见 monitor.Thresholds）
	DiskPaths      string     `gorm:"type:text" json:"disk_paths"` // host+local：宿主 bind mount 目录（换行/逗号分隔，MoviePilot 式磁盘监控）
	LastOnline     *bool      `json:"last_online"`                 // 上次探活结果（nil=从未探活）
	LastError      string     `gorm:"type:text" json:"last_error"`
	LastCheckedAt  *time.Time `json:"last_checked_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// MetricSnapshot — 单次采集的指标快照（时序）。Metrics 为 monitor.Metrics 序列化 JSON。
type MetricSnapshot struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TargetID  uint      `gorm:"index:idx_snap_target_time,priority:1;not null" json:"target_id"`
	SnapAt    time.Time `gorm:"index:idx_snap_target_time,priority:2" json:"snap_at"`
	Online    bool      `json:"online"`
	Metrics   string    `gorm:"type:text" json:"metrics"`
	CreatedAt time.Time `gorm:"index" json:"created_at"` // 清理任务按此索引删旧
}

// MonitorAlert — 阈值触发或离线产生的告警记录（状态机：ResolvedAt 非空表示已恢复）。
type MonitorAlert struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	TargetID       uint       `gorm:"index;not null" json:"target_id"`
	Level          string     `gorm:"size:16;index" json:"level"` // info | warning | danger
	Metric         string     `gorm:"size:64" json:"metric"`      // cpu_usage | disk_usage | offline ...
	Value          string     `gorm:"size:64" json:"value"`
	Message        string     `gorm:"type:text" json:"message"`
	Read           bool       `gorm:"default:false;index" json:"read"`
	AcknowledgedAt *time.Time `json:"acknowledged_at"`
	AcknowledgedBy string     `gorm:"size:64" json:"acknowledged_by"`
	SilencedUntil  *time.Time `json:"silenced_until"`
	CreatedAt      time.Time  `gorm:"index" json:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
}

// OpsCommandRun records one audited batch/deploy command execution.
type OpsCommandRun struct {
	ID           uint                  `gorm:"primaryKey" json:"id"`
	Command      string                `gorm:"type:text" json:"command"`
	TimeoutSec   int                   `json:"timeout_sec"`
	Source       string                `gorm:"size:32;index" json:"source"` // batch | deploy | manual
	Note         string                `gorm:"type:text" json:"note"`
	Status       string                `gorm:"size:16;index" json:"status"` // running | success | partial | failed
	RiskLevel    string                `gorm:"size:16;index" json:"risk_level"`
	RiskReason   string                `gorm:"size:255" json:"risk_reason"`
	CreatedBy    string                `gorm:"size:64" json:"created_by"`
	TotalCount   int                   `json:"total_count"`
	SuccessCount int                   `json:"success_count"`
	FailureCount int                   `json:"failure_count"`
	StartedAt    time.Time             `json:"started_at"`
	FinishedAt   *time.Time            `json:"finished_at"`
	DurationMS   int64                 `json:"duration_ms"`
	Targets      []OpsCommandRunTarget `gorm:"foreignKey:RunID" json:"targets,omitempty"`
	CreatedAt    time.Time             `gorm:"index" json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

// OpsCommandRunTarget records one target result in an OpsCommandRun.
type OpsCommandRunTarget struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	RunID      uint       `gorm:"index;not null" json:"run_id"`
	TargetID   uint       `gorm:"index" json:"target_id"`
	TargetName string     `gorm:"size:64" json:"target_name"`
	Status     string     `gorm:"size:16;index" json:"status"` // running | success | error
	Stdout     string     `gorm:"type:text" json:"stdout"`
	Stderr     string     `gorm:"type:text" json:"stderr"`
	Error      string     `gorm:"type:text" json:"error"`
	DurationMS int64      `json:"duration_ms"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	CreatedAt  time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Notice — 全局通知中心收件箱。聚合各类业务事件（站庆/邮件/考核/公告/掉线/辅种/下载/备份/站免）。
// Source 标事件类型枚举；Key 用于去重（同 Source+Key 当天已发则跳过，避免周期 cron 刷屏；一次性事件留空）。
type Notice struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Source    string    `gorm:"size:32;index;uniqueIndex:uk_notice_src_key,priority:1" json:"source"`
	Key       string    `gorm:"size:160;index;uniqueIndex:uk_notice_src_key,priority:2" json:"key,omitempty"`
	Title     string    `gorm:"size:255" json:"title"`
	Body      string    `gorm:"type:text" json:"body,omitempty"`
	Level     string    `gorm:"size:16" json:"level"` // info | success | warning | danger
	Read      bool      `gorm:"default:false;index" json:"read"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// CheckinRecord — 站点签到历史。每次签到（cron 自动 + 手动）落一条，便于回看成功/失败/魔力。
// Source: cron（定时自动）/ manual（Web/Flutter 手动触发，含一键签到）。
type CheckinRecord struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SiteID    uint      `gorm:"index;not null" json:"site_id"`
	SiteName  string    `gorm:"size:64" json:"site_name"` // 冗余存名，删站后历史仍可读
	Success   bool      `json:"success"`
	Message   string    `gorm:"size:255" json:"message"`
	Bonus     float64   `gorm:"default:0" json:"bonus"`
	Error     string    `gorm:"type:text" json:"error,omitempty"`
	Source    string    `gorm:"size:16;index" json:"source"` // cron | manual
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// Subscription — 媒体订阅（对标 MoviePilot 订阅）。cron 定时跨站搜种子，命中即推送到下载器 + 通知。
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
	EnableRatingGuard bool `gorm:"default:false" json:"enable_rating_guard"`
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
	//   BestVersionFull：整季包优先模式。
	BestVersion     bool       `gorm:"default:false" json:"best_version"`
	BestVersionFull bool       `gorm:"default:false" json:"best_version_full"`
	CurrentPriority int        `gorm:"default:0" json:"current_priority"`
	EpisodePriority string     `gorm:"type:text" json:"episode_priority"`
	LastCheckedAt   *time.Time `json:"last_checked_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// IntSlice — uint 切片自定义类型：DB 存 JSON 文本（"[]" / "[1,2]"），JSON/API 暴露为 number 数组。
// 用于 Subscription.SiteIDs 等数字 id 列表字段（项目此前无 JSON int-slice 先例；FilterGroups 那种管道
// string 不适合数字数组）。MarshalJSON 保证空值序列化为 [] 而非 null（对齐 go-nil-slice-json-null 教训，
// 免去前端逐处 ?? [] 防御）。
type IntSlice []uint

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
// 对标 MoviePilot TransferHistory。download_hash 索引供 transfer-sweep 去重（同一种子不重复整理）。
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
	Status       string    `gorm:"size:16;index" json:"status"` // success | failed
	ErrorMsg     string    `gorm:"type:text" json:"error_msg,omitempty"`
	Files        string    `gorm:"type:text" json:"files,omitempty"` // JSON：已整理文件列表
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// DownloadMediaLink — 下载推送 → 媒体身份映射：subscribe/rss/workflow 推送种子时落库，
// 整理时按 hash 秒级命中媒体身份（对标 MP DownloadHistory 的 tmdbid/doubanid 路径），免再搜索。
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

// AgentConversation — AI 助手对话会话（多轮上下文记忆）。
type AgentConversation struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Title     string    `gorm:"size:255" json:"title"` // 首条用户消息摘要
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentMessage — 单条对话消息（user/assistant/tool），含思考过程与工具调用记录。
type AgentMessage struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ConversationID uint      `gorm:"index;not null" json:"conversation_id"`
	Role           string    `gorm:"size:16;not null" json:"role"`          // user | assistant | tool
	Content        string    `gorm:"type:text" json:"content"`              // 文本内容
	Reasoning      string    `gorm:"type:text" json:"reasoning,omitempty"`  // 深度思考过程（reasoning_content / thinking）
	ToolCalls      string    `gorm:"type:text" json:"tool_calls,omitempty"` // JSON: [{id,name,arguments}]（assistant 发起的调用）
	ToolCallID     string    `gorm:"size:64" json:"tool_call_id,omitempty"` // role=tool：回应哪个调用
	Name           string    `gorm:"size:64" json:"name,omitempty"`         // role=tool：工具名
	CreatedAt      time.Time `gorm:"index" json:"created_at"`
}

// SSHHostKey — SSH 远程监控的 host key 记录（TOFU：首次连接记录，后续校验，不符即拒）。
// Host key 本身是公钥，明文存储（标准 known_hosts 做法）；Key 为 marshaled wire format，用于精确比对。
type SSHHostKey struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Host        string    `gorm:"size:255;index:idx_ssh_host,priority:1;not null" json:"host"`
	Port        int       `gorm:"index:idx_ssh_host,priority:2;default:22" json:"port"`
	KeyType     string    `gorm:"size:32" json:"key_type"`     // ssh-ed25519 / ssh-rsa / ecdsa-...
	Fingerprint string    `gorm:"size:128" json:"fingerprint"` // sha256:...（便于展示/排查）
	Key         []byte    `json:"-"`                           // marshaled host key（比对用），公钥不敏感
	AcceptedAt  time.Time `json:"accepted_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Agent — push 架构的被监控 agent 身份（per-agent secret 鉴权）。1:1 关联 MonitorTarget
// （type=host, protocol=agent）。SecretHash 为 bcrypt；明文 secret 仅创建/重置时返回一次。
type Agent struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	Name       string     `gorm:"size:64;not null" json:"name"`
	SecretHash string     `gorm:"size:128;not null" json:"-"` // bcrypt(明文 secret)
	SecretHint string     `gorm:"size:16" json:"secret_hint"` // 明文前 8 位（运维辨认，不泄露）
	Enabled    bool       `gorm:"default:true" json:"enabled"`
	TargetID   uint       `gorm:"uniqueIndex;not null" json:"target_id"` // 1:1 关联 MonitorTarget
	Online     bool       `gorm:"default:false" json:"online"`           // WS 是否在线
	Version    string     `gorm:"size:32" json:"version"`                // agent 上报版本号
	Hostname   string     `gorm:"size:128" json:"hostname"`              // agent 上报的机器名（Metrics.Hostname）
	IP         string     `gorm:"size:64" json:"ip"`                     // WS 连接来源 IP
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// AssessmentTask — 站点考核任务快照（per site+task_name 唯一）。3 级缓存（内存→DB→网络）的 DB 层。
type AssessmentTask struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	SiteID      uint      `gorm:"uniqueIndex:uk_assess_site_task,priority:1;not null" json:"site_id"`
	TaskName    string    `gorm:"size:128;uniqueIndex:uk_assess_site_task,priority:2;not null" json:"task_name"`
	Description string    `gorm:"type:text" json:"description"`
	StartTime   int64     `json:"start_time"` // epoch ms（0=未知）
	EndTime     int64     `json:"end_time"`
	Status      string    `gorm:"size:16;default:ACTIVE" json:"status"`   // ACTIVE/COMPLETED/FAILED/EXEMPT/UNKNOWN
	MetricsJSON string    `gorm:"type:text;column:metrics_json" json:"-"` // []TaskMetric 序列化
	RawJSON     string    `gorm:"type:text" json:"-"`
	FetchedAt   time.Time `gorm:"index" json:"fetched_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Medal — 站点勋章快照（per site+medal_id 唯一）。勋章墙 3 级缓存（内存→DB→网络）的 DB 层。
// isOwned 不持久化，由 PurchaseStatus=="OWNED" 派生；group 是 SQL 保留字 → group_name 列。
type Medal struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	SiteID           uint      `gorm:"uniqueIndex:uk_medal_site,priority:1;not null" json:"site_id"`
	MedalID          string    `gorm:"size:64;uniqueIndex:uk_medal_site,priority:2;not null" json:"medal_id"` // 站点内业务主键，字符串兼容非数字 id
	Name             string    `gorm:"size:128;not null" json:"name"`
	Description      string    `gorm:"type:text" json:"description"`
	ImageSmall       string    `json:"image_small"`
	OriginalImageURL string    `json:"original_image_url"`
	SaleBeginTime    *int64    `json:"sale_begin_time"` // epoch ms（nil=未知，0 值过滤为 nil）
	SaleEndTime      *int64    `json:"sale_end_time"`
	Price            float64   `json:"price"`
	OriginalPrice    float64   `json:"original_price"`                                 // 原价（溢价时 < Price，前端划线展示）
	Validity         string    `gorm:"size:32" json:"validity"`                        // "30天"/"永久"
	BonusRate        string    `gorm:"size:32" json:"bonus_rate"`                      // "+20%"
	PurchaseStatus   string    `gorm:"size:16;default:UNKNOWN" json:"purchase_status"` // OWNED/AVAILABLE/OUT_OF_STOCK/NOT_ON_SALE/DISCONTINUED/UNKNOWN
	GiftStatus       string    `gorm:"size:32" json:"gift_status"`
	Stock            int       `gorm:"default:-1" json:"stock"` // -1=无限/未知
	StockStatus      string    `gorm:"size:32" json:"stock_status"`
	Currency         string    `gorm:"size:16;default:魔力" json:"currency"` // 魔力/电力/金币/积分/BP
	GroupName        string    `gorm:"column:group_name" json:"group"`
	WearStatus       bool      `json:"wear_status"`
	FetchedAt        time.Time `gorm:"index" json:"fetched_at"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Workflow — 可视化节点图工作流定义。nodes/edges 为前端节点画布 JSON（见 workflow.GraphNode/GraphEdge）。
// trigger_type 决定触发方式：timer(cron 定时)/event(事件总线)/manual(仅手动)。
// state: idle(空闲)/running(执行中)/success(上次成功)/failed(上次失败)。单 admin 应用，无 user_id。
type Workflow struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Name            string     `gorm:"size:128;not null" json:"name"`
	Description     string     `gorm:"type:text" json:"description"`
	TriggerType     string     `gorm:"size:16;default:manual;index" json:"trigger_type"` // timer|event|manual
	Timer           string     `gorm:"size:64" json:"timer"`                             // cron 表达式（trigger_type=timer）
	EventType       string     `gorm:"size:64" json:"event_type"`                        // 监听事件类型（trigger_type=event）
	EventConditions string     `gorm:"type:text" json:"event_conditions"`                // 事件触发条件 JSON（[]workflow.EventCondition）
	Enabled         bool       `gorm:"default:false;index" json:"enabled"`
	State           string     `gorm:"size:16;default:idle" json:"state"` // idle|running|success|failed
	RunCount        int        `gorm:"default:0" json:"run_count"`
	LastRunAt       *time.Time `json:"last_run_at"`
	LastResult      string     `gorm:"type:text" json:"last_result"`
	Nodes           string     `gorm:"type:text" json:"nodes"` // []workflow.GraphNode 序列化 JSON
	Edges           string     `gorm:"type:text" json:"edges"` // []workflow.GraphEdge 序列化 JSON
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// WorkflowRun — 单次工作流执行记录。node_executions 为 []workflow.LogEntry 序列化 JSON。
type WorkflowRun struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	WorkflowID     uint       `gorm:"index;not null" json:"workflow_id"`
	State          string     `gorm:"size:16;default:running;index" json:"state"` // running|success|failed
	Trigger        string     `gorm:"size:16" json:"trigger"`                     // manual|timer|event
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	DurationMS     int64      `json:"duration_ms"`
	NodeExecutions string     `gorm:"type:text" json:"node_executions"` // []LogEntry JSON
	Result         string     `gorm:"type:text" json:"result"`          // 最终摘要/错误
	CreatedAt      time.Time  `gorm:"index" json:"created_at"`          // 清理任务按此索引删旧
}

// RssSubscription — RSS 订阅源。可选关联 SiteID（PT 站 cookie 鉴权 RSS）。
// cron 全局扫描（RSS_CHECK_CRON）按各订阅 interval_min vs last_checked_at 决定是否到点抓取。
type RssSubscription struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Name          string     `gorm:"size:128;not null" json:"name"`
	URL           string     `gorm:"size:512;not null" json:"url"`
	SiteID        uint       `gorm:"index" json:"site_id"` // 0=匿名 RSS；非 0=用站点 cookie 鉴权抓取
	Enabled       bool       `gorm:"default:true;index" json:"enabled"`
	IntervalMin   int        `gorm:"default:30" json:"interval_min"` // 独立轮询间隔（分钟）
	TitleInclude  string     `gorm:"type:text" json:"title_include"` // 标题包含关键词（| 分隔）
	TitleExclude  string     `gorm:"type:text" json:"title_exclude"`
	MinSize       int64      `json:"min_size"` // 字节，0 不过滤
	MaxSize       int64      `json:"max_size"`
	Resolution    string     `gorm:"size:32" json:"resolution"` // 1080p/2160p，空不过滤
	AutoDownload  bool       `gorm:"default:false" json:"auto_download"`
	DownloaderID  uint       `gorm:"index" json:"downloader_id"` // 0=默认下载器
	SavePath      string     `gorm:"size:255" json:"save_path"`
	Category      string     `gorm:"size:64" json:"category"`
	Tags          string     `gorm:"type:text" json:"tags"`         // 逗号分隔
	FilterGroups  string     `gorm:"size:255" json:"filter_groups"` // 绑定规则组（| 分隔，空=不过滤）
	LastCheckedAt *time.Time `json:"last_checked_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// RssItem — RSS 条目去重记录。item_key=sha1(guid)[:16]，与 SubscriptionID 联合唯一。
type RssItem struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	SubscriptionID uint       `gorm:"index:idx_rss_item,priority:1;uniqueIndex:uk_rss_item,priority:1" json:"subscription_id"`
	ItemKey        string     `gorm:"size:64;index:idx_rss_item,priority:2;uniqueIndex:uk_rss_item,priority:2" json:"item_key"`
	Title          string     `gorm:"size:255" json:"title"`
	Link           string     `gorm:"size:512" json:"link"`
	TorrentURL     string     `gorm:"size:512" json:"torrent_url"`
	Size           int64      `json:"size"`
	PubDate        *time.Time `json:"pub_date"`
	Downloaded     bool       `gorm:"default:false" json:"downloaded"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Webhook — 进站 Webhook 接收端点（外部系统 POST /api/v1/webhooks/:token 触发）。
// Token 为 URL 内的密钥（随机），即鉴权凭据；可选 Secret 做 HMAC 签名校验（X-Signature 头）。
// TargetWorkflowID 非空时直接触发该工作流（payload 注入 Variables）；同时广播 "webhook" 事件供 event 型工作流监听。
type Webhook struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	Name             string     `gorm:"size:128;not null" json:"name"`
	Token            string     `gorm:"size:64;uniqueIndex;not null" json:"token"` // URL 密钥（随机 32 hex）
	TargetWorkflowID uint       `gorm:"index" json:"target_workflow_id"`           // 0=不直接触发工作流
	SecretEnc        string     `gorm:"type:text" json:"-"`                        // AES-GCM 加密的 HMAC 校验密钥（可选）
	HasSecret        bool       `gorm:"-" json:"has_secret,omitempty"`             // 回显用（是否设了签名密钥）
	Enabled          bool       `gorm:"default:true" json:"enabled"`
	TriggerCount     int        `gorm:"default:0" json:"trigger_count"`
	LastTriggeredAt  *time.Time `json:"last_triggered_at"`
	LastPayload      string     `gorm:"type:text" json:"-"` // 最近一次请求体（截断，调试用）
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// BrushFlowTask — 刷流任务。按规则在指定站点选种加入下载器，监控做种达到删除规则
// （分享率/保种时长/上传量目标，满足任一）后自动删种，循环刷上传量与分享率。
// 识别：加入的种子打 tag "brushflow:<task_id>"，扫删/限流都基于该 tag 在下载器侧统计，
// 不依赖跨系统 infohash 匹配（站点种子 ID ≠ 下载器 infohash，无法直接对应）。
type BrushFlowTask struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	Name         string `gorm:"size:128;not null" json:"name"`
	SiteID       uint   `gorm:"index;not null" json:"site_id"`
	DownloaderID uint   `gorm:"index;not null" json:"downloader_id"`
	Enabled      bool   `gorm:"default:true" json:"enabled"`
	// 选种规则
	Keyword      string `gorm:"size:128" json:"keyword"`        // 搜种关键字，空=综合
	Category     string `gorm:"size:32" json:"category"`        // 传给 Search 的分类(movie/tv/music/"")
	MinSizeMB    int64  `json:"min_size_mb"`                    // 种子大小下限(MB)，0=不限
	MaxSizeMB    int64  `json:"max_size_mb"`                    // 种子大小上限(MB)，0=不限
	MinSeeders   int    `json:"min_seeders"`                    // 做种人数下限
	MaxSeeders   int    `json:"max_seeders"`                    // 做种人数上限，0=不限
	FreeOnly     bool   `gorm:"default:false" json:"free_only"` // 仅免费种
	Paused       bool   `gorm:"default:false" json:"paused"`    // 加入时暂停(手动放行)
	FilterGroups string `gorm:"size:255" json:"filter_groups"`  // 绑定规则组（| 分隔，空=用体积/做种/Free 硬过滤）
	// 删除规则（满足任一即删；全 0 = 永不自动删）
	DeleteRatio     float64 `json:"delete_ratio"`                      // 分享率达此值删，0=不限
	DeleteSeedHours float64 `json:"delete_seed_hours"`                 // 做种时长达此值删，0=不限
	DeleteUploadMB  int64   `json:"delete_upload_mb"`                  // 累计上传达此值删，0=不限
	DeleteFiles     bool    `gorm:"default:false" json:"delete_files"` // 删种时是否删文件
	// 并发与分类
	MaxSeeds   int    `gorm:"default:10" json:"max_seeds"` // 该任务最大同时做种数
	SavePath   string `gorm:"size:255" json:"save_path"`
	CategoryDL string `gorm:"size:64" json:"category_dl"` // 下载器内分类
	// 统计（cron 累加）
	TotalAdded   int64      `gorm:"default:0" json:"total_added"`
	TotalDeleted int64      `gorm:"default:0" json:"total_deleted"`
	TotalUpload  int64      `gorm:"default:0" json:"total_upload"` // 累计上传字节（删种时计入）
	LastSelectAt *time.Time `json:"last_select_at"`
	LastSweepAt  *time.Time `json:"last_sweep_at"`
	LastError    string     `gorm:"type:text" json:"last_error"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// BrushFlowSeed — 刷流种子加入历史。用于去重（同任务不重复加同一站点种子）与展示，
// 不维护删除状态（删种判定基于下载器 tag，统计累加在 task 上）。
type BrushFlowSeed struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TaskID       uint      `gorm:"index;not null" json:"task_id"`
	DownloaderID uint      `gorm:"index" json:"downloader_id"`
	SourceKey    string    `gorm:"size:255;index" json:"source_key"` // 站点种子标识(DownloadURL 优先，否则 ID)，去重用
	Title        string    `gorm:"size:512" json:"title"`
	Size         int64     `json:"size"` // bytes
	SiteID       uint      `json:"site_id"`
	AddedAt      time.Time `json:"added_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ContractRule — 保种契约规则。为站点设分享率/做种数/上传量下限，automation cron 定时检查
// SiteChain.UserInfo，低于任一下限则告警通知（防分享率过低被封；与 H&R 告警同属保号自动化）。
// 每站一条（site_id 唯一），Upsert 按 site_id 合并。
type ContractRule struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	SiteID           uint       `gorm:"uniqueIndex;not null" json:"site_id"`
	Enabled          bool       `gorm:"default:true" json:"enabled"`
	MinRatio         float64    `json:"min_ratio"`                          // 分享率下限，0=不检
	MinSeeding       int        `json:"min_seeding"`                        // 做种数下限，0=不检
	MinUploadGB      int        `json:"min_upload_gb"`                      // 上传量下限(GiB)，0=不检
	OfficialOnly     bool       `gorm:"default:false" json:"official_only"` // true=只统计官种做种数/体积
	MinSeedingSizeGB int        `json:"min_seeding_size_gb"`                // 做种总体积下限(GiB)，0=不检
	Period           string     `gorm:"size:16" json:"period"`              // 考核周期 "" | weekly | monthly（告警去重窗口）
	StartTime        *time.Time `json:"start_time"`                         // 生效起始，nil=立即
	LastCheckedAt    *time.Time `json:"last_checked_at"`
	LastStatus       string     `gorm:"size:16" json:"last_status"` // ok | breach | error
	LastDetail       string     `gorm:"type:text" json:"last_detail"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	DriveID   uint      `gorm:"index;not null" json:"drive_id"`
	CloudPath string    `gorm:"size:255;not null" json:"cloud_path"` // 云盘源目录（扫描起点）
	OutputDir string    `gorm:"size:255;not null" json:"output_dir"` // 本地输出目录（.strm 落地根）
	MountRoot string    `gorm:"size:255" json:"mount_root"`          // 挂载根（模板 {mount} 替换，如 /CloudDrive2/115）
	Template  string    `gorm:"size:255" json:"template"`            // 路径模板，默认 {mount}{cloudpath}
	Recursive bool      `gorm:"default:true" json:"recursive"`       // 递归扫描子目录
	KeepExt   bool      `gorm:"default:true" json:"keep_ext"`        // .strm 文件名保留源扩展名（a.mkv→a.mkv.strm）否则 a.strm
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
