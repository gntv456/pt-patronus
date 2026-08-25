package chain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/medalwall"
)

func medalKey(it medalwall.Item) string {
	if it.MedalID != "" {
		return "id:" + it.MedalID
	}
	return "name:" + medalNormName(it.Name)
}

// medalOOS 是否缺货（状态 OUT_OF_STOCK 或库存 0）。
func medalOOS(it medalwall.Item) bool {
	return it.PurchaseStatus == medalwall.StatusOutOfStock || it.Stock == 0
}

// fmtMedalPrice 价格 + 货币展示。
func fmtMedalPrice(it medalwall.Item) string {
	return fmt.Sprintf("%.0f %s", it.Price, it.Currency)
}

// mustFindBySite 读 DB（忽略错误，返回空切片）。
func mustFindBySite(op *oper.MedalOper, siteID uint) []models.Medal {
	ms, _ := op.FindBySite(siteID)
	if ms == nil {
		return []models.Medal{}
	}
	return ms
}

// discoverUID 从 usercp.php 抠当前用户 uid（site.ExtractUIDAndName 扫 userdetails.php?id= 链接）。
func (c *MedalChain) discoverUID(ctx context.Context, s *models.Site, def *site.SiteMetadata, cookie string) string {
	path := def.UserInfo.Path
	if path == "" {
		path = "usercp.php"
	}
	resp, err := c.hc.Get(ctx, medalJoinBase(s.BaseURL, path), cookie, true)
	if err != nil || resp == nil || len(resp.Body) < 200 {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return ""
	}
	uid, _ := site.ExtractUIDAndName(doc)
	return uid
}

// mergeMedals 商店勋章与已拥有勋章合并：medalId 精确 → 名称精确 → 名称模糊（contains）三级匹配。
// 命中则置 PurchaseStatus=OWNED + 回填 WearStatus；未命中的 owned（商店无售但用户拥有/佩戴，
// 如等级勋章、成就勋章）追加到结果标 OWNED——这样「佩戴中」筛选能看到非商店来源的勋章。
func mergeMedals(shop []medalwall.Item, owned []medalwall.Owned) []medalwall.Item {
	if len(owned) == 0 {
		return shop
	}
	byID := map[string]int{}
	byName := map[string]int{}
	ownedNorm := make([]string, 0, len(owned))
	for i, o := range owned {
		if o.MedalID != "" {
			byID[o.MedalID] = i
		}
		if o.Name != "" {
			n := medalNormName(o.Name)
			byName[n] = i
			ownedNorm = append(ownedNorm, n)
		}
	}
	matched := make([]bool, len(owned))
	for i := range shop {
		it := &shop[i]
		oi := -1
		if it.MedalID != "" {
			if idx, ok := byID[it.MedalID]; ok {
				oi = idx
			}
		}
		if oi < 0 && it.Name != "" {
			if idx, ok := byName[medalNormName(it.Name)]; ok {
				oi = idx
			}
		}
		if oi < 0 && it.Name != "" {
			n := medalNormName(it.Name)
			for _, on := range ownedNorm {
				if on != "" && (strings.Contains(on, n) || strings.Contains(n, on)) {
					oi = byName[on]
					break
				}
			}
		}
		if oi >= 0 {
			it.PurchaseStatus = medalwall.StatusOwned
			it.WearStatus = owned[oi].Wearing
			matched[oi] = true
		}
	}
	// 未匹配的 owned（商店无售但拥有/佩戴）追加，标 OWNED
	for i, o := range owned {
		if matched[i] || o.Name == "" {
			continue
		}
		shop = append(shop, medalwall.Item{
			MedalID:          "owned:" + medalNormName(o.Name), // 商店无售的拥有勋章无真实 id，用 name 派生避 UNIQUE(site_id,medal_id) 冲突
			Name:             o.Name,
			ImageSmall:       o.Image,
			OriginalImageURL: o.Image,
			PurchaseStatus:   medalwall.StatusOwned,
			WearStatus:       o.Wearing,
			Currency:         "魔力",
		})
	}
	return shop
}

// ensureMedalIDs gives parser rows without a real site id a stable read-only id
// so DB upserts cannot collide on (site_id, ""). Synthetic ids are blocked from
// write actions below because they cannot be sent back to the site safely.
func ensureMedalIDs(items []medalwall.Item) []medalwall.Item {
	variantsByID := make(map[string]map[string]struct{})
	for _, it := range items {
		id := strings.TrimSpace(it.MedalID)
		if id == "" {
			continue
		}
		if variantsByID[id] == nil {
			variantsByID[id] = make(map[string]struct{})
		}
		variantsByID[id][medalVariantKey(it)] = struct{}{}
	}

	out := make([]medalwall.Item, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		variantKey := medalVariantKey(it)
		exactKey := strings.TrimSpace(it.MedalID) + "\x00" + variantKey
		if _, ok := seen[exactKey]; ok {
			continue
		}
		seen[exactKey] = struct{}{}

		id := strings.TrimSpace(it.MedalID)
		if id == "" || len(variantsByID[id]) > 1 {
			sum := sha256.Sum256([]byte(id + "\x00" + variantKey))
			it.MedalID = "derived:" + hex.EncodeToString(sum[:8])
			if it.PurchaseStatus == medalwall.StatusAvailable {
				it.PurchaseStatus = medalwall.StatusUnknown
			}
		}
		out = append(out, it)
	}
	return out
}

func medalVariantKey(it medalwall.Item) string {
	return medalNormName(it.Name) + "\x00" + it.ImageSmall
}

func medalIDActionable(id string) bool {
	return id != "" && !strings.HasPrefix(id, "derived:") &&
		!strings.HasPrefix(id, "owned:") && !strings.HasPrefix(id, "syn:")
}

// medalNormName 归一名（trim + lower + 去空白），合并匹配用。
func medalNormName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Join(strings.Fields(s), "")
}

// medalDef 把 site.SiteMetadata 投影为 medalwall.Def。
func medalDef(def *site.SiteMetadata) medalwall.Def {
	return medalwall.Def{Schema: def.Schema, ShopPath: def.Medal.ShopPath, OwnPath: def.Medal.OwnPath}
}

// medalJoinBase 拼 baseURL + 相对路径。
func medalJoinBase(base, path string) string {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + "/" + path
}

// itemToCard 把勋章条目转为 UI 卡片（IsOwned 由 PurchaseStatus 派生）。
func itemToCard(siteID uint, siteName string, it medalwall.Item) MedalCard {
	return MedalCard{
		SiteID:           siteID,
		SiteName:         siteName,
		MedalID:          it.MedalID,
		Name:             it.Name,
		Description:      it.Description,
		ImageSmall:       it.ImageSmall,
		OriginalImageURL: it.OriginalImageURL,
		SaleBeginTime:    it.SaleBeginTime,
		SaleEndTime:      it.SaleEndTime,
		Price:            it.Price,
		OriginalPrice:    it.OriginalPrice,
		Validity:         it.Validity,
		BonusRate:        it.BonusRate,
		PurchaseStatus:   string(it.PurchaseStatus),
		Stock:            it.Stock,
		Currency:         it.Currency,
		Group:            it.Group,
		WearStatus:       it.WearStatus,
		IsOwned:          it.PurchaseStatus == medalwall.StatusOwned,
		Actionable:       medalIDActionable(it.MedalID),
	}
}

// itemsToMedals / medalsToItems：medalwall.Item ↔ models.Medal 互转。
func itemsToMedals(items []medalwall.Item) []models.Medal {
	out := make([]models.Medal, 0, len(items))
	for _, it := range items {
		out = append(out, models.Medal{
			MedalID:          it.MedalID,
			Name:             it.Name,
			Description:      it.Description,
			ImageSmall:       it.ImageSmall,
			OriginalImageURL: it.OriginalImageURL,
			SaleBeginTime:    it.SaleBeginTime,
			SaleEndTime:      it.SaleEndTime,
			Price:            it.Price,
			OriginalPrice:    it.OriginalPrice,
			Validity:         it.Validity,
			BonusRate:        it.BonusRate,
			PurchaseStatus:   string(it.PurchaseStatus),
			GiftStatus:       it.GiftStatus,
			Stock:            it.Stock,
			StockStatus:      it.StockStatus,
			Currency:         it.Currency,
			GroupName:        it.Group,
			WearStatus:       it.WearStatus,
		})
	}
	return out
}

func medalsToItems(ms []models.Medal) []medalwall.Item {
	out := make([]medalwall.Item, 0, len(ms))
	for _, m := range ms {
		out = append(out, medalwall.Item{
			MedalID:          m.MedalID,
			Name:             m.Name,
			Description:      m.Description,
			ImageSmall:       m.ImageSmall,
			OriginalImageURL: m.OriginalImageURL,
			SaleBeginTime:    m.SaleBeginTime,
			SaleEndTime:      m.SaleEndTime,
			Price:            m.Price,
			OriginalPrice:    m.OriginalPrice,
			Validity:         m.Validity,
			BonusRate:        m.BonusRate,
			PurchaseStatus:   medalwall.Status(m.PurchaseStatus),
			GiftStatus:       m.GiftStatus,
			Stock:            m.Stock,
			StockStatus:      m.StockStatus,
			Currency:         m.Currency,
			Group:            m.GroupName,
			WearStatus:       m.WearStatus,
		})
	}
	return out
}

// medalToItem 单条 model→item。
func medalToItem(m models.Medal) medalwall.Item {
	return medalsToItems([]models.Medal{m})[0]
}

// evict 清单站内存缓存（写操作后强制重抓用）。
