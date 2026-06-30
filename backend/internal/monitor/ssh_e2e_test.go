package monitor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"golang.org/x/crypto/ssh"
)

// 本文件用真实内存 SSH server（net.Listen + ssh.NewServerConn）端到端验证 dialSSH 的认证路径，
// 覆盖之前只有 fake dial 而缺失的私钥/密码握手。

// genKeyPair 生成一对 ed25519 密钥，返回无口令 PKCS8 PEM 私钥与对应的 ssh 公钥。
func genKeyPair(t *testing.T) (privPEM string, pub ssh.PublicKey) {
	t.Helper()
	pubKey, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 keygen: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	privPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	sp, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		t.Fatalf("ssh.NewPublicKey: %v", err)
	}
	return privPEM, sp
}

// startTestSSHServer 启一个内存 SSH server（真实 TCP）。
// authorizedPub 非空启用 publickey 认证（仅放行该公钥）；password 非空启用 password 认证。
// exec 请求固定回 "ok\n" 并 exit-status 0。返回监听地址与 cleanup。
func startTestSSHServer(t *testing.T, authorizedPub ssh.PublicKey, password string) (addr string, cleanup func()) {
	t.Helper()
	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("host keygen: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}
	cfg := &ssh.ServerConfig{}
	if authorizedPub != nil {
		want := authorizedPub.Marshal()
		cfg.PublicKeyCallback = func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if !equalBytes(key.Marshal(), want) {
				return nil, fmt.Errorf("unknown public key")
			}
			return nil, nil
		}
	}
	if password != "" {
		cfg.PasswordCallback = func(_ ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if string(pw) != password {
				return nil, fmt.Errorf("password rejected")
			}
			return nil, nil
		}
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleSSHConn(conn, cfg)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// handleSSHConn 处理一条 SSH 连接：完成握手，对每个 session channel 的 exec 请求回 "ok\n"。
func handleSSHConn(conn net.Conn, cfg *ssh.ServerConfig) {
	defer conn.Close()
	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)
	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			newCh.Reject(ssh.UnknownChannelType, "only session supported")
			continue
		}
		ch, chReqs, err := newCh.Accept()
		if err != nil {
			continue
		}
		go func(ch ssh.Channel, reqs <-chan *ssh.Request) {
			defer ch.Close()
			for req := range reqs {
				if req.Type == "exec" {
					req.Reply(true, nil)
					ch.Write([]byte("ok\n"))
					ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0}) // exit-status=0（裸 uint32 BE）
					return
				}
				req.Reply(false, nil)
			}
		}(ch, chReqs)
	}
}

func testTarget(host string, port int) *models.MonitorTarget {
	return &models.MonitorTarget{Host: host, Port: port, TimeoutSec: 10}
}

func splitAddr(t *testing.T, addr string) (host string, port int) {
	t.Helper()
	h, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr %q: %v", addr, err)
	}
	p, _ := strconv.Atoi(portStr)
	return h, p
}

// TestDialSSH_PrivateKeyOK：合法私钥 → dialSSH 成功 + Run 返回 "ok" + TOFU 首次记录 host key。
func TestDialSSH_PrivateKeyOK(t *testing.T) {
	privPEM, pub := genKeyPair(t)
	addr, cleanup := startTestSSHServer(t, pub, "")
	defer cleanup()
	host, port := splitAddr(t, addr)

	store := &fakeHostKeyStore{keys: map[string][]byte{}}
	creds := Credentials{Username: "root", PrivateKey: privPEM}
	ex, err := dialSSH(store, testTarget(host, port), creds, 10*time.Second)
	if err != nil {
		t.Fatalf("dial with valid key: %v", err)
	}
	defer ex.Close()
	out, err := ex.Run(context.Background(), "echo ok")
	if err != nil || !strings.Contains(out, "ok") {
		t.Fatalf("run: out=%q err=%v", out, err)
	}
	if store.recorded != 1 {
		t.Errorf("host key should be recorded (TOFU), got recorded=%d", store.recorded)
	}
}

// TestDialSSH_PrivateKeyWrongRejected：客户端私钥与服务器授权公钥不匹配 → 认证失败。
func TestDialSSH_PrivateKeyWrongRejected(t *testing.T) {
	_, authorizedPub := genKeyPair(t)
	wrongPEM, _ := genKeyPair(t)
	addr, cleanup := startTestSSHServer(t, authorizedPub, "")
	defer cleanup()
	host, port := splitAddr(t, addr)

	store := &fakeHostKeyStore{keys: map[string][]byte{}}
	creds := Credentials{Username: "root", PrivateKey: wrongPEM}
	_, err := dialSSH(store, testTarget(host, port), creds, 10*time.Second)
	if err == nil {
		t.Fatal("expected dial to fail with wrong key, got nil error")
	}
	if !strings.Contains(err.Error(), "unable to authenticate") && !strings.Contains(err.Error(), "no supported methods") {
		t.Errorf("expected auth failure error, got: %v", err)
	}
}

// TestDialSSH_NoKeyNoPasswordRejected：无私钥无密码 → 无 Auth 方法 → dial 失败。
func TestDialSSH_NoKeyNoPasswordRejected(t *testing.T) {
	_, pub := genKeyPair(t)
	addr, cleanup := startTestSSHServer(t, pub, "")
	defer cleanup()
	host, port := splitAddr(t, addr)

	store := &fakeHostKeyStore{keys: map[string][]byte{}}
	creds := Credentials{Username: "root"} // 无任何凭据
	_, err := dialSSH(store, testTarget(host, port), creds, 10*time.Second)
	if err == nil {
		t.Fatal("expected dial to fail with no auth methods, got nil error")
	}
}

// TestDialSSH_PasswordOK：密码认证路径（确认未因私钥改动而回归）。
func TestDialSSH_PasswordOK(t *testing.T) {
	addr, cleanup := startTestSSHServer(t, nil, "s3cret")
	defer cleanup()
	host, port := splitAddr(t, addr)

	store := &fakeHostKeyStore{keys: map[string][]byte{}}
	creds := Credentials{Username: "root", Password: "s3cret"}
	ex, err := dialSSH(store, testTarget(host, port), creds, 10*time.Second)
	if err != nil {
		t.Fatalf("dial with password: %v", err)
	}
	defer ex.Close()
	if out, err := ex.Run(context.Background(), "echo ok"); err != nil || !strings.Contains(out, "ok") {
		t.Fatalf("run: out=%q err=%v", out, err)
	}
}

// TestSSHAuthMethods_BadPEM：私钥非 PEM → 返回解析失败错误（而非静默）。
func TestSSHAuthMethods_BadPEM(t *testing.T) {
	_, err := sshAuthMethods("", "not a pem")
	if err == nil {
		t.Fatal("expected error for bad pem, got nil")
	}
	if !strings.Contains(err.Error(), "解析私钥失败") {
		t.Errorf("expected 解析私钥失败 in error, got: %v", err)
	}
}

// TestSSHAuthMethods_Empty：密码和私钥都空 → 返回空方法集、无 error（dialSSH 会因此失败）。
func TestSSHAuthMethods_Empty(t *testing.T) {
	methods, err := sshAuthMethods("", "")
	if err != nil {
		t.Fatalf("expected no error for empty creds, got: %v", err)
	}
	if len(methods) != 0 {
		t.Errorf("expected 0 methods, got %d", len(methods))
	}
}
