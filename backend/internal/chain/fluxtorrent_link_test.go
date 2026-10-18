package chain

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/schemas"
)

const fluxLinkPasskey = "aa11bb22cc33dd44ee55ff66aa11bb22"

// TestFluxTorrentTokenToDownloadLink 端到端钉住 FluxTorrent 对外承诺（README/06 站点定义指南）：
// 只填 fxo_ Token → 首次用户信息抓取从 ptppUserInfo 发现 passkey 并回写 → 下载直链免手填即用。
// 这条链此前只有各段单测（适配器解析 / chain 补全），没有一把抓过「Token→passkey→种子字节」。
func TestFluxTorrentTokenToDownloadLink(t *testing.T) {
	var downloadQueries, downloadCookies []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/plugins/ptppUserInfo", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Token fxo_") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"message":"unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"id":7,"name":"ptp","levelName":"用户",` +
			`"uploaded":1099511627776,"downloaded":536870912,"seeding":3,"leeching":0,"seedingSize":1,` +
			`"bonus":10,"passkey":"` + fluxLinkPasskey + `"}}`))
	})
	mux.HandleFunc("/api/v1/compat/nexusphp/download.php", func(w http.ResponseWriter, r *http.Request) {
		downloadQueries = append(downloadQueries, r.URL.RawQuery)
		downloadCookies = append(downloadCookies, r.Header.Get("Cookie"))
		if r.URL.Query().Get("passkey") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"message":"passkey required"}`))
			return
		}
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write(validTorrentFixture)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	in := oper.SiteInput{Name: "flux", Platform: "fluxtorrent", BaseURL: srv.URL}
	s, err := siteOp.Create(in) // 用户此刻只填了地址，Token 还没填
	if err != nil {
		t.Fatal(err)
	}
	mm := module.New()
	mm.Register(schemas.NewFluxTorrent(hc)) // 与 app_setup 一致：单实例服务全部站点
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())
	ch.SetHTTPClient(hc)
	ctx := context.Background()

	// 1. 没填 Token：错误要指向「API Token」，而不是把 passkey 当 Token 发出去换 401「未登录」
	if _, err := ch.UserInfo(ctx, s); err == nil {
		t.Fatal("缺 API Token 却抓取成功")
	} else if errors.Is(err, site.ErrNotLoggedIn) || !strings.Contains(err.Error(), "token") {
		t.Fatalf("报错未指向 API Token（用户照着「未登录」只会去重填 cookie）：%v", err)
	}

	// 2. 填了 Token：抓用户信息，passkey 由站点自动下发并回写
	in.APIKey = "fxo_link_token"
	if _, err := siteOp.Update(s.ID, in); err != nil {
		t.Fatalf("Update: %v", err)
	}
	withToken, err := siteOp.Get(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ch.UserInfo(ctx, withToken); err != nil {
		t.Fatalf("UserInfo: %v", err)
	}
	fresh, err := siteOp.Get(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := siteOp.DecryptedPasskey(fresh); got != fluxLinkPasskey {
		t.Fatalf("passkey 未自动回写：got %q want %q", got, fluxLinkPasskey)
	}

	// 3. 下载链立刻可用：Search 给的 URL 不含 passkey 时由 chain 补全，且凭据包不得当 Cookie 发出
	content, _, err := ch.FetchTorrentFile(ctx, fresh,
		srv.URL+"/api/v1/compat/nexusphp/download.php?id=7")
	if err != nil {
		t.Fatalf("FetchTorrentFile: %v", err)
	}
	if string(content) != string(validTorrentFixture) {
		t.Errorf("种子内容不符：%q", string(content))
	}
	if len(downloadQueries) != 1 || !strings.Contains(downloadQueries[0], "passkey="+fluxLinkPasskey) {
		t.Errorf("下载请求未带自动发现的 passkey：%+v", downloadQueries)
	}
	for i, ck := range downloadCookies {
		if ck != "" {
			t.Errorf("第 %d 次下载带了 Cookie 头（FluxTorrent 兼容层不认 cookie）：%q", i+1, ck)
		}
	}
}
