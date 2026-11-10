// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Code split from models.go. Package models defines GORM models for the persistence layer.
package models

import "time"

// OpsCredential — 运维凭据中心。密码/私钥加密存储；明文只在创建/更新入参中出现，不回传。
type OpsCredential struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	Name          string `gorm:"size:64;not null" json:"name"`
	Type          string `gorm:"size:16;index;not null" json:"type"` // ssh | nas | generic
	UsernameEnc   string `gorm:"type:text" json:"-"`
	PasswordEnc   string `gorm:"type:text" json:"-"`
	PrivateKeyEnc string `gorm:"type:text" json:"-"`
	Username      string `gorm:"-" json:"username,omitempty"`
	// HasUsername 与 HasPassword/HasPrivateKey 同口径：只看密文是否存在，不要求能解密。
	// 少了它，主密钥换过后列表会显示「无用户名」，同一行却写着「密码 · 私钥」，自相矛盾。
	HasUsername   bool      `gorm:"-" json:"has_username"`
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
// Protocol 决定采集器分发：host=local/ssh/agent；nas=local/agent/api/snmp；camera=onvif/rtsp。
type MonitorTarget struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Name           string     `gorm:"size:64;not null" json:"name"`
	Type           string     `gorm:"size:16;index;not null" json:"type"` // host | nas | camera
	Host           string     `gorm:"size:255" json:"host"`               // 地址/IP；host 本机留空
	Port           int        `json:"port"`
	Vendor         string     `gorm:"size:32" json:"vendor"`              // nas: synology|truenas|snmp；camera: onvif|rtsp
	Protocol       string     `gorm:"size:16" json:"protocol"`            // local|ssh|agent|api|snmp|onvif|rtsp
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
	DiskPaths      string     `gorm:"type:text" json:"disk_paths"` // host/nas+local：宿主 bind mount 目录（换行/逗号分隔，MoviePilot 式磁盘监控）
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
// （type=host/nas, protocol=agent）。SecretHash 为 bcrypt；明文 secret 仅创建/重置时返回一次。
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
