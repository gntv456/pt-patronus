package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SubscriptionOper 封装订阅与命中记录的读写。
type SubscriptionOper struct {
	db *gorm.DB
}

func NewSubscriptionOper(db *gorm.DB) *SubscriptionOper {
	return &SubscriptionOper{db: db}
}

// List 全部订阅（按创建时间倒序）。
func (o *SubscriptionOper) List() ([]models.Subscription, error) {
	var ss []models.Subscription
	err := o.db.Order("created_at DESC").Find(&ss).Error
	normalizeSubSlices(ss)
	return ss, err
}

// normalizeSubSlices 把 nil 切片归一为空：DB NULL 文本经 Scan 得 nil；MarshalJSON
// 虽已把 nil 输出 []，但序列化前统一归一（含 Get/ListActive 同源数据），防 null 外漏。
func normalizeSubSlices(ss []models.Subscription) {
	for i := range ss {
		if ss[i].PromoTypes == nil {
			ss[i].PromoTypes = models.StringSlice{}
		}
		if ss[i].SiteIDs == nil {
			ss[i].SiteIDs = models.IntSlice{}
		}
	}
}

// ListActive 仅订阅中（status=subscribed）的订阅，供 cron 遍历。
func (o *SubscriptionOper) ListActive() ([]models.Subscription, error) {
	var ss []models.Subscription
	err := o.db.Where("status = ?", "subscribed").Order("last_checked_at NULLS FIRST").Find(&ss).Error
	return ss, err
}

// Get 单条。
func (o *SubscriptionOper) Get(id uint) (*models.Subscription, error) {
	var s models.Subscription
	if err := o.db.First(&s, id).Error; err != nil {
		return nil, err
	}
	normalizeSubSlices([]models.Subscription{s})
	return &s, nil
}

// FindActiveBySource 按源+source_id 找活跃订阅（创建时去重用）。
// subscribed 与 paused 都算「已存在」：paused 订阅再次订阅时由调用方复用复活，
// 否则会产生同源双订阅——恢复后同 torrent 双推送、洗版进度各维护一份。
func (o *SubscriptionOper) FindActiveBySource(source, sourceID string) (*models.Subscription, error) {
	var s models.Subscription
	err := o.db.Where("source = ? AND source_id = ? AND status IN ?", source, sourceID, []string{"subscribed", "paused"}).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Create 新建。
func (o *SubscriptionOper) Create(s *models.Subscription) error {
	return o.db.Create(s).Error
}

// Update 保存（resolution/downloader/status 等编辑）。
func (o *SubscriptionOper) Update(s *models.Subscription) error {
	return o.db.Save(s).Error
}

// UpdateEditableFields 仅写用户可编辑字段（列级 Updates），不覆盖 cron 维护的
// last_checked_at/current_priority/episode_priority，避免与 cron CheckAll 并发丢字段。
// 用于 SubscribeChain.Update（handler 编辑 / ResetFields）。updated_at 显式刷新
// （map 形式 Updates 不触发 GORM 自动时间戳）。
func (o *SubscriptionOper) UpdateEditableFields(s *models.Subscription) error {
	return o.db.Model(&models.Subscription{}).Where("id = ?", s.ID).Updates(map[string]any{
		"enable_rating_guard": s.EnableRatingGuard,
		"filter_groups":       s.FilterGroups,
		"best_version":        s.BestVersion,
		"best_version_full":   s.BestVersionFull,
		"official_only":       s.OfficialOnly,
		"promo_types":         s.PromoTypes,
		"site_ids":            s.SiteIDs,
		"resolution":          s.Resolution,
		"status":              s.Status,
		"downloader_id":       s.DownloaderID,
		"save_path":           s.SavePath,
		"updated_at":          time.Now(),
	}).Error
}

// SetPosterURL 仅回填海报，避免列表补图时用 Save 覆盖订阅的并发编辑。
func (o *SubscriptionOper) SetPosterURL(id uint, posterURL string) error {
	return o.db.Model(&models.Subscription{}).Where("id = ?", id).Update("poster_url", posterURL).Error
}

// Delete 删除订阅及其命中记录。
func (o *SubscriptionOper) Delete(id uint) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subscription_id = ?", id).Delete(&models.SubscriptionHit{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Subscription{}, id).Error
	})
}

// MarkChecked 记录本次检查时间。
func (o *SubscriptionOper) MarkChecked(id uint) error {
	now := time.Now()
	return o.db.Model(&models.Subscription{}).Where("id = ?", id).Update("last_checked_at", now).Error
}

// SetStatus 更新状态（subscribed/completed/paused）。
func (o *SubscriptionOper) SetStatus(id uint, status string) error {
	return o.db.Model(&models.Subscription{}).Where("id = ?", id).Update("status", status).Error
}

// SetBestVersionProgress 回填洗版进度：当前最优 PriOrder（电影）/ 按集 PriOffset JSON（剧集）。
// 用列级 Updates 避免全行 Save 覆盖并发变更。
func (o *SubscriptionOper) SetBestVersionProgress(id uint, currentPriority int, episodePriority string) error {
	return o.db.Model(&models.Subscription{}).Where("id = ?", id).
		Updates(map[string]any{"current_priority": currentPriority, "episode_priority": episodePriority}).Error
}

// HitExists 该订阅是否已推送过此 torrent_key（去重）。
func (o *SubscriptionOper) HitExists(subID uint, key string) (bool, error) {
	var count int64
	err := o.db.Model(&models.SubscriptionHit{}).
		Where("subscription_id = ? AND torrent_key = ?", subID, key).Count(&count).Error
	return count > 0, err
}

// AddHit 记录一次命中。
func (o *SubscriptionOper) AddHit(h *models.SubscriptionHit) error {
	// OnConflict DoNothing：(subscription_id,torrent_key) 唯一索引兜底，并发/重叠重复命中静默跳过。
	return o.db.Clauses(clause.OnConflict{DoNothing: true}).Create(h).Error
}
