package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// #280：Trakt 的三条出口（发起授权 / 轮询授权状态 / 手动同步）从前把「用户自己能修好」的
// 处境全压成 400「请求参数有误，请检查后重试」——而 device-code 与 sync 两个 POST
// 根本没有请求体，界面那句「发起授权失败」/移动端「同步失败，请先完成授权」还各自猜一个原因。
//
// 路径照生产抄（routes_monitor.go：guarded=/api/v1 + /trakt 组）。

// seedTraktSettings 直接写链层的设置快照（字段名用 JSON tag，测试不依赖结构体导出）。
func seedTraktSettings(t *testing.T, db *gorm.DB, s map[string]any) {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := oper.NewSystemConfigOper(db).Set("trakt.settings", string(raw)); err != nil {
		t.Fatal(err)
	}
}

func traktRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/trakt.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.SystemConfig{}, &models.Subscription{}, &models.SubscriptionHit{}, &models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	subCh := chain.NewSubscribeChain(nil, nil, nil, oper.NewSubscriptionOper(db), nil, nil, nil, nil)
	h := NewTraktHandler(chain.NewTraktChain(subCh, oper.NewSystemConfigOper(db), nil))
	r := gin.New()
	r.POST("/api/v1/trakt/device-code", h.DeviceCode)
	r.GET("/api/v1/trakt/device-status", h.DeviceStatus)
	r.POST("/api/v1/trakt/sync", h.Sync)
	return r, db
}

func doTraktProd(t *testing.T, r *gin.Engine, method, path string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.ServeHTTP(rec, req)
	var body map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &body); e != nil {
		t.Fatalf("%s %s → %d，响应不是 JSON：%q", method, path, rec.Code, rec.Body.String())
	}
	return rec.Code, body
}

func TestTraktDeviceCodeNotConfiguredIsNotParamError(t *testing.T) {
	r, _ := traktRouter(t)
	status, env := doTraktProd(t, r, http.MethodPost, "/api/v1/trakt/device-code")
	if status != http.StatusServiceUnavailable || env["code"] != errcode.TraktNotConfigured.Code {
		t.Fatalf("status=%d 整包=%v，期望 503 + %q", status, env, errcode.TraktNotConfigured.Code)
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "client_id") || !strings.Contains(msg, "设置") {
		t.Errorf("要说清去哪儿填：%q", msg)
	}
	if strings.Contains(msg, "参数有误") {
		t.Errorf("这个 POST 没有请求体，不该让人去检查输入：%q", msg)
	}
	if env["code"] == errcode.TraktUpstreamFailed.Code {
		t.Error("落到上游兜底档＝哨兵没判到（可能根本没打到链层）")
	}
}

func TestTraktSyncWithoutAuthorizationSaysNextStep(t *testing.T) {
	r, db := traktRouter(t)
	seedTraktSettings(t, db, map[string]any{"client_id": "cid", "client_secret": "sec"})
	status, env := doTraktProd(t, r, http.MethodPost, "/api/v1/trakt/sync")
	if status != http.StatusConflict || env["code"] != errcode.TraktNotAuthorized.Code {
		t.Fatalf("status=%d 整包=%v，期望 409 + %q", status, env, errcode.TraktNotAuthorized.Code)
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "发起授权") {
		t.Errorf("未授权要给出可点的那一步：%q", msg)
	}
	if strings.Contains(msg, "参数有误") {
		t.Errorf("仍说成参数问题：%q", msg)
	}
}

// 授权码过期是「再等也不会亮」的终端态：轮询面必须给出停下来的依据（terminal），
// 否则面板每 3s 打一次、用户盯着已经作废的码干等。
func TestTraktDeviceStatusExpiredIsTerminal(t *testing.T) {
	r, db := traktRouter(t)
	seedTraktSettings(t, db, map[string]any{
		"client_id": "cid", "client_secret": "sec",
		"pending_device_code": "dc-old",
		"pending_user_code":   "ABCD-1234",
		"pending_expires_at":  time.Now().Add(-30 * time.Second).Unix(),
	})
	status, env := doTraktProd(t, r, http.MethodGet, "/api/v1/trakt/device-status")
	if status != http.StatusConflict || env["code"] != errcode.TraktCodeExpired.Code {
		t.Fatalf("status=%d 整包=%v，期望 409 + %q", status, env, errcode.TraktCodeExpired.Code)
	}
	if env["authorized"] != false {
		t.Errorf("authorized 字段形状不能变（移动端按 bool 读）：%v", env["authorized"])
	}
	if env["terminal"] != true {
		t.Errorf("必须标出这是终端态，前端才知道要停止轮询：%v", env["terminal"])
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "重新") {
		t.Errorf("要给出下一步：%q", msg)
	}
}

// 正控：判不出的失败仍然是「上游没完成」这一档，且英文原文不进响应。
func TestTraktUpstreamFailureStaysGeneric(t *testing.T) {
	e := traktCodeFor(errors.New("dial tcp 142.0.0.1:443: i/o timeout"))
	if e.Code != errcode.TraktUpstreamFailed.Code || e.HTTP != http.StatusBadGateway {
		t.Fatalf("未知失败应判成上游档：%+v", e)
	}
	if strings.Contains(e.Message, "dial tcp") {
		t.Errorf("网络原文不该推给人看：%q", e.Message)
	}
	// 三个哨兵档必须各归各的（抄错一个 errors.Is 就整批退化成 502）。
	for _, tc := range []struct {
		err  error
		want string
	}{
		{chain.ErrTraktNotConfigured, errcode.TraktNotConfigured.Code},
		{chain.ErrTraktNotAuthorized, errcode.TraktNotAuthorized.Code},
		{chain.ErrTraktCodeExpired, errcode.TraktCodeExpired.Code},
	} {
		if got := traktCodeFor(tc.err).Code; got != tc.want {
			t.Errorf("%v → %q, want %q", tc.err, got, tc.want)
		}
	}
}
