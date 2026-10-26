package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/tgbot"
)

// TgBotHandler Telegram 入站 bot 配置（管理员）。bot 本身是后台 goroutine（app.go 接线）。
type TgBotHandler struct {
	kv     tgbot.KVStore
	crypto tgbot.Crypto
	reload func() // 保存成功后热重载 bot goroutine（app_runtime 注入；可空）
}

// NewTgBotHandler 构造。kv/crypto 由 app.go 绑 oper.SystemConfigOper + security.Crypto。
// reload：保存成功后回调（停旧 bot、按新配置再起）；nil 则维持旧行为（需重启生效）。
func NewTgBotHandler(kv tgbot.KVStore, crypto tgbot.Crypto, reload func()) *TgBotHandler {
	return &TgBotHandler{kv: kv, crypto: crypto, reload: reload}
}

// tgBotConfigReq 配置更新请求体（bot_token 留空=不改）。
type tgBotConfigReq struct {
	Enabled        bool    `json:"enabled"`
	BotToken       string  `json:"bot_token"`
	AllowedUserIDs []int64 `json:"allowed_user_ids"`
	AllowGroupChat bool    `json:"allow_group_chat"`
	APIURL         string  `json:"api_url"`
}

// GetConfig 读 bot 配置（脱敏，token 仅留 _configured）。
func (h *TgBotHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, tgbot.Redacted(tgbot.Load(h.kv)))
}

// SetConfig 写 bot 配置。bot_token 非空才加密覆盖。保存成功后热重载 bot（停旧按新配置再起）。
func (h *TgBotHandler) SetConfig(c *gin.Context) {
	var in tgBotConfigReq
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	cfg := tgbot.Load(h.kv)
	cfg.Enabled = in.Enabled
	cfg.AllowedUserIDs = in.AllowedUserIDs
	cfg.AllowGroupChat = in.AllowGroupChat
	cfg.APIURL = strings.TrimSpace(in.APIURL)
	if strings.TrimSpace(in.BotToken) != "" {
		enc, err := h.crypto.Encrypt(strings.TrimSpace(in.BotToken))
		if err != nil {
			errcode.Respond(c, errcode.Internal.WithErr(err))
			return
		}
		cfg.BotTokenEnc = enc
	}
	if err := tgbot.Save(h.kv, cfg); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if h.reload != nil {
		h.reload()
	}
	c.JSON(http.StatusOK, tgbot.Redacted(cfg))
}
