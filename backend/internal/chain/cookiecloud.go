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
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/errtext"
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
	Mode         string `json:"mode"`         // external=外部服务器（默认）；builtin=内置服务端（PTPatronus 自身充当）
	Server       string `json:"server"`       // external 模式：CookieCloud 服务器 URL；builtin 模式忽略
	UUID         string `json:"uuid"`         // 用户 UUID/key（builtin 模式自动生成）
	PasswordEnc  string `json:"password_enc"` // AES-GCM 加密的端到端密码（security.Crypto.Encrypt）
	IntervalMin  int    `json:"interval_min"` // 同步间隔（分钟），软间隔
	UpdateMode   string `json:"update_mode"`  // disconnected=仅失效站（默认）；always=总是用云端覆盖同名 cookie
	Direction    string `json:"direction"`    // pull=仅拉取；push=仅推送；both=双向（默认）
	LastSyncUnix int64  `json:"last_sync_unix"`
	LastError    string `json:"last_error"`
}

// CookieCloudMode 常量。
const (
	CookieCloudModeExternal = "external"
	CookieCloudModeBuiltin  = "builtin"
)

// normalizeCookieCloudMode 归一化模式（空=external）。
func normalizeCookieCloudMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), CookieCloudModeBuiltin) {
		return CookieCloudModeBuiltin
	}
	return CookieCloudModeExternal
}

// defaultCookieCloudSettings 填充合理默认值（间隔 2 小时）。
func defaultCookieCloudSettings() CookieCloudSettings {
	return CookieCloudSettings{IntervalMin: 120, UpdateMode: "disconnected", Direction: "both"}
}

// CookieCloudSettingsInput 是 handler 传入的输入（密码明文，空=保留旧密码）。
type CookieCloudSettingsInput struct {
	Enabled     bool   `json:"enabled"`
	Mode        string `json:"mode"`       // external | builtin
	Server      string `json:"server"`     // external 模式用
	UUID        string `json:"uuid"`       // builtin 模式留空=自动生成；也可显式指定
	Password    string `json:"password"`   // 明文；空=保留旧密码（builtin 首次保存自动生成）
	Regenerate  bool   `json:"regenerate"` // builtin 模式：重新生成 UUID+密码（旧数据作废）
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
// builtin 模式下本链同时充当服务端（srv store + ServerUpdate 推送入口）。
type CookieCloudChain struct {
	store     ccStore
	crypto    *security.Crypto
	siteOp    *oper.SiteOper
	registry  *site.Registry
	siteChain *SiteChain
	hc        *httpclient.Client

	srvMu sync.Mutex
	srv   cookiecloud.ServerStore
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
// builtin 模式：UUID/密码留空时自动生成（返回值含新 UUID；密码不回传明文，
// 由 handler 在响应里一次性给前端展示）。
func (c *CookieCloudChain) SaveSettings(in CookieCloudSettingsInput) (CookieCloudSettings, error) {
	cur := c.GetSettings()
	s := cur
	s.Enabled = in.Enabled
	s.Mode = normalizeCookieCloudMode(in.Mode)
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
	// builtin：自动生成缺失凭证；regenerate 强制重生成（旧推送数据作废——uuid 变了，
	// 旧 envelope 不再匹配，天然隔离，无需显式清 store）。
	if s.Mode == CookieCloudModeBuiltin && (in.Regenerate || s.UUID == "" || s.PasswordEnc == "") {
		if in.Regenerate || s.UUID == "" {
			s.UUID = genCookieCloudToken(22)
		}
		if in.Regenerate || s.PasswordEnc == "" {
			enc, err := c.crypto.Encrypt(genCookieCloudToken(22))
			if err != nil {
				return cur, fmt.Errorf("生成密码失败: %w", err)
			}
			s.PasswordEnc = enc
		}
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
	// builtin 模式直读内置 store，无需外部 server 地址；external 模式才要求 server/uuid/password 齐全。
	if !s.Enabled || s.UUID == "" || s.PasswordEnc == "" {
		return
	}
	if normalizeCookieCloudMode(s.Mode) != CookieCloudModeBuiltin && s.Server == "" {
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
		s.LastError = errtext.Text(err)
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
	builtin := normalizeCookieCloudMode(s.Mode) == CookieCloudModeBuiltin
	if builtin {
		if s.UUID == "" || s.PasswordEnc == "" {
			return CookieCloudSyncReport{}, errors.New("内置服务端未生成凭证（保存配置自动生成）")
		}
	} else if s.Server == "" || s.UUID == "" || s.PasswordEnc == "" {
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
			if dir == "pull" && !builtin {
				return report, ferr
			}
			if dir == "pull" && builtin {
				// builtin：store 无数据=浏览器还没推过，不算错误，报空报告
				slog.Info("cookiecloud builtin store empty, skip pull", "err", ferr)
			} else if errors.Is(ferr, cookiecloud.ErrEmptyPayload) {
				// 云端该 UUID 还没数据（首次推送的常态）→ 从空 base 推。
				payload = map[string][]cookiecloud.CookieEntry{}
			} else {
				// 拉取真失败（网络抖动、密码不匹配、服务端 5xx…）：此时云端有什么我们完全不知道，
				// 而 CookieCloud 的 PUT 是**整体替换**——从空 base 推送会把云端已有的其它
				// host 全部抹掉（别的设备/浏览器要重新推一遍）。本轮放弃推送，报错误下轮再试。
				slog.Warn("cookiecloud fetch failed; skip push to avoid overwriting unknown cloud data", "err", ferr)
				return report, fmt.Errorf("拉取云端失败，已跳过本次推送以免覆盖云端数据: %w", ferr)
			}
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

// fetchPayload 拉取并解密。builtin 模式直读内置 store（无数据时返回「未推送」错误
// 供调用方区分）；external 模式 GET /get/{uuid}。
func (c *CookieCloudChain) fetchPayload(ctx context.Context, s CookieCloudSettings, password string) (map[string][]cookiecloud.CookieEntry, error) {
	if normalizeCookieCloudMode(s.Mode) == CookieCloudModeBuiltin {
		env, ok := c.serverStore().Get(s.UUID)
		if !ok {
			return nil, fmt.Errorf("内置服务端暂无数据（浏览器扩展推送后自动同步）: %w", cookiecloud.ErrEmptyPayload)
		}
		body, err := cookiecloud.MarshalEnvelope(env)
		if err != nil {
			return nil, err
		}
		payload, err := cookiecloud.DecodeEnvelope(body, s.UUID, password)
		if err != nil {
			return nil, fmt.Errorf("解密 CookieCloud 数据失败: %w", err)
		}
		if payload == nil {
			payload = map[string][]cookiecloud.CookieEntry{}
		}
		return payload, nil
	}
	// server 是管理员在设置页配的可信 URL（常含内网自建 CookieCloud），ssrf=false 放行内网。
	// 带 crypto_type=aes-128-cbc-fixed：若服务端支持该参数会按 fixed-IV 格式返回密文；
	// 不支持的服务端（老版本）忽略参数返回 legacy 密文，解密侧按前缀自动识别两种格式。
	fetchURL := strings.TrimRight(s.Server, "/") + "/get/" + url.PathEscape(s.UUID) + "?crypto_type=aes-128-cbc-fixed"
	resp, err := c.hc.Get(ctx, fetchURL, "", false)
	if err != nil {
		return nil, fmt.Errorf("拉取 CookieCloud 失败: %w", err)
	}
	// 非 2xx（5xx/网关错误页等）不能当成「云端没数据」：那样会走进「从空 base 推送」
	// 分支，把云端其它 host 抹掉。状态异常一律按拉取失败处理，由调用方跳过本次推送。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("拉取 CookieCloud 失败: HTTP %d", resp.StatusCode)
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
