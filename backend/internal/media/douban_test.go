package media

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoubanDetailReturnsCompleteCastAndRequestsTwentyRecommendations(t *testing.T) {
	actors := make([]map[string]any, 24)
	for i := range actors {
		actors[i] = map[string]any{
			"name": fmt.Sprintf("演员 %d", i+1), "character": "角色",
			"avatar": map[string]any{"large": "https://img9.doubanio.com/view/celebrity/m/public/p1.jpg"},
		}
	}
	recommendations := make([]map[string]any, 16)
	for i := range recommendations {
		recommendations[i] = map[string]any{
			"id": fmt.Sprintf("%d", i+1), "title": fmt.Sprintf("推荐 %d", i+1),
			"pic": map[string]any{"large": "https://img9.doubanio.com/view/photo/m_ratio_poster/public/p1.jpg"},
		}
	}
	recommendationCount := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/1":
			_, _ = w.Write([]byte(`{"title":"测试电影"}`))
		case "/movie/1/celebrities":
			_ = json.NewEncoder(w).Encode(map[string]any{"actors": actors})
		case "/movie/1/recommendations":
			recommendationCount = r.URL.Query().Get("count")
			_ = json.NewEncoder(w).Encode(recommendations)
		case "/movie/1/photos":
			_, _ = w.Write([]byte(`{"photos":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	douban := NewDouban("", "test-key")
	douban.base = srv.URL
	detail, err := douban.Detail(context.Background(), "movie:1")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Cast) != len(actors) {
		t.Fatalf("cast count = %d, want %d", len(detail.Cast), len(actors))
	}
	if len(detail.Recommendations) != len(recommendations) {
		t.Fatalf("recommendation count = %d, want %d", len(detail.Recommendations), len(recommendations))
	}
	if recommendationCount != "20" {
		t.Fatalf("recommendation request count = %q, want 20", recommendationCount)
	}
}

// TestDoubanSearchAndDetail：canned frodo JSON 解析 + 微信 UA/apiKey 注入校验。
func TestDoubanSearchAndDetail(t *testing.T) {
	var gotUA, gotReferer, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotReferer = r.Header.Get("Referer")
		gotKey = r.URL.Query().Get("apiKey")
		switch r.URL.Path {
		case "/search":
			// 2024 起 frodo 改版：subjects.items + target_type
			_, _ = w.Write([]byte(`{"subjects":{"items":[
				{"target_type":"movie","target":{"id":"1292052","title":"肖申克的救赎","year":"1994","card_subtitle":"弗兰克 / 1994 / 美国","cover_url":"https://img.doubanio.com/x.jpg","rating":{"value":9.7}}},
				{"target_type":"book","target":{"id":"1","title":"应被过滤的书"}}
			]}}`))
		case "/movie/1292052":
			_, _ = w.Write([]byte(`{"title":"肖申克的救赎","original_title":"The Shawshank Redemption","year":"1994",
				"summary":"希望是好事","rating":{"value":9.7},"images":{"large":"https://img.doubanio.com/large.jpg"},
				"genres":["犯罪","剧情"],"pubdate":["1994-09-10"]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	d := NewDouban("", "test-key")
	d.base = srv.URL // 指向测试服务器

	res, err := d.Search(context.Background(), "肖申克", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotUA, "MicroMessenger") {
		t.Errorf("User-Agent = %q, want MicroMessenger (微信 UA)", gotUA)
	}
	if !strings.Contains(gotReferer, "servicewechat.com") {
		t.Errorf("Referer = %q, want servicewechat.com", gotReferer)
	}
	if gotKey != "test-key" {
		t.Errorf("apiKey = %q, want test-key", gotKey)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 movie (book filtered), got %d", len(res))
	}
	m := res[0]
	if m.Title != "肖申克的救赎" || m.SourceID != "movie:1292052" || m.Year != 1994 || m.Rating != 9.7 {
		t.Errorf("search result = %+v", m)
	}
	if m.PosterURL != "https://img.doubanio.com/x.jpg" || m.Type != TypeMovie || m.Source != "douban" {
		t.Errorf("search result fields = %+v", m)
	}
	if m.Overview != "弗兰克 / 1994 / 美国" {
		t.Errorf("overview (card_subtitle) = %q", m.Overview)
	}

	det, err := d.Detail(context.Background(), "movie:1292052")
	if err != nil {
		t.Fatal(err)
	}
	if det.Title != "肖申克的救赎" || det.Year != 1994 || len(det.Genres) != 2 || det.Overview != "希望是好事" {
		t.Errorf("detail = %+v", det)
	}
	if det.PosterURL != "https://img.doubanio.com/large.jpg" {
		t.Errorf("detail poster = %q", det.PosterURL)
	}
}

// Current Frodo detail payloads use intro/pic/actors and scalar pubdate. Keep this
// regression test aligned with the live payload so the detail page does not silently
// lose its poster, synopsis, and cast after an upstream field migration.
func TestDoubanDetailCurrentFrodoFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/36953973" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{
			"title":"特立独行","original_title":"The Standalone","year":2026,
			"intro":"这是一段当前字段名的简介。","rating":{"value":7.4,"count":1200},
			"pic":{"large":"https://img3.doubanio.com/view/photo/m_ratio_poster/public/p2933638302.jpg"},
			"genres":["剧情","喜剧"],"pubdate":"2026-07-03(中国大陆)",
			"countries":["中国大陆"],"durations":["109分钟"],
			"directors":[{"name":"邢文雄"}],
			"actors":[{"name":"白敬亭"},{"name":"魏翔"}]
		}`))
	}))
	defer srv.Close()

	d := NewDouban("", "test-key")
	d.base = srv.URL
	got, err := d.Detail(context.Background(), "movie:36953973")
	if err != nil {
		t.Fatal(err)
	}
	if got.PosterURL != "https://img9.doubanio.com/view/photo/m_ratio_poster/public/p2933638302.jpg" {
		t.Fatalf("poster_url = %q", got.PosterURL)
	}
	if got.Overview != "这是一段当前字段名的简介。" {
		t.Fatalf("overview = %q", got.Overview)
	}
	if got.ReleaseDate != "2026-07-03(中国大陆)" || got.Year != 2026 {
		t.Fatalf("release date/year = %q/%d", got.ReleaseDate, got.Year)
	}
	if len(got.Cast) != 2 || got.Cast[0].Name != "白敬亭" || got.Cast[1].Name != "魏翔" {
		t.Fatalf("cast = %+v", got.Cast)
	}
}

// TestDoubanDetailEnrichments：detail 后并行补 celebrities（带头像演员）/recommendations
// （裸数组）/photos（横幅剧照）。钉死：① celebrity 头像原样返回，不被 doubanPosterURL 的
// (p\d+)\.jpg 误重写成不存在的海报路径；② 推荐从 year 取年，缺失则从 card_subtitle 提取；
// ③ backdrop 跳过竖图、取首张横幅剧照。
//
// 注：TestDoubanDetailCurrentFrodoFields 的 handler 只认 detail path（其余 404）→ 走降级路径
// （cast 回退 detail 裸 actors）；本测走成功路径，两条路径互补。
func TestDoubanDetailEnrichments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/1":
			_, _ = w.Write([]byte(`{"title":"X","year":"2026","pic":{"large":"https://img9.doubanio.com/view/photo/m_ratio_poster/public/p1.jpg"}}`))
		case "/movie/1/celebrities":
			_, _ = w.Write([]byte(`{"directors":[],"actors":[
				{"name":"白敬亭","character":"男主","uri":"douban://douban.com/celebrity/1?subject_id=1001","avatar":{"large":"https://img1.doubanio.com/view/personage/m/public/p1.jpg"}},
				{"name":"魏翔","character":"男二","uri":"douban://douban.com/celebrity/2?subject_id=1002","avatar":{"large":"https://img3.doubanio.com/view/personage/m/public/p2.jpg"}}
			]}`))
		case "/movie/1/recommendations":
			_, _ = w.Write([]byte(`[
				{"id":"36999793","title":"类似片A","year":"2026","pic":{"large":"https://img2.doubanio.com/view/photo/m_ratio_poster/public/p2929759261.jpg"},"rating":{"value":7.4}},
				{"id":"123","title":"类似片B","card_subtitle":"2025 / 中国","pic":{"large":"https://img9.doubanio.com/view/photo/l/public/p999.jpg"},"rating":{"value":6.0}}
			]`))
		case "/movie/1/photos":
			_, _ = w.Write([]byte(`{"photos":[
				{"image":{"large":{"url":"https://img.doubanio.com/view/photo/vertical.jpg","width":400,"height":600}}},
				{"image":{"large":{"url":"https://img.doubanio.com/view/photo/banner.jpg","width":1920,"height":1080}}}
			]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	d := NewDouban("", "test-key")
	d.base = srv.URL
	got, err := d.Detail(context.Background(), "movie:1")
	if err != nil {
		t.Fatal(err)
	}
	// 演员：celebrities 带头像，avatar 原样（personage 路径不可过 doubanPosterURL）
	if len(got.Cast) != 2 || got.Cast[0].Name != "白敬亭" {
		t.Fatalf("cast = %+v", got.Cast)
	}
	if got.Cast[0].Avatar != "https://img1.doubanio.com/view/personage/m/public/p1.jpg" {
		t.Errorf("cast[0].avatar = %q, want personage 原样（不被重写成 m_ratio_poster）", got.Cast[0].Avatar)
	}
	if got.Cast[0].ID != "1001" {
		t.Errorf("cast[0].id = %q, want 1001（从 actor.uri 提取 subject_id）", got.Cast[0].ID)
	}
	if got.Cast[0].Role != "男主" {
		t.Errorf("cast[0].role = %q, want 男主", got.Cast[0].Role)
	}
	// 推荐：裸数组映射；rec[1] 无 year 字段，从 card_subtitle "2025 / 中国" 提取
	if len(got.Recommendations) != 2 {
		t.Fatalf("recommendations = %+v", got.Recommendations)
	}
	r0 := got.Recommendations[0]
	if r0.SourceID != "movie:36999793" || r0.Title != "类似片A" || r0.Year != 2026 || r0.Rating != 7.4 {
		t.Errorf("rec[0] = %+v", r0)
	}
	if r0.PosterURL != "https://img9.doubanio.com/view/photo/m_ratio_poster/public/p2929759261.jpg" {
		t.Errorf("rec[0].poster = %q（应经 doubanPosterURL 规整为 m_ratio_poster）", r0.PosterURL)
	}
	if got.Recommendations[1].Year != 2025 {
		t.Errorf("rec[1].year = %d, want 2025（从 card_subtitle 提取）", got.Recommendations[1].Year)
	}
	// backdrop：跳过竖图，取首张横幅剧照
	if got.BackdropURL != "https://img.doubanio.com/view/photo/banner.jpg" {
		t.Errorf("backdrop = %q, want 横幅剧照 banner.jpg", got.BackdropURL)
	}
}

// TestDoubanCelebrity：点演员头像拉的简介。/elessar/subject/{id} 返回 desc（HTML 多段 <p>）+
// extra.info 结构化 + short_info + modules.work_collections 参演作品。钉死：goquery 解析 desc 成段落、
// info/works 映射、avatar 原样（personage 路径不过 doubanPosterURL）。
func TestDoubanCelebrity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/elessar/subject/1001" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{
			"name":"白敬亭","latin_title":"Bai Jingting",
			"cover":{"large":{"url":"https://img1.doubanio.com/view/personage/l/public/p1.jpg"}},
			"followed_count":1234,
			"desc":"<div class=\"content\"><p>第一段生平。</p><p>第二段生平。</p></div>",
			"url":"https://www.douban.com/personage/1001",
			"extra":{"info":[["性别","男"],["出生日期","1993年"]],"short_info":"演员 / 作品A 作品B"},
			"modules":[{"type":"work_collections","payload":{"collections":[{"works":[
				{"roles":["演员 - 男主"],"subject":{"title":"作品A","url":"https://movie.douban.com/subject/a"}},
				{"roles":["演员"],"subject":{"title":"作品B","url":"https://movie.douban.com/subject/b"}}
			]}]}}]
		}`))
	}))
	defer srv.Close()
	d := NewDouban("", "test-key")
	d.base = srv.URL
	p, err := d.Celebrity(context.Background(), "1001")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "白敬亭" || p.LatinName != "Bai Jingting" {
		t.Errorf("name/latin = %q/%q", p.Name, p.LatinName)
	}
	// avatar 原样（personage 路径不过 doubanPosterURL）
	if p.Avatar != "https://img1.doubanio.com/view/personage/l/public/p1.jpg" {
		t.Errorf("avatar = %q, want 原样 personage/l", p.Avatar)
	}
	if len(p.Bio) != 2 || p.Bio[0] != "第一段生平。" || p.Bio[1] != "第二段生平。" {
		t.Errorf("bio = %v（应 goquery 解析 2 段 <p>）", p.Bio)
	}
	if len(p.Info) != 2 || p.Info[0] != [2]string{"性别", "男"} {
		t.Errorf("info = %v", p.Info)
	}
	if p.ShortInfo != "演员 / 作品A 作品B" || p.FollowedCount != 1234 {
		t.Errorf("short_info/followed = %q/%d", p.ShortInfo, p.FollowedCount)
	}
	if len(p.Works) != 2 || p.Works[0].Title != "作品A" || p.Works[0].Role != "演员 - 男主" {
		t.Errorf("works = %+v", p.Works)
	}
	if p.DoubanURL != "https://www.douban.com/personage/1001" {
		t.Errorf("douban_url = %q", p.DoubanURL)
	}
}

func TestDoubanDetailYearFallsBackWhenPubdateMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"title":"待映影片","year":"2027","intro":"简介"}`))
	}))
	defer srv.Close()
	d := NewDouban("", "test-key")
	d.base = srv.URL
	got, err := d.Detail(context.Background(), "movie:1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Year != 2027 {
		t.Fatalf("year = %d, want 2027", got.Year)
	}
}

// TestDoubanYearFilter：年份容差 ±1 过滤。
func TestDoubanYearFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"type":"movie","target":{"id":"1","title":"A","year":"2020"}}]}`))
	}))
	defer srv.Close()
	d := NewDouban("", "")
	d.base = srv.URL

	// 搜 2022，结果 2020（差 2 年）→ 过滤
	res, _ := d.Search(context.Background(), "A", 2022)
	if len(res) != 0 {
		t.Errorf("expected filtered by year, got %d", len(res))
	}
	// 搜 2021（差 1 年，容差内）→ 保留
	res, _ = d.Search(context.Background(), "A", 2021)
	if len(res) != 1 {
		t.Errorf("expected within ±1 year tolerance, got %d", len(res))
	}
}

// TestDoubanUsesDefaultKey：apiKey 为空时用公开默认值（开箱即用）。
func TestDoubanUsesDefaultKey(t *testing.T) {
	var key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.URL.Query().Get("apiKey")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()
	d := NewDouban("", "")
	d.base = srv.URL
	_, _ = d.Search(context.Background(), "x", 0)
	if key != doubanDefaultKey {
		t.Errorf("apiKey = %q, want default %q", key, doubanDefaultKey)
	}
}

// TestDoubanPosterURL：把 frodo 返回的横裁/防盗链 cover_url 规整成 m_ratio_poster 竖版海报。
func TestDoubanPosterURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{
			"https://qnmob3.doubanio.com/view/photo/large/public/p2911205318.jpg?imageView2/0/q/80/w/9999/h/120/format/jpg",
			"https://img9.doubanio.com/view/photo/m_ratio_poster/public/p2911205318.jpg",
		},
		{
			"https://img9.doubanio.com/view/photo/l/public/p2483150779.jpg",
			"https://img9.doubanio.com/view/photo/m_ratio_poster/public/p2483150779.jpg",
		},
		{"", ""}, // 空
		{"https://img9.doubanio.com/view/subject/m/public/s33778346.jpg", // s* 封面无法换算，原样
			"https://img9.doubanio.com/view/subject/m/public/s33778346.jpg"},
	}
	for _, c := range cases {
		if got := doubanPosterURL(c.in); got != c.want {
			t.Errorf("doubanPosterURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFrodoSign：用 bestyize/DoubanAPI 真实抓包向量钉死算法+密钥正确性。
//
//	（抓自豆瓣 App：GET /api/v2/elessar/subject/27260217/photos, _ts=1599316450,
//	 _sig=gfTX2YbSiADYaG/zXJ+pNo5IhbI=, key=0dad551ec0f84ed02907ff5c42e8ec70, secret=bf7dddc7c9cfe6f7）
func TestFrodoSign(t *testing.T) {
	got := frodoSign(http.MethodGet, "/api/v2/elessar/subject/27260217/photos", "1599316450", "bf7dddc7c9cfe6f7")
	const want = "gfTX2YbSiADYaG/zXJ+pNo5IhbI="
	if got != want {
		t.Fatalf("frodoSign = %q, want %q（算法/密钥与真实抓包不一致）", got, want)
	}
}

// TestPctEncode：等价 Python parse.quote(s, safe="")——/ 与空格均编码，unreserved 直通。
func TestPctEncode(t *testing.T) {
	cases := map[string]string{
		"/api/v2/search": "%2Fapi%2Fv2%2Fsearch",
		"/a b/c":         "%2Fa%20b%2Fc",
		"ABC-_.~09":      "ABC-_.~09",
	}
	for in, want := range cases {
		if got := pctEncode(in); got != want {
			t.Errorf("pctEncode(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDoubanSignedRequest：secret 配了 → 签名策略：安卓 UA、_sig（对实际 path 重算一致）、
// _ts、os_rom=android、安卓 key、无 servicewechat Referer。
func TestDoubanSignedRequest(t *testing.T) {
	var gotUA, gotReferer, gotKey, gotSig, gotTS, gotOS, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotReferer = r.Header.Get("Referer")
		gotKey = r.URL.Query().Get("apiKey")
		gotSig = r.URL.Query().Get("_sig")
		gotTS = r.URL.Query().Get("_ts")
		gotOS = r.URL.Query().Get("os_rom")
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	d := NewDouban("bf7dddc7c9cfe6f7", "") // secret 非空 → 签名为主
	d.base = srv.URL
	if _, err := d.Search(context.Background(), "x", 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotUA, "com.douban.frodo") {
		t.Errorf("UA = %q, want 含 com.douban.frodo（安卓）", gotUA)
	}
	if gotReferer != "" {
		t.Errorf("签名策略不应带 Referer, got %q", gotReferer)
	}
	if gotKey != doubanSignedKey {
		t.Errorf("apiKey = %q, want signed key %q", gotKey, doubanSignedKey)
	}
	if gotOS != "android" {
		t.Errorf("os_rom = %q, want android", gotOS)
	}
	if gotTS == "" {
		t.Error("_ts 缺失")
	}
	// _sig 必须与对实际请求 path（测试 base 无前缀 → 即 /search）用默认 secret 重算一致
	if want := frodoSign(http.MethodGet, gotPath, gotTS, "bf7dddc7c9cfe6f7"); gotSig != want {
		t.Errorf("_sig = %q, want %q", gotSig, want)
	}
}

// TestDoubanSignedFallsBackToWechat：签名策略 401（坏 sig）→ 自动回退微信免签。
func TestDoubanSignedFallsBackToWechat(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.Header.Get("User-Agent"), "com.douban.frodo"): // 签名尝试
			hits = append(hits, "signed")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"msg":"wrong_sig"}`))
		default: // 微信兜底
			hits = append(hits, "wechat")
			_, _ = w.Write([]byte(`{"items":[]}`))
		}
	}))
	defer srv.Close()
	d := NewDouban("bf7dddc7c9cfe6f7", "")
	d.base = srv.URL
	if _, err := d.Search(context.Background(), "x", 0); err != nil {
		t.Fatalf("应回退成功, err=%v", err)
	}
	if len(hits) != 2 || hits[0] != "signed" || hits[1] != "wechat" {
		t.Errorf("hits = %v, want [signed wechat]", hits)
	}
}

// TestDoubanWechatFallsBackToSigned：零配置（微信为主）下微信 403（白名单失效）→
// 回退签名（公开默认凭据），证明零配置下签名 fallback 也可用。
func TestDoubanWechatFallsBackToSigned(t *testing.T) {
	var (
		hits               []string
		signedKey, sig, ts string
		signedPath         string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.Header.Get("User-Agent"), "MicroMessenger"): // 微信尝试
			hits = append(hits, "wechat")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"msg":"api_key forbidden"}`))
		default: // 签名兜底（公开默认 key/secret）
			hits = append(hits, "signed")
			signedKey = r.URL.Query().Get("apiKey")
			sig = r.URL.Query().Get("_sig")
			ts = r.URL.Query().Get("_ts")
			signedPath = r.URL.Path
			_, _ = w.Write([]byte(`{"items":[]}`))
		}
	}))
	defer srv.Close()
	d := NewDouban("", "") // 零配置
	d.base = srv.URL
	if _, err := d.Search(context.Background(), "x", 0); err != nil {
		t.Fatalf("应回退成功, err=%v", err)
	}
	if len(hits) != 2 || hits[0] != "wechat" || hits[1] != "signed" {
		t.Errorf("hits = %v, want [wechat signed]", hits)
	}
	if signedKey != doubanSignedKey {
		t.Errorf("回退签名 apiKey = %q, want %q", signedKey, doubanSignedKey)
	}
	// 回退 _sig 必须对实际 path 用公开默认 secret 重算一致 → 证明零配置签名凭据已生效
	if want := frodoSign(http.MethodGet, signedPath, ts, doubanSignedSecret); sig != want {
		t.Errorf("回退 _sig = %q, want %q", sig, want)
	}
}

// TestDoubanFallbackOn400Sig：400 带(api_key|sig)的响应视为鉴权失败 → 自动回退另一策略。
func TestDoubanFallbackOn400Sig(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if strings.Contains(r.Header.Get("User-Agent"), "MicroMessenger") { // 微信尝试
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"msg":"api_key invalid"}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[]}`)) // 签名兜底
	}))
	defer srv.Close()
	d := NewDouban("", "")
	d.base = srv.URL
	if _, err := d.Search(context.Background(), "x", 0); err != nil {
		t.Fatalf("应回退成功, err=%v", err)
	}
	if hits != 2 {
		t.Errorf("hits = %d, want 2（400+api_key 应触发回退）", hits)
	}
}

// TestDoubanNoFallbackOnNonAuth：404/429 等非鉴权错不触发回退（只打一次）。
func TestDoubanNoFallbackOnNonAuth(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"404", http.StatusNotFound, `{"msg":"not_found"}`},
		{"429", 429, `{"msg":"rate"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits++
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			d := NewDouban("", "")
			d.base = srv.URL
			if _, err := d.Search(context.Background(), "x", 0); err == nil {
				t.Fatal("want error, got nil")
			}
			if hits != 1 {
				t.Errorf("hits = %d, want 1（非鉴权错不应回退）", hits)
			}
		})
	}
}
