package oper

import (
	"errors"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// CloudTaskOper 云端任务队列 CRUD（进度/重试/历史）。
type CloudTaskOper struct {
	db *gorm.DB
}

func NewCloudTaskOper(db *gorm.DB) *CloudTaskOper { return &CloudTaskOper{db: db} }

// Enqueue 入队一条任务（pending）。
func (o *CloudTaskOper) Enqueue(t *models.CloudTask) error {
	t.Status = "pending"
	if t.Progress != 0 {
		t.Progress = 0
	}
	return o.db.Create(t).Error
}

// ClaimPending 按创建序取一条 pending 任务（executor 轮询用）。
func (o *CloudTaskOper) ClaimPending() (*models.CloudTask, error) {
	var t models.CloudTask
	err := o.db.Where("status = ?", "pending").Order("id asc").First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// 抢占：pending→running（乐观更新，并发 executor 下只有一个成功）
	res := o.db.Model(&models.CloudTask{}).Where("id = ? AND status = ?", t.ID, "pending").
		Update("status", "running")
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil // 被别的 worker 抢走
	}
	return &t, nil
}

// Succeed 标记成功（progress=100）。
func (o *CloudTaskOper) Succeed(id uint) error {
	return o.db.Model(&models.CloudTask{}).Where("id = ?", id).
		Updates(map[string]any{"status": "success", "progress": 100, "error": ""}).Error
}

// Fail 标记失败；attempts<max 时自动重入队（pending），否则终态 failed。
func (o *CloudTaskOper) Fail(id uint, errMsg string, maxRetries int) error {
	var t models.CloudTask
	if err := o.db.First(&t, id).Error; err != nil {
		return err
	}
	t.Attempts++
	t.Error = errMsg
	if maxRetries > 0 && t.Attempts <= maxRetries {
		t.Status = "pending" // 重入队（退避由 executor 控制）
	} else {
		t.Status = "failed"
	}
	return o.db.Save(&t).Error
}

// UpdateProgress 更新进度（0-100，粗粒度阶段）。
func (o *CloudTaskOper) UpdateProgress(id uint, pct int) error {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return o.db.Model(&models.CloudTask{}).Where("id = ?", id).
		Update("progress", pct).Error
}

// List 按状态列任务（status 空=全部；时间倒序，limit 上限 200）。
func (o *CloudTaskOper) List(status string, limit int) ([]models.CloudTask, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := o.db.Order("id desc").Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var ts []models.CloudTask
	if err := q.Find(&ts).Error; err != nil {
		return nil, err
	}
	return ts, nil
}

// Retry 把 failed 任务重入队（手动重试入口）。
func (o *CloudTaskOper) Retry(id uint) error {
	return o.db.Model(&models.CloudTask{}).Where("id = ? AND status = ?", id, "failed").
		Updates(map[string]any{"status": "pending", "attempts": 0, "error": ""}).Error
}

// Delete 删除任务记录。
func (o *CloudTaskOper) Delete(id uint) error {
	return o.db.Delete(&models.CloudTask{}, id).Error
}

// RequeueStaleRunning 把 running 超时的任务重置 pending（进程重启恢复）。
func (o *CloudTaskOper) RequeueStaleRunning(olderThan time.Duration) (int64, error) {
	res := o.db.Model(&models.CloudTask{}).
		Where("status = ? AND updated_at < ?", "running", time.Now().Add(-olderThan)).
		Update("status", "pending")
	return res.RowsAffected, res.Error
}

// PendingCount 待处理数（看板用）。
func (o *CloudTaskOper) PendingCount() (int64, error) {
	var n int64
	err := o.db.Model(&models.CloudTask{}).Where("status IN ?", []string{"pending", "running"}).
		Count(&n).Error
	return n, err
}

// PendingStats 待处理数 + 最久等待时长（doctor 队列体检用）。
func (o *CloudTaskOper) PendingStats() (int64, time.Duration, error) {
	var n int64
	if err := o.db.Model(&models.CloudTask{}).Where("status IN ?", []string{"pending", "running"}).
		Count(&n).Error; err != nil {
		return 0, 0, err
	}
	if n == 0 {
		return 0, 0, nil
	}
	var oldest models.CloudTask
	if err := o.db.Where("status IN ?", []string{"pending", "running"}).
		Order("created_at asc").First(&oldest).Error; err != nil {
		return n, 0, err
	}
	return n, time.Since(oldest.CreatedAt), nil
}
