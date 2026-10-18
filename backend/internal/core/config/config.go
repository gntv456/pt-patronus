// Package config loads application configuration from the environment with
// sensible defaults. Runtime-mutable settings live in the DB (SystemConfig table).
package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds all process-level configuration.
type Config struct {
	AppName                  string
	Env                      string // debug | release
	HTTPAddr                 string
	DataDir                  string
	PluginDevWatch           bool
	PluginDevWatchIntervalMS int

	// AppBaseURL 本服务对外访问基址（含协议+主机+端口；含路径前缀亦可）。
	// strm 内容渲染为该基址下的绝对 URL——Emby 把 strm 内容直接交给播放器，
	// 相对路径无法解析。空=保留相对路径（同机播放器场景）。
	AppBaseURL string

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
	AuthSiteURL     string // e.g. https://www.hxpt.org
	AuthSiteCookie  string // bootstrap cookie for VIP verification (movable to DB later)
	AuthVIPLevel    int    // minimum VIP level required
	VIPGraceDays    int    // VIP 宽限天数（好学不可达时缓存 VIP 的有效期），默认 3
	DemoMode        bool   // 显式开启演示登录（默认关闭，仅用于本地评审/公开演示）
	// AllowPrivateSites 放行把「站点」建在内网/环回地址（默认关闭，站点抓取保持 SSRF 守卫）。
	// 场景：自建站点（如本机 Docker 起的 FluxTorrent）、NAS/局域网 tracker——站长侧是常态需求。
	// 开启后站点抓取客户端不再拦 127.0.0.1/10.x/192.168.x 等目标。
	AllowPrivateSites bool

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
	WXClawBotToken     string
	WXClawBotAccountID string
	WXClawBotTarget    string // 接收通知的微信号 wxid
	WXClawBotBaseURL   string // 穙→官方 ilinkai.weixin.qq.com
	WebhookURL         string

	// 自动化
	StatsSyncCron           string // 统计同步 cron，默认每 30 分钟
	CookieKeepAliveCron     string // cookie 保活 cron，默认每小时（13 * * * *，防 Laravel qw_session 等 session 在两次保活间过期致自动化全失效）
	RegistrationCron        string // 注册开放检测 cron，默认每 6 小时
	HRWarnDeadlineHours     int    // H&R 临期告警阈值（剩余小时），默认 24
	CheckinRecordRetainDays int    // 签到历史保留天数，默认 90（超期自动清理，控制表体积）

	// 站点信息聚合
	FreeleechCron     string // 全站免费/促销检测 cron，默认每 30 分钟
	AnnouncementCron  string // 站点公告抓取 cron，默认每 60 分钟
	SaveptCron        string // PT监护室 savept.icu 刷新 cron，默认每 2 小时
	AssessmentCron    string // 站点考核刷新 cron；临时止血默认每天3点(原每小时裸访 userdetails 致52movie警告)，块B后并入 site-snapshot
	MedalCron         string // 勋章墙刷新 cron（价格/上架/售罄 diff → 通知），默认每 12 小时（对齐其 24h 内存缓存）
	KeepSeedCron      string // IYUU 定时自动辅种(仅扫启用 AutoReseed 的下载器) cron，默认每天 3 点
	MessagesCron      string // 站点私信新邮件轮询 cron（通知中心「新邮件」来源），默认每 30 分钟
	SubscribeCron     string // 媒体订阅自动搜索+下载 cron，默认每 30 分钟
	CloudStrmCron     string // 云盘 STRM 生成 + 订阅追更 cron，默认每 6 小时
	RssCheckCron      string // RSS 订阅全局扫描 cron，默认每 5 分钟（按各订阅 interval_min 决定是否实际抓取）
	TransferSweepCron string // 媒体整理 cron（兜底/重试：已完成种子→识别→重命名→硬链→刮削），默认每 10 分钟
	SnapshotCron      string // 聚合站点快照 cron（合并 stats/hr/free/ann/msg/keepalive/assessment 7 任务为一次串行抓取），默认 7 */4 * * *
	BackupCron        string // 定时备份 cron（BACKUP_CRON）；空=不启用。须与 BackupDir 同时配置才生效
	BackupDir         string // 定时备份输出目录（BACKUP_DIR）；备份含明文密钥，文件 0600，仅保留最近 10 份

	// 媒体元数据源
	TMDBAPIKey     string
	TMDBAPIDomain  string // TMDB API 域名(TMDB_API_DOMAIN)，默认 api.tmdb.org（官方 api.themoviedb.org 国内常被墙，短域名可达）
	DoubanSecret   string // Frodo HMAC 签名密钥(DOUBAN_SECRET)；空则用公开默认。非空→签名为主、微信免签兜底
	DoubanAppKey   string
	BangumiToken   string // bgm.tv 可选 Bearer token（提升速率限制；空走匿名配额）
	CunguangjiBase string // 寸光集（短剧）站点根，默认 https://dj.hxpt.org；可指向本地实例联调
	OMDbAPIKey     string // OMDb( IMDb 评分源 ) API Key(OMDB_API_KEY)；空则跳过 IMDb 聚合

	// 字幕源（env 提供 SystemConfig 默认；UI 改动运行时覆盖）
	AssrtToken       string // assrt.net API token
	OpenSubtitlesKey string // opensubtitles.com Api-Key

	// 安全 / 限流
	CORSAllowedOrigins string // 逗号分隔；"*" 或空=开发全开，生产应收敛为具体域名
	APIKey             string // 程序化访问密钥（X-API-Key 头）；为空禁用
	MetricsToken       string // /metrics 抓取令牌（METRICS_TOKEN，缺省回退 API_KEY）；release 下未配置则端点禁用
	TrustedProxies     string // 逗号分隔 CIDR/IP；空=不信任任何代理（ClientIP 取直连对端），防 XFF 伪造绕过限流
	LoginRatePerMin    int    // 登录/激活端点每 IP 每分钟上限，默认 10
	GlobalRatePerSec   int    // 全局每 IP 每秒上限，默认 20

	// 在线更新
	UpdateManifestURL    string // 官方版本清单 URL(gist raw 等)；空=只读自托管清单。客户端经本后端中转检查，不直连官方源
	UpdateManifestPubKey string // 可选 ed25519 公钥(base64)；配置后验证远端 manifest 签名

	// 安全模式（SAFE_MODE）：跳过插件/调度/监控激活，仅启动核心 HTTP + 诊断端点（出问题时自救）
	SafeMode bool

	// 娱乐游戏自动化总开关（ENTERTAINMENT_ENABLED，默认 false）：
	// 转盘/下注/红包等属站点游戏玩法的自动化，可能违反站规（封号风险由使用者自担）。
	// 默认关闭：关闸状态下整个 /entertainment 路由组不注册（404），
	// 用户在配置或环境变量显式开启才暴露——风险告知见 config.example 注释。
	EntertainmentEnabled bool
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
		DemoMode:                getenvBool("DEMO_MODE", false),
		AllowPrivateSites:       getenvBool("ALLOW_PRIVATE_SITES", false),
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
		AppBaseURL:              strings.TrimRight(getenv("APP_BASE_URL", ""), "/"),
		WebhookURL:              getenv("WEBHOOK_URL", ""),
		StatsSyncCron:           getenv("STATS_SYNC_CRON", "*/30 * * * *"),
		CookieKeepAliveCron:     getenv("COOKIE_KEEPALIVE_CRON", "13 * * * *"),
		RegistrationCron:        getenv("REGISTRATION_CRON", "0 */6 * * *"),
		HRWarnDeadlineHours:     getenvInt("HR_WARN_DEADLINE_HOURS", 24),
		CheckinRecordRetainDays: getenvInt("CHECKIN_RECORD_RETAIN_DAYS", 90),
		FreeleechCron:           getenv("FREELEECH_CRON", "*/30 * * * *"),
		AnnouncementCron:        getenv("ANNOUNCEMENT_CRON", "0 * * * *"),
		SaveptCron:              getenv("SAVEPT_CRON", "0 */2 * * *"),
		// 临时止血：原 "0 * * * *" 每整小时裸访 userdetails.php(无 uid)致 52movie 警告。
		// 块B 完成后 assessment 并入 site-snapshot，此 cron 废弃。
		AssessmentCron:    getenv("ASSESSMENT_CRON", "0 3 * * *"),
		MedalCron:         getenv("MEDAL_CRON", "37 */12 * * *"),
		KeepSeedCron:      getenv("KEEPSEED_CRON", "0 3 * * *"),
		MessagesCron:      getenv("MESSAGES_CRON", "*/30 * * * *"),
		SubscribeCron:     getenv("SUBSCRIBE_CRON", "*/30 * * * *"),
		CloudStrmCron:     getenv("CLOUD_STRM_CRON", "0 */6 * * *"),
		RssCheckCron:      getenv("RSS_CHECK_CRON", "*/5 * * * *"),
		TransferSweepCron: getenv("TRANSFER_SWEEP_CRON", "*/10 * * * *"),
		SnapshotCron:      getenv("SNAPSHOT_CRON", "7 */4 * * *"),
		BackupCron:        getenv("BACKUP_CRON", ""), // 默认关闭：自动把明文密钥写盘属敏感行为，须显式开启
		BackupDir:         getenv("BACKUP_DIR", ""),
		// TMDB 默认带 MoviePilot 同款公开 key（TMDB API key 本就客户端可见、可公开），
		// 让影视搜索开箱即用；仍可用 TMDB_API_KEY 覆盖。
		TMDBAPIKey:           getenv("TMDB_API_KEY", "db55323b8d3e4154498498a75642b381"),
		TMDBAPIDomain:        getenv("TMDB_API_DOMAIN", "api.tmdb.org"),
		DoubanSecret:         getenv("DOUBAN_SECRET", ""),
		DoubanAppKey:         getenv("DOUBAN_API_KEY", ""),
		BangumiToken:         getenv("BANGUMI_TOKEN", ""),
		CunguangjiBase:       getenvFallback("CUNGUANGJI_BASE", "XINGGUANGJI_BASE", "https://dj.hxpt.org"),
		OMDbAPIKey:           getenv("OMDB_API_KEY", ""),
		AssrtToken:           getenv("ASSRT_TOKEN", ""),
		OpenSubtitlesKey:     getenv("OPENSUBTITLES_KEY", ""),
		CORSAllowedOrigins:   getenv("CORS_ALLOWED_ORIGINS", ""),
		APIKey:               getenv("API_KEY", ""),
		MetricsToken:         getenvFallback("METRICS_TOKEN", "API_KEY", ""),
		TrustedProxies:       getenv("TRUSTED_PROXIES", ""),
		LoginRatePerMin:      getenvInt("LOGIN_RATE_PER_MIN", 10),
		GlobalRatePerSec:     getenvInt("GLOBAL_RATE_PER_SEC", 20),
		UpdateManifestURL:    getenv("UPDATE_MANIFEST_URL", ""),
		UpdateManifestPubKey: getenv("UPDATE_MANIFEST_PUBKEY", ""),
		SafeMode:             getenvBool("SAFE_MODE", false),
		EntertainmentEnabled: getenvBool("ENTERTAINMENT_ENABLED", false),
	}
	cfg.PluginDevWatch = getenvBool("PLUGIN_DEV_WATCH", cfg.Env == "debug")
	cfg.PluginDevWatchIntervalMS = getenvInt("PLUGIN_DEV_WATCH_INTERVAL_MS", 1000)
	if err := cfg.ensureSecrets(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ensureSecrets 保证 release 模式下密钥非空且非源码公开默认值。
// 未显式设环境变量时,自动生成强随机密钥并持久化到 DataDir(secret_key / activation_secret
// 文件,0600),实现"零配置安全启动"——每个部署密钥都不同(防公开默认 key 裸奔),普通用户
// 无需手填 compose 即可安全运行;显式设值永远优先(高级用户/生产可接管)。debug 模式回退
// 开发默认并显著告警。
func (cfg *Config) ensureSecrets() error {
	if !cfg.IsRelease() {
		if cfg.SecretKey == "" {
			slog.Warn("SECRET_KEY empty in debug mode, using insecure dev default — never use in production")
			cfg.SecretKey = "dev-insecure-secret-change-me"
		}
		return nil
	}
	// 源码里出现过的公开默认值必须拒绝（等于公开 = 免认证接管）。
	if cfg.SecretKey == "dev-insecure-secret-change-me" {
		return errors.New("SECRET_KEY must not be the public dev default in release mode")
	}
	if cfg.ActivationSecret == "activation-insecure-secret" {
		return errors.New("ACTIVATION_SECRET must not be the public dev default in release mode")
	}
	// 未显式设 → 自动生成 + 持久化到 DataDir（挂载持久化，重启不丢、用户不被踢）。
	if cfg.SecretKey == "" {
		k, err := cfg.loadOrGenerateSecret("secret_key", "SECRET_KEY")
		if err != nil {
			return err
		}
		cfg.SecretKey = k
	}
	if cfg.ActivationSecret == "" {
		k, err := cfg.loadOrGenerateSecret("activation_secret", "ACTIVATION_SECRET")
		if err != nil {
			return err
		}
		cfg.ActivationSecret = k
	}
	if len(cfg.SecretKey) < 32 {
		slog.Warn("SECRET_KEY shorter than 32 bytes in release mode; rotate to a strong random key")
	}
	return nil
}

// loadOrGenerateSecret 从 DataDir/<filename> 读密钥；不存在则生成 32 字节随机 base64 写入（0600）。
// 每个部署首次启动自动生成不同随机密钥，持久化供后续复用（重启不变、JWT 不失效）。
func (cfg *Config) loadOrGenerateSecret(filename, envVar string) (string, error) {
	path := filepath.Join(cfg.DataDir, filename)
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, nil // 复用已持久化的密钥
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	s := base64.StdEncoding.EncodeToString(raw)
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", cfg.DataDir, err)
	}
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w (检查 DataDir 是否可写/已挂载)", path, err)
	}
	slog.Info("auto-generated secret (未显式设环境变量,已自动生成并持久化)",
		"file", path, "hint", "如需接管:设 "+envVar+" 环境变量并删除该文件")
	return s, nil
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

// getenvFallback 读 primary；空则读 secondary；都空用 def。
func getenvFallback(primary, secondary, def string) string {
	if v := getenv(primary, ""); v != "" {
		return v
	}
	if v := getenv(secondary, ""); v != "" {
		return v
	}
	return def
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
