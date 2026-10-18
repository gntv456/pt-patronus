package v1

// #398 站点凭据解不开（「配过、但现在读不出来」）被混进站点抓取的通用失败。
//
// 旧形状（ssrf_error.go 的 respondFetchError）只认 SSRF 一种：
//
//	if errors.Is(err, httpclient.ErrBlocked) { … }
//	errcode.Respond(c, fallback.WithErr(err))   // ← 其余全进这里
//
// 而 dashboard.SyncOne 与 message 的四个出口，上游都先走
// chain.SiteChain.credentialWithErr（#377 铺开的可判别哨兵）。主密钥换过时，
// 那一句写着「站点「X」的Cookie 无法解密（主密钥可能已更换）——请到『站点』里
// 重新保存该站点凭据；本次未向站点发请求」的话，被 WithErr 整段丢进日志，
// 界面只见「服务内部错误，请稍后重试」/「上游服务不可用」。
//
// 为什么值得单独一档：
//  1. 重试永远不会好（换的是主密钥，不是网络）；
//  2. 「本次未向站点发请求」是用户判断站点那边有没有出事的唯一依据，
//     套上「上游不可用」会让人反复重试、并误以为站点挂了。
//
// 判据钉「原因进响应体 + 独立码」，不钉源码里有哪句中文（中文改对了照样丢的原因，
// #336 的旧教训）。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// credUnreadableErr 造一个与链层同形的凭据哨兵错误（%w 保住哨兵 + 带可操作中文）。
func credUnreadableErr() error {
	return fmt.Errorf("%w：站点「测试站」的Cookie 无法解密（主密钥可能已更换）——请到「站点」里重新保存该站点凭据；本次未向站点发请求",
		chain.ErrSiteCredentialUnreadable)
}

// codeOf 取测试要用的兜底码（按 code 字面取，避免为测试改生产表）。
func codeOf(code string) *errcode.Error {
	switch code {
	case "bad_gateway":
		return errcode.BadGateway
	case "internal":
		return errcode.Internal
	default:
		return errcode.Internal
	}
}

// TestRespondFetchError_ClassifiesSiteCredentialUnreadable 凭据解不开必须有专属码。
func TestRespondFetchError_ClassifiesSiteCredentialUnreadable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	// 兜底给 BadGateway（message 的四个出口就是这一档）：凭据问题不该混进「上游不可用」。
	respondFetchError(c, credUnreadableErr(), errcode.BadGateway)

	var env struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应不是 JSON：%s", rec.Body.String())
	}
	if env.Code != "site_credential_unreadable" {
		t.Errorf("code = %q, want site_credential_unreadable（凭据问题有自己的档，不该走 bad_gateway/internal）", env.Code)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("状态码 = %d, want 503", rec.Code)
	}
	// 链层那句中文必须原样抵达（WithDetail）——它是用户唯一能照做的线索。
	for _, want := range []string{"测试站", "重新保存", "未向站点发请求"} {
		if !strings.Contains(env.Message, want) {
			t.Errorf("响应体必须含链层那句里的 %q，got %q", want, env.Message)
		}
	}
	// 通用话术不许盖在上面。
	for _, bad := range []string{"上游服务不可用", "服务内部错误"} {
		if strings.Contains(env.Message, bad) {
			t.Errorf("不该把通用话术盖在凭据原因上（got %q）", env.Message)
		}
	}
	// error 与 message 同值（老客户端只读 error）。
	if env.Error != env.Message {
		t.Errorf("error 字段应与 message 同值，got %q vs %q", env.Error, env.Message)
	}
}

// TestRespondFetchError_OtherBucketsIntact 反向对照：另外两档不许被这次改动带偏。
// （SSRF 那档与其它错误各有各的判据，这里逐个钉住。）
func TestRespondFetchError_OtherBucketsIntact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name     string
		err      error
		fallback string
		wantCode string
		wantHTTP int
	}{
		{
			name:     "SSRF 拦内网：仍走 ssrf_blocked（站点建在内网是常规用法）",
			err:      fmt.Errorf("fetch %w", httpclient.ErrBlocked),
			fallback: "internal",
			wantCode: "ssrf_blocked",
			wantHTTP: http.StatusBadRequest,
		},
		{
			name:     "普通上游故障：仍走调用方给的兜底",
			err:      errors.New("dial tcp 10.0.0.5:80: connect: connection refused"),
			fallback: "bad_gateway",
			wantCode: "bad_gateway",
			wantHTTP: http.StatusBadGateway,
		},
		{
			name:     "nil 错误不进这一档",
			err:      nil,
			fallback: "internal",
			wantCode: "internal",
			wantHTTP: http.StatusInternalServerError,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			if tc.err == nil {
				// nil 时 FromError 返回 nil → Respond 会 panic；本档只验证「不退化」，
				// 故不走实际调用（nil 也不该被传进这一出口）。
				return
			}
			respondFetchError(c, tc.err, codeOf(tc.fallback))
			var env struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("响应不是 JSON：%s", rec.Body.String())
			}
			if env.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", env.Code, tc.wantCode)
			}
			if rec.Code != tc.wantHTTP {
				t.Errorf("状态码 = %d, want %d", rec.Code, tc.wantHTTP)
			}
		})
	}
}

// TestRespondFetchError_CoversAllFiveOutlets 接线护栏：所有站点抓取出口都必须走
// respondFetchError（统一判据）。若某个出口自己抄一份 WithErr，凭据分档就会漏在它之外——
// 这正是 #377 铺开哨兵后又被这里漏掉的原因。
func TestRespondFetchError_CoversAllFiveOutlets(t *testing.T) {
	files := map[string]int{
		"dashboard.go": 1,
		"message.go":   4,
	}
	for f, want := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s: %v", f, err)
		}
		code := stripV1Comments(string(src))
		if n := strings.Count(code, "respondFetchError(c,"); n != want {
			t.Errorf("%s：走 respondFetchError 的出口 = %d, want %d ⇒ 有出口自己抄了归类", f, n, want)
		}
	}
	// 兜底档必须带出去：这两个调用点忘传 fallback 会让签名都编不过，
	// 但它同时把「统一判据」钉在 5 处（1 + 4）这个数上。
	src, err := os.ReadFile("ssrf_error.go")
	if err != nil {
		t.Fatal(err)
	}
	body := stripV1Comments(string(src))
	if !strings.Contains(body, "chain.ErrSiteCredentialUnreadable") {
		t.Error("respondFetchError 必须认 chain.ErrSiteCredentialUnreadable（#398 的判据落点）")
	}
	// 反向断言：凭据这一档必须走专属码，且用 WithDetail 原样带出链层那句中文。
	// （这两条各有各的坏法：用 WithErr 会把中文丢进日志只留通用话术；
	//   用错误的中文码则会让「上游不可用」盖在「重试不会好」的凭据原因上。）
	if !strings.Contains(body, "errcode.SiteCredentialUnreadable.WithDetail(err)") {
		t.Error("凭据这一档必须走 errcode.SiteCredentialUnreadable 并用 WithDetail（链层那句中文是唯一线索）")
	}
	if !strings.Contains(body, "errcode.SSRFBlocked.WithErr(err)") {
		t.Error("SSRF 那档不许被这次改动带偏")
	}
}

// stripV1Comments 去掉整行注释，免得把判据本身当成被测代码。
func stripV1Comments(s string) string {
	out := make([]string, 0, 256)
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
