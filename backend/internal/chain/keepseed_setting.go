package chain

import (
	"errors"
	"log/slog"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// loadSetting 取该下载器的辅种设置（save_path 覆盖 / 添加后暂停）；无记录返默认值
// （PauseAfterAdd=true，与 Setting/模型 default 一致——避免无 setting 行时「不暂停」添加）。
func (c *KeepSeedChain) loadSetting(downloaderID uint) models.CrossSeedSetting {
	s, err := c.Setting(downloaderID)
	if err != nil {
		// DB 瞬态错误：返默认值（PauseAfterAdd=true）而非零值（false），避免种子加下载器不暂停违反文档默认。
		slog.Warn("keepseed loadSetting failed, using default", "downloader", downloaderID, "err", err)
		return models.CrossSeedSetting{DownloaderID: downloaderID, PauseAfterAdd: true}
	}
	return s
}

// Setting 取该下载器的辅种设置（无记录返默认值行：PauseAfterAdd=true 对齐模型 default）。
func (c *KeepSeedChain) Setting(downloaderID uint) (models.CrossSeedSetting, error) {
	var s models.CrossSeedSetting
	err := c.db.Where("downloader_id = ?", downloaderID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.CrossSeedSetting{DownloaderID: downloaderID, PauseAfterAdd: true}, nil
	}
	return s, err
}

// SaveSetting 按 downloader_id upsert 辅种设置（AutoReseed/AutoReseedAdd/SavePath/PauseAfterAdd）。
func (c *KeepSeedChain) SaveSetting(s models.CrossSeedSetting) error {
	var row models.CrossSeedSetting
	// #386 从前这一行是裸调（错误连 _ 都没写）：查不到与查不动都让 row 保持零值，
	// 于是 row.ID==0 一律走 Create。DB 只是**读**故障时（连接池打满 / database is locked
	// 落在读路径 / 只读副本滞后）Create 可能仍成功 ⇒ 同一个 downloader_id 插出第二行，
	// 此后 Setting 取哪行不确定、ListAutoReseedSettings 会把同一台列两遍、cron 重复触发。
	// 同文件的 Setting() 分得清清楚楚，这里必须同一口径：不存在才创建，其余一律报出去。
	findErr := c.db.Where("downloader_id = ?", s.DownloaderID).First(&row).Error
	if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return findErr
	}
	// map-based Updates 强制写入零值 bool(false)：GORM Create 对「带 default 标签的零值字段」会
	// 省略插入、由 DB 填默认（PauseAfterAdd default:true 会把 false 落库为 true，实测如此）；用 map 绕开。
	updates := map[string]any{
		"save_path":       s.SavePath,
		"pause_after_add": s.PauseAfterAdd,
		"auto_reseed":     s.AutoReseed,
		"auto_reseed_add": s.AutoReseedAdd,
	}
	if row.ID == 0 {
		s.ID = 0
		if err := c.db.Create(&s).Error; err != nil {
			return err
		}
		return c.db.Model(&models.CrossSeedSetting{}).Where("id = ?", s.ID).Updates(updates).Error
	}
	row.SavePath = s.SavePath
	row.PauseAfterAdd = s.PauseAfterAdd
	row.AutoReseed = s.AutoReseed
	row.AutoReseedAdd = s.AutoReseedAdd
	return c.db.Save(&row).Error
}

// ListAutoReseedSettings 列出所有启用定时自动辅种的下载器设置（cron 用）。
func (c *KeepSeedChain) ListAutoReseedSettings() ([]models.CrossSeedSetting, error) {
	var out []models.CrossSeedSetting
	if err := c.db.Where("auto_reseed = ?", true).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
