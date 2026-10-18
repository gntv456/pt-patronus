package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/plugins"
	"gorm.io/gorm"
)

// #269：回滚是破坏性动作——进函数就先停插件、再把当前目录改名出去。
// 从前它的所有失败一律 400「请求参数有误」，用户被支去改输入框并反复重试，
// 而「版本记录没了 / 没配插件目录 / 快照文件没了 / 旧目录没能放回 / 账面没写进去」
// 没有一种能靠改输入框解决。下面两层各钉一遍：分类器 + 真走一遍路由。

func TestPluginRollbackFailureClassifiesEachRealSituation(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantHTTP int
	}{
		{"版本记录已被删", gorm.ErrRecordNotFound, errcode.PluginReleaseNotFound.Code, 404},
		{"包装过的记录缺失", fmt.Errorf("读取版本: %w", gorm.ErrRecordNotFound), errcode.PluginReleaseNotFound.Code, 404},
		{"版本属于别的插件", plugins.ErrReleaseNotForPlugin, errcode.PluginReleaseMismatch.Code, 400},
		{"没配插件目录", plugins.ErrPluginRootUnconfigured, errcode.PluginRootUnconfigured.Code, 409},
		{"快照目录不在磁盘", &fs.PathError{Op: "stat", Path: "plugins/demo", Err: fs.ErrNotExist}, errcode.PluginRollbackNoSnapshot.Code, 404},
		{"旧目录放回失败", fmt.Errorf("%w: %v", plugins.ErrRollbackRestoreFailed, errors.New("SECRETCANARY")), errcode.PluginRollbackRestoreFailed.Code, 500},
		{"账面没写进去", fmt.Errorf("%w: %v", plugins.ErrRollbackLedgerNotWritten, errors.New("SECRETCANARY")), errcode.PluginRollbackLedgerUnwritten.Code, 500},
		// 正控：认不出的失败落 500，不许退回「参数有误」把用户支去改输入框。
		{"认不出的失败", errors.New("disk full"), errcode.Internal.Code, 500},
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		got := pluginRollbackFailure(tc.err)
		if got.Code != tc.wantCode {
			t.Errorf("%s: code=%s want %s", tc.name, got.Code, tc.wantCode)
		}
		if got.HTTP != tc.wantHTTP {
			t.Errorf("%s: http=%d want %d", tc.name, got.HTTP, tc.wantHTTP)
		}
		if tc.wantCode != errcode.Internal.Code {
			seen[tc.wantCode] = true
		}
		if strings.TrimSpace(got.Message) == "" {
			t.Errorf("%s: 响应文案为空，界面只剩一句通用错误", tc.name)
		}
		if strings.Contains(got.Message, "SECRETCANARY") {
			t.Errorf("%s: 底层 Go 错误顶掉了注册表里的中文文案（Message=%q）", tc.name, got.Message)
		}
	}
	if len(seen) != 6 {
		t.Errorf("应是 6 条各自独立的域码，实测 %d 条：%v", len(seen), seen)
	}
}

// 5xx 必须把原因留在 Err 上：Respond 只对 5xx 记 WARN——界面拿中文，日志拿原因。
// 4xx 那几条不该被挂成 5xx（否则每次点都刷一条 WARN 却还是看不见原因）。
func TestPluginRollbackFailureKeepsReasonForLogs(t *testing.T) {
	restore := pluginRollbackFailure(fmt.Errorf("%w: rename denied", plugins.ErrRollbackRestoreFailed))
	if restore.Err == nil {
		t.Fatal("500 的失败原因没挂到 Err——响应只给固定文案时，日志是唯一还能说话的地方")
	}
	if !strings.Contains(restore.Err.Error(), "rename denied") {
		t.Errorf("日志侧原因丢失：%v", restore.Err)
	}
	if got := pluginRollbackFailure(plugins.ErrReleaseNotForPlugin); got.HTTP >= 500 {
		t.Errorf("版本归属错应当是 4xx，实测 %d", got.HTTP)
	}
}

// 接线红证：分类器不是摆设——真走一遍 POST /plugins/:id/rollback。
func TestRollbackHandlerSurfacesEachSituationThroughRealRoute(t *testing.T) {
	type envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	newStore := func(t *testing.T) (*plugins.Manager, *gorm.DB, string) {
		t.Helper()
		dir := t.TempDir()
		db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "rollback_outlet.db")), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		if err := db.AutoMigrate(&models.PluginRelease{}, &models.PluginState{}, &models.PluginInstance{}); err != nil {
			t.Fatal(err)
		}
		return plugins.NewManager(db, event.New(), nil, nil, nil), db, dir
	}
	post := func(t *testing.T, mgr *plugins.Manager, body string) (int, envelope) {
		t.Helper()
		h := NewPluginHandler(mgr, nil)
		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.POST("/plugins/:id/rollback", h.Rollback)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/plugins/demo/rollback", strings.NewReader(body)))
		var env envelope
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("响应不是 JSON（%d）：%q", rec.Code, rec.Body.String())
		}
		return rec.Code, env
	}

	t.Run("版本记录已被删", func(t *testing.T) {
		mgr, _, _ := newStore(t)
		status, env := post(t, mgr, `{"release_id":999}`)
		if status != http.StatusNotFound || env.Code != errcode.PluginReleaseNotFound.Code {
			t.Errorf("status=%d code=%q，期望 404 + %q", status, env.Code, errcode.PluginReleaseNotFound.Code)
		}
	})

	t.Run("没配插件目录", func(t *testing.T) {
		mgr, db, _ := newStore(t)
		if err := db.Create(&models.PluginRelease{PluginID: "demo", Version: "1.0", Action: "install", SnapshotDir: "x"}).Error; err != nil {
			t.Fatal(err)
		}
		status, env := post(t, mgr, `{"release_id":1}`)
		if status != http.StatusConflict || env.Code != errcode.PluginRootUnconfigured.Code {
			t.Errorf("status=%d code=%q，期望 409 + %q", status, env.Code, errcode.PluginRootUnconfigured.Code)
		}
		if !strings.Contains(env.Message, "插件目录") {
			t.Errorf("文案没指出可操作的下一步：%q", env.Message)
		}
	})

	t.Run("快照目录不在磁盘", func(t *testing.T) {
		mgr, db, dir := newStore(t)
		mgr.SetPluginRoot(dir)
		if err := db.Create(&models.PluginRelease{
			PluginID: "demo", Version: "1.0", Action: "install", SnapshotDir: filepath.Join(dir, "gone"),
		}).Error; err != nil {
			t.Fatal(err)
		}
		status, env := post(t, mgr, `{"release_id":1}`)
		if status != http.StatusNotFound || env.Code != errcode.PluginRollbackNoSnapshot.Code {
			t.Errorf("status=%d code=%q，期望 404 + %q", status, env.Code, errcode.PluginRollbackNoSnapshot.Code)
		}
	})

	// 正控：这条域里再出现「请求参数有误」就是缺陷本体回来了。
	t.Run("不得退回参数有误", func(t *testing.T) {
		mgr, _, dir := newStore(t)
		mgr.SetPluginRoot(dir)
		_, env := post(t, mgr, `{"release_id":4242}`)
		if env.Code == errcode.InvalidInput.Code || strings.Contains(env.Message, "参数有误") {
			t.Errorf("仍被压成参数有误：code=%q message=%q", env.Code, env.Message)
		}
	})
}
