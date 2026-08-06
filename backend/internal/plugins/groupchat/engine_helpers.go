package groupchat

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// —— 辅助 ——

func (e *Engine) resolveSelectedSites(all []SiteRef) []SiteRef {
	var out []SiteRef
	switch v := e.cfg["chat_sites"].(type) {
	case []any:
		// multiselect: 选中的站点 ID 数组
		for _, item := range v {
			id := cfgToUint(item)
			if id == 0 {
				continue
			}
			for _, s := range all {
				if s.ID == id {
					out = append(out, s)
					break
				}
			}
		}
	case string:
		// 向后兼容旧 textarea（每行 id 或站名关键字）
		raw := strings.TrimSpace(v)
		if raw == "" {
			return nil
		}
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if id, err := strconv.ParseUint(line, 10, 64); err == nil {
				for _, s := range all {
					if uint(id) == s.ID {
						out = append(out, s)
						break
					}
				}
				continue
			}
			if s, ok := MatchSiteByName(all, line); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// cfgToUint 把配置值（JSON 反序列化为 float64/int/string）转 uint，失败返回 0。
func cfgToUint(v any) uint {
	switch x := v.(type) {
	case float64:
		return uint(x)
	case int:
		return uint(x)
	case int64:
		return uint(x)
	case string:
		if id, err := strconv.ParseUint(strings.TrimSpace(x), 10, 64); err == nil {
			return uint(id)
		}
	}
	return 0
}

func hasZmSite(sites []SiteRef) bool {
	for _, s := range sites {
		if strings.Contains(s.Name, "织梦") {
			return true
		}
	}
	return false
}

func findSiteByID(sites []SiteRef, id uint) (SiteRef, bool) {
	for _, s := range sites {
		if s.ID == id {
			return s, true
		}
	}
	return SiteRef{}, false
}

func levelFor(results []SiteResult) string {
	for _, r := range results {
		if len(r.Failed) > 0 {
			return "warning"
		}
	}
	return "success"
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return false
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-t.C:
		return false
	}
}

func boolCfg(cfg map[string]any, key string, def bool) bool {
	v, ok := cfg[key]
	if !ok || v == nil {
		return def
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "1" || x == "yes"
	default:
		return def
	}
}

func intCfg(cfg map[string]any, key string, def int) int {
	v, ok := cfg[key]
	if !ok || v == nil {
		return def
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
			return n
		}
	}
	return def
}

func strCfg(cfg map[string]any, key string) string {
	v, ok := cfg[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}
