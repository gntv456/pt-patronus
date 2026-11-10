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
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/backup"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// #285 备份导入的两档混淆（#257 修了 .ptd 与自有格式，MoviePilot 这一路一条分类都没有）：
//   - 选错文件（传进来是对象而不是站点数组）⇒ 400「请求参数有误，请检查后重试」，
//     界面既不说「该换个文件」也不说要什么形状；
//   - 站点批量写入失败（库坏了 / 加密失败）⇒ 同样 400「参数有误」，而这批写在**一个事务**里，
//     实际一个都没落库，用户却以为文件有问题反复重选、重试。
//
// 路径照生产抄（routes_ops.go:40-42 + guarded=/api/v1）。

func backupRouter(t *testing.T) (r *gin.Engine, closeDB func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/bk285.db"), &gorm.Config{})
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
	crypto := security.NewCrypto("bk285-seed")
	h := NewBackupHandler(backup.New(oper.NewSiteOper(db, crypto), oper.NewDownloaderOper(db, crypto), nil), nil)
	r = gin.New()
	r.POST("/api/v1/backup/import", h.Import)
	r.POST("/api/v1/backup/import-moviepilot", h.ImportMoviePilot)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	return r, func() { _ = sqlDB.Close() }
}

func postBackup(t *testing.T, r *gin.Engine, path, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("%s → %d，响应不是 JSON：%q", path, rec.Code, rec.Body.String())
	}
	return rec.Code, env
}

// 正控：合法的 MoviePilot 数组照常导入（加分类不许碰通这条路）。
func TestImportMoviePilotHappyPathStillWorks(t *testing.T) {
	r, _ := backupRouter(t)
	status, env := postBackup(t, r, "/api/v1/backup/import-moviepilot",
		`[{"name":"_siteA","url":"https://sitea.test"},{"name":"_siteB","url":"https://siteb.test"}]`)
	if status != http.StatusOK {
		t.Fatalf("status=%d 整包=%v，期望 200", status, env)
	}
	if env["imported_sites"] != float64(2) {
		t.Fatalf("应导入 2 个站点：%v", env)
	}
}

func TestImportMoviePilotWrongFileShape(t *testing.T) {
	r, _ := backupRouter(t)
	status, env := postBackup(t, r, "/api/v1/backup/import-moviepilot", `{"sites":[1,2]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("形状不对应 400：status=%d 整包=%v", status, env)
	}
	if env["code"] != errcode.BackupNotMoviePilotFile.Code {
		t.Fatalf("要说清「这不是 MoviePilot 的站点 JSON」，不能只甩参数有误：整包=%v", env)
	}
	msg, _ := env["message"].(string)
	for _, want := range []string{"MoviePilot", "数组"} {
		if !strings.Contains(msg, want) {
			t.Errorf("缺 %q：%q", want, msg)
		}
	}
	// Go 侧的 json 原文（"parse moviepilot sites: json: cannot unmarshal object…"）只进日志。
	if strings.Contains(msg, "unmarshal") || strings.Contains(msg, "parse moviepilot") {
		t.Errorf("英文原文进了界面：%q", msg)
	}
}

// 库写不进去：这批在一个事务里 ⇒ 一个都没落库，必须交代「没有导入任何站点」。
func TestImportMoviePilotWriteFailureIsNotParamError(t *testing.T) {
	r, closeDB := backupRouter(t)
	closeDB() // 真故障：连接池已关，CreateMany 必然报错
	status, env := postBackup(t, r, "/api/v1/backup/import-moviepilot", `[{"name":"s1","url":"https://s1.test"}]`)
	if env["code"] == errcode.InvalidInput.Code || env["code"] == errcode.BackupNotMoviePilotFile.Code {
		t.Fatalf("把库坏了说成文件/参数问题：status=%d 整包=%v", status, env)
	}
	if status != http.StatusServiceUnavailable || env["code"] != errcode.BackupWriteFailed.Code {
		t.Fatalf("status=%d 整包=%v，期望 503 + %q", status, env, errcode.BackupWriteFailed.Code)
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "没有") || !strings.Contains(msg, "导入") {
		t.Errorf("要交代一个都没落库（否则用户以为导了一半）：%q", msg)
	}
}

// 自有格式同样不许把写库失败甩成「参数有误」。
func TestImportOwnFormatWriteFailureIsNotParamError(t *testing.T) {
	r, closeDB := backupRouter(t)
	closeDB()
	status, env := postBackup(t, r, "/api/v1/backup/import", `{"version":1,"sites":[{"name":"s","base_url":"https://s.test"}]}`)
	if env["code"] == errcode.InvalidInput.Code {
		t.Fatalf("写库失败被压成 400 参数有误：status=%d 整包=%v", status, env)
	}
	if status != http.StatusServiceUnavailable || env["code"] != errcode.BackupWriteFailed.Code {
		t.Fatalf("status=%d 整包=%v，期望 503 + %q", status, env, errcode.BackupWriteFailed.Code)
	}
}

// 正控：自有格式已有的两档不许被我的改动带坏。
func TestImportOwnFormatExistingBuckets(t *testing.T) {
	r, _ := backupRouter(t)
	_, env := postBackup(t, r, "/api/v1/backup/import", `{}`)
	if env["code"] != errcode.BackupNotOurFile.Code {
		t.Fatalf("空对象仍应判成「不是备份文件」：整包=%v", env)
	}
	_, env = postBackup(t, r, "/api/v1/backup/import", `{"version":99999,"sites":[]}`)
	if env["code"] != errcode.BackupVersionTooNew.Code {
		t.Fatalf("高版本备份仍应判成「请先升级」：整包=%v", env)
	}
}
