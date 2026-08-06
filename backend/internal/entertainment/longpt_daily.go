package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 每日抽奖 ────────────────────────────────────────────────

// LongPTDailyGame 每日抽奖：GET join 报名（每天 1 次，**后端权威防重**——Go 不自判，按 code/msg 处理）。
// 23:30 后端统一开奖，join 本身不返回奖品。奖品/时间窗口/参与人数走活动配置 + getCountJoin。
type LongPTDailyGame struct{}

func (LongPTDailyGame) ID() string   { return "longpt_daily" }
func (LongPTDailyGame) Name() string { return "每日抽奖" }

// Play 参与每日抽奖（join）。
func (LongPTDailyGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	_, code, msg, err := longptGet(ctx, hc, baseURL, "lucky/pluginsPrizeReceiptRecord/join", "", cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	return &PlayResult{Success: code == 0, Message: msg}, nil
}

// Stats 参与人数（getCountJoin）。时间窗口为站点前端硬编码默认（00:00-23:29 参与，23:30 开奖），
// pluginsLuckyConfig/list 不返回时间，故此处给默认值供 UI 展示。
func (LongPTDailyGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	res := &StatsResult{
		StartTime: "00:00",
		EndTime:   "23:29",
		OpenTime:  "23:30",
	}
	if d, c, _, err := longptGet(ctx, hc, baseURL, "lucky/pluginsPrizeReceiptRecord/getCountJoin", "", cr.Cookie); err == nil && c == 0 {
		var v any
		if json.Unmarshal(d, &v) == nil {
			res.Participants = int(longptNum(v))
		}
	}
	return res, nil
}

// Prizes 今日奖品（pluginsLuckyConfig/list，data 直接是奖品数组 [{desc,prizeType,winner,…}]）。
// desc 形如 "333333魔力,1"（奖品描述,获奖人数），取逗号前作展示名。
func (LongPTDailyGame) Prizes(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]PrizeDef, error) {
	data, code, msg, err := longptPost(ctx, hc, baseURL, "lucky/pluginsLuckyConfig/list",
		map[string]any{"type": "LUCKY_EVERY_DAY"}, cr.Cookie)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("%s", msg)
	}
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse daily prizes: %w", err)
	}
	out := make([]PrizeDef, 0, len(items))
	for i, obj := range items {
		out = append(out, PrizeDef{ID: i + 1, Name: longptDailyDescName(strField(obj, "desc")), Type: strField(obj, "prizeType")})
	}
	return out, nil
}

// longptDailyDescName 每日奖品 desc 取逗号前（奖品描述）；兼容中英文逗号；空则"奖品"。
func longptDailyDescName(desc string) string {
	if desc == "" {
		return "奖品"
	}
	for _, sep := range []string{",", "，"} {
		if i := strings.Index(desc, sep); i >= 0 {
			return desc[:i]
		}
	}
	return desc
}

// History 每日抽奖获奖记录（pluginsPrizeReceiptRecord/getRecordsByPage）。
func (LongPTDailyGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	data, code, msg, err := longptPost(ctx, hc, baseURL, "lucky/pluginsPrizeReceiptRecord/getRecordsByPage",
		map[string]any{"pageNum": 1, "pageSize": 20}, cr.Cookie)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("%s", msg)
	}
	return parseLongPTRecords(data)
}

// parseLongPTRecords 解析分页记录 {records,total}。LongPT 字段为 camelCase，记录精确字段以实测为准。
func parseLongPTRecords(data json.RawMessage) ([]GameRecord, error) {
	var page struct {
		Records []map[string]any `json:"records"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("parse records: %w", err)
	}
	out := make([]GameRecord, 0, len(page.Records))
	for _, r := range page.Records {
		out = append(out, GameRecord{
			PrizeName: firstNonEmpty(strField(r, "prizeName"), strField(r, "name"), strField(r, "rewardTitle")),
			PrizeType: strField(r, "prizeType"),
			Amount:    numField(r, "prizeValue"),
			CreatedAt: firstNonEmpty(strField(r, "createTime"), strField(r, "createdAt")),
		})
	}
	return out, nil
}

// firstNonEmpty 返回首个非空串。
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
