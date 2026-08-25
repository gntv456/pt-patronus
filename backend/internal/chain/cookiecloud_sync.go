package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/cookiecloud"
	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// applyPull 把云端 cookie 写入本地失效站 + 自动新增匹配定义站。
func (c *CookieCloudChain) applyPull(ctx context.Context, s CookieCloudSettings, payload map[string][]cookiecloud.CookieEntry, report *CookieCloudSyncReport) error {
	var mu sync.Mutex
	addErr := func(msg string) {
		mu.Lock()
		report.Errors = append(report.Errors, msg)
		mu.Unlock()
	}

	sites, err := c.siteOp.ListActive()
	if err != nil {
		return fmt.Errorf("读取本地站点失败: %w", err)
	}
	occupied := map[string]bool{}
	for i := range sites {
		occupied[site.RegisteredDomain(sites[i].BaseURL)] = true
	}

	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for i := range sites {
		st := &sites[i]
		cookieStr := cookieForSite(payload, st.BaseURL)
		if cookieStr == "" {
			mu.Lock()
			report.Skipped++
			mu.Unlock()
			continue
		}
		wg.Add(1)
		safego.Go("cookiecloud:305", func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			force := normalizeCookieCloudUpdateMode(s.UpdateMode) == "always"
			if !force && c.isConnected(ctx, st) {
				mu.Lock()
				report.Skipped++
				mu.Unlock()
				return
			}
			if cur, err := c.siteOp.Cookie(st); err == nil && cur != "" {
				cookieStr = site.MergeCookieHeader(cur, cookieStr)
			}
			if err := c.siteOp.UpdateCookie(st.ID, cookieStr); err != nil {
				addErr(fmt.Sprintf("%s: 更新失败 %v", st.Name, err))
				return
			}
			mu.Lock()
			report.Updated++
			mu.Unlock()
		})
	}
	wg.Wait()

	defs := c.registry.ActiveDefinitions()
	for h := range payload {
		dom := site.RegisteredDomain(h)
		if dom == "" || occupied[dom] {
			continue
		}
		cookieStr := cookieForSite(payload, "https://"+h)
		if cookieStr == "" {
			continue
		}
		def := findDefinitionByHost(h, defs)
		if def == nil || len(def.URLs) == 0 {
			continue
		}
		_, err := c.siteOp.Create(oper.SiteInput{
			Name:       def.Name,
			Platform:   def.Schema,
			Definition: def.ID,
			BaseURL:    def.URLs[0],
			Cookie:     cookieStr,
			IsActive:   true,
		})
		if err != nil {
			addErr(fmt.Sprintf("%s: 新增失败 %v", def.Name, err))
			continue
		}
		mu.Lock()
		report.Added++
		occupied[dom] = true
		mu.Unlock()
	}
	return nil
}

// pushLocal 把本地启用站 cookie 合并进 base payload 后写回云端。
// builtin 模式：直接写内置 store；external 模式：POST /update。
// 返回成功写入云端的 host 数。base 可为 nil。
func (c *CookieCloudChain) pushLocal(ctx context.Context, s CookieCloudSettings, password string, base map[string][]cookiecloud.CookieEntry) (int, error) {
	sites, err := c.siteOp.ListActive()
	if err != nil {
		return 0, fmt.Errorf("读取本地站点失败: %w", err)
	}
	local := map[string][]cookiecloud.CookieEntry{}
	for i := range sites {
		st := &sites[i]
		host := cookieHostFromBaseURL(st.BaseURL)
		if host == "" {
			continue
		}
		ck, err := c.siteOp.Cookie(st)
		if err != nil || strings.TrimSpace(ck) == "" {
			continue
		}
		entries := cookiecloud.EntriesFromCookieHeader(host, ck)
		if len(entries) == 0 {
			continue
		}
		local[host] = entries
	}
	if len(local) == 0 {
		return 0, nil
	}
	merged := cookiecloud.MergeCookieData(base, local)
	enc, err := cookiecloud.EncryptPayload(merged, s.UUID, password)
	if err != nil {
		return 0, fmt.Errorf("加密推送数据失败: %w", err)
	}
	if normalizeCookieCloudMode(s.Mode) == CookieCloudModeBuiltin {
		c.serverStore().Put(s.UUID, cookiecloud.ServerEnvelope{
			Encrypted: enc, CryptoType: "aes-128-cbc-fixed",
		})
		return len(local), nil
	}
	body, err := cookiecloud.EncodeUpdateRequest(s.UUID, enc)
	if err != nil {
		return 0, err
	}
	updateURL := strings.TrimRight(s.Server, "/") + "/update"
	resp, err := c.hc.Do(ctx, "POST", updateURL, body, map[string]string{"Content-Type": "application/json"}, "", false)
	if err != nil {
		return 0, fmt.Errorf("推送 CookieCloud 失败: %w", err)
	}
	// 标准服务返回 {"action":"done"|"error"}；兼容无 body / 其它 2xx。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("推送 CookieCloud 返回 HTTP %d", resp.StatusCode)
	}
	if len(resp.Body) > 0 {
		var out struct {
			Action string `json:"action"`
		}
		if json.Unmarshal(resp.Body, &out) == nil && out.Action != "" && out.Action != "done" {
			return 0, fmt.Errorf("推送 CookieCloud 失败: action=%s", out.Action)
		}
	}
	return len(local), nil
}

// Push 仅推送本地 cookie 到云端（忽略配置中的 direction；手动按钮用）。
// 回写 last_sync 时保留用户原 direction，避免把配置改成 push。
func (c *CookieCloudChain) Push(ctx context.Context) (CookieCloudSyncReport, error) {
	s := c.GetSettings()
	run := s
	run.Direction = "push"
	report, err := c.doSync(ctx, run)
	c.recordSyncResult(s, report, err)
	return report, err
}

// isConnected 复用 SiteChain.TestConnection 判定当前 cookie 是否仍有效（err=nil 即已登录）。
func (c *CookieCloudChain) isConnected(ctx context.Context, s *models.Site) bool {
	if c.siteChain == nil {
		return false
	}
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := c.siteChain.TestConnection(tctx, s)
	return err == nil
}

// ── 纯函数（便于单测，不依赖 DB/网络）──

// cookieForSite 从 CookieCloud payload 中提取匹配 siteBaseURL 注册域的所有 cookie，拼成 "k=v; k2=v2"。
// 同名 cookie 去重：更长（更具体）的 host 后写入并覆盖短 host。
func cookieForSite(payload map[string][]cookiecloud.CookieEntry, siteBaseURL string) string {
	siteDom := site.RegisteredDomain(siteBaseURL)
	if siteDom == "" {
		return ""
	}
	type hostBag struct {
		host    string
		cookies []cookiecloud.CookieEntry
	}
	var matched []hostBag
	for h, cookies := range payload {
		if site.RegisteredDomain(h) != siteDom {
			continue
		}
		matched = append(matched, hostBag{host: h, cookies: cookies})
	}
	if len(matched) == 0 {
		return ""
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return len(matched[i].host) < len(matched[j].host)
	})
	m := map[string]string{}
	for _, hb := range matched {
		for _, ck := range hb.cookies {
			if ck.Name == "" {
				continue
			}
			m[ck.Name] = ck.Value
		}
	}
	return site.FormatCookieHeader(m)
}

// normalizeCookieCloudUpdateMode 归一化更新策略。
func normalizeCookieCloudUpdateMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "always":
		return "always"
	case "disconnected", "":
		return "disconnected"
	default:
		return ""
	}
}

func findDefinitionByHost(host string, defs []*site.SiteMetadata) *site.SiteMetadata {
	dom := site.RegisteredDomain(host)
	if dom == "" {
		return nil
	}
	for _, d := range defs {
		for _, u := range d.URLs {
			if site.RegisteredDomain(u) == dom {
				return d
			}
		}
	}
	return nil
}

// normalizeCookieCloudDirection 归一化同步方向。
func normalizeCookieCloudDirection(dir string) string {
	switch strings.ToLower(strings.TrimSpace(dir)) {
	case "pull", "push", "both":
		return strings.ToLower(strings.TrimSpace(dir))
	case "":
		return "both"
	default:
		return ""
	}
}

// cookieHostFromBaseURL 取 URL hostname（小写、无端口）作为 CookieCloud cookie_data 的 key。
func cookieHostFromBaseURL(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Hostname() == "" {
		return site.RegisteredDomain(baseURL)
	}
	return strings.ToLower(u.Hostname())
}
