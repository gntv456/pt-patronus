package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// 编译期断言：unit3d / gazelle / luminance / generic-json(sunnypt) 均实现 site.KeepAliveCapable，
// 从而被 automation 的 cookie 保活 cron 自动纳入。
// GenericJSON 实现 KeepAlive 是 sunnypt 专用（def.ID=="sunnypt" 分支），其余 tnode 站返回 ErrNotSupported。
var (
	_ site.KeepAliveCapable = (*Unit3D)(nil)
	_ site.KeepAliveCapable = (*Gazelle)(nil)
	_ site.KeepAliveCapable = (*Luminance)(nil)
	_ site.KeepAliveCapable = (*GenericJSON)(nil)
)

// TestKeepAliveIndexLoggedIn 首页返回正常（title 不含登录标记）→ 报告仍登录。
func TestKeepAliveIndexLoggedIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>首页 - 站点</title></head><body>欢迎回来</body></html>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u := NewUnit3D(hc)
	def := &site.SiteMetadata{ID: "u3d", Schema: "unit3d",
		NoLogin: site.NoLoginDef{Content: []string{"Login", "登录"}}}

	res, err := u.KeepAlive(context.Background(), def, srv.URL, "session=1")
	if err != nil {
		t.Fatalf("KeepAlive error: %v", err)
	}
	if !res.LoggedIn {
		t.Error("expected LoggedIn=true for a normal index page")
	}
}

// TestKeepAliveIndexLoggedOut 首页 title 含 "Login"（会话过期被重定向到登录页）→ 报告掉线。
func TestKeepAliveIndexLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Login :: 站点</title></head><body>请登录</body></html>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	// 三站共用 keepAliveIndex，任选一个验证逻辑即可；这里用 Gazelle 覆盖共享路径。
	g := NewGazelle(hc)
	def := &site.SiteMetadata{ID: "gz", Schema: "gazelle",
		NoLogin: site.NoLoginDef{Content: []string{"Login", "登录"}}}

	res, err := g.KeepAlive(context.Background(), def, srv.URL, "session=expired")
	if err != nil {
		t.Fatalf("KeepAlive error: %v", err)
	}
	if res.LoggedIn {
		t.Error("expected LoggedIn=false for a login page")
	}
}
