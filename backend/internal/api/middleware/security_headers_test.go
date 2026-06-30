package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeadersPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	for _, want := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		if w.Header().Get(want) == "" {
			t.Errorf("expected security header %q to be set", want)
		}
	}
	// test 模式非 release，不应下发 HSTS
	if w.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS should not be set outside release mode")
	}
}
