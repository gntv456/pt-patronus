// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// 不存在的表必须回 404 + 本域码：从前回 200 {"columns":[]}，界面会画一张
// 没有列的表（与真正的空结构同形）；前端只能靠错误码提示「刷新表清单」。
func TestDatabaseSchemaMissingTableIs404(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.GET("/tables/:table/schema", h.Schema)

	// 正控：真实表的列正常返回
	w := doJSONReq(t, r, http.MethodGet, "/tables/users/schema", "")
	if w.Code != http.StatusOK {
		t.Fatalf("users schema = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var ok struct {
		Columns []map[string]any `json:"columns"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ok); err != nil || len(ok.Columns) == 0 {
		t.Fatalf("users 应有列, body=%s err=%v", w.Body.String(), err)
	}

	w2 := doJSONReq(t, r, http.MethodGet, "/tables/no_such_table/schema", "")
	if w2.Code != http.StatusNotFound {
		t.Fatalf("缺失表应回 404, got %d body=%s", w2.Code, w2.Body.String())
	}
	var env struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应不是 JSON: %v", err)
	}
	if env.Code != "database_table_not_found" {
		t.Fatalf("缺失表要回本域码, got %q", env.Code)
	}
}
