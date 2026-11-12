// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"errors"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TransferHistoryOper 封装媒体整理历史的读写。
type TransferHistoryOper struct {
	db *gorm.DB
}

// NewTransferHistoryOper 构造。
func NewTransferHistoryOper(db *gorm.DB) *TransferHistoryOper {
	return &TransferHistoryOper{db: db}
}

// List 按创建时间倒序；可选 status 过滤（""=全部）。始终返回非 nil 切片（防 JSON null）。
func (o *TransferHistoryOper) List(status string, limit int) ([]models.TransferHistory, error) {
	if limit <= 0 {
		limit = 200
	} else if limit > 500 {
		limit = 500
	}
	q := o.db.Order("created_at DESC").Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	hs := make([]models.TransferHistory, 0)
	return hs, q.Find(&hs).Error
}

// CountByStatus 按 status 计数（""=全部）。用于仪表盘汇总。
func (o *TransferHistoryOper) CountByStatus(status string) (int64, error) {
	q := o.db.Model(&models.TransferHistory{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var n int64
	return n, q.Count(&n).Error
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

// AddSkipped 记录一次跳过（如种子未下完未到整理阶段，避免误记 failed）。
func (o *TransferHistoryOper) AddSkipped(h *models.TransferHistory, reason string) error {
	h.Status = "skipped"
	h.ErrorMsg = reason
	return o.db.Create(h).Error
}

// Delete 删单条。
func (o *TransferHistoryOper) Delete(id uint) error {
	return o.db.Delete(&models.TransferHistory{}, id).Error
}

// DeleteMany 批量删除整理历史，返回「不存在」的 id 列表（同 SiteOper.DeleteMany）。
// 先 Pluck 存在集，再一条 SQL 删除已存在的，不存在的计入 failed —— 既能批量又能精确汇报逐条结果。
func (o *TransferHistoryOper) DeleteMany(ids []uint) ([]uint, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var found []uint
	if err := o.db.Model(&models.TransferHistory{}).Where("id IN ?", ids).Pluck("id", &found).Error; err != nil {
		return nil, err
	}
	foundSet := make(map[uint]struct{}, len(found))
	for _, id := range found {
		foundSet[id] = struct{}{}
	}
	var failed []uint
	for _, id := range ids {
		if _, ok := foundSet[id]; !ok {
			failed = append(failed, id)
		}
	}
	if len(found) > 0 {
		if err := o.db.Where("id IN ?", found).Delete(&models.TransferHistory{}).Error; err != nil {
			return nil, err
		}
	}
	return failed, nil
}

// DeleteByStatus 按 status 批量删除，返回删除条数（一键清失败：DELETE WHERE status='failed'，
// 仅限 failed，不动 success 以免破坏 SuccessExistsByHash/SrcPath 去重）。
func (o *TransferHistoryOper) DeleteByStatus(status string) (int64, error) {
	if status == "" {
		return 0, nil
	}
	res := o.db.Where("status = ?", status).Delete(&models.TransferHistory{})
	return res.RowsAffected, res.Error
}

// Cleanup 清理 before 之前的整理历史（每次下载完成落一行，含 Files JSON；无 TTL 会
// 无限膨胀，CountByStatus 全表 COUNT 与列表页随之变钝）。成功记录删除后 Sweep 的
// SuccessExistsByHash 去重会重新放行同名种子——因此 TTL 只清 180 天前的陈迹，
// 远超任何现实里的辅种/整理回看窗口，实际去重不受影响。
func (o *TransferHistoryOper) Cleanup(before time.Time) (int64, error) {
	res := o.db.Where("created_at < ?", before).Delete(&models.TransferHistory{})
	return res.RowsAffected, res.Error
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

// RecentFailureExistsByHash 判断指定下载器上的种子是否仍处于自动重试冷却期。
// 手动整理在 chain 层绕过该查询，用户修正配置后可立即验证。
func (o *TransferHistoryOper) RecentFailureExistsByHash(downloaderID uint, hash string, since time.Time) (bool, error) {
	if hash == "" || since.IsZero() {
		return false, nil
	}
	var count int64
	err := o.db.Model(&models.TransferHistory{}).
		Where("downloader_id = ? AND download_hash = ? AND status = ? AND (manual = ? OR manual IS NULL) AND created_at >= ?", downloaderID, hash, "failed", false, since).
		Count(&count).Error
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &l, nil
}
