// Package backup 实现配置导出/导入：自身 JSON 格式（含站点/下载器）+ MoviePilot 站点导入。
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// SiteExport 站点导出项（密钥明文，由用户妥善保管备份文件）。
type SiteExport struct {
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	Definition string `json:"definition"`
	BaseURL    string `json:"base_url"`
	Cookie     string `json:"cookie"`
	APIKey     string `json:"api_key"`
	Passkey    string `json:"passkey"`
	IsActive   bool   `json:"is_active"`
}

// DownloaderExport 下载器导出项。
type DownloaderExport struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	IsDefault bool   `json:"is_default"`
}

// Backup 是自身格式的备份结构。
type Backup struct {
	Version     int                `json:"version"`
	ExportedAt  time.Time          `json:"exported_at"`
	Sites       []SiteExport       `json:"sites"`
	Downloaders []DownloaderExport `json:"downloaders"`
}

const backupVersion = 1

// Service 备份服务。
type Service struct {
	siteOp *oper.SiteOper
	dlOp   *oper.DownloaderOper
	reg    *site.Registry // .ptd 恢复时按 URL 匹配站点定义（MatchByURL），避免全退化为默认 nexusphp
}

// New 构造。
func New(siteOp *oper.SiteOper, dlOp *oper.DownloaderOper, reg *site.Registry) *Service {
	return &Service{siteOp: siteOp, dlOp: dlOp, reg: reg}
}

// Export 导出为 JSON 字节。
func (s *Service) Export(ctx context.Context) ([]byte, error) {
	b := Backup{Version: backupVersion, ExportedAt: time.Now()}

	sites, err := s.siteOp.List()
	if err != nil {
		return nil, err
	}
	for _, st := range sites {
		cookie, _ := s.siteOp.Cookie(&st)
		b.Sites = append(b.Sites, SiteExport{
			Name: st.Name, Platform: st.Platform, Definition: st.Definition,
			BaseURL: st.BaseURL, Cookie: cookie, IsActive: st.IsActive,
		})
	}

	dls, err := s.dlOp.List()
	if err != nil {
		return nil, err
	}
	for _, d := range dls {
		pw, _ := s.dlOp.Password(&d)
		b.Downloaders = append(b.Downloaders, DownloaderExport{
			Name: d.Name, Type: d.Type, URL: d.URL, Username: d.Username,
			Password: pw, IsDefault: d.IsDefault,
		})
	}
	return json.MarshalIndent(b, "", "  ")
}

// backupKeepLatest 定时备份保留的最新文件数。
const backupKeepLatest = 10

// ExportToFile 导出备份并写入 dir（ptpatronus-backup-<时间戳>.json），只保留最新
// backupKeepLatest 份（按文件名时间戳轮换，仅删除本工具命名的文件）。
// 备份含站点 Cookie/下载器密码明文：文件权限 0600、目录 0700，目录可信性由使用者保证。
// 供定时备份 cron 使用；返回写入路径。
func (s *Service) ExportToFile(ctx context.Context, dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("备份目录为空")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("创建备份目录: %w", err)
	}
	data, err := s.Export(ctx)
	if err != nil {
		return "", err
	}
	stamp := time.Now().Format("20060102-150405")
	// 四轮 P2 修复：后缀掺 UnixNano——此前 Stat-then-Write 防撞在并发导出（手动
	// 连点 + cron 同秒撞车）时双双 Stat 到不存在、选同一 -1 后缀互相覆盖丢一份。
	path := filepath.Join(dir, fmt.Sprintf("ptpatronus-backup-%s-%d.json", stamp, time.Now().UnixNano()%1e9))
	// 四轮 P2 修复：tmp+rename 原子写——直写崩溃会留截断 JSON 且被轮换策略当成
	// "最新"保留，灾难恢复时才发现坏。
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", fmt.Errorf("写备份文件: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("落盘备份文件: %w", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "ptpatronus-backup-*.json"))
	if len(matches) > backupKeepLatest {
		sort.Strings(matches) // 文件名时间戳字典序 = 时间序
		for _, old := range matches[:len(matches)-backupKeepLatest] {
			_ = os.Remove(old)
		}
	}
	return path, nil
}

// Import 从自身格式 JSON 导入（重复站点按注册域跳过，CreateMany 统一去重）。
func (s *Service) Import(ctx context.Context, data []byte) (sites, downloaders int, err error) {
	var b Backup
	if err := json.Unmarshal(data, &b); err != nil {
		return 0, 0, fmt.Errorf("parse backup: %w", err)
	}
	siteIns := make([]oper.SiteInput, 0, len(b.Sites))
	for _, se := range b.Sites {
		defID := se.Definition
		if defID == "" {
			defID = s.matchDef(se.BaseURL) // 备份未带 definition → 按 URL 匹配 yaml
		}
		siteIns = append(siteIns, oper.SiteInput{
			Name: se.Name, Platform: se.Platform, Definition: defID,
			BaseURL: se.BaseURL, Cookie: se.Cookie, IsActive: se.IsActive,
		})
	}
	sites, err = s.siteOp.CreateMany(siteIns) // 单事务批量：N 次 fsync → 1 次
	if err != nil {
		return 0, 0, err
	}
	for _, de := range b.Downloaders {
		if _, e := s.dlOp.Create(oper.DownloaderInput{
			Name: de.Name, Type: de.Type, URL: de.URL, Username: de.Username,
			Password: de.Password, IsDefault: de.IsDefault,
		}); e != nil {
			continue
		}
		downloaders++
	}
	return sites, downloaders, nil
}

// moviePilotSite 是 MoviePilot 站点配置的可识别子集。
type moviePilotSite struct {
	Name   string `json:"name"`
	URL    string `json:"url" json_url:"url"`
	Domain string `json:"domain"`
	Cookie string `json:"cookie"`
	Pri    int    `json:"pri"`
}

// ImportMoviePilot 从 MoviePilot 站点 JSON 数组导入站点。
func (s *Service) ImportMoviePilot(ctx context.Context, data []byte) (int, error) {
	var list []moviePilotSite
	if err := json.Unmarshal(data, &list); err != nil {
		return 0, fmt.Errorf("parse moviepilot sites: %w", err)
	}
	ins := make([]oper.SiteInput, 0, len(list))
	for _, m := range list {
		if m.Name == "" {
			continue
		}
		ins = append(ins, oper.SiteInput{
			Name: m.Name, Platform: "nexusphp", Definition: s.matchDef(m.URL),
			BaseURL: m.URL, Cookie: m.Cookie, IsActive: true,
		})
	}
	return s.siteOp.CreateMany(ins)
}

// matchDef 按 baseURL 匹配站点定义（definitions/*.yaml 的 host）。用于各导入路径
// （.ptd / 自身 JSON / MoviePilot）统一挂「正确定义」，避免全退化为默认 nexusphp——
// tnode/特殊 schema 站走错适配器会彻底抓空，nexusphp 站用通用 def 也常抓不全。
// s.reg 为空（测试）或无匹配返回空串，调用方回退到 platform 推断。
func (s *Service) matchDef(baseURL string) string {
	if s.reg == nil {
		return ""
	}
	id, _ := s.reg.MatchByURL(baseURL)
	return id
}
