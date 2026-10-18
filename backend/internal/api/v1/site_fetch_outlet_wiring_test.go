package v1

// #399 两个站点抓取出口自己抄了一份 `BadGateway.WithErr`，把两种「重试不会好」
// 的失败整段吞进日志：
//
//	site_search.go 的 TorrentDetail —— 种子详情抽屉（Search.vue 真实读者）
//	site_request.go 的 RequestHandler.All(site_id=…) —— 单站求种列表
//
// 两处的链层第一步都是 SiteChain.credentialWithErr（#377 的哨兵），所以：
//  1. 主密钥换过 ⇒ 用户看到「上游服务不可用，请稍后重试」，反复点、永远不会好，
//     而真正该做的是「到『站点』里重新保存凭据」；
//  2. 站点建在内网被 SSRF 守卫拦 ⇒ 同样只给通用话术，连 #366 已有的 SSRF 码都绕过了
//     （这两处压根没走 respondFetchError，等于把已修好的分档又漏掉了两处）。
//
// 修法：两处都归统一判据 respondFetchError，不再各自抄归类。
// 判据钉「出口必须走统一判据 + 凭据/SSRF 两档都到得了界面」，不钉源码措辞。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// TestSiteFetchOutlets_UseSharedClassifier 接线护栏：这两个出口必须走 respondFetchError。
//
// 反向断言是重点：它们各自的旧形状（裸 BadGateway.WithErr）曾让 #398 的凭据分档
// 与 #366 的 SSRF 分档同时漏在门外。钉「不许再有裸 WithErr」比钉「必须有某句代码」
// 更耐改——下次再加一条同类出口时，这条会提醒先想清楚该归哪一档。
func TestSiteFetchOutlets_UseSharedClassifier(t *testing.T) {
	for _, f := range []string{"site_search.go", "site_request.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s: %v", f, err)
		}
		code := stripV1Comments(string(src))
		if !strings.Contains(code, "respondFetchError(c, err, errcode.BadGateway)") {
			t.Errorf("%s：该走 respondFetchError（凭据解不开 / SSRF 拦内网都得有自己的档）", f)
		}
		if strings.Contains(code, "errcode.BadGateway.WithErr(err)") {
			t.Errorf("%s：又出现裸 BadGateway.WithErr —— 那会把重试不会好的原因丢进日志、只回通用话术", f)
		}
	}
}

// TestSiteFetchOutlets_TotalSharedClassifierUsers 统一判据的使用者总数钉在这里。
// #398 是 5 处（dashboard 1 + message 4），#399 补上这 2 处 ⇒ 7。
// 将来若有新的站点抓取出口，数字会变 —— 那时**要么改这里、要么确认它不该走这一档**，
// 不许悄悄绕过（#377 的教训：哨兵铺开了、出口没跟上）。
func TestSiteFetchOutlets_TotalSharedClassifierUsers(t *testing.T) {
	total := 0
	for _, f := range []string{"dashboard.go", "message.go", "site_search.go", "site_request.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s: %v", f, err)
		}
		total += strings.Count(stripV1Comments(string(src)), "respondFetchError(c,")
	}
	if total != 7 {
		t.Errorf("走 respondFetchError 的出口 = %d, want 7（dashboard 1 + message 4 + 详情/求种 2）", total)
	}
}

// TestSiteFetchOutlets_CredentialAndSSRFBothReachClient 行为测试：这两处经过统一判据后，
// 凭据解不开与 SSRF 拦内网两档都必须真的到得了界面（各有各的码）。
//
// 只钉接线不够 —— #336 的教训就是「源码里有那句中文」曾被测试当证据，而出口照样丢。
func TestSiteFetchOutlets_CredentialAndSSRFBothReachClient(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("凭据解不开 → 503 + site_credential_unreadable", func(t *testing.T) {
		r, id := credRotatedRouter(t, "old-master-key", "rotated-master-key")
		for _, p := range []string{
			fmt.Sprintf("/api/v1/sites/%d/torrent/detail?url=https://msg.test/details.php?id=1", id),
			fmt.Sprintf("/api/v1/site-requests?site_id=%d", id),
		} {
			status, env := doCredReq(t, r, http.MethodGet, p, "")
			if credEnvStr(env, "code") != errcode.SiteCredentialUnreadable.Code {
				t.Errorf("%s：code = %q（整包 %v），期望 %q",
					p, credEnvStr(env, "code"), env, errcode.SiteCredentialUnreadable.Code)
			}
			if status != http.StatusServiceUnavailable {
				t.Errorf("%s：状态码 = %d, want 503", p, status)
			}
			// 链层那句「本次未向站点发请求」必须抵达（这是 #398 存在的理由）。
			if !strings.Contains(credEnvStr(env, "message"), "未向站点发请求") {
				t.Errorf("%s：文案缺「未向站点发请求」：%q", p, credEnvStr(env, "message"))
			}
		}
	})

	t.Run("SSRF 拦内网 → ssrf_blocked（#366 在这两处曾经漏掉）", func(t *testing.T) {
		// 直接测统一判据的这两档分类（SSRF 档与凭据档同在 respondFetchError），
		// 目的是钉「这两处换用统一判据之后，SSRF 档也一并回来了」。
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		respondFetchError(c, fmt.Errorf("fetch %w", httpclient.ErrBlocked), errcode.BadGateway)
		var env map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("响应不是 JSON：%s", rec.Body.String())
		}
		if envStr(env, "code") != "ssrf_blocked" {
			t.Errorf("code = %v, want ssrf_blocked", env["code"])
		}
		if !strings.Contains(envStr(env, "message"), "ALLOW_PRIVATE_SITES") {
			t.Errorf("SSRF 档必须给出可操作的下一步（设 ALLOW_PRIVATE_SITES）：%v", env["message"])
		}
	})
}
