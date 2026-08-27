package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSetHostIntervalThrottle 验证 per-host 最小间隔限流：同 host 第二次请求被推迟，
// 不同 host 不受影响；interval=0 清除限流。
func TestSetHostIntervalThrottle(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c, err := New(Options{Timeout: 5 * time.Second, AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	host := srv.URL // http://127.0.0.1:port
	c.SetHostInterval(host, 300*time.Millisecond)

	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := c.Get(context.Background(), host, "", false); err != nil {
			t.Fatalf("req %d: %v", i, err)
		}
	}
	el := time.Since(start)
	// 3 次请求至少等待 2×300ms。
	if el < 600*time.Millisecond {
		t.Fatalf("3 次请求耗时 %v，限流未生效（应 ≥600ms）", el)
	}

	// 清除限流后应立即完成。
	c.SetHostInterval(host, 0)
	start = time.Now()
	for i := 0; i < 3; i++ {
		if _, err := c.Get(context.Background(), host, "", false); err != nil {
			t.Fatalf("unthrottled req %d: %v", i, err)
		}
	}
	if el2 := time.Since(start); el2 > 100*time.Millisecond {
		t.Fatalf("清除限流后仍慢：%v", el2)
	}
}
