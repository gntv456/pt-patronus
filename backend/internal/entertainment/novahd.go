package entertainment

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// NovaHD（pt.novahd.top）娱乐：魔力值刮刮乐 / 幸运抽奖；魔力农场为通用插件（好学 hxpt + NovaHD 共用同一套 magic_fram.php，见 MagicFarmGame）。
//
// 契约来自站点源码备份，均纯 Cookie 鉴权（NexusPHP loggedinorreturn），货币 = users.seedbonus
// （好学叫“火花”、NovaHD 叫“魔力值”，余额抠 .points-display 内数字，不依赖货币名文本）：
//   - 魔力农场 magic_fram.php（通用，好学+NovaHD 同套插件）：GET + 整页 HTML（无 JSON API），操作 ?action=plant|breed|harvest|harvest_all|sell。
//   - 魔力值刮刮乐 magic_scratch.php：buy(POST→HTML 抠 scratch_id) → scratch(POST→JSON) → settle(GET)。
//   - 幸运抽奖 fortune-wheel-spin.php：POST count=1|10|20|50 → JSON {success, results:[{status,type,value,unit}]}。
//
// 参考 hxpt.go（NexusPHP cookie + PostFormAjax/GetWithHeaders + HTML 抠取）、longpt.go（抽奖聚合）。

// novaHDNum 解析可能带千分位的数值串（去逗号）。
func novaHDNum(s string) float64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0
	}
	var f float64
	_, _ = fmt.Sscanf(s, "%f", &f)
	return f
}

// ── 魔力农场 ────────────────────────────────────────────────

// magicFarmItemDef 农场单项基础配置（name/grow/price 两站一致；expire 从页面 section 抠，icon 好学/NovaHD 不同但前端用 emoji 不依赖）。
type magicFarmItemDef struct {
	ID        int
	Name      string
	Icon      string
	GrowHours int
	Price     float64
}

var magicFarmCrops = []magicFarmItemDef{
	{1, "小麦", "pic/farm/crop_wheat4.png", 4, 500},
	{2, "玉米", "pic/farm/crop_corn4.png", 8, 1000},
	{3, "花生", "pic/farm/crop_peanut4.png", 12, 1500},
	{4, "土豆", "pic/farm/crop_potato1.png", 24, 2000},
}
var magicFarmAnimals = []magicFarmItemDef{
	{1, "鸡", "pic/farm/animal_chicken1.png", 24, 1000},
	{2, "猪", "pic/farm/animal_pig1.png", 48, 2000},
	{3, "羊", "pic/farm/animal_sheep1.png", 72, 5000},
	{4, "牛", "pic/farm/animal_cow2.png", 96, 10000},
}

// MagicFarmGame 魔力农场。
type MagicFarmGame struct{}

func (MagicFarmGame) ID() string   { return "magic_farm" }
func (MagicFarmGame) Name() string { return "魔力农场" }

// magicFarmGet GET magic_fram.php(?action=query)，返回 HTML body。query 形如 "plant&type=crop&id=1" / "harvest_all" / "sell&key=xxx"。
func magicFarmGet(ctx context.Context, hc *httpclient.Client, baseURL, query, cookie string) ([]byte, error) {
	u := joinPath(baseURL, "magic_fram.php")
	if query != "" {
		u += "?action=" + query
	}
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{"Referer": joinPath(baseURL, "magic_fram.php")}, true)
	if err != nil {
		return nil, err
	}
	logResp("magic_farm "+query, u, resp)
	return resp.Body, nil
}

// magicFarmBalance 抠页面余额（.points-display 内数字，如「当前魔力值: 18,899.0」）。
func magicFarmBalance(body []byte) float64 {
	pts := firstGroup(body, `(?s)points-display[^>]*>(.*?)</div>`)
	return novaHDNum(firstGroup([]byte(pts), `([\d,.]+)`))
}

// magicFarmMsg 抠操作提示（<div class="message">$msg</div>）；无提示返回空。
func magicFarmMsg(body []byte) string {
	m := firstGroup(body, `(?s)<div class="message"[^>]*>(.*?)</div>`)
	return strings.TrimSpace(stripTags(m))
}

// magicFarmStatus 判断单项状态：页面含 harvest 链接=ready，含 plant/breed 链接=idle，否则 growing。
func magicFarmStatus(body []byte, section string, id int) string {
	s := string(body)
	if strings.Contains(s, fmt.Sprintf("action=harvest&type=%s&id=%d", section, id)) {
		return "ready"
	}
	plantAct := "plant"
	if section == "animal" {
		plantAct = "breed"
	}
	if strings.Contains(s, fmt.Sprintf("action=%s&type=%s&id=%d", plantAct, section, id)) {
		return "idle"
	}
	return "growing"
}

// magicFarmWarehouse 抠仓库表行（名称/数量/出售 key）。
func magicFarmWarehouse(body []byte) []StorageItem {
	re := regexp.MustCompile(`(?s)<tr>\s*<td>.*?</td>\s*<td>([^<]+)</td>\s*<td>(\d+)</td>.*?action=sell&key=([^"&']+?)["&']`)
	out := make([]StorageItem, 0)
	for _, m := range re.FindAllStringSubmatch(string(body), -1) {
		count, _ := strconv.Atoi(strings.TrimSpace(m[2]))
		out = append(out, StorageItem{Key: m[3], Name: strings.TrimSpace(m[1]), Count: count})
	}
	return out
}

// magicFarmMarket 抠菜市场两张表（第一张 crops，其后 animals）。
func magicFarmMarket(body []byte) []MarketItem {
	catRe := regexp.MustCompile(`(?s)market-category.*?<table[^>]*>(.*?)</table>`)
	rowRe := regexp.MustCompile(`(?s)<tr>\s*<td>.*?</td>\s*<td>([^<]+)</td>\s*<td>([\d,.]+)</td>`)
	out := make([]MarketItem, 0)
	cats := catRe.FindAllStringSubmatch(string(body), -1)
	for i, cat := range cats {
		section := "crop"
		if i > 0 {
			section = "animal"
		}
		for _, r := range rowRe.FindAllStringSubmatch(cat[1], -1) {
			out = append(out, MarketItem{Name: strings.TrimSpace(r[1]), Section: section, MarketPrice: novaHDNum(r[2])})
		}
	}
	return out
}

// magicFarmExpireDays 抠页面 section 标题的有效期天数（「农作物有效期N天」/「动物有效期N天」，好学 5 / NovaHD 农作物 2）。未匹配用默认。
func magicFarmExpireDays(body []byte, section string) int {
	label := "农作物有效期"
	if section == "animal" {
		label = "动物有效期"
	}
	if m := regexp.MustCompile(label + `\s*(\d+)\s*天`).FindSubmatch(body); len(m) >= 2 {
		n, _ := strconv.Atoi(string(m[1]))
		return n
	}
	if section == "crop" {
		return 2
	}
	return 5
}

// magicParseFarm 解析农场全景 HTML → FarmState。
func magicParseFarm(body []byte) *FarmState {
	cropExpire := magicFarmExpireDays(body, "crop")
	animalExpire := magicFarmExpireDays(body, "animal")
	st := &FarmState{
		Balance: magicFarmBalance(body),
		Crops:   make([]FarmItem, 0, len(magicFarmCrops)),
		Animals: make([]FarmItem, 0, len(magicFarmAnimals)),
	}
	for _, c := range magicFarmCrops {
		st.Crops = append(st.Crops, FarmItem{
			ID: c.ID, Name: c.Name, Icon: c.Icon, Section: "crop",
			GrowHours: c.GrowHours, Price: c.Price, ExpireDays: cropExpire,
			Status: magicFarmStatus(body, "crop", c.ID),
		})
	}
	for _, a := range magicFarmAnimals {
		st.Animals = append(st.Animals, FarmItem{
			ID: a.ID, Name: a.Name, Icon: a.Icon, Section: "animal",
			GrowHours: a.GrowHours, Price: a.Price, ExpireDays: animalExpire,
			Status: magicFarmStatus(body, "animal", a.ID),
		})
	}
	st.Warehouse = magicFarmWarehouse(body)
	st.Market = magicFarmMarket(body)
	return st
}

// Farm 农场全景。
func (MagicFarmGame) Farm(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*FarmState, error) {
	body, err := magicFarmGet(ctx, hc, baseURL, "", cr.Cookie)
	if err != nil {
		return nil, err
	}
	return magicParseFarm(body), nil
}

// magicFarmDo 执行种植/收获类操作（plant|breed|harvest），返回刷新页面的提示与余额。
func magicFarmDo(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, section string, itemID int, op string) (*PlayResult, error) {
	action := op
	if op == "plant" && section == "animal" {
		action = "breed"
	}
	body, err := magicFarmGet(ctx, hc, baseURL, fmt.Sprintf("%s&type=%s&id=%d", action, section, itemID), cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &PlayResult{Success: true, Message: magicFarmMsg(body), Balance: magicFarmBalance(body)}, nil
}

// Plant 种植农作物 / 养动物（section=animal 时走 breed）。
func (MagicFarmGame) Plant(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, section string, itemID int) (*PlayResult, error) {
	return magicFarmDo(ctx, hc, baseURL, cr, section, itemID, "plant")
}

// Harvest 收获单个。
func (MagicFarmGame) Harvest(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, section string, itemID int) (*PlayResult, error) {
	return magicFarmDo(ctx, hc, baseURL, cr, section, itemID, "harvest")
}

// HarvestAll 一键收获。
func (MagicFarmGame) HarvestAll(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	body, err := magicFarmGet(ctx, hc, baseURL, "harvest_all", cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &PlayResult{Success: true, Message: magicFarmMsg(body), Balance: magicFarmBalance(body)}, nil
}

// Sell 出售仓库物品（按市场价）。
func (MagicFarmGame) Sell(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, key string) (*PlayResult, error) {
	body, err := magicFarmGet(ctx, hc, baseURL, "sell&key="+key, cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &PlayResult{Success: true, Message: magicFarmMsg(body), Balance: magicFarmBalance(body)}, nil
}

// Play（LotteryGame）= 一键收获。
func (MagicFarmGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return MagicFarmGame{}.HarvestAll(ctx, hc, baseURL, cr)
}

// Stats 余额。
func (MagicFarmGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := magicFarmGet(ctx, hc, baseURL, "", cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: magicFarmBalance(body)}, nil
}

// ── 魔力值刮刮乐 ────────────────────────────────────────────

// NovaHDScratchGame 魔力值刮刮乐：buy(POST→HTML 抠 scratch_id) → scratch(POST→JSON) → settle。
// 源码配置 cost=5000, daily_limit=5, 6 档奖品。
type NovaHDScratchGame struct{}

func (NovaHDScratchGame) ID() string   { return "novahd_scratch" }
func (NovaHDScratchGame) Name() string { return "魔力值刮刮乐" }

func (NovaHDScratchGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	endpoint := joinPath(baseURL, "magic_scratch.php")
	referer := endpoint

	// 1. 购买资格：POST 返 HTML，内嵌 currentScratchId=N。
	buyResp, err := hc.PostFormAjax(ctx, endpoint+"?action=buy", cr.Cookie, "", referer, true)
	if err != nil {
		return nil, err
	}
	logResp("novahd_scratch buy", endpoint, buyResp)
	scratchID := firstGroup(buyResp.Body, `currentScratchId\s*=\s*(\d+)`)
	if scratchID == "" {
		scratchID = firstGroup(buyResp.Body, `scratch_id[=:" ]+(\d+)`)
	}
	if scratchID == "" {
		// 买卡失败（余额不足/达上限）：抠提示文本。
		msg := strings.TrimSpace(stripTags(firstGroup(buyResp.Body, `(?s)<div class="message"[^>]*>(.*?)</div>`)))
		if msg == "" {
			msg = "购买刮奖资格失败（余额不足或今日已达上限）"
		}
		return &PlayResult{Success: false, Message: msg}, nil
	}

	// 2. 刮开：POST scratch_id 返 JSON {success, prize, prize_level, scratch_id}。
	scratchResp, err := hc.PostFormAjax(ctx, endpoint+"?action=scratch", cr.Cookie, "scratch_id="+scratchID, referer, true)
	if err != nil {
		return nil, err
	}
	logResp("novahd_scratch scratch", endpoint, scratchResp)
	m, err := parseRaw(scratchResp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "解析刮奖结果失败"}, nil
	}
	res := &PlayResult{
		Success: boolField(m, "success"),
		Amount:  numField(m, "prize"),
		Prize:   strField(m, "prize_level"),
		Raw:     m,
	}
	if res.Success && res.Amount > 0 {
		res.Message = fmt.Sprintf("%s：获得 %g 魔力值", res.Prize, res.Amount)
	} else if res.Success {
		res.Prize, res.Message = "未中奖", "很遗憾，未中奖"
	}

	// 3. 结算（best-effort）：GET settle，把中奖魔力入账。
	if res.Success && res.Amount > 0 {
		_, _ = hc.GetWithHeaders(ctx, endpoint+"?action=settle&scratch_id="+scratchID, cr.Cookie, map[string]string{"Referer": referer}, true)
	}
	return res, nil
}

// Stats 余额 + 今日剩余/上限（抠 magic_scratch.php 页面）。
func (NovaHDScratchGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "magic_scratch.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	balance := novaHDNum(firstGroup(resp.Body, `余额[^0-9-]*<[^>]*>\s*([\d,.]+)`))
	if balance == 0 {
		balance = novaHDNum(firstGroup(resp.Body, `余额[^0-9-]*([\d,.]+)`))
	}
	limit, remaining := 5, 5
	if g := findGroups(resp.Body, `(\d+)\s*/\s*(\d+)`); len(g) >= 3 { // 今日剩余 remaining/limit
		remaining, _ = strconv.Atoi(strings.TrimSpace(g[1]))
		limit, _ = strconv.Atoi(strings.TrimSpace(g[2]))
	}
	return &StatsResult{
		Balance:     balance,
		TodaySpins:  limit - remaining,
		MaxDaily:    limit,
		CostPerPlay: 5000,
	}, nil
}

// Prizes 6 档中奖规则（源码 magic_scratch.php 11-18 行常量）。
func (NovaHDScratchGame) Prizes(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]PrizeDef, error) {
	return []PrizeDef{
		{ID: 0, Name: "未中奖（无奖励）", Type: "99"},
		{ID: 5, Name: "五等奖 +500 魔力值", Type: "01"},
		{ID: 4, Name: "四等奖 +1000 魔力值", Type: "01"},
		{ID: 3, Name: "三等奖 +3000 魔力值", Type: "01"},
		{ID: 2, Name: "二等奖 +5000 魔力值", Type: "01"},
		{ID: 1, Name: "一等奖 +100000 魔力值", Type: "01"},
	}, nil
}

// History 刮奖记录（页面渲染结构待真实校准，暂返回空）。
func (NovaHDScratchGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	return []GameRecord{}, nil
}

// ── 幸运抽奖 ────────────────────────────────────────────────

// NovaHDLuckyGame 幸运抽奖：POST fortune-wheel-spin.php count=N。
// 响应 {success, results:[{status,type,value,unit,name?}]}，奖品 type：bonus/attendance_card/upload/nothing。
type NovaHDLuckyGame struct{}

func (NovaHDLuckyGame) ID() string   { return "novahd_lucky" }
func (NovaHDLuckyGame) Name() string { return "幸运抽奖" }

func (NovaHDLuckyGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return NovaHDLuckyGame{}.spin(ctx, hc, baseURL, cr, 1)
}

// PlayMulti 连抽（count=10/20/50）。
func (NovaHDLuckyGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count < 1 {
		count = 1
	}
	if count > maxMultiCount {
		count = maxMultiCount
	}
	return NovaHDLuckyGame{}.spin(ctx, hc, baseURL, cr, count)
}

func (NovaHDLuckyGame) spin(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	u := joinPath(baseURL, "fortune-wheel-spin.php")
	referer := joinPath(baseURL, "fortune-wheel.php")
	resp, err := hc.PostFormAjax(ctx, u, cr.Cookie, fmt.Sprintf("count=%d", count), referer, true)
	if err != nil {
		return nil, err
	}
	logResp("novahd_lucky spin", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	if !boolField(m, "success") {
		return &PlayResult{Success: false, Message: strField(m, "message")}, nil
	}
	results, _ := m["results"].([]any)
	out := &PlayResult{Success: true, Prizes: make([]PrizeOutcome, 0, len(results))}
	var sum float64
	for _, r := range results {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		o := novaHDLuckyOutcome(rm)
		out.Prizes = append(out.Prizes, o)
		if o.IsWin && o.PrizeType == "bonus" { // 顶层 Amount 仅累加魔力值收益（补签卡/上传 value 是其他单位）
			sum += o.Amount
		}
	}
	out.Amount = sum
	for _, o := range out.Prizes {
		if o.IsWin {
			out.PrizeID, out.Prize = o.PrizeID, o.PrizeName
			break
		}
	}
	if out.Prize == "" {
		if len(out.Prizes) > 0 {
			out.Prize, out.Message = "未中奖", "很遗憾，本次未中奖"
		} else {
			out.Message = "抽奖完成"
		}
	}
	return out, nil
}

// novaHDLuckyOutcome 把单个奖品结果转 PrizeOutcome。
func novaHDLuckyOutcome(rm map[string]any) PrizeOutcome {
	typ := strField(rm, "type")
	name := firstNonEmpty(strField(rm, "name"), strField(rm, "prize_name"))
	if name == "" {
		switch typ {
		case "bonus":
			name = "魔力值"
		case "attendance_card":
			name = "补签卡"
		case "upload":
			name = "上传量"
		case "nothing":
			name = "谢谢参与"
		default:
			name = "奖品"
		}
	}
	// 站点可能省略 status 只回 type+value：仅以 typ 判定中奖（nothing/空=未中）。
	isWin := typ != "nothing" && typ != ""
	return PrizeOutcome{
		PrizeID:   int(numField(rm, "id")),
		PrizeName: name,
		PrizeType: typ,
		Amount:    numField(rm, "value"),
		UnitName:  strField(rm, "unit"),
		IsWin:     isWin,
	}
}

// Stats 抽奖信息（余额/每次消耗/今日/免费，抠 fortune-wheel.php 页面，文本以真实页面为准）。
func (NovaHDLuckyGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "fortune-wheel.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	body := resp.Body
	res := &StatsResult{}
	// 余额（魔力值，页面可能不直接展示，抠首个大数兜底）。
	res.Balance = novaHDNum(firstGroup(body, `魔力[^0-9<]{0,6}([\d,.]+)`))
	// 每次消耗。
	if v := firstGroup(body, `消耗[^0-9]{0,6}([\d,.]+)`); v != "" {
		res.CostPerPlay = novaHDNum(v)
	}
	// 今日已抽 N/M。
	if g := findGroups(body, `(\d+)\s*/\s*(\d+)`); len(g) >= 3 {
		res.TodaySpins, _ = strconv.Atoi(strings.TrimSpace(g[1]))
		res.MaxDaily, _ = strconv.Atoi(strings.TrimSpace(g[2]))
	}
	return res, nil
}

// Prizes 奖品列表（由 getPrizeGrades 渲染，结构待真实校准，暂返回空）。
func (NovaHDLuckyGame) Prizes(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]PrizeDef, error) {
	return []PrizeDef{}, nil
}

// History 抽奖记录（getUserDrawRecords 渲染，待真实校准，暂返回空）。
func (NovaHDLuckyGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	return []GameRecord{}, nil
}
