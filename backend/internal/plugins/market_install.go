package plugins

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
		return nil, fmt.Errorf("%w：%w", ErrMarketUnreachable, err)
	}
	var item *MarketPlugin
	for i := range idx.Plugins {
		if idx.Plugins[i].ID == id {
			item = &idx.Plugins[i]
			break
		}
	}
	if item == nil {
		return nil, fmt.Errorf("%w：%s", ErrPluginNotInMarket, id)
	}
	if !item.Compatible {
		reason := strings.TrimSpace(item.CompatibilityReason)
		if reason == "" {
			reason = "plugin is not compatible with the current host"
		}
		return nil, fmt.Errorf("%w：不兼容原因：%s", ErrPluginNotInstallable, reason)
	}
	if !item.Installable {
		reason := strings.TrimSpace(item.InstallReason)
		if reason == "" {
			reason = "install preflight failed"
		}
		return nil, fmt.Errorf("%w：%s", ErrPluginNotInstallable, reason)
	}
	plan := cloneInstallPlan(item.InstallPlan)
	if len(plan) == 0 {
		return nil, fmt.Errorf("%w：市场给出的安装计划是空的（%s）", ErrPluginNotInstallable, item.ID)
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
			return nil, fmt.Errorf("%w：安装计划引用了市场里不存在的 %s", ErrPluginNotInMarket, step.ID)
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
		return fmt.Errorf("%w：缺少 %s 的权限确认", ErrPermissionsUnconfirmed, step.ID)
	}
	if !PermissionSetEqual(accepted, step.Permissions) {
		return fmt.Errorf("%w：已确认的权限集合与市场声明的不一致（%s）", ErrPermissionsUnconfirmed, step.ID)
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
		return nil, fmt.Errorf("%w：市场声明的运行时 %s 本机不支持", ErrPluginNotInstallable, item.Runtime)
	}
	archiveSource, err := resolveArchiveSource(source, item.Archive)
	if err != nil {
		return nil, fmt.Errorf("%w：归档地址无法解析：%w", ErrMarketUnreachable, err)
	}
	raw, err := readSource(ctx, archiveSource, maxPluginArchiveSize)
	if err != nil {
		return nil, fmt.Errorf("%w：取插件包失败：%w", ErrMarketUnreachable, err)
	}
	if item.SHA256 != "" {
		sum := sha256.Sum256(raw)
		got := hex.EncodeToString(sum[:])
		if !strings.EqualFold(got, strings.TrimSpace(item.SHA256)) {
			// 接口层只把这个原因写到 Debug 日志（4xx 档位），而篡改证据不能等到有人开 debug 才留痕。
			slog.Warn("plugin package sha256 mismatch", "id", item.ID, "source", source, "want", strings.TrimSpace(item.SHA256), "got", got)
			return nil, fmt.Errorf("%w：市场声明 sha256=%s，实际=%s", ErrUntrustedPackage, strings.TrimSpace(item.SHA256), got)
		}
	}
	// 签名验证（P1 安全修复）：用受信公钥集（内置官方 + 用户配置），不再用市场索引自带的 idx.PublicKey
	// ——那是被验证数据本身，攻击者篡改市场源可同时换公钥+签名绕过。未签名需显式 opt-in。
	if err := verifySignatureTrusted(m.TrustedPublicKeys(), item.Signature, raw); err != nil {
		if errors.Is(err, ErrUnsigned) && (options.AllowUnsigned || m.AllowUnsigned()) {
			slog.Warn("installing unsigned plugin (allow_unsigned opt-in)", "id", item.ID, "source", source)
		} else {
			slog.Warn("plugin package signature rejected", "id", item.ID, "source", source, "err", err)
			return nil, fmt.Errorf("%w：签名验证失败：%w", ErrUntrustedPackage, err)
		}
	}
	root := m.PluginRoot()
	if root == "" {
		return nil, fmt.Errorf("%w：插件根目录未配置", ErrInstallWriteFailed)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("%w：建插件根目录失败：%w", ErrInstallWriteFailed, err)
	}
	tmp, err := os.MkdirTemp(root, ".install-*")
	if err != nil {
		return nil, fmt.Errorf("%w：建临时解压目录失败：%w", ErrInstallWriteFailed, err)
	}
	defer os.RemoveAll(tmp)
	if err := unzipSafe(raw, tmp); err != nil {
		return nil, fmt.Errorf("%w：解压插件包失败：%w", ErrPackageInvalid, err)
	}
	manifestPath, err := findManifest(tmp)
	if err != nil {
		return nil, fmt.Errorf("%w：%w", ErrPackageInvalid, err)
	}
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("%w：读 manifest 失败：%w", ErrPackageInvalid, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, fmt.Errorf("%w：manifest 不是合法 JSON：%w", ErrPackageInvalid, err)
	}
	normalizeManifest(&manifest)
	if err := validateExternalManifest(manifest); err != nil {
		return nil, fmt.Errorf("%w：manifest 不合规：%w", ErrPackageInvalid, err)
	}
	if _, ok, reason := m.evaluateDependencies(manifest.Dependencies, false); !ok {
		if strings.TrimSpace(reason) == "" {
			reason = "plugin dependencies are not satisfied"
		}
		return nil, fmt.Errorf("%w：%s", ErrPluginNotInstallable, reason)
	}
	if manifest.ID != item.ID {
		return nil, fmt.Errorf("%w：包内 id=%s 与市场声明的 %s 不一致", ErrUntrustedPackage, manifest.ID, item.ID)
	}
	if manifest.Runtime != RuntimeExternalHTTP {
		return nil, fmt.Errorf("%w：包内声明的运行时 %s 本机不支持", ErrPluginNotInstallable, manifest.Runtime)
	}
	// 权限对齐：归档内 manifest 的权限必须与市场索引声明的（用户已确认接受的）完全一致。
	// 签名只覆盖归档字节、不覆盖市场索引元数据；若不校验，受信仓库被劫持改 market index
	// （不动归档→签名仍有效）即可让用户接受的低权集合（如 log:write）静默升级为 host:* 全权。
	if !PermissionSetEqual(manifest.Permissions, item.Permissions) {
		slog.Warn("plugin package permission mismatch", "id", item.ID, "market", item.Permissions, "archive", manifest.Permissions)
		return nil, fmt.Errorf("%w：市场声明权限 %v 与包内权限 %v 不一致（疑似权限提升）", ErrUntrustedPackage, item.Permissions, manifest.Permissions)
	}
	srcDir := filepath.Dir(manifestPath)
	dst := filepath.Join(root, manifest.ID)
	if !isSubpath(root, dst) {
		return nil, fmt.Errorf("%w：安装目录越出插件根（%s）", ErrInstallWriteFailed, manifest.ID)
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
		return nil, fmt.Errorf("%w：写入插件目录失败：%w", ErrInstallWriteFailed, err)
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
		return nil, fmt.Errorf("%w：插件加载失败（已回滚到上一版）：%w", ErrInstallWriteFailed, err)
	}
	if _, err := m.persistReleaseSnapshot(manifest.ID, manifest.Version, installAction, source, archiveSource, item.SHA256, dst, true); err != nil {
		// 插件已装上也加载成功，只是版本台账没记上：必须说清「已装上」，
		// 否则用户以为失败，重来一遍反而撞上「已存在」。
		return nil, fmt.Errorf("%w：插件已装上但版本记录失败：%w", ErrInstallWriteFailed, err)
	}
	// 百鸟朝凤数据源：安装成功记一次事件（老树洞累计/蹦蹦新窝最近）。best-effort 不阻塞安装结果。
	m.mu.RLock()
	mop := m.marketOp
	m.mu.RUnlock()
	if mop != nil {
		_ = mop.RecordInstall(manifest.ID, manifest.Version, source)
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
