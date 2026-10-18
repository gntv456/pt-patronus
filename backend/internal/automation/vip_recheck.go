package automation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notification"
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
		// 静默 = VIP 宽限复查整轮漏跑，用户可能在窗口外被锁死，读失败必须出声。
		if s.notif != nil {
			s.notif.Notify(ctx, notification.Message{
				Title: "VIP 复查", Body: "本轮复查未能开始：读取用户列表失败 — " + err.Error(), Level: "warning",
			})
		}
		return
	}
	// #365 「解不开 cookie」与「验证不通过」是三件不同的事，混在一起 continue 会锁死用户：
	//   - 好学不可达 / cookie 过期 / 非 VIP：该自然进宽限期（用户自己会重新登录）；
	//   - cookie 解不开（主密钥换过 / 密文损坏 / 算法升级）：这是**本地事故**，
	//     重试一万次也不会好，只有管理员重新保存 cookie 才能恢复。
	// 从前两者都 `continue` 静默跳过，于是解不开的用户的 VipVerifiedAt 永不刷新，
	// 到期后被 auth/guard.go 的 VIPExpired（403）挡在门外，界面上还看不出任何异常。
	// 本函数上面 8 行的读列表失败已经写了「静默 = 用户可能被锁死，读失败必须出声」，
	// 这里同样要出声 —— 通知说清是「凭据解不开」，管理员才知道该去重新保存而不是干等。
	undecryptable := 0
	for i := range users {
		u := &users[i]
		cookie, err := s.crypto.Decrypt(u.HxptCookieEnc)
		if err != nil {
			undecryptable++
			slog.Warn("vip recheck cookie undecryptable", "user", u.ID, "err", err)
			continue
		}
		if cookie == "" {
			continue // 本来就没存 cookie：不构成事故，不打扰
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
	// #365 汇总一条而不是每人一条：主密钥换过时是全员事故，逐个通知只会把通知中心刷爆
	// 并把真正需要处理的那条埋掉。措辞指向「去重新保存 cookie」，而不是「稍后重试」。
	if undecryptable > 0 && s.notif != nil {
		s.notif.Notify(ctx, notification.Message{
			Title: "VIP 复查",
			Body: fmt.Sprintf("有 %d 个用户的登录凭据解不开（可能是主密钥更换过或数据损坏）："+
				"这些用户的 VIP 复查已跳过，逾期会被拒绝访问。请在「用户」里重新保存好学 cookie", undecryptable),
			Level: "warning",
		})
	}
}
