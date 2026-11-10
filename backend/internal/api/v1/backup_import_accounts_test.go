// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/backup"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// #257 的两条导入路：
//   - MoviePilot 导入从前只回 `imported_sites`，缺 name 的条目被 `continue` 丢掉却不记账，
//     于是「20 条里有 8 条没进来」在界面上就是一句「已导入 12 个站点」；
//   - .ptd 导入把「不是 ZIP / 已加密 / 缺 manifest / 没有可用站点」这些人话原因
//     一律压成 400「请求参数有误，请检查后重试」，用户不知道该换个文件还是先去解密。
//
// 打的都是真生产通路（handler → backup.Service → SiteOper → SQLite），路径照生产注册
// （routes_ops.go:41-42：/backup/import-moviepilot、/backup/import-ptd）。
func setupBackupServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/bk.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Site{}, &models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	crypto := security.NewCrypto("bk-test-seed")
	svc := backup.New(oper.NewSiteOper(db, crypto), oper.NewDownloaderOper(db, crypto), nil)
	h := NewBackupHandler(svc, nil)
	r := gin.New()
	r.POST("/backup/import-moviepilot", h.ImportMoviePilot)
	r.POST("/backup/import-ptd", h.ImportPTD)
	return r
}

func doBackup(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestImportMoviePilotReportsSkipsAndEmpty(t *testing.T) {
	r := setupBackupServer(t)

	// 两条里有一条缺 name：必须算进跳过账，不能悄悄蒸发。
	rec := doBackup(t, r, http.MethodPost, "/backup/import-moviepilot",
		`[{"name":"好学","url":"https://hxpt.example.com"},{"url":"https://no-name.example.com"}]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("MP 导入应回 200，实际 %d, body=%q", rec.Code, rec.Body.String())
	}
	var got struct {
		ImportedSites int  `json:"imported_sites"`
		SitesSkipped  int  `json:"sites_skipped"`
		Empty         bool `json:"empty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("响应不是 JSON: %v, raw=%q", err, rec.Body.String())
	}
	if got.ImportedSites != 1 || got.SitesSkipped != 1 {
		t.Errorf("导入/跳过计数 = %d/%d, want 1/1（缺 name 的条目从前不记账）body=%q",
			got.ImportedSites, got.SitesSkipped, rec.Body.String())
	}
	if got.Empty {
		t.Errorf("导进了 1 条却报 empty=true: %q", rec.Body.String())
	}

	// 空数组（选错文件/导出为空）：一条都没进来，必须说 empty。
	rec = doBackup(t, r, http.MethodPost, "/backup/import-moviepilot", `[]`)
	var again struct {
		ImportedSites int  `json:"imported_sites"`
		Empty         bool `json:"empty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	if again.ImportedSites != 0 || !again.Empty {
		t.Errorf("空导入 = %d 条 / empty=%v, want 0 / true（否则界面把「什么都没导进来」报成成功）",
			again.ImportedSites, again.Empty)
	}
}

func TestImportPTDReasonReachesClient(t *testing.T) {
	r := setupBackupServer(t)

	// 随便几个字节：不是 ZIP。后端那句「打不开这个 .ptd（它不是 ZIP 包）」必须进响应。
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("file", "backup.ptd")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("这不是一个 ZIP 文件")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/backup/import-ptd", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("坏 .ptd 应回 400，实际 %d, body=%q", rec.Code, rec.Body.String())
	}
	var env struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应不是 JSON 信封: %v, raw=%q", err, rec.Body.String())
	}
	if env.Code != "invalid_input" {
		t.Errorf("本域码 = %q, want invalid_input（NoRoute 也回信封，只认状态码会被骗）", env.Code)
	}
	shown := env.Error + env.Message
	if strings.Contains(shown, "请求参数有误，请检查后重试") {
		t.Errorf("还在只回通用文案，用户不知道该换文件还是去解密: %q", shown)
	}
	if !strings.Contains(shown, ".ptd") {
		t.Errorf("响应里没带上后端那句人话原因: %q", shown)
	}
}
