package notification

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestDecodeEnvelopeRefusesFailOpen 渠道的 200 里有两类「判不出送达」必须算失败：
// 空响应体、以及不是 JSON 的 200（反向代理/网关/运营商塞回的 HTML 拦截页）。
// 修复前 10 个渠道写的是 `json.Unmarshal(raw, &be) == nil && 码不对` —— 解析失败短路成
// 「判成功」，于是面板显示已送达、失败重试也不触发（渠道存在的全部意义是出事时能说话）。
func TestDecodeEnvelopeRefusesFailOpen(t *testing.T) {
	var v struct {
		Code int `json:"code"`
	}
	if err := decodeEnvelope("demo", []byte("  "), &v); err == nil || !strings.Contains(err.Error(), "空响应体") {
		t.Errorf("空体必须失败并说明原因，got %v", err)
	}
	html := []byte("<html><body>Request blocked by WAF</body></html>")
	err := decodeEnvelope("demo", html, &v)
	if err == nil {
		t.Fatalf("非 JSON 的 200 不能算送达（这是缺陷本体），got nil")
	}
	if !strings.Contains(err.Error(), "不是合法 JSON") || !strings.Contains(err.Error(), "Request blocked by WAF") {
		t.Errorf("错误要点名「不是 JSON」并带上原文线索，got %v", err)
	}
	if err := decodeEnvelope("demo", []byte(`{"code":0}`), &v); err != nil {
		t.Errorf("合法 JSON 要放行交给业务码判定，got %v", err)
	}
}

// TestServerChanRejectsIntercepted200 打真渠道实现（不是只测助手函数）：
// 上游回 200 + HTML 拦截页时 Send 必须报错，且错误里带着渠道名与原文。
func TestServerChanRejectsIntercepted200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "<html>gateway blocked</html>")
	}))
	defer srv.Close()

	err := NewServerChan(srv.URL, "SCTKEY").Send(context.Background(), Message{Title: "T", Body: "B"})
	if err == nil {
		t.Fatalf("被网关拦下的 200 必须判失败（从前判成功 ⇒ 面板显示已送达）")
	}
	for _, want := range []string{"serverchan", "不是合法 JSON", "gateway blocked"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误里缺 %q，got %q", want, err)
		}
	}
}

// TestServerChanAcceptsRealSuccess 正控：合法成功信封仍要判成功（防我改成「一律失败」）。
func TestServerChanAcceptsRealSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"code":0,"message":"ok"}`)
	}))
	defer srv.Close()
	if err := NewServerChan(srv.URL, "SCTKEY").Send(context.Background(), Message{Title: "T", Body: "B"}); err != nil {
		t.Fatalf("code=0 必须算送达，got %v", err)
	}
}

// TestNoChannelReimplementsEnvelopeParse 结构守卫：这条判据只许留在 decodeEnvelope 一份。
// 从前它是十个渠道各抄一遍（且十个都有同一个 fail-open 洞）——「同一判定抄第二遍必漂移」。
func TestNoChannelReimplementsEnvelopeParse(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "envelope.go" {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if strings.Contains(src, "json.Unmarshal(raw, &be) == nil") {
			t.Errorf("%s 仍在自己解析业务码（绕过共用判据=老洞复活）", name)
		}
		if strings.Contains(src, "decodeEnvelope(") {
			seen++
		}
	}
	if seen < 10 {
		t.Errorf("至少 10 个渠道该走 decodeEnvelope，实际 %d 个（判据没接满=覆盖假象）", seen)
	}
}
