package medalwall

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

func fetchSiQiShop(ctx context.Context, hc HTTPDoer, baseURL, cookie, path string) ([]Item, error) {
	u := joinBase(baseURL, path)
	resp, err := hc.Get(ctx, u, cookie, true)
	if err != nil {
		return nil, &ShopFetchError{Kind: "request", URL: u, Err: err}
	}
	if resp == nil {
		return nil, &ShopFetchError{Kind: "request", URL: u}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == 401 || resp.StatusCode == 403 || isLoginURL(resp.FinalURL) {
			return nil, &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
		}
		return nil, &ShopFetchError{Kind: "response", URL: u, StatusCode: resp.StatusCode}
	}
	if isLoginURL(resp.FinalURL) {
		return nil, &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
	}
	items := parseShopHTML(string(resp.Body), baseURL)
	if len(items) == 0 {
		return nil, &ShopFetchError{Kind: "parse", URL: u, StatusCode: resp.StatusCode, Err: ErrNoMedals}
	}
	return items, nil
}

// fetchSynthesisMedals 抓 LongPT 六十四卦合成勋章（pluginsMedalInfo/getList，POST {medalUnit:3}）。
// 仅 LongPT 有此插件 API（host 含 longpt），其他站直接跳过不发请求。
// 已合成的（owned=true）标 OWNED，其余 SYNTHESIS（仅合成）；medalId 用 syn: 前缀避与商店 id 冲突。

func fetchSynthesisMedals(ctx context.Context, hc HTTPDoer, baseURL, cookie string) []Item {
	if !strings.Contains(strings.ToLower(baseURL), "longpt") {
		return nil
	}
	u := joinBase(baseURL, "pt-api/v1/plugins/medals/pluginsMedalInfo/getList")
	resp, err := hc.Do(ctx, "POST", u, []byte(`{"medalUnit":"3"}`),
		map[string]string{"Content-Type": "application/json", "X-Requested-With": "XMLHttpRequest"}, cookie, true)
	if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var env struct {
		Code int              `json:"code"`
		Data []map[string]any `json:"data"`
	}
	if json.Unmarshal(resp.Body, &env) != nil || env.Code != 0 {
		return nil
	}
	out := make([]Item, 0, len(env.Data))
	for _, m := range env.Data {
		name, _ := m["medalName"].(string)
		if name == "" {
			continue
		}
		img, _ := m["medalImageUrl"].(string)
		bonus := ""
		if bf, ok := m["bonusAdditionFactor"]; ok && bf != nil {
			bonus = fmt.Sprintf("%v", bf)
		}
		st := StatusSynthesis
		if owned, _ := m["owned"].(bool); owned {
			st = StatusOwned
		}
		out = append(out, Item{
			MedalID:          "syn:" + fmt.Sprintf("%v", m["medalRealId"]),
			Name:             name,
			ImageSmall:       resolveAsset(baseURL, img),
			OriginalImageURL: resolveAsset(baseURL, img),
			BonusRate:        bonus,
			Currency:         "魔力",
			PurchaseStatus:   st,
		})
	}
	return out
}

// fetchShopPaged 通用分页抓取：先 GET {baseURL}/{path} 首页，再以 ?{param}={N} 翻页，含 4 重防伪分页
// （内容指纹去重 / 整页皆已见即停 / 行数骤降 <40% / 20 页上限）。NexusPHP(page) 与 TNode(p) 共用。

func fetchShopPaged(ctx context.Context, hc HTTPDoer, baseURL, cookie, path, param string) ([]Item, error) {
	shop := joinBase(baseURL, path)
	var all []Item
	seenFp := map[string]bool{}
	firstPageCount := 0
	for page := 0; page < shopMaxPages; page++ {
		u := shop
		if page > 0 {
			u = appendParam(shop, fmt.Sprintf("%s=%d", param, page))
		}
		resp, err := hc.Get(ctx, u, cookie, true)
		if resp != nil && isLoginURL(resp.FinalURL) && len(all) == 0 {
			return nil, &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
		}
		if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 || len(resp.Body) < 200 {
			// 尚未抓到任何数据时才把失败上抛；翻页越界则静默停止。
			if len(all) == 0 {
				if err != nil {
					slog.Warn("medal shop fetch failed", "url", u, "err", err)
					return nil, &ShopFetchError{Kind: "request", URL: u, Err: err}
				} else if resp == nil {
					return nil, &ShopFetchError{Kind: "request", URL: u}
				} else if resp != nil {
					slog.Warn("medal shop fetch non-2xx/empty", "url", u, "status", resp.StatusCode, "bytes", len(resp.Body))
					if resp.StatusCode == 401 || resp.StatusCode == 403 || isLoginURL(resp.FinalURL) {
						return nil, &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
					}
					if resp.StatusCode < 200 || resp.StatusCode >= 300 {
						return nil, &ShopFetchError{Kind: "response", URL: u, StatusCode: resp.StatusCode}
					}
					if isLoginURL(resp.FinalURL) {
						return nil, &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
					}
					return nil, &ShopFetchError{Kind: "parse", URL: u, StatusCode: resp.StatusCode}
				}
			}
			break
		}
		items := parseShopHTML(string(resp.Body), baseURL)
		if len(items) == 0 {
			// 少数站点只在 ?page=1 返回首页，允许无参数页解析为空后再试一次。
			if page == 0 {
				continue
			}
			if len(all) == 0 {
				slog.Warn("medal shop parse empty", "url", u, "bytes", len(resp.Body))
				return nil, &ShopFetchError{Kind: "parse", URL: u, StatusCode: resp.StatusCode, Err: ErrNoMedals}
			}
			break
		}
		if firstPageCount == 0 {
			firstPageCount = len(items)
		}
		// 内容指纹：整页全是已见条目 → 翻页失效
		dup := 0
		var fps []string
		for _, it := range items {
			fp := medalFingerprint(it)
			fps = append(fps, fp)
			if seenFp[fp] {
				dup++
			}
		}
		if dup == len(items) {
			break
		}
		for _, fp := range fps {
			seenFp[fp] = true
		}
		// 行数骤降 < 首页 40% → 越界
		if len(all) > 0 && firstPageCount > 0 && float64(len(items)) < float64(firstPageCount)*0.4 {
			break
		}
		all = append(all, items...)
	}
	return all, nil
}

func isLoginURL(rawURL string) bool {
	u := strings.ToLower(rawURL)
	return strings.Contains(u, "login.php") || strings.Contains(u, "takelogin.php") || strings.Contains(u, "/login")
}

// FetchOwned 抓已拥有勋章；uid 为空返回 nil（chain 层负责发现 uid）。
