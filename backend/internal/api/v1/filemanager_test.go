package v1

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/filemanager"
	"gorm.io/gorm"
)

// setupFileServer 构造真实 FileStorageOper + Manager + Chain（SQLite）+ 路由。
// 文件操作对 local 后端走真实路径（t.TempDir）——覆盖浏览/建目录等本地语义。
func setupFileServer(t *testing.T) (*gin.Engine, *gorm.DB, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	db, err := gorm.Open(sqlite.Open(root+"/fs.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.FileStorage{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewFileStorageOper(db, security.NewCrypto("fs-test-seed"))
	mgr := filemanager.NewManager(op)
	fileCh := chain.NewFileManagerChain(chain.NewBase(module.New(), event.New()), mgr)
	h := NewFileStorageHandler(op, fileCh, mgr)
	r := gin.New()
	r.GET("/file-storages", h.List)
	r.POST("/file-storages", h.Create)
	r.PUT("/file-storages/:id", h.Update)
	r.DELETE("/file-storages/:id", h.Delete)
	r.GET("/file-storages/:id/files", h.Files)
	r.POST("/file-storages/:id/mkdir", h.Mkdir)
	r.GET("/file-storages/:id/test", h.Test)
	return r, db, root
}

func doFS(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req, _ = http.NewRequest(method, path, nil)
	} else {
		req, _ = http.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestFileStorage_CRUD 存储后端增删改查 + 类型校验。
func TestFileStorage_CRUD(t *testing.T) {
	r, _, root := setupFileServer(t)

	// 创建 local 后端
	rec := doFS(t, r, http.MethodPost, "/file-storages", `{"name":"本地库","type":"local","base_path":"`+filepath.ToSlash(root)+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 列表
	rec = doFS(t, r, http.MethodGet, "/file-storages", "")
	if !strings.Contains(rec.Body.String(), "本地库") {
		t.Fatalf("列表应含新存储: %s", rec.Body.String())
	}
	// 更新（密码空=保留）
	rec = doFS(t, r, http.MethodPut, "/file-storages/1", `{"name":"改名库","type":"local","base_path":"`+filepath.ToSlash(root)+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 非法 id → 400
	if rec = doFS(t, r, http.MethodPut, "/file-storages/zz", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
	// 非法类型 → 400 或 500 均为拒收（不落脏数据）；关键是不 200 假成功
	rec = doFS(t, r, http.MethodPost, "/file-storages", `{"name":"坏","type":"ftp"}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("非法类型不得 200: %s", rec.Body.String())
	}
	// 删除
	rec = doFS(t, r, http.MethodDelete, "/file-storages/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除失败 code=%d", rec.Code)
	}
}

// TestFileStorage_FilesAndMkdir local 后端浏览/建目录。
func TestFileStorage_FilesAndMkdir(t *testing.T) {
	r, _, root := setupFileServer(t)
	doFS(t, r, http.MethodPost, "/file-storages", `{"name":"本地","type":"local","base_path":"`+filepath.ToSlash(root)+`"}`)

	// 浏览根目录
	rec := doFS(t, r, http.MethodGet, "/file-storages/1/files?path=/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("浏览 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 建目录
	rec = doFS(t, r, http.MethodPost, "/file-storages/1/mkdir", `{"path":"/","name":"新目录"}`)
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("mkdir 不应 500: %d %s", rec.Code, rec.Body.String())
	}
	// 探活
	rec = doFS(t, r, http.MethodGet, "/file-storages/1/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("test code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestFileStorage_ParamValidation id 与 body 校验路径。
func TestFileStorage_ParamValidation(t *testing.T) {
	r, _, _ := setupFileServer(t)
	if rec := doFS(t, r, http.MethodGet, "/file-storages/zz/files", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("files 非法 id 应 400，got %d", rec.Code)
	}
	// mkdir 非法 body：宽松解析为空 struct 时走后续校验——只要不 panic 即可
	_ = doFS(t, r, http.MethodPost, "/file-storages/1/mkdir", `{bad`)
	if rec := doFS(t, r, http.MethodGet, "/file-storages/zz/test", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("test 非法 id 应 400，got %d", rec.Code)
	}
}

// TestFileManager_MissingStorageErrorIsChinese 回归：软失败面（200+{ok:false,error}）
// 不得把 raw Go 错误 "record not found" 弹给中文界面用户。
// 形状仍是 200（读列表失败要在原地显示而不是整页抛错），只本地化文案。
func TestFileManager_MissingStorageErrorIsChinese(t *testing.T) {
	r, _, _ := setupFileServer(t)
	rec := doFS(t, r, http.MethodGet, "/file-storages/999/files?path=/", "")
	body := rec.Body.String()
	if !strings.Contains(body, "存储后端不存在") {
		t.Fatalf("应给中文说明，got %s", body)
	}
	if strings.Contains(body, "record not found") {
		t.Fatalf("不得回显 Go 原文，got %s", body)
	}
}
