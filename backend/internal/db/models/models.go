// Package models defines GORM models for the persistence layer.
package models

import "time"

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
	Type        string    `gorm:"size:16;not null" json:"type"` // webdav | alist | smb | agent
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
	ID            uint       `gorm:"primaryKey" json:"id"`
	Source        string     `gorm:"size:16" json:"source"`          // tmdb | douban | bangumi
	SourceID      string     `gorm:"size:64;index" json:"source_id"` // movie:123
	Title         string     `gorm:"size:255;not null" json:"title"`
	Year          int        `json:"year"`
	Type          string     `gorm:"size:16" json:"type"`     // movie | tv
	Category      string     `gorm:"size:16" json:"category"` // 传给 SearchSites 的 category（movie/tv）
	PosterURL     string     `gorm:"size:512" json:"poster_url"`
	Resolution    string     `gorm:"size:32" json:"resolution"`  // 可选：1080p/2160p，空=不过滤
	DownloaderID  uint       `gorm:"index" json:"downloader_id"` // 0=默认下载器
	SavePath      string     `gorm:"size:255" json:"save_path"`
	Status        string     `gorm:"size:16;default:subscribed;index" json:"status"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
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
	Tags          string     `gorm:"type:text" json:"tags"` // 逗号分隔
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
