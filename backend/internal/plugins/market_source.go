package plugins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) hasPlugin(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.plugins[id]
	return ok
}

func (m *Manager) hasInstalled(id string) bool {
	if m.hasPlugin(id) {
		return true
	}
	root := m.PluginRoot()
	if root == "" {
		return false
	}
	manifestPath := filepath.Join(root, id, "plugin.json")
	if !isSubpath(root, manifestPath) {
		return false
	}
	_, err := os.Stat(manifestPath)
	return err == nil
}

func (m *Manager) installedVersion(id string) string {
	if v, err := m.Get(id); err == nil {
		return v.Version
	}
	root := m.PluginRoot()
	if root == "" {
		return ""
	}
	manifest, err := readInstalledManifest(filepath.Join(root, id))
	if err != nil {
		return ""
	}
	return manifest.Version
}

// normalizeMarketSource 把市场源地址归一化为可直接请求的 URL/路径。
// GitHub 仓库页（含 owner/repo 简写）→ jsDelivr CDN 上的 plugin-market.json
// （jsDelivr 国内/全球均比 raw.githubusercontent.com 直连快且稳定，raw 国内偶发 >30s 超时）：
//
//	https://github.com/o/r                  → https://cdn.jsdelivr.net/gh/o/r@main/plugin-market.json
//	https://github.com/o/r/tree/dev/market  → https://cdn.jsdelivr.net/gh/o/r@dev/market/plugin-market.json
//	o/r / github.com/o/r（无 scheme）        → 同上补 https
//
// 已是 raw.githubusercontent.com / file / 其它 http(s) 源 → 原样返回。
// resolveArchiveSource 会把 index 内相对 archive 基于此 URL 解析，自动指向仓库内 zip。
func normalizeMarketSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	s := source
	if !strings.Contains(s, "://") {
		switch {
		case strings.HasPrefix(strings.ToLower(s), "github.com/"):
			s = "https://" + s
		case looksLikeOwnerRepo(s):
			s = "https://github.com/" + s // owner/repo 简写
		default:
			return source // file 路径或其它非 URL → 原样
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return source
	}
	switch strings.ToLower(u.Hostname()) {
	case "raw.githubusercontent.com":
		return source // 已是 raw
	case "github.com", "www.github.com":
		// 落到下面转换
	default:
		return source // 其它 http(s) 源 → 原样
	}
	path := strings.Trim(u.Path, "/")
	if path == "" {
		return source
	}
	// 走 jsDelivr CDN（国内/全球均比 raw.githubusercontent.com 直连快且稳定，raw 直连国内偶发 >30s 超时）。
	const defaultRef, indexFile = "main", "plugin-market.json"
	const cdnBase = "https://cdn.jsdelivr.net/gh/"
	if i := strings.Index(path, "/tree/"); i >= 0 {
		repoPart := path[:i]
		sub := strings.Trim(path[i+len("/tree/"):], "/") // ref/path...
		if sub == "" {
			return source
		}
		// sub 形如 "ref" 或 "ref/dir1/dir2" → jsDelivr 用 @ref 区分 ref 与路径。
		ref, dir := sub, ""
		if j := strings.Index(sub, "/"); j >= 0 {
			ref, dir = sub[:j], sub[j+1:]+"/"
		}
		return cdnBase + repoPart + "@" + ref + "/" + dir + indexFile
	}
	return cdnBase + path + "@" + defaultRef + "/" + indexFile
}

// looksLikeOwnerRepo 判断 owner/repo 简写（无 scheme、非绝对/相对路径）。
func looksLikeOwnerRepo(s string) bool {
	if strings.Contains(s, "://") || strings.HasPrefix(s, "/") || strings.HasPrefix(s, ".") {
		return false
	}
	parts := strings.Split(s, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	// 排除形如「目录/文件.json」的本地相对路径：repo 名一般不含点。
	return !strings.Contains(parts[1], ".")
}

// resolveMarketSource 解析最终市场源 URL：source 非空时直接归一化；为空时优先取 UI 持久化的
// 市场源（SystemConfig），再回退本地 plugin-market.json。FetchMarket / InstallFromMarket 共用，
// 保证浏览与安装走同一源（archive 相对路径基于此 URL 解析）。
func (m *Manager) resolveMarketSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		if persisted := m.GetMarketSources(); len(persisted) > 0 {
			source = persisted[0]
		} else {
			source = m.DefaultMarketSource()
		}
	}
	return normalizeMarketSource(source)
}

func readSource(ctx context.Context, source string, maxBytes int64) ([]byte, error) {
	if strings.TrimSpace(source) == "" {
		return nil, errors.New("source is required")
	}
	u, err := url.Parse(source)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, err
		}
		client := &http.Client{Timeout: 30 * time.Second}
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, fmt.Errorf("source status %d", res.StatusCode)
		}
		return readLimited(res.Body, maxBytes)
	}
	if err == nil && u.Scheme == "file" {
		source = u.Path
	}
	f, err := os.Open(filepath.Clean(source))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readLimited(f, maxBytes)
}

func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("source exceeds %d bytes", maxBytes)
	}
	return raw, nil
}

func resolveArchiveSource(indexSource, archive string) (string, error) {
	if archive == "" {
		return "", errors.New("archive is required")
	}
	u, err := url.Parse(archive)
	if err == nil && u.Scheme != "" {
		return archive, nil
	}
	idx, err := url.Parse(indexSource)
	if err == nil && (idx.Scheme == "http" || idx.Scheme == "https") {
		return idx.ResolveReference(&url.URL{Path: archive}).String(), nil
	}
	if err == nil && idx.Scheme == "file" {
		indexSource = idx.Path
	}
	if filepath.IsAbs(archive) {
		return archive, nil
	}
	return filepath.Join(filepath.Dir(indexSource), archive), nil
}

// 插件 zip 解压上限（防 zip 炸弹，仿 backup/ptd.go）。单条目略宽于 PTD（插件含 .py/.so 资源）。
