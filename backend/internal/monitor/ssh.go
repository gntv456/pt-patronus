package monitor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"golang.org/x/crypto/ssh"
)

// sshExecutor 抽象「在一个 SSH 会话里执行命令」，便于测试注入 fake（解析逻辑单测无需真实连接）。
type sshExecutor interface {
	Run(ctx context.Context, cmd string) (string, error)
	Close() error
}

// HostKeyStore 抽象 SSH host key 的持久化（TOFU），由 oper 层注入实现，避免 monitor→oper 反向依赖。
type HostKeyStore interface {
	// Lookup 返回 host:port 已记录的 host key（marshaled wire format）；ok=false 表示未记录。
	Lookup(host string, port int) (marshaled []byte, ok bool, err error)
	// Record 首次记录 host key（TOFU）。
	Record(host string, port int, keyType string, marshaled []byte, fingerprint string) error
}

// SSHCollector 经 SSH 登录远程 Linux 主机，执行一条 /proc 脚本解析指标。
type SSHCollector struct {
	// dial 建立到 target 的会话；默认基于 keyStore 的 dialSSH，测试可注入 fake。
	dial func(t *models.MonitorTarget, creds Credentials, timeout time.Duration) (sshExecutor, error)
}

// NewSSHCollector 构造。keyStore 用于 TOFU host key 校验；为 nil 时退化为不校验（仅兼容无注入场景）。
func NewSSHCollector(keyStore HostKeyStore) SSHCollector {
	return SSHCollector{dial: func(t *models.MonitorTarget, c Credentials, to time.Duration) (sshExecutor, error) {
		return dialSSH(keyStore, t, c, to)
	}}
}

func (SSHCollector) String() string { return "host(ssh)" }

// sshScript 一次会话采集 CPU(两次 stat 差值)/内存/负载/运行时长/磁盘/网络，减少往返。
const sshScript = `printf '@@STAT1@@\n'; head -n1 /proc/stat; sleep 1; printf '@@STAT2@@\n'; head -n1 /proc/stat; ` +
	`printf '@@MEM@@\n'; cat /proc/meminfo; printf '@@LOAD@@\n'; cat /proc/loadavg; ` +
	`printf '@@UPT@@\n'; cat /proc/uptime; printf '@@DISK@@\n'; df -B1 -x tmpfs -x devtmpfs -x squashfs 2>/dev/null; ` +
	`printf '@@NET@@\n'; cat /proc/net/dev; printf '@@END@@\n'`

// Probe 探活：建立会话并执行 echo。
func (c SSHCollector) Probe(ctx context.Context, t *models.MonitorTarget, creds Credentials) (bool, error) {
	ex, err := c.dial(t, creds, sshTimeout(t))
	if err != nil {
		return false, err
	}
	defer ex.Close()
	_, err = ex.Run(ctx, "echo ok")
	return err == nil, err
}

// Collect 执行采集脚本并解析。
func (c SSHCollector) Collect(ctx context.Context, t *models.MonitorTarget, creds Credentials) (Metrics, error) {
	ex, err := c.dial(t, creds, sshTimeout(t))
	if err != nil {
		return Metrics{}, err
	}
	defer ex.Close()
	out, err := ex.Run(ctx, sshScript)
	if err != nil {
		return Metrics{Online: false}, err
	}
	return parseRemoteProc(out), nil
}

// Power executes a fixed, whitelisted power action over SSH.
func (c SSHCollector) Power(ctx context.Context, t *models.MonitorTarget, creds Credentials, action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	if action != PowerActionReboot {
		return fmt.Errorf("%w: %s", ErrUnsupportedPowerAction, action)
	}
	ex, err := c.dial(t, creds, sshTimeout(t))
	if err != nil {
		return err
	}
	defer ex.Close()
	_, err = ex.Run(ctx, "sudo -n reboot")
	return err
}

func sshTimeout(t *models.MonitorTarget) time.Duration {
	sec := t.TimeoutSec
	if sec < 5 {
		sec = 5 // 脚本含 sleep 1，至少给足
	}
	return time.Duration(sec) * time.Second
}

// dialSSH 建立 SSH 会话（密码/私钥认证；creds.PrivateKey 非空时优先 publickey）。
func dialSSH(store HostKeyStore, t *models.MonitorTarget, creds Credentials, timeout time.Duration) (sshExecutor, error) {
	if creds.Username == "" {
		return nil, fmt.Errorf("ssh username is required")
	}
	port := t.Port
	if port == 0 {
		port = 22
	}
	auth, err := sshAuthMethods(creds.Password, creds.PrivateKey)
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            creds.Username,
		Auth:            auth,
		Timeout:         timeout,
		HostKeyCallback: makeHostKeyCallback(store, t.Host, port),
	}
	client, err := ssh.Dial("tcp", net.JoinHostPort(t.Host, strconv.Itoa(port)), cfg)
	if err != nil {
		return nil, err
	}
	return &sshExec{client: client}, nil
}

// sshAuthMethods 构造认证方法集。
//   - privateKey 非空：解析 PEM 并前置 ssh.PublicKeys（publickey）。解析失败（格式错/有口令）
//     返回 error，dialSSH 失败 → 设备离线 + last_error 含原因。私钥前置即最安全方法优先。
//   - password 非空：追加 password + keyboard-interactive（兼容 PAM/KbdInteractive-only 服务器）。
//
// 库按服务器返回的可用方法自动择一尝试。典型场景：root 服务器 PermitRootLogin
// prohibit-password 只返回 [publickey]，publickey 命中；普通服务器 [publickey,password]，publickey 先命中。
func sshAuthMethods(password, privateKey string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if privateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil {
			var passErr *ssh.PassphraseMissingError
			if errors.As(err, &passErr) {
				return nil, errors.New("ssh: 私钥有口令保护，当前不支持；请导出无口令私钥（如 ssh-keygen -p 解除）或改用密码认证")
			}
			return nil, fmt.Errorf("ssh: 解析私钥失败（%v）；请粘贴合法 PEM 私钥", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if password != "" {
		methods = append(methods, ssh.Password(password))
		methods = append(methods, ssh.KeyboardInteractive(func(_ string, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range questions {
				answers[i] = password
			}
			return answers, nil
		}))
	}
	return methods, nil
}

// makeHostKeyCallback 构造 TOFU host key 校验回调：
//   - store 为 nil → 不校验（兼容无注入场景）。
//   - 未记录 → 记录并放行（首次连接 TOFU）。
//   - 已记录且匹配 → 放行。
//   - 已记录但不符 → 返回错误拒绝（dialSSH 失败 → 设备显离线 + LastError 含 mismatch）。
func makeHostKeyCallback(store HostKeyStore, host string, port int) ssh.HostKeyCallback {
	if store == nil {
		return ssh.InsecureIgnoreHostKey()
	}
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		marshaled := key.Marshal()
		stored, ok, err := store.Lookup(host, port)
		if err != nil {
			return fmt.Errorf("lookup ssh host key: %w", err)
		}
		if !ok {
			return store.Record(host, port, key.Type(), marshaled, ssh.FingerprintSHA256(key))
		}
		if !bytes.Equal(stored, marshaled) {
			return fmt.Errorf("ssh host key mismatch for %s:%d（设备可能重装或遭中间人，核实后调用 accept-host-key 重置）", host, port)
		}
		return nil
	}
}

// sshExec 实现 sshExecutor，复用底层 *ssh.Client（多次 Run 共享连接）。
type sshExec struct{ client *ssh.Client }

func (e *sshExec) Run(ctx context.Context, cmd string) (string, error) {
	sess, err := e.client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	var b bytes.Buffer
	sess.Stdout = &b
	sess.Stderr = io.Discard
	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()
	select {
	case err := <-done:
		return b.String(), err
	case <-ctx.Done():
		_ = sess.Signal(ssh.SIGKILL)
		return "", ctx.Err()
	}
}

func (e *sshExec) Close() error { return e.client.Close() }

// hostAddr 工具：拼 host:port（供其它采集器复用）。
func hostAddr(t *models.MonitorTarget, defPort int) string {
	port := t.Port
	if port == 0 {
		port = defPort
	}
	return net.JoinHostPort(strings.TrimSpace(t.Host), strconv.Itoa(port))
}
