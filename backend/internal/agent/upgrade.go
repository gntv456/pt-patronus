package agent

import (
	"crypto/sha256"
	"strings"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// upgrade.go — agent 自更新：收到 upgrade 命令后下载新二进制、校验、替换、退出（systemd 重启）。
// 安全：sha256 校验下载完整性；失败不覆盖原二进制（写临时文件 → rename 原子替换）。
// 限制：仅 Linux（agent 部署目标）；Dst=下载 URL，Checksum=期望 sha256。

// cmdUpgrade 处理自更新命令。
func (a *agentConn) cmdUpgrade(env Envelope) {
	if runtime.GOOS != "linux" {
		_ = a.writeResp(env.ID, false, "自更新仅支持 Linux", nil)
		return
	}
	url := strings.ReplaceAll(env.Cmd.Dst, "{arch}", runtime.GOARCH)
	if url == "" {
		_ = a.writeResp(env.ID, false, "缺少下载地址", nil)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		_ = a.writeResp(env.ID, false, "无法确定当前可执行文件路径: "+err.Error(), nil)
		return
	}
	exe, _ = filepath.EvalSymlinks(exe)
	if exe == "" {
		exe, _ = os.Executable()
	}

	// 下载到临时文件
	tmp := exe + ".new"
	if err := downloadToFile(url, tmp, env.Cmd.Checksum); err != nil {
		os.Remove(tmp)
		_ = a.writeResp(env.ID, false, "下载/校验失败: "+err.Error(), nil)
		return
	}

	// 备份旧二进制（rename 失败时回退）
	backup := exe + ".bak"
	os.Remove(backup)
	if err := os.Rename(exe, backup); err != nil {
		os.Remove(tmp)
		_ = a.writeResp(env.ID, false, "备份旧版本失败: "+err.Error(), nil)
		return
	}
	// 原子替换
	if err := os.Rename(tmp, exe); err != nil {
		// 回退
		os.Rename(backup, exe)
		_ = a.writeResp(env.ID, false, "替换失败: "+err.Error(), nil)
		return
	}
	os.Chmod(exe, 0o755)
	// 备份保留一个，下次成功升级会被 Remove 覆盖

	_ = a.writeResp(env.ID, true, "", nil)

	// 异步退出，让 systemd（Restart=always）拉起新版本
	go func() {
		time.Sleep(500 * time.Millisecond) // 给响应帧留发送时间
		os.Exit(0)
	}()
}

// downloadToFile 下载 URL 到 path 并强制 sha256 校验。
// checksum 为空一律拒绝执行（安全审计 P1-5：空校验 + 第三方网盘/镜像直链
// 等于把 agent 自更新的执行权交给下载源）。
func downloadToFile(url, path, checksum string) error {
	if checksum == "" {
		return fmt.Errorf("拒绝下载：缺少 sha256 校验值（更新清单损坏或配置错误）")
	}
	hc := &http.Client{Timeout: 5 * time.Minute}
	resp, err := hc.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()
	got := hex.EncodeToString(h.Sum(nil))
	if got != checksum {
		return fmt.Errorf("sha256 校验失败: 期望 %s，实际 %s", checksum, got)
	}
	return nil
}
