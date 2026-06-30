package automation

import (
	"context"
	"log/slog"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// runVIPRecheck 对近期将过期的 VIP 用户用存的 cookie 重新 Verify，刷新 VipVerifiedAt。
// 提前在宽限窗口内重验，避免用户在好学偶发可达窗口外被锁死。
// 失败（好学不可达/cookie 过期/非 VIP）不更新，使其自然进宽限期/超期拒绝。
func (s *Service) runVIPRecheck() {
	if s.hxptAuth == nil || s.crypto == nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.rootCtx, 3*time.Minute)
	defer cancel()

	lead := s.cfg.VIPGraceDays - 1
	if lead < 1 {
		lead = 1
	}
	threshold := time.Now().Add(-time.Duration(lead) * 24 * time.Hour)
	var users []models.User
	if err := s.db.Select("id, hxpt_cookie_enc, vip_verified_at").
		Where("hxpt_cookie_enc <> '' AND vip_verified_at < ?", threshold).
		Find(&users).Error; err != nil {
		slog.Warn("vip recheck list failed", "err", err)
		return
	}
	for i := range users {
		u := &users[i]
		cookie, err := s.crypto.Decrypt(u.HxptCookieEnc)
		if err != nil || cookie == "" {
			continue
		}
		status := s.hxptAuth.Verify(ctx, cookie)
		if status.Verified {
			now := time.Now()
			if err := s.db.Model(&models.User{}).Where("id = ?", u.ID).
				Updates(map[string]any{"vip_verified_at": now, "vip_level": 1}).Error; err != nil {
				slog.Warn("vip recheck update failed", "user", u.ID, "err", err)
			} else {
				slog.Info("vip rechecked", "user", u.ID, "level", status.Level)
			}
		}
	}
}
