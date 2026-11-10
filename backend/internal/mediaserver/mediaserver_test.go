// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package mediaserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
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
	data, _, err := GenerateCover(context.Background(), cli, "lib", opt)
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

// TestEmbyAdvanced 跑通观影报告/重复检测所需端点：Users / PlayedItems / LibraryItems。
func TestEmbyAdvanced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Users":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"Id":"u1","Name":"Alice"},{"Id":"u2","Name":"Bob"}]`))
		case "/Users/u1/Items":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Items":[{"Id":"i1","Name":"Movie A","Type":"Movie","ProductionYear":2021,"UserData":{"PlayCount":3,"LastPlayedDate":"2024-01-02T03:04:05Z","RuntimeTicks":66000000000}}]}`))
		case "/Items":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Items":[
				{"Id":"a","Name":"Dune","Type":"Movie","ProductionYear":2021,"ProviderIds":{"Tmdb":"438631"}},
				{"Id":"b","Name":"Dune (2021)","Type":"Movie","ProductionYear":2021,"ProviderIds":{"Tmdb":"438631"}},
				{"Id":"c","Name":"Oldboy","Type":"Movie","ProductionYear":2003,"ProviderIds":{"Imdb":"tt007"}}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cli, err := NewClient(Server{Type: "emby", URL: srv.URL, Token: "T"}, newTestClient(t))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	users, err := cli.Users(ctx)
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	if len(users) != 2 || users[0].ID != "u1" || users[0].Name != "Alice" {
		t.Fatalf("users = %+v", users)
	}

	plays, err := cli.PlayedItems(ctx, "u1", 10)
	if err != nil {
		t.Fatalf("PlayedItems: %v", err)
	}
	if len(plays) != 1 || plays[0].Name != "Movie A" || plays[0].PlayCount != 3 || plays[0].Duration != 6600 {
		t.Fatalf("plays = %+v", plays)
	}

	items, err := cli.LibraryItems(ctx, "lib")
	if err != nil {
		t.Fatalf("LibraryItems: %v", err)
	}
	if len(items) != 3 || items[0].TMDBID != "438631" || items[2].IMDbID != "tt007" {
		t.Fatalf("items = %+v", items)
	}
}

// TestDetectDuplicates 按 TMDB ID（优先）/IMDb ID/归一化标题+年份归并，仅返回 Count>=2 的组。
func TestDetectDuplicates(t *testing.T) {
	items := []DetailedItem{
		{ItemID: "1", Name: "Dune", Year: 2021, TMDBID: "438631"},
		{ItemID: "2", Name: "Dune (2021)", Year: 2021, TMDBID: "438631"},
		{ItemID: "3", Name: "Oldboy", Year: 2003, IMDbID: "tt007"},
		{ItemID: "4", Name: "Oldboy", Year: 2013, IMDbID: "tt007"}, // 同 IMDb 但不同年份（翻拍）→ 仍判重
		{ItemID: "5", Name: "Inception.2010", Year: 2010},
		{ItemID: "6", Name: "Inception (2010)", Year: 2010}, // 无 Provider ID，按归一化标题+年份归并
		{ItemID: "7", Name: "Solo", Year: 2018},             // 无重复
	}
	groups := DetectDuplicates(items)
	if len(groups) != 3 {
		t.Fatalf("expected 3 duplicate groups, got %d: %+v", len(groups), groups)
	}
	// 第一组应是重复数最多的（TMDB/IMDb 组各 2，标题组 2；按标题稳定排序）。
	want := map[string]int{}
	for _, g := range groups {
		want[g.Title] = g.Count
	}
	if want["Dune"] != 2 || want["Oldboy"] != 2 || want["Inception.2010"] != 2 {
		t.Fatalf("group counts unexpected: %+v", want)
	}
}

// TestDetectDuplicatesEmpty 空输入返回空切片（非 nil）。
func TestDetectDuplicatesEmpty(t *testing.T) {
	got := DetectDuplicates(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

// TestNormalizeTitle 验证归一化：去尾部年份块 + 折叠分隔符。
func TestNormalizeTitle(t *testing.T) {
	cases := map[string]string{
		"Inception.2010":     "inception",
		"Inception (2010)":   "inception",
		"  Movie_Name-2023 ": "movie name",
		"AlreadyClean":       "alreadyclean",
	}
	for in, want := range cases {
		if got := normalizeTitle(in); got != want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGenerateCoverSkipBadImage(t *testing.T) {
	poster := func() []byte {
		img := solidImage(color.RGBA{R: 0, G: 200, B: 0, A: 255}, 60, 90)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, nil); err != nil {
			t.Fatalf("jpeg encode: %v", err)
		}
		return buf.Bytes()
	}()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/Items":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{
				{"Id": "1", "Name": "A"}, {"Id": "2", "Name": "B"},
			}})
		case r.URL.Path == "/Items/1/Images/Primary":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(poster)
		default:
			http.NotFound(w, r) // 第二张取不到
		}
	}))
	defer srv.Close()
	cli, err := NewClient(Server{Type: "emby", URL: srv.URL, Token: "T"}, newTestClient(t))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	data, st, err := GenerateCover(context.Background(), cli, "lib", CoverOptions{Cols: 2, Rows: 1, ThumbW: 20, ThumbH: 30})
	if err != nil {
		t.Fatalf("部分封面取不到时仍该出图: %v", err)
	}
	if ct := http.DetectContentType(data); ct != "image/jpeg" {
		t.Errorf("content type = %q", ct)
	}
	// 账面必须把两种留黑原因分开（界面上的黑格子看不出区别，排障只能靠这份数）
	if st.Items != 2 || st.Placed != 1 || st.FetchFail != 1 || st.NoImage != 0 {
		t.Errorf("账面 = %+v，期望 items=2 placed=1 fetch_fail=1 no_image=0", st)
	}
}

// #304：一张都没贴上时必须报错，不能回一张全黑的 200 成功图。
// 黑图在界面上等于「这批新片都没有海报」，而真实情况常常是图片通道整个坏了（未授权/不可达）。
func TestGenerateCoverAllImagesFailIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Items":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{
				{"Id": "1", "Name": "A"}, {"Id": "2", "Name": "B"},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cli, err := NewClient(Server{Type: "emby", URL: srv.URL, Token: "T"}, newTestClient(t))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	data, st, err := GenerateCover(context.Background(), cli, "lib", CoverOptions{Cols: 2, Rows: 1, ThumbW: 20, ThumbH: 30})
	if !errors.Is(err, ErrNoCoverPlaced) {
		t.Fatalf("全失败应回 ErrNoCoverPlaced，got err=%v data=%d bytes", err, len(data))
	}
	if len(data) != 0 {
		t.Errorf("全失败不得返回图片字节（会被界面当成功渲染）")
	}
	if st.Placed != 0 || st.FetchFail != 2 {
		t.Errorf("账面 = %+v，期望 placed=0 fetch_fail=2", st)
	}
	if !strings.Contains(err.Error(), "2") || !strings.Contains(err.Error(), "图片通道") {
		t.Errorf("错误句子要带数字与下一步: %q", err.Error())
	}
	// 正控：库确实是空的 → 不算失败（空态与故障必须能分开）
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Items":[]}`))
	}))
	defer empty.Close()
	ecli, err := NewClient(Server{Type: "emby", URL: empty.URL, Token: "T"}, newTestClient(t))
	if err != nil {
		t.Fatalf("NewClient(empty): %v", err)
	}
	if _, est, eerr := GenerateCover(context.Background(), ecli, "lib", CoverOptions{Cols: 2, Rows: 1, ThumbW: 20, ThumbH: 30}); eerr != nil {
		t.Errorf("空库不该报「取不到封面」: %v", eerr)
	} else if est.Items != 0 {
		t.Errorf("空库账面 items=%d，期望 0", est.Items)
	}
}

// TestClampCoverOptions 防封面接口 OOM：超大 cols/rows/w/h 被夹到安全范围，且总像素有上限。
func TestClampCoverOptions(t *testing.T) {
	got := clampCoverOptions(CoverOptions{Cols: 2000, Rows: 2000, ThumbW: 2000, ThumbH: 2000})
	if got.Cols > maxCoverCols || got.Rows > maxCoverRows {
		t.Errorf("cols/rows not capped: %+v", got)
	}
	if got.ThumbW > maxCoverThumbW || got.ThumbH > maxCoverThumbH {
		t.Errorf("thumb size not capped: %+v", got)
	}
	if px := got.Cols * got.Rows * got.ThumbW * got.ThumbH; px > maxCoverPixels {
		t.Errorf("total pixels %d > cap %d", px, maxCoverPixels)
	}
	// 现实参数不受影响。
	normal := clampCoverOptions(CoverOptions{Cols: 3, Rows: 3, ThumbW: 200, ThumbH: 300})
	if normal != (CoverOptions{Cols: 3, Rows: 3, ThumbW: 200, ThumbH: 300}) {
		t.Errorf("normal options altered: %+v", normal)
	}
}
