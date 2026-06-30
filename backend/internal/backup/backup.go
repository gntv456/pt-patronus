// Package backup 实现配置导出/导入：自身 JSON 格式（含站点/下载器）+ MoviePilot 站点导入。
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/oper"
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
}

// New 构造。
func New(siteOp *oper.SiteOper, dlOp *oper.DownloaderOper) *Service {
	return &Service{siteOp: siteOp, dlOp: dlOp}
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

// Import 从自身格式 JSON 导入（重复站点按 name 跳过已存在）。
func (s *Service) Import(ctx context.Context, data []byte) (sites, downloaders int, err error) {
	var b Backup
	if err := json.Unmarshal(data, &b); err != nil {
		return 0, 0, fmt.Errorf("parse backup: %w", err)
	}
	for _, se := range b.Sites {
		if _, e := s.siteOp.Create(oper.SiteInput{
			Name: se.Name, Platform: se.Platform, Definition: se.Definition,
			BaseURL: se.BaseURL, Cookie: se.Cookie, IsActive: se.IsActive,
		}); e != nil {
			continue // 重复等错误跳过
		}
		sites++
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
	n := 0
	for _, m := range list {
		if m.Name == "" {
			continue
		}
		if _, err := s.siteOp.Create(oper.SiteInput{
			Name: m.Name, Platform: "nexusphp", BaseURL: m.URL, Cookie: m.Cookie, IsActive: true,
		}); err != nil {
			continue
		}
		n++
	}
	return n, nil
}
