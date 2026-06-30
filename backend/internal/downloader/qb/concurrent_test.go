package qb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestQBClientConcurrentSafe 回归测试：模拟 broadcaster（每 2s ListTorrents→Sync 写 state map）
// 与 API 请求（GlobalStats/ListTags/ListCategories/tagsOf 读 state map）并发操作同一 Client。
// 修复前（state 无锁）会触发 fatal error: concurrent map read/write；修复后（stateMu RWMutex）必须通过。
func TestQBClientConcurrentSafe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			if rid := r.URL.Query().Get("rid"); rid == "" || rid == "0" {
				_, _ = w.Write([]byte(`{
					"rid":1,"full_update":true,
					"torrents":{"a":{"name":"A","size":100,"tags":"t1,t2","progress":0.5,"state":"uploadingUP","ratio":2,"dlspeed":0,"upspeed":1024,"added_on":1700000000}},
					"server_state":{"dl_info_speed":0,"up_info_speed":2048},
					"categories":{"c1":{},"c2":{}},
					"tags":["seed","movie"]
				}`))
			} else {
				// 增量：持续写新字段，触发 merge 写 map（与 broadcaster 并发）
				_, _ = w.Write([]byte(`{"rid":2,"torrents":{"a":{"upspeed":4096,"ratio":3},"b":{"name":"B","size":200}},"server_state":{"up_info_speed":8192}}`))
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.TestConnection(context.Background()); err != nil {
		t.Fatalf("test conn: %v", err)
	}

	// 8 goroutine × 50 轮，混合写（Sync）与读，覆盖所有 state 访问点。
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			ctx := context.Background()
			for j := 0; j < 50; j++ {
				switch (seed + j) % 6 {
				case 0: // broadcaster 主路径：ListTorrents（Sync 写 + range 读）
					if _, err := c.ListTorrents(ctx); err != nil {
						t.Errorf("ListTorrents: %v", err)
						return
					}
				case 1: // broadcaster 复用：CachedStats（纯读 ServerState）
					_ = c.CachedStats()
				case 2: // API：GlobalStats（Sync 写 + 读 ServerState）
					if _, err := c.GlobalStats(ctx); err != nil {
						t.Errorf("GlobalStats: %v", err)
						return
					}
				case 3: // API：ListTags（Sync 写 + 读 Tags）
					if _, err := c.ListTags(ctx); err != nil {
						t.Errorf("ListTags: %v", err)
						return
					}
				case 4: // API：ListCategories（Sync 写 + 读 Categories）
					if _, err := c.ListCategories(ctx); err != nil {
						t.Errorf("ListCategories: %v", err)
						return
					}
				case 5: // API：tagsOf（Sync 写 + 读 Torrents[hash]）
					if _, err := c.tagsOf(ctx, "a"); err != nil {
						t.Errorf("tagsOf: %v", err)
						return
					}
				}
			}
		}(i)
	}
	wg.Wait()
	// 到此处未 fatal/panic 即通过：state 的并发读写已被 stateMu 正确串行化。
}

// TestQBClientVersionConcurrentSafe 回归测试：fetchVersion（写 version/verMaj/verMin）
// 与 ge5（读）并发。修复前 version 字段无锁 → -race 报 data race；修复后（verMu）必须通过。
// 本地 Windows 无 gcc 跑不了 -race，此测试靠 CI(ubuntu) 的 -race 抓回归。
func TestQBClientVersionConcurrentSafe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.0.0"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.TestConnection(context.Background()); err != nil {
		t.Fatalf("test conn: %v", err)
	}

	// 10 组并发：fetchVersion 写 version ↔ ge5 读 version。
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = c.fetchVersion(context.Background())
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = c.ge5()
			}
		}()
	}
	wg.Wait()
	// 到此处未 panic 即通过（-race 下还能验证无 data race）。
}
