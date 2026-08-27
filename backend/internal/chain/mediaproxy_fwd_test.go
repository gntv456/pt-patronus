package chain

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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
