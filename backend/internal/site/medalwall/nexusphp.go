package medalwall

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/PuerkitoBio/goquery"
)
// NexusPHPAdapter NexusPHP 系勋章墙：GET medal.php?page={N} 分页解析（卡片/表格双布局）+
// GET userdetails.php?id=<uid> 抓已拥有；购买/佩戴/摘下走 AJAX POST（多端点回退）。
type NexusPHPAdapter struct{}

func (NexusPHPAdapter) Platform() string { return "nexusphp" }

func (NexusPHPAdapter) Supported() bool  { return true }

// 分页上限 + 关键词门槛（整页须含勋章关键词，防误识非勋章页）。
const (
	shopMaxPages = 20
)

var shopKeywordRe = regexp.MustCompile(`加成|火花|佩戴|購買|购买|庫存|库存|價格|价格|魔力|電力|金币|金幣|勋章|勛章|有效期`)

// FetchShop 分页抓商店勋章（medal.php?page=N），含防伪分页（委托 fetchShopPaged）。
// 末尾追加 LongPT 六十四卦合成勋章（pluginsMedalInfo/getList，仅 LongPT 有，其他站跳过）。
func (NexusPHPAdapter) FetchShop(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie string) ([]Item, error) {
	path := def.ShopPath
	if path == "" {
		path = "medal.php"
	}
	if isSiQiSite(baseURL) {
		return fetchSiQiShop(ctx, hc, baseURL, cookie, path)
	}
	items, err := fetchShopPaged(ctx, hc, baseURL, cookie, path, "page")
	items = append(items, fetchSynthesisMedals(ctx, hc, baseURL, cookie)...)
	if len(items) > 0 {
		return items, nil
	}
	return nil, err
}

// FetchOwned 抓已拥有勋章；uid 为空返回 nil（chain 层负责发现 uid）。
func (NexusPHPAdapter) FetchOwned(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, uid string) ([]Owned, error) {
	// 思齐的 userdetails.php 是一个全站聚合页：其中的
	// img.nexus-username-medal 属于排行榜里的其他用户和玩偶，不能用来判断当前账号。
	// medal.php 的卡片才带有当前账号的 owned 状态；该页面不需要 uid。
	if isSiQiSite(baseURL) {
		resp, err := hc.Get(ctx, shopPath(def, baseURL), cookie, true)
		if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, nil
		}
		return parseSiQiOwnedHTML(string(resp.Body), baseURL), nil
	}
	if uid == "" {
		return nil, nil
	}
	u := fmt.Sprintf("%s?id=%s", ownPath(def, baseURL), uid)
	resp, err := hc.Get(ctx, u, cookie, true)
	if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}
	return parseOwnedHTML(string(resp.Body), baseURL, uid), nil
}

// Buy 购买：rousipro/通用走 ajax.php?action=buyMedal，否则回退 3 个 medal.php?action=buy 端点。
// 依序尝试，首个确认成功即返回（对齐旧版 4 端点策略）。
func (NexusPHPAdapter) Buy(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error) {
	if medalID == "" {
		return ActionResult{Success: false, Message: "缺少勋章 ID"}, nil
	}
	shop := shopPath(def, baseURL)
	hdrs := ajaxHeaders(shop)
	// 端点 1：ajax.php action=buyMedal
	form := url.Values{}
	form.Set("action", "buyMedal")
	form.Set("params[medal_id]", medalID)
	if r, err := hc.Do(ctx, "POST", joinBase(baseURL, "ajax.php"), []byte(form.Encode()), hdrs, cookie, true); err == nil && r != nil {
		if ok, msg := parseActionOK(r.Body); ok {
			return ActionResult{Success: true, Message: firstNonEmpty(msg, "购买成功")}, nil
		}
	}
	// 端点 2-4：medal.php?action=buy&id= / &medalid= / &medal_id=
	// body 同时带 action/id（Audiences medal_center.php 走 form hidden，后端读 $_POST 而非 query）。
	for _, key := range []string{"id", "medalid", "medal_id"} {
		u := appendParam(shop, fmt.Sprintf("action=buy&%s=%s", key, medalID))
		body := fmt.Sprintf("buy=1&action=buy&%s=%s", key, medalID)
		if r, err := hc.Do(ctx, "POST", u, []byte(body), hdrs, cookie, true); err == nil && r != nil {
			if ok, msg := parseActionOK(r.Body); ok {
				return ActionResult{Success: true, Message: firstNonEmpty(msg, "购买成功")}, nil
			}
		}
	}
	return ActionResult{Success: false, Message: "购买失败（站点未确认成功）"}, nil
}

// Wear 佩戴：POST medal.php?action=wear&id= body wear=1，HTTP 2xx 判定。
func (NexusPHPAdapter) Wear(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error) {
	return nexusFormAction(ctx, hc, shopPath(def, baseURL), cookie, medalID, "wear", "wear")
}

// TakeOff 摘下：POST medal.php?action=takeoff&id= body takeoff=1。
func (NexusPHPAdapter) TakeOff(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error) {
	return nexusFormAction(ctx, hc, shopPath(def, baseURL), cookie, medalID, "takeoff", "takeoff")
}

// nexusFormAction 佩戴/摘下通用：2xx 视为成功。
func nexusFormAction(ctx context.Context, hc HTTPDoer, shop, cookie, medalID, action, bodyKey string) (ActionResult, error) {
	u := appendParam(shop, fmt.Sprintf("action=%s&id=%s", action, medalID))
	r, err := hc.Do(ctx, "POST", u, []byte(bodyKey+"=1"), ajaxHeaders(shop), cookie, true)
	if err != nil {
		return ActionResult{Success: false, Message: "请求失败：" + err.Error()}, nil
	}
	if r != nil && r.StatusCode >= 200 && r.StatusCode < 300 {
		return ActionResult{Success: true, Message: "操作已提交"}, nil
	}
	return ActionResult{Success: false, Message: "操作失败"}, nil
}

// ── 商店页解析（卡片 + 表格双布局）──

// parseShopHTML 入口：关键词门槛 → goquery → parseShopDoc。
func parseShopHTML(html, baseURL string) []Item {
	if !shopKeywordRe.MatchString(html) {
		return nil // 非勋章页，防误识
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return nil
	}
	if isSiQiSite(baseURL) {
		return parseSiQiShopDoc(doc, baseURL)
	}
	return parseShopDoc(doc, baseURL)
}
