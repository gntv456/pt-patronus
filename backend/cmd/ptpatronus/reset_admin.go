package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// runResetAdmin 离线应急：两个用途（好学站点关闭后的本地密码主入口，#<issue>）——
//  1. 无参数（旧行为兼容）：创建/重置 HxptUID=cli:admin 的管理员，打印临时 JWT。
//  2. --password <新密码>：给实例主人（首个真实用户；无则建 cli:admin）设置本地
//     密码，之后用 Web/移动端登录页的本地密码登录，不再打印 token。
//
// 不走 HTTP、完全旁路好学，仅由运维在服务器执行。
func runResetAdmin(args []string) error {
	fs := flag.NewFlagSet("reset-admin", flag.ContinueOnError)
	pw := fs.String("password", "", "为实例主人设置本地登录密码（至少 8 位且含字母和数字）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	database, err := db.Open(cfg)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}

	if *pw != "" {
		return resetAdminPassword(database, *pw)
	}

	// 旧路径：打印临时 JWT（保留兼容，推荐改用 --password 走正式登录）。
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
	fmt.Println("推荐改用：ptpatronus reset-admin --password <新密码> 走本地密码登录。")
	fmt.Println("用以下 token 作为 Authorization: Bearer 登录后端/Web：")
	fmt.Println(tok)
	return nil
}

// resetAdminPassword 给实例主人设本地密码：优先首个真实用户（好学站点关闭后
// 存量实例的主人就是首位登录者），无真实用户则创建 cli:admin 管理员并设密。
func resetAdminPassword(database *gorm.DB, password string) error {
	if len(password) < 8 || !strings.ContainsFunc(password, func(r rune) bool {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
	}) || !strings.ContainsFunc(password, func(r rune) bool {
		return r >= '0' && r <= '9'
	}) {
		return fmt.Errorf("密码至少 8 位且需包含字母和数字")
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	var u models.User
	// 与 v1.existingRealUserUID 同口径：排除 oidc/emergency/demo 的首个真实用户。
	err = database.Where(
		"hxpt_uid NOT LIKE 'oidc:%' AND hxpt_uid NOT LIKE 'emergency:%'" +
			" AND hxpt_uid != 'demo'").
		Order("id ASC").First(&u).Error
	now := time.Now()
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		u = models.User{
			Username: "admin", HxptUID: "local:admin", HxptUsername: "admin",
			IsAdmin: true, VipLevel: 1, VipVerifiedAt: &now, PasswordHash: hash,
		}
		if err := database.Create(&u).Error; err != nil {
			return fmt.Errorf("create: %w", err)
		}
	case err != nil:
		return fmt.Errorf("query: %w", err)
	default:
		u.IsAdmin = true
		u.PasswordHash = hash
		if u.VipVerifiedAt == nil {
			u.VipVerifiedAt = &now
		}
		if err := database.Save(&u).Error; err != nil {
			return fmt.Errorf("save: %w", err)
		}
	}
	slog.Info("reset-admin password ok", "user_id", u.ID, "username", u.Username)
	fmt.Printf("已为实例主人 %q 设置本地密码。请在登录页用该用户名 + 新密码登录。\n", u.Username)
	return nil
}
