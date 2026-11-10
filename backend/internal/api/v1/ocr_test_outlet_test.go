// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// #314 OCR「测试」按钮把 Go 的英文原文直接贴到界面。
// 真路由取证拿到的实际响应（backend 起在同机、端点填 127.0.0.1:9）：
//
//	502 {"error":"Post \"http://127.0.0.1:9/ocr\": dial tcp 127.0.0.1:9: connectex:
//	     No connection could be made because the target machine actively refused it.","ok":false}
//
// 而 OcrPanel.test() 是 `ElMessage.error(r.error || t('ocrp.testFailed'))` ⇒ 这串英文就是用户看到的。
// 同一族的其它出口（SSO/Docker）早就在 errcode 里写了「连不上 X：该核对什么」的人话，OCR 这两个
// 归类缺失：连不上（地址/端口/服务没起）与 答了但读不懂（多为 mode 与服务端形态不匹配）是两条不同的下一步。

type ocrStub struct {
	vals map[string]string
}

func (o *ocrStub) Get(k string) (string, error) { return o.vals[k], nil }
func (o *ocrStub) Set(k, v string) error        { o.vals[k] = v; return nil }

func doTest(t *testing.T, endpoint string) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/system/ocr/test", nil)
	h := NewOCRHandler(&ocrStub{vals: map[string]string{"ocr.endpoint": endpoint, "ocr.mode": "paddle"}})
	h.Test(c)
	var env map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
	}
	return rec.Code, env
}

// ①连不上：界面文案必须是中文人话，且不许把 Go/os 原文透出去。
func TestOCRTestUnreachableIsChineseNotRawGoError(t *testing.T) {
	code, env := doTest(t, "http://127.0.0.1:9/ocr") // discard 端口，秒拒
	if code != http.StatusBadGateway {
		t.Fatalf("连不上要归 502，实际 %d %v", code, env)
	}
	msg, _ := env["error"].(string)
	if !strings.Contains(msg, "连不上") {
		t.Errorf("界面文案要中文人话（和 SSO/Docker 同风格），实际 %q", msg)
	}
	for _, raw := range []string{"dial tcp", "connectex", "Post \""} {
		if strings.Contains(msg, raw) {
			t.Errorf("Go/os 原文不许进界面（%q 出现在文案里）：%q", raw, msg)
		}
	}
	if c, _ := env["code"].(string); c != "ocr_service_unreachable" {
		t.Errorf("要有本域码便于日志与前端分支，实际 %v", env)
	}
}

// ②答了但读不懂/状态异常：另一条下一步（核对 mode），也不许是英文原文。
func TestOCRTestBadAnswerIsChinese(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"msg":"boom"}`))
	}))
	defer srv.Close()
	code, env := doTest(t, srv.URL)
	if code != http.StatusBadGateway {
		t.Fatalf("服务答 500 要归 502，实际 %d %v", code, env)
	}
	msg, _ := env["error"].(string)
	if !strings.Contains(msg, "响应") || strings.Contains(msg, "ocr http") {
		t.Errorf("要中文人话且不把内部技术串透出去，实际 %q", msg)
	}
}

// 正控：真识别出文本、以及「能连通但什么都没认出来」都算可用（#312 定的两种语义）。
func TestOCRTestStillSaysOKWhenServiceAnswers(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[[{"text":"ab12"}]]}`))
	}))
	defer good.Close()
	if code, env := doTest(t, good.URL); code != http.StatusOK || env["ok"] != true {
		t.Errorf("正常应答要 200 ok:true，实际 %d %v", code, env)
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[]}`)) // 1x1 红点本来就没字
	}))
	defer empty.Close()
	if code, env := doTest(t, empty.URL); code != http.StatusOK || env["ok"] != true {
		t.Errorf("空结果对连通性测试算可用（#312），实际 %d %v", code, env)
	}
}
