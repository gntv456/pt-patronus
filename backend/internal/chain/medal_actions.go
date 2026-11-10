// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site/medalwall"
)

func (c *MedalChain) evict(siteID uint) {
	c.mu.Lock()
	delete(c.cache, siteID)
	c.mu.Unlock()
}

// validatePurchaseCandidate 购买前校验（best-effort，从 DB 读该站勋章的状态）。
func validatePurchaseCandidate(it *medalwall.Item) error {
	if it == nil {
		return nil // DB 无记录，交站点校验
	}
	switch it.PurchaseStatus {
	case medalwall.StatusOwned:
		return errors.New("已拥有该勋章")
	case medalwall.StatusNotOnSale:
		return errors.New("勋章未开售")
	case medalwall.StatusDiscontinued:
		return errors.New("勋章已下架")
	}
	if it.Stock == 0 {
		return errors.New("勋章库存为 0")
	}
	return nil
}

// Buy 购买勋章：校验 → adapter.Buy → 成功后清缓存重抓确认 isOwned。
func (c *MedalChain) Buy(ctx context.Context, s *models.Site, medalID string) (medalwall.ActionResult, error) {
	if !medalIDActionable(medalID) {
		return medalwall.ActionResult{Success: false, Message: "该勋章缺少可用的站点 ID，暂不支持写操作"}, nil
	}
	// in-flight 去重：防双击/重放重复购买扣站点魔力。
	guardKey := fmt.Sprintf("buy:%d:%s", s.ID, medalID)
	if !c.guard.acquire(guardKey) {
		return medalwall.ActionResult{Success: false, Message: "购买正在进行中，请勿重复触发"}, nil
	}
	defer c.guard.release(guardKey)
	def := c.siteCh.defFor(s)
	adapter := c.reg.Get(def.Schema)
	if !adapter.Supported() {
		return medalwall.ActionResult{Success: false, Message: "该平台不支持勋章购买"}, nil
	}
	// 校验（best-effort）
	for _, m := range mustFindBySite(c.op, s.ID) {
		if m.MedalID == medalID {
			it := medalToItem(m)
			if err := validatePurchaseCandidate(&it); err != nil {
				return medalwall.ActionResult{Success: false, Message: err.Error()}, nil
			}
			break
		}
	}
	cookie, cerr := c.siteCh.credentialWithErr(s, def)
	if cerr != nil {
		return medalwall.ActionResult{}, cerr
	}
	res, err := adapter.Buy(ctx, c.hc, medalDef(def), s.BaseURL, cookie, medalID)
	if err != nil {
		return medalwall.ActionResult{Success: false, Message: "请求失败：" + err.Error()}, nil
	}
	if res.Success {
		c.evict(s.ID)
		_, _ = c.medalSite(ctx, nil, s, true) // 重抓确认
	}
	return res, nil
}

// Wear 佩戴勋章。
func (c *MedalChain) Wear(ctx context.Context, s *models.Site, medalID string) (medalwall.ActionResult, error) {
	return c.formAction(ctx, s, medalID, "wear")
}

// TakeOff 摘下勋章。
func (c *MedalChain) TakeOff(ctx context.Context, s *models.Site, medalID string) (medalwall.ActionResult, error) {
	return c.formAction(ctx, s, medalID, "takeoff")
}

func (c *MedalChain) formAction(ctx context.Context, s *models.Site, medalID, action string) (medalwall.ActionResult, error) {
	if !medalIDActionable(medalID) {
		return medalwall.ActionResult{Success: false, Message: "该勋章缺少可用的站点 ID，暂不支持写操作"}, nil
	}
	def := c.siteCh.defFor(s)
	adapter := c.reg.Get(def.Schema)
	if !adapter.Supported() {
		return medalwall.ActionResult{Success: false, Message: "该平台不支持勋章操作"}, nil
	}
	cookie, cerr := c.siteCh.credentialWithErr(s, def)
	if cerr != nil {
		return medalwall.ActionResult{}, cerr
	}
	var res medalwall.ActionResult
	var err error
	switch action {
	case "wear":
		res, err = adapter.Wear(ctx, c.hc, medalDef(def), s.BaseURL, cookie, medalID)
	case "takeoff":
		res, err = adapter.TakeOff(ctx, c.hc, medalDef(def), s.BaseURL, cookie, medalID)
	}
	if err != nil {
		return medalwall.ActionResult{Success: false, Message: "请求失败：" + err.Error()}, nil
	}
	if res.Success {
		c.evict(s.ID)
		_, _ = c.medalSite(ctx, nil, s, true)
	}
	return res, nil
}
