package plugins

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type externalHTTPPlugin struct {
	manifest Manifest
	dir      string
	client   *http.Client

	mu      sync.Mutex
	cmd     *exec.Cmd
	baseURL string
	token   string
}

type externalActionRequest struct {
	Action string         `json:"action"`
	Input  map[string]any `json:"input,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

type externalEventRequest struct {
	Type   string         `json:"type"`
	Data   map[string]any `json:"data,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

type externalActionResponse struct {
	OK     bool           `json:"ok"`
	Output map[string]any `json:"output"`
	Error  string         `json:"error"`
}

func NewExternalHTTPPlugin(manifest Manifest, dir string) Plugin {
	manifest.Runtime = RuntimeExternalHTTP
	manifest.Builtin = false
	return &externalHTTPPlugin{
		manifest: manifest,
		dir:      dir,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *externalHTTPPlugin) Manifest() Manifest {
	return p.manifest
}

func (p *externalHTTPPlugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	p.cmd = nil
	p.baseURL = ""
	p.token = ""
	return nil
}

func (p *externalHTTPPlugin) DefaultConfig() map[string]any {
	out := make(map[string]any, len(p.manifest.ConfigSchema))
	for _, f := range p.manifest.ConfigSchema {
		if f.Default != nil {
			out[f.Key] = f.Default
			continue
		}
		switch f.Type {
		case "switch":
			out[f.Key] = false
		case "number":
			out[f.Key] = 0
		default:
			out[f.Key] = ""
		}
	}
	return out
}

func (p *externalHTTPPlugin) HandleEvent(ctx context.Context, rt Runtime, e Event) error {
	if err := p.ensureStarted(ctx); err != nil {
		return err
	}
	req := externalEventRequest{Type: e.Type, Data: e.Data, Config: rt.Config()}
	return p.post(ctx, "/event", req, nil)
}

func (p *externalHTTPPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	if err := p.ensureStarted(ctx); err != nil {
		return nil, err
	}
	req := externalActionRequest{Action: action, Input: input, Config: rt.Config()}
	var res externalActionResponse
	if err := p.post(ctx, "/action", req, &res); err != nil {
		return nil, err
	}
	if res.Error != "" {
		return nil, errors.New(res.Error)
	}
	if res.Output == nil {
		res.Output = map[string]any{}
	}
	return res.Output, nil
}

func (p *externalHTTPPlugin) ensureStarted(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.baseURL != "" {
		return nil
	}
	entry := p.manifest.Entry
	if entry == nil {
		return errors.New("external plugin entry is missing")
	}
	if entry.BaseURL != "" {
		p.baseURL = strings.TrimRight(entry.BaseURL, "/")
		return p.health(ctx)
	}
	if entry.Command == "" {
		return errors.New("external plugin command or base_url is required")
	}

	port, err := freePort()
	if err != nil {
		return err
	}
	token, err := randomToken()
	if err != nil {
		return err
	}
	p.token = token
	p.baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	env := map[string]string{
		"PTP_PLUGIN_ID":       p.manifest.ID,
		"PTP_PLUGIN_PORT":     fmt.Sprint(port),
		"PTP_PLUGIN_TOKEN":    token,
		"PTP_PLUGIN_BASE_URL": p.baseURL,
		"PTP_PLUGIN_DIR":      p.dir,
	}
	for k, v := range entry.Env {
		env[k] = replaceVars(v, port, token, p.dir, p.manifest.ID)
	}

	cmdPath := replaceVars(entry.Command, port, token, p.dir, p.manifest.ID)
	cmdPath = resolveCommand(p.dir, cmdPath)
	args := make([]string, 0, len(entry.Args))
	for _, arg := range entry.Args {
		args = append(args, replaceVars(arg, port, token, p.dir, p.manifest.ID))
	}
	cmd := exec.Command(cmdPath, args...)
	cmd.Dir = resolveWorkDir(p.dir, entry.WorkDir)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if err := cmd.Start(); err != nil {
		p.baseURL = ""
		return err
	}
	p.cmd = cmd
	go func() { _ = cmd.Wait() }()

	deadline := time.Now().Add(8 * time.Second)
	for {
		if err := p.health(ctx); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("external plugin %s health check timeout", p.manifest.ID)
		}
		time.Sleep(250 * time.Millisecond)
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
		return json.NewDecoder(res.Body).Decode(out)
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
