package v1

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
)

func TestAuthCompatibilityAndNilDependencies(t *testing.T) {
	h := &AuthHandler{}
	r := gin.New()
	r.POST("/setup", h.Setup)
	r.POST("/login", h.Login)
	r.POST("/logout", h.Logout)
	r.POST("/activate", h.Activate)
	r.POST("/verify", h.VerifyVIP)
	r.GET("/status", h.Status)
	r.GET("/oidc", h.GetOIDCConfig)
	r.PUT("/oidc", h.SetOIDCConfig)

	for _, path := range []string{"/setup", "/login"} {
		rec := doReq(t, r, http.MethodPost, path, `{}`)
		if rec.Code != http.StatusGone {
			t.Fatalf("%s code = %d, want 410", path, rec.Code)
		}
	}
	if rec := doReq(t, r, http.MethodPost, "/logout", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("logout code = %d, want 400", rec.Code)
	}
	if rec := doReq(t, r, http.MethodPost, "/activate", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("activate code = %d, want 400", rec.Code)
	}
	if rec := doReq(t, r, http.MethodPost, "/verify", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("verify code = %d, want 400", rec.Code)
	}
	if rec := doReq(t, r, http.MethodGet, "/status", ""); rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rec.Code)
	} else if bodyJSON(t, rec)["authenticated"] != false {
		t.Fatalf("status body = %s", rec.Body.String())
	}
	if rec := doReq(t, r, http.MethodGet, "/oidc", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("get oidc code = %d, want 503", rec.Code)
	}
	if rec := doReq(t, r, http.MethodPut, "/oidc", `{}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("set oidc code = %d, want 503", rec.Code)
	}
	if boolStr(true) != "ok" || boolStr(false) != "fail" {
		t.Fatal("boolStr returned unexpected values")
	}
}

func TestOIDCRedirectURI(t *testing.T) {
	for _, tt := range []struct {
		name   string
		header string
		want   string
	}{
		{name: "http", want: "http://example.test/api/v1/auth/oidc/callback"},
		{name: "forwarded https", header: "https", want: "https://example.test/api/v1/auth/oidc/callback"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.test/start", nil)
			if tt.header != "" {
				req.Header.Set("X-Forwarded-Proto", tt.header)
			}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = req
			if got := oidcRedirectURI(ctx); got != tt.want {
				t.Fatalf("redirect = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCloudHandlerBoundaryValidation(t *testing.T) {
	h := NewCloudHandler(nil)
	r := gin.New()
	r.POST("/drives", h.Create)
	r.PUT("/drives/:id", h.Update)
	r.DELETE("/drives/:id", h.Delete)
	r.POST("/rules", h.CreateRule)
	r.PUT("/rules/:id", h.UpdateRule)
	r.POST("/generate", h.Generate)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/drives", `{bad`},
		{http.MethodPut, "/drives/bad", `{}`},
		{http.MethodDelete, "/drives/bad", ""},
		{http.MethodPost, "/rules", `{bad`},
		{http.MethodPut, "/rules/bad", `{}`},
		{http.MethodPost, "/generate", `{bad`},
	}
	for _, tt := range tests {
		if rec := doReq(t, r, tt.method, tt.path, tt.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s code = %d, want 400", tt.method, tt.path, rec.Code)
		}
	}
}

func TestRespondCloudErr(t *testing.T) {
	for _, tt := range []struct {
		err  error
		want int
	}{
		{err: oper.ErrInvalidInput, want: http.StatusBadRequest},
		{err: errors.New("boom"), want: http.StatusInternalServerError},
	} {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		respondCloudErr(ctx, tt.err)
		if rec.Code != tt.want {
			t.Fatalf("code = %d, want %d", rec.Code, tt.want)
		}
	}
}

func TestAgentHandlerBoundaryValidation(t *testing.T) {
	h := NewAgentHandler(nil, nil, nil)
	r := gin.New()
	r.POST("/agents", h.Create)
	r.DELETE("/agents/:id", h.Delete)
	r.POST("/agents/:id/rotate", h.Rotate)

	for _, body := range []string{`{bad`, `{}`, `{"name":""}`} {
		if rec := doReq(t, r, http.MethodPost, "/agents", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("create code = %d, want 400", rec.Code)
		}
	}
	if rec := doReq(t, r, http.MethodDelete, "/agents/bad", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete code = %d, want 400", rec.Code)
	}
	if rec := doReq(t, r, http.MethodPost, "/agents/bad/rotate", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("rotate code = %d, want 400", rec.Code)
	}
}

func TestDownloaderOperationHelpersAndValidation(t *testing.T) {
	for _, tt := range []struct {
		value   string
		want    int64
		wantErr bool
	}{
		{value: "", want: 0},
		{value: "1024", want: 1024},
		{value: "-1", wantErr: true},
		{value: "bad", wantErr: true},
	} {
		got, err := parseLimit(tt.value)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Fatalf("parseLimit(%q) = %d, %v", tt.value, got, err)
		}
	}
	if got := splitCSV(" a, ,b,a "); !reflect.DeepEqual(got, []string{"a", "b", "a"}) {
		t.Fatalf("splitCSV = %v", got)
	}

	for _, tt := range []struct {
		err      error
		wantCode any
	}{
		{err: nil},
		{err: errors.New("boom")},
		{err: downloader.ErrNotImplemented, wantCode: "not_implemented"},
	} {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		opResult(ctx, tt.err)
		if rec.Code != http.StatusOK {
			t.Fatalf("opResult code = %d", rec.Code)
		}
		if tt.wantCode != nil && bodyJSON(t, rec)["code"] != tt.wantCode {
			t.Fatalf("opResult body = %s", rec.Body.String())
		}
	}

	h := NewDownloaderHandler(nil, nil, nil)
	r := gin.New()
	r.POST("/downloaders/:id/upload", h.UploadTorrent)
	r.POST("/downloaders/:id/category", h.SetCategoryOp)
	r.POST("/downloaders/:id/tags", h.TagsOp)
	r.POST("/downloaders/:id/recheck", h.Recheck)
	if rec := doReq(t, r, http.MethodPost, "/downloaders/bad/upload", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad upload id code = %d", rec.Code)
	}
	if rec := doReq(t, r, http.MethodPost, "/downloaders/1/upload", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing upload file code = %d", rec.Code)
	}
	for _, path := range []string{"/downloaders/1/category", "/downloaders/1/tags", "/downloaders/1/recheck"} {
		if rec := doReq(t, r, http.MethodPost, path, `{bad`); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s code = %d, want 400", path, rec.Code)
		}
	}
}
