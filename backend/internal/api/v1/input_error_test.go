package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// oper 的校验文案是写给人看的中文（「本机存储必须填写根目录」）。
// 从前 handler 只用 WithErr 挂原因、响应照回「请求参数有误，请检查后重试」，
// 用户对着表单来回猜缺了哪一栏；这里钉住「原因抵达用户」，
// 同时钉住「非人工文案（Go 原文/裸哨兵）不得进界面」。
func errEnvelope(t *testing.T, rec *httptest.ResponseRecorder) (code, msg string) {
	t.Helper()
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是错误信封：%s", rec.Body.String())
	}
	if body.Message != body.Error {
		t.Errorf("message/error 不同值（只读 error 的客户端会拿到空）：%q / %q", body.Message, body.Error)
	}
	return body.Code, body.Message
}

func TestInputReasonReachesUser(t *testing.T) {
	t.Run("本机存储缺根目录要点名缺的是根目录", func(t *testing.T) {
		r, _, _ := setupFileServer(t)
		rec := doFS(t, r, http.MethodPost, "/file-storages", `{"name":"s1","type":"local"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400（响应体：%s）", rec.Code, rec.Body.String())
		}
		code, msg := errEnvelope(t, rec)
		if code != "invalid_input" {
			t.Errorf("code = %q, want invalid_input", code)
		}
		if msg != "本机存储必须填写根目录" {
			t.Errorf("msg = %q, want 原因「本机存储必须填写根目录」", msg)
		}
	})

	t.Run("网盘类型不在白名单要给出可选清单", func(t *testing.T) {
		r, _ := setupCloudServer(t)
		rec := doCloud(t, r, http.MethodPost, "/cloud/drives", `{"name":"d1","type":"nas"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400（响应体：%s）", rec.Code, rec.Body.String())
		}
		if _, msg := errEnvelope(t, rec); msg != "无效的网盘类型（仅支持 115/quark/123）" {
			t.Errorf("msg = %q, want 原因「无效的网盘类型（仅支持 115/quark/123）」", msg)
		}
	})

	t.Run("下载器协议不对要说协议不是泛化参数有误", func(t *testing.T) {
		r, _ := setupDownloaderServer(t)
		rec := doDL(t, r, http.MethodPost, "/downloaders",
			`{"name":"q1","type":"qb","url":"ftp://127.0.0.1:8080"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400（响应体：%s）", rec.Code, rec.Body.String())
		}
		code, msg := errEnvelope(t, rec)
		if code != "downloader_invalid_input" {
			t.Errorf("code = %q, want downloader_invalid_input", code)
		}
		if msg != `地址必须是 http/https 协议（当前为 "ftp"）` {
			t.Errorf("msg = %q, want 协议原因", msg)
		}
	})

	// handler 自己写的原因（类型白名单）同样不能只进日志。
	t.Run("下载器类型不支持要带可选清单", func(t *testing.T) {
		r, _ := setupDownloaderServer(t)
		rec := doDL(t, r, http.MethodPost, "/downloaders", `{"name":"x","type":"nope","url":"http://127.0.0.1:8080"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		_, msg := errEnvelope(t, rec)
		if !strings.Contains(msg, "不支持") || !strings.Contains(msg, "可选") {
			t.Errorf("msg = %q, want 点名不支持的类型并列出可选", msg)
		}
	})
}

// TestInputErrorStaysGeneric 反向闸：原因只有「我们自己写的中文」可以透出。
func TestInputErrorStaysGeneric(t *testing.T) {
	t.Run("第三方错误按 fallback 归类且不外泄原文", func(t *testing.T) {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		respondCloudErr(ctx, errors.New("boom: dial tcp 10.0.0.1:5178 refused"))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
		_, msg := errEnvelope(t, rec)
		if strings.Contains(msg, "boom") {
			t.Errorf("msg = %q, 不得带出内部错误原文", msg)
		}
		if msg != errcode.Internal.Message {
			t.Errorf("msg = %q, want 泛化文案 %q", msg, errcode.Internal.Message)
		}
	})

	// 裸哨兵（含 fmt.Errorf 包装但没有人工文案）只用于分类，
	// 原样带回就是界面上的 "invalid input"。
	t.Run("裸哨兵不外泄英文标记", func(t *testing.T) {
		for _, err := range []error{
			oper.ErrInvalidInput,
			fmt.Errorf("check input: %w", oper.ErrInvalidInput),
		} {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			respondCloudErr(ctx, err)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400（err=%v）", rec.Code, err)
			}
			_, msg := errEnvelope(t, rec)
			// #348 这里原先硬编码比对「请求参数有误，请检查后重试」那句话，于是任何
			// 文案改进都会撞上它、把人逼回去改回旧文案。判据该钉行为不钉字面：
			//  1) 必须是 InvalidInput 这个码的文案（不是别的兜底码）；
			//  2) 不得带出裸哨兵的英文原文（"invalid input"）。
			if msg != errcode.InvalidInput.Message {
				t.Errorf("msg = %q, want InvalidInput 的泛化文案 %q", msg, errcode.InvalidInput.Message)
			}
			if strings.Contains(strings.ToLower(msg), "invalid input") {
				t.Errorf("msg = %q, 不得把裸哨兵英文原文带进界面", msg)
			}
		}
	})
}

// hasAll 判断文案同时含若干片段。
func hasAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// TestMissingRecordIs404Not500 编辑一条已经被删掉的记录：
// 从前各域一律报 500「服务内部错误」或 400「参数有误」，
// 用户以为程序坏了 / 回头查表单，实际该刷新列表重选。
func TestMissingRecordIs404Not500(t *testing.T) {
	fsR, _, _ := setupFileServer(t)
	cloudR, _ := setupCloudServer(t)
	dlR, _ := setupDownloaderServer(t)

	cases := []struct {
		name       string
		req        func() *httptest.ResponseRecorder
		wantCode   string
		wantPhrase string
	}{
		{"存储后端", func() *httptest.ResponseRecorder {
			return doFS(t, fsR, http.MethodPut, "/file-storages/999999", `{"name":"t","type":"local","base_path":"C:/x"}`)
		}, "file_storage_not_found", "存储后端不存在"},
		{"云盘账号", func() *httptest.ResponseRecorder {
			return doCloud(t, cloudR, http.MethodPut, "/cloud/drives/999999", `{"name":"t","type":"115"}`)
		}, "cloud_drive_not_found", "云盘账号不存在"},
		{"STRM 规则", func() *httptest.ResponseRecorder {
			return doCloud(t, cloudR, http.MethodPut, "/cloud/rules/999998", `{"name":"t","drive_id":1,"cloud_path":"/x","output_dir":"/y"}`)
		}, "strm_rule_not_found", "STRM 规则"},
		{"下载器", func() *httptest.ResponseRecorder {
			return doDL(t, dlR, http.MethodPut, "/downloaders/999999", `{"name":"t","type":"qb","url":"http://127.0.0.1:8080"}`)
		}, "downloader_not_found", "下载器不存在"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := tc.req()
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404（响应体：%s）", rec.Code, rec.Body.String())
			}
			code, msg := errEnvelope(t, rec)
			if code != tc.wantCode {
				t.Errorf("code = %q, want %q", code, tc.wantCode)
			}
			if !strings.Contains(msg, tc.wantPhrase) {
				t.Errorf("msg = %q, want 说清「%s」", msg, tc.wantPhrase)
			}
			if hasAll(msg, "内部错误") || hasAll(msg, "参数有误") {
				t.Errorf("msg = %q, 记录不存在不得报成服务端故障或参数问题", msg)
			}
		})
	}
}
