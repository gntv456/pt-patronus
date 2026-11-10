// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"gorm.io/gorm"
)

// 上一批 requireExists 接上了 15 个 DELETE 出口，但其中 5 个当时没有现成 setup，
// 只做了静态核对。这里补上它们各自的接线用例：路径照生产路由写
// （/ops/credentials/:id、/ops/configs/:id、/registrations/:id、/webhooks/:id、/mediaservers/:id），
// 少接一个就红一条。
func newDeleteProbeDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/probe.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestDeleteMissingMoreOutlets(t *testing.T) {
	cases := []struct {
		name string
		path string
		code string
		rig  func(t *testing.T) *gin.Engine
	}{
		{"credential", "/ops/credentials/999999", "ops_credential_not_found", func(t *testing.T) *gin.Engine {
			op := oper.NewCredentialOper(newDeleteProbeDB(t, &models.OpsCredential{}, &models.MonitorTarget{}), security.NewCrypto("probe"))
			h := NewCredentialHandler(op)
			r := gin.New()
			r.DELETE("/ops/credentials/:id", h.Delete)
			return r
		}},
		{"opsConfig", "/ops/configs/999999", "ops_config_not_found", func(t *testing.T) *gin.Engine {
			op := oper.NewOpsConfigOper(newDeleteProbeDB(t, &models.OpsConfig{}, &models.OpsConfigHistory{}))
			h := NewOpsConfigHandler(op)
			r := gin.New()
			r.DELETE("/ops/configs/:id", h.Delete)
			return r
		}},
		{"registration", "/registrations/999999", "registration_not_found", func(t *testing.T) *gin.Engine {
			op := oper.NewRegistrationOper(newDeleteProbeDB(t, &models.RegistrationWatch{}))
			// Delete 只用 op；链与 cron 侧的依赖不在本条出口上，传 nil 即可。
			h := NewRegistrationHandler(op, nil)
			r := gin.New()
			r.DELETE("/registrations/:id", h.Delete)
			return r
		}},
		{"webhook", "/webhooks/999999", "webhook_not_found", func(t *testing.T) *gin.Engine {
			op := oper.NewWebhookOper(newDeleteProbeDB(t, &models.Webhook{}), security.NewCrypto("probe"))
			h := NewWebhookHandler(op, nil, nil, nil)
			r := gin.New()
			r.DELETE("/webhooks/:id", h.Delete)
			return r
		}},
		{"notice", "/notices/999999", "notice_not_found", func(t *testing.T) *gin.Engine {
			db := newDeleteProbeDB(t, &models.Notice{})
			h := NewNoticeHandler(notice.NewService(oper.NewNoticeOper(db), nil, nil))
			r := gin.New()
			r.DELETE("/notices/:id", h.Delete)
			return r
		}},
		{"mediaserver", "/mediaserver/999999", "mediaserver_not_found", func(t *testing.T) *gin.Engine {
			db := newDeleteProbeDB(t, &models.MediaServer{})
			msOp := oper.NewMediaServerOper(db, security.NewCrypto("probe"))
			svc, err := chain.NewMediaServerChain(chain.NewBase(module.New(), event.New()), msOp, nil)
			if err != nil {
				t.Fatal(err)
			}
			h := NewMediaServerHandler(svc)
			r := gin.New()
			r.DELETE("/mediaserver/:id", h.Delete) // 生产路由是 guarded.Group("/mediaserver")
			return r
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.rig(t)
			req, _ := http.NewRequest(http.MethodDelete, tc.path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			body := rec.Body.String()
			if rec.Code == http.StatusOK && strings.Contains(body, `"deleted":999999`) {
				t.Fatalf("%s 把「本来就没有」报成删除成功：%s", tc.name, body)
			}
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s code=%d body=%s，应为 404", tc.name, rec.Code, body)
			}
			if !strings.Contains(body, tc.code) {
				t.Fatalf("%s body=%s 应带本域码 %s", tc.name, body, tc.code)
			}
		})
	}
}
