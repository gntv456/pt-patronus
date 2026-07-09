package plugins

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

type externalHTTPPlugin struct {
	manifest Manifest
	dir      string
	client   *http.Client

	mu      sync.Mutex
	cmd     *exec.Cmd
	baseURL string
	token   string
	host    *externalHostServer
	pid     int

	startedAt     *time.Time
	stoppedAt     *time.Time
	lastHealthAt  *time.Time
	lastRestartAt *time.Time
	nextRestartAt *time.Time
	lastExit      string
	restartCount  int
	stopping      bool
	restarting    bool
	manager       *Manager

	runtimeMu sync.RWMutex
	runtime   Runtime
}

type externalActionRequest struct {
	Action string               `json:"action"`
	Input  map[string]any       `json:"input,omitempty"`
	Config map[string]any       `json:"config,omitempty"`
	Host   *externalHostContext `json:"host,omitempty"`
}

type externalEventRequest struct {
	Type   string               `json:"type"`
	Data   map[string]any       `json:"data,omitempty"`
	Config map[string]any       `json:"config,omitempty"`
	Host   *externalHostContext `json:"host,omitempty"`
}

type externalActionResponse struct {
	OK     bool           `json:"ok"`
	Output map[string]any `json:"output"`
	Error  string         `json:"error"`
}

type externalHostContext struct {
	ProtocolVersion  int      `json:"protocol_version"`
	PluginID         string   `json:"plugin_id"`
	BaseURL          string   `json:"base_url"`
	Token            string   `json:"token"`
	Permissions      []string `json:"permissions,omitempty"`
	HostCapabilities []string `json:"host_capabilities,omitempty"`
}

func NewExternalHTTPPlugin(manifest Manifest, dir string, manager *Manager) Plugin {
	manifest.Runtime = RuntimeExternalHTTP
	manifest.Builtin = false
	return &externalHTTPPlugin{
		manifest: manifest,
		dir:      dir,
		client:   &http.Client{Timeout: 30 * time.Second},
		manager:  manager,
	}
}

func (p *externalHTTPPlugin) Manifest() Manifest {
	return p.manifest
}

func (p *externalHTTPPlugin) RuntimeStatus() RuntimeStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := "stopped"
	if p.baseURL != "" {
		state = "running"
	}
	if p.cmd != nil && p.cmd.ProcessState != nil && p.cmd.ProcessState.Exited() {
		state = "exited"
	}
	hostBaseURL := ""
	if p.host != nil {
		hostBaseURL = p.host.baseURL
	}
	managed := p.manifest.Entry != nil && strings.TrimSpace(p.manifest.Entry.Command) != ""
	return RuntimeStatus{
		State:         state,
		Managed:       managed,
		PID:           p.pid,
		BaseURL:       p.baseURL,
		HostBaseURL:   hostBaseURL,
		RestartCount:  p.restartCount,
		StartedAt:     cloneTimePtr(p.startedAt),
		StoppedAt:     cloneTimePtr(p.stoppedAt),
		LastHealthAt:  cloneTimePtr(p.lastHealthAt),
		LastRestartAt: cloneTimePtr(p.lastRestartAt),
		NextRestartAt: cloneTimePtr(p.nextRestartAt),
		LastExit:      p.lastExit,
	}
}

func (p *externalHTTPPlugin) Stop() error {
	p.mu.Lock()
	p.stopping = true
	p.restarting = false
	p.nextRestartAt = nil
	cmd := p.cmd
	host := p.host
	shutdownPath := ""
	if p.manifest.Entry != nil {
		shutdownPath = p.manifest.Entry.Shutdown
	}
	p.mu.Unlock()
	if shutdownPath != "" {
		_ = p.shutdown(shutdownPath)
		time.Sleep(500 * time.Millisecond)
	}
	p.mu.Lock()
	if p.cmd == cmd {
		p.cmd = nil
		p.baseURL = ""
		p.token = ""
		p.pid = 0
	}
	if p.host == host {
		p.host = nil
	}
	now := time.Now()
	p.stoppedAt = &now
	if p.lastExit == "" {
		p.lastExit = "stopped"
	}
	p.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if host != nil {
		_ = host.stop()
	}
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
	if err := p.ensureStarted(ctx, rt); err != nil {
		return err
	}
	req := externalEventRequest{Type: e.Type, Data: e.Data, Config: rt.Config(), Host: p.hostContext()}
	return p.post(ctx, "/event", req, nil)
}

func (p *externalHTTPPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	if err := p.ensureStarted(ctx, rt); err != nil {
		return nil, err
	}
	req := externalActionRequest{Action: action, Input: input, Config: rt.Config(), Host: p.hostContext()}
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

func (p *externalHTTPPlugin) ensureStarted(ctx context.Context, rt Runtime) error {
	p.setRuntime(rt)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopping = false
	p.restarting = false
	p.nextRestartAt = nil

	if err := p.ensureHostLocked(); err != nil {
		return err
	}
	if p.baseURL != "" {
		return nil
	}
	entry := p.manifest.Entry
	if entry == nil {
		return errors.New("external plugin entry is missing")
	}
	if entry.BaseURL != "" {
		p.baseURL = strings.TrimRight(entry.BaseURL, "/")
		if err := p.health(ctx); err != nil {
			return err
		}
		now := time.Now()
		p.startedAt = &now
		p.lastRestartAt = &now
		p.lastHealthAt = &now
		p.stoppedAt = nil
		p.lastExit = ""
		return nil
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
	if p.host != nil {
		env["PTP_HOST_URL"] = p.host.baseURL
		env["PTP_HOST_TOKEN"] = p.host.token
		env["PTP_HOST_PROTOCOL"] = "1"
		env["PTP_HOST_CAPABILITIES"] = strings.Join(supportedHostCapabilities(), ",")
	}
	for k, v := range entry.Env {
		env[k] = replaceVars(v, port, token, p.dir, p.manifest.ID)
	}
	// 让声明的 pip 依赖（安装到 {dir}/.deps）可被 import：prepend 到已有 PYTHONPATH。
	depsPath := filepath.Join(p.dir, ".deps")
	if existing := os.Getenv("PYTHONPATH"); existing != "" {
		env["PYTHONPATH"] = depsPath + string(os.PathListSeparator) + existing
	} else {
		env["PYTHONPATH"] = depsPath
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
	cmd.Stdout = &pluginOutputWriter{plugin: p, level: logInfo, eventName: "process.stdout"}
	cmd.Stderr = &pluginOutputWriter{plugin: p, level: logError, eventName: "process.stderr"}
	if err := cmd.Start(); err != nil {
		p.baseURL = ""
		return err
	}
	p.cmd = cmd
	p.pid = cmd.Process.Pid
	now := time.Now()
	p.startedAt = &now
	p.lastRestartAt = &now
	p.stoppedAt = nil
	p.lastExit = ""
	go func() {
		err := cmd.Wait()
		p.recordExit(cmd, err)
	}()

	deadline := time.Now().Add(8 * time.Second)
	for {
		if err := p.health(ctx); err == nil {
			now := time.Now()
			p.lastHealthAt = &now
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("external plugin %s health check timeout", p.manifest.ID)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (p *externalHTTPPlugin) recordExit(cmd *exec.Cmd, err error) {
	p.mu.Lock()
	if p.cmd != cmd {
		p.mu.Unlock()
		return
	}
	host := p.host
	p.cmd = nil
	p.baseURL = ""
	p.token = ""
	p.host = nil
	p.pid = 0
	now := time.Now()
	p.stoppedAt = &now
	p.nextRestartAt = nil
	if err != nil {
		p.lastExit = err.Error()
	} else {
		p.lastExit = "exited"
	}
	shouldRestart := !p.stopping && p.manifest.Entry != nil && strings.TrimSpace(p.manifest.Entry.Command) != "" && p.manager != nil && p.manager.pluginEnabled(p.manifest.ID)
	p.mu.Unlock()
	if host != nil {
		_ = host.stop()
	}
	if shouldRestart {
		p.scheduleRestart()
	}
}

func (p *externalHTTPPlugin) scheduleRestart() {
	p.mu.Lock()
	if p.restarting || p.stopping {
		p.mu.Unlock()
		return
	}
	p.restarting = true
	backoff := restartDelay(p.restartCount)
	next := time.Now().Add(backoff)
	p.nextRestartAt = &next
	p.restartCount++
	restartCount := p.restartCount
	manager := p.manager
	pluginID := p.manifest.ID
	p.mu.Unlock()
	if manager != nil {
		manager.log(pluginID, logWarning, "process.restart", fmt.Sprintf("plugin restart scheduled in %s", backoff), map[string]any{"restart_count": restartCount})
	}
	time.AfterFunc(backoff, func() {
		p.mu.Lock()
		stopping := p.stopping
		p.mu.Unlock()
		if stopping || manager == nil || !manager.pluginEnabled(pluginID) {
			p.mu.Lock()
			p.restarting = false
			p.nextRestartAt = nil
			p.mu.Unlock()
			return
		}
		rt := p.currentRuntime()
		if rt == nil {
			p.mu.Lock()
			p.restarting = false
			p.nextRestartAt = nil
			p.mu.Unlock()
			return
		}
		if err := p.ensureStarted(context.Background(), rt); err != nil {
			p.mu.Lock()
			p.restarting = false
			p.lastExit = "restart failed: " + err.Error()
			p.mu.Unlock()
			if manager != nil {
				manager.log(pluginID, logError, "process.restart", err.Error(), nil)
			}
			p.scheduleRestart()
			return
		}
		if manager != nil {
			manager.log(pluginID, logInfo, "process.restart", "plugin process restarted", map[string]any{"restart_count": restartCount})
		}
	})
}

func restartDelay(restartCount int) time.Duration {
	switch {
	case restartCount <= 0:
		return time.Second
	case restartCount == 1:
		return 2 * time.Second
	case restartCount == 2:
		return 5 * time.Second
	case restartCount == 3:
		return 10 * time.Second
	default:
		return 30 * time.Second
	}
}

func (p *externalHTTPPlugin) shutdown(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url(path), nil)
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
		return fmt.Errorf("shutdown status %d", res.StatusCode)
	}
	return nil
}

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

type externalHostServer struct {
	plugin  *externalHTTPPlugin
	server  *http.Server
	baseURL string
	token   string
}

func newExternalHostServer(plugin *externalHTTPPlugin) (*externalHostServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token, err := randomToken()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	h := &externalHostServer{
		plugin:  plugin,
		baseURL: "http://" + ln.Addr().String(),
		token:   token,
	}
	h.server = &http.Server{Handler: h}
	go func() {
		if err := h.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// Best effort: plugin host callbacks are loopback-only and surfaced through plugin calls.
		}
	}()
	return h, nil
}

func (h *externalHostServer) stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return h.server.Shutdown(ctx)
}

func (h *externalHostServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" {
		writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if !h.authorized(r) {
		writeHostJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	rt := h.plugin.currentRuntime()
	if rt == nil {
		writeHostJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "plugin runtime unavailable"})
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/capabilities":
		writeHostJSON(w, http.StatusOK, map[string]any{
			"api_version":       CurrentPluginAPIVersion,
			"plugin_id":         h.plugin.manifest.ID,
			"permissions":       h.plugin.manifest.Permissions,
			"host_capabilities": supportedHostCapabilities(),
			"endpoints": []string{
				"GET /capabilities",
				"GET /config",
				"POST /runtime/config",
				"POST /log",
				"POST /notice",
				"POST /events",
				"GET /kv/{key}",
				"PUT /kv/{key}",
				"DELETE /kv/{key}",
				"GET /sites",
				"GET /sites/{id}/cookie",
				"POST /media/search",
			},
		})
	case r.Method == http.MethodGet && r.URL.Path == "/config":
		if !h.requirePermission(w, "config:read") {
			return
		}
		writeHostJSON(w, http.StatusOK, map[string]any{"config": rt.Config()})
	case r.Method == http.MethodPost && r.URL.Path == "/runtime/config":
		if !h.requirePermission(w, "config:write") {
			return
		}
		h.handleRuntimeConfig(w, r, rt)
	case r.Method == http.MethodPost && r.URL.Path == "/log":
		if !h.requirePermission(w, "log:write") {
			return
		}
		h.handleLog(w, r, rt)
	case r.Method == http.MethodPost && r.URL.Path == "/notice":
		if !h.requirePermission(w, "notice:write") {
			return
		}
		h.handleNotice(w, r, rt)
	case r.Method == http.MethodPost && r.URL.Path == "/events":
		if !h.requirePermission(w, "event:publish") {
			return
		}
		h.handlePublishEvent(w, r, rt)
	case strings.HasPrefix(r.URL.Path, "/kv/"):
		h.handleKV(w, r, rt)
	case r.Method == http.MethodGet && r.URL.Path == "/sites":
		if !h.requirePermission(w, "site:read") {
			return
		}
		writeHostJSON(w, http.StatusOK, map[string]any{"sites": rt.SiteList()})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/sites/"):
		if !h.requirePermission(w, "site:cookie") {
			return
		}
		h.handleSiteCookie(w, r, rt)
	case r.Method == http.MethodPost && r.URL.Path == "/media/search":
		if !h.requirePermission(w, "media:search") {
			return
		}
		h.handleMediaSearch(w, r)
	default:
		writeHostJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func (h *externalHostServer) handleMediaSearch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Keyword string `json:"keyword"`
		Year    int    `json:"year"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
		return
	}
	results, err := h.plugin.manager.MediaSearch(r.Context(), strings.TrimSpace(body.Keyword), body.Year)
	if err != nil {
		writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (h *externalHostServer) authorized(r *http.Request) bool {
	token := strings.TrimSpace(h.token)
	if token == "" {
		return true
	}
	if r.Header.Get("Authorization") == "Bearer "+token {
		return true
	}
	return r.Header.Get("X-PTP-Host-Token") == token
}

func (h *externalHostServer) requirePermission(w http.ResponseWriter, required string) bool {
	if hasPluginPermission(h.plugin.manifest.Permissions, required) {
		return true
	}
	writeHostJSON(w, http.StatusForbidden, map[string]any{
		"error":      "permission denied",
		"permission": required,
	})
	return false
}

func hasPluginPermission(perms []string, required string) bool {
	required = strings.TrimSpace(required)
	for _, perm := range perms {
		perm = strings.TrimSpace(perm)
		switch perm {
		case "*", "host:*", "plugin:*", required:
			return true
		}
		if strings.HasSuffix(perm, ":*") {
			prefix := strings.TrimSuffix(perm, "*")
			if strings.HasPrefix(required, prefix) {
				return true
			}
		}
	}
	return false
}

func (h *externalHostServer) handleLog(w http.ResponseWriter, r *http.Request, rt Runtime) {
	var in struct {
		Level   string         `json:"level"`
		Event   string         `json:"event"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if !decodeHostJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Level) == "" {
		in.Level = logInfo
	}
	if strings.TrimSpace(in.Event) == "" {
		in.Event = "plugin.host"
	}
	rt.Log(in.Level, in.Event, in.Message, in.Data)
	writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *externalHostServer) handleNotice(w http.ResponseWriter, r *http.Request, rt Runtime) {
	var in NoticeInput
	if !decodeHostJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Level) == "" {
		in.Level = logInfo
	}
	if err := rt.Notice(r.Context(), in); err != nil {
		writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *externalHostServer) handlePublishEvent(w http.ResponseWriter, r *http.Request, rt Runtime) {
	var in Event
	if !decodeHostJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Type) == "" {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "type is required"})
		return
	}
	rt.Publish(r.Context(), in.Type, in.Data)
	writeHostJSON(w, http.StatusOK, map[string]any{"published": true, "type": in.Type})
}

func (h *externalHostServer) handleRuntimeConfig(w http.ResponseWriter, r *http.Request, rt Runtime) {
	var in struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	if !decodeHostJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Key) == "" {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "key is required"})
		return
	}
	if err := rt.SetRuntimeConfig(in.Key, in.Value); err != nil {
		writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *externalHostServer) handleKV(w http.ResponseWriter, r *http.Request, rt Runtime) {
	key, err := hostPathValue(r.URL.Path, "/kv/")
	if err != nil || key == "" {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "key is required"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.requirePermission(w, "kv:read") {
			return
		}
		var value any
		err := rt.KVGet(key, &value)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeHostJSON(w, http.StatusOK, map[string]any{"found": false})
			return
		}
		if err != nil {
			writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeHostJSON(w, http.StatusOK, map[string]any{"found": true, "value": value})
	case http.MethodPut, http.MethodPost:
		if !h.requirePermission(w, "kv:write") {
			return
		}
		var in struct {
			Value any `json:"value"`
		}
		if !decodeHostJSON(w, r, &in) {
			return
		}
		if err := rt.KVSet(key, in.Value); err != nil {
			writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		if !h.requirePermission(w, "kv:delete") {
			return
		}
		if err := rt.KVDelete(key); err != nil {
			writeHostJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeHostJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeHostJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (h *externalHostServer) handleSiteCookie(w http.ResponseWriter, r *http.Request, rt Runtime) {
	rest, err := hostPathValue(r.URL.Path, "/sites/")
	if err != nil {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid site path"})
		return
	}
	if !strings.HasSuffix(rest, "/cookie") {
		writeHostJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	idText := strings.TrimSuffix(rest, "/cookie")
	id, err := strconv.ParseUint(strings.Trim(idText, "/"), 10, 64)
	if err != nil {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid site id"})
		return
	}
	cookie, err := rt.SiteCookie(uint(id))
	if err != nil {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeHostJSON(w, http.StatusOK, map[string]any{"cookie": cookie})
}

func hostPathValue(path, prefix string) (string, error) {
	raw := strings.TrimPrefix(path, prefix)
	value, err := url.PathUnescape(raw)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func decodeHostJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	if r.Body == nil {
		return true
	}
	defer r.Body.Close()
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(out); err != nil {
		writeHostJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return false
	}
	return true
}

func writeHostJSON(w http.ResponseWriter, status int, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		status = http.StatusInternalServerError
		raw = []byte(`{"error":"json encode failed"}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

type pluginOutputWriter struct {
	plugin    *externalHTTPPlugin
	level     string
	eventName string
	mu        sync.Mutex
	buf       []byte
}

func (w *pluginOutputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	data := append(w.buf, p...)
	lines := bytes.SplitAfter(data, []byte("\n"))
	w.buf = w.buf[:0]
	for i, line := range lines {
		if i == len(lines)-1 && !bytes.HasSuffix(line, []byte("\n")) {
			w.buf = append(w.buf, line...)
			continue
		}
		w.logLine(line)
	}
	if len(w.buf) > 4096 {
		w.logLine(w.buf)
		w.buf = w.buf[:0]
	}
	return len(p), nil
}

func (w *pluginOutputWriter) logLine(line []byte) {
	msg := strings.TrimSpace(string(line))
	if msg == "" {
		return
	}
	if len(msg) > 4000 {
		msg = msg[:4000]
	}
	if rt := w.plugin.currentRuntime(); rt != nil {
		rt.Log(w.level, w.eventName, msg, nil)
	}
}
