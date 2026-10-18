package v1

// #374 保存字幕源配置失败只说「服务内部错误」，用户既不知道原因、
// 也不知道五个字段里哪几个已经存进去了。
//
// SetConfig → chain.SetConfig → provider.ApplySettings（chain/subtitle.go:82）。
// 接口层从前是：
//
//	if err := h.svc.SetConfig(...); err != nil {
//		errcode.Respond(c, errcode.Internal.WithErr(err))   // 500「服务内部错误，请稍后重试」
//	}
//
// 用户刚填完一屏 token/cookie 点保存，看到这句既不知道出了什么事，也不知道
// 该重试还是该找管理员。改成 SaveFailed + 带上真实原因（与 #371 Webhook 写出口同口径）。
//
// 链层那侧「失败后停在半更新」由 chain/subtitle_settings_allornothing_test.go 钉。

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

func TestSubtitleSettingsWriteFailureClassified(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondSubtitleSettingsErr(c, errors.New("sql: database is closed"))
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	if env["code"] != errcode.SaveFailed.Code {
		t.Errorf("写失败该落 save_failed，got %v（旧形状：internal「服务内部错误，请稍后重试」）", env["code"])
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "database is closed") {
		t.Errorf("响应体该带出真实原因供排查，got %q", msg)
	}
}

// TestSubtitleSettingsOutletUsesClassifier 接线护栏：SetConfig 出口必须走分类器。
func TestSubtitleSettingsOutletUsesClassifier(t *testing.T) {
	src, err := os.ReadFile("subtitle.go")
	if err != nil {
		t.Fatalf("读不到 subtitle.go：%v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")
	// #387 原来只查 SetConfig；SetAutomationConfig 是同一类的设置写入出口，被落下了。
	// 它的失败同样是 kv.Set 写不进去（磁盘满/连接断/锁），用户却只看到「服务内部错误」。
	for _, fn := range []string{"SetConfig", "SetAutomationConfig"} {
		at := strings.Index(code, "func (h *SubtitleHandler) "+fn+"(")
		if at < 0 {
			t.Errorf("找不到 %s（判据没有落点）", fn)
			continue
		}
		tail := code[at:]
		if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
			tail = tail[:1+n]
		}
		if strings.Contains(tail, "errcode.Internal.WithErr(err)") {
			t.Errorf("%s 不许把写失败说成「服务内部错误，请稍后重试」（旧形状）", fn)
		}
		if !strings.Contains(tail, "respondSubtitleSettingsErr(c, err)") {
			t.Errorf("%s 必须走 respondSubtitleSettingsErr（分类对了出口没接上是空转）", fn)
		}
	}
}
