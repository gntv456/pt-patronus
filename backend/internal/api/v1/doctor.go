package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/doctor"
	"github.com/hxpt/ptpatronus/internal/version"
	"gorm.io/gorm"
)

// DoctorHandler 离线健康诊断面板（对标 MP doctor）。safe-mode 下仍注册（自救用途）。
type DoctorHandler struct {
	cfg   *config.Config
	db    *gorm.DB
	extra []doctor.Check // app.go 注入的 chain 依赖检查（downloaders/mediaserver/sites 等）；MVP 为 nil
}

// NewDoctorHandler 构造。extra 为可选的额外检查（依赖业务 chain）。
func NewDoctorHandler(cfg *config.Config, db *gorm.DB, extra []doctor.Check) *DoctorHandler {
	return &DoctorHandler{cfg: cfg, db: db, extra: extra}
}

func (h *DoctorHandler) checks() []doctor.Check {
	cs := []doctor.Check{
		doctor.CheckDatabase(h.db),
		doctor.CheckRuntimePaths(h.cfg),
		doctor.CheckSystemResources(),
		doctor.CheckConfigSecurity(h.cfg),
		doctor.CheckSafeMode(h.cfg),
	}
	cs = append(cs, h.extra...)
	return cs
}

func (h *DoctorHandler) env(fix bool) map[string]any {
	return map[string]any{
		"version":   version.Version,
		"safe_mode": h.cfg.SafeMode,
		"release":   h.cfg.IsRelease(),
		"fix":       fix,
	}
}

// Run GET /system/doctor —— 跑全部检查，返回 Report。
func (h *DoctorHandler) Run(c *gin.Context) {
	c.JSON(http.StatusOK, doctor.Run(c.Request.Context(), h.checks(), h.env(false)))
}

// Fix POST /system/doctor/fix —— MVP 暂无自动修复项；原样跑一遍（Fixable 检查未来扩展）。
func (h *DoctorHandler) Fix(c *gin.Context) {
	c.JSON(http.StatusOK, doctor.Run(c.Request.Context(), h.checks(), h.env(true)))
}
