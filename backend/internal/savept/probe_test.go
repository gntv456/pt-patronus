package savept

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// probeExtra 行为：403（CF 盾）算活、404 算死、连接拒绝算死。
// httptest 是 127.0.0.1 环回，需 AllowPrivate 放行（生产 SSRF 防护不变）。
func TestProbeExtra(t *testing.T) {
	alive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer alive.Close()
	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer notFound.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	s := NewService(hc, nil, nil)

	cards := []SaveptSiteCard{
		{Name: "活站", URL: alive.URL, Domain: "alive.test"},
		{Name: "404站", URL: notFound.URL, Domain: "notfound.test"},
		{Name: "死站", URL: "http://127.0.0.1:1/", Domain: "dead.test"},
	}
	probeMu.Lock()
	probeOkMap = map[string]bool{}
	probeAttempts = 0
	probeMu.Unlock()

	done := make(chan struct{})
	go func() { s.probeExtra(context.Background(), cards); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("probeExtra timeout")
	}

	if cards[0].Status != HealthHealthy {
		t.Errorf("alive(403 CF 盾): got %q want healthy", cards[0].Status)
	}
	if cards[1].Status != HealthClosed {
		t.Errorf("404: got %q want closed", cards[1].Status)
	}
	if cards[2].Status != HealthClosed {
		t.Errorf("dead: got %q want closed", cards[2].Status)
	}
}
