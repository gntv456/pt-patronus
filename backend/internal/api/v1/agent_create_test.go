package v1

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

func TestAgentHandlerCreateTargetType(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/agent_handler.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	} else {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.MonitorTarget{}, &models.Agent{}); err != nil {
		t.Fatal(err)
	}

	h := NewAgentHandler(
		oper.NewAgentOper(db),
		oper.NewMonitorOper(db, security.NewCrypto("agent-handler-test")),
		db,
	)
	r := gin.New()
	r.POST("/agents", h.Create)

	tests := []struct {
		body     string
		wantType string
	}{
		{body: `{"name":"domestic-nas","type":"nas"}`, wantType: "nas"},
		{body: `{"name":"legacy-host"}`, wantType: "host"},
	}
	for _, tt := range tests {
		rec := doReq(t, r, http.MethodPost, "/agents", tt.body)
		if rec.Code != http.StatusOK {
			t.Fatalf("create body=%s code=%d body=%s", tt.body, rec.Code, rec.Body.String())
		}
		payload := bodyJSON(t, rec)
		if payload["plain_secret"] == "" {
			t.Fatalf("create body=%s did not return plain_secret", tt.body)
		}

		var target models.MonitorTarget
		if err := db.Order("id desc").First(&target).Error; err != nil {
			t.Fatal(err)
		}
		if target.Type != tt.wantType || target.Protocol != "agent" {
			t.Errorf("created target = %s|%s, want %s|agent", target.Type, target.Protocol, tt.wantType)
		}
	}
}
