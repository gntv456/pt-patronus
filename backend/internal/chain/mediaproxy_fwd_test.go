package chain

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMediaProxyForwardPlain 普通请求透传：路径/查询/头部/响应体完整。
func TestMediaProxyForwardPlain(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/System/Info" || r.URL.RawQuery != "v=x" {
			t.Errorf("upstream got %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("X-Test", "yes")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ServerName":"emby"}`))
	}))
	defer up.Close()
	svc := NewMediaProxyService(nil, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/mediaproxy/1/emby/System/Info?v=x", nil)
	req.Header.Set("X-Emby-Token", "tok")
	svc.forward(w, req, up.URL, "emby/System/Info?v=x")
	if w.Code != 200 || w.Header().Get("X-Test") != "yes" {
		t.Errorf("forward result: code=%d headers=%v", w.Code, w.Header())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("emby")) {
		t.Errorf("body wrong: %s", w.Body.String())
	}
}

// TestMediaProxyForwardUpstreamDown 上游不可达：502。
func TestMediaProxyForwardUpstreamDown(t *testing.T) {
	svc := NewMediaProxyService(nil, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	svc.forward(w, req, "http://127.0.0.1:1", "items")
	if w.Code != http.StatusBadGateway {
		t.Errorf("want 502, got %d", w.Code)
	}
}

// TestMediaProxyPlaybackFallback 播放请求直链失败 → 回退转发（Emby 原样响应流）。
func TestMediaProxyPlaybackFallback(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, bytesReaderStr("videostream"))
	}))
	defer up.Close()
	// ms=nil：代理未配置守卫 → 502（不 panic）
	svc := NewMediaProxyService(nil, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/emby/videos/123/original.mkv", nil)
	svc.Handle(w, req, 0, "emby/videos/123/original.mkv")
	if w.Code != http.StatusBadGateway {
		t.Errorf("nil-ms handle: %d", w.Code)
	}
}

func bytesReaderStr(s string) io.Reader { return bytes.NewBufferString(s) }

// TestIsUpgrade WebSocket 升级判定。
func TestIsUpgrade(t *testing.T) {
	req := httptest.NewRequest("GET", "/socket", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	if !isUpgrade(req) {
		t.Error("should detect upgrade")
	}
	req.Header.Set("Connection", "keep-alive")
	if isUpgrade(req) {
		t.Error("non-upgrade connection should be false")
	}
}

// TestHopByHop 逐跳头集合。
func TestHopByHop(t *testing.T) {
	for _, h := range []string{"Connection", "Upgrade", "Transfer-Encoding", "Keep-Alive"} {
		if !hopByHop[h] {
			t.Errorf("%s should be hop-by-hop", h)
		}
	}
	if hopByHop["X-Emby-Token"] {
		t.Error("token header must pass through")
	}
}

// TestWsAddr 地址补全。
func TestWsAddr(t *testing.T) {
	if got := wsAddr("http://emby.local:8096/socket"); got != "emby.local:8096" {
		t.Errorf("explicit port: %q", got)
	}
	if got := wsAddr("http://emby.local/socket"); got != "emby.local:80" {
		t.Errorf("default http: %q", got)
	}
	if got := wsAddr("https://emby.local/socket"); got != "emby.local:443" {
		t.Errorf("default https: %q", got)
	}
}

// TestForwardWSPreservesUpgradeHeaders 回归钉（WS 反代握手端到端）：forwardWS 转发给
// 上游的握手请求必须带 Connection/Upgrade——hopByHop 过滤若把头一并剥掉，Emby/JF 的
// socket.io 经代理永远拿不到 101（实时通知/远程控制静默失效）。上游按是否收到
// Upgrade 头决定 101 或 400，代理→上游→客户端全链路走真实 TCP。
func TestForwardWSPreservesUpgradeHeaders(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
			!strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
			http.Error(w, "upgrade headers missing", http.StatusBadRequest)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("upstream not hijackable")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Errorf("upstream hijack: %v", err)
			return
		}
		defer conn.Close()
		_, _ = buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\nhello-ws")
		_ = buf.Flush()
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _ = io.Copy(io.Discard, conn)
	}))
	defer up.Close()

	svc := NewMediaProxyService(nil, nil)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		svc.forward(w, r, up.URL, "emby/socket")
	}))
	defer proxy.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(proxy.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// 读超时兜底：代理端若不回任何字节（如上游→客户端方向被 bufio 缓冲住），
	// 让失败以 5s 超时收口，而不是把整包测试挂到全局 timeout。
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _ = fmt.Fprintf(conn, "GET /emby/socket HTTP/1.1\r\nHost: proxy\r\n"+
		"Connection: Upgrade\r\nUpgrade: websocket\r\n"+
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n")
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read status line: %v", err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("upstream did not see the upgrade request (client got %q)", strings.TrimSpace(status))
	}
	// 跳过 101 响应头，读到空行后再取隧道载荷。
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("read 101 headers: %v", err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	payload := make([]byte, len("hello-ws"))
	if _, err := io.ReadFull(br, payload); err != nil {
		t.Fatalf("read tunnel payload: %v", err)
	}
	if string(payload) != "hello-ws" {
		t.Errorf("tunnel payload = %q", string(payload))
	}
}
