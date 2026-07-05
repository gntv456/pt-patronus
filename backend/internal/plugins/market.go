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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxPluginArchiveSize = 50 << 20

func (m *Manager) FetchMarket(ctx context.Context, source string) (*MarketIndex, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = m.DefaultMarketSource()
	}
	raw, err := readSource(ctx, source, maxPluginArchiveSize)
	if err != nil {
		return nil, err
	}
	var idx MarketIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("parse market index: %w", err)
	}
	for i := range idx.Plugins {
		idx.Plugins[i].Installed = m.hasInstalled(idx.Plugins[i].ID)
	}
	return &idx, nil
}

func (m *Manager) InstallFromMarket(ctx context.Context, source, id string) (*InstallResult, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("plugin id is required")
	}
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
	if err := verifySignature(idx.PublicKey, item.Signature, raw); err != nil {
		return nil, err
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
	if manifest.ID != item.ID {
		return nil, fmt.Errorf("manifest id mismatch: %s != %s", manifest.ID, item.ID)
	}
	if manifest.Runtime != RuntimeExternalHTTP {
		return nil, fmt.Errorf("manifest runtime %s is not supported", manifest.Runtime)
	}
	srcDir := filepath.Dir(manifestPath)
	dst := filepath.Join(root, manifest.ID)
	if !isSubpath(root, dst) {
		return nil, errors.New("invalid install path")
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
	if _, err := m.LoadExternalDir(dst); err != nil {
		_ = os.RemoveAll(dst)
		if _, statErr := os.Stat(backup); statErr == nil {
			_ = os.Rename(backup, dst)
		}
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

func verifySignature(publicKey, signature string, payload []byte) error {
	publicKey = strings.TrimSpace(publicKey)
	signature = strings.TrimSpace(signature)
	if publicKey == "" && signature == "" {
		return nil
	}
	if publicKey == "" || signature == "" {
		return errors.New("market public_key and plugin signature must be provided together")
	}
	pub, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return fmt.Errorf("decode market public key: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("decode plugin signature: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid ed25519 public key size %d", len(pub))
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), payload, sig) {
		return errors.New("plugin archive signature verification failed")
	}
	return nil
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

func unzipSafe(raw []byte, dst string) error {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
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
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
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
