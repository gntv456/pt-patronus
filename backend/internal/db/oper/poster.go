package oper

import (
	"crypto/sha1"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// PosterCacheOper 封装海报/元数据缓存的读写。
type PosterCacheOper struct {
	db *gorm.DB
}

// NewPosterCacheOper 构造。
func NewPosterCacheOper(db *gorm.DB) *PosterCacheOper {
	return &PosterCacheOper{db: db}
}

// CacheKeyFrom 由关键字/年份/类型生成稳定缓存键。
func CacheKeyFrom(keyword string, year int, mtype string) string {
	k := strings.ToLower(strings.TrimSpace(keyword))
	h := sha1.Sum([]byte(strings.Join([]string{k, mtype, strconv.Itoa(year)}, "|")))
	return "search:" + hex.EncodeToString(h[:])
}

// Get 取缓存并自增命中次数。
func (o *PosterCacheOper) Get(key string) (*models.PosterCache, error) {
	var c models.PosterCache
	err := o.db.Where("cache_key = ?", key).First(&c).Error
	if err == nil {
		o.db.Model(&c).Where("id = ?", c.ID).UpdateColumn("hit_count", gorm.Expr("hit_count + ?", 1))
	}
	return &c, err
}

// Put upsert 一条缓存。
func (o *PosterCacheOper) Put(c *models.PosterCache) error {
	return o.db.Where("cache_key = ?", c.CacheKey).
		Assign(*c).
		FirstOrCreate(c).Error
}

// ClearSearch 清除所有搜索结果缓存（cache_key 以 "search:" 开头）。
// 媒体源凭据/域名变更后调用——旧结果集（如 TMDB 不可达时只剩豆瓣的残缺集）不应再被命中。
func (o *PosterCacheOper) ClearSearch() error {
	return o.db.Where("cache_key LIKE ?", "search:%").Delete(&models.PosterCache{}).Error
}
