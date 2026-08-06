package plugins

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
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

