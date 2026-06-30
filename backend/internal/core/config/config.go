// Package config loads application configuration from the environment with
// sensible defaults. Runtime-mutable settings live in the DB (SystemConfig table).
package config

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config holds all process-level configuration.
type Config struct {
	AppName  string
	Env      string // debug | release
	HTTPAddr string
	DataDir  string

	// Security
	SecretKey        string // JWT signing + AES key derivation seed
	ActivationSecret string // HMAC secret for activation codes

	// Database
	DBType string // sqlite | postgres
	DSN    string // used when DBType=postgres; sqlite path derived from DataDir otherwise

	// JWT
	JWTExpireMin int

	// 好学 APP 专属 VIP 准入（AUTH_SITE 机制）
	AuthSiteEnabled bool
	AuthSiteURL     string // e.g. https://hxpt.org
	AuthSiteCookie  string // bootstrap cookie for VIP verification (movable to DB later)
	AuthVIPLevel    int    // minimum VIP level required
	VIPGraceDays    int    // VIP 宽限天数（好学不可达时缓存 VIP 的有效期），默认 3

	// 通知通道（首启 env 引导默认；运行时以 SystemConfig「设置→通知」为准，UI 热改）
	BarkServer string
	BarkKey    string
	TGToken    string
	TGChatID   string
	TGAPIURL   string // Telegram Bot API 自定义域名（国内反代）；空→官方
	// QQ 开放平台机器人（OAuth2：appId/appSecret 换 token，主动消息推 openid/group_openid）
	QQAppID       string
	QQAppSecret   string
	QQOpenID      string
	QQGroupOpenID string
	// 微信 ClawBot（iLink Bot，逆向协议；bot_token/account_id 需扫码登录获取）
	WXClawBotToken    string
	WXClawBotAccountID string
	WXClawBotTarget    string // 接收通知的微信号 wxid
	WXClawBotBaseURL   string // 穙→官方 ilinkai.weixin.qq.com
	WebhookURL string

	// 自动化
	StatsSyncCron           string // 统计同步 cron，默认每 30 分钟
	CookieKeepAliveCron     string // cookie 保活 cron，默认每 4 小时（防 cookie 过期致自动化全失效）
	RegistrationCron        string // 注册开放检测 cron，默认每 6 小时
	HRWarnDeadlineHours     int    // H&R 临期告警阈值（剩余小时），默认 24
	CheckinRecordRetainDays int    // 签到历史保留天数，默认 90（超期自动清理，控制表体积）

	// 站点信息聚合
	FreeleechCron    string // 全站免费/促销检测 cron，默认每 30 分钟
	AnnouncementCron string // 站点公告抓取 cron，默认每 60 分钟
	SaveptCron       string // PT监护室 savept.icu 刷新 cron，默认每 2 小时
	AssessmentCron   string // 站点考核刷新 cron，默认每 60 分钟
	MedalCron        string // 勋章墙刷新 cron（价格/上架/售罄 diff → 通知），默认每 60 分钟
	KeepSeedCron     string // IYUU 定时自动辅种(仅扫启用 AutoReseed 的下载器) cron，默认每天 3 点
	MessagesCron     string // 站点私信新邮件轮询 cron（通知中心「新邮件」来源），默认每 30 分钟
	SubscribeCron    string // 媒体订阅自动搜索+下载 cron，默认每 30 分钟
	RssCheckCron     string // RSS 订阅全局扫描 cron，默认每 5 分钟（按各订阅 interval_min 决定是否实际抓取）

	// 媒体元数据源
	TMDBAPIKey    string
	TMDBAPIDomain string // TMDB API 域名(TMDB_API_DOMAIN)，默认 api.tmdb.org（官方 api.themoviedb.org 国内常被墙，短域名可达）
	DoubanSecret  string // Frodo HMAC 签名密钥(DOUBAN_SECRET)；空则用公开默认。非空→签名为主、微信免签兜底
	DoubanAppKey  string
	BangumiToken  string // bgm.tv 可选 Bearer token（提升速率限制；空走匿名配额）
	XingguangjiBase string // 星光集（短剧）站点根，默认 https://dj.hxpt.org；可指向本地实例联调

	// 安全 / 限流
	CORSAllowedOrigins string // 逗号分隔；"*" 或空=开发全开，生产应收敛为具体域名
	APIKey             string // 程序化访问密钥（X-API-Key 头）；为空禁用
	TrustedProxies     string // 逗号分隔 CIDR/IP；空=不信任任何代理（ClientIP 取直连对端），防 XFF 伪造绕过限流
	LoginRatePerMin    int    // 登录/激活端点每 IP 每分钟上限，默认 10
	GlobalRatePerSec   int    // 全局每 IP 每秒上限，默认 20

	// 在线更新
	UpdateManifestURL string // 官方版本清单 URL(gist raw 等)；空=只读自托管清单。客户端经本后端中转检查，不直连官方源
}

// Load reads configuration from the environment.
func Load() (*Config, error) {
	cfg := &Config{
		AppName:                 getenv("APP_NAME", "PTPatronus"),
		Env:                     getenv("APP_ENV", "debug"),
		HTTPAddr:                getenv("HTTP_ADDR", ":8088"),
		DataDir:                 getenv("DATA_DIR", "./data"),
		SecretKey:               getenv("SECRET_KEY", ""),
		ActivationSecret:        getenv("ACTIVATION_SECRET", ""),
		DBType:                  strings.ToLower(getenv("DB_TYPE", "sqlite")),
		DSN:                     getenv("DATABASE_URL", ""),
		JWTExpireMin:            getenvInt("JWT_EXPIRE_MIN", 1440),
		AuthSiteEnabled:         getenvBool("AUTH_SITE_ENABLED", false),
		AuthSiteURL:             getenv("AUTH_SITE_URL", "https://www.hxpt.org"),
		AuthSiteCookie:          getenv("AUTH_SITE_COOKIE", ""),
		AuthVIPLevel:            getenvInt("AUTH_VIP_LEVEL", 1),
		VIPGraceDays:            getenvInt("VIP_GRACE_DAYS", 3),
		BarkServer:              getenv("BARK_SERVER", ""),
		BarkKey:                 getenv("BARK_KEY", ""),
		TGToken:                 getenv("TELEGRAM_TOKEN", ""),
		TGChatID:                getenv("TELEGRAM_CHAT_ID", ""),
		TGAPIURL:                getenv("TELEGRAM_API_URL", ""),
		QQAppID:                 getenv("QQ_APP_ID", ""),
		QQAppSecret:             getenv("QQ_APP_SECRET", ""),
		QQOpenID:                getenv("QQ_OPENID", ""),
		QQGroupOpenID:           getenv("QQ_GROUP_OPENID", ""),
		WXClawBotToken:          getenv("WECHAT_CLAWBOT_TOKEN", ""),
		WXClawBotAccountID:      getenv("WECHAT_CLAWBOT_ACCOUNT_ID", ""),
		WXClawBotTarget:         getenv("WECHAT_CLAWBOT_TARGET", ""),
		WXClawBotBaseURL:        getenv("WECHAT_CLAWBOT_BASE_URL", ""),
		WebhookURL:              getenv("WEBHOOK_URL", ""),
		StatsSyncCron:           getenv("STATS_SYNC_CRON", "*/30 * * * *"),
		CookieKeepAliveCron:     getenv("COOKIE_KEEPALIVE_CRON", "0 */4 * * *"),
		RegistrationCron:        getenv("REGISTRATION_CRON", "0 */6 * * *"),
		HRWarnDeadlineHours:     getenvInt("HR_WARN_DEADLINE_HOURS", 24),
		CheckinRecordRetainDays: getenvInt("CHECKIN_RECORD_RETAIN_DAYS", 90),
		FreeleechCron:           getenv("FREELEECH_CRON", "*/30 * * * *"),
		AnnouncementCron:        getenv("ANNOUNCEMENT_CRON", "0 * * * *"),
		SaveptCron:              getenv("SAVEPT_CRON", "0 */2 * * *"),
		AssessmentCron:          getenv("ASSESSMENT_CRON", "0 * * * *"),
		MedalCron:               getenv("MEDAL_CRON", "0 * * * *"),
		KeepSeedCron:            getenv("KEEPSEED_CRON", "0 3 * * *"),
		MessagesCron:            getenv("MESSAGES_CRON", "*/30 * * * *"),
		SubscribeCron:           getenv("SUBSCRIBE_CRON", "*/30 * * * *"),
		RssCheckCron:            getenv("RSS_CHECK_CRON", "*/5 * * * *"),
		// TMDB 默认带 MoviePilot 同款公开 key（TMDB API key 本就客户端可见、可公开），
		// 让影视搜索开箱即用；仍可用 TMDB_API_KEY 覆盖。
		TMDBAPIKey:         getenv("TMDB_API_KEY", "db55323b8d3e4154498498a75642b381"),
		TMDBAPIDomain:      getenv("TMDB_API_DOMAIN", "api.tmdb.org"),
		DoubanSecret:       getenv("DOUBAN_SECRET", ""),
		DoubanAppKey:       getenv("DOUBAN_API_KEY", ""),
		BangumiToken:       getenv("BANGUMI_TOKEN", ""),
		XingguangjiBase:    getenv("XINGGUANGJI_BASE", "https://dj.hxpt.org"),
		CORSAllowedOrigins: getenv("CORS_ALLOWED_ORIGINS", ""),
		APIKey:             getenv("API_KEY", ""),
		TrustedProxies:     getenv("TRUSTED_PROXIES", ""),
		LoginRatePerMin:    getenvInt("LOGIN_RATE_PER_MIN", 10),
		GlobalRatePerSec:   getenvInt("GLOBAL_RATE_PER_SEC", 20),
		UpdateManifestURL:  getenv("UPDATE_MANIFEST_URL", ""),
	}
	if err := cfg.validateSecrets(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validateSecrets 强制 release 模式下密钥非空且非公开默认值（否则任何人都能用源码里
// 的默认 key 离线伪造管理员 JWT，并解密库里全部密文 = 免认证完全接管）；debug 模式
// 回退到开发默认并显著告警。
func (cfg *Config) validateSecrets() error {
	if !cfg.IsRelease() {
		if cfg.SecretKey == "" {
			slog.Warn("SECRET_KEY empty in debug mode, using insecure dev default — never use in production")
			cfg.SecretKey = "dev-insecure-secret-change-me"
		}
		return nil
	}
	switch {
	case cfg.SecretKey == "", cfg.SecretKey == "dev-insecure-secret-change-me":
		return errors.New("SECRET_KEY must be set in release mode (generate: openssl rand -base64 32)")
	case len(cfg.SecretKey) < 32:
		slog.Warn("SECRET_KEY shorter than 32 bytes in release mode; rotate to a strong random key")
	}
	if cfg.ActivationSecret == "" || cfg.ActivationSecret == "activation-insecure-secret" {
		return errors.New("ACTIVATION_SECRET must be set in release mode")
	}
	return nil
}

// IsRelease reports whether the app runs in release mode.
func (c *Config) IsRelease() bool { return c.Env == "release" }

// SQLitePath returns the SQLite database file path under the data directory.
func (c *Config) SQLitePath() string {
	if c.DataDir == "" {
		return "ptpatronus.db"
	}
	return strings.TrimRight(c.DataDir, "/\\") + string(os.PathSeparator) + "ptpatronus.db"
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
