package v1

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/cookiecloud"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// newCCServerTestEnv 构造挂好公开路由的 gin engine + builtin 已启用的 chain。
func newCCServerTestEnv(t *testing.T) (*gin.Engine, *chain.CookieCloudChain, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	crypto := security.NewCrypto("seed")
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ccsrv.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.SystemConfig{}, &models.Site{}); err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(db, crypto)
	reg := site.NewRegistry()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	ch := chain.NewCookieCloudChain(&ccMemStore{}, crypto, siteOp, reg, nil, hc)

	s, err := ch.SaveSettings(chain.CookieCloudSettingsInput{Enabled: true, Mode: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	pw, err := ch.DecryptedPassword(s)
	if err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	h := NewCookieCloudServerHandler(ch)
	r.GET("/api/v1/cookiecloud/get/:uuid", h.Get)
	r.POST("/api/v1/cookiecloud/update", h.Update)
	return r, ch, s.UUID, pw
}

// ccMemStore 是 chain.ccStore 的内存实现（跨包测试用）。
type ccMemStore struct{ m map[string]string }

func (s *ccMemStore) Get(k string) (string, error) { return s.m[k], nil }
func (s *ccMemStore) Set(k, v string) error {
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[k] = v
	return nil
}

// TestCCServer_UpdateThenGet gzip 推送 → 200 done → GET 取回可解密 envelope（CORS 头存在）。
func TestCCServer_UpdateThenGet(t *testing.T) {
	r, _, uuid, pw := newCCServerTestEnv(t)
	enc, err := cookiecloud.EncryptPayload(map[string][]cookiecloud.CookieEntry{
		"demo.pt": {{Name: "sess", Value: "abc", Domain: "demo.pt", Path: "/"}},
	}, uuid, pw)
	if err != nil {
		t.Fatal(err)
	}
	// 官方扩展行为：JSON body gzip 后发送 + Content-Encoding: gzip
	raw, _ := json.Marshal(map[string]string{"uuid": uuid, "encrypted": enc})
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(raw)
	zw.Close()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookiecloud/update", &gz)
	req.Header.Set("Content-Encoding", "gzip")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"done"`) {
		t.Fatalf("update: code=%d body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS ACAO * missing on update")
	}

	// GET 取回
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/v1/cookiecloud/get/"+uuid, nil))
	if w2.Code != http.StatusOK {
		t.Fatalf("get: code=%d body=%s", w2.Code, w2.Body.String())
	}
	if w2.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS ACAO * missing on get")
	}
	payload, err := cookiecloud.DecodeEnvelope(w2.Body.Bytes(), uuid, pw)
	if err != nil {
		t.Fatal(err)
	}
	if got := cookiecloud.CookieStringForHost(payload, "demo.pt"); !strings.Contains(got, "sess=abc") {
		t.Errorf("payload=%q", got)
	}
}

// TestCCServer_NotFound 未推送数据 GET 404；uuid 错误 update 拒绝。
func TestCCServer_NotFound(t *testing.T) {
	r, _, uuid, _ := newCCServerTestEnv(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/cookiecloud/get/"+uuid, nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("empty store get: code=%d want 404", w.Code)
	}

	w2 := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"uuid": "intruder", "encrypted": "QUJDRA=="})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookiecloud/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusBadRequest || !strings.Contains(w2.Body.String(), "error") {
		t.Errorf("mismatched uuid update: code=%d body=%s", w2.Code, w2.Body.String())
	}
}

// TestCCServer_ExternalMode404 mode=external 时公开端点整体 404（不泄露存在）。
func TestCCServer_ExternalMode404(t *testing.T) {
	r, ch, uuid, _ := newCCServerTestEnv(t)
	if _, err := ch.SaveSettings(chain.CookieCloudSettingsInput{Enabled: true, Mode: "external"}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/cookiecloud/get/"+uuid, nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("external mode get: code=%d want 404", w.Code)
	}
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/api/v1/cookiecloud/update", strings.NewReader(`{}`)))
	if w2.Code != http.StatusNotFound {
		t.Errorf("external mode update: code=%d want 404", w2.Code)
	}
}
