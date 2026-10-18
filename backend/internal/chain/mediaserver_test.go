package chain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// TestParseMediaWebhook 覆盖 Emby/JF JSON、Plex JSON、Plex form-encoded、空载荷。
func TestParseMediaWebhook(t *testing.T) {
	// Emby/JF：顶层 Title + Event
	title, event := parseMediaWebhook([]byte(`{"Title":"Inception","Event":"play"}`))
	if title != "Inception" || event != "play" {
		t.Errorf("emby: title=%q event=%q", title, event)
	}

	// Plex JSON：Metadata.title + event
	title, event = parseMediaWebhook([]byte(`{"event":"media.scrobble","Metadata":{"title":"Inception"}}`))
	if title != "Inception" || event != "media.scrobble" {
		t.Errorf("plex json: title=%q event=%q", title, event)
	}

	// Plex form-encoded
	title, event = parseMediaWebhook([]byte(`event=media.play&metadata[title]=Foo+Bar`))
	if title != "Foo+Bar" || event != "media.play" {
		t.Errorf("plex form: title=%q event=%q", title, event)
	}

	// 空载荷
	title, event = parseMediaWebhook(nil)
	if title != "" || event != "" {
		t.Errorf("empty: title=%q event=%q want both empty", title, event)
	}

	// 缺 event 字段 → 归一为 "play"
	_, event = parseMediaWebhook([]byte(`{"Title":"X"}`))
	if event != "play" {
		t.Errorf("default event = %q want play", event)
	}
}

// newMediaServerTestChain 构建一条接内存 DB 的 MediaServerChain（不依赖真实 media server）。
// 额外返回 db（供测试直接破坏加密字段）与 bus（订阅 Emit 事件观测出口）。
func newMediaServerTestChain(t *testing.T) (*MediaServerChain, *oper.MediaServerOper, *gorm.DB, *event.Bus) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ms.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.MediaServer{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	op := oper.NewMediaServerOper(db, security.NewCrypto("test-seed"))
	bus := event.New()
	base := NewBase(module.New(), bus)
	ch, err := NewMediaServerChain(base, op, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ch, op, db, bus
}

// webhookEvents 订阅 MediaServerWebhook 事件，返回计数读取函数。
// Publish 为同步扇出（wg.Wait 后返回），故 ReceiveWebhook 返回即可断言。
func webhookEvents(t *testing.T, bus *event.Bus) func() int {
	t.Helper()
	var mu sync.Mutex
	n := 0
	bus.Subscribe("MediaServerWebhook", 0, func(_ context.Context, _ event.Event) {
		mu.Lock()
		n++
		mu.Unlock()
	})
	return func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

// TestWebhookTokenUnreadableRejected token 密文损坏/解密失败时必须 fail-closed 拒绝：
// 旧实现把「校验不了」与「没配 token」同待，公开端点可匿名伪造通知。
func TestWebhookTokenUnreadableRejected(t *testing.T) {
	ch, op, db, bus := newMediaServerTestChain(t)
	got := webhookEvents(t, bus)

	m, err := op.Create(oper.MediaServerInput{
		Name: "emby", Type: "emby", URL: "http://10.0.0.1:8096",
		Token: "sekret", NotifyOnPlay: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// 直接写坏密文：模拟密钥轮换 / 数据损坏后 token 不可读
	if err := db.Model(&models.MediaServer{}).Where("id = ?", m.ID).
		Update("token_enc", "!!!not-a-valid-ciphertext!!!").Error; err != nil {
		t.Fatal(err)
	}
	// 前提断言：损坏密文必须真的读不出（若此断言先红，说明破坏手段失效）
	if m2, gerr := op.Get(m.ID); gerr == nil {
		if _, terr := op.Token(m2); terr == nil {
			t.Fatal("前提不成立：损坏密文应解密失败")
		}
	}
	ch.ReceiveWebhook(context.Background(), m.ID, []byte(`{"Title":"伪造标题","Event":"play"}`), "")
	if n := got(); n != 0 {
		t.Fatalf("token 不可读时必须拒绝 webhook（fail-closed），实际 emit %d 次", n)
	}
}

// TestWebhookTokenCheckPaths 正控组：校验三分支行为——错误 token 拒、正确 token 放行、
// 未配置 token 保留旧行为放行（文档化兼容）。
func TestWebhookTokenCheckPaths(t *testing.T) {
	ch, op, _, bus := newMediaServerTestChain(t)
	got := webhookEvents(t, bus)
	ctx := context.Background()
	payload := []byte(`{"Title":"Inception","Event":"play"}`)

	m, err := op.Create(oper.MediaServerInput{
		Name: "emby", Type: "emby", URL: "http://10.0.0.1:8096",
		Token: "sekret", NotifyOnPlay: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 错误 token → 拒绝（校验在生效）
	ch.ReceiveWebhook(ctx, m.ID, payload, "wrong")
	if n := got(); n != 0 {
		t.Fatalf("错误 token 应拒绝，emit=%d", n)
	}

	// 正确 token → 放行
	ch.ReceiveWebhook(ctx, m.ID, payload, "sekret")
	if n := got(); n != 1 {
		t.Fatalf("正确 token 应放行，emit=%d want 1", n)
	}

	// 未配置 token → 保留旧行为放行（部分 Emby webhook 模板不含 token 字段）
	m2, err := op.Create(oper.MediaServerInput{
		Name: "jf", Type: "jellyfin", URL: "http://10.0.0.2:8096", NotifyOnPlay: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ch.ReceiveWebhook(ctx, m2.ID, payload, "")
	if n := got(); n != 2 {
		t.Fatalf("未配置 token 应放行，emit=%d want 2", n)
	}
}

// TestDirectLinkCleansDotDot 公开直链端点：path 含 ../ 不得逃出 DirectLinkBase 路径前缀
// （旧实现原样拼接，CDN 归一后 /d/media/../../other 逃到 /other）。
func TestDirectLinkCleansDotDot(t *testing.T) {
	ch, op, _, _ := newMediaServerTestChain(t)
	m, err := op.Create(oper.MediaServerInput{
		Name: "emby", Type: "emby", URL: "http://10.0.0.1:8096",
		DirectLinkEnabled: true, DirectLinkBase: "https://cdn.example.com/d/media",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ctx := context.Background()

	// 正控：正常相对路径改写结果保持不变
	target, _, err := ch.DirectLink(ctx, m.ID, "Movies/Inception (2010)/Inception.mkv")
	if err != nil {
		t.Fatalf("DirectLink: %v", err)
	}
	if want := "https://cdn.example.com/d/media/Movies/Inception (2010)/Inception.mkv"; target != want {
		t.Errorf("normal path rewrite = %q want %q", target, want)
	}

	// 缺陷场景：.. 必须被清理且困在 base 前缀内
	target, _, err = ch.DirectLink(ctx, m.ID, "../../other/file.mkv")
	if err != nil {
		t.Fatalf("DirectLink dotdot: %v", err)
	}
	if strings.Contains(target, "..") {
		t.Fatalf("path 未清理，target=%q", target)
	}
	if want := "https://cdn.example.com/d/media/other/file.mkv"; target != want {
		t.Errorf("dotdot rewrite = %q want %q", target, want)
	}
}

// TestMediaServerCRUDOper oper 创建 + token 加密往返 + 类型白名单。
func TestMediaServerCRUDOper(t *testing.T) {
	_, op, _, _ := newMediaServerTestChain(t)

	// 非法类型被拒
	if _, err := op.Create(oper.MediaServerInput{Name: "x", Type: "foo", URL: "http://x"}); err == nil {
		t.Fatal("非法类型应被拒")
	}

	m, err := op.Create(oper.MediaServerInput{
		Name: "emby-home", Type: "emby", URL: "http://10.0.0.1:8096",
		Token: "secret-key", RefreshOnDownload: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tok, err := op.Token(m)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "secret-key" {
		t.Errorf("token round-trip = %q", tok)
	}

	// Update token 空串=保留；显式保留 RefreshOnDownload=true（Update 覆盖全部字段）
	if _, err := op.Update(m.ID, oper.MediaServerInput{
		Name: "renamed", Type: "emby", URL: "http://10.0.0.1:8096", RefreshOnDownload: true,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	m2, _ := op.Get(m.ID)
	tok2, _ := op.Token(m2)
	if tok2 != "secret-key" || m2.Name != "renamed" {
		t.Errorf("update kept token? token=%q name=%q", tok2, m2.Name)
	}

	// ListRefreshOnDownload 只返回开启刷新的
	ms, err := op.ListRefreshOnDownload()
	if err != nil || len(ms) != 1 {
		t.Errorf("ListRefreshOnDownload = %v (want 1)", ms)
	}
}

// TestOnDownloadCompletedRefreshes 下载完成钩子触发对 mock media server 的整库扫描。
func TestOnDownloadCompletedRefreshes(t *testing.T) {
	scanned := make(map[string]bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Library/MediaFolders":
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{
				{"Name": "电影", "Id": "lib1"},
			}})
		default:
			if r.URL.Path == "/Items/lib1/Refresh" {
				scanned["lib1"] = true
			}
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	ch, op, _, _ := newMediaServerTestChain(t)
	m, err := op.Create(oper.MediaServerInput{
		Name: "emby", Type: "emby", URL: srv.URL, Token: "t", RefreshOnDownload: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = m

	ch.OnDownloadCompleted(context.Background(), "Some.Movie.2024")
	if !scanned["lib1"] {
		t.Error("expected LibraryScan on lib1 after download completion")
	}
}

// refreshEvents 订阅 MediaServerRefresh，返回最近一次事件载荷的读取函数。
func refreshEvents(t *testing.T, bus *event.Bus) func() map[string]any {
	t.Helper()
	var mu sync.Mutex
	var last map[string]any
	bus.Subscribe("MediaServerRefresh", 0, func(_ context.Context, ev event.Event) {
		mu.Lock()
		last = ev.Data
		mu.Unlock()
	})
	return func() map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return last
	}
}

// TestOnDownloadCompletedSurfacesFailure 「下载完成自动刷库」失败必须回传原因。
// 旧实现所有失败只打 Debug（默认级别看不见），事件里只有 servers 数量——
// 用户开了这功能却一台都没扫成时，链路上没有任何信号。
func TestOnDownloadCompletedSurfacesFailure(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer down.Close()

	ch, op, _, bus := newMediaServerTestChain(t)
	got := refreshEvents(t, bus)
	if _, err := op.Create(oper.MediaServerInput{
		Name: "飞牛影视", Type: "emby", URL: down.URL, Token: "t", RefreshOnDownload: true,
	}); err != nil {
		t.Fatal(err)
	}
	ch.OnDownloadCompleted(context.Background(), "Some.Movie.2024")

	data := got()
	if data == nil {
		t.Fatal("未广播 MediaServerRefresh")
	}
	if n, _ := data["scanned"].(int); n != 0 {
		t.Fatalf("前提：连不上的服务器 scanned=%d want 0", n)
	}
	fails, _ := data["failures"].([]string)
	if len(fails) == 0 || !strings.Contains(fails[0], "飞牛影视") {
		t.Errorf("失败原因必须随事件回传，got %#v（旧行为：事件里只有 servers=%v）", data["failures"], data["servers"])
	}
}

// TestOnDownloadCompletedScansHealthy 正控：正常服务器仍要扫到库，且不带失败原因。
func TestOnDownloadCompletedScansHealthy(t *testing.T) {
	var scanned int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Library/MediaFolders":
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{
				{"Name": "电影", "Id": "lib1", "CollectionType": "movies"},
			}})
		default:
			atomic.AddInt32(&scanned, 1)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer up.Close()

	ch, op, _, bus := newMediaServerTestChain(t)
	got := refreshEvents(t, bus)
	if _, err := op.Create(oper.MediaServerInput{
		Name: "Emby", Type: "emby", URL: up.URL, Token: "t", RefreshOnDownload: true,
	}); err != nil {
		t.Fatal(err)
	}
	ch.OnDownloadCompleted(context.Background(), "Some.Movie.2024")

	data := got()
	if data == nil {
		t.Fatal("未广播 MediaServerRefresh")
	}
	if n, _ := data["scanned"].(int); n != 1 {
		t.Errorf("正常服务器应扫到 1 个库，got %#v", data)
	}
	if fails, _ := data["failures"].([]string); len(fails) != 0 {
		t.Errorf("正常路径不该有 failures，got %v", fails)
	}
	if atomic.LoadInt32(&scanned) == 0 {
		t.Error("扫描请求未发出（假服务器未收到 Refresh 调用）")
	}
}
