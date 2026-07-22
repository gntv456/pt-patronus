package medalwall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Unit3DAdapter Unit3D（Laravel）勋章墙：JSON API 优先，HTML（Bootstrap 卡）回退。货币恒 BP。
// Buy 试 /api/bonus/buy/{id}；Unit3D 通常无佩戴概念，Wear/TakeOff 返回不支持。
type Unit3DAdapter struct{}

func (Unit3DAdapter) Platform() string { return "unit3d" }
func (Unit3DAdapter) Supported() bool  { return true }

// FetchShop JSON API 候选端点优先，无果 HTML 卡片回退。
func (Unit3DAdapter) FetchShop(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie string) ([]Item, error) {
	var lastErr error
	for _, ep := range []string{"api/bonus/items", "api/store/items", "api/medals", "api/bonus/transactions"} {
		u := joinBase(baseURL, ep)
		resp, err := hc.Get(ctx, u, cookie, true)
		if err != nil {
			lastErr = &ShopFetchError{Kind: "request", URL: u, Err: err}
			continue
		}
		if resp == nil {
			lastErr = &ShopFetchError{Kind: "request", URL: u}
			continue
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 || isLoginURL(resp.FinalURL) {
			lastErr = &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = &ShopFetchError{Kind: "response", URL: u, StatusCode: resp.StatusCode}
			continue
		}
		if items := parseUnit3DJSON(resp.Body, baseURL); len(items) > 0 {
			return items, nil
		}
		lastErr = &ShopFetchError{Kind: "parse", URL: u, StatusCode: resp.StatusCode, Err: ErrNoMedals}
	}
	for _, ep := range []string{"bonus", "store", "shop", "medals", "bonus/store", "bonus/shop"} {
		u := joinBase(baseURL, ep)
		resp, err := hc.Get(ctx, u, cookie, true)
		if err != nil {
			lastErr = &ShopFetchError{Kind: "request", URL: u, Err: err}
			continue
		}
		if resp == nil {
			lastErr = &ShopFetchError{Kind: "request", URL: u}
			continue
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 || isLoginURL(resp.FinalURL) {
			lastErr = &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = &ShopFetchError{Kind: "response", URL: u, StatusCode: resp.StatusCode}
			continue
		}
		if items := parseUnit3DHTML(string(resp.Body), baseURL); len(items) > 0 {
			return items, nil
		}
		lastErr = &ShopFetchError{Kind: "parse", URL: u, StatusCode: resp.StatusCode, Err: ErrNoMedals}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNoMedals
}

// FetchOwned Unit3D 已拥有勋章：从用户资料页 /users/{name} 的勋章区解析（best-effort）。
// uid 这里通常是用户名（Unit3D 走 /users/{name}，非数字 id）。
func (Unit3DAdapter) FetchOwned(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, uid string) ([]Owned, error) {
	if uid == "" {
		return nil, nil
	}
	resp, err := hc.Get(ctx, joinBase(baseURL, fmt.Sprintf("users/%s", uid)), cookie, true)
	if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}
	return parseUnit3DOwned(string(resp.Body), baseURL), nil
}

// Buy 试 POST /api/bonus/buy/{id}（无果回退 /bonus/buy/{id}），表单 best-effort。
func (Unit3DAdapter) Buy(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error) {
	if medalID == "" {
		return ActionResult{Success: false, Message: "缺少勋章 ID"}, nil
	}
	referer := joinBase(baseURL, "bonus")
	for _, ep := range []string{fmt.Sprintf("api/bonus/buy/%s", medalID), fmt.Sprintf("bonus/buy/%s", medalID)} {
		resp, err := hc.Do(ctx, "POST", joinBase(baseURL, ep), nil, ajaxHeaders(referer), cookie, true)
		if err != nil || resp == nil {
			continue
		}
		if ok, msg := parseActionOK(resp.Body); ok {
			return ActionResult{Success: true, Message: firstNonEmpty(msg, "购买成功")}, nil
		}
	}
	return ActionResult{Success: false, Message: "购买失败"}, nil
}

// Wear/TakeOff Unit3D 通常无佩戴概念。
func (Unit3DAdapter) Wear(context.Context, HTTPDoer, Def, string, string, string) (ActionResult, error) {
	return ActionResult{Success: false, Message: "Unit3D 暂不支持勋章佩戴"}, nil
}
func (Unit3DAdapter) TakeOff(context.Context, HTTPDoer, Def, string, string, string) (ActionResult, error) {
	return ActionResult{Success: false, Message: "Unit3D 暂不支持勋章摘下"}, nil
}

// parseUnit3DJSON best-effort 解析 JSON：递归找含 name+cost 的对象，抽 id/image/stock。货币 BP。
func parseUnit3DJSON(body []byte, baseURL string) []Item {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil
	}
	var out []Item
	walkJSON(root, func(o map[string]any) {
		name := jsonStr(o, "name", "title", "label")
		cost := jsonNum(o, "cost", "price", "bp", "bonus", "amount")
		if name == "" || cost == 0 {
			return // 无名/无价格 → 非勋章条目
		}
		out = append(out, Item{
			MedalID:        jsonStr(o, "id", "medal_id", "item_id", "uuid"),
			Name:           name,
			Description:    jsonStr(o, "description", "desc"),
			ImageSmall:     resolveAsset(baseURL, jsonStr(o, "image", "icon", "icon_url", "image_url")),
			Price:          cost,
			Currency:       "BP",
			Stock:          int(jsonNum(o, "stock", "remaining", "quantity")),
			PurchaseStatus: StatusAvailable,
		})
	})
	return out
}

// walkJSON 递归遍历，对每个 map[string]any 对象调 fn。
func walkJSON(v any, fn func(map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		fn(t)
		for _, sub := range t {
			walkJSON(sub, fn)
		}
	case []any:
		for _, sub := range t {
			walkJSON(sub, fn)
		}
	}
}

func jsonStr(o map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := o[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
			if n, ok := v.(float64); ok {
				return fmt.Sprintf("%v", n)
			}
		}
	}
	return ""
}

func jsonNum(o map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := o[k]; ok {
			if n, ok := v.(float64); ok {
				return n
			}
		}
	}
	return 0
}

// parseUnit3DHTML Bootstrap 卡片解析（.bonus-card/.store-item/.shop-card/.card-body）。
func parseUnit3DHTML(html, baseURL string) []Item {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return nil
	}
	var out []Item
	doc.Find(".bonus-card, .store-item, .shop-card, .card-body, article.card").Each(func(_ int, s *goquery.Selection) {
		name := strings.TrimSpace(s.Find(".card-title, .item-name, .store-name, h3, h4, .title").First().Text())
		price := parsePrice(s.Find(".price, .cost, .bp, .bonus-cost").First().Text())
		if name == "" || price == 0 {
			return
		}
		stock := -1
		if stText := s.Find(".stock, .remaining").First().Text(); stText != "" {
			stock = parseStock(stText)
		}
		out = append(out, Item{
			MedalID:        s.AttrOr("data-id", ""),
			Name:           name,
			ImageSmall:     resolveImg(s.Find("img"), baseURL),
			Price:          price,
			Currency:       "BP",
			Stock:          stock,
			PurchaseStatus: StatusAvailable,
		})
	})
	return out
}

// parseUnit3DOwned 用户资料页勋章区解析（.user-medal/.achievement/img[class*=medal]）。
func parseUnit3DOwned(html, baseURL string) []Owned {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return nil
	}
	var out []Owned
	seen := map[string]bool{}
	doc.Find(".user-medal, .achievement, .badge-medal, img[class*='medal']").Each(func(_ int, s *goquery.Selection) {
		name := strings.TrimSpace(firstNonEmpty(s.AttrOr("title", ""), s.AttrOr("alt", ""), s.Find(".name, .title").First().Text()))
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		// 图片：s 可能是 div（内含 img）或 img 本身
		imgURL := resolveImg(s.Find("img"), baseURL)
		if imgURL == "" {
			if src, ok := s.Attr("src"); ok && src != "" {
				imgURL = resolveAsset(baseURL, src)
			}
		}
		out = append(out, Owned{Name: name, Image: imgURL})
	})
	return out
}
