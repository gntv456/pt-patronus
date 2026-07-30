package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxPluginArchiveSize = 50 << 20

func (m *Manager) FetchMarket(ctx context.Context, source string) (*MarketIndex, error) {
	idx, err := m.fetchRawMarket(ctx, source)
	if err != nil {
		return nil, err
	}
	m.finalizeIndex(idx)
	return idx, nil
}

// fetchRawMarket 取单个源的原始索引：归一化地址、读取、反序列化，并给每条 plugin 打上
// 解析后的 Source（供浏览/安装时回传定位其 archive）。不做兼容性/依赖后处理（交由 finalizeIndex）。
func (m *Manager) fetchRawMarket(ctx context.Context, source string) (*MarketIndex, error) {
	source = m.resolveMarketSource(source)
	raw, err := readSource(ctx, source, maxPluginArchiveSize)
	if err != nil {
		// 本地默认市场源（data/plugin-market.json）不存在 = 用户未配置市场源：
		// 返回空市场让前端引导点「设置仓库」配置 GitHub 源，而非把"no such file"当错误抛给用户。
		// （http/远程源失败仍按错误返回——只有本地文件缺失才视为"未配置"。）
		if errors.Is(err, fs.ErrNotExist) {
			return &MarketIndex{Plugins: []MarketPlugin{}}, nil
		}
		return nil, err
	}
	var idx MarketIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("parse market index: %w", err)
	}
	idx.APIVersion = CurrentPluginAPIVersion
	idx.AppVersion = currentAppVersion()
	idx.HostCapabilities = supportedHostCapabilities()
	for i := range idx.Plugins {
		idx.Plugins[i].Source = source
	}
	return &idx, nil
}

// finalizeIndex 对市场索引做兼容性评估、依赖/安装计划等后处理（原地修改）。
func (m *Manager) finalizeIndex(idx *MarketIndex) {
	for i := range idx.Plugins {
		if idx.Plugins[i].APIVersion == "" {
			idx.Plugins[i].APIVersion = CurrentPluginAPIVersion
		}
		idx.Plugins[i].HostCapabilities = effectiveMarketHostCapabilities(idx.Plugins[i])
		idx.Plugins[i].OptionalHostCapabilities = normalizeOptionalHostCapabilities(idx.Plugins[i].OptionalHostCapabilities)
		idx.Plugins[i].Dependencies = normalizeDependencies(idx.Plugins[i].Dependencies)
		compatibility := evaluateMarketCompatibility(idx.Plugins[i])
		idx.Plugins[i].Compatible = compatibility.Compatible
		idx.Plugins[i].CompatibilityReason = compatibility.Reason
		idx.Plugins[i].AvailableOptionalHostCapabilities, idx.Plugins[i].MissingOptionalHostCapabilities = evaluateOptionalHostCapabilities(idx.Plugins[i].OptionalHostCapabilities, idx.HostCapabilities)
		idx.Plugins[i].TrustLevel = trustLevel(idx.PublicKey, idx.Plugins[i])
		idx.Plugins[i].Installed = m.hasInstalled(idx.Plugins[i].ID)
		idx.Plugins[i].InstalledVersion = m.installedVersion(idx.Plugins[i].ID)
		idx.Plugins[i].UpdateAvailable = idx.Plugins[i].Installed && idx.Plugins[i].InstalledVersion != "" && idx.Plugins[i].InstalledVersion != idx.Plugins[i].Version
	}
	planner := newMarketInstallPlanner(m, idx)
	for i := range idx.Plugins {
		item := &idx.Plugins[i]
		if item.Compatible {
			result := planner.resolve(item.ID, nil)
			item.DependenciesStatus = result.statuses
			item.DependenciesSatisfied = result.installable
			item.InstallPlan = result.plan
			item.InstallReason = result.reason
			item.Installable = result.installable
			continue
		}
		statuses, _, depsSatisfied, reason := planner.resolveDependencies(item, []string{item.ID})
		item.DependenciesStatus = statuses
		item.DependenciesSatisfied = depsSatisfied
		item.InstallPlan = []InstallPlanStep{}
		item.Installable = false
		item.InstallReason = item.CompatibilityReason
		if item.InstallReason == "" {
			item.InstallReason = reason
		}
	}
}

// FetchMarketMulti 聚合多个市场源：逐源拉取原始索引，按 ID 合并（先列者优先），
// 统一做兼容性/依赖后处理。sources 为空时解析为持久化配置，再退本地默认源。
// 单个源失败记日志并跳过（容错，单仓挂掉不影响其它）；全部失败才返回首个错误。
func (m *Manager) FetchMarketMulti(ctx context.Context, sources []string) (*MarketIndex, error) {
	resolved := normalizeSources(sources)
	if len(resolved) == 0 {
		resolved = m.GetMarketSources()
	}
	if len(resolved) == 0 {
		if def := m.DefaultMarketSource(); def != "" {
			resolved = []string{def}
		}
	}

	merged := &MarketIndex{
		APIVersion:       CurrentPluginAPIVersion,
		AppVersion:       currentAppVersion(),
		HostCapabilities: supportedHostCapabilities(),
	}
	var groups [][]MarketPlugin
	var firstErr error
	for _, src := range resolved {
		idx, err := m.fetchRawMarket(ctx, src)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("fetch market source skipped", "source", src, "err", err)
			continue
		}
		groups = append(groups, idx.Plugins)
	}
	merged.Plugins = mergeMarketPlugins(merged.Plugins, groups...)
	if len(merged.Plugins) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return merged, nil // 无源且无默认源：空索引，前端显示"暂无市场数据"
	}
	m.finalizeIndex(merged)
	return merged, nil
}

// mergeMarketPlugins 把多组 plugin 按 ID 合并进 out（先列者优先；跳过空 ID），返回合并结果。
// 纯函数，便于单测；不做兼容性/依赖后处理（调用方负责 finalize）。
func mergeMarketPlugins(out []MarketPlugin, groups ...[]MarketPlugin) []MarketPlugin {
	seen := make(map[string]struct{}, len(out))
	for _, p := range out {
		if p.ID != "" {
			seen[p.ID] = struct{}{}
		}
	}
	for _, group := range groups {
		for _, p := range group {
			if p.ID == "" {
				continue
			}
			if _, ok := seen[p.ID]; ok {
				continue // 先列者优先：已存在则跳过
			}
			seen[p.ID] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

func trustLevel(publicKey string, item MarketPlugin) string {
	signed := strings.TrimSpace(publicKey) != "" && strings.TrimSpace(item.Signature) != ""
	verifiedPublisher := item.Publisher != nil && item.Publisher.Verified
	switch {
	case signed && verifiedPublisher:
		return "verified-signed"
	case signed:
		return "signed"
	case verifiedPublisher:
		return "publisher-verified"
	default:
		return "unsigned"
	}
}

type MarketInstallOptions struct {
	AcceptedPermissions         []string
	AcceptedPermissionsByPlugin map[string][]string
	AllowUnsigned               bool // 显式允许本次安装未签名插件（覆盖全局 plugin.allow_unsigned）
}

func (m *Manager) InstallFromMarket(ctx context.Context, source, id string, acceptedPermissions ...[]string) (*InstallResult, error) {
	options := MarketInstallOptions{}
	if len(acceptedPermissions) > 0 {
		options.AcceptedPermissions = acceptedPermissions[0]
	}
	return m.InstallFromMarketWithOptions(ctx, source, id, options)
}

func (m *Manager) InstallFromMarketWithOptions(ctx context.Context, source, id string, options MarketInstallOptions) (*InstallResult, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("plugin id is required")
	}
	source = m.resolveMarketSource(source)
	idx, err := m.FetchMarket(ctx, source)
	if err != nil {
		return nil, err
	}
	var item *MarketPlugin
	for i := range idx.Plugins {
		if idx.Plugins[i].ID == id {
			item = &idx.Plugins[i]
			break
		}
	}
	if item == nil {
		return nil, fmt.Errorf("plugin %s not found in market", id)
	}
	if !item.Compatible {
		reason := strings.TrimSpace(item.CompatibilityReason)
		if reason == "" {
			reason = "plugin is not compatible with the current host"
		}
		return nil, fmt.Errorf("plugin %s cannot be installed: %s", item.ID, reason)
	}
	if !item.Installable {
		reason := strings.TrimSpace(item.InstallReason)
		if reason == "" {
			reason = "install preflight failed"
		}
		return nil, fmt.Errorf("plugin %s cannot be installed: %s", item.ID, reason)
	}
	plan := cloneInstallPlan(item.InstallPlan)
	if len(plan) == 0 {
		return nil, fmt.Errorf("plugin %s cannot be installed: install plan is empty", item.ID)
	}
	for _, step := range plan {
		if err := validateMarketInstallPermissions(step, id, options); err != nil {
			return nil, err
		}
	}
	root := m.PluginRoot()
	if root == "" {
		return nil, errors.New("plugin root is not configured")
	}
	itemsByID := make(map[string]*MarketPlugin, len(idx.Plugins))
	for i := range idx.Plugins {
		itemsByID[idx.Plugins[i].ID] = &idx.Plugins[i]
	}
	var result *InstallResult
	for _, step := range plan {
		if step.Action == installPlanActionReuse {
			if step.ID == id {
				result = &InstallResult{
					ID:            step.ID,
					Name:          step.Name,
					Version:       step.Version,
					InstalledDir:  filepath.Join(root, step.ID),
					RestartNeeded: false,
				}
			}
			continue
		}
		planItem := itemsByID[step.ID]
		if planItem == nil {
			return nil, fmt.Errorf("plugin %s not found in market install plan", step.ID)
		}
		stepResult, err := m.installMarketItem(ctx, source, idx, planItem, options)
		if err != nil {
			return nil, err
		}
		if step.ID == id {
			result = stepResult
		}
	}
	if result == nil {
		result = &InstallResult{
			ID:            item.ID,
			Name:          item.Name,
			Version:       item.Version,
			InstalledDir:  filepath.Join(root, item.ID),
			RestartNeeded: false,
		}
	}
	result.Items = plan
	return result, nil
}

func validateMarketInstallPermissions(step InstallPlanStep, targetID string, options MarketInstallOptions) error {
	if step.Action == installPlanActionReuse {
		return nil
	}
	accepted, ok := acceptedPermissionsFor(step.ID, targetID, options)
	if !ok {
		return fmt.Errorf("accepted permissions for plugin %s are required", step.ID)
	}
	if !PermissionSetEqual(accepted, step.Permissions) {
		return fmt.Errorf("accepted permissions for plugin %s do not match market plugin permissions", step.ID)
	}
	return nil
}

func acceptedPermissionsFor(id, targetID string, options MarketInstallOptions) ([]string, bool) {
	if accepted, ok := options.AcceptedPermissionsByPlugin[id]; ok {
		return accepted, true
	}
	if id == targetID && options.AcceptedPermissions != nil {
		return options.AcceptedPermissions, true
	}
	return nil, false
}

func (m *Manager) installMarketItem(ctx context.Context, source string, idx *MarketIndex, item *MarketPlugin, options MarketInstallOptions) (*InstallResult, error) {
	if item.Runtime != RuntimeExternalHTTP {
		return nil, fmt.Errorf("unsupported market plugin runtime %s", item.Runtime)
	}
	archiveSource, err := resolveArchiveSource(source, item.Archive)
	if err != nil {
		return nil, err
	}
	raw, err := readSource(ctx, archiveSource, maxPluginArchiveSize)
	if err != nil {
		return nil, err
	}
	if item.SHA256 != "" {
		sum := sha256.Sum256(raw)
		got := hex.EncodeToString(sum[:])
		if !strings.EqualFold(got, strings.TrimSpace(item.SHA256)) {
			return nil, fmt.Errorf("sha256 mismatch: got %s", got)
		}
	}
	// 签名验证（P1 安全修复）：用受信公钥集（内置官方 + 用户配置），不再用市场索引自带的 idx.PublicKey
	// ——那是被验证数据本身，攻击者篡改市场源可同时换公钥+签名绕过。未签名需显式 opt-in。
	if err := verifySignatureTrusted(m.TrustedPublicKeys(), item.Signature, raw); err != nil {
		if errors.Is(err, ErrUnsigned) && (options.AllowUnsigned || m.AllowUnsigned()) {
			slog.Warn("installing unsigned plugin (allow_unsigned opt-in)", "id", item.ID, "source", source)
		} else {
			return nil, fmt.Errorf("plugin signature verification failed: %w", err)
		}
	}
	root := m.PluginRoot()
	if root == "" {
		return nil, errors.New("plugin root is not configured")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(root, ".install-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := unzipSafe(raw, tmp); err != nil {
		return nil, err
	}
	manifestPath, err := findManifest(tmp)
	if err != nil {
		return nil, err
	}
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, err
	}
	normalizeManifest(&manifest)
	if err := validateExternalManifest(manifest); err != nil {
		return nil, fmt.Errorf("validate plugin manifest: %w", err)
	}
	if _, ok, reason := m.evaluateDependencies(manifest.Dependencies, false); !ok {
		if strings.TrimSpace(reason) == "" {
			reason = "plugin dependencies are not satisfied"
		}
		return nil, fmt.Errorf("plugin %s cannot be installed: %s", manifest.ID, reason)
	}
	if manifest.ID != item.ID {
		return nil, fmt.Errorf("manifest id mismatch: %s != %s", manifest.ID, item.ID)
	}
	if manifest.Runtime != RuntimeExternalHTTP {
		return nil, fmt.Errorf("manifest runtime %s is not supported", manifest.Runtime)
	}
	// 权限对齐：归档内 manifest 的权限必须与市场索引声明的（用户已确认接受的）完全一致。
	// 签名只覆盖归档字节、不覆盖市场索引元数据；若不校验，受信仓库被劫持改 market index
	// （不动归档→签名仍有效）即可让用户接受的低权集合（如 log:write）静默升级为 host:* 全权。
	if !PermissionSetEqual(manifest.Permissions, item.Permissions) {
		return nil, fmt.Errorf("manifest permissions mismatch: market declared %v but archive has %v (possible permission escalation)", item.Permissions, manifest.Permissions)
	}
	srcDir := filepath.Dir(manifestPath)
	dst := filepath.Join(root, manifest.ID)
	if !isSubpath(root, dst) {
		return nil, errors.New("invalid install path")
	}
	if p, err := m.plugin(manifest.ID); err == nil {
		stopPlugin(p)
	}
	installAction := "install"
	if _, err := os.Stat(dst); err == nil {
		installAction = "upgrade"
		if err := m.ensureCurrentReleaseSnapshot(manifest.ID, dst); err != nil {
			return nil, err
		}
	}
	backup := dst + ".bak-" + time.Now().Format("20060102150405")
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, backup); err != nil {
			return nil, err
		}
		defer os.RemoveAll(backup)
	}
	if err := copyDir(srcDir, dst); err != nil {
		_ = os.RemoveAll(dst)
		if _, statErr := os.Stat(backup); statErr == nil {
			_ = os.Rename(backup, dst)
		}
		return nil, err
	}
	// 安装声明的 Python 依赖到 {dst}/.deps（best-effort：pip 不在就跳过；失败仅记录，不阻断安装）。
	if len(manifest.Requirements) > 0 {
		if err := installPythonRequirements(dst, manifest.Requirements); err != nil {
			fmt.Fprintf(os.Stderr, "plugin %s dependency install failed: %v\n", manifest.ID, err)
		}
	}
	if _, err := m.LoadExternalDir(dst); err != nil {
		_ = os.RemoveAll(dst)
		if _, statErr := os.Stat(backup); statErr == nil {
			_ = os.Rename(backup, dst)
		}
		return nil, err
	}
	if _, err := m.persistReleaseSnapshot(manifest.ID, manifest.Version, installAction, source, archiveSource, item.SHA256, dst, true); err != nil {
		return nil, err
	}
	return &InstallResult{
		ID:            manifest.ID,
		Name:          manifest.Name,
		Version:       manifest.Version,
		InstalledDir:  dst,
		RestartNeeded: false,
	}, nil
}

// ErrUnsigned 表示插件归档无签名（由 verifySignatureTrusted 返回，调用方据 AllowUnsigned 决定是否放行）。
var ErrUnsigned = errors.New("plugin archive is not signed")

// verifySignatureTrusted 用受信公钥集验证插件归档签名：任一受信公钥验过即通过。
// 与 verifySignature（单公钥）的区别：公钥不再取自市场索引（被验证数据本身，可篡改），
// 而是来自内置官方锚 + 用户配置（Manager.TrustedPublicKeys）——独立信任源。
func verifySignatureTrusted(trustedKeys []string, signature string, payload []byte) error {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return ErrUnsigned
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("decode plugin signature: %w", err)
	}
	for _, keyB64 := range trustedKeys {
		keyB64 = strings.TrimSpace(keyB64)
		if keyB64 == "" {
			continue
		}
		pub, err := base64.StdEncoding.DecodeString(keyB64)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			continue
		}
		if ed25519.Verify(ed25519.PublicKey(pub), payload, sig) {
			return nil
		}
	}
	return errors.New("plugin signature is not from a trusted publisher")
}

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
const (
	maxPluginEntrySize int64 = 32 << 20 // 单文件解压上限 32MB
	maxPluginTotalSize int64 = 96 << 20 // 全包累计上限 96MB
)

func unzipSafe(raw []byte, dst string) error {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	// zip 炸弹防护：单条目 + 累计总量双上限。
	var total int64
	for _, f := range zr.File {
		target := filepath.Join(dst, filepath.Clean(f.Name))
		if !isSubpath(dst, target) {
			return fmt.Errorf("unsafe zip path %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		// 预检声明大小（UncompressedSize64 可被伪造，下面 LimitReader 双保险）。
		if int64(f.UncompressedSize64) > maxPluginEntrySize {
			return fmt.Errorf("plugin entry %q too large after decompress (%d bytes, max %d)", f.Name, f.UncompressedSize64, maxPluginEntrySize)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.FileInfo().Mode())
		if err != nil {
			rc.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(rc, maxPluginEntrySize+1))
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if n > maxPluginEntrySize {
			return fmt.Errorf("plugin entry %q exceeds per-file decompress limit", f.Name)
		}
		if closeErr != nil {
			return closeErr
		}
		total += n
		if total > maxPluginTotalSize {
			return fmt.Errorf("plugin archive exceeds total decompress limit (%d bytes)", maxPluginTotalSize)
		}
	}
	return nil
}

func findManifest(root string) (string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "plugin.json" {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return "", errors.New("plugin.json not found in archive")
	}
	if len(found) > 1 {
		return "", errors.New("archive contains multiple plugin.json files")
	}
	return found[0], nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if !isSubpath(dst, target) {
			return fmt.Errorf("unsafe copy path %s", target)
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

// installPythonRequirements 把声明的 pip 需求装到 {pluginDir}/.deps（best-effort）。
// pip/pip3 不在 PATH 时静默跳过（宿主没装 pip 就没法装依赖）；pip 存在但安装失败时返回错误，
// 由调用方记录但不阻断安装（插件可能仅靠 stdlib 仍可运行）。--target 装入 .deps 后，进程启动时
// 由 external.go 把 .deps 注入 PYTHONPATH。
func installPythonRequirements(pluginDir string, reqs []string) error {
	pip, err := exec.LookPath("pip")
	if err != nil {
		if pip, err = exec.LookPath("pip3"); err != nil {
			return nil
		}
	}
	depsDir := filepath.Join(pluginDir, ".deps")
	if err := os.MkdirAll(depsDir, 0o755); err != nil {
		return err
	}
	args := append([]string{"install", "--quiet", "--disable-pip-version-check", "--no-warn-script-location", "--target", depsDir}, reqs...)
	cmd := exec.Command(pip, args...)
	cmd.Dir = pluginDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pip install %s: %w: %s", strings.Join(reqs, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isSubpath(root, target string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
