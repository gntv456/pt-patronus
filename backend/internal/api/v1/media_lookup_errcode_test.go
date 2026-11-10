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
	"github.com/hxpt/ptpatronus/internal/chain"
)

// 未知 source 是 query 参数错误：从前 Detail/Celebrity 一律 500「服务内部错误」，
// 界面与 AI 工具都拿不到「源名不存在」这条线索。
func TestMediaDetailUnknownSourceIs400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mediaCh := chain.NewMediaChain(chain.Base{}, &chain.MediaSourceProvider{}, nil, nil)
	h := NewMediaHandler(mediaCh, nil)

	for _, tc := range []struct {
		name string
		path string
		fn   func(c *gin.Context)
	}{
		{"detail", "/api/v1/media/detail?source=nope&id=1", h.Detail},
		{"celebrity", "/api/v1/media/celebrity?source=nope&id=1", h.Celebrity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)
			tc.fn(c)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("未知源应回 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			var env struct {
				Code     string `json:"code"`
				Message  string `json:"message"`
				Specific bool   `json:"specific"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("响应不是 JSON: %v", err)
			}
			if env.Code != "invalid_input" || !env.Specific {
				t.Fatalf("要回 invalid_input 且带本请求专属原因, got %+v", env)
			}
			if !strings.Contains(env.Message, "未知媒体源") || !strings.Contains(env.Message, "nope") {
				t.Fatalf("原因要点名源名, got %q", env.Message)
			}
		})
	}
}
