package chain

import (
	"encoding/json"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// subscribeGlobalDefaultsKey — 全局订阅默认设置存 SystemConfig 的 key（对齐 filter 包 PascalCase 风格）。
// 未单独设置某项的订阅，Check 时回退到此处的值（运行时回退，非创建时快照）。
const subscribeGlobalDefaultsKey = "SubscribeGlobalDefaults"

// GlobalDefaults 全局订阅默认设置。bool 在全局是二态（默认 false）；
// 订阅级 EnableRatingGuard/BestVersion 用 *bool 三态（nil=未设置→回退此全局默认）。
type GlobalDefaults struct {
	Resolution        string   `json:"resolution"`          // 空=不限
	PromoTypes        []string `json:"promo_types"`         // 空=不限（任何促销都接受）
	SiteIDs           []uint   `json:"site_ids"`            // 空=全部活跃站点
	EnableRatingGuard bool     `json:"enable_rating_guard"` // 默认关
	BestVersion       bool     `json:"best_version"`        // 默认关
}

// loadGlobalDefaults 读全局订阅默认；cfgOp nil（测试/未装配）或未配置返回零值。
func loadGlobalDefaults(cfgOp *oper.SystemConfigOper) GlobalDefaults {
	var gd GlobalDefaults
	if cfgOp == nil {
		return gd
	}
	raw, err := cfgOp.Get(subscribeGlobalDefaultsKey)
	if err != nil || raw == "" {
		return gd
	}
	_ = json.Unmarshal([]byte(raw), &gd)
	return gd
}

// saveGlobalDefaults 写全局订阅默认（JSON 序列化）。
func saveGlobalDefaults(cfgOp *oper.SystemConfigOper, gd GlobalDefaults) error {
	b, err := json.Marshal(gd)
	if err != nil {
		return err
	}
	return cfgOp.Set(subscribeGlobalDefaultsKey, string(b))
}

// LoadGlobalDefaults 读全局订阅默认设置（handler 用）。
func (c *SubscribeChain) LoadGlobalDefaults() GlobalDefaults { return loadGlobalDefaults(c.cfgOp) }

// SaveGlobalDefaults 写全局订阅默认设置（handler 用）。
func (c *SubscribeChain) SaveGlobalDefaults(gd GlobalDefaults) error { return saveGlobalDefaults(c.cfgOp, gd) }

// effectiveSettings 订阅各项设置经回退后的有效值（订阅自定义优先，空/nil 回退全局默认）。
type effectiveSettings struct {
	Resolution        string
	PromoTypes        []string
	SiteIDs           []uint
	EnableRatingGuard bool
	BestVersion       bool
}

// resolveEffective 计算订阅有效设置：订阅级值优先；resolution/promo_types/site_ids 空回退全局，
// EnableRatingGuard/BestVersion 的 *bool nil 回退全局（非 nil 用订阅值）。
func resolveEffective(sub *models.Subscription, gd GlobalDefaults) effectiveSettings {
	eff := effectiveSettings{
		Resolution: sub.Resolution,
		PromoTypes: sub.PromoTypes,
		SiteIDs:    sub.SiteIDs,
	}
	if eff.Resolution == "" {
		eff.Resolution = gd.Resolution
	}
	if len(eff.PromoTypes) == 0 {
		eff.PromoTypes = gd.PromoTypes
	}
	if len(eff.SiteIDs) == 0 {
		eff.SiteIDs = gd.SiteIDs
	}
	if sub.EnableRatingGuard != nil {
		eff.EnableRatingGuard = *sub.EnableRatingGuard
	} else {
		eff.EnableRatingGuard = gd.EnableRatingGuard
	}
	if sub.BestVersion != nil {
		eff.BestVersion = *sub.BestVersion
	} else {
		eff.BestVersion = gd.BestVersion
	}
	return eff
}
