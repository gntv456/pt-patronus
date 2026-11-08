// Package sitedefs 站点定义远端源同步：把官方站点定义仓库（ptpatronus-sites）
// 的最新定义拉到本地数据目录并热加载，站点适配不再绑定主程序发版。
//
// 分发模型（对标 MoviePilot 站点资源）：
//
//	内置 embed（随版本发布的兜底全集）
//	  ↓ 被同 ID 覆盖
//	DEFINITIONS_DIR（用户手动挂载的私有定义，可选）
//	  ↓ 被同 ID 覆盖
//	DATA_DIR/site-definitions（本包维护的远端缓存，自动热更）
//
// 远端格式：GitHub 仓库 main.tar.gz（顶层 *.yaml 即定义全集）。校验三道闸：
// tarball 大小上限（防恶意超大包）、YAML 可解析 + id 非空、min_app_version
// 不满足的定义跳过（记升级诊断）。落地原子（整目录 .tmp→rename）；任一步
// 失败保留上次缓存（离线可用），下次 cron 重试。
package sitedefs

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
	"gopkg.in/yaml.v3"
)

const (
	// cacheDirName 远端定义缓存在数据目录下的子目录名。
	cacheDirName = "site-definitions"
	// maxTarballBytes tarball 大小上限（定义全集远小于此；防劫持/误配撑爆磁盘）。
	maxTarballBytes = 20 << 20
	// maxDefBytes 单个定义文件上限。
	maxDefBytes = 1 << 20
	// httpTimeout 拉取超时（慢源不至于占死 cron 周期）。
	httpTimeout = 90 * time.Second
)

// DefaultRemoteURL 官方站点定义仓库 tarball 地址（gntv456/ptpatronus-sites@main）。
const DefaultRemoteURL = "https://github.com/gntv456/ptpatronus-sites/archive/refs/heads/main.tar.gz"

// Syncer 持有远端定义同步的全部依赖（app 装配期构造一次）。
type Syncer struct {
	hc        *httpclient.Client
	dlClient  *httpclient.Client // 拉取专用（FollowRedirect=true）；nil 时测试注入 hc
	registry  *site.Registry
	dataDir   string        // DATA_DIR（缓存落地 <dataDir>/site-definitions）
	appVer    string        // min_app_version 门控用
	extraDirs []string      // DEFINITIONS_DIR（热加载时要一并叠上的外部目录）
	remoteURL func() string // 每次同步现读设置（改源不用重启）
}

// NewSyncer 构造。remoteURLGetter 每次调用时读最新配置（SystemConfig 热更）。
// 内部自建带 FollowRedirect 的下载 client（GitHub archive 302 跳
// objects.githubusercontent.com；共享站点抓取 client 不带重定向）。
func NewSyncer(hc *httpclient.Client, registry *site.Registry, dataDir, appVersion string,
	extraDirs []string, remoteURLGetter func() string) *Syncer {
	dl, err := httpclient.New(httpclient.Options{Timeout: httpTimeout, FollowRedirect: true})
	if err != nil {
		slog.Error("sitedefs download client init failed; sync will fall back to shared client", "err", err)
	}
	return &Syncer{hc: hc, dlClient: dl, registry: registry, dataDir: dataDir, appVer: appVersion,
		extraDirs: extraDirs, remoteURL: remoteURLGetter}
}

// CacheDir 远端定义缓存目录（启动加载与热加载共用此路径）。
func (s *Syncer) CacheDir() string { return filepath.Join(s.dataDir, cacheDirName) }

// Sync 拉取远端定义并热加载。返回本次生效的定义数（诊断细节在 registry.diag）。
// 语义：下载/解包失败 → 返回错误（保留上次缓存）；新表为空 → 视为远端异常，
// 保留旧缓存不热加载（防止误清空把所有远端站抹掉）。
func (s *Syncer) Sync(ctx context.Context) (int, error) {
	url := strings.TrimSpace(s.remoteURL())
	if url == "" {
		url = DefaultRemoteURL
	}
	defs, err := s.fetchRemote(ctx, url)
	if err != nil {
		return 0, err
	}
	if len(defs) == 0 {
		return 0, fmt.Errorf("远端定义源解包后为空（%s），保留现有缓存", url)
	}
	// 原子落地：全部 YAML 写进 .tmp，成功后 rename 换目录（写入中途中断不留半份）。
	cacheDir := s.CacheDir()
	tmpDir := cacheDir + ".tmp"
	if err := os.RemoveAll(tmpDir); err != nil {
		return 0, fmt.Errorf("clean tmp dir: %w", err)
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return 0, err
	}
	for id, def := range defs {
		// 原始 YAML 原样落地（零损失：远端文件里的注释/未知键都保留）。
		name := sanitizeFileName(id) + ".yaml"
		if err := os.WriteFile(filepath.Join(tmpDir, name), def.raw, 0o644); err != nil {
			return 0, err
		}
	}
	oldDir := cacheDir + ".old"
	_ = os.RemoveAll(oldDir)
	if err := os.Rename(cacheDir, oldDir); err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	if err := os.Rename(tmpDir, cacheDir); err != nil {
		_ = os.Rename(oldDir, cacheDir) // 回滚
		return 0, err
	}
	_ = os.RemoveAll(oldDir)

	// 热加载：bundle 语义已在内置表，远端缓存叠加其上（同 ID 覆盖）；
	// ReloadDefinitions 整表替换，故 DEFINITIONS_DIR（用户手挂私有定义）一并带上。
	dirs := append([]string{}, s.extraDirs...)
	dirs = append(dirs, s.CacheDir())
	if err := s.registry.ReloadDefinitions(s.appVer, dirs...); err != nil {
		return 0, fmt.Errorf("hot reload: %w", err)
	}
	// 返回当前注册表定义总数（含门控跳过的差值效果）：界面「同步完成，共 N 个
	// 站点定义」比「下载了 N 个」更贴用户视角。
	total := len(s.registry.Definitions())
	slog.Info("site definitions synced from remote", "url", url,
		"remote_defs", len(defs), "total", total)
	return total, nil
}

// fetchRemote 下载 tarball 并解包出全部合法定义。
// key = 定义 id（解析后），value = 原始 YAML（保留未识别键，落地回写零损失）。
func (s *Syncer) fetchRemote(ctx context.Context, url string) (map[string]remoteDef, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	// 独立 client 开 FollowRedirect：GitHub archive 会 302 到
	// objects.githubusercontent.com；共享 hc 不带重定向（站点抓取口径）。
	dlClient := s.hc
	if s.dlClient != nil {
		dlClient = s.dlClient
	}
	resp, err := dlClient.Get(ctx, url, "", false)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	if len(resp.Body) > maxTarballBytes {
		return nil, fmt.Errorf("tarball %d bytes exceeds limit %d", len(resp.Body), maxTarballBytes)
	}
	return parseTarball(resp.Body)
}

// remoteDef 是一个通过校验的远端定义（raw 为原始 YAML 文本）。
type remoteDef struct {
	id  string
	raw []byte
}

// parseTarball 解 gzip/tar 流：顶层 *.yaml 逐个 yaml 解析校验（id 非空即收，
// min_app_version 的跳过判定交给 registry 加载期——那里有统一诊断）。
func parseTarball(body []byte) (map[string]remoteDef, error) {
	gz, err := gzip.NewReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("gunzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	out := map[string]remoteDef{}
	seenPrefix := ""
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.ToSlash(hdr.Name)
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		// GitHub tarball 顶层是 <repo>-<ref>/ 前缀；取首个目录名统一剥掉。
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 {
			continue
		}
		if seenPrefix == "" {
			seenPrefix = parts[0] + "/"
		}
		rel := strings.TrimPrefix(name, seenPrefix)
		if strings.Contains(rel, "/") { // 顶层文件才算（子目录不递归，对齐 registry 语义）
			continue
		}
		if hdr.Size > maxDefBytes {
			continue
		}
		raw := make([]byte, hdr.Size)
		if _, err := io.ReadFull(tr, raw); err != nil {
			continue
		}
		var meta struct {
			ID string `yaml:"id"`
		}
		if err := yaml.Unmarshal(raw, &meta); err != nil || strings.TrimSpace(meta.ID) == "" {
			continue
		}
		out[strings.TrimSpace(meta.ID)] = remoteDef{id: strings.TrimSpace(meta.ID), raw: raw}
	}
	return out, nil
}

func sanitizeFileName(id string) string {
	id = strings.TrimSpace(id)
	var b strings.Builder
	for _, r := range id {
		ok := r == '-' || r == '_' || r == '.' ||
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if ok {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "unnamed"
	}
	return b.String()
}
