package chain

import (
	"context"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site/medalwall"
)

// dispatchMedalChanges 对比新旧勋章，对新勋章/上架/降价/涨价/售罄变动发通知（当天去重）。
func (c *MedalChain) dispatchMedalChanges(ctx context.Context, s *models.Site, old, cur []medalwall.Item) {
	oldByID := map[string]medalwall.Item{}
	for _, o := range old {
		oldByID[medalKey(o)] = o
	}
	for _, n := range cur {
		o, hadOld := oldByID[medalKey(n)]
		if !hadOld {
			if n.PurchaseStatus == medalwall.StatusAvailable {
				_ = c.notice.Notice(ctx, notice.Input{
					Source: notice.SourceMedalNew, Key: fmt.Sprintf("medal:%d:%s:new", s.ID, n.MedalID),
					Title: "新勋章：" + s.Name, Body: n.Name + " 可购买（" + fmtMedalPrice(n) + "）", Level: "info",
				}, true)
			}
			continue
		}
		// 价格变动
		if o.Price > 0 && n.Price > 0 && o.Price != n.Price {
			if n.Price < o.Price {
				_ = c.notice.Notice(ctx, notice.Input{
					Source: notice.SourceMedalPriceDrop, Key: fmt.Sprintf("medal:%d:%s:drop", s.ID, n.MedalID),
					Title: "勋章降价：" + s.Name, Body: fmt.Sprintf("%s %s → %s", n.Name, fmtMedalPrice(o), fmtMedalPrice(n)), Level: "success",
				}, true)
			} else {
				_ = c.notice.Notice(ctx, notice.Input{
					Source: notice.SourceMedalPriceRise, Key: fmt.Sprintf("medal:%d:%s:rise", s.ID, n.MedalID),
					Title: "勋章涨价：" + s.Name, Body: fmt.Sprintf("%s %s → %s", n.Name, fmtMedalPrice(o), fmtMedalPrice(n)), Level: "warning",
				}, true)
			}
		}
		// 上架可购买
		if o.PurchaseStatus != medalwall.StatusAvailable && n.PurchaseStatus == medalwall.StatusAvailable {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceMedalAvailable, Key: fmt.Sprintf("medal:%d:%s:avail", s.ID, n.MedalID),
				Title: "勋章上架：" + s.Name, Body: n.Name + " 现可购买", Level: "info",
			}, true)
		}
		// 售罄
		if !medalOOS(o) && medalOOS(n) {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceMedalOutOfStock, Key: fmt.Sprintf("medal:%d:%s:oos", s.ID, n.MedalID),
				Title: "勋章售罄：" + s.Name, Body: n.Name + " 已缺货", Level: "danger",
			}, true)
		}
	}
}
