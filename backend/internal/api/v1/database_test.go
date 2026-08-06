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

// === 写操作 ===

func TestDatabaseQueryWriteNeedsConfirm(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"DELETE FROM users WHERE id=1"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400 (needs confirm)", w.Code)
	}
	w = doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"DELETE FROM users WHERE id=1","confirm_risk":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var result dbQueryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if !result.IsWrite || result.Affected != 1 {
		t.Errorf("expected write affected=1, got isWrite=%v affected=%d", result.IsWrite, result.Affected)
	}
}

func TestDatabaseQueryInsertWithConfirm(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"INSERT INTO users (id,name) VALUES (3,'charlie')","confirm_risk":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("insert: code=%d, body=%s", w.Code, w.Body.String())
	}
	var result dbQueryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if !result.IsWrite || result.Affected != 1 {
		t.Errorf("insert: isWrite=%v affected=%d", result.IsWrite, result.Affected)
	}
}

func TestDatabaseQueryUpdateWithConfirm(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"UPDATE users SET name='alice2' WHERE id=1","confirm_risk":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("update: code=%d", w.Code)
	}
	var result dbQueryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Affected != 1 {
		t.Errorf("update affected=%d, want 1", result.Affected)
	}
}

func TestDatabaseQueryWriteThenVerify(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"INSERT INTO users (id,name) VALUES (3,'dave')","confirm_risk":true}`)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"SELECT count(*) as c FROM users","limit":1}`)
	var result dbQueryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Rows) != 1 {
		t.Fatalf("verify query rows=%d", len(result.Rows))
	}
	count, _ := result.Rows[0][0].(float64)
	if int(count) != 3 {
		t.Errorf("after insert: count=%v, want 3", result.Rows[0][0])
	}
}

// === 安全防护 ===

func TestDatabaseQueryForbiddenSQL(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"DROP DATABASE test","confirm_risk":true}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("DROP DATABASE should be forbidden, got code=%d", w.Code)
	}
}

func TestDatabaseQueryTruncateTableBlocked(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"TRUNCATE TABLE users","confirm_risk":true}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("TRUNCATE TABLE should be forbidden, got code=%d", w.Code)
	}
}

func TestDatabaseQueryAlterTableDropBlocked(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"ALTER TABLE users DROP COLUMN name","confirm_risk":true}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ALTER TABLE DROP should be forbidden, got code=%d", w.Code)
	}
}

func TestDatabaseQueryDropTableBlocked(t *testing.T) {
	h := setupTestDBHandler(t)
	r := gin.New()
	r.POST("/query", h.Query)
	w := doJSONReq(t, r, http.MethodPost, "/query", `{"sql":"DROP TABLE users","confirm_risk":true}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("DROP TABLE should be forbidden, got code=%d", w.Code)
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
