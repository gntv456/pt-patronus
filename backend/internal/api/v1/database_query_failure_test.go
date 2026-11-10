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
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// #290 运维 SQL 控制台：四类「输入不合规」各有专门码，唯独真正执行时数据库给出的那句报错
// 被压成通用 400「请求参数有误，请检查后重试」——而 no such column / syntax error 就是操作者
// 要改的地方。路径照生产抄（routes_ops.go:164-169：/api/v1/ops/database/query，组内 RequireAdmin）。

func dbQueryRouter(t *testing.T) *gin.Engine {
	t.Helper()
	r := gin.New()
	r.POST("/api/v1/ops/database/query", setupTestDBHandler(t).Query)
	return r
}

func TestDatabaseQueryFailureCarriesDBReason(t *testing.T) {
	r := dbQueryRouter(t)

	cases := []struct {
		name string
		sql  string
		want string
	}{
		{"表不存在", `SELECT * FROM no_such_table`, "no such table"},
		{"列不存在", `SELECT no_such_col FROM users`, "no such column"},
		{"子查询没写完", `SELECT * FROM (SELECT 1`, "incomplete input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSONReq(t, r, http.MethodPost, "/api/v1/ops/database/query",
				`{"sql":"`+tc.sql+`","limit":10}`)
			var env map[string]any
			if e := json.Unmarshal(w.Body.Bytes(), &env); e != nil {
				t.Fatalf("响应不是 JSON：%q", w.Body.String())
			}
			if w.Code != http.StatusBadRequest || env["code"] != errcode.DatabaseQueryFailed.Code {
				t.Fatalf("status=%d 整包=%v，期望 400 + %q", w.Code, env, errcode.DatabaseQueryFailed.Code)
			}
			msg, _ := env["message"].(string)
			// 通用「请求参数有误」等于什么都没指：必须同时给出下一步与数据库原话。
			if strings.Contains(msg, "请求参数有误") {
				t.Errorf("仍是通用文案：%q", msg)
			}
			if !strings.Contains(msg, tc.want) {
				t.Errorf("报错原话没带回（要含 %q）：%q", tc.want, msg)
			}
		})
	}
}

// 正控：前面四类判据与正常查询都不许被这条改动带坏。
func TestDatabaseQueryOtherBucketsIntact(t *testing.T) {
	r := dbQueryRouter(t)

	w := doJSONReq(t, r, http.MethodPost, "/api/v1/ops/database/query", `{"sql":"  ","limit":10}`)
	if code := envCode(t, w); code != errcode.FieldRequired.Code {
		t.Errorf("空 SQL 应仍是 field_required，得 %s", code)
	}
	w = doJSONReq(t, r, http.MethodPost, "/api/v1/ops/database/query", `{"sql":"DELETE FROM users","limit":10}`)
	if code := envCode(t, w); code != errcode.WriteSQLForbidden.Code {
		t.Errorf("写语句应仍是 write_sql_forbidden，得 %s", code)
	}
	w = doJSONReq(t, r, http.MethodPost, "/api/v1/ops/database/query", `{"sql":"SELECT 1; SELECT 2","limit":10}`)
	if code := envCode(t, w); code != errcode.MultiStatement.Code {
		t.Errorf("多语句应仍是 multi_statement，得 %s", code)
	}
	w = doJSONReq(t, r, http.MethodPost, "/api/v1/ops/database/query", `{"sql":"SELECT id FROM users","limit":10}`)
	if w.Code != http.StatusOK {
		t.Fatalf("正常查询应 200，得 %d body=%q", w.Code, w.Body.String())
	}
}

func envCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var env map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", w.Body.String())
	}
	code, _ := env["code"].(string)
	return code
}
