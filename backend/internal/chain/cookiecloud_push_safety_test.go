package chain

// CookieCloud 推送安全性：拉取失败时绝不能「从空 base 推送」——CookieCloud 的 PUT 是
// 整体替换，那样会把云端已有的其它 host 抹掉（别的设备要重新推一遍）。
// 只有「云端确实没数据」（首次推送）才允许以空 base 起步。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// ccServer 假 CookieCloud：GET /get 按 getResponse 出招，POST /update 计数。
func ccServer(t *testing.T, getResponse func(w http.ResponseWriter), posts *atomic.Int64) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			_, _ = w.Write([]byte(`{"action":"done"}`))
			return
		}
		getResponse(w)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func ccChainWithLocalCookie(t *testing.T) *CookieCloudChain {
	t.Helper()
	crypto := security.NewCrypto("cc-push-safety")
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, crypto)
	if _, err := siteOp.Create(oper.SiteInput{
		Name: "Demo", Platform: "nexusphp", BaseURL: "https://demo.pt", Cookie: "sess=local1; uid=9", IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	return NewCookieCloudChain(&memStore{}, crypto, siteOp, site.NewRegistry(), nil, hc)
}

func ccSettings(t *testing.T, server, uuid, password string) CookieCloudSettings {
	t.Helper()
	s := CookieCloudSettings{Server: server, UUID: uuid, Direction: "push"}
	s.PasswordEnc, _ = security.NewCrypto("cc-push-safety").Encrypt(password)
	return s
}

// TestDoSync_FetchHTTPErrorSkipsPush 服务端报错（5xx / 404 空信封）时：不推送、报明确原因。
// 旧行为是从空 map 起步推送 → 云端其它 host 被整体替换掉。
// 响应体特意写成「看起来就是云端没数据」的空信封：只有状态码检查能把这种情形挡住。
func TestDoSync_FetchHTTPErrorSkipsPush(t *testing.T) {
	var posts atomic.Int64
	srv := ccServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"encrypted":"","saved_at":0}`))
	}, &posts)

	ch := ccChainWithLocalCookie(t)
	_, err := ch.doSync(context.Background(), ccSettings(t, srv, "u-500", "pass"))
	if err == nil || !strings.Contains(err.Error(), "跳过本次推送") {
		t.Fatalf("拉取 5xx 应跳过推送并说明原因, got %v", err)
	}
	if got := posts.Load(); got != 0 {
		t.Errorf("跳过推送时不该有 PUT 上传（会覆盖云端其它 host）, posts=%d", got)
	}
}

// TestDoSync_DecryptFailureSkipsPush 密码不匹配/云端被别台设备用不同密码覆写时，
// 拉到的密文解不开——同样不能当「云端没数据」去推。
func TestDoSync_DecryptFailureSkipsPush(t *testing.T) {
	var posts atomic.Int64
	// 用另一个密码加密的合法信封：状态 200、字段齐，但本端密码解不开。
	uuid, wrongPW := "u-dec", "other-device-password"
	enc := ccEncrypt(t, `{"cookie_data":{"keep.me":[{"name":"k","value":"v","domain":"keep.me","path":"/"}]}}`, uuid, wrongPW)
	srv := ccServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"encrypted":"` + enc + `"}`))
	}, &posts)

	ch := ccChainWithLocalCookie(t)
	_, err := ch.doSync(context.Background(), ccSettings(t, srv, uuid, "local-password"))
	if err == nil || !strings.Contains(err.Error(), "跳过本次推送") {
		t.Fatalf("解密失败应跳过推送, got %v", err)
	}
	if got := posts.Load(); got != 0 {
		t.Errorf("解密失败仍推送会抹掉云端 keep.me 等其它 host, posts=%d", got)
	}
}

// TestDoSync_FirstPushOnEmptyCloud 正控：云端确实还没有数据（encrypted 为空）时，
// 首次推送必须照常进行——修复不能把新配好的 CookieCloud 堵死。
func TestDoSync_FirstPushOnEmptyCloud(t *testing.T) {
	var posts atomic.Int64
	srv := ccServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"encrypted":"","saved_at":0}`))
	}, &posts)

	ch := ccChainWithLocalCookie(t)
	report, err := ch.doSync(context.Background(), ccSettings(t, srv, "u-first", "pass"))
	if err != nil {
		t.Fatalf("云端为空的首次推送不该报错: %v", err)
	}
	if report.Pushed != 1 || posts.Load() == 0 {
		t.Errorf("首次推送应上传本地站点, pushed=%d posts=%d", report.Pushed, posts.Load())
	}
}

// TestDoSync_BuiltinEmptyStillPushes 正控：内置模式下 store 还没数据（扩展尚未推过）
// 属于「云端确实为空」，首次推送必须照常——不能被「拉取失败就不推」的新规则误伤。
func TestDoSync_BuiltinEmptyStillPushes(t *testing.T) {
	ch := ccChainWithLocalCookie(t)
	crypto := security.NewCrypto("cc-push-safety")
	s := CookieCloudSettings{Mode: CookieCloudModeBuiltin, UUID: "u-builtin", Direction: "push"}
	s.PasswordEnc, _ = crypto.Encrypt("pass")

	report, err := ch.doSync(context.Background(), s)
	if err != nil {
		t.Fatalf("内置模式空数据的首次推送不该报错: %v", err)
	}
	if report.Pushed != 1 {
		t.Errorf("应把本地站点写进内置 store, pushed=%d", report.Pushed)
	}
}
