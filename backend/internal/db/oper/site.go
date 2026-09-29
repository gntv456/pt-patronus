// Package oper 实现数据访问层（Oper 模式）：每个领域一个操作类，封装 GORM 事务
// 与敏感字段的加解密。借鉴 MoviePilot 的 Oper 模式。
package oper

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// ErrDuplicateSite 同注册域的站点已存在（防重复添加致仪表盘聚合双计）。
var ErrDuplicateSite = errors.New("该站点已存在")

// SiteOper 封装 Site 表的 CRUD 与密钥加解密。
type SiteOper struct {
	db *gorm.DB
	c  *security.Crypto
}

// NewSiteOper 构造 SiteOper。
func NewSiteOper(db *gorm.DB, c *security.Crypto) *SiteOper {
	return &SiteOper{db: db, c: c}
}

// DecryptedPasskey 解密站点 PasskeyEnc；空或解密失败返回 ""（辅种拼下载链接用）。
func (o *SiteOper) DecryptedPasskey(s *models.Site) string {
	if s == nil || s.PasskeyEnc == "" {
		return ""
	}
	pk, err := o.c.Decrypt(s.PasskeyEnc)
	if err != nil {
		return ""
	}
	return pk
}

// SiteInput 是创建/更新站点的入参（密钥为明文，由 Oper 加密入库）。
type SiteInput struct {
	Name          string `json:"name" binding:"required,min=1,max=64"`
	Platform      string `json:"platform" binding:"required,min=1,max=32"`
	Definition    string `json:"definition" binding:"max=64"`
	BaseURL       string `json:"base_url" binding:"required,min=1,max=512"`
	Cookie        string `json:"cookie" binding:"max=65536"`
	APIKey        string `json:"api_key" binding:"max=256"`
	Passkey       string `json:"passkey" binding:"max=128"`
	Username      string `json:"username" binding:"max=64"`  // 站点账号（财神等 API 登录用）
	Password      string `json:"password" binding:"max=128"` // 站点密码（明文，由 Oper 加密入库）
	IsActive      bool   `json:"is_active"`
	Priority      int    `json:"priority"`
	LimitInterval int    `json:"limit_interval"`
}

// ErrInvalidBaseURL 站点地址非法（非 http/https，或解析不出 host）。
// base_url 会被适配器拼接成抓取请求 URL，javascript:/file:/data: 等 scheme 入库
// 不仅必然抓取失败，还会在 ResolveReference 等路径产生危险语义——入口处拒绝。
var ErrInvalidBaseURL = errors.New("站点地址必须是 http(s):// 开头的合法 URL")

// validateBaseURL 校验站点地址：必须能 url.Parse 出 http/https scheme 与 host。
func validateBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ErrInvalidBaseURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidBaseURL
	}
	return nil
}

// dupDomain 取「重复判定」用的注册域（eTLD+1）。RegisteredDomain 已自带剥端口
// （hxpt.org:8080 → hxpt.org，端口变体不得绕过去重、仪表盘聚合双计）；
// 这里保留一次防御性剥端口，统一口径。
func dupDomain(rawURL string) string {
	dom := site.RegisteredDomain(rawURL)
	if h, _, err := net.SplitHostPort(dom); err == nil {
		return h
	}
	return dom
}

// Create 创建站点。
//
// 按注册域（eTLD+1）去重：同注册域视为同一站点，拒绝添加——对齐 MoviePilot「一站点一账号」。
// 无论 name / 子域 / path / trailing-slash 写法如何，同站只能有一条；杜绝重复站点导致的仪表盘
// 聚合双计（上传/做种翻倍）与搜索结果重复。CreateMany 同样按注册域跳过（见下）。
func (o *SiteOper) Create(in SiteInput) (*models.Site, error) {
	if err := validateBaseURL(in.BaseURL); err != nil {
		return nil, err
	}
	if dom := dupDomain(in.BaseURL); dom != "" {
		var existing []string
		if err := o.db.Model(&models.Site{}).Pluck("base_url", &existing).Error; err != nil {
			return nil, err
		}
		for _, u := range existing {
			if dupDomain(u) == dom {
				return nil, fmt.Errorf("%w: %s", ErrDuplicateSite, dom)
			}
		}
	}
	return o.createOnDB(o.db, in)
}

// createOnDB 在给定 db（o.db 或事务 tx）上创建站点，复用加密 + IsActive 兜底逻辑。
func (o *SiteOper) createOnDB(db *gorm.DB, in SiteInput) (*models.Site, error) {
	s := &models.Site{
		Name: in.Name, Platform: in.Platform, Definition: in.Definition,
		// BaseURL 去首选空白再入库：validateBaseURL 按空白修剪校验，若原样存储带空格的
		// URL，适配器拼接请求必然失败，且与去重口径（TrimSpace 后比较）不一致。
		BaseURL: strings.TrimSpace(in.BaseURL), Username: in.Username, IsActive: in.IsActive,
		Priority: in.Priority, LimitInterval: in.LimitInterval,
	}
	if err := o.encryptSecrets(s, in); err != nil {
		return nil, err
	}
	if err := db.Create(s).Error; err != nil {
		return nil, err
	}
	// GORM default:true 会把 IsActive=false 当零值吞为 true（导入 PT-Depiler isOffline
	// 站时本应停用却变启用）→ 显式写回。Update 走 Save 写全字段，不受此影响。
	if !in.IsActive {
		if err := db.Model(s).Update("is_active", false).Error; err != nil {
			return nil, err
		}
	}
	return s, nil
}

// CreateMany 单事务批量创建站点。逐条 INSERT 以保留「单条失败跳过、其余继续」语义，
// 但全部包在一个事务内——把 N 次 fsync 压成 1 次（慢盘上 200 站从数十秒降到 <1s）。
//
// 按注册域去重：库内已存在 或 本批次已处理的注册域 → 跳过（对齐 MoviePilot，防备份导入产生
// 重复站点/重复账号）。单条其他错误同样跳过不计；仅事务级失败（ctx 取消 / 磁盘满）整体回滚。
// 返回成功创建的条数（调用方用 len(ins)-n 自算跳过数）。
func (o *SiteOper) CreateMany(ins []SiteInput) (int, error) {
	if len(ins) == 0 {
		return 0, nil
	}
	n := 0
	err := o.db.Transaction(func(tx *gorm.DB) error {
		// 已占用注册域集合（库内既有 + 本批次已建），同注册域只保留第一条。
		seen := map[string]bool{}
		var existing []string
		if e := tx.Model(&models.Site{}).Pluck("base_url", &existing).Error; e != nil {
			return e
		}
		for _, u := range existing {
			seen[dupDomain(u)] = true
		}
		for _, in := range ins {
			dom := dupDomain(in.BaseURL)
			if dom != "" && seen[dom] {
				continue // 同注册域已存在/已处理 → 跳过
			}
			if _, e := o.createOnDB(tx, in); e != nil {
				continue
			}
			seen[dom] = true
			n++
		}
		return nil
	})
	return n, err
}

// Update 更新站点（密钥为空则保留原值）。
func (o *SiteOper) Update(id uint, in SiteInput) (*models.Site, error) {
	s, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	if err := validateBaseURL(in.BaseURL); err != nil {
		return nil, err
	}
	// 按注册域去重（与 Create/CreateMany 一致）：新 BaseURL 的注册域不得与其它站点冲突（排除自身），
	// 否则会产生同注册域重复站点，导致仪表盘聚合双计、搜索结果重复。
	if dom := dupDomain(in.BaseURL); dom != "" {
		var existing []string
		if err := o.db.Model(&models.Site{}).Where("id <> ?", id).Pluck("base_url", &existing).Error; err != nil {
			return nil, err
		}
		for _, u := range existing {
			if dupDomain(u) == dom {
				return nil, fmt.Errorf("%w: %s", ErrDuplicateSite, dom)
			}
		}
	}
	s.Name = in.Name
	s.Platform = in.Platform
	s.Definition = in.Definition
	s.BaseURL = strings.TrimSpace(in.BaseURL)
	s.Username = in.Username
	s.IsActive = in.IsActive
	s.Priority = in.Priority
	s.LimitInterval = in.LimitInterval
	if err := o.encryptSecrets(s, in); err != nil {
		return nil, err
	}
	if err := o.db.Save(s).Error; err != nil {
		return nil, err
	}
	return s, nil
}

// List 列出全部站点（密钥字段 json:"-" 不外泄）。
func (o *SiteOper) List() ([]models.Site, error) {
	var sites []models.Site
	if err := o.db.Order("priority desc, id asc").Find(&sites).Error; err != nil {
		return nil, err
	}
	return sites, nil
}

// ListActive 列出启用的站点。
func (o *SiteOper) ListActive() ([]models.Site, error) {
	var sites []models.Site
	if err := o.db.Where("is_active = ?", true).Order("priority desc, id asc").Find(&sites).Error; err != nil {
		return nil, err
	}
	return sites, nil
}

// Get 取单个站点。
func (o *SiteOper) Get(id uint) (*models.Site, error) {
	var s models.Site
	if err := o.db.First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// Delete 删除站点。
func (o *SiteOper) Delete(id uint) error {
	return o.db.Delete(&models.Site{}, id).Error
}

// DeleteMany 批量删除站点，返回「不存在/删除失败」的 id 列表。
// 与 Delete 一致的硬删语义（无级联，等同多次单删）：先查存在集，再一条 SQL 删除已存在的，
// 不存在的计入 failed —— 这样既能批量又能精确汇报逐条结果，避免「整批失败」。
func (o *SiteOper) DeleteMany(ids []uint) ([]uint, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// 去重（保持顺序）：重复 id 会让调用方按 len(ids)-len(failed) 虚报删除数。
	seen := make(map[uint]struct{}, len(ids))
	uniq := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	ids = uniq
	var found []uint
	if err := o.db.Model(&models.Site{}).Where("id IN ?", ids).Pluck("id", &found).Error; err != nil {
		return nil, err
	}
	foundSet := make(map[uint]struct{}, len(found))
	for _, id := range found {
		foundSet[id] = struct{}{}
	}
	var failed []uint
	for _, id := range ids {
		if _, ok := foundSet[id]; !ok {
			failed = append(failed, id)
		}
	}
	if len(found) > 0 {
		if err := o.db.Where("id IN ?", found).Delete(&models.Site{}).Error; err != nil {
			return nil, err
		}
	}
	return failed, nil
}

// Cookie 解密返回站点 cookie。
func (o *SiteOper) Cookie(s *models.Site) (string, error) {
	return o.c.Decrypt(s.CookieEnc)
}

// FindByDownloadURL 按 download URL 的注册域匹配启用站点（手动加种/AI 工具无 SiteID 时反查用）。
// 命中返回首个；未命中返回 (nil, nil)——调用方据此退化为旧 URL 直推（不阻断，向后兼容）。
func (o *SiteOper) FindByDownloadURL(rawURL string) (*models.Site, error) {
	dom := site.RegisteredDomain(rawURL)
	if dom == "" {
		return nil, nil
	}
	sites, err := o.ListActive()
	if err != nil {
		return nil, err
	}
	for i := range sites {
		if site.RegisteredDomain(sites[i].BaseURL) == dom {
			return &sites[i], nil
		}
	}
	return nil, nil
}

// UpdateCookie 仅更新站点 cookie（WebView 登录后上传用）。
func (o *SiteOper) UpdateCookie(id uint, cookie string) error {
	s, err := o.Get(id)
	if err != nil {
		return err
	}
	enc, err := o.c.Encrypt(cookie)
	if err != nil {
		return err
	}
	s.CookieEnc = enc
	return o.db.Save(s).Error
}

// APIKey 解密返回站点 API key。
func (o *SiteOper) APIKey(s *models.Site) (string, error) {
	return o.c.Decrypt(s.APIKeyEnc)
}

// Password 解密返回站点密码（财神 API 登录用）。
func (o *SiteOper) Password(s *models.Site) (string, error) {
	return o.c.Decrypt(s.PasswordEnc)
}

// encryptSecrets 把明文密钥加密写入 s；空值保留原密文（用于更新）。
func (o *SiteOper) encryptSecrets(s *models.Site, in SiteInput) error {
	if in.Cookie != "" {
		enc, err := o.c.Encrypt(in.Cookie)
		if err != nil {
			return err
		}
		s.CookieEnc = enc
	}
	if in.APIKey != "" {
		enc, err := o.c.Encrypt(in.APIKey)
		if err != nil {
			return err
		}
		s.APIKeyEnc = enc
	}
	if in.Passkey != "" {
		enc, err := o.c.Encrypt(in.Passkey)
		if err != nil {
			return err
		}
		s.PasskeyEnc = enc
	}
	if in.Password != "" {
		enc, err := o.c.Encrypt(in.Password)
		if err != nil {
			return err
		}
		s.PasswordEnc = enc
	}
	return nil
}

// UpsertConnectivity 覆盖写一站联通状态缓存（site_id 唯一：存在则更新，不存在则创建）并累加统计。
// connected → Success++ 且（seconds>0 时）滑动平均更新 AvgSeconds；disconnected → Fail++；
// disabled（停用站未发请求）不计统计。status/level/error_msg/checked_at 始终覆盖。
// 读-改-写：单站检测串行（每站一个 goroutine / 手动触发），无并发累加冲突。
func (o *SiteOper) UpsertConnectivity(siteID uint, status, level, errMsg string, seconds float64) error {
	rec := models.SiteConnectivity{SiteID: siteID}
	if err := o.db.Where(&models.SiteConnectivity{SiteID: siteID}).FirstOrCreate(&rec).Error; err != nil {
		return err
	}
	switch status {
	case "connected":
		rec.Success++
		if seconds > 0 {
			// 滑动平均：新均 = (旧均 × (n-1) + 本次) / n（n 为本次成功后的次数）
			rec.AvgSeconds = (rec.AvgSeconds*float64(rec.Success-1) + seconds) / float64(rec.Success)
		}
	case "disconnected":
		rec.Fail++
	}
	rec.Status = status
	rec.Level = level
	rec.ErrorMsg = errMsg
	rec.CheckedAt = time.Now()
	return o.db.Save(&rec).Error
}

// ListConnectivity 返回全部站点联通状态缓存（GET /sites/status 进页面用，瞬时读、不发网络请求）。
func (o *SiteOper) ListConnectivity() ([]models.SiteConnectivity, error) {
	var list []models.SiteConnectivity
	if err := o.db.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
