package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// TestKeepAliveCapturesRotatedCookie：服务端 Set-Cookie 轮换后，
// RefreshedCookie 应反映最新值，LoggedIn=true。
func TestKeepAliveCapturesRotatedCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 模拟服务端轮换 session cookie
		w.Header().Set("Set-Cookie", "session=rotated-token; Path=/")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>usercp page</body></html>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	n := NewNexusPHP(hc)
	def := &site.SiteMetadata{NoLogin: site.NoLoginDef{URLPattern: []string{"login.php"}}}

	res, err := n.KeepAlive(context.Background(), def, srv.URL, "session=old-token")
	if err != nil {
		t.Fatal(err)
	}
	if !res.LoggedIn {
		t.Errorf("expected LoggedIn=true")
	}
	if res.RefreshedCookie != "session=rotated-token" {
		t.Errorf("RefreshedCookie = %q, want session=rotated-token", res.RefreshedCookie)
	}
}

// TestKeepAliveDetectsLoggedOut：重定向到登录页 → LoggedIn=false。
func TestKeepAliveDetectsLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login.php", http.StatusFound)
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true, FollowRedirect: false})
	n := NewNexusPHP(hc)
	def := &site.SiteMetadata{NoLogin: site.NoLoginDef{URLPattern: []string{"login.php"}}}

	res, err := n.KeepAlive(context.Background(), def, srv.URL, "session=x")
	if err != nil {
		t.Fatal(err)
	}
	if res.LoggedIn {
		t.Errorf("expected LoggedIn=false on redirect to login")
	}
}
