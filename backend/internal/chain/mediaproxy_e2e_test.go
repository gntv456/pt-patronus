package chain

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// newProxyDB 代理测试共用内存 DB（MediaServer + Cloud 表）。
func newProxyDB(t *testing.T) *gormDB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/px.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.MediaServer{}, &models.CloudDrive{}, &models.StrmRule{}, &models.StrmRecord{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, e := db.DB(); e == nil {
			_ = s.Close()
		}
	})
	return db
}

// createEmbyServer 建一个指向 upURL 的 Emby 服务器记录。
func createEmbyServer(t *testing.T, db *gormDB, upURL string) *models.MediaServer {
	t.Helper()
	crypto := security.NewCrypto("seed")
	sv, err := oper.NewMediaServerOper(db, crypto).Create(oper.MediaServerInput{
		Name: "emby", Type: "emby", URL: upURL, Token: "k",
	})
	if err != nil {
		t.Fatal(err)
	}
	return sv
}

// newProxySvc 构建 MediaProxyService（ms + cloud 双链）。
func newProxySvc(t *testing.T, db *gormDB) *MediaProxyService {
	t.Helper()
	crypto := security.NewCrypto("seed")
	base := NewBase(module.New(), event.New())
	msCh, err := NewMediaServerChain(base, oper.NewMediaServerOper(db, crypto), nil)
	if err != nil {
		t.Fatal(err)
	}
	clCh, err := NewCloudChain(base, oper.NewCloudDriveOper(db, crypto), oper.NewStrmRuleOper(db), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewMediaProxyService(msCh, clCh)
}

// gormDB 别名（测试文件统一）。
type gormDB = gorm.DB

// newProxyTestStack 内存 DB 的 MediaServerChain + CloudChain + 假 Emby 上游。
// Emby 上游响应：/Items/{id}/PlaybackInfo 返回 .strm 路径（tempdir 下真实文件）。
func newProxyTestStack(t *testing.T, strmContent string) (*MediaProxyService, *MediaServerChain, string) {
	t.Helper()
	strmPath := t.TempDir() + "/a.mkv.strm"
	if err := writeFileStr(strmPath, strmContent); err != nil {
		t.Fatal(err)
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/PlaybackInfo") {
			fmt.Fprintf(w, `{"MediaSources":[{"Path":%q}]}`, strmPath)
			return
		}
		_, _ = w.Write([]byte("passthrough"))
	}))
	t.Cleanup(up.Close)

	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/px.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.MediaServer{}, &models.CloudDrive{}, &models.StrmRule{}, &models.StrmRecord{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, e := db.DB(); e == nil {
			_ = s.Close()
		}
	})
	crypto := security.NewCrypto("seed")
	msOp := oper.NewMediaServerOper(db, crypto)
	_, err = msOp.Create(oper.MediaServerInput{Name: "emby", Type: "emby", URL: up.URL, Token: "k"})
	if err != nil {
		t.Fatal(err)
	}
	base := NewBase(module.New(), event.New())
	msCh, err := NewMediaServerChain(base, msOp, nil)
	if err != nil {
		t.Fatal(err)
	}
	dOp := oper.NewCloudDriveOper(db, crypto)
	rOp := oper.NewStrmRuleOper(db)
	clCh, err := NewCloudChain(base, dOp, rOp, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewMediaProxyService(msCh, clCh), msCh, up.URL
}

// TestMediaProxy302ViaDirectStrm 播放请求 → strm 指向 /cloud/direct → 解析（fake 上游无网盘记录 → 失败）→ 回退转发。
// 此处验证「回退」路径端到端不 panic 且透传上游；302 命中路径需真实网盘凭据（真机联调项）。
func TestMediaProxy302ViaDirectStrm(t *testing.T) {
	svc, _, upURL := newProxyTestStack(t, "/api/v1/cloud/direct/3/媒体/a.mkv\n")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/emby/videos/42/original.mkv", nil)
	svc.Handle(w, req, 1, "emby/videos/42/original.mkv")
	// 网盘 id=3 不存在 → ResolveDirectLink 失败 → 回退转发 → 上游 passthrough
	if w.Code != 200 || w.Body.String() != "passthrough" {
		t.Errorf("fallback result: code=%d body=%q", w.Code, w.Body.String())
	}
	_ = upURL
}

// TestMediaProxy302WithMountStrm 挂载路径型 strm（无 direct 标记）→ 记录匹配路径 → 失败回退。
func TestMediaProxy302WithMountStrm(t *testing.T) {
	svc, _, _ := newProxyTestStack(t, "/mnt/115/媒体/a.mkv\n")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/emby/videos/42/original.mkv", nil)
	svc.Handle(w, req, 1, "emby/videos/42/original.mkv")
	if w.Code != 200 || w.Body.String() != "passthrough" {
		t.Errorf("mount-strm fallback: code=%d body=%q", w.Code, w.Body.String())
	}
}

// TestMediaProxyNonPlaybackForward 非播放请求直接透传（不触发拦截）。
func TestMediaProxyNonPlaybackForward(t *testing.T) {
	svc, _, _ := newProxyTestStack(t, "")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/emby/Items?IncludeItemTypes=Movie", nil)
	svc.Handle(w, req, 1, "emby/Items?IncludeItemTypes=Movie")
	if w.Code != 200 || w.Body.String() != "passthrough" {
		t.Errorf("plain forward: code=%d body=%q", w.Code, w.Body.String())
	}
}

func writeFileStr(p, s string) error {
	return os.WriteFile(p, []byte(s), 0o644)
}
