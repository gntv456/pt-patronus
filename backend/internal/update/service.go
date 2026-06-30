package update

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/version"
)

const cacheTTL = 5 * time.Minute // 远端官方清单的内存缓存有效期(对齐旧版 UpdateChecker 的 5 分钟)

// Service 在线更新检查服务。取清单优先级：自托管清单(data/update-manifest.json，管理员
// 后台写入)→ 官方源(UPDATE_MANIFEST_URL，5 分钟缓存，失败降级用过期缓存)。与当前版本比较。
type Service struct {
	hc          *httpclient.Client
	manifestURL string
	overlayPath string // 自托管清单落盘路径(<DataDir>/update-manifest.json)

	mu       sync.RWMutex
	cached   *Manifest // 远端官方清单缓存(不含自托管)
	cachedAt time.Time
}

// NewService 构造。manifestURL 为空则只读自托管清单(无官方源)。
func NewService(cfg *config.Config, hc *httpclient.Client) *Service {
	var overlay string
	if cfg.DataDir != "" {
		overlay = filepath.Join(cfg.DataDir, "update-manifest.json")
	}
	return &Service{hc: hc, manifestURL: cfg.UpdateManifestURL, overlayPath: overlay}
}

// Result 是 Check 的返回，序列化为 GET /system/update/check 的响应体。
type Result struct {
	HasUpdate      bool   `json:"hasUpdate"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	ReleaseName    string `json:"releaseName,omitempty"`
	ReleaseNotes   string `json:"releaseNotes,omitempty"`
	DownloadURL    string `json:"downloadUrl,omitempty"`    // 该平台的下载地址(网盘/直链)
	SHA256         string `json:"sha256,omitempty"`         // 下载后校验
	Image          string `json:"image,omitempty"`          // docker 镜像 tag(docker 形态)
	Instructions   string `json:"instructions,omitempty"`   // 更新命令(docker 形态，如 docker compose pull && up -d)
	IsForced       bool   `json:"isForced,omitempty"`       // 强制更新(软强制：UI 不可忽略)
	IsBelowMin     bool   `json:"isBelowMin,omitempty"`     // 当前版本低于 minVersion(真·强制阻断)
}

// Check 拉清单并比较。platform 决定取 Targets 里的哪个产物；currentVersion 为空则用
// 内置 version.Version(服务端自检场景)。
func (s *Service) Check(ctx context.Context, platform, currentVersion string) (*Result, error) {
	if currentVersion == "" {
		currentVersion = version.Version
	}
	m, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	if m == nil || m.LatestVersion == "" {
		// 无清单(未配官方源且无自托管)：无可报更新
		return &Result{HasUpdate: false, CurrentVersion: currentVersion}, nil
	}
	res := &Result{
		CurrentVersion: currentVersion,
		LatestVersion:  m.LatestVersion,
		ReleaseName:    m.ReleaseName,
		ReleaseNotes:   m.ReleaseNotes,
		IsForced:       m.IsForced,
	}
	res.HasUpdate = compareVersions(m.LatestVersion, currentVersion) > 0
	// 低于最低版本 → 真强制(前端应阻断使用)
	if m.MinVersion != "" && compareVersions(currentVersion, m.MinVersion) < 0 {
		res.IsBelowMin = true
		res.IsForced = true
	}
	if platform != "" {
		if t, ok := m.Targets[platform]; ok {
			res.DownloadURL = t.URL
			res.SHA256 = t.SHA256
			res.Image = t.Image
			res.Instructions = t.Instructions
		}
	}
	return res, nil
}

// SetOverlay 写入/清除自托管清单(管理员后台调用)。overlay=nil(或 LatestVersion 空)清除，
// 回退官方源。落盘后立即清掉内存缓存，使下次 fetch 重新判定来源(自托管优先)。
func (s *Service) SetOverlay(overlay *Manifest) error {
	s.mu.Lock()
	s.cached = nil // 清官方缓存，强制 fetch 重读(优先自托管)
	overlayPath := s.overlayPath
	s.mu.Unlock()

	if overlay == nil || overlay.LatestVersion == "" {
		if overlayPath != "" {
			_ = os.Remove(overlayPath) // 不存在不算错
		}
		return nil
	}
	if overlayPath == "" {
		return fmt.Errorf("data dir 未配置，无法持久化自托管清单")
	}
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(overlayPath, b, 0o644)
}

// fetch 取生效清单：自托管文件优先 → 官方源(带缓存)。
func (s *Service) fetch(ctx context.Context) (*Manifest, error) {
	if m := s.readOverlay(); m != nil && m.LatestVersion != "" {
		return m, nil
	}
	if s.manifestURL == "" {
		return nil, nil
	}
	s.mu.RLock()
	cached, age := s.cached, time.Since(s.cachedAt)
	s.mu.RUnlock()
	if cached != nil && cached.LatestVersion != "" && age < cacheTTL {
		return cached, nil
	}
	m, err := s.fetchRemote(ctx)
	if err != nil {
		// 远端失败：降级用过期缓存(比直接报错好，旧版亦如此)
		if cached != nil {
			return cached, nil
		}
		return nil, err
	}
	s.mu.Lock()
	s.cached = m
	s.cachedAt = time.Now()
	s.mu.Unlock()
	return m, nil
}

func (s *Service) fetchRemote(ctx context.Context) (*Manifest, error) {
	// ssrf=true：UPDATE_MANIFEST_URL 是可配置的，防被设成内网地址做 SSRF 探测。
	resp, err := s.hc.Get(ctx, s.manifestURL, "", true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("manifest fetch: HTTP %d", resp.StatusCode)
	}
	const maxManifest = 1 << 20 // 1MB 上限防滥用
	if len(resp.Body) > maxManifest {
		return nil, fmt.Errorf("manifest too large: %d bytes", len(resp.Body))
	}
	var m Manifest
	if err := json.Unmarshal(resp.Body, &m); err != nil {
		return nil, fmt.Errorf("manifest parse: %w", err)
	}
	return &m, nil
}

func (s *Service) readOverlay() *Manifest {
	if s.overlayPath == "" {
		return nil
	}
	b, err := os.ReadFile(s.overlayPath)
	if err != nil {
		return nil
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return &m
}
