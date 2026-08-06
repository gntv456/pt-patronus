package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/cookiecloud"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/core/safego"
)

// cookiecloudSettingsKey 是 SystemConfig 中存储 CookieCloud 配置的 key（JSON）。
const cookiecloudSettingsKey = "cookiecloud.settings"

// ccStore 抽象 SystemConfigOper 的 Get/Set（便于无 DB 单测；oper.SystemConfigOper 隐式实现）。
type ccStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// CookieCloudSettings 是 CookieCloud 同步配置（落 SystemConfig，密码 AES-GCM 加密）。
// 对齐 MoviePilot：浏览器扩展把最新 cookie 端到端加密推到 CookieCloud 服务器，本端定时拉取
// 并按域名覆盖本地失效站的 cookie，根治 Laravel qw_session 等 session 短寿站的掉 cookie 问题。
type CookieCloudSettings struct {
	Enabled      bool   `json:"enabled"`
	Server       string `json:"server"`       // CookieCloud 服务器 URL，如 https://cc.example.com
	UUID         string `json:"uuid"`         // 用户 UUID/key
	PasswordEnc  string `json:"password_enc"` // AES-GCM 加密的端到端密码（security.Crypto.Encrypt）
	IntervalMin  int    `json:"interval_min"` // 同步间隔（分钟），软间隔
	UpdateMode   string `json:"update_mode"`  // disconnected=仅失效站（默认）；always=总是用云端覆盖同名 cookie
	Direction    string `json:"direction"`    // pull=仅拉取；push=仅推送；both=双向（默认）
	LastSyncUnix int64  `json:"last_sync_unix"`
	LastError    string `json:"last_error"`
}

// defaultCookieCloudSettings 填充合理默认值（间隔 2 小时）。
func defaultCookieCloudSettings() CookieCloudSettings {
	return CookieCloudSettings{IntervalMin: 120, UpdateMode: "disconnected", Direction: "both"}
}

// CookieCloudSettingsInput 是 handler 传入的输入（密码明文，空=保留旧密码）。
type CookieCloudSettingsInput struct {
	Enabled     bool   `json:"enabled"`
	Server      string `json:"server"`
	UUID        string `json:"uuid"`
	Password    string `json:"password"` // 明文；空=保留旧密码
	IntervalMin int    `json:"interval_min"`
	UpdateMode  string `json:"update_mode"` // disconnected | always
	Direction   string `json:"direction"`   // pull | push | both
}

// CookieCloudSyncReport 是一次同步的结果汇总。
type CookieCloudSyncReport struct {
	Updated   int       `json:"updated"`
	Added     int       `json:"added"`
	Skipped   int       `json:"skipped"`
	Pushed    int       `json:"pushed"` // 推送到云端的站点 host 数
	Errors    []string  `json:"errors,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}

// CookieCloudChain 编排 CookieCloud 同步：拉取加密 payload → 解密 → 按 eTLD+1 匹配本地站 →
// 仅更新已失效站的 cookie + 自动新增匹配内置定义的站。设置落 SystemConfig，密码加密。
type CookieCloudChain struct {
	store     ccStore
	crypto    *security.Crypto
	siteOp    *oper.SiteOper
	registry  *site.Registry
	siteChain *SiteChain
	hc        *httpclient.Client
}

// NewCookieCloudChain 构造。
func NewCookieCloudChain(store ccStore, c *security.Crypto, siteOp *oper.SiteOper,
	registry *site.Registry, siteChain *SiteChain, hc *httpclient.Client) *CookieCloudChain {
	return &CookieCloudChain{store: store, crypto: c, siteOp: siteOp,
		registry: registry, siteChain: siteChain, hc: hc}
}

// GetSettings 读取配置（含密文密码；handler 对外脱敏）。
func (c *CookieCloudChain) GetSettings() CookieCloudSettings {
	s := defaultCookieCloudSettings()
	if c.store == nil {
		return s
	}
	raw, err := c.store.Get(cookiecloudSettingsKey)
	if err != nil || raw == "" {
		return s
	}
	_ = json.Unmarshal([]byte(raw), &s) // 合并到默认值上，缺失字段不丢
	if s.UpdateMode == "" {
		s.UpdateMode = "disconnected"
	}
	if s.Direction == "" {
		s.Direction = "both"
	}
	return s
}

func (c *CookieCloudChain) saveSettings(s CookieCloudSettings) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return c.store.Set(cookiecloudSettingsKey, string(raw))
}

// decryptedPassword 解密密码；未配置返回空串。
func (c *CookieCloudChain) decryptedPassword(s CookieCloudSettings) (string, error) {
	if s.PasswordEnc == "" {
		return "", nil
	}
	return c.crypto.Decrypt(s.PasswordEnc)
}

// passwordConfigured 报告是否已配置密码（handler 脱敏用）。
func (c *CookieCloudChain) passwordConfigured() bool { return c.GetSettings().PasswordEnc != "" }

// SaveSettings 保存配置：密码非空则加密存，空则保留旧密文。返回保存后的配置。
func (c *CookieCloudChain) SaveSettings(in CookieCloudSettingsInput) (CookieCloudSettings, error) {
	cur := c.GetSettings()
	s := cur
	s.Enabled = in.Enabled
	s.Server = strings.TrimRight(strings.TrimSpace(in.Server), "/")
	s.UUID = strings.TrimSpace(in.UUID)
	if in.IntervalMin > 0 {
		s.IntervalMin = in.IntervalMin
	}
	if mode := normalizeCookieCloudUpdateMode(in.UpdateMode); mode != "" {
		s.UpdateMode = mode
	} else if s.UpdateMode == "" {
		s.UpdateMode = "disconnected"
	}
	if dir := normalizeCookieCloudDirection(in.Direction); dir != "" {
		s.Direction = dir
	} else if s.Direction == "" {
		s.Direction = "both"
	}
	if in.Password != "" {
		enc, err := c.crypto.Encrypt(in.Password)
		if err != nil {
			return cur, fmt.Errorf("加密密码失败: %w", err)
		}
		s.PasswordEnc = enc
	}
	if err := c.saveSettings(s); err != nil {
		return cur, err
	}
	return s, nil
}

// MaybeSync 是软间隔入口（被高频 cron tick 调）：未启用/未配置/未到间隔则跳过，否则同步。
// 间隔即时生效（每次读 DB 最新 settings，改 interval 无需重注册 cron）。
func (c *CookieCloudChain) MaybeSync(ctx context.Context) {
	s := c.GetSettings()
	if !s.Enabled || s.Server == "" || s.UUID == "" || s.PasswordEnc == "" {
		return
	}
	interval := time.Duration(s.IntervalMin) * time.Minute
	if interval <= 0 {
		interval = 2 * time.Hour
	}
	if time.Since(time.Unix(s.LastSyncUnix, 0)) < interval {
		return
	}
	report, err := c.doSync(ctx, s)
	c.recordSyncResult(s, report, err)
}

// Sync 手动强制同步（handler 用，忽略间隔）。
func (c *CookieCloudChain) Sync(ctx context.Context) (CookieCloudSyncReport, error) {
	s := c.GetSettings()
	report, err := c.doSync(ctx, s)
	c.recordSyncResult(s, report, err)
	return report, err
}

// recordSyncResult 把同步结果（时间戳/错误）回写 settings。
func (c *CookieCloudChain) recordSyncResult(s CookieCloudSettings, report CookieCloudSyncReport, err error) {
	if err != nil {
		s.LastError = err.Error()
	} else {
		s.LastError = ""
		s.LastSyncUnix = time.Now().Unix()
	}
	_ = c.saveSettings(s)
	if err != nil {
		slog.Warn("cookiecloud sync failed", "err", err)
	} else {
		slog.Info("cookiecloud sync done", "updated", report.Updated, "added", report.Added, "skipped", report.Skipped, "pushed", report.Pushed)
	}
}

// doSync 核心逻辑：按 direction 拉取 / 推送 / 双向。
//   - pull：云端 → 本地（失效站更新 + 自动新增）
//   - push：本地启用站 cookie → 合并进云端 cookie_data 后 POST /update
//   - both：先 pull 再 push（推送用最新本地态，含刚 pull 的结果）
func (c *CookieCloudChain) doSync(ctx context.Context, s CookieCloudSettings) (CookieCloudSyncReport, error) {
	if s.Server == "" || s.UUID == "" || s.PasswordEnc == "" {
		return CookieCloudSyncReport{}, errors.New("CookieCloud 未配置完整（server/uuid/password）")
	}
	password, err := c.decryptedPassword(s)
	if err != nil {
		return CookieCloudSyncReport{}, fmt.Errorf("解密密码失败: %w", err)
	}

	dir := normalizeCookieCloudDirection(s.Direction)
	if dir == "" {
		dir = "both"
	}
	report := CookieCloudSyncReport{FetchedAt: time.Now()}

	// 需要云端现有 payload 的场景：pull 必拉；push/both 也先拉以便合并，避免冲掉其它设备 host。
	var payload map[string][]cookiecloud.CookieEntry
	if dir == "pull" || dir == "push" || dir == "both" {
		p, ferr := c.fetchPayload(ctx, s, password)
		if ferr != nil {
			if dir == "pull" {
				return report, ferr
			}
			// push/both：拉取失败时从空 map 推送，至少上传本端站点
			slog.Warn("cookiecloud fetch before push failed; pushing from empty base", "err", ferr)
			payload = map[string][]cookiecloud.CookieEntry{}
			report.Errors = append(report.Errors, "拉取云端失败，将仅推送本地: "+ferr.Error())
		} else {
			payload = p
		}
	}

	if dir == "pull" || dir == "both" {
		if err := c.applyPull(ctx, s, payload, &report); err != nil {
			return report, err
		}
	}

	if dir == "push" || dir == "both" {
		pushed, perr := c.pushLocal(ctx, s, password, payload)
		if perr != nil {
			return report, perr
		}
		report.Pushed = pushed
	}
	return report, nil
}

// fetchPayload GET /get/{uuid} 并解密。
func (c *CookieCloudChain) fetchPayload(ctx context.Context, s CookieCloudSettings, password string) (map[string][]cookiecloud.CookieEntry, error) {
	// server 是管理员在设置页配的可信 URL（常含内网自建 CookieCloud），ssrf=false 放行内网。
	fetchURL := strings.TrimRight(s.Server, "/") + "/get/" + url.PathEscape(s.UUID)
	resp, err := c.hc.Get(ctx, fetchURL, "", false)
	if err != nil {
		return nil, fmt.Errorf("拉取 CookieCloud 失败: %w", err)
	}
	payload, err := cookiecloud.DecodeEnvelope(resp.Body, s.UUID, password)
	if err != nil {
		return nil, fmt.Errorf("解密 CookieCloud 数据失败: %w", err)
	}
	if payload == nil {
		payload = map[string][]cookiecloud.CookieEntry{}
	}
	return payload, nil
}

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

// pushLocal 把本地启用站 cookie 合并进 base payload 后 POST /update。
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
