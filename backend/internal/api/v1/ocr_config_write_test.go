package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// #313 OCR 配置写面两个形状：
// ①`_ = h.cfgOp.Set(...)` 三处丢弃写错误 —— OcrPanel.save() 只要拿到 200 就弹「已保存」，
//
//	于是「数据库没写进去」被演成「配置生效了」，而且回显还是旧值（用户学到：改了没用）。
//
// ②三个键一律「空串=不改」—— endpoint 是界面预填后整份回传的，也是这条链的开关
//
//	（GetConfig 用 Endpoint!="" 报 enabled）。用户在框里删空想停用 OCR，保存后配置照旧、
//	enabled 标签还在；想停用只能改成填一个别的地址。api_key 是脱敏字段，留空才该是「不改」。
type fakeCfg struct {
	vals map[string]string
	boom string // 非空 = 该键写失败
}

func (f *fakeCfg) Get(key string) (string, error) { return f.vals[key], nil }

func (f *fakeCfg) Set(key, val string) error {
	if f.boom != "" && strings.HasPrefix(key, f.boom) {
		return errors.New("database is locked")
	}
	f.vals[key] = val
	return nil
}

func newOCR(t *testing.T, body string, f *fakeCfg) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/system/ocr", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h := NewOCRHandler(f)
	h.SetConfig(c)
	var env map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
	}
	return rec, env
}

func cfgOp() *fakeCfg {
	return &fakeCfg{vals: map[string]string{
		"ocr.endpoint": "http://10.0.0.9:8888/ocr",
		"ocr.api_key":  "secret-old",
		"ocr.mode":     "paddle",
	}}
}

// ①写库失败必须不是 200：界面据此不再弹「已保存」。
func TestOCRSetConfigWriteFailureIsNotSuccess(t *testing.T) {
	for _, key := range []string{"ocr.endpoint", "ocr.api_key", "ocr.mode"} {
		f := cfgOp()
		f.boom = key
		body := `{"endpoint":"http://10.0.0.9:8888/ocr","api_key":"new-key","mode":"simple"}`
		rec, env := newOCR(t, body, f)
		if rec.Code == http.StatusOK {
			t.Errorf("%s 写失败却回 200（界面弹「已保存」，实际没落库）：env=%v", key, env)
			continue
		}
		if code, _ := env["code"].(string); code != errcode.SaveFailed.Code {
			t.Errorf("%s 写失败要归 save_failed（真·服务端故障），实际 %v", key, env)
		}
	}
	// 正控：写得动就照常回 200 + 脱敏回显。
	rec, env := newOCR(t, `{"endpoint":"http://10.0.0.9:8888/ocr","api_key":"new-key","mode":"simple"}`, cfgOp())
	if rec.Code != http.StatusOK {
		t.Fatalf("正常保存要 200，实际 %d %v", rec.Code, env)
	}
	if env["api_key"] == "new-key" || env["api_key_set"] != true {
		t.Errorf("api_key 不许原样回显，要脱敏+配置位：env=%v", env)
	}
	if env["mode"] != "simple" {
		t.Errorf("回显要反映刚存的值，实际 %v", env["mode"])
	}
}

// ②删空 endpoint = 停用 OCR：这一份必须真写下去，enabled 跟着翻成 false。
func TestOCRClearingEndpointDisables(t *testing.T) {
	f := cfgOp()
	rec, env := newOCR(t, `{"endpoint":"","api_key":"","mode":"paddle"}`, f)
	if rec.Code != http.StatusOK {
		t.Fatalf("停用要能保存，实际 %d %v", rec.Code, env)
	}
	if got := f.vals["ocr.endpoint"]; got != "" {
		t.Errorf("删空地址必须真写空（停用），实际库里还是 %q", got)
	}
	if env["enabled"] != false {
		t.Errorf("停用后回显 enabled 必须是 false，实际 %v（界面 enabled 标签就这样赖着不走）", env["enabled"])
	}
	// api_key 留空仍是「不改」——脱敏字段，用户没法回填原值，清空只能靠误伤。
	if f.vals["ocr.api_key"] != "secret-old" {
		t.Errorf("api_key 留空不该抹掉已存的密钥，实际 %q", f.vals["ocr.api_key"])
	}
}
