package plugins

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (p *externalHTTPPlugin) ensureHostLocked() error {
	if p.host != nil {
		return nil
	}
	host, err := newExternalHostServer(p)
	if err != nil {
		return err
	}
	p.host = host
	return nil
}

func (p *externalHTTPPlugin) setRuntime(rt Runtime) {
	p.runtimeMu.Lock()
	defer p.runtimeMu.Unlock()
	p.runtime = rt
}

func (p *externalHTTPPlugin) currentRuntime() Runtime {
	p.runtimeMu.RLock()
	defer p.runtimeMu.RUnlock()
	return p.runtime
}

func (p *externalHTTPPlugin) hostContext() *externalHostContext {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.host == nil {
		return nil
	}
	return &externalHostContext{
		ProtocolVersion:  1,
		PluginID:         p.manifest.ID,
		BaseURL:          p.host.baseURL,
		Token:            p.host.token,
		Permissions:      append([]string(nil), p.manifest.Permissions...),
		HostCapabilities: supportedHostCapabilities(),
	}
}

func (p *externalHTTPPlugin) health(ctx context.Context) error {
	entry := p.manifest.Entry
	path := "/health"
	if entry != nil && entry.Health != "" {
		path = entry.Health
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url(path), nil)
	if err != nil {
		return err
	}
	p.authorize(req)
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("health status %d", res.StatusCode)
	}
	return nil
}

func (p *externalHTTPPlugin) post(ctx context.Context, path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url(path), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	p.authorize(req)
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("external plugin status %d", res.StatusCode)
	}
	if out != nil {
		// 限制解码体积防 OOM（受信插件仍是外部进程，可能回传超大 body）。1<<20 对齐 host 回调侧。
		return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
	}
	return nil
}

func (p *externalHTTPPlugin) authorize(req *http.Request) {
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
}

func (p *externalHTTPPlugin) url(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(p.baseURL, "/") + path
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func replaceVars(s string, port int, token, dir, id string) string {
	repl := strings.NewReplacer(
		"${PORT}", fmt.Sprint(port),
		"${TOKEN}", token,
		"${PLUGIN_DIR}", dir,
		"${PLUGIN_ID}", id,
	)
	return repl.Replace(s)
}

func resolveCommand(dir, cmd string) string {
	if filepath.IsAbs(cmd) {
		return cmd
	}
	candidate := filepath.Join(dir, cmd)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return cmd
}

func resolveWorkDir(dir, workDir string) string {
	if workDir == "" {
		return dir
	}
	if filepath.IsAbs(workDir) {
		return workDir
	}
	return filepath.Join(dir, workDir)
}
