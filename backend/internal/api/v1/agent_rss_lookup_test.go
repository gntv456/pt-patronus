package v1

// #347 被控端与 RSS 订阅的「按 id 取」失败出口各自手抄（#345 的同族，同一处病）。
//
// 又是两版并存：
//
//	agent.go:118  Enable/…  → respondRecordError(…)              统一
//	agent.go:96   Delete     → Respond(AgentNotFound)              手抄
//	rss.go:133     Delete     → respondRecordError(…)              统一
//	rss.go:108     Update     → Respond(RssSubscriptionGone)       手抄
//	rss.go:147     Check      → Respond(RssSubscriptionGone)       手抄
//
// 手抄的几处只判「记录不存在」，不判 DB 真故障：AgentOper.Get /
// RssChain.Get 底层都是 db.First，磁盘满/连接断时也被说成「被控端不存在」/
// 「RSS 订阅不存在」，且 err 没挂 WithErr ⇒ 日志里也查不到。
//
// 另一端 backup.go 的两处（ImportPTD 的 FormFile/fh.Open 失败）同形但**不是洞**：
// 那两处的 err 来自「请求里没有文件」/「文件句柄打不开」，码（备份文件无效 /
// 读取文件失败）本身就是完整语义，不需要带原因。判据要分得开这两种。

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// TestAgentRssLookupShareClassifier 三处手抄出口必须走统一判据。
func TestAgentRssLookupShareClassifier(t *testing.T) {
	for _, c := range []struct{ file, code string }{
		{"agent.go", "AgentNotFound"},
		{"rss.go", "RssSubscriptionGone"},
	} {
		src, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatalf("读不到 %s：%v", c.file, err)
		}
		code := stripCommentsForTest(string(src))
		if n := strings.Count(code, "Respond(c, errcode."+c.code+")"); n != 0 {
			t.Errorf("%s 还有 %d 处裸 Respond(%s)：只判「不存在」，DB 真故障会被说成记录没了", c.file, n, c.code)
		}
	}
}

// TestBackupFileErrorsStayAsIs 反向对照：入参/IO 类失败的码本身就是语义，
// 不许被这次统一改造成「查记录」那一类。
func TestBackupFileErrorsStayAsIs(t *testing.T) {
	src, err := os.ReadFile("backup.go")
	if err != nil {
		t.Fatalf("读不到 backup.go：%v", err)
	}
	code := stripCommentsForTest(string(src))
	// 这两处的 err 是「没带文件 / 句柄打不开」，码本身说清了，不带原因也成立。
	for _, want := range []string{"errcode.BackupFileInvalid)", "errcode.BackupReadFailed)"} {
		if !strings.Contains(code, want) {
			t.Errorf("backup.go 缺 %s：入参/IO 失败的码本身就是语义，不该被改成别的", want)
		}
	}
}

// TestAgentMissingStill404 反向对照：真·不存在仍 404 + 本域码。
func TestAgentMissingStill404(t *testing.T) {
	r := setupAgentLookupRouter(t)
	rec := doSub(t, r, http.MethodDelete, "/agents/999999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("被控端不存在应 404，got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent_not_found") {
		t.Errorf("应回本域码 got %s", rec.Body.String())
	}
}

// TestRssMissingStill404 同上，RSS 域。
func TestRssMissingStill404(t *testing.T) {
	r := setupRssLookupRouter(t)
	rec := doRss(t, r, http.MethodPut, "/rss/999999", `{"title":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("RSS 订阅不存在应 404，got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "rss_subscription_not_found") {
		t.Errorf("应回本域码 got %s", rec.Body.String())
	}
}

// setupAgentLookupRouter 只挂 Agent 的 Delete（要 admin 闸门，公共 harness 没挂）。
func setupAgentLookupRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/agent-lookup.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.MonitorTarget{}, &models.Agent{}); err != nil {
		t.Fatal(err)
	}
	h := NewAgentHandler(
		oper.NewAgentOper(db),
		oper.NewMonitorOper(db, security.NewCrypto("agent-lookup-seed")),
		db, nil,
	)
	r := gin.New()
	r.DELETE("/agents/:id", h.Delete)
	return r
}

// setupRssLookupRouter 用 rss_test.go 的公共 harness（它已挂 Update/Delete/Check）。
func setupRssLookupRouter(t *testing.T) *gin.Engine {
	t.Helper()
	r, _ := setupRssServer(t)
	return r
}
