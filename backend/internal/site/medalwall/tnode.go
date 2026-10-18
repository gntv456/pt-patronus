package medalwall

import (
	"context"
	"fmt"
)

// TNodeAdapter NexusPHP 二次开发版（TNode）勋章墙：medals.php?p={N} 卡片链，
// 无果回退 medal.php 表格（复用 parseShopHTML）；已拥有走 medals.php?action=owned。
// Buy/Wear/TakeOff 端点契约与 NexusPHP 相同（medal.php?action=），委托 NexusPHPAdapter。
type TNodeAdapter struct{}

func (TNodeAdapter) Platform() string { return "tnode" }
func (TNodeAdapter) Supported() bool  { return true }

// tNodeNexus 复用 NexusPHP 的写操作端点逻辑。
var tNodeNexus = NexusPHPAdapter{}

// FetchShop 端点链 medals.php?p → medal.php?p → medal.php?page（首个产出非空即返回）。
func (TNodeAdapter) FetchShop(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie string) ([]Item, error) {
	endpoints := []struct{ path, param string }{
		{"medals.php", "p"},
		{"medal.php", "p"},
		{"medal.php", "page"},
	}
	var lastErr error
	for _, ep := range endpoints {
		items, err := fetchShopPaged(ctx, hc, baseURL, cookie, ep.path, ep.param)
		if len(items) > 0 {
			return items, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNoMedals
}

// FetchOwned 已拥有：medals.php?action=owned（回退 medal.php?action=owned），复用 parseOwnedHTML。
// 全部端点均未拿到有效响应 → 返回错误（chain 借此保留旧 OWNED 快照）；拿到 2xx 但解析为空 →
// 视为「确实没有已拥有勋章」的正常空结果。
func (TNodeAdapter) FetchOwned(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, uid string) ([]Owned, error) {
	if uid == "" {
		return nil, ErrOwnedUnavailable
	}
	var lastErr error
	gotResponse := false
	for _, ep := range []string{"medals.php", "medal.php"} {
		u := fmt.Sprintf("%s?action=owned", joinBase(baseURL, ep))
		resp, err := hc.Get(ctx, u, cookie, true)
		if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = ownedFetchErr(err, resp)
			continue
		}
		gotResponse = true
		if owned := parseOwnedHTML(string(resp.Body), baseURL, uid); len(owned) > 0 {
			return owned, nil
		}
	}
	if !gotResponse {
		return nil, lastErr
	}
	return nil, nil
}

// Buy/Wear/TakeOff 复用 NexusPHP 端点契约。
func (TNodeAdapter) Buy(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error) {
	return tNodeNexus.Buy(ctx, hc, def, baseURL, cookie, medalID)
}
func (TNodeAdapter) Wear(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error) {
	return tNodeNexus.Wear(ctx, hc, def, baseURL, cookie, medalID)
}
func (TNodeAdapter) TakeOff(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error) {
	return tNodeNexus.TakeOff(ctx, hc, def, baseURL, cookie, medalID)
}
