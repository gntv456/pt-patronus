package v1

// #398 行为测试（端到端）：主密钥换过 ⇒ 站点凭据解不开 ⇒ 五个抓取出口必须
// 各自给出专属码，而不是通用话术。
//
// 与同文件的 respondFetchError 分类器测试分工：
//   本文件证「真实链层错误确实走到那个出口」（真 SiteOper + 真链，只换主密钥）；
//   另一个文件证「那个出口怎么分类」。两层缺一不可——
//   #377 正是链层哨兵铺开了、出口却没读者（分类器修了、接线没验）；
//   #399 又在详情/求种两处发现同一族的漏网（连已修好的 SSRF 分档也一并绕过）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// credRotatedRouter 用 credKey 写站点凭据、用 readKey 建链（两者不同即「换过主密钥」），
// 并把 dashboard.SyncOne 与 message 的四个出口挂上（路径照生产抄）。
func credRotatedRouter(t *testing.T, credKey, readKey string) (*gin.Engine, uint) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cred-rot.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Site{}, &models.Downloader{}, &models.SiteStat{}); err != nil {
		t.Fatal(err)
	}
	if readKey == "" {
		readKey = credKey
	}
	// 写入用 credKey；读用 readKey（不同 ⇒ 解不开，正是 #398 的触发条件）。
	s, err := oper.NewSiteOper(db, security.NewCrypto(credKey)).Create(oper.SiteInput{
		Name: "幻月府", Platform: "nexusphp", BaseURL: "https://msg.test", Cookie: "uid=7; pass=secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(db, security.NewCrypto(readKey))
	dlOp := oper.NewDownloaderOper(db, security.NewCrypto(readKey))
	siteCh := chain.NewSiteChain(chain.NewBase(module.New(), event.New()), siteOp, nil)

	dashH := NewDashboardHandler(chain.NewDashboardChain(db, siteOp, dlOp, siteCh))
	msgH := NewMessageHandler(siteCh, siteOp)

	reqH := NewSiteRequestHandler(siteCh)
	siteH := NewSiteHandler(siteOp, siteCh, nil, nil)
	r := gin.New()
	r.POST("/api/v1/dashboard/sync/:id", dashH.SyncOne)
	// #399 这两处曾自己抄 `BadGateway.WithErr`，绕过了凭据/SSRF 分档。
	r.GET("/api/v1/sites/:id/torrent/detail", siteH.TorrentDetail)
	r.GET("/api/v1/site-requests", reqH.All)
	r.GET("/api/v1/messages/:id/content", msgH.Content)
	r.POST("/api/v1/messages/:id/reply", msgH.Reply)
	r.POST("/api/v1/messages/:id/delete", msgH.Delete)
	r.POST("/api/v1/messages/:id/mark-read", msgH.MarkRead)
	return r, s.ID
}

func doCredReq(t *testing.T, r *gin.Engine, method, path, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(rec, req)
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("%s %s → %d，响应不是 JSON：%q", method, path, rec.Code, rec.Body.String())
	}
	return rec.Code, env
}

func credEnvStr(env map[string]any, k string) string {
	v, _ := env[k].(string)
	return v
}

// TestCredRotated_AllSevenOutletsReportCredentialUnreadable 主密钥换过后，
// 七个站点抓取出口都要说「凭据读不出来、去站点里重存、本次没发请求」。
func TestCredRotated_AllSevenOutletsReportCredentialUnreadable(t *testing.T) {
	cases := []struct{ name, method, path, body string }{
		{"仪表盘单站刷新（SyncOne）", http.MethodPost, "/api/v1/dashboard/sync/%d", ""},
		{"消息正文", http.MethodGet, "/api/v1/messages/%d/content?url=https://msg.test/details.php?id=1", ""},
		{"回复消息", http.MethodPost, "/api/v1/messages/%d/reply", `{"url":"https://msg.test/reply.php?id=1","content":"x"}`},
		{"删除消息", http.MethodPost, "/api/v1/messages/%d/delete", `{"url":"https://msg.test/del.php?id=1"}`},
		{"标记已读", http.MethodPost, "/api/v1/messages/%d/mark-read", `{"ids":["1"]}`},
		// #399：这两处曾自己抄 BadGateway.WithErr，绕过凭据/SSRF 分档。
		{"种子详情（TorrentDetail）", http.MethodGet, "/api/v1/sites/%d/torrent/detail?url=https://msg.test/details.php?id=1", ""},
		{"单站求种（RequestHandler.All）", http.MethodGet, "/api/v1/site-requests?site_id=%d", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// credKey != readKey ⇒ 凭据解不开。
			r, id := credRotatedRouter(t, "old-master-key", "rotated-master-key")
			path := fmt.Sprintf(tc.path, id)
			status, env := doCredReq(t, r, tc.method, path, tc.body)

			msg := credEnvStr(env, "message")
			if status == http.StatusOK && credEnvStr(env, "ok") == "false" {
				t.Fatalf("这一档必须走硬失败面（带 code），却回软失败 %v", env)
			}
			if credEnvStr(env, "code") != errcode.SiteCredentialUnreadable.Code {
				t.Fatalf("code = %q（整包 %v），期望 %q",
					credEnvStr(env, "code"), env, errcode.SiteCredentialUnreadable.Code)
			}
			if status != http.StatusServiceUnavailable {
				t.Errorf("状态码 = %d, want 503（重试不会好，不是「上游不可用」）", status)
			}
			// 链层那句可操作的中文必须抵达（点名站点、说下一步、说没发请求）。
			for _, want := range []string{"幻月府", "重新保存", "未向站点发请求"} {
				if !strings.Contains(msg, want) {
					t.Errorf("界面文案缺 %q：%q", want, msg)
				}
			}
			// 通用话术不许盖在上面。
			for _, bad := range []string{"上游服务不可用", "服务内部错误", "请稍后重试"} {
				if strings.Contains(msg, bad) {
					t.Errorf("不该把 %q 盖在凭据原因上（会让人一直重试）：%q", bad, msg)
				}
			}
		})
	}
}

// TestCredRotated_SameKeyStillReachesUpstream 反向对照：主密钥没换时，这七个出口
// 不该被误判成凭据问题（否则会把真·上游故障说成「去重存凭据」）。
func TestCredRotated_SameKeyStillReachesUpstream(t *testing.T) {
	// 同一个 key：凭据解得开 ⇒ 失败原因在别处（站点建在不可达地址 msg.test）。
	// 这一条不钉具体原因，只钉「不是凭据这一档」。
	r, id := credRotatedRouter(t, "same-key", "same-key")
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, fmt.Sprintf("/api/v1/dashboard/sync/%d", id), ""},
		{http.MethodPost, fmt.Sprintf("/api/v1/messages/%d/mark-read", id), `{"ids":["1"]}`},
	} {
		_, env := doCredReq(t, r, tc.method, tc.path, tc.body)
		if credEnvStr(env, "code") == errcode.SiteCredentialUnreadable.Code {
			t.Errorf("%s %s：主密钥没换却报凭据读不出来 ⇒ 误判（整包 %v）", tc.method, tc.path, env)
		}
	}
}
