package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDBHandler(t *testing.T) *DatabaseHandler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)")
	db.Exec("INSERT INTO users (id, name) VALUES (1, 'alice')")
	db.Exec("INSERT INTO users (id, name) VALUES (2, 'bob')")
	return NewDatabaseHandler(db)
}

func doJSONReq(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// === 基础功能 ===

func TestDatabaseTables(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.GET("/tables", h.Tables)
	w := doJSONReq(t, r, http.MethodGet, "/tables", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Tables []dbTable `json:"tables"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tb := range resp.Tables {
		if tb.Name == "users" {
			found = true
			if tb.RowCount != 2 {
				t.Errorf("users row_count = %d, want 2", tb.RowCount)
			}
		}
	}
	if !found {
		t.Error("users table not found")
	}
}

func TestDatabaseSchema(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.GET("/tables/:table/schema", h.Schema)
	w := doJSONReq(t, r, http.MethodGet, "/tables/users/schema", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", w.Code)
	}
	var resp struct {
		Columns []dbColumn `json:"columns"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(resp.Columns))
	}
}

func TestDatabaseSchemaInvalidIdent(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.GET("/tables/:table/schema", h.Schema)
	w := doJSONReq(t, r, http.MethodGet, "/tables/bad%20table/schema", "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid ident should be rejected, got code=%d", w.Code)
	}
}

// === 只读查询 ===

func TestDatabaseQueryReadOnly(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"SELECT * FROM users","limit":10}`)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var result dbQueryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.IsWrite {
		t.Error("expected read-only query")
	}
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result.Rows))
	}
}

func TestDatabaseQuerySelectWithSemicolon(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"SELECT * FROM users;","limit":5}`)
	if w.Code != http.StatusOK {
		t.Errorf("trailing semicolon SELECT: code=%d, want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestDatabaseQuerySelectAggregation(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"SELECT count(*) as cnt FROM users","limit":10}`)
	if w.Code != http.StatusOK {
		t.Fatalf("aggregate: code=%d, body=%s", w.Code, w.Body.String())
	}
	var result dbQueryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Rows) != 1 {
		t.Errorf("aggregate: expected 1 row, got %d", len(result.Rows))
	}
}

func TestDatabaseQueryLimitClamping(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	var result dbQueryResult

	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"SELECT * FROM users","limit":0}`)
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Limit != 200 {
		t.Errorf("limit=0 should clamp to 200, got %d", result.Limit)
	}

	w = doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"SELECT * FROM users","limit":99999}`)
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Limit != 500 {
		t.Errorf("limit=99999 should clamp to 500, got %d", result.Limit)
	}

	w = doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"SELECT * FROM users","limit":-5}`)
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Limit != 200 {
		t.Errorf("negative limit should clamp to 200, got %d", result.Limit)
	}
	_ = w
}

// === 写操作一律拒绝（安全审计 P0-2：曾允许 confirm_risk 执行任意写 SQL） ===

func TestDatabaseQueryWriteAlwaysRejected(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	cases := []struct {
		name string
		sql  string
	}{
		{"delete_with_confirm", `DELETE FROM users WHERE id=1`},
		{"insert_with_confirm", `INSERT INTO users (id,name) VALUES (3,'charlie')`},
		{"update_with_confirm", `UPDATE users SET name='alice2' WHERE id=1`},
		{"update_users_priv", `UPDATE users SET is_admin=1`},
		{"create_table", `CREATE TABLE evil (id INTEGER)`},
	}
	for _, tc := range cases {
		body := `{"sql":"` + tc.sql + `","confirm_risk":true}`
		w := doJSONReq(t, r, http.MethodPost, "/query", body)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: want 403, got code=%d body=%s", tc.name, w.Code, w.Body.String())
		}
	}
	// 库未被改动：仍 2 行。
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"SELECT count(*) as c FROM users","limit":1}`)
	var result dbQueryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if count, _ := result.Rows[0][0].(float64); int(count) != 2 {
		t.Errorf("write leaked through: count=%v, want 2", result.Rows[0][0])
	}
}

func TestDatabaseQueryWriteWordInSubqueryRejected(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	// 写关键字即使不处于语句开头也必须被拒（防 CTE/子查询藏写语句）。
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"WITH x AS (DELETE FROM users RETURNING *) SELECT * FROM x","limit":5}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("CTE-hidden write should be rejected, got code=%d", w.Code)
	}
}

// === 安全防护 ===

func TestDatabaseQueryForbiddenSQL(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	// 写关键字无论形态：非 SELECT 开头走 write_sql_forbidden(403)，
	// SELECT 开头但藏写关键字的走 forbidden_sql(400)——都拒。
	for _, sql := range []string{"DROP DATABASE test", "DROP TABLE users", "TRUNCATE TABLE users", "ALTER TABLE users DROP COLUMN name"} {
		w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"`+sql+`","confirm_risk":true}`)
		if w.Code != http.StatusForbidden && w.Code != http.StatusBadRequest {
			t.Errorf("%s should be forbidden, got code=%d", sql, w.Code)
		}
	}
}

func TestDatabaseQueryMultiStatementBlocked(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"SELECT 1; DROP TABLE users","confirm_risk":true}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("multi-statement should be blocked, got code=%d", w.Code)
	}
}

func TestDatabaseQueryEmptySQL(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":""}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty SQL should be rejected, got code=%d", w.Code)
	}
}

func TestDatabaseQuerySQLTooLong(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	longSQL := strings.Repeat("a", 5001)
	body := `{"sql":"` + longSQL + `"}`
	w := doJSONReq(t, r, http.MethodPost, "/query", body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("too long SQL: code=%d, want 400", w.Code)
	}
}
