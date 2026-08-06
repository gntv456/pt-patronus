package plugins

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type mpSubscription struct {
	Name string
	URL  string
}

func mpParseSubscriptions(raw string) ([]mpSubscription, error) {
	var out []mpSubscription
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, rawURL, ok := strings.Cut(line, "|")
		if !ok {
			return nil, fmt.Errorf("订阅配置格式错误：%s", line)
		}
		name = strings.TrimSpace(name)
		rawURL = strings.TrimSpace(rawURL)
		u, err := url.Parse(rawURL)
		if name == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("订阅配置无效：%s", line)
		}
		out = append(out, mpSubscription{Name: name, URL: rawURL})
	}
	if len(out) == 0 {
		return nil, errors.New("没有有效订阅配置")
	}
	return out, nil
}

func mpFetchHostsSubscriptions(ctx context.Context, subs []mpSubscription, timeout time.Duration, retries int, allowPrivate bool) (string, []map[string]any) {
	var blocks []string
	results := make([]map[string]any, 0, len(subs))
	for _, sub := range subs {
		result := map[string]any{"name": sub.Name, "url": sub.URL, "ok": false, "records": 0}
		if err := mpValidateHTTPURL(ctx, sub.URL, allowPrivate); err != nil {
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}
		var body []byte
		var err error
		for attempt := 0; attempt <= retries; attempt++ {
			_, _, body, err = mpHTTPRequest(ctx, http.MethodGet, sub.URL, nil, map[string]string{
				"Accept":        "text/plain,text/html,*/*",
				"Cache-Control": "no-cache",
			}, "", timeout)
			if err == nil {
				break
			}
			if attempt < retries {
				time.Sleep(time.Duration(attempt+1) * time.Second)
			}
		}
		if err != nil {
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}
		cleaned := mpCleanHostsContent(string(body))
		records, invalid := mpParseHosts(cleaned)
		if len(records) == 0 {
			result["error"] = "未找到有效 hosts 记录"
			result["invalid"] = invalid
			results = append(results, result)
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "# ===== %s 订阅开始 ===== #\n", sub.Name)
		for _, rec := range records {
			fmt.Fprintf(&b, "%s %s\n", rec["ip"], rec["host"])
		}
		fmt.Fprintf(&b, "# ===== %s 订阅结束 (%d 条记录) ===== #\n", sub.Name, len(records))
		blocks = append(blocks, b.String())
		result["ok"] = true
		result["records"] = len(records)
		result["invalid_count"] = len(invalid)
		results = append(results, result)
	}
	return strings.Join(blocks, "\n"), results
}

func mpCleanHostsContent(content string) string {
	pre := regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`).FindStringSubmatch(content)
	if len(pre) > 1 {
		content = pre[1]
	}
	content = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(content, "")
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func mpParseHosts(content string) ([]map[string]string, []string) {
	var records []map[string]string
	var invalid []string
	for _, line := range strings.Split(content, "\n") {
		original := strings.TrimSpace(line)
		if original == "" || strings.HasPrefix(original, "#") {
			continue
		}
		line = strings.SplitN(original, "#", 2)[0]
		parts := strings.Fields(line)
		if len(parts) < 2 {
			invalid = append(invalid, original)
			continue
		}
		ip := net.ParseIP(parts[0])
		if ip == nil {
			invalid = append(invalid, original)
			continue
		}
		host := strings.TrimSpace(parts[1])
		if host == "" || strings.ContainsAny(host, `/\`) {
			invalid = append(invalid, original)
			continue
		}
		records = append(records, map[string]string{"ip": ip.String(), "host": host})
	}
	return records, invalid
}

func mpMergeSubscriptionHosts(existing, subscriptions string) string {
	if strings.TrimSpace(existing) == "" {
		return strings.TrimSpace(subscriptions)
	}
	var kept []string
	inBlock := false
	for _, line := range strings.Split(existing, "\n") {
		if strings.Contains(line, "# =====") && strings.Contains(line, "订阅开始") {
			inBlock = true
			continue
		}
		if strings.Contains(line, "# =====") && strings.Contains(line, "订阅结束") && inBlock {
			inBlock = false
			continue
		}
		if !inBlock {
			kept = append(kept, strings.TrimRight(line, "\r"))
		}
	}
	base := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if strings.TrimSpace(subscriptions) == "" {
		return base
	}
	if base != "" {
		return base + "\n\n" + strings.TrimSpace(subscriptions)
	}
	return strings.TrimSpace(subscriptions)
}
