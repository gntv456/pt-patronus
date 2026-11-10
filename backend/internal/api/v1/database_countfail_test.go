// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #397 运维数据库页「表行数读不出来 ⇒ 被说成 0 行」。
//
// 旧形状（backend/internal/api/v1/database.go 的 listTables）：
//
//	for _, name := range raw {
//		count, _ := h.countRows(name)          // ← 失败被整个吞掉
//		out = append(out, dbTable{Name: name, RowCount: count})
//	}
//	return out, nil                          // ← 照样 200
//
// countRows 失败时返回 (0, err)，所以那张表在响应里就是 **row_count: 0**。
// 而这一页（前端 components/ops/OpsDatabase.vue）是管理员的「库还在不在」体检页：
// users 0 / sites 0 在这里只有一个读法——数据没了。用户于是去翻备份、想去重建，
// 而真相只是那一张表的 COUNT(*) 没跑出来（库被锁 / 连接池打满 / 表刚被并发删掉）。
//
// 这是本仓反复出现的那一族（#214/#217/#335/#352…）：新增的失败出口没有读者，
// 或者被压成一个在当前语境里有害的「中性值」。0 在别处是中性的，在这里不是。
//
// 正控在本文件，负控（把吞错那行改回旧形状）必须恰好让
// TestDatabase_ListTablesMustNotSwallowCountErr 一条红。

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupCountFailDB 建一个库（users 2 行、sites 1 行），并让 countRows 固定失败。
//
// 这一页真实故障的形状是：**表名还能列出来，只是逐表计数没跑出来**（连接池打满 /
// 库被锁 / 该表正被长事务占着）。所以这里不去关连接池——那会让 listTables 第一步
// （sqlite_master 取表名）也失败，整个请求退成 500，测的就不是本次要修的那一档。
func setupCountFailDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/countfail.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// 关掉底层连接，否则 Windows 上 TempDir 清不掉（文件仍被占用）。
	t.Cleanup(func() {
		if s, e := db.DB(); e == nil {
			_ = s.Close()
		}
	})
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"alice", "bob"} {
		if err := db.Exec("INSERT INTO users (name) VALUES (?)", n).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("CREATE TABLE sites (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO sites (id) VALUES (1)").Error; err != nil {
		t.Fatal(err)
	}
	return db
}

// —— 1) 行为测试：计数失败必须让 row_count 为 null，且不能是 0 ——

// TestDatabase_TableCountFailureIsNotZero 行数读不出来时 row_count 必须是 null（未知），
// **绝不能是 0**：这一页上 0 会被读成「数据没了」，那正是本次要消灭的误导。
func TestDatabase_TableCountFailureIsNotZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewDatabaseHandler(setupCountFailDB(t))
	h.countRowsFn = func(string) (int64, error) { return 0, errors.New("SQL logic error: database is locked (1)") }
	r := gin.New()
	r.GET("/tables", h.Tables)
	w := doJSONReq(t, r, http.MethodGet, "/tables", "")

	// 表名那一步成功过，所以清单里应当真有表；但计数全失败 ⇒ 一律 null。
	var resp struct {
		Tables []struct {
			Name     string `json:"name"`
			RowCount *int64 `json:"row_count"`
			CountErr string `json:"count_error"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是 JSON: %s", w.Body.String())
	}
	if len(resp.Tables) == 0 {
		t.Fatalf("没列出任何表，响应=%s", w.Body.String())
	}
	for _, tb := range resp.Tables {
		if tb.RowCount != nil {
			t.Errorf("表 %s 计数已失败，row_count 却给出了 %d（必须是 null=未知，不能是数字、更不能是 0）",
				tb.Name, *tb.RowCount)
		}
	}
	// 反向：响应体里不许出现 `"row_count":0`（那正是用户会读成「数据没了」的那个值）。
	if strings.Contains(w.Body.String(), `"row_count":0`) {
		t.Errorf("响应体出现了 row_count:0 —— 行数未知被说成了 0 行：%s", w.Body.String())
	}
}

// TestDatabase_TableCountFailureStillSendsTableList 计数失败时**表清单仍要发出去**。
//
// 这一条是本次自己踩到的坑：先把出口写成 Respond(code) 之后，失败响应里根本没有
// `tables`，前端于是一个表都不渲染——而「一个表都没有」比「某张表显示 0 行」更能
// 误导人（用户会直接认定库被清空）。表名是取到了的，不该被连坐。
func TestDatabase_TableCountFailureStillSendsTableList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewDatabaseHandler(setupCountFailDB(t))
	h.countRowsFn = func(string) (int64, error) { return 0, errors.New("SQL logic error: database is locked (1)") }
	r := gin.New()
	r.GET("/tables", h.Tables)
	w := doJSONReq(t, r, http.MethodGet, "/tables", "")

	var resp struct {
		Code        string    `json:"code"`
		Tables      []dbTable `json:"tables"`
		CountFailed []string  `json:"count_failed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是 JSON: %s", w.Body.String())
	}
	if len(resp.Tables) == 0 {
		t.Fatalf("计数失败时表清单被一起丢掉了（界面会显示「一个表都没有」→ 用户以为库被清空）: %s", w.Body.String())
	}
	// 表名要在，且行数必须全为未知。
	names := map[string]bool{}
	for _, tb := range resp.Tables {
		names[tb.Name] = true
		if tb.RowCount != nil {
			t.Errorf("表 %s 计数失败却给出了行数 %d", tb.Name, *tb.RowCount)
		}
	}
	for _, want := range []string{"users", "sites"} {
		if !names[want] {
			t.Errorf("清单里应含 %q，实际=%v", want, names)
		}
	}
	// 还要点名哪几张没数出来（界面据此显示 ? 与原因）。
	if len(resp.CountFailed) != len(resp.Tables) {
		t.Errorf("count_failed 应列出全部读不出的表，got %v", resp.CountFailed)
	}
}

// TestDatabase_TableCountFailureHasOwnCode 计数失败必须有自己的码，且文案必须
// 点名「不是 0 行」「别据此重建」——这是这一页唯一能阻止误操作的提示。
func TestDatabase_TableCountFailureHasOwnCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewDatabaseHandler(setupCountFailDB(t))
	h.countRowsFn = func(string) (int64, error) { return 0, errors.New("SQL logic error: database is locked (1)") }
	r := gin.New()
	r.GET("/tables", h.Tables)
	w := doJSONReq(t, r, http.MethodGet, "/tables", "")

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("计数失败应 503（暂时数不出来，不是服务端崩了），got %d: %s", w.Code, w.Body.String())
	}
	var env struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != "database_table_count_failed" {
		t.Errorf("code = %q, want database_table_count_failed（这是独立一类，不该挤进 internal）", env.Code)
	}
	// 文案三条硬要求：说清不是 0、说清别重建、点名显示成 ?。
	for _, want := range []string{"不是 0 行", "不要据此重建", "?"} {
		if !strings.Contains(env.Message, want) {
			t.Errorf("文案必须含 %q（管理员据这句话才不会去重建库），got %q", want, env.Message)
		}
	}
	// error 字段与 message 同值（老客户端只读 error）。
	if env.Error != env.Message {
		t.Errorf("error 字段应与 message 同值，got %q vs %q", env.Error, env.Message)
	}
	// 表名必须带出来：运维得知道先去数哪几张。
	for _, name := range []string{"users", "sites"} {
		if !strings.Contains(env.Message, name) {
			t.Errorf("文案必须点名读不出行数的表 %q，got %q", name, env.Message)
		}
	}
}

// —— 2) 反向对照：真正的 0 行必须仍然是 0，不能被新逻辑误伤 ——

// TestDatabase_EmptyTableStillReportsZero 真正的空表（数出来了 = 0）必须回 0，
// 不许被「未知」逻辑污染。这一条是本修复最容易写坏的地方：把 nil 当 0 修、
// 又把 0 当 nil 修回来，就成了「永远数不出行数」。
func TestDatabase_EmptyTableStillReportsZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/empty.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, e := db.DB(); e == nil {
			_ = s.Close()
		}
	})
	if err := db.Exec("CREATE TABLE empty_t (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	h := NewDatabaseHandler(db)
	r := gin.New()
	r.GET("/tables", h.Tables)
	w := doJSONReq(t, r, http.MethodGet, "/tables", "")
	if w.Code != http.StatusOK {
		t.Fatalf("计数成功应 200，got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Tables []struct {
			Name     string `json:"name"`
			RowCount *int64 `json:"row_count"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tb := range resp.Tables {
		if tb.Name == "empty_t" {
			found = true
			if tb.RowCount == nil {
				t.Error("空表数出来了（=0），row_count 不该是 null")
			} else if *tb.RowCount != 0 {
				t.Errorf("空表 row_count = %d, want 0", *tb.RowCount)
			}
		}
	}
	if !found {
		t.Errorf("没列出 empty_t：%s", w.Body.String())
	}
}

// —— 3) 接线护栏：吞错的那行不许回来 ——

// TestDatabase_ListTablesMustNotSwallowCountErr 反向断言：listTables 里不许再出现
// `count, _ := h.countRows` 这种把失败整个咽掉的写法（#384 那种「假修复」的同族）。
func TestDatabase_ListTablesMustNotSwallowCountErr(t *testing.T) {
	src, err := os.ReadFile("database.go")
	if err != nil {
		t.Fatalf("读不到被测源文件: %v", err)
	}
	code := stripDatabaseComments(string(src))
	at := strings.Index(code, "func (h *DatabaseHandler) listTables()")
	if at < 0 {
		t.Fatal("找不到 listTables（判据没有落点）")
	}
	body := code[at:]
	if n := strings.Index(body[1:], "\nfunc "); n > 0 {
		body = body[:1+n]
	}
	if strings.Contains(body, "count, _ := h.countRows") || strings.Contains(body, ", _ := h.countRows") {
		t.Error("listTables 又把 countRows 的失败整个吞掉了（行数未知会变回 0 行假象）")
	}
	if !strings.Contains(body, "tableCountError") {
		t.Error("listTables 必须把计数失败分类出来（tableCountError），不能只回 200")
	}
	// RowCount 必须是指针：非指针就分不出「未知」与「0」。
	if !strings.Contains(code, "RowCount *int64") {
		t.Error("dbTable.RowCount 必须是 *int64：非指针分不出「行数未知」与「真的是 0 行」")
	}
}

// stripDatabaseComments 去掉整行注释，免得把判据本身当成被测代码。
func stripDatabaseComments(s string) string {
	out := make([]string, 0, 128)
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
