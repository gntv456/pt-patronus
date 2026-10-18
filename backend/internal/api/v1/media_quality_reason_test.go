package v1

// #394 媒体质量配置的写出口只说「服务内部错误，请稍后重试」。
//
// media.go SetQuality 的形状：
//
//	if err := h.mediaCh.SaveQuality(q); err != nil {
//	    errcode.Respond(c, errcode.Internal.WithErr(err))   // 500「服务内部错误，请稍后重试」
//	}
//
// SaveQuality（chain/media_rating.go）的失败只有两支：
//	json.Marshal(q)          结构体，实际不会失败；
//	c.cfg.Set(cfgKeyMediaQuality, raw)  ← 存储写不进去（磁盘满/连接断/锁）
//
// 前端 MediaQuality.vue 直接 `await mediaQualityApi.setConfig(...)` 并在 catch 里
// `ElMessage.error(e?.message)` ⇒ 后端那句「服务内部错误，请稍后重试」会**原样弹给用户**。
//
// 这是 #387（字幕自动化配置）/ #392（辅种配置）同一族的第三处，也是最纯粹的一处：
// 单次写、无「部分生效」问题，纯粹是**分类错了** —— 一次用户能判断的写失败被说成
// 无从下手的服务端故障，用户只会反复点保存，而重试一万次也不会好。
//
// 前置事实：读侧 9 处 Internal.WithErr 都是 GET 查询（DB 读失败 ⇒ Internal 准确），
// 只有 SetQuality 是写面。

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

// TestMediaQualityWriteFailureClassified 写失败必须落 save_failed 且带上真原因。
func TestMediaQualityWriteFailureClassified(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondMediaConfigWriteErr(c, errors.New("sql: database is locked"))

	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	if env["code"] != errcode.SaveFailed.Code {
		t.Errorf("写失败该落 save_failed（文案已点名 SECRET_KEY / 存储故障），got %v", env["code"])
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "database is locked") {
		t.Errorf("原因必须进响应体（前端 ElMessage.error 直接显示它，用户与运维都靠它定位），got %q", msg)
	}
	if strings.Contains(msg, "服务内部错误") {
		t.Errorf("写失败不该被压成通用内部错误，got %q", msg)
	}
}

// TestMediaQualityOutletUsesClassifier 接线护栏：SetQuality 必须走新判据。
func TestMediaQualityOutletUsesClassifier(t *testing.T) {
	src, err := os.ReadFile("media.go")
	if err != nil {
		t.Fatalf("读不到 media.go: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")

	at := strings.Index(code, "func (h *MediaHandler) SetQuality(")
	if at < 0 {
		t.Fatal("找不到 SetQuality（判据没有落点）")
	}
	body := code[at:]
	if n := strings.Index(body[1:], "\nfunc "); n > 0 {
		body = body[:1+n]
	}
	if strings.Contains(body, "errcode.Internal.WithErr(err)") {
		t.Error("SetQuality 还在用 Internal.WithErr（旧形状：用户只看到「服务内部错误，请稍后重试」，" +
			"而这是存储写不进去 —— 反复点保存不会好）")
	}
	if !strings.Contains(body, "respondMediaConfigWriteErr(c, err)") {
		t.Error("必须走 respondMediaConfigWriteErr（分类对了出口没接上是空转）")
	}
}

// TestMediaReadOutletsStayInternal 反向划界：读侧 9 处不该跟着一起改。
// 判据比修法宽会把正确的地方也改掉 —— 那正是「为了统一而统一」的坑。
func TestMediaReadOutletsStayInternal(t *testing.T) {
	src, err := os.ReadFile("media.go")
	if err != nil {
		t.Fatalf("读不到 media.go: %v", err)
	}
	code := string(src)
	// SetQuality 之外仍允许 Internal.WithErr（读侧）。只要它没被一刀切掉就行。
	if strings.Contains(code, "respondMediaConfigWriteErr(c, err)") {
		// 新判据只该出现在 SetQuality 里
		if n := strings.Count(code, "respondMediaConfigWriteErr(c, err)"); n != 1 {
			t.Errorf("respondMediaConfigWriteErr 出现了 %d 次 —— 读侧出口（DB 读失败）不该走它", n)
		}
	}
}
