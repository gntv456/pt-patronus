package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

// setupFileOpServer 挂上「按路径操作文件」的端点 + 一条本机存储（根目录=独立临时目录）。
// 现有 setupFileServer 只路由了 CRUD/列目录，删除与下载这两条最容易说谎的面没人守着。
func setupFileOpServer(t *testing.T) (*gin.Engine, uint, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fs.db")), &gorm.Config{})
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
	op := oper.NewFileStorageOper(db, security.NewCrypto("fs-op-seed"))
	st, err := op.Create(oper.FileStorageInput{Name: "loc", Type: "local", BasePath: root})
	if err != nil {
		t.Fatal(err)
	}
	mgr := filemanager.NewManager(op)
	fileCh := chain.NewFileManagerChain(chain.NewBase(module.New(), event.New()), mgr)
	h := NewFileStorageHandler(op, fileCh, mgr)
	r := gin.New()
	r.POST("/file-storages/:id/delete", h.DeleteFile)
	r.GET("/file-storages/:id/download", h.Download)
	return r, st.ID, root
}

func doFileOp(t *testing.T, r *gin.Engine, method, format string, a ...any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, fmt.Sprintf(format, a...), nil))
	return rec
}

// TestFileDownloadMissingIs404 下载一个磁盘上没有的文件：从前一律 500「服务内部错误，请稍后重试」，
// 用户以为存储坏了，其实是列表过期（文件已被别的程序删走）。
func TestFileDownloadMissingIs404(t *testing.T) {
	r, id, _ := setupFileOpServer(t)
	rec := doFileOp(t, r, http.MethodGet, "/file-storages/%d/download?path=/gone.bin", id)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404（body=%s）", rec.Code, rec.Body.String())
	}
	code, msg := errEnvelope(t, rec)
	if code != "file_not_found" {
		t.Errorf("code = %q, want file_not_found", code)
	}
	if !strings.Contains(msg, "不存在") {
		t.Errorf("msg = %q, want 说清文件不存在", msg)
	}
	if strings.Contains(msg, "The system cannot find") || strings.Contains(msg, "no such file") {
		t.Errorf("msg = %q, 不得把 Go 的英文错误弹给用户", msg)
	}
}

// TestFileDeleteMissingReportsFailure 删除一条已经不存在的记录：
// os.RemoveAll 对 ENOENT 返回 nil，从前界面收到 {"ok":true} 并报「已删除」。
func TestFileDeleteMissingReportsFailure(t *testing.T) {
	r, id, _ := setupFileOpServer(t)
	rec := doFileOp(t, r, http.MethodPost, "/file-storages/%d/delete?path=/gone.txt", id)
	var body struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是软失败信封：%s", rec.Body.String())
	}
	if body.OK {
		t.Errorf("ok = true，空操作不得报成功：%s", rec.Body.String())
	}
	if !strings.Contains(body.Error, "不存在") {
		t.Errorf("error = %q, want 中文说明文件不存在（不是 Go 原文）", body.Error)
	}
}

// TestFileDownloadStillServesRealFile 正控：加了 404 分支不能把正常下载一起掐掉。
func TestFileDownloadStillServesRealFile(t *testing.T) {
	r, id, root := setupFileOpServer(t)
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := doFileOp(t, r, http.MethodGet, "/file-storages/%d/download?path=/real.txt", id)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200（body=%s）", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "hello" {
		t.Errorf("body = %q, want 文件内容 hello", rec.Body.String())
	}
}
