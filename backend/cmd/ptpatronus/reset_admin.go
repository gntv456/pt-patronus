package main

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// runResetAdmin 离线应急：创建/重置一个 HxptUID=cli:admin 的管理员（含本地 VIP 标记），
// 打印临时 JWT。用于好学 POST 登录链路坏掉、所有人无法登录时的恢复入口。
// 不走 HTTP、完全旁路好学，仅由运维在服务器执行。
func runResetAdmin() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	database, err := db.Open(cfg)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	j := security.NewJWT(cfg.SecretKey, cfg.JWTExpireMin)

	var u models.User
	err = database.Where("hxpt_uid = ?", "cli:admin").First(&u).Error
	now := time.Now()
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		u = models.User{
			Username: "cli-admin", HxptUID: "cli:admin", HxptUsername: "cli-admin",
			IsAdmin: true, VipLevel: 1, VipVerifiedAt: &now,
		}
		if err := database.Create(&u).Error; err != nil {
			return fmt.Errorf("create: %w", err)
		}
	case err != nil:
		return fmt.Errorf("query: %w", err)
	default:
		u.IsAdmin = true
		u.VipLevel = 1
		u.VipVerifiedAt = &now
		if err := database.Save(&u).Error; err != nil {
			return fmt.Errorf("save: %w", err)
		}
	}

	tok, err := j.Issue(u.ID, u.Username, u.IsAdmin, u.TokenVersion, u.VipLevel, now)
	if err != nil {
		return fmt.Errorf("issue: %w", err)
	}
	slog.Info("reset-admin ok", "user_id", u.ID)
	fmt.Println("应急管理员已就绪（HxptUID=cli:admin，本地 VIP 标记有效）。")
	fmt.Println("用以下 token 作为 Authorization: Bearer 登录后端/Web：")
	fmt.Println(tok)
	return nil
}
