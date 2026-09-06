// Package hxcard 把 PTPatronus 聚合的站点数据以「PT Patronus 守护神」名片推送到好学，
// 对应好学侧 public/api/ptlife/receive.php（cookie 身份 + HMAC-SHA256 请求体签名双重鉴权）。
package hxcard

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"gorm.io/gorm"
)

// payload 为 receive.php 约定的请求体（站点明细 + 聚合；服务端会重算聚合兜底）。
type payload struct {
	ClientVersion string  `json:"client_version"`
	Totals        totals  `json:"totals"`
	Sites         []sites `json:"sites"`
}

type totals struct {
	Uploaded      int64   `json:"uploaded"`
	Downloaded    int64   `json:"downloaded"`
	SeedingSize   int64   `json:"seeding_size"`
	Bonus         float64 `json:"bonus"`
	TrueUploaded  int64   `json:"true_uploaded,omitempty"`
	TrueDownloaed int64   `json:"true_downloaded,omitempty"`
}

type sites struct {
	Site        string  `json:"site"`
	Uploaded    int64   `json:"uploaded"`
	Downloaded  int64   `json:"downloaded"`
	SeedingSize int64   `json:"seeding_size"`
	Bonus       float64 `json:"bonus"`
	Seeding     int     `json:"seeding,omitempty"`
	Ratio       float64 `json:"ratio"`
	Level       string  `json:"level,omitempty"`
	UpdatedAt   string  `json:"updated_at,omitempty"`
}

// Pusher 推送客户端。零值不可用，经 New 构造。
type Pusher struct {
	hc         posterClient
	cfgOp      *oper.SystemConfigOper
	siteOp     *oper.SiteOper
	crypto     *security.Crypto
	version    string
	userCookie func() (string, error)
	db         *gorm.DB
}

// posterClient 对齐 httpclient.Client.Do 的最小接口（共享站点抓取的限流/UA 栈）。
type posterClient interface {
	Do(ctx context.Context, method, rawURL string, body []byte,
		headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error)
}

// New 构造推送器。任一依赖为 nil 时 Push/Collect 返回明确错误。
func New(hc posterClient, cfgOp *oper.SystemConfigOper, siteOp *oper.SiteOper,
	crypto *security.Crypto, version string, userCookie func() (string, error), db *gorm.DB) *Pusher {
	return &Pusher{hc: hc, cfgOp: cfgOp, siteOp: siteOp, crypto: crypto, version: version, userCookie: userCookie, db: db}
}

// 配置键（system_configs 表，前端设置页读写同键）。
const (
	cfgKeyEnabled = "hxcard.enabled"
	cfgKeySecret  = "hxcard.secret"
	cfgKeyURL     = "hxcard.endpoint"
)

// DefaultEndpoint 好学侧接收端点。
const DefaultEndpoint = "https://www.hxpt.org/api/ptlife/receive.php"

// Settings 返回当前配置（secret 掩码显示）。
type Settings struct {
	Enabled  bool   `json:"enabled"`
	Endpoint string `json:"endpoint"`
	Secret   string `json:"secret"` // 掩码；空=未配置
}

// GetSettings 读配置供前端展示。
func (p *Pusher) GetSettings() Settings {
	s := Settings{Endpoint: DefaultEndpoint}
	if p == nil || p.cfgOp == nil {
		return s
	}
	if v, _ := p.cfgOp.Get(cfgKeyEnabled); v == "1" {
		s.Enabled = true
	}
	if v, _ := p.cfgOp.Get(cfgKeyURL); v != "" {
		s.Endpoint = v
	}
	if v, _ := p.cfgOp.Get(cfgKeySecret); v != "" {
		s.Secret = mask(v)
	}
	return s
}

// SetSettings 更新配置。secret 为空保持不变，"-" 清除；endpoint 为空回默认。
func (p *Pusher) SetSettings(enabled bool, endpoint, secret string) error {
	if p == nil || p.cfgOp == nil {
		return fmt.Errorf("配置存储未就绪")
	}
	if err := p.cfgOp.Set(cfgKeyEnabled, boolStr(enabled)); err != nil {
		return err
	}
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	if err := p.cfgOp.Set(cfgKeyURL, strings.TrimRight(endpoint, "/")); err != nil {
		return err
	}
	if secret == "-" {
		return p.cfgOp.Set(cfgKeySecret, "")
	}
	if secret != "" {
		return p.cfgOp.Set(cfgKeySecret, secret)
	}
	return nil
}

// Collect 汇总当前各站最新统计数据（含站点行 + 聚合）。
// level/snap 时间取 site_stats 最新一条；站点行为空说明从未成功快照。
func (p *Pusher) Collect() (*payload, error) {
	if p == nil || p.siteOp == nil || p.db == nil {
		return nil, fmt.Errorf("站点数据未就绪")
	}
	active, err := p.siteOp.ListActive()
	if err != nil {
		return nil, err
	}
	pl := &payload{Sites: make([]sites, 0, len(active))}
	for i := range active {
		st := &active[i]
		var stat models.SiteStat
		if err := p.db.Where("site_id = ?", st.ID).Order("snap_at DESC").First(&stat).Error; err != nil {
			continue // 从未成功抓取的站不进名片
		}
		pl.Sites = append(pl.Sites, sites{
			Site:        st.Name,
			Uploaded:    stat.Uploaded,
			Downloaded:  stat.Downloaded,
			SeedingSize: stat.SeedingSize,
			Bonus:       stat.Bonus,
			Seeding:     stat.Seeding,
			Ratio:       stat.Ratio,
			Level:       stat.Level,
			UpdatedAt:   stat.SnapAt.Format(time.DateTime),
		})
		pl.Totals.Uploaded += stat.Uploaded
		pl.Totals.Downloaded += stat.Downloaded
		pl.Totals.SeedingSize += stat.SeedingSize
		pl.Totals.Bonus += stat.Bonus
		pl.Totals.TrueUploaded += stat.TrueUploaded
		pl.Totals.TrueDownloaed += stat.TrueDownloaded
	}
	pl.ClientVersion = p.version
	return pl, nil
}

// Push 立即推送一次：聚合 → 签名 → POST。需要已配置 secret 且当前用户有好学 cookie。
func (p *Pusher) Push(ctx context.Context) error {
	if p == nil || p.cfgOp == nil || p.crypto == nil {
		return fmt.Errorf("推送组件未就绪")
	}
	secret, _ := p.cfgOp.Get(cfgKeySecret)
	if secret == "" {
		return fmt.Errorf("未配置推送密钥，请在设置中填写好学下发的 PT Patronus 密钥")
	}
	endpoint, _ := p.cfgOp.Get(cfgKeyURL)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	pl, err := p.Collect()
	if err != nil {
		return err
	}
	if len(pl.Sites) == 0 {
		return fmt.Errorf("暂无站点数据可推送（先完成一次站点快照）")
	}
	// 当前登录用户的好学 cookie（AES-GCM 加密存 user 表）。与 VIP 校验同样需要
	// c_secure_uid：好学 logincookie() 只下发 c_secure_pass，而 receive.php 侧
	// 直接读 $_COOKIE["c_secure_uid"]，故复用 auth 包的补齐逻辑。
	if p.userCookie == nil {
		return fmt.Errorf("未找到当前用户的好学登录凭证")
	}
	cookie, err := p.userCookie()
	if err != nil || cookie == "" {
		return fmt.Errorf("好学 cookie 不可用（%v），请重新登录好学", err)
	}
	cookie = auth.EnsureVerifyCookie(cookie)

	body, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	sign := hmacSign(body, secret)
	resp, err := p.hc.Do(ctx, http.MethodPost, endpoint, body, map[string]string{
		"Content-Type":  "application/json",
		"X-PTLife-Sign": sign,
	}, cookie, false)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	slog.Info("hxcard push response", "status", resp.StatusCode, "body", string(resp.Body))
	var out struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return fmt.Errorf("响应解析失败(HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK || !out.Success {
		if out.Reason == "RATE_LIMITED" {
			return fmt.Errorf("好学侧限流（每小时 60 次），请稍后再试")
		}
		return fmt.Errorf("好学拒绝(%s): %s", out.Reason, out.Message)
	}
	slog.Info("hxcard pushed", "sites", len(pl.Sites), "endpoint", endpoint)
	return nil
}

func hmacSign(body []byte, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func mask(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
