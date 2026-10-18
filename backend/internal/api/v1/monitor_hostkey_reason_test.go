package v1

// #341 AcceptHostKey 的目标查找失败出口用了通用码、且把原因整个丢掉。
//
// 冷门域典型（同 #5）：这个出口此前零测试覆盖，写的是
//
//	t, err := h.op.Get(id)
//	if err != nil {
//	    errcode.Respond(c, errcode.ResourceNotFound)   // 连 err 都没带
//	    return
//	}
//
// 两个问题叠在一起：
//  1. 用的是全仓通用的 ResourceNotFound（「资源不存在」），而同域已经有专属码
//     MonitorTargetNotFound（「运维目标不存在，可能已被删除，请在运维中心重新选择
//     目标主机」）—— 带下一步。同域有专属码却不用，等于把方向指丢了。
//  2. op.Get 底层是 db.First：可能是「记录不存在」（该刷新列表），也可能是真故障
//     （DB 不可用）。两者一律报 404「资源不存在」⇒ 数据库挂了也被说成「没这��」。
//     且 err 没挂 WithErr ⇒ 日志里也查不到。

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"gorm.io/gorm"
)

// TestMonitorAcceptHostKeyUsesDomainCode 同域专属码优先于通用码。
func TestMonitorAcceptHostKeyUsesDomainCode(t *testing.T) {
	code := stripCommentsForTest(mustReadMonitorSource(t))

	// 这个出口必须走本域专属码（文案带「运维中心重新选择」这步）。
	if !strings.Contains(code, "MonitorTargetNotFound") {
		t.Error("AcceptHostKey 必须用 MonitorTargetNotFound（同域已有专属码，通用 ResourceNotFound 指丢了方向）")
	}
	// 不许再用通用 ResourceNotFound。
	if strings.Contains(code, "errcode.ResourceNotFound") {
		t.Error("AcceptHostKey 不许再用通用 ResourceNotFound（旧形状：把 DB 故障也说成「资源不存在」）")
	}
	// 原因必须挂上（WithErr/WithDetail），否则日志里查不到。
	//
	// #373 这条判据的形状更新过一次：从前要求字面量 `MonitorTargetNotFound.With`，
	// 而现在出口走的是 respondRecordError(…, errcode.MonitorTargetNotFound, …) ——
	// 分类器内部正是 notFound.WithErr(err)（input_error.go），语义没变，只是写法从
	// 手抄换成了统一判据。两种形状都算通过。
	if !strings.Contains(code, "MonitorTargetNotFound.With") &&
		!strings.Contains(code, "respondRecordError(c, err, errcode.MonitorTargetNotFound") {
		t.Error("失败出口必须用本域码且挂上底层原因（WithErr/WithDetail，或走 respondRecordError）")
	}
}

// TestMonitorHostKeyCodesDifferentPaths 反向对照：专属码与通用码不许文案相同，
// 否则「换了个码但还是那句话」，等于没修。
func TestMonitorHostKeyCodesDifferentPaths(t *testing.T) {
	if errcode.MonitorTargetNotFound.Message == errcode.ResourceNotFound.Message {
		t.Errorf("两个码文案相同（%q），换码等于没换", errcode.MonitorTargetNotFound.Message)
	}
	if !strings.Contains(errcode.MonitorTargetNotFound.Message, "运维中心") {
		t.Errorf("专属码必须给可走的下一步，got %q", errcode.MonitorTargetNotFound.Message)
	}
}

// TestMonitorMissingTargetStillReturns404 目标真不存在时仍应是 404 +
// monitor_target_not_found —— 别把「让原因进响应体」改成 200 或换码。
func TestMonitorMissingTargetStillReturns404(t *testing.T) {
	// setupMonitorServer 没挂 accept-host-key（它要 admin 闸门），这里只挂目标查找那一段。
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/mon-hk.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.MonitorTarget{}, &models.MonitorAlert{}, &models.MetricSnapshot{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewMonitorOper(db, security.NewCrypto("mon-hk-seed"))
	svc := monitor.NewService(op, monitor.NewRegistry(), nil, 0, nil)
	h := NewMonitorHandler(op, svc, oper.NewSSHHostKeyOper(db))
	rg := gin.New()
	rg.POST("/monitor/targets/:id/accept-host-key", h.AcceptHostKey)

	rec := doMon(t, rg, http.MethodPost, "/monitor/targets/9999/accept-host-key", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("目标不存在应 404，got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "monitor_target_not_found") {
		t.Errorf("应回本域专属码 got %s", rec.Body.String())
	}
}

func mustReadMonitorSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("monitor.go")
	if err != nil {
		t.Fatalf("读不到 monitor.go：%v", err)
	}
	return string(b)
}
