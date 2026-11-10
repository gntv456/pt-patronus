// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
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
		"endpoint":    cfg.Endpoint,
		"api_key":     maskSecret(cfg.APIKey),
		"api_key_set": cfg.APIKey != "",
		"mode":        cfg.Mode,
		"enabled":     cfg.Endpoint != "",
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
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	var req ocrConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	// 两种语义必须分开：endpoint 就是这条链的开关（GetConfig 用 Endpoint!="" 报 enabled），
	// 界面是预填后整份回传的 ⇒ 框里删空 = 用户要停用 OCR，得真写空；
	// 从前一律「空串=不改」，删空保存后配置照旧、enabled 标签赖着不走，想停用只能改成填个别的地址。
	// api_key 是脱敏字段（用户没法回填原值），留空仍是「不改」。
	// 写库失败绝不能回 200：OcrPanel.save() 拿到 200 就弹「已保存」，回显又还是旧值 ⇒ 没落库也被说成生效。
	if err := h.cfgOp.Set("ocr.endpoint", strings.TrimSpace(req.Endpoint)); err != nil {
		errcode.Respond(c, errcode.SaveFailed.WithErr(err))
		return
	}
	if req.APIKey != "" {
		if err := h.cfgOp.Set("ocr.api_key", req.APIKey); err != nil {
			errcode.Respond(c, errcode.SaveFailed.WithErr(err))
			return
		}
	}
	if mode := strings.TrimSpace(req.Mode); mode != "" {
		if err := h.cfgOp.Set("ocr.mode", mode); err != nil {
			errcode.Respond(c, errcode.SaveFailed.WithErr(err))
			return
		}
	}
	h.GetConfig(c)
}

// Test POST /system/ocr/test —— 用一张 1x1 红色 PNG base64 试调（验证连通/鉴权/解析）。
func (h *OCRHandler) Test(c *gin.Context) {
	cli := ocr.New(h.config())
	if !cli.Enabled() {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	// 1x1 红色 PNG：无论识别结果如何，能返回 200 且可解析即视为服务可用。
	_, err := cli.Recognize(c.Request.Context(), png1x1Base64)
	// 空结果是这张图的正常结局（本来就没字）：连通、鉴权、解析三关都过了，仍算可用。
	// 识别真文本时同一条 ErrEmptyResult 必须算失败，两种语义靠哨兵分开，不靠猜。
	if err != nil && !errors.Is(err, ocr.ErrEmptyResult) {
		// 从前这里把 userErrText(err) 原样贴到界面：连不上时用户看到的是
		// `Post "http://…": dial tcp …: connectex: No connection could be made…`
		// （真路由实测），而 OcrPanel.test() 就是 ElMessage.error(r.error)。
		// 现在两种处境各给人话文案，Go/os 原文只进日志。
		slog.Warn("ocr test failed", "err", err, "endpoint", h.config().Endpoint)
		var ue *url.Error
		if errors.As(err, &ue) {
			errcode.Respond(c, errcode.OCRServiceUnreachable)
			return
		}
		errcode.Respond(c, errcode.OCRServiceBadAnswer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// png1x1Base64 1x1 红色像素 PNG。
const png1x1Base64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// ocrRecognizeReq POST /system/ocr/recognize 请求体（浏览器扩展验证码识别用）。
type ocrRecognizeReq struct {
	Image string `json:"image" binding:"required,max=4194304"` // data URL 或裸 base64；4MB 上限
}

// Recognize POST /system/ocr/recognize —— 识别任意 base64 图片（扩展 content script 截取站点
// 登录验证码后经 background 上传）。复用 ocr.Client（paddle/simple/ddddoc 三模式）；
// 未配置端点返回 400 供扩展静默回退人工输入。RequireAdmin：识别能力不开放给普通登录态。
func (h *OCRHandler) Recognize(c *gin.Context) {
	cli := ocr.New(h.config())
	if !cli.Enabled() {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	var req ocrRecognizeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	text, err := cli.Recognize(c.Request.Context(), req.Image)
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"text": text})
}
