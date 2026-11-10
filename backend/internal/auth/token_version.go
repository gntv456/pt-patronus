// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package auth

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// ErrUserMissing：token 指向的账号已不在库里（删号、换数据目录、恢复备份）。
// 调用方必须把它与「读不出来」分开：前者该让用户重新登录，后者不该把锅甩给会话。
var ErrUserMissing = errors.New("token version: user row missing")

// TokenVersionCache 缓存 uid→TokenVersion，避免 JWTAuth 每请求查库；登出/改绑时 Invalidate。
// 多实例部署下 Invalidate 仅对本进程即时生效，其它实例最多滞后 TTL（默认 60s）；
// 当前按单实例设计，多副本需引入 Redis pub/sub 或缩短 TTL。
type TokenVersionCache struct {
	mu      sync.RWMutex
	entries map[uint]tvEntry
	ttl     time.Duration
	db      *gorm.DB
}

type tvEntry struct {
	version   uint
	fetchedAt time.Time
}

// NewTokenVersionCache 构造。ttl 为缓存有效期。
func NewTokenVersionCache(db *gorm.DB, ttl time.Duration) *TokenVersionCache {
	return &TokenVersionCache{entries: map[uint]tvEntry{}, ttl: ttl, db: db}
}

// Get 返回该 uid 的当前 TokenVersion（命中且未过期则不查库）。
func (c *TokenVersionCache) Get(uid uint) (uint, error) {
	c.mu.RLock()
	if e, ok := c.entries[uid]; ok && time.Since(e.fetchedAt) < c.ttl {
		c.mu.RUnlock()
		return e.version, nil
	}
	c.mu.RUnlock()

	var u models.User
	if err := c.db.Select("token_version").First(&u, uid).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, fmt.Errorf("%w: uid=%d", ErrUserMissing, uid)
		}
		return 0, err
	}
	c.mu.Lock()
	c.entries[uid] = tvEntry{version: u.TokenVersion, fetchedAt: time.Now()}
	c.mu.Unlock()
	return u.TokenVersion, nil
}

// Invalidate 使下次 Get 回查 DB（登出/改绑后调用）。
func (c *TokenVersionCache) Invalidate(uid uint) {
	c.mu.Lock()
	delete(c.entries, uid)
	c.mu.Unlock()
}
