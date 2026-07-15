package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/cookiecloud"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
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
	Server       string `json:"server"`        // CookieCloud 服务器 URL，如 https://cc.example.com
	UUID         string `json:"uuid"`          // 用户 UUID/key
	PasswordEnc  string `json:"password_enc"`  // AES-GCM 加密的端到端密码（security.Crypto.Encrypt）
	IntervalMin  int    `json:"interval_min"`  // 同步间隔（分钟），软间隔
	LastSyncUnix int64  `json:"last_sync_unix"`
	LastError    string `json:"last_error"`
}

// defaultCookieCloudSettings 填充合理默认值（间隔 2 小时）。
func defaultCookieCloudSettings() CookieCloudSettings {
	return CookieCloudSettings{IntervalMin: 120}
}

// CookieCloudSettingsInput 是 handler 传入的输入（密码明文，空=保留旧密码）。
type CookieCloudSettingsInput struct {
	Enabled     bool   `json:"enabled"`
	Server      string `json:"server"`
	UUID        string `json:"uuid"`
	Password    string `json:"password"` // 明文；空=保留旧密码
	IntervalMin int    `json:"interval_min"`
}

// CookieCloudSyncReport 是一次同步的结果汇总。
type CookieCloudSyncReport struct {
	Updated   int       `json:"updated"`
	Added     int       `json:"added"`
	Skipped   int       `json:"skipped"`
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
		slog.Info("cookiecloud sync done", "updated", report.Updated, "added", report.Added, "skipped", report.Skipped)
	}
}

// doSync 核心逻辑：拉取 → 匹配本地失效站更新 → 自动新增匹配定义的站。
func (c *CookieCloudChain) doSync(ctx context.Context, s CookieCloudSettings) (CookieCloudSyncReport, error) {
	if s.Server == "" || s.UUID == "" || s.PasswordEnc == "" {
		return CookieCloudSyncReport{}, errors.New("CookieCloud 未配置完整（server/uuid/password）")
	}
	password, err := c.decryptedPassword(s)
	if err != nil {
		return CookieCloudSyncReport{}, fmt.Errorf("解密密码失败: %w", err)
	}

	// 拉取：server 是管理员在设置页配的可信 URL（常含内网自建 CookieCloud 容器，如 192.168.x.x / 容器名），
	// 非「站点抓取」里用户可控 URL，故 ssrf=false 放行内网。
	fetchURL := strings.TrimRight(s.Server, "/") + "/get/" + url.PathEscape(s.UUID)
	resp, err := c.hc.Get(ctx, fetchURL, "", false)
	if err != nil {
		return CookieCloudSyncReport{}, fmt.Errorf("拉取 CookieCloud 失败: %w", err)
	}
	payload, err := cookiecloud.DecodeEnvelope(resp.Body, s.UUID, password)
	if err != nil {
		return CookieCloudSyncReport{}, fmt.Errorf("解密 CookieCloud 数据失败: %w", err)
	}

	report := CookieCloudSyncReport{FetchedAt: time.Now()}
	var mu sync.Mutex
	addErr := func(msg string) {
		mu.Lock()
		report.Errors = append(report.Errors, msg)
		mu.Unlock()
	}

	// 本地启用站点 + 已占用域名集合（用于判断 CookieCloud 里哪些 host 是新站）。
	sites, err := c.siteOp.ListActive()
	if err != nil {
		return report, fmt.Errorf("读取本地站点失败: %w", err)
	}
	occupied := map[string]bool{}
	for i := range sites {
		occupied[site.RegisteredDomain(sites[i].BaseURL)] = true
	}

	// 并发处理本地站点：仅更新已失效站（TestConnection 失败才覆盖），仍有效则跳过。
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for i := range sites {
		st := &sites[i]
		cookieStr := cookieForSite(payload, st.BaseURL)
		if cookieStr == "" {
			// CookieCloud 里没有该站的 cookie：跳过（不置 occupied 之外的动作）。
			mu.Lock()
			report.Skipped++
			mu.Unlock()
			continue
		}
		wg.Add(1)
		go func(st *models.Site, cookieStr string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if c.isConnected(ctx, st) {
				// 当前 cookie 仍有效：不动手动维护的有效 cookie（对齐 MoviePilot）。
				mu.Lock()
				report.Skipped++
				mu.Unlock()
				return
			}
			if err := c.siteOp.UpdateCookie(st.ID, cookieStr); err != nil {
				addErr(fmt.Sprintf("%s: 更新失败 %v", st.Name, err))
				return
			}
			mu.Lock()
			report.Updated++
			mu.Unlock()
		}(st, cookieStr)
	}
	wg.Wait()

	// 自动新增：CookieCloud 里匹配不到任何本地站的 host → 查内置定义 → 找到则创建（带 cookie）。
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
			continue // 非内置 286 站之一，不自动新增（范围边界）
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
	return report, nil
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
func cookieForSite(payload map[string][]cookiecloud.CookieEntry, siteBaseURL string) string {
	siteDom := site.RegisteredDomain(siteBaseURL)
	if siteDom == "" {
		return ""
	}
	var parts []string
	for h, cookies := range payload {
		if site.RegisteredDomain(h) != siteDom {
			continue
		}
		for _, ck := range cookies {
			if ck.Name != "" {
				parts = append(parts, ck.Name+"="+ck.Value)
			}
		}
	}
	return strings.Join(parts, "; ")
}

// findDefinitionByHost 在内置站点定义里按注册域匹配 host，返回首个命中者（用于自动新增）。
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
