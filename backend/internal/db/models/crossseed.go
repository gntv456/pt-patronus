// Code split from models.go. Package models defines GORM models for the persistence layer.
package models

import "time"

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
	DownloaderID  uint       `gorm:"index;uniqueIndex:uk_cross_seed_dl,priority:1" json:"downloader_id"` // 源种子所在下载器（辅种要复用本机的保存路径，各台各一条）
	SiteID        uint       `gorm:"index" json:"site_id"`                                               // 候选命中的本地站（MTeam genDlToken 现取 baseURL+api_key 用）
	InfoHash      string     `gorm:"index;size:40;uniqueIndex:uk_cross_seed_dl,priority:2" json:"info_hash"`
	SourceSite    string     `gorm:"size:64" json:"source_site"`
	SourceName    string     `gorm:"size:255" json:"source_name"`
	CandidateSite string     `gorm:"size:64;uniqueIndex:uk_cross_seed_dl,priority:3" json:"candidate_site"`
	CandidateURL  string     `gorm:"size:512" json:"candidate_url"`
	SavePath      string     `gorm:"size:255" json:"save_path"`             // 辅种目标保存路径（默认复用源种子路径）
	Status        string     `gorm:"size:16;default:pending" json:"status"` // pending | added | skipped | failed
	MatchScore    int        `json:"match_score"`
	CreatedAt     time.Time  `json:"created_at"`
	ResolvedAt    *time.Time `json:"resolved_at"`
}
