package plugins

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
)

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
		env["PTP_HOST_PERMISSIONS"] = strings.Join(p.manifest.Permissions, ",")
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
	safego.Go("external:311", func() {
		err := cmd.Wait()
		p.recordExit(cmd, err)
	})

	deadline := time.Now().Add(8 * time.Second)
	for {
		if err := p.health(ctx); err == nil {
			now := time.Now()
			p.lastHealthAt = &now
			return nil
		}
		if time.Now().After(deadline) {
			// 健康检查超时：清理已起的子进程与 host 回调端口，避免泄漏。
			p.cleanupStartedProcessLocked("health check timeout")
			return fmt.Errorf("external plugin %s health check timeout", p.manifest.ID)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// cleanupStartedProcessLocked 在 ensureStarted 失败路径上清理已起的子进程与 host 回调服务，
// 复位运行态（调用方已持有 p.mu）。recordExit 的后台 goroutine 因 p.cmd 被置 nil 而早退
// （p.cmd != cmd），不会重复清理或触发重启。lastExit 记录失败原因便于排障。
func (p *externalHTTPPlugin) cleanupStartedProcessLocked(lastExit string) {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	host := p.host
	p.cmd = nil
	p.baseURL = ""
	p.token = ""
	p.host = nil
	p.pid = 0
	now := time.Now()
	p.stoppedAt = &now
	p.lastExit = lastExit
	p.nextRestartAt = nil
	if host != nil {
		_ = host.stop()
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
		// 重启成功：复位 restarting，否则下次崩溃 scheduleRestart 因 restarting==true 早退，
		// 插件永久失去自愈（restartCount 不归零，保留累计计数可见性）。
		p.mu.Lock()
		p.restarting = false
		p.nextRestartAt = nil
		p.mu.Unlock()
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
