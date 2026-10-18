package monitor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"golang.org/x/crypto/ssh"
)

// sshExecutor 抽象「在一个 SSH 会话里执行命令」，便于测试注入 fake（解析逻辑单测无需真实连接）。
type sshExecutor interface {
	Run(ctx context.Context, cmd string) (string, error)
	Close() error
}

type sshOutputExecutor interface {
	RunOutput(ctx context.Context, cmd string) (stdout string, stderr string, err error)
}

type sshTerminalExecutor interface {
	Shell(ctx context.Context, input io.Reader, output io.Writer, cols, rows int) error
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

// sshScript 一次会话采集 CPU/内存/负载/运行时长/磁盘/网络/主机信息/温度，减少往返。
// HOST/TEMP 为扩展段：旧服务端解析会忽略未知标记，向前兼容。
const sshScript = `printf '@@STAT1@@\n'; head -n1 /proc/stat; sleep 1; printf '@@STAT2@@\n'; cat /proc/stat; ` +
	`printf '@@MEM@@\n'; cat /proc/meminfo; printf '@@LOAD@@\n'; cat /proc/loadavg; ` +
	`printf '@@UPT@@\n'; cat /proc/uptime; printf '@@DISK@@\n'; df -B1 -x tmpfs -x devtmpfs -x squashfs -x overlay -x overlay2 -x aufs 2>/dev/null; ` +
	`printf '@@NET@@\n'; cat /proc/net/dev; ` +
	`printf '@@HOST@@\n'; printf 'hostname='; cat /proc/sys/kernel/hostname 2>/dev/null; printf '\nkernel='; uname -r 2>/dev/null; printf '\nos='; ( . /etc/os-release 2>/dev/null; echo "${PRETTY_NAME:-$(uname -s)}"); printf '\ncores='; (grep -c '^processor' /proc/cpuinfo 2>/dev/null || nproc 2>/dev/null || echo 0); ` +
	`printf '@@TEMP@@\n'; (for d in /sys/class/hwmon/hwmon*; do [ -d "$d" ] || continue; n=$(cat "$d/name" 2>/dev/null || echo hwmon); for f in "$d"/temp*_input; do [ -r "$f" ] || continue; lbl=$(cat "${f%_input}_label" 2>/dev/null || basename "$f" | sed 's/_input$//'); v=$(cat "$f" 2>/dev/null) || continue; echo "$n $lbl $v"; done; done; for z in /sys/class/thermal/thermal_zone*; do [ -d "$z" ] || continue; t=$(cat "$z/type" 2>/dev/null || echo zone); v=$(cat "$z/temp" 2>/dev/null) || continue; echo "thermal $t $v"; done) 2>/dev/null; ` +
	`printf '@@END@@\n'`

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
	cmd, err := sshPowerCommand(action)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnsupportedPowerAction, action)
	}
	ex, err := c.dial(t, creds, sshTimeout(t))
	if err != nil {
		return err
	}
	defer ex.Close()
	_, err = ex.Run(ctx, cmd)
	if err != nil && powerDispatchConfirmed(err) {
		// 设备真的开始重启/关机时，SSH 会话会在交出退出状态之前断掉——这正是**动作已生效**的
		// 信号，不是失败。命令跑完但返回非零（*ssh.ExitError，例如 sudo 要求密码）仍如实报错。
		slog.Info("ssh power action dropped session (expected)", "target", t.Name, "action", action, "err", err)
		return nil
	}
	return err
}

func sshPowerCommand(action string) (string, error) {
	switch action {
	case PowerActionReboot:
		// sudo -n (non-interactive): fails immediately if NOPASSWD not configured.
		// Fallback to plain sudo so password-prompt-capable sudoers still work via PTY;
		// the clear error guides the user to configure NOPASSWD for unattended ops.
		return "sudo -n reboot 2>/dev/null || sudo reboot", nil
	case PowerActionShutdown:
		return "sudo -n shutdown -h now 2>/dev/null || sudo shutdown -h now", nil
	default:
		return "", ErrUnsupportedPowerAction
	}
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
		return nil, enrichSSHDialError(err, t.Host, port)
	}
	return &sshExec{client: client}, nil
}

// enrichSSHDialError 给 SSH 连接类错误补「含义 + 具体问题清单 + 排查命令」三层提示，
// 让用户不看文档就能定位。各错误形态的语义：
//   - handshake failed: EOF —— TCP 连接被接受，但对端在返回 SSH 版本标识（SSH-2.0-…）前
//     就关闭了。端口后面不是正常工作的 SSH 服务。具体问题：
//     ① 端口填成了 Web 面板/宝塔等其它服务端口（最常见）；② sshd 挂了或换了端口；
//     ③ fail2ban / sshd MaxStartUps 丢弃未认证连接（直接关闭不报错，反复重试会触发）；
//     ④ 端口转发/反向代理的后端已死。
//   - i/o timeout —— TCP 都连不上：地址/端口不可达或防火墙丢包。
//   - connection refused —— 端口上无进程监听：sshd 没起或端口填错。
//   - unable to authenticate / no supported methods remain —— 网络与 SSH 服务都正常，
//     认证被拒：密码错、用户名错、root 被 PermitRootLogin prohibit-password 禁密码登录。
func enrichSSHDialError(err error, host string, port int) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "handshake failed: EOF"):
		return fmt.Errorf(`%w
├─ 含义：%d 端口能连通（TCP 已建立），但对方没返回 SSH 标识就断开——端口后面不是正常工作的 SSH 服务
├─ 具体问题（按概率排序）：
│   1. 端口填错：填成了 Web 面板 / 宝塔等其它服务端口（SSH 默认 22，改过端口的站要填实际端口）
│   2. sshd 挂了或正在重启
│   3. fail2ban / sshd MaxStartUps 拦截：反复点添加/探活会触发，封禁时直接关闭不报错
│   4. 端口转发/反向代理的后端已死（前端端口活着，转到的 SSH 后端不在）
└─ 排查：服务器上执行 ss -tlnp | grep sshd 确认监听端口；fail2ban-client status sshd 看封禁列表`, err, port)
	case strings.Contains(msg, "i/o timeout"):
		return fmt.Errorf(`%w
├─ 含义：连 %s:%d 超时——TCP 都建立不了，地址/端口不可达或防火墙丢包
├─ 具体问题：IP/端口填错、目标机器关机/断网、中间有防火墙丢包（不拒绝也不通）
└─ 排查：从 PTPatronus 所在机器 ping %s；能 ping 通再试 nc -zv %s %d`, err, host, port, host, host, port)
	case strings.Contains(msg, "connection refused"):
		return fmt.Errorf(`%w
├─ 含义：%s:%d 上没有进程在监听（对方明确拒绝）
├─ 具体问题：sshd 没启动、端口填错（对方 SSH 在别的端口）
└─ 排查：服务器上执行 systemctl status sshd 和 ss -tlnp | grep sshd`, err, host, port)
	case strings.Contains(msg, "unable to authenticate"), strings.Contains(msg, "no supported methods remain"):
		return fmt.Errorf(`%w
├─ 含义：网络和 SSH 服务都正常，但认证被拒
├─ 具体问题：密码错误、用户名错误、或服务器禁止该用户密码登录（root 常见：PermitRootLogin prohibit-password 只允许密钥）
└─ 排查：核对账号密码；root 用户可改用私钥，或在服务器放开 PermitRootLogin yes 后重启 sshd`, err)
	}
	return err
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
	out, _, err := e.RunOutput(ctx, cmd)
	return out, err
}

func (e *sshExec) RunOutput(ctx context.Context, cmd string) (string, string, error) {
	sess, err := e.client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer sess.Close()
	// 四轮 P1 修复：输出夹上限——裸 bytes.Buffer 无界，`cat bigfile`/`yes` 类命令
	// 在超时窗口内可产生数 GB stdout 先撑内存再灌库。超限截断（命令输出语义保留前缀）。
	const maxSSHOutput = 10 << 20 // 10MB
	stdout := &cappedBuffer{max: maxSSHOutput}
	stderr := &cappedBuffer{max: 1 << 20}
	sess.Stdout = stdout
	sess.Stderr = stderr
	done := make(chan error, 1)
	safego.Go("ssh:228", func() { done <- sess.Run(cmd) })
	select {
	case err := <-done:
		return stdout.String(), stderr.String(), err
	case <-ctx.Done():
		// ctx 取消时必须显式结束 session，否则底层 sess.Run goroutine 持续阻塞（远端命令
		// 不因本地 ctx 取消而停），goroutine 与 session 句柄长期残留——每次 RunCommandNow
		// 超时都会泄漏一个。Signal(SIGKILL) 触发 Run 返回错误，goroutine 才能退出。
		_ = sess.Signal(ssh.SIGKILL)
		<-done // 等 sess.Run 收尾，确保 defer sess.Close 不与 Run 竞争
		return stdout.String(), stderr.String(), ctx.Err()
	}
}

func (e *sshExec) Shell(ctx context.Context, input io.Reader, output io.Writer, cols, rows int) error {
	sess, err := e.client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 32
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		return err
	}
	sess.Stdin = input
	sess.Stdout = output
	sess.Stderr = output
	if err := sess.Shell(); err != nil {
		return err
	}
	done := make(chan error, 1)
	safego.Go("ssh:269", func() { done <- sess.Wait() })
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// ctx 取消时关 session 让底层 sess.Wait 返回（PTY shell 没有可中断远端命令的
		// Signal 路径，Close 会向远端发 EOF/关闭 channel，Wait 才能收尾）。
		// 不等 done 会导致 defer sess.Close 与 Wait 竞争 + goroutine 泄漏。
		_ = sess.Close()
		<-done
		return ctx.Err()
	}

}
func (e *sshExec) Close() error { return e.client.Close() }

// cappedBuffer 有上限的输出缓冲（四轮 P1：SSH 命令输出防 `yes`/`cat bigfile` 撑爆内存）。
type cappedBuffer struct {
	buf     bytes.Buffer
	max     int
	trunced bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len() >= b.max {
		b.trunced = true
		return len(p), nil // 丢弃并假装写成功（session 不会因写错误中断）
	}
	room := b.max - b.buf.Len()
	if len(p) > room {
		b.buf.Write(p[:room])
		b.trunced = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *cappedBuffer) String() string {
	out := b.buf.String()
	if b.trunced {
		out = out + "\n...[输出超过上限已截断]"
	}
	return out
}
