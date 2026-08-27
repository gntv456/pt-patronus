package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/ocr"
)

// OCRHandler OCR 服务配置（对标 MSaber OCR 周边服务）：读写 SystemConfig 的
// ocr.endpoint / ocr.api_key / ocr.mode。登录验证码自动识别用；未配置时回退人工输入。
type OCRHandler struct {
	cfgOp systemConfigGetter
}

// systemConfigGetter SystemConfigOper 最小接口（破 v1→oper 依赖，便于测试）。
type systemConfigGetter interface {
	Get(key string) (string, error)
	Set(key, val string) error
}

// NewOCRHandler 构造（cfgOp 可空 → 端点 503）。
func NewOCRHandler(cfgOp systemConfigGetter) *OCRHandler { return &OCRHandler{cfgOp: cfgOp} }

func (h *OCRHandler) config() ocr.Config {
	if h.cfgOp == nil {
		return ocr.Config{}
	}
	get := func(k string) string { v, _ := h.cfgOp.Get(k); return v }
	return ocr.Config{Endpoint: get("ocr.endpoint"), APIKey: get("ocr.api_key"), Mode: get("ocr.mode")}
}

// GetConfig GET /system/ocr —— 当前配置（api_key 脱敏）。
func (h *OCRHandler) GetConfig(c *gin.Context) {
	cfg := h.config()
	c.JSON(http.StatusOK, gin.H{
		"endpoint":        cfg.Endpoint,
		"api_key":         maskSecret(cfg.APIKey),
		"api_key_set":     cfg.APIKey != "",
		"mode":            cfg.Mode,
		"enabled":         cfg.Endpoint != "",
	})
}

// ocrConfigReq PUT 请求体。空字段=不修改（与媒体源配置同语义）。
type ocrConfigReq struct {
	Endpoint string `json:"endpoint" binding:"max=512"`
	APIKey   string `json:"api_key" binding:"max=256"`
	Mode     string `json:"mode" binding:"omitempty,oneof=paddle simple ddddoc"`
}

// SetConfig PUT /system/ocr —— 保存配置（即时生效；登录验证码下发时现读）。
func (h *OCRHandler) SetConfig(c *gin.Context) {
	if h.cfgOp == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "配置存储不可用"})
		return
	}
	var req ocrConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	setIf := func(key, val string) {
		if val == "" {
			return
		}
		_ = h.cfgOp.Set(key, val)
	}
	setIf("ocr.endpoint", req.Endpoint)
	setIf("ocr.api_key", req.APIKey)
	if req.Mode != "" {
		_ = h.cfgOp.Set("ocr.mode", req.Mode)
	}
	h.GetConfig(c)
}

// Test POST /system/ocr/test —— 用一张 1x1 红色 PNG base64 试调（验证连通/鉴权/解析）。
func (h *OCRHandler) Test(c *gin.Context) {
	cli := ocr.New(h.config())
	if !cli.Enabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未配置 OCR 端点"})
		return
	}
	// 1x1 红色 PNG：无论识别结果如何，能返回 200 且可解析即视为服务可用。
	_, err := cli.Recognize(c.Request.Context(), png1x1Base64)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// png1x1Base64 1x1 红色像素 PNG。
const png1x1Base64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
