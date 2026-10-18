package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// fetchUserInfoErr 与 fetchTestUserInfo 同构造，但把错误交回调用方断言。
func fetchUserInfoErr(t *testing.T, id, baseURL string) (*site.UserInfo, error) {
	t.Helper()
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition(id)
	if !ok {
		t.Fatalf("%s definition missing", id)
	}
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	return NewNexusPHP(hc).FetchUserInfo(context.Background(), def, baseURL, "cookie")
}

// TestFetchUserInfoRejectsServer5xx 站点返回 5xx 错误页时必须报错。
// 回归：fetchDoc 只对 CF 盾报错，502/503 的 HTML 会被 goquery 正常解析、选择器全落空，
// 上层拿到「什么都没解析到但没有错误」的结果。
func TestFetchUserInfoRejectsServer5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`<html><body>502 Bad Gateway</body></html>`))
	}))
	defer srv.Close()

	u, err := fetchUserInfoErr(t, "1ptba", srv.URL)
	if err == nil {
		t.Fatalf("5xx 必须报错，got user=%+v", u)
	}
	if !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("错误应带状态码，got %v", err)
	}
}

// TestFetchUserInfoRejectsEmptyParse 200 但什么都没解析到（改版/错误页）时同样要报错：
// 全 0 的 UserInfo 一旦被当成功返回，聚合任务会写进 site_stats 并标 connected，
// 仪表盘于是呈现「分享率一夜跌到 0」的假事故，真因被埋。
func TestFetchUserInfoRejectsEmptyParse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>系统提示</title></head><body>页面不存在或已改版</body></html>`))
	}))
	defer srv.Close()

	u, err := fetchUserInfoErr(t, "1ptba", srv.URL)
	if err == nil {
		t.Fatalf("空解析必须报错，got user=%+v", u)
	}
	if !strings.Contains(err.Error(), "未从") || !strings.Contains(err.Error(), "解析到任何用户信息") {
		t.Errorf("错误应说明没解析到东西，got %v", err)
	}
}

// TestFetchUserInfoAcceptsZeroValueAccount 正控（关键误杀边界）：
// 真·新号——有用户名、数值全是 0——绝不能被当成「空解析」拒掉，
// 否则小号用户的仪表盘永远显示抓取失败。
func TestFetchUserInfoAcceptsZeroValueAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.php" {
			_, _ = w.Write([]byte(`<div id="info_block"><a href="userdetails.php?id=999">newbie-user</a></div>`))
			return
		}
		_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
	}))
	defer srv.Close()

	u, err := fetchUserInfoErr(t, "1ptba", srv.URL)
	if err != nil {
		t.Fatalf("有用户名的全 0 新号应正常返回: %v", err)
	}
	if u.Name != "newbie-user" {
		t.Fatalf("用户名应解析出来，got %q", u.Name)
	}
	if u.Uploaded != 0 || u.Bonus != 0 {
		t.Errorf("该号数值本就为 0，got uploaded=%d bonus=%g", u.Uploaded, u.Bonus)
	}
}
