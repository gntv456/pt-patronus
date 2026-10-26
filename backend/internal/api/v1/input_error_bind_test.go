package v1

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"net/http/httptest"
)

// 写端点的绑定失败必须点名到字段（#69）。
// 真机跑过一次的三条（缺 platform / body 不是 JSON / 必填齐→200）留在这里当常驻用例，
// 免得这层翻译哪天又被折回「没能指出具体是哪一项」。
type bindProbeInput struct {
	Name     string `json:"name" binding:"required,max=64"`
	BaseURL  string `json:"base_url" binding:"required"`
	Platform string `json:"platform" binding:"required"`
}

func bindErrOf(t *testing.T, body string) error {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/v1/sites", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	var in bindProbeInput
	err := c.ShouldBindJSON(&in)
	if err == nil {
		t.Fatalf("这具请求体本该绑失败，却没报错")
	}
	return err
}

func TestBindFieldReasonNamesTheField(t *testing.T) {
	// 缺一个必填项 → 必须说出是哪一个
	r := bindFieldReason(bindErrOf(t, `{"name":"probe","base_url":"http://x"}`))
	if !strings.Contains(r, "platform") || !strings.Contains(r, "缺少必填项") {
		t.Errorf("没点名到缺失字段，实得 %q", r)
	}
	// 两个都缺 → 两个都要说，不许只报第一个
	r2 := bindFieldReason(bindErrOf(t, `{"name":"probe"}`))
	if !strings.Contains(r2, "base_url") || !strings.Contains(r2, "platform") {
		t.Errorf("多个缺项应逐条列出，实得 %q", r2)
	}
	// 超长（max）也要有方向，不能与「缺项」共用一句
	r3 := bindFieldReason(bindErrOf(t, `{"name":"`+strings.Repeat("n", 80)+`","base_url":"http://x","platform":"p"}`))
	if !strings.Contains(r3, "超出长度上限") {
		t.Errorf("max 校验应说长度上限，实得 %q", r3)
	}
	// 垃圾 JSON 与「字段不合规则」分得开
	if got := bindFieldReason(bindErrOf(t, `{oops`)); got != "请求体不是合法 JSON" {
		t.Errorf("JSON 语法错应单列一句，实得 %q", got)
	}
	// 正控：认不出来的错必须退回空串（调用方据此走通用文案 + 日志留原文），
	// 不许瞎编一句字段名骗界面。
	if got := bindFieldReason(errUnknownBind{}); got != "" {
		t.Errorf("不认识的错误应返回空串，实得 %q", got)
	}
}

// 嵌套结构体字段（本仓配置类请求大量是嵌套的）也要点名到 JSON 路径：
// 实测「PUT /notification/config 只传 webhook」被 email.port 的 min 拦下，
// 旧正则只认一层点，界面于是只说「没能指出具体是哪一项」。
type bindProbeNested struct {
	Email struct {
		Enabled bool `json:"enabled"`
		Port    int  `json:"port" binding:"min=1,max=65535"`
	} `json:"email"`
	Webhook struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url" binding:"max=512"`
	} `json:"webhook"`
}

func TestBindFieldReasonNamesNestedField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/v1/notification/config",
		strings.NewReader(`{"webhook":{"enabled":true,"url":"http://x"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	var in bindProbeNested
	err := c.ShouldBindJSON(&in)
	if err == nil {
		t.Fatal("这具请求体本该被 email.port 的 min 拦下")
	}
	r := bindFieldReason(err)
	if !strings.Contains(r, "email.port") || !strings.Contains(r, "低于最小长度") {
		t.Fatalf("嵌套字段要点名到 JSON 路径，实得 %q", r)
	}
}

type errUnknownBind struct{}

func (errUnknownBind) Error() string { return "some other binder complaint" }

// 字段名必须折成客户端真写的 JSON 名：validator 的文本只给结构体字段，
// 直接小写会编出一个不存在的名字（baseurl），用户照着改永远改不对。
func TestJSONFieldNameMatchesJSONTags(t *testing.T) {
	for field, want := range map[string]string{
		"BaseURL":  "base_url",
		"Platform": "platform",
		"ID":       "id",
		"HxptUID":  "hxpt_uid",
		"NoLogin":  "no_login",
		"VIPLevel": "vip_level",
		"Name":     "name",
		"LimitV":   "limit_v",
	} {
		if got := jsonFieldName(field); got != want {
			t.Errorf("%s 应折成 %s，实得 %s", field, want, got)
		}
	}
	// 正控：朴素小写确实是错的（证明这条测试在防的东西真实存在）
	if strings.ToLower("BaseURL") == "base_url" {
		t.Error("小写对照本身失效（这条正控失去意义）")
	}
}
