package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// TestCreateSiteDuplicateReturns409 重复注册域必须返回 409 site_duplicate（带明确文案），
// 而非 500 internal——否则用户重复添加（如三月传媒 duckboobee.org 已存在）会误判成服务故障。
func TestCreateSiteDuplicateReturns409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/dup.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewSiteOper(db, security.NewCrypto("test-seed"))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	h := NewSiteHandler(op, nil, nil, nil)

	body := func(u string) *bytes.Reader {
		b, _ := json.Marshal(map[string]any{
			"name": "三月传媒", "platform": "nexusphp", "definition": "march",
			"base_url": u, "is_active": true,
		})
		return bytes.NewReader(b)
	}

	// 第一次创建成功
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/sites", body("https://duckboobee.org/"))
	h.Create(c)
	if w.Code != http.StatusOK {
		t.Fatalf("first create: status=%d body=%s", w.Code, w.Body.String())
	}

	// 同注册域（不同子域写法）再次创建 → 409 site_duplicate
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/sites", body("https://www.duckboobee.org/index.php"))
	h.Create(c2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate create: status=%d body=%s, want 409", w2.Code, w2.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w2.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != errcode.SiteDuplicate.Code {
		t.Fatalf("code=%q, want %q (body=%s)", resp["code"], errcode.SiteDuplicate.Code, w2.Body.String())
	}
	if resp["message"] == errcode.Internal.Message {
		t.Fatalf("message must not be generic internal error: %s", w2.Body.String())
	}
}

// TestCreateSiteDuplicatePortVariantReturns409 同注册域仅端口写法不同（hxpt.org:8080 vs
// hxpt.org）也必须判重返回 409——publicsuffix 不识别端口，判定须剥端口统一口径。
func TestCreateSiteDuplicatePortVariantReturns409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/dupport.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewSiteOper(db, security.NewCrypto("test-seed"))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	h := NewSiteHandler(op, nil, nil, nil)

	body := func(u string) *bytes.Reader {
		b, _ := json.Marshal(map[string]any{
			"name": "好学", "platform": "nexusphp", "base_url": u, "is_active": true,
		})
		return bytes.NewReader(b)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/sites", body("https://hxpt.org"))
	h.Create(c)
	if w.Code != http.StatusOK {
		t.Fatalf("first create: status=%d body=%s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/sites", body("https://hxpt.org:8080"))
	h.Create(c2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("port-variant duplicate create: status=%d body=%s, want 409", w2.Code, w2.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w2.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != errcode.SiteDuplicate.Code {
		t.Fatalf("code=%q, want %q (body=%s)", resp["code"], errcode.SiteDuplicate.Code, w2.Body.String())
	}
}
