package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWXSendHeadersAndPayload：断言鉴权头 + 协议层报文(item_list.text_item)。
func TestWXSendHeadersAndPayload(t *testing.T) {
	var gotHeaders http.Header
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{}})
	}))
	defer srv.Close()

	w := NewWeChatClawBot(srv.URL, "bot-token", "acct-1", "target-1")
	if err := w.Send(context.Background(), Message{Title: "标题", Body: "正文", Data: map[string]any{"url": "https://x"}}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotHeaders.Get("AuthorizationType") != "ilink_bot_token" {
		t.Errorf("AuthorizationType = %q", gotHeaders.Get("AuthorizationType"))
	}
	if gotHeaders.Get("Authorization") != "Bearer bot-token" {
		t.Errorf("Authorization = %q", gotHeaders.Get("Authorization"))
	}
	if gotHeaders.Get("X-WECHAT-UIN") == "" {
		t.Error("X-WECHAT-UIN missing")
	}
	if !strings.Contains(gotBody, `"channel_version":"1.0.2"`) {
		t.Errorf("base_info.channel_version missing: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"text_item":{"text":`) || !strings.Contains(gotBody, `"to_user_id":"target-1"`) {
		t.Errorf("protocol payload missing: %s", gotBody)
	}
	if !strings.Contains(gotBody, "查看详情：https://x") {
		t.Errorf("link not composed: %s", gotBody)
	}
}

// TestWXSendCandidateFallback：第一个 URL（sendmessage）失败 → 回退到 ?bot_type=3 成功。
func TestWXSendCandidateFallback(t *testing.T) {
	var hitPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitPaths = append(hitPaths, r.URL.String())
		if strings.Contains(r.URL.RawQuery, "bot_type=3") {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "errmsg": "no"})
	}))
	defer srv.Close()

	w := NewWeChatClawBot(srv.URL, "tok", "acct", "tgt")
	if err := w.Send(context.Background(), Message{Title: "T"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(hitPaths) < 2 {
		t.Fatalf("expected candidate fallback across URLs, got %v", hitPaths)
	}
}

// TestWXSendErrors：缺 token / 缺 target 报错。
func TestWXSendErrors(t *testing.T) {
	if err := (&WeChatClawBot{target: "t"}).Send(context.Background(), Message{Title: "x"}); err == nil {
		t.Error("no token → expect error")
	}
	if err := (&WeChatClawBot{botToken: "t"}).Send(context.Background(), Message{Title: "x"}); err == nil {
		t.Error("no target → expect error")
	}
}

// TestWXRequestQRCode：解析 qrcode id + url。
func TestWXRequestQRCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "bot_type=3") {
			t.Errorf("qrcode request should carry bot_type=3: %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"qrcode": "qr-xyz", "qrcode_url": "https://wx.example/qr.png"},
		})
	}))
	defer srv.Close()
	qr, url, err := RequestQRCode(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("RequestQRCode: %v", err)
	}
	if qr != "qr-xyz" || url != "https://wx.example/qr.png" {
		t.Errorf("got qr=%q url=%q", qr, url)
	}
}

// TestWXPollQRStatus：未扫码 waiting；已扫码返回 token/accountID。
func TestWXPollQRStatus(t *testing.T) {
	scanned := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if scanned {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"status": "loggedin", "bot_token": "tok-abc", "account_id": "acct-9"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"status": "waiting"}})
	}))
	defer srv.Close()

	r1, err := PollQRStatus(context.Background(), srv.URL, "qr")
	if err != nil {
		t.Fatal(err)
	}
	if r1.Success || r1.Status != "waiting" {
		t.Errorf("first poll: %+v", r1)
	}
	scanned = true
	r2, err := PollQRStatus(context.Background(), srv.URL, "qr")
	if err != nil {
		t.Fatal(err)
	}
	if !r2.Success || r2.Token != "tok-abc" || r2.AccountID != "acct-9" {
		t.Errorf("second poll: %+v", r2)
	}
}

func TestWXComposeText(t *testing.T) {
	if got := wxComposeText("T", "B", "https://l"); !strings.Contains(got, "T\n\nB\n\n查看详情：https://l") {
		t.Errorf("compose: %q", got)
	}
}
