// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package auth

import (
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// Guard is a Gin middleware that enforces the activation gate.
// When requireActivation is true and no code has been activated, only
// the /auth/* routes remain reachable (configured at the router level).
func Guard(act *Activation, requireActivation bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireActivation {
			c.Next()
			return
		}
		if !act.AnyActivated() {
			errcode.Respond(c, errcode.NotActivated)
			return
		}
		c.Next()
	}
}

// RequireVIP 强制好学 APP 专属 VIP 准入（按用户，替代实例级 Guard）。
// API-Key 旁路（程序化/应急 admin）放行；优先用 JWT claims 里的 vip_verified_at 快照
// 判宽限（不查库），落空回查 User.VipVerifiedAt；宽限内放行、超期 403。
func RequireVIP(db *gorm.DB, graceDays int) gin.HandlerFunc {
	grace := time.Duration(graceDays) * 24 * time.Hour
	return func(c *gin.Context) {
		if v, ok := c.Get("auth_via"); ok && v == "apikey" {
			c.Next()
			return
		}
		if raw, ok := c.Get("vip_verified_at"); ok {
			if ts, ok := raw.(int64); ok && ts > 0 && time.Since(time.Unix(ts, 0)) <= grace {
				c.Next()
				return
			}
		}
		uid, _ := c.Get("uid")
		id, _ := uid.(uint)
		if id == 0 {
			errcode.Respond(c, errcode.VIPNotVerified)
			return
		}
		var u models.User
		if err := db.Select("vip_verified_at").First(&u, id).Error; err != nil {
			errcode.Respond(c, errcode.VIPUnknown)
			return
		}
		if u.VipVerifiedAt == nil {
			errcode.Respond(c, errcode.VIPNotVerified)
			return
		}
		if time.Since(*u.VipVerifiedAt) > grace {
			errcode.Respond(c, errcode.VIPExpired)
			return
		}
		c.Next()
	}
}
