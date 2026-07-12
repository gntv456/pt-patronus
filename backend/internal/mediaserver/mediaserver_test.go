package mediaserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// newTestClient 构建一个指向 httptest server 的 httpclient（AllowPrivate 以放行 127.0.0.1）。
func newTestClient(t *testing.T) *httpclient.Client {
	t.Helper()
	c, err := httpclient.New(httpclient.Options{AllowPrivate: true, Timeout: 5e9})
	if err != nil {
		t.Fatalf("httpclient: %v", err)
	}
	return c
}

// solidImage 生成一张纯色 RGBA 图片（封面生成测试用）。
func solidImage(c color.RGBA, w, h int) image.Image {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.SetRGBA(x, y, c)
		}
	}
	return m
}

// TestEmbyClientFlow 跑通 Emby/JF：Test/Libraries/RecentItems/Notify，断言鉴权头携带 X-Emby-Token。
func TestEmbyClientFlow(t *testing.T) {
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Emby-Token")
		switch r.URL.Path {
		case "/System/Info":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ServerName": "MyEmby", "Version": "4.8"})
		case "/Library/MediaFolders":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{
				{"Name": "电影", "Id": "11", "CollectionType": "movies"},
				{"Name": "剧集", "Id": "22", "CollectionType": "tvshows"},
			}})
		case "/Notifications/Admin":
			w.WriteHeader(200)
		case "/Items":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{
				{"Id": "a1", "Name": "Movie A"},
				{"Id": "a2", "Name": "Movie B"},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cli, err := NewClient(Server{Type: "emby", URL: srv.URL, Token: "TKN"}, newTestClient(t))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	info, err := cli.Test(ctx)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if gotToken != "TKN" {
		t.Errorf("X-Emby-Token header = %q, want TKN", gotToken)
	}
	if info != "MyEmby 4.8" {
		t.Errorf("info = %q", info)
	}

	libs, err := cli.Libraries(ctx)
	if err != nil {
		t.Fatalf("Libraries: %v", err)
	}
	if len(libs) != 2 || libs[0].ID != "11" || libs[0].Name != "电影" {
		t.Fatalf("libs = %+v", libs)
	}

	if err := cli.Notify(ctx, "hi", "body"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	items, err := cli.RecentItems(ctx, "11", 2)
	if err != nil {
		t.Fatalf("RecentItems: %v", err)
	}
	if len(items) != 2 || items[0].ID != "a1" || items[0].ImageURL == "" {
		t.Fatalf("items = %+v", items)
	}
}

// TestPlexSectionID 把 Plex Key 取末段。
func TestPlexSectionID(t *testing.T) {
	if got := plexSectionID("/library/sections/2"); got != "2" {
		t.Errorf("section id = %q", got)
	}
	if got := plexSectionID("7"); got != "7" {
		t.Errorf("section id = %q", got)
	}
}

// TestPlexNotifyUnsupported Plex 无通知端点应返回明确错误。
func TestPlexNotifyUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	cli, err := NewClient(Server{Type: "plex", URL: srv.URL, Token: "T"}, newTestClient(t))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := cli.Notify(context.Background(), "x", "y"); err == nil {
		t.Fatal("expected unsupported error, got nil")
	}
}

// TestGenerateCover 端到端拼图：mock media server 返回条目 + 海报图，断言输出合法 JPEG 且尺寸正确。
func TestGenerateCover(t *testing.T) {
	posterBytes := func() []byte {
		img := solidImage(color.RGBA{R: 255, G: 0, B: 0, A: 255}, 400, 600)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, nil); err != nil {
			t.Fatalf("jpeg encode: %v", err)
		}
		return buf.Bytes()
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/Items" && r.URL.Query().Get("ParentId") != "":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{
				{"Id": "1", "Name": "A"}, {"Id": "2", "Name": "B"},
			}})
		case r.URL.Path == "/Items/1/Images/Primary", r.URL.Path == "/Items/2/Images/Primary":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(posterBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cli, err := NewClient(Server{Type: "emby", URL: srv.URL, Token: "T"}, newTestClient(t))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	opt := CoverOptions{Cols: 2, Rows: 1, ThumbW: 50, ThumbH: 75}
	data, err := GenerateCover(context.Background(), cli, "lib", opt)
	if err != nil {
		t.Fatalf("GenerateCover: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty cover bytes")
	}
	if ct := http.DetectContentType(data); ct != "image/jpeg" {
		t.Errorf("content type = %q, want image/jpeg", ct)
	}
	// 解码验证画布尺寸 = cols*thumbW × rows*thumbH。
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.Width != 100 || cfg.Height != 75 {
		t.Errorf("cover size = %dx%d, want 100x75", cfg.Width, cfg.Height)
	}
}

// TestResizeImage 缩放后尺寸正确，且纯色源缩放后中心像素保持颜色。
func TestResizeImage(t *testing.T) {
	src := solidImage(color.RGBA{R: 10, G: 20, B: 30, A: 255}, 60, 90)
	dst := resizeImage(src, 6, 9)
	if dst.Bounds().Dx() != 6 || dst.Bounds().Dy() != 9 {
		t.Fatalf("dst size = %dx%d", dst.Bounds().Dx(), dst.Bounds().Dy())
	}
	r, g, b, _ := dst.At(3, 4).RGBA()
	if r>>8 != 10 || g>>8 != 20 || b>>8 != 30 {
		t.Errorf("center pixel = (%d,%d,%d), want (10,20,30)", r>>8, g>>8, b>>8)
	}
}

// TestGenerateCoverSkipBadImage 单张海报拉取失败（404）被跳过，整体仍成功。
func TestGenerateCoverSkipBadImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/Items":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"Items":[{"Id":"1","Name":"A"}]}`)
		case r.URL.Path == "/Items/1/Images/Primary":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cli, err := NewClient(Server{Type: "emby", URL: srv.URL, Token: "T"}, newTestClient(t))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	data, err := GenerateCover(context.Background(), cli, "lib", CoverOptions{Cols: 1, Rows: 1, ThumbW: 20, ThumbH: 30})
	if err != nil {
		t.Fatalf("GenerateCover should tolerate bad image: %v", err)
	}
	if ct := http.DetectContentType(data); ct != "image/jpeg" {
		t.Errorf("content type = %q", ct)
	}
}
