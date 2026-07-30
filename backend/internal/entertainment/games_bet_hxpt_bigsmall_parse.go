package entertainment

import (
	"strings"
)

// ── 好学猜大小 解析助手 ──────────────────────────────────

func hxptTrend(v any) BigSmallTrend {
	m, _ := v.(map[string]any)
	if m == nil {
		return BigSmallTrend{}
	}
	return BigSmallTrend{
		BigCount:   int(longptNum(m["big"])),
		SmallCount: int(longptNum(m["small"])),
		HouseCount: int(longptNum(m["house"])),
		BigPct:     int(longptNum(m["big_pct"])),
		SmallPct:   int(longptNum(m["small_pct"])),
	}
}

func hxptBettors(v any) []BigSmallBetSpot {
	arr, _ := v.([]any)
	out := make([]BigSmallBetSpot, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallBetSpot{Username: strField(m, "username"), Amount: numField(m, "amount")})
	}
	return out
}

// hxptRankTabs 榜单 tab→数据键/金额键/次数键（对齐站点 JS bigsmall.js:690-701 动态取键）。
var hxptRankTabs = []struct{ key, dataKey, title, amtKey, cntKey string }{
	{"monthly_king", "monthly_king", "本月赌神", "total_won", "king_count"},
	{"daily_king", "daily_kings", "今日赌王", "total_won", "king_count"},
	{"monthly_charity", "monthly_charity", "本月慈善", "total_lost", "charity_count"},
	{"daily_charity", "daily_charity", "今日慈善", "total_lost", "charity_count"},
	{"monthly_charity_king", "monthly_charity_king", "慈善之神", "net_amount", "game_count"},
	{"daily_charity_king", "daily_charity_king", "慈善大王", "net_amount", "game_count"},
}

func hxptRankings(d map[string]any) []BigSmallRank {
	out := make([]BigSmallRank, 0, len(hxptRankTabs))
	for _, t := range hxptRankTabs {
		arr, _ := d[t.dataKey].([]any)
		if len(arr) == 0 {
			continue
		}
		r := BigSmallRank{Key: t.key, Title: t.title, Entries: make([]BigSmallRankEntry, 0, len(arr))}
		for _, it := range arr {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			r.Entries = append(r.Entries, BigSmallRankEntry{
				Username: strField(m, "username"),
				Amount:   numField(m, t.amtKey),
				Count:    int(longptNum(m[t.cntKey])),
			})
		}
		out = append(out, r)
	}
	return out
}

func hxptMyRecords(arr []any) []BigSmallRecord {
	out := make([]BigSmallRecord, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallRecord{
			GameNo:      firstNonEmpty(strField(m, "game_no"), numToStr(m["game_no"])),
			StartTime:   strField(m, "start_time"),
			EndTime:     strField(m, "end_time"),
			PlayerCount: int(longptNum(m["player_count"])),
			TotalBet:    numField(m, "total_bet"),
			Result:      strField(m, "result"),
			MyBetType:   strField(m, "my_bet_type"),
			MyBet:       numField(m, "my_bet"),
			MyResult:    numField(m, "my_result"),
		})
	}
	return out
}

func hxptAllRecords(arr []any) []BigSmallRecord {
	out := make([]BigSmallRecord, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		rec := BigSmallRecord{
			GameNo:    firstNonEmpty(strField(m, "game_no"), numToStr(m["game_no"])),
			StartTime: strField(m, "start_time"),
			EndTime:   strField(m, "end_time"),
			Result:    strField(m, "result"),
		}
		if bets, ok := m["bets"].([]any); ok {
			var sb strings.Builder
			for _, b := range bets {
				bm, _ := b.(map[string]any)
				if bm == nil {
					continue
				}
				if sb.Len() > 0 {
					sb.WriteString("；")
				}
				if u := strField(bm, "username"); u != "" {
					sb.WriteString(u)
					sb.WriteString(": ")
				}
				sb.WriteString(strField(bm, "bet_info"))
			}
			rec.Bets = sb.String()
		}
		out = append(out, rec)
	}
	return out
}

func hxptTasks(arr []any) []BigSmallTask {
	out := make([]BigSmallTask, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		status := "idle"
		switch {
		case boolField(m, "can_reward"):
			status = "rewardable"
		case boolField(m, "can_claim"):
			status = "doing"
		case boolField(m, "is_completed"):
			status = "done"
		}
		out = append(out, BigSmallTask{
			ID:           int(longptNum(m["id"])),
			Title:        firstNonEmpty(strField(m, "name"), strField(m, "title")),
			Desc:         strField(m, "description"),
			Reward:       firstNonEmpty(strField(m, "reward_description"), strField(m, "reward_name")),
			ProgressText: strField(m, "progress_text"),
			Progress:     int(longptNum(m["progress"])),
			Status:       status,
			MonthlyCount: int(longptNum(m["monthly_count"])),
			MonthlyLimit: int(longptNum(m["monthly_limit"])),
		})
	}
	return out
}

func hxptProps(v any) []BigSmallProp {
	arr, _ := v.([]any)
	out := make([]BigSmallProp, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallProp{
			ID:       numToStr(m["id"]),
			Tag:      strField(m, "code"),
			Name:     strField(m, "name"),
			Desc:     strField(m, "description"),
			Image:    strField(m, "image"),
			Quantity: int(longptNum(m["quantity"])),
			Usable:   boolField(m, "usable_in_bet"),
		})
	}
	return out
}

func hxptChat(arr []any) []BigSmallChat {
	out := make([]BigSmallChat, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallChat{
			ID:       int(longptNum(m["id"])),
			Username: strField(m, "username"),
			Message:  strField(m, "message"),
			Time:     strField(m, "time"),
		})
	}
	return out
}
