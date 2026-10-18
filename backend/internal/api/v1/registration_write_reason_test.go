package v1

// #396 注册检测的三个写出口只说「服务内部错误，请稍后重试」，而它们的语义各不相同。
//
// registration.go 的形状：
//
//	Delete:      requireExists(…) 之后
//	             h.op.Delete(id)          失败 → Internal.WithErr
//	Check（无法判定）：
//	             h.op.TouchChecked(id)     失败 → Internal.WithErr
//	Check（已判定）：
//	             h.op.UpdateStatus(id,open) 失败 → Internal.WithErr
//
// 三者的下一步完全不同，不能共用一句「服务内部错误」：
//
//	Delete        纯 DB 写。失败＝什么都没删（幂等，重试安全）。
//	TouchChecked  只刷新「最近查过」时间戳、**刻意不动 last_open**。
//	               失败＝检测已经跑完了、时间戳没记上；站点的开/关结论**不受影响**。
//	UpdateStatus  回写「是否开放」这个**业务结论**。
//	               失败＝这一次检测白跑了：链层下一轮还会读到旧值，
//	               而用户看到的是「服务内部错误」，不知道结论到底算不算数。
//
// UpdateStatus 那条最值得说：oper 层它返回 (*bool, error)，第一个值是**上一个状态**
// （chain/registration.go:138 正是用它算「本轮刚开放的站」好发通知）。手动触发这条路径
// 用 `_` 丢掉了，于是用户点一次「检测」，即便状态翻转了也不会被告知「有变化」。
// 那属于功能增强，不是本条的缺陷；但「写不进去」这件事必须说清楚。
//
// 为什么现在值得查：Check 这条路径**零测试覆盖**（delete_missing_more_test.go 只测 Delete），
// 而注册检测是定时任务 + API（前端无独立页面）⇒ 失败时没有任何界面提示，
// 只有 slog。运维看到的是「状态不更新」，查不出是磁盘满还是连接断。
//
// 与 #387/#392/#394/#395 同族（写面分类错），这一处的读者不是界面用户而是 cron，
// 所以文案要落在「查得到日志里的原因」上，而不是「用户该点哪里」。

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// TestRegistrationCheckWriteFailureClassified 检测结论写不进去必须归 save_failed。
func TestRegistrationCheckWriteFailureClassified(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondRegistrationWriteErr(c, errors.New("sql: database is locked"))

	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	// 用本域专属码而不是通用 save_failed：这条路径的读者是 cron/运维，
	// 前端若将来加页面也要能按码分开处理。（第一版这里写死 save_failed，
	//  而实现选了专属码 —— 判据与实现不一致时，先问「哪个更好」再改，
	//  不要为了让自己绿而把实现降级。）
	if env["code"] != errcode.RegistrationWriteFailed.Code {
		t.Errorf("写面失败该落 registration_write_failed（旧形状：internal ⇒ "+
			"这条路径零测试覆盖，没人发现），got %v", env["code"])
	}
	if env["code"] == errcode.Internal.Code {
		t.Error("不该报 internal：读者是 cron/运维，必须能按码定位")
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "database is locked") {
		t.Errorf("这条路径的读者是 cron/运维，原因必须可查，got %q", msg)
	}
}

// TestRegistrationCheckOutletsUseClassifier 接线护栏：三个写出口都要走新判据。
func TestRegistrationCheckOutletsUseClassifier(t *testing.T) {
	src, err := os.ReadFile("registration.go")
	if err != nil {
		t.Fatalf("读不到 registration.go: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")

	// 三个写调用点，每个都必须紧跟新判据。
	for _, call := range []string{"h.op.Delete(id)", "h.op.TouchChecked(id)", "h.op.UpdateStatus(id, open)"} {
		at := strings.Index(code, call)
		if at < 0 {
			t.Errorf("找不到 %s（判据没有落点）", call)
			continue
		}
		after := code[at:]
		if len(after) > 260 {
			after = after[:260]
		}
		if strings.Contains(after, "errcode.Internal.WithErr(err)") {
			t.Errorf("%s 之后还在用 Internal.WithErr（旧形状）", call)
		}
		if !strings.Contains(after, "respondRegistrationWriteErr(") {
			t.Errorf("%s 必须走 respondRegistrationWriteErr（分类对了出口没接上是空转）", call)
		}
	}
}
