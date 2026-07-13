package oper

import (
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TransferHistoryOper 封装媒体整理历史的读写（对标 MP TransferHistoryOper）。
type TransferHistoryOper struct {
	db *gorm.DB
}

// NewTransferHistoryOper 构造。
func NewTransferHistoryOper(db *gorm.DB) *TransferHistoryOper {
	return &TransferHistoryOper{db: db}
}

// List 按创建时间倒序；可选 status 过滤（""=全部）。始终返回非 nil 切片（防 JSON null）。
func (o *TransferHistoryOper) List(status string, limit int) ([]models.TransferHistory, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := o.db.Order("created_at DESC").Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	hs := make([]models.TransferHistory, 0)
	return hs, q.Find(&hs).Error
}

// Get 单条。
func (o *TransferHistoryOper) Get(id uint) (*models.TransferHistory, error) {
	var h models.TransferHistory
	if err := o.db.First(&h, id).Error; err != nil {
		return nil, err
	}
	return &h, nil
}

// AddSuccess 记录一次成功整理。
func (o *TransferHistoryOper) AddSuccess(h *models.TransferHistory) error {
	h.Status = "success"
	return o.db.Create(h).Error
}

// AddFail 记录一次失败（保留 src/title 便于排查 + redo）。
func (o *TransferHistoryOper) AddFail(h *models.TransferHistory, errMsg string) error {
	h.Status = "failed"
	h.ErrorMsg = errMsg
	return o.db.Create(h).Error
}

// Delete 删单条。
func (o *TransferHistoryOper) Delete(id uint) error {
	return o.db.Delete(&models.TransferHistory{}, id).Error
}

// SuccessExistsByHash 该 hash 是否已有成功记录（Sweep 去重：避免重复整理同一种子）。
func (o *TransferHistoryOper) SuccessExistsByHash(hash string) (bool, error) {
	if hash == "" {
		return false, nil
	}
	var count int64
	err := o.db.Model(&models.TransferHistory{}).
		Where("download_hash = ? AND status = ?", hash, "success").Count(&count).Error
	return count > 0, err
}

// SuccessExistsBySrcPath 该源路径是否已有成功记录（目录监视去重：键与 engine Result.Src 对齐，
// 故 watcher 与 DoneHook 处理同一物理文件时互补，不重复硬链）。
func (o *TransferHistoryOper) SuccessExistsBySrcPath(src string) (bool, error) {
	if src == "" {
		return false, nil
	}
	var count int64
	err := o.db.Model(&models.TransferHistory{}).
		Where("src = ? AND status = ?", src, "success").Count(&count).Error
	return count > 0, err
}

// HasSuccessByMedia 该媒体（source+source_id）是否已有成功整理记录（Servarr hasFile 判定）。
func (o *TransferHistoryOper) HasSuccessByMedia(source, sourceID string) bool {
	if source == "" || sourceID == "" {
		return false
	}
	var count int64
	_ = o.db.Model(&models.TransferHistory{}).
		Where("source = ? AND source_id = ? AND status = ?", source, sourceID, "success").Count(&count).Error
	return count > 0
}

// DownloadMediaLinkOper 封装「下载推送 → 媒体身份」映射的读写。
type DownloadMediaLinkOper struct {
	db *gorm.DB
}

// NewDownloadMediaLinkOper 构造。
func NewDownloadMediaLinkOper(db *gorm.DB) *DownloadMediaLinkOper {
	return &DownloadMediaLinkOper{db: db}
}

// Upsert 按 download_hash 落库（subscribe/rss/workflow 推送种子时调用）。
// 同一种子多次推送以最新身份为准。OnConflict 更新全部非主键列。
func (o *DownloadMediaLinkOper) Upsert(l *models.DownloadMediaLink) error {
	if l.DownloadHash == "" {
		return nil
	}
	return o.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "download_hash"}},
		DoUpdates: clause.AssignmentColumns([]string{"source", "source_id", "title", "year", "type"}),
	}).Create(l).Error
}

// GetByHash 按 hash 取媒体身份（整理时秒级识别 Path A）。不存在返回 (nil, nil)。
func (o *DownloadMediaLinkOper) GetByHash(hash string) (*models.DownloadMediaLink, error) {
	if hash == "" {
		return nil, nil
	}
	var l models.DownloadMediaLink
	err := o.db.Where("download_hash = ?", hash).First(&l).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &l, nil
}
