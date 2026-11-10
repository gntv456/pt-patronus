// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Code split from models.go. Package models defines GORM models for the persistence layer.
package models

import "time"

// Site — PT 站点配置。
type Site struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	Name          string `gorm:"size:64;not null" json:"name"`
	Platform      string `gorm:"size:32;index" json:"platform"` // nexusphp | mteam | unit3d | gazelle | tnode | rousi
	Definition    string `gorm:"size:64" json:"definition"`     // 引用 definitions/*.yaml 的 id
	BaseURL       string `gorm:"size:255" json:"base_url"`
	CookieEnc     string `gorm:"type:text" json:"-"` // AES-GCM 加密
	APIKeyEnc     string `gorm:"type:text" json:"-"` // MTeam 等 API key（加密）
	PasskeyEnc    string `gorm:"type:text" json:"-"`
	Username      string `gorm:"size:64" json:"username"` // 站点账号（财神等 API 登录用）
	PasswordEnc   string `gorm:"type:text" json:"-"`      // 站点密码（AES-GCM 加密）
	IsActive      bool   `gorm:"default:true" json:"is_active"`
	LimitInterval int    `gorm:"default:0" json:"limit_interval"` // 抓取限流（秒）
	Priority      int    `gorm:"default:0" json:"priority"`
	// 注册域（eTLD+1 小写）存储列 + 唯一索引：兜底 Create 的应用层 check-then-act
	// 去重在并发（CookieCloud 自动建站 vs 手工添加）下的竞态窗口（四轮 P2 修复）。
	// 解析不出域名的行存 NULL 绕过唯一约束（迁移 v3 对历史空行同样置 NULL）。
	RegisteredDomain *string   `gorm:"size:255;uniqueIndex:uk_sites_registered_domain" json:"-"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// SiteStat — 站点统计快照。
type SiteStat struct {
	ID                        uint      `gorm:"primaryKey" json:"id"`
	SiteID                    uint      `gorm:"index;index:idx_site_stat_site_snap,priority:1;not null" json:"site_id"`
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
	SnapAt                    time.Time `gorm:"index;index:idx_site_stat_site_snap,priority:2" json:"snap_at"`
	CreatedAt                 time.Time `json:"created_at"`
}

// SiteConnectivity — 站点联通状态缓存（每站一行，upsert 覆盖）。
// 与 SiteStat（仅 UserInfo 成功才写）互补：此处也记录 disconnected/disabled，
// 供站点运营矩阵/Dashboard 进页面瞬时显示「上次检测结果」，后台 SSE 逐站刷新覆盖。
// Success/Fail/AvgSeconds 为累计统计（对标 MP SiteStatistic），供展示站点稳定性（成功率/平均耗时）。
type SiteConnectivity struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SiteID     uint      `gorm:"uniqueIndex;not null" json:"site_id"`
	Status     string    `gorm:"size:16" json:"status"` // connected | disconnected | disabled
	Level      string    `gorm:"size:64" json:"level"`
	ErrorMsg   string    `gorm:"size:512" json:"error_msg"`
	Success    int       `gorm:"default:0" json:"success"`     // 累计成功次数（connected 时 +1）
	Fail       int       `gorm:"default:0" json:"fail"`        // 累计失败次数（disconnected 时 +1）
	AvgSeconds float64   `gorm:"default:0" json:"avg_seconds"` // 平均成功响应耗时（秒，仅 seconds>0 的成功计入，snapshot 不计）
	CheckedAt  time.Time `json:"checked_at"`
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
