package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

// MTeamAdapter 馒头考核：POST /api/examine/getMyActiveList（x-api-key）。
// 考核指标字段含馒头后端拼写错误（uploaed/downloaed/bonuses），逐字匹配。
type MTeamAdapter struct{}

// Platform 返回平台标识。
func (MTeamAdapter) Platform() string { return "mteam" }

// Supported 默认支持。
func (MTeamAdapter) Supported() bool { return true }

// FetchAssessments 抓取活跃考核任务。
func (MTeamAdapter) FetchAssessments(ctx context.Context, hc HTTPDoer, def Def, baseURL, token, uid string) ([]Task, error) {
	if token == "" {
		return nil, nil
	}
	body := []byte("{}")
	resp, err := hc.Do(ctx, "POST", joinBase(baseURL, "/api/examine/getMyActiveList"), body, map[string]string{
		"x-api-key":    token,
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}, "", true)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("mteam assessment: empty response")
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mteam assessment: status %d", resp.StatusCode)
	}
	tasks, err := parseMTeamExamine(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, nil
	}
	return tasks, nil
}

// parseMTeamExamine 解析 /api/examine/getMyActiveList 响应。
// 响应 data 为数组，每项含 uploadTarget/uploaed(拼写错误)/downloadTarget/downloaed/bonusTarget/bonuses 等（字节）。
// 时间字段（startTime/endTime，同 freeleech 端点）为 "2006-01-02 15:04:05" 字符串，须走
// ParseInLocation（见 parseTimeMs），不能按数字拼接——否则日期串的数字会被拼成错乱的 epoch ms。
func parseMTeamExamine(body []byte) ([]Task, error) {
	var env struct {
		Code    json.Number      `json:"code"`
		Message string           `json:"message"`
		Data    []map[string]any `json:"data"`
	}
	// 非 JSON 与 code!=0 都是失败：旧写法 return nil，考核页于是显示「暂无考核」，
	// 正处于考核期的用户看不到任何提醒（漏达标会被降级/封号）。
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("mteam 考核响应不是 JSON（可能是错误页或登录墙）: %w", err)
	}
	if code := env.Code.String(); code != "" && code != "0" {
		return nil, fmt.Errorf("mteam 考核接口报错（code=%s msg=%s）", code, strings.TrimSpace(env.Message))
	}
	if len(env.Data) == 0 {
		return nil, nil
	}
	var out []Task
	for _, item := range env.Data {
		metrics := mTeamExamineMetrics(item)
		name := jsonString(item, "name", "title", "taskName", "examineName")
		if name == "" {
			name = "馒头考核"
		}
		// 状态：仅当解析到指标时才按「全部达标」判 COMPLETED；馒头常只给倒计时、无指标，此时保持 ACTIVE。
		status := StatusActive
		if len(metrics) > 0 {
			allMet := true
			for _, m := range metrics {
				if !m.IsCompleted {
					allMet = false
					break
				}
			}
			if allMet {
				status = StatusCompleted
			}
		}
		startTime := parseTimeMs(item, "startTime", "beginTime", "beginDate")
		endTime := parseTimeMs(item, "endTime", "deadline", "expireTime", "endDate")
		// 馒头常以描述串给倒计时（"距離考核結束還有4 天，考核期間不享受所有魔力加成"），无独立绝对
		// 日期字段时回退：从串里抽 N 天，EndTime = now + N 天（convertTaskToCard 据此算剩余天数）。
		if endTime == 0 {
			if days := extractRemainDays(item); days > 0 {
				endTime = time.Now().Add(time.Duration(days) * 24 * time.Hour).UnixMilli()
			}
		}
		// 已过期 → FAILED（与 NexusPHP 对齐）
		if status == StatusActive && endTime > 0 && time.Now().UnixMilli() > endTime {
			status = StatusFailed
		}
		out = append(out, Task{
			Name: name, Metrics: metrics, Status: status,
			Description: extractNote(item, name),
			StartTime:   startTime,
			EndTime:     endTime,
			FetchedAt:   time.Now(),
		})
	}
	return out, nil
}

// mteamRemainDaysRe 从馒头考核描述串抽剩余天数，兼容繁简：
// "距離考核結束還有4 天" / "距离考核结束还有4天" / "剩餘3天" / "剩余3天"。
var mteamRemainDaysRe = regexp.MustCompile(`(?:還有|还有|剩[餘余]|結束|结束)\s*(\d{1,4})\s*天`)

// extractRemainDays 扫描 item 所有字符串字段，取考核剩余天数。
// 馒头 examine 常以一句描述给出倒计时（而非独立的绝对日期字段），故遍历所有 string 值。
func extractRemainDays(item map[string]any) int {
	for _, v := range item {
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		if m := mteamRemainDaysRe.FindStringSubmatch(s); len(m) > 1 {
			if n := int(atofSimpleInt64(m[1])); n > 0 {
				return n
			}
		}
	}
	return 0
}

// mteamNoteRe 考核说明串特征词（繁简兼容）：加成/考核期間/倒计时词。
var mteamNoteRe = regexp.MustCompile(`加成|考核期間|考核期间|還有|还有|剩[餘余]`)

// extractNote 取考核说明整句（"距離考核結束還有4 天，考核期間不享受所有魔力加成"）展示在卡片。
// 取含特征词的最长字符串字段，跳过与 name 相同者（避免把短名当说明）。
func extractNote(item map[string]any, name string) string {
	best := ""
	for _, v := range item {
		s, ok := v.(string)
		if !ok || s == "" || s == name {
			continue
		}
		if mteamNoteRe.MatchString(s) && len([]rune(s)) > len([]rune(best)) {
			best = s
		}
	}
	return best
}

// mTeamExamineMetrics 考核指标（字节→GB）。注意馒头拼写错误字段 uploaed/downloaed/bonuses。
func mTeamExamineMetrics(item map[string]any) []Metric {
	gb := func(v any) float64 { return float64(jsonInt64(v)) / 1073741824.0 }
	var out []Metric
	// 上传量
	if req, ok := item["uploadTarget"]; ok {
		cur := firstPresent(item, "uploaed", "uploaded", "upload") // uploaed 是馒头拼写错误，优先
		met := gb(cur) >= gb(req)
		out = append(out, Metric{Label: "上传量", Required: gb(req), Current: gb(cur), Unit: "GB", IsCompleted: met})
	}
	// 下载量
	if req, ok := item["downloadTarget"]; ok {
		cur := firstPresent(item, "downloaed", "downloaded", "download")
		met := gb(cur) >= gb(req)
		out = append(out, Metric{Label: "下载量", Required: gb(req), Current: gb(cur), Unit: "GB", IsCompleted: met})
	}
	// 魔力值
	if req, ok := item["bonusTarget"]; ok {
		cur := firstPresent(item, "bonuses", "bonus", "bonusCurrent") // bonuses 复数，优先
		reqF := float64(jsonInt64(req))
		curF := float64(jsonInt64(cur))
		out = append(out, Metric{Label: "魔力值", Required: reqF, Current: curF, IsCompleted: curF >= reqF})
	}
	return out
}

// TNodeAdapter TNode（朱雀）用技能释放系统，无传统考核。
type TNodeAdapter struct{}

// Platform 返回平台标识。
func (TNodeAdapter) Platform() string { return "tnode" }

// Supported 返回 false。
func (TNodeAdapter) Supported() bool { return false }

// FetchAssessments 返回空。
func (TNodeAdapter) FetchAssessments(context.Context, HTTPDoer, Def, string, string, string) ([]Task, error) {
	return nil, nil
}

// ── JSON 容错取值 helper ──

func jsonString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func firstPresent(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return 0
}

func jsonInt64(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	case string:
		return atofSimpleInt64(t)
	}
	return 0
}

// parseTimeMs 容错解析时间字段：先按日期串（"2006-01-02 15:04:05" 等，MTeam 多端点返回此格式）
// 解析，再回退到 epoch ms/s 数字。直接对字符串走 jsonInt64 会让日期串的数字被拼成一串
// （如 "2026-01-02 03:04:05" → 20260102030405ms ≈ 公元 2600 年），导致前端考核时间显示错乱。
func parseTimeMs(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		// 字符串：先按日期格式解析（MTeam 常见）；失败再交给数字回退（纯数字串如 "1735689600"）
		if s, ok := v.(string); ok && s != "" {
			if ms := parseDateTimeMs(s); ms > 0 {
				return ms
			}
		}
		if n := jsonInt64(v); n > 1e12 {
			return n // ms
		} else if n > 1e9 {
			return n * 1000 // s → ms
		}
	}
	return 0
}

func atofSimpleInt64(s string) int64 {
	n := int64(0)
	neg := false
	for i, r := range s {
		switch {
		case r == '-' && i == 0:
			neg = true
		case r >= '0' && r <= '9':
			n = n*10 + int64(r-'0')
		}
	}
	if neg {
		return -n
	}
	return n
}
