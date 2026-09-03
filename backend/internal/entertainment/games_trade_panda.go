package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── PANDA 好友买卖（pandapt.net，friend-trade.php）────────────────
// 与猜大小同一 /ajax.php 家族：POST action=friendTrade*，信封 {ret,data,msg}（ret 0=ok）。
// 主要 action（页面 .post() 全量枚举）：
//
//	friendTradeInitOrHome（首页状态：me/my_owner/my_profile_price/claimable_income/my_assets）
//	friendTradeClaimIncome（领取收益）、friendTradeRedeem（赎身， params redeem_price）
//	friendTradeWork（安排工作）、friendTradeInteract（互动）、friendTradeProtect（保镖）
//	friendTradeRelease（释放奴隶）、friendTradeBuyCapacitySlot、friendTradeRedeem/AcceptSeedRelation
//	任务/成就/海报：ClaimTaskReward、AchievementClaim、LevelPosterClaim
//
// Play 只读 InitOrHome；花钱操作（Redeem/Work/Protect/Release）走 Bet 显式映射。
type PandaFriendTradeGame struct{}

func (PandaFriendTradeGame) ID() string   { return "panda_friendtrade" }
func (PandaFriendTradeGame) Name() string { return "好友买卖" }

// pandaTradeAjax 调 /ajax.php（form：action + params[…]），Referer 换 friend-trade.php。
func pandaTradeAjax(ctx context.Context, hc *httpclient.Client, baseURL, action, cookie string, params map[string]string) (map[string]any, error) {
	u := joinPath(baseURL, "ajax.php")
	form := "action=" + action
	for k, v := range params {
		form += "&params[" + k + "]=" + url.QueryEscape(v)
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "friend-trade.php"),
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("panda_friendtrade "+action, u, resp)
	return parseRaw(resp.Body)
}

// FriendTradeState 全景快照（InitOrHome data 直读，前端富组件渲染）。只读。
func (g PandaFriendTradeGame) FriendTradeState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*FriendTradeState, error) {
	m, err := pandaTradeAjax(ctx, hc, baseURL, "friendTradeInitOrHome", cr.Cookie, nil)
	if err != nil {
		return nil, err
	}
	d := pandaBSData(m)
	if d == nil || int(longptNum(m["ret"])) != 0 {
		return nil, fmt.Errorf("好友买卖状态获取失败：%s", firstNonEmpty(strField(m, "msg"), "可能未登录"))
	}
	st := &FriendTradeState{
		ProfilePrice:    pandaStr(d, "my_profile_price"),
		ClaimableIncome: pandaStr(d, "claimable_income"),
		AssetTotalValue: pandaStr(d, "asset_total_value"),
		Assets:          []FriendTradeSlave{},
	}
	if me, ok := d["me"].(map[string]any); ok {
		st.MeUID = pandaStr(me, "uid")
		st.MeName = pandaStr(me, "username")
	}
	if owner, ok := d["my_owner"].(map[string]any); ok && strField(owner, "username") != "" {
		st.OwnerName = strField(owner, "username")
		st.OwnerUID = pandaStr(owner, "uid")
		st.RedeemPrice = pandaStr(owner, "redeem_price")
		if rc, ok := owner["redeem_context"].(map[string]any); ok {
			st.CanRedeem = boolField(rc, "can_redeem")
		}
	}
	for _, a := range pandaBSArrAt(d, "my_assets") {
		am, ok := a.(map[string]any)
		if !ok {
			continue
		}
		sl := FriendTradeSlave{
			UID:      pandaStr(am, "slave_uid"),
			Username: pandaStr(am, "slave_username"),
			Avatar:   pandaStr(am, "slave_avatar"),
			Price:    pandaStr(am, "acquire_price", "price"),
		}
		if w, ok := am["today_work"].(map[string]any); ok {
			sl.WorkLabel = pandaStr(w, "work_label")
			sl.IncomeAmount = pandaStr(w, "income_amount")
			sl.HasWork = boolField(w, "has_work")
		}
		st.Assets = append(st.Assets, sl)
	}
	st.OwnedCount = len(st.Assets)
	if cs, ok := d["capacity_summary"].(map[string]any); ok {
		st.BaseCapacity = int(longptNum(cs["base_capacity"]))
		if st.OwnedCount == 0 {
			st.OwnedCount = int(longptNum(cs["owned_count"]))
		}
		st.NextSlotPrice = pandaStr(cs, "next_slot_price")
	}
	if is, ok := d["income_summary"].(map[string]any); ok {
		st.TodayEarned = pandaStr(is, "today_earned")
		st.TotalEarned = pandaStr(is, "total_earned")
	}
	for _, t := range pandaBSArrAt(d, "task_status") {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		st.Tasks = append(st.Tasks, FriendTradeTask{
			Key:          pandaStr(tm, "task_key"),
			Progress:     int(longptNum(tm["progress"])),
			Target:       int(longptNum(tm["target"])),
			Status:       pandaStr(tm, "status"),
			RewardAmount: pandaStr(tm, "reward_amount"),
		})
	}
	for _, f := range pandaBSArrAt(d, "activity_feed") {
		fm, ok := f.(map[string]any)
		if !ok {
			continue
		}
		st.Feed = append(st.Feed, FriendTradeFeed{
			Text:      pandaStr(fm, "relation_text"),
			CreatedAt: pandaStr(fm, "created_at"),
		})
	}
	return st, nil
}

// Play 只读首页状态：身价/主人/可领收益/奴隶数。不自动领取——收益领取走 Bet(claim)。
func (g PandaFriendTradeGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := pandaTradeAjax(ctx, hc, baseURL, "friendTradeInitOrHome", cr.Cookie, nil)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	d := pandaBSData(m)
	if d == nil || int(longptNum(m["ret"])) != 0 {
		return &PlayResult{Success: false,
			Message: firstNonEmpty(strField(m, "msg"), "状态获取失败（可能未登录）")}, nil
	}
	res := &PlayResult{Success: true, Raw: m}
	price := pandaStr(d, "my_profile_price")
	claimable := pandaStr(d, "claimable_income")
	var parts []string
	if price != "" {
		parts = append(parts, fmt.Sprintf("身价 %s", price))
	}
	if claimable != "" {
		parts = append(parts, fmt.Sprintf("可领收益 %s", claimable))
	}
	if n := len(pandaBSArrAt(d, "my_assets")); n > 0 {
		parts = append(parts, fmt.Sprintf("持有奴隶 %d", n))
	}
	if owner, ok := d["my_owner"].(map[string]any); ok && strField(owner, "username") != "" {
		parts = append(parts, fmt.Sprintf("主人 %s（赎身 %s）",
			strField(owner, "username"), pandaStr(owner, "redeem_price")))
	} else {
		parts = append(parts, "自由身")
	}
	res.Message = strings.Join(parts, "，")
	return res, nil
}

func pandaBSArrAt(m map[string]any, key string) []any {
	if a, ok := m[key].([]any); ok {
		return a
	}
	return nil
}

// pandaTradeHomeRe 页面兜底：InitOrHome 失败时从 friend-trade.php 内嵌 bootstrap JSON 提取。
var pandaTradeHomeRe = regexp.MustCompile(`normalizeFriendTradeHome\((\{.*?\})\);?\s*new Vue`)

func pandaTradeHomeFromPage(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (map[string]any, error) {
	u := joinPath(baseURL, "friend-trade.php")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, nil, true)
	if err != nil {
		return nil, err
	}
	m := pandaTradeHomeRe.FindSubmatch(resp.Body)
	if m == nil {
		return nil, fmt.Errorf("页面无 bootstrap 数据")
	}
	var d map[string]any
	if err := json.Unmarshal(m[1], &d); err != nil {
		return nil, err
	}
	return d, nil
}

// Stats 余额尽力取（InitOrHome data 里无 bonus 字段时留空）。
func (g PandaFriendTradeGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	d, err := pandaTradeHomeFromPage(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &StatsResult{}
	if me, ok := d["me"].(map[string]any); ok {
		st.Balance = firstNonZero(numField(me, "bonus"), numField(me, "seedbonus"))
	}
	return st, nil
}

// Bet 显式动作映射：Selection=claim（领收益）/ redeem（赎身）/ protect（护主，Extra.target_uid）。
// 买人/工作/互动涉及目标选择与策略，不在自动适配范围。
func (g PandaFriendTradeGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	action, params := "", map[string]string{}
	switch strings.TrimSpace(req.Selection) {
	case "claim":
		action = "friendTradeClaimIncome"
	case "redeem":
		action = "friendTradeRedeem"
	case "protect":
		if req.Extra["target_uid"] == "" {
			return &PlayResult{Success: false, Message: "protect 需 Extra.target_uid"}, nil
		}
		action = "friendTradeProtect"
		params["target_uid"] = req.Extra["target_uid"]
	default:
		return &PlayResult{Success: false, Message: "支持动作：claim（领收益）/ redeem（赎身）/ protect（护主）"}, nil
	}
	m, err := pandaTradeAjax(ctx, hc, baseURL, action, cr.Cookie, params)
	if err != nil {
		return nil, err
	}
	ok := int(longptNum(m["ret"])) == 0
	msg := firstNonEmpty(strField(m, "msg"), strField(m, "message"))
	if !ok && msg == "" {
		msg = "操作失败"
	}
	if ok && msg == "" {
		msg = "操作成功"
	}
	return &PlayResult{Success: ok, Message: msg, Raw: m}, nil
}
