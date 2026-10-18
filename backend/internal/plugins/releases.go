package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func (m *Manager) ListReleases(pluginID string, limit int) ([]ReleaseView, error) {
	releases, err := m.op.ListReleases(pluginID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ReleaseView, 0, len(releases))
	for _, rel := range releases {
		out = append(out, releaseView(rel))
	}
	return out, nil
}

func (m *Manager) Rollback(ctx context.Context, pluginID string, releaseID uint) (*View, error) {
	rel, err := m.op.GetRelease(releaseID)
	if err != nil {
		return nil, err
	}
	if rel.PluginID != pluginID {
		return nil, ErrReleaseNotForPlugin
	}
	root := m.PluginRoot()
	if root == "" {
		return nil, ErrPluginRootUnconfigured
	}
	dst := filepath.Join(root, pluginID)
	if !isSubpath(root, dst) {
		return nil, errors.New("invalid rollback path")
	}
	if _, err := os.Stat(rel.SnapshotDir); err != nil {
		return nil, err
	}
	if p, err := m.plugin(pluginID); err == nil {
		stopPlugin(p)
	}
	backup := dst + ".rollback-" + time.Now().Format("20060102150405")
	// keepBackupOnFailure：放回原位也失败时，那份改名出去的旧目录必须留着——
	// 从前是无条件 defer RemoveAll(backup)，那会把唯一一份旧代码一起删掉。
	keepBackupOnFailure := false
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, backup); err != nil {
			return nil, err
		}
		defer func() {
			if !keepBackupOnFailure {
				_ = os.RemoveAll(backup)
			}
		}()
	}
	if err := copyDir(rel.SnapshotDir, dst); err != nil {
		if rerr := restoreRollbackBackup(dst, backup); rerr != nil {
			keepBackupOnFailure = true
			return nil, rerr
		}
		return nil, err
	}
	if _, err := m.LoadExternalDir(dst); err != nil {
		if rerr := restoreRollbackBackup(dst, backup); rerr != nil {
			keepBackupOnFailure = true
			return nil, rerr
		}
		return nil, err
	}
	if err := m.op.SetActiveRelease(pluginID, releaseID); err != nil {
		// 磁盘已经换成旧版本，账面却没记下当前版本——两份读数会长期不一致，不能只回「参数有误」。
		return nil, fmt.Errorf("%w: %v", ErrRollbackLedgerNotWritten, err)
	}
	m.log(pluginID, logInfo, "release.rollback", fmt.Sprintf("plugin rolled back to %s", rel.Version), map[string]any{"release_id": releaseID})
	return m.Get(pluginID)
}

// restoreRollbackBackup 回滚失败后把改名出去的旧目录放回原位。
// 放回也失败时必须把它带回错误里：此刻磁盘上既没有新版也没有旧版，
// 只说「回滚失败，请重试」会让人反复点，而旧代码其实还留在那个备份目录里。
func restoreRollbackBackup(dst, backup string) error {
	_ = os.RemoveAll(dst)
	if _, err := os.Stat(backup); err != nil {
		// 原本就没有插件目录（新装后首次回滚失败）——清空即回到初态，不算二次损失。
		return nil
	}
	if err := os.Rename(backup, dst); err != nil {
		return fmt.Errorf("%w (old version kept at %s): %v", ErrRollbackRestoreFailed, backup, err)
	}
	return nil
}

func (m *Manager) persistReleaseSnapshot(pluginID, version, action, source, archive, sha256, srcDir string, active bool) (*models.PluginRelease, error) {
	root, err := m.releaseRoot(pluginID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	name := time.Now().Format("20060102150405") + "-" + releaseSlug(version)
	snapshotDir := filepath.Join(root, name)
	if err := copyDir(srcDir, snapshotDir); err != nil {
		_ = os.RemoveAll(snapshotDir)
		return nil, err
	}
	rel := &models.PluginRelease{
		PluginID:    pluginID,
		Version:     strings.TrimSpace(version),
		Action:      strings.TrimSpace(action),
		Source:      source,
		Archive:     archive,
		SHA256:      sha256,
		SnapshotDir: snapshotDir,
		Active:      active,
	}
	if rel.Version == "" {
		rel.Version = "unknown"
	}
	if rel.Action == "" {
		rel.Action = "install"
	}
	if err := m.op.CreateRelease(rel); err != nil {
		_ = os.RemoveAll(snapshotDir)
		return nil, err
	}
	return rel, nil
}

func (m *Manager) ensureCurrentReleaseSnapshot(pluginID string, srcDir string) error {
	_, err := m.op.ActiveRelease(pluginID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	manifest, err := readInstalledManifest(srcDir)
	if err != nil {
		return err
	}
	_, err = m.persistReleaseSnapshot(pluginID, manifest.Version, "install", "local", "", "", srcDir, true)
	return err
}

func (m *Manager) releaseRoot(pluginID string) (string, error) {
	root := m.PluginRoot()
	if root == "" {
		return "", errors.New("plugin root is not configured")
	}
	dir := filepath.Join(filepath.Dir(root), "plugin-releases", pluginID)
	if !isSubpath(filepath.Dir(root), dir) {
		return "", errors.New("invalid release root")
	}
	return dir, nil
}

func readInstalledManifest(dir string) (Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, err
	}
	normalizeManifest(&manifest)
	return manifest, nil
}

func releaseSlug(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "unknown"
	}
	repl := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	return repl.Replace(version)
}

func releaseView(rel models.PluginRelease) ReleaseView {
	return ReleaseView{
		ID:          rel.ID,
		PluginID:    rel.PluginID,
		Version:     rel.Version,
		Action:      rel.Action,
		Source:      rel.Source,
		Archive:     rel.Archive,
		SHA256:      rel.SHA256,
		SnapshotDir: rel.SnapshotDir,
		Active:      rel.Active,
		CreatedAt:   rel.CreatedAt,
	}
}
