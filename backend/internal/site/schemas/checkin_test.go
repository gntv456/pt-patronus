package schemas

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/ocr"
	"github.com/hxpt/ptpatronus/internal/site"
)

// newCheckinServer 起一个 mock：首页返回 homeHTML（探测用），其余路径交 checkin 回调，
// 并把收到的签到请求 path/method/formhash 回填到 gotXxx 便于断言。
func newCheckinServer(t *testing.T, homeHTML string, checkin func(w http.ResponseWriter, r *http.Request),
) (srv *httptest.Server, gotPath, gotMethod, gotForm *string) {
	t.Helper()
	gp, gm, gf := "", "", ""
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.php" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, homeHTML)
			return
		}
		gp = r.URL.Path
		gm = r.Method
		_ = r.ParseForm()
		gf = r.Form.Get("formhash")
		checkin(w, r)
	}))
	return srv, &gp, &gm, &gf
}

func mustCheckin(t *testing.T, srv *httptest.Server, def *site.SiteMetadata) *site.CheckinResult {
	t.Helper()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	n := NewNexusPHP(hc)
	if def == nil {
		def = &site.SiteMetadata{Schema: "nexusphp"}
	}
	r, err := n.Checkin(context.Background(), def, srv.URL, "session=abc")
	if err != nil {
		t.Fatalf("Checkin error: %v", err)
	}
	return r
}

// 纯 HTML 站：首页无签到入口→探测回退 POST attendance.php→返回「签到成功」HTML。
func TestNexusPHPCheckinHTMLSuccess(t *testing.T) {
	srv, _, _, _ := newCheckinServer(t, "<html><body>home</body></html>", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>签到成功，获得 10 魔力</html>")
	})
	defer srv.Close()
	r := mustCheckin(t, srv, nil)
	if !r.Success {
		t.Fatalf("want success, got %+v", r)
	}
	if r.Bonus != 10 {
		t.Errorf("bonus want 10, got %v", r.Bonus)
	}
}

// HTML 已签到。
func TestNexusPHPCheckinHTMLAlready(t *testing.T) {
	srv, _, _, _ := newCheckinServer(t, "<html><body>home</body></html>", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>您今天已经签到过了</html>")
	})
	defer srv.Close()
	r := mustCheckin(t, srv, nil)
	if !r.Success || !strings.Contains(r.Message, "已签到") {
		t.Fatalf("want already-checked success, got %+v", r)
	}
}

// JSON 签到（msg 不含 HTML 关键字，走 JSON 分支，success:true）。
func TestNexusPHPCheckinJSONSuccess(t *testing.T) {
	srv, _, _, _ := newCheckinServer(t, "<html><body>home</body></html>", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"msg":"ok","bonus":10}`)
	})
	defer srv.Close()
	r := mustCheckin(t, srv, nil)
	if !r.Success {
		t.Fatalf("want success, got %+v", r)
	}
	if r.Bonus != 10 {
		t.Errorf("bonus want 10, got %v", r.Bonus)
	}
}

// JSON 已签（msg=「重复签到」，走 JSON 分支）。
func TestNexusPHPCheckinJSONAlready(t *testing.T) {
	srv, _, _, _ := newCheckinServer(t, "<html><body>home</body></html>", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ret":1,"msg":"重复签到"}`)
	})
	defer srv.Close()
	r := mustCheckin(t, srv, nil)
	if !r.Success || !strings.Contains(r.Message, "已签到") {
		t.Fatalf("want already-checked success, got %+v", r)
	}
}

// JSON 仅 ret=0 无 msg（按 ret 推断成功）。
func TestNexusPHPCheckinJSONRet0(t *testing.T) {
	srv, _, _, _ := newCheckinServer(t, "<html><body>home</body></html>", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ret":0}`)
	})
	defer srv.Close()
	r := mustCheckin(t, srv, nil)
	if !r.Success {
		t.Fatalf("want success on ret=0, got %+v", r)
	}
}

// 探测命中签到表单：POST signin.php 并携带探测到的 formhash。
func TestNexusPHPCheckinProbeForm(t *testing.T) {
	home := `<html><body><form action="signin.php" method="post">` +
		`<input type="hidden" name="formhash" value="abc123"/>` +
		`<input type="submit" value="签到"/></form></body></html>`
	srv, gotPath, gotMethod, gotForm := newCheckinServer(t, home, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>签到成功</html>")
	})
	defer srv.Close()
	r := mustCheckin(t, srv, nil)
	if !r.Success {
		t.Fatalf("want success, got %+v", r)
	}
	if *gotPath != "/signin.php" {
		t.Errorf("probe should POST signin.php, got path=%q", *gotPath)
	}
	if *gotMethod != "POST" {
		t.Errorf("form method should be POST, got %q", *gotMethod)
	}
	if *gotForm != "abc123" {
		t.Errorf("probe should carry formhash=abc123, got %q", *gotForm)
	}
}

// 探测命中签到链接：GET signin.php?op=ds。
func TestNexusPHPCheckinProbeLink(t *testing.T) {
	home := `<html><body><a href="signin.php?op=ds">每日签到</a></body></html>`
	srv, gotPath, gotMethod, _ := newCheckinServer(t, home, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>签到成功</html>")
	})
	defer srv.Close()
	r := mustCheckin(t, srv, nil)
	if !r.Success {
		t.Fatalf("want success, got %+v", r)
	}
	if *gotPath != "/signin.php" {
		t.Errorf("probe should hit signin.php, got path=%q", *gotPath)
	}
	if *gotMethod != "GET" {
		t.Errorf("link should be GET, got %q", *gotMethod)
	}
}

// 自定义 Headers 透传（绕 WAF/CSRF 出口）。
func TestNexusPHPCheckinCustomHeaders(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.php" {
			_, _ = io.WriteString(w, "<html><body>home</body></html>")
			return
		}
		gotHeader = r.Header.Get("X-Foo")
		_, _ = io.WriteString(w, "<html>unknown</html>")
	}))
	defer srv.Close()
	def := &site.SiteMetadata{Schema: "nexusphp", Checkin: site.CheckinDef{Headers: map[string]string{"X-Foo": "bar"}}}
	mustCheckin(t, srv, def)
	if gotHeader != "bar" {
		t.Errorf("custom header X-Foo=bar not sent, got %q", gotHeader)
	}
}

// 响应无可识别内容→兜底「未识别」。
func TestNexusPHPCheckinUnrecognized(t *testing.T) {
	srv, _, _, _ := newCheckinServer(t, "<html><body>home</body></html>", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>some opaque response</html>")
	})
	defer srv.Close()
	r := mustCheckin(t, srv, nil)
	if r.Success {
		t.Fatalf("want failure on unrecognized response, got %+v", r)
	}
	if !strings.Contains(r.Message, "未识别") {
		t.Errorf("want 未识别 fallback message, got %q", r.Message)
	}
}

// ── 签到图形验证码 + OCR ────────────────────────────────────

// OCR mock 服务：POST {image} → {results:[[text]]}（paddle 模式）。
func newOCRServer(t *testing.T, text string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[[{"text":"`+text+`"}]]}`)
	}))
}

// 签到表单带验证码 + OCR 已配置且识别成功：验证码答案并进表单提交，签到成功。
func TestNexusPHPCheckinCaptchaOCR(t *testing.T) {
	var gotCaptcha string
	home := `<html><body><form action="signin.php" method="post">` +
		`<input type="hidden" name="formhash" value="abc123"/>` +
		`<img src="image.php?action=show&code=ckpt"/><input type="text" name="imagestring"/>` +
		`<input type="submit" value="立即签到"/></form></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/index.php":
			_, _ = io.WriteString(w, home)
		case "/image.php":
			// 假验证码图（≥64 字节即可，OCR 服务是独立 mock）
			_, _ = w.Write(bytes.Repeat([]byte("PNGDATA"), 16))
		default:
			_ = r.ParseForm()
			gotCaptcha = r.Form.Get("imagestring")
			_, _ = io.WriteString(w, "<html>签到成功</html>")
		}
	}))
	defer srv.Close()

	ocrSrv := newOCRServer(t, "GR76DH")
	defer ocrSrv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	n := NewNexusPHP(hc)
	n.SetOCRConfig(func() ocr.Config {
		return ocr.Config{Endpoint: ocrSrv.URL, Mode: "paddle"}
	})
	r, err := n.Checkin(context.Background(), &site.SiteMetadata{Schema: "nexusphp"}, srv.URL, "session=abc")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Success {
		t.Fatalf("want success, got %+v", r)
	}
	if gotCaptcha != "GR76DH" {
		t.Errorf("captcha answer = %q, want GR76DH", gotCaptcha)
	}
}

// 签到表单带验证码但未配置 OCR：不盲提交，明确报错提示配置 OCR 或人工签到。
func TestNexusPHPCheckinCaptchaNoOCR(t *testing.T) {
	home := `<html><body><form action="signin.php" method="post">` +
		`<img src="image.php?code=ckpt"/><input type="text" name="imagestring"/>` +
		`<input type="submit" value="立即签到"/></form></body></html>`
	submitted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" || r.URL.Path == "/index.php":
			_, _ = io.WriteString(w, home)
		case r.URL.Path == "/image.php":
			_, _ = w.Write(bytes.Repeat([]byte("PNGDATA"), 16))
		case r.URL.Path == "/signin.php":
			submitted = true
			_, _ = io.WriteString(w, "<html>验证码错误</html>")
		default:
			_, _ = io.WriteString(w, "<html>验证码错误</html>")
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	n := NewNexusPHP(hc)
	r, err := n.Checkin(context.Background(), &site.SiteMetadata{Schema: "nexusphp"}, srv.URL, "session=abc")
	if err != nil {
		t.Fatal(err)
	}
	if submitted {
		t.Error("should not submit when captcha unresolved")
	}
	if r.Success || !strings.Contains(r.Message, "OCR") {
		t.Errorf("want explicit OCR hint, got %+v", r)
	}
}

// 未登录响应进入 CheckinResult.Message（数据字段，界面原样显示）：
// 必须是可操作中文，不能把哨兵原文 "not logged in" 贴给用户。
func TestNexusPHPCheckinLoggedOutIsChinese(t *testing.T) {
	srv, _, _, _ := newCheckinServer(t, "<html><body>home</body></html>", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>请先登录</title></head><body></body></html>`))
	})
	r := mustCheckin(t, srv, nil)
	if r.Success {
		t.Fatalf("未登录应 Success=false, got %+v", r)
	}
	if !strings.Contains(r.Message, "登录态已失效") || strings.Contains(r.Message, "not logged in") {
		t.Fatalf("Message 要中文可操作, got %q", r.Message)
	}
}
