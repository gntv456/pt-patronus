package assessment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

// SunnyptAdapter sunnypt（schema=tnode 特例）：GET /api/v1/user/assessments。
// 鉴权：Bearer accessToken（从 AuthSession cookie JWT payload 抽取，与 schemas/sunnypt 一致）。
// 指标字段（前端映射）：uploaded_bytes / downloaded_bytes / seed_time_seconds /
// seed_points / torrent_count / bonus；每项 {value,target,completed}。
// 状态：in_progress / passed / failed / cancelled。
type SunnyptAdapter struct{}

func (SunnyptAdapter) Platform() string { return "sunnypt" }
func (SunnyptAdapter) Supported() bool  { return true }

// FetchAssessments 抓取用户考核列表。
func (SunnyptAdapter) FetchAssessments(ctx context.Context, hc HTTPDoer, def Def, baseURL, token, uid string) ([]Task, error) {
	if token == "" {
		return nil, nil
	}
	access, err := extractSunnyAccessToken(token)
	if err != nil || access == "" {
		return nil, nil
	}
	apiBase := sunnyAPIBaseFrom(baseURL)
	path := "/api/v1/user/assessments?limit=100&offset=0"
	resp, err := hc.Do(ctx, "GET", joinBase(apiBase, path), nil, map[string]string{
		"Authorization": "Bearer " + access,
		"Accept":        "application/json",
		"Origin":        "https://sunnypt.top",
		"Referer":       "https://sunnypt.top/",
	}, "", true)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("sunnypt assessment: empty response")
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sunnypt assessment: status %d", resp.StatusCode)
	}
	return parseSunnyptAssessments(resp.Body), nil
}

func parseSunnyptAssessments(body []byte) []Task {
	var wrap struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]any `json:"list"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &wrap) != nil || wrap.Code != 0 {
		return nil
	}
	out := make([]Task, 0, len(wrap.Data.List))
	for _, item := range wrap.Data.List {
		name := jsonString(item, "name", "title", "task_name")
		if name == "" {
			name = "Sunny 考核"
		}
		status := mapSunnyStatus(jsonString(item, "status", "state"))
		metrics := sunnyMetrics(item)
		// 全部达标 → COMPLETED（若状态仍是 active）
		if status == StatusActive && len(metrics) > 0 {
			all := true
			for _, m := range metrics {
				if !m.IsCompleted {
					all = false
					break
				}
			}
			if all {
				status = StatusCompleted
			}
		}
		start := parseTimeMs(item, "start_time", "startTime", "begin_time", "created_at")
		end := parseTimeMs(item, "end_time", "endTime", "deadline", "expire_at", "expires_at")
		if status == StatusActive && end > 0 && time.Now().UnixMilli() > end {
			status = StatusFailed
		}
		desc := jsonString(item, "description", "desc", "note")
		if reward, ok := item["success_reward_bonus"]; ok {
			if n := jsonInt64(reward); n > 0 {
				if desc != "" {
					desc += "；"
				}
				desc += fmt.Sprintf("达标奖励 %d 魔力", n)
			}
		}
		out = append(out, Task{
			Name: name, Description: desc, Metrics: metrics, Status: status,
			StartTime: start, EndTime: end, FetchedAt: time.Now(),
		})
	}
	return out
}

func mapSunnyStatus(s string) Status {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "passed", "completed", "success", "done":
		return StatusCompleted
	case "failed", "fail", "rejected":
		return StatusFailed
	case "cancelled", "canceled", "exempt":
		return StatusExempt
	case "in_progress", "active", "pending", "ongoing", "":
		return StatusActive
	default:
		return StatusActive
	}
}

// sunnyMetricLabels 前端显示名。
var sunnyMetricLabels = map[string]string{
	"uploaded_bytes":    "上传量",
	"downloaded_bytes":  "下载量",
	"seed_time_seconds": "做种时长",
	"seed_points":       "做种积分",
	"torrent_count":     "发布种子",
	"bonus":             "获得魔力",
}

func sunnyMetrics(item map[string]any) []Metric {
	raw, ok := item["metrics"]
	if !ok || raw == nil {
		return nil
	}
	// metrics 可能是 map[string]object 或 array
	switch m := raw.(type) {
	case map[string]any:
		out := make([]Metric, 0, len(m))
		for k, v := range m {
			if v == nil {
				continue
			}
			obj, ok := v.(map[string]any)
			if !ok {
				continue
			}
			label := sunnyMetricLabels[k]
			if label == "" {
				label = k
			}
			cur := jsonFloat(obj, "value", "current", "now")
			req := jsonFloat(obj, "target", "required", "goal")
			completed := jsonBool(obj, "completed", "is_completed", "done")
			if !completed && req > 0 && cur >= req {
				completed = true
			}
			unit := "count"
			if strings.Contains(k, "bytes") {
				unit = "bytes"
			} else if strings.Contains(k, "time") {
				unit = "seconds"
			} else if k == "bonus" || k == "seed_points" {
				unit = "points"
			}
			out = append(out, Metric{
				Label: label, Required: req, Current: cur, Unit: unit, IsCompleted: completed,
			})
		}
		return out
	case []any:
		out := make([]Metric, 0, len(m))
		for _, it := range m {
			obj, ok := it.(map[string]any)
			if !ok {
				continue
			}
			key := jsonString(obj, "key", "type", "name", "metric")
			label := sunnyMetricLabels[key]
			if label == "" {
				label = jsonString(obj, "label", "name", "title")
			}
			if label == "" {
				label = key
			}
			cur := jsonFloat(obj, "value", "current", "now")
			req := jsonFloat(obj, "target", "required", "goal")
			completed := jsonBool(obj, "completed", "is_completed", "done")
			out = append(out, Metric{
				Label: label, Required: req, Current: cur, IsCompleted: completed,
			})
		}
		return out
	}
	return nil
}

func jsonFloat(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch t := v.(type) {
			case float64:
				return t
			case json.Number:
				f, _ := t.Float64()
				return f
			case string:
				var f float64
				fmt.Sscanf(t, "%f", &f)
				return f
			}
		}
	}
	return 0
}

func jsonBool(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch t := v.(type) {
			case bool:
				return t
			case float64:
				return t != 0
			case string:
				return t == "true" || t == "1" || t == "yes"
			}
		}
	}
	return false
}

// extractSunnyAccessToken 从 cookie 串解析 AuthSession JWT → accessToken。
func extractSunnyAccessToken(cookie string) (string, error) {
	// 复用最简逻辑，避免 assessment 包依赖 schemas。
	re := sunnyAuthSessionFind(cookie)
	if re == "" {
		return "", fmt.Errorf("no AuthSession")
	}
	parts := strings.Split(re, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("bad jwt")
	}
	payload, err := b64URLDecode(parts[1])
	if err != nil {
		return "", err
	}
	var p struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", err
	}
	if p.AccessToken == "" {
		return "", fmt.Errorf("empty accessToken")
	}
	return p.AccessToken, nil
}

func sunnyAuthSessionFind(cookie string) string {
	best := ""
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "AuthSession=") {
			continue
		}
		val := strings.TrimPrefix(part, "AuthSession=")
		if strings.Count(val, ".") == 2 && len(val) > len(best) {
			best = val
		}
	}
	return best
}

func b64URLDecode(s string) ([]byte, error) {
	pad := (4 - len(s)%4) % 4
	s = s + strings.Repeat("=", pad)
	return base64.URLEncoding.DecodeString(s)
}

func sunnyAPIBaseFrom(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return "https://api.sunnypt.top"
	}
	host := u.Hostname()
	if !strings.HasPrefix(host, "api.") {
		host = "api." + host
	}
	u.Host = host
	if port := u.Port(); port != "" {
		u.Host += ":" + port
	}
	return strings.TrimRight(u.String(), "/")
}
