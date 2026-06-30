package auth

import (
	"context"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/site"
)

// LoginOutcome 是账号登录成功后的结果。
type LoginOutcome struct {
	Cookie   string // 好学登录 cookie（落库加密 + VIP 重验用）
	UserInfo *site.UserInfo
	VIP      *VIPStatus
	HxptUID  string // 好学用户 ID（= UserInfo.ID），作外部身份 upsert 主键
}

// HxptAuthenticator 抽象好学登录与 VIP 校验，便于 AuthHandler 注入 fake（测试）。
// 生产实现 *HxptLogin；账号登录失败/挑战模式/非 VIP 由具体错误或 VIP.Verified 表达。
type HxptAuthenticator interface {
	// FetchCaptcha 取登录页验证码 / 探测模式（账号登录 Step A）。
	FetchCaptcha(ctx context.Context) (*site.CaptchaChallenge, error)
	// Login 账号登录（Step B）：POST takelogin.php + VIP 校验。
	// 挑战响应模式返回 site.ErrChallengeMode；凭据/验证码/2FA 错返回 site.ErrLoginFailed。
	Login(ctx context.Context, creds site.LoginCredentials, ch *site.CaptchaChallenge, imagestring, twoStepCode string) (*LoginOutcome, error)
	// Verify 用 cookie 校验 VIP（cookie 登录 + 定期重验复用）。
	Verify(ctx context.Context, cookie string) *VIPStatus
}

// HxptLogin 是 HxptAuthenticator 的生产实现：编排登录两步 + VIP 校验。
type HxptLogin struct {
	registry    *site.Registry
	defID       string // 默认 "hxpt"
	fallbackURL string
	vip         *HxptVIP // 复用 Verify（cookie 登录 + VIP 重验 cron）
	// newClient 每次返回独立 jar、FollowRedirect=false 的临时 client（隔离并发登录）。
	// 为 nil 表示账号登录不可用（仅 cookie 登录走 Verify）。
	newClient func() (site.LoginHTTPDoer, error)
}

// NewHxptLogin 构造。
func NewHxptLogin(registry *site.Registry, defID, fallbackURL string, vip *HxptVIP,
	newClient func() (site.LoginHTTPDoer, error)) *HxptLogin {
	if defID == "" {
		defID = "hxpt"
	}
	return &HxptLogin{registry: registry, defID: defID, fallbackURL: fallbackURL, vip: vip, newClient: newClient}
}

// resolveLogin 解析好学定义/baseURL/适配器（type-assert LoginCapable）。
func (h *HxptLogin) resolveLogin() (*site.SiteMetadata, string, site.LoginCapable, error) {
	if h.registry == nil || h.newClient == nil {
		return nil, "", nil, site.ErrLoginNotSupported
	}
	def, ok := h.registry.Definition(h.defID)
	if !ok {
		return nil, "", nil, fmt.Errorf("definition %q not loaded", h.defID)
	}
	baseURL := h.fallbackURL
	if len(def.URLs) > 0 {
		baseURL = def.URLs[0]
	}
	adapter, ok := h.registry.AdapterFor(def.Schema)
	if !ok {
		return nil, "", nil, fmt.Errorf("no adapter for schema %q", def.Schema)
	}
	lc, ok := adapter.(site.LoginCapable)
	if !ok {
		return nil, "", nil, site.ErrLoginNotSupported
	}
	return def, baseURL, lc, nil
}

// FetchCaptcha 实现 HxptAuthenticator：取登录页验证码 / 探测模式。
func (h *HxptLogin) FetchCaptcha(ctx context.Context) (*site.CaptchaChallenge, error) {
	def, baseURL, lc, err := h.resolveLogin()
	if err != nil {
		return nil, err
	}
	client, err := h.newClient()
	if err != nil {
		return nil, fmt.Errorf("create login http client: %w", err)
	}
	return lc.FetchLoginPage(ctx, client, def, baseURL)
}

// Login 实现 HxptAuthenticator：POST takelogin.php + VIP 校验，返回 LoginOutcome。
func (h *HxptLogin) Login(ctx context.Context, creds site.LoginCredentials, ch *site.CaptchaChallenge,
	imagestring, twoStepCode string) (*LoginOutcome, error) {

	def, baseURL, lc, err := h.resolveLogin()
	if err != nil {
		return nil, err
	}
	client, err := h.newClient()
	if err != nil {
		return nil, fmt.Errorf("create login http client: %w", err)
	}
	sess, err := lc.PostLogin(ctx, client, def, baseURL, creds, ch, imagestring, twoStepCode)
	if err != nil {
		return nil, err
	}
	// VIP 判定统一走 verifyAccess.php（账号登录拿到 cookie 后与 cookie 登录同源）。
	// 不再用刮等级文本的 site.IsVIP——APP专属VIP 是 DB 字段，非等级行头衔（历史误判根因）。
	var vip *VIPStatus
	if h.vip != nil {
		vip = h.vip.Verify(ctx, sess.Cookie)
	} else {
		vip = &VIPStatus{Verified: site.IsVIP(sess.UserInfo, def), Level: sess.UserInfo.Level,
			Username: sess.UserInfo.Name, Message: "非 VIP"}
		if vip.Verified {
			vip.Message = "好学 VIP 验证通过"
		}
	}
	uid := sess.UserInfo.ID
	if uid == "" {
		uid = vip.ID // 资料页未解析出 ID 时回退 verifyAccess 的 uid
	}
	if vip.Username == "" {
		vip.Username = sess.UserInfo.Name
	}
	return &LoginOutcome{
		Cookie:   sess.Cookie,
		UserInfo: sess.UserInfo,
		VIP:      vip,
		HxptUID:  uid,
	}, nil
}

// Verify 实现 HxptAuthenticator：用 cookie 校验 VIP（复用 HxptVIP）。
func (h *HxptLogin) Verify(ctx context.Context, cookie string) *VIPStatus {
	if h.vip == nil {
		return &VIPStatus{Verified: false, Message: "VIP 校验未启用"}
	}
	return h.vip.Verify(ctx, cookie)
}
