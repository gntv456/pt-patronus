package app

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// bootstrapLocalAdmin 让 compose 声明本地账号，并让好学 VIP 存量实例升级后零手工迁移。
//
// 业务配置（sites / downloaders / subscriptions / system_configs 等）是全局表，不按
// 本地 user_id 归属；唯一按 uid 过滤的是 UserPreference。因此升级保住 User.ID 即可，
// 不需要导出/导入任何数据——本函数对存量账号只补 password_hash，绝不新建平行账号。
//
// 语义（改动前请先读）：
//   - 空库 + 配了密码 → 启动即建管理员，堵住「首登谁先访问谁拿 admin」抢注窗口。
//   - 存量账号（PasswordHash 为空，好学 VIP/OIDC 时代遗留）+ 配了密码 → 原地补本地密码，
//     不改 ID / Username / 任何业务数据；这是升级的无感迁移入口，替代手工 reset-admin CLI。
//   - 已有密码 → 只告警、绝不覆盖。env 每次启动都在，允许覆盖会把用户在 UI 改过的密码打回。
//   - 未配密码 + 存量无密码账号 → 打一条可操作的 WARN，不阻断启动。
//   - 密码不合规则 / secrets 文件读不到 → 启动失败，不静默留一个弱口令或没账号的实例。
//   - 密码绝不进日志、绝不回显。
func bootstrapLocalAdmin(database *gorm.DB, cfg *config.Config) error {
	// 与 v1.existingRealUserUID 同口径：demo / oidc / emergency 不是实例主人。
	// 无论有没有配密码都要查一次，用于「存量未迁移」的启动提示。
	var existing models.User
	err := database.Where(
		"hxpt_uid NOT LIKE 'oidc:%' AND hxpt_uid NOT LIKE 'emergency:%' " +
			"AND hxpt_uid != 'demo'").
		Order("id ASC").First(&existing).Error
	hasOwner := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("bootstrap admin: lookup owner: %w", err)
	}

	pwd, err := resolveAdminPassword(cfg)
	if err != nil {
		return err
	}
	if pwd == "" {
		if hasOwner && existing.PasswordHash == "" {
			slog.Warn("存量账号没有本地密码：设 ADMIN_PASSWORD 重启或跑 reset-admin --password",
				"username", existing.Username)
		}
		return nil
	}
	if !security.ValidatePassword(pwd) {
		return fmt.Errorf("ADMIN_PASSWORD 不符合密码规则：至少 8 位且包含字母和数字")
	}
	hash, err := security.HashPassword(pwd)
	if err != nil {
		return fmt.Errorf("bootstrap admin: hash: %w", err)
	}

	if hasOwner {
		if existing.PasswordHash != "" {
			slog.Warn("ADMIN_PASSWORD ignored: owner already has a local password",
				"username", existing.Username)
			return nil
		}
		// 升级迁移：只补密码，User.ID 不变 → UserPreference 与全部全局配置原样可读。
		// 与 reset-admin --password 对齐：补管理员标记 + VIP 历史快照。
		now := time.Now()
		updates := map[string]interface{}{"password_hash": hash, "is_admin": true}
		if existing.VipVerifiedAt == nil {
			updates["vip_verified_at"] = now
		}
		q := database.Model(&models.User{}).Where("id = ?", existing.ID)
		if err := q.Updates(updates).Error; err != nil {
			return fmt.Errorf("bootstrap admin: migrate legacy owner: %w", err)
		}
		slog.Info("bootstrap admin: legacy owner password initialized",
			"user_id", existing.ID, "username", existing.Username)
		return nil
	}

	username := strings.TrimSpace(cfg.AdminUsername)
	if username == "" {
		username = "admin"
	}
	if len(username) > 64 {
		return fmt.Errorf("ADMIN_USERNAME 超过 64 字符")
	}
	now := time.Now()
	u := models.User{
		Username: username, PasswordHash: hash,
		HxptUID: "local:" + username, HxptUsername: username,
		VipLevel: 1, VipVerifiedAt: &now, IsAdmin: true,
	}
	if err := database.Create(&u).Error; err != nil {
		// 并发启动兜底：另一进程已抢先建好账号就不算错误（username 有唯一索引）。
		var again models.User
		if database.Where("username = ?", username).First(&again).Error == nil {
			slog.Warn("bootstrap admin raced; using existing account",
				"username", username)
			return nil
		}
		return fmt.Errorf("bootstrap admin: create: %w", err)
	}
	slog.Info("bootstrap admin created from compose env", "username", username)
	return nil
}

// resolveAdminPassword 优先 ADMIN_PASSWORD_FILE（Docker secrets / 0600 文件），
// 其次 ADMIN_PASSWORD 明文。文件内容去掉首尾空白（secrets 常带结尾换行）。
func resolveAdminPassword(cfg *config.Config) (string, error) {
	if cfg.AdminPasswordFile == "" {
		return cfg.AdminPassword, nil
	}
	b, err := os.ReadFile(cfg.AdminPasswordFile)
	if err != nil {
		return "", fmt.Errorf("ADMIN_PASSWORD_FILE 读不到: %w", err)
	}
	pwd := strings.TrimSpace(string(b))
	if pwd == "" {
		return "", fmt.Errorf("ADMIN_PASSWORD_FILE 内容为空: %s", cfg.AdminPasswordFile)
	}
	return pwd, nil
}
