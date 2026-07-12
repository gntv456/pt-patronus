package v1

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

type PluginHandler struct {
	manager *plugins.Manager
	tickets *auth.TicketStore
}

const pluginAssetTicketTTL = 2 * time.Minute
const pluginAssetTicketCookieName = "ptp_plugin_asset_ticket"

func NewPluginHandler(manager *plugins.Manager, tickets *auth.TicketStore) *PluginHandler {
	return &PluginHandler{manager: manager, tickets: tickets}
}

type pluginConfigReq struct {
	Config map[string]any `json:"config"`
}

type pluginActionReq struct {
	Action string         `json:"action" binding:"max=64"`
	Input  map[string]any `json:"input"`
}

type pluginEventReq struct {
	Type string         `json:"type" binding:"max=64"`
	Data map[string]any `json:"data"`
}

type pluginBridgeReq struct {
	Method string         `json:"method" binding:"max=16"`
	Params map[string]any `json:"params"`
}

type pluginInstallReq struct {
	Source                      string                          `json:"source" binding:"max=256"`
	ID                          string                          `json:"id" binding:"max=128"`
	AcceptedPermissions         []string                        `json:"accepted_permissions" binding:"dive,max=64"`
	AcceptedPermissionsByPlugin []pluginInstallPermissionAccept `json:"accepted_permissions_by_plugin" binding:"dive"`
}

type pluginInstallPermissionAccept struct {
	ID                  string   `json:"id" binding:"max=128"`
	AcceptedPermissions []string `json:"accepted_permissions" binding:"dive,max=64"`
}

type pluginPermissionReq struct {
	AcceptedPermissions []string `json:"accepted_permissions" binding:"dive,max=64"`
}

type pluginRollbackReq struct {
	ReleaseID uint `json:"release_id" binding:"min=1"`
}

func pluginAssetScope(id string) string {
	return "plugin-assets:" + id
}

func (h *PluginHandler) List(c *gin.Context) {
	ps, err := h.manager.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"plugins": ps})
}

// MarketSources 返回 UI 持久化的插件市场源列表（GitHub 仓库地址或任意 URL）。
func (h *PluginHandler) MarketSources(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"sources": h.manager.GetMarketSources()})
}

type pluginMarketSourcesReq struct {
	Sources []string `json:"sources" binding:"dive,max=256"`
}

// SetMarketSources 持久化插件市场源列表。
func (h *PluginHandler) SetMarketSources(c *gin.Context) {
	var req pluginMarketSourcesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.SetMarketSources(req.Sources); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sources": h.manager.GetMarketSources()})
}

// TrustConfig 返回插件签名信任配置（官方只读公钥 + 用户受信公钥 + allow_unsigned）。
func (h *PluginHandler) TrustConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.manager.GetTrustConfig())
}

type pluginTrustConfigReq struct {
	TrustedPublicKeys []string `json:"trusted_public_keys" binding:"dive,max=2048"` // ed25519 base64 公钥约 68 字符，给宽上界
	AllowUnsigned     bool     `json:"allow_unsigned"`
}

// SetTrustConfig 持久化插件签名信任配置（受信公钥 + 是否允许未签名）。官方公钥只读不可改。
func (h *PluginHandler) SetTrustConfig(c *gin.Context) {
	var req pluginTrustConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.SetTrustConfig(req.TrustedPublicKeys, req.AllowUnsigned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.manager.GetTrustConfig())
}

func (h *PluginHandler) Get(c *gin.Context) {
	p, err := h.manager.Get(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *PluginHandler) Enable(c *gin.Context) {
	if err := h.validateAcceptedPermissions(c); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.SetEnabled(c.Param("id"), true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true})
}

func (h *PluginHandler) Disable(c *gin.Context) {
	if err := h.manager.SetEnabled(c.Param("id"), false); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": false})
}

func (h *PluginHandler) SaveConfig(c *gin.Context) {
	var in pluginConfigReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Config == nil {
		in.Config = map[string]any{}
	}
	p, err := h.manager.SaveConfig(c.Param("id"), in.Config)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *PluginHandler) Action(c *gin.Context) {
	var in pluginActionReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Input == nil {
		in.Input = map[string]any{}
	}
	out, err := h.manager.ExecuteAction(c.Request.Context(), c.Param("id"), in.Action, in.Input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "output": out})
}

func (h *PluginHandler) Logs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	logs, err := h.manager.Logs(c.Query("plugin_id"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

func (h *PluginHandler) Asset(c *gin.Context) {
	if _, ok := c.Get("uid"); !ok {
		if h.tickets == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		ticket := c.Query("ticket")
		if ticket == "" {
			if cookie, err := c.Cookie(pluginAssetTicketCookieName); err == nil {
				ticket = cookie
			}
		}
		if _, ok := h.tickets.Validate(ticket, pluginAssetScope(c.Param("id"))); !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired ticket"})
			return
		}
	}
	asset, err := h.manager.ReadAsset(c.Param("id"), c.Param("path"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, asset.ContentType, asset.Data)
}

func (h *PluginHandler) AssetTicket(c *gin.Context) {
	if h.tickets == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ticket store is not configured"})
		return
	}
	tk, exp, err := h.tickets.IssueScoped(c.GetUint("uid"), pluginAssetScope(c.Param("id")), pluginAssetTicketTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "issue asset ticket failed"})
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		pluginAssetTicketCookieName,
		tk,
		int(time.Until(exp).Seconds()),
		"/api/v1/plugins/"+c.Param("id")+"/assets/",
		"",
		false,
		true,
	)
	c.JSON(http.StatusOK, gin.H{
		"ticket":     tk,
		"expires_at": exp,
		"expires_in": int(time.Until(exp).Seconds()),
	})
}

func (h *PluginHandler) Releases(c *gin.Context) {
	releases, err := h.manager.ListReleases(c.Param("id"), 50)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"releases": releases})
}

func (h *PluginHandler) Bridge(c *gin.Context) {
	var in pluginBridgeReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	out, err := h.manager.Bridge(c.Request.Context(), c.Param("id"), in.Method, in.Params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "result": out})
}

func (h *PluginHandler) Rollback(c *gin.Context) {
	var in pluginRollbackReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	out, err := h.manager.Rollback(c.Request.Context(), c.Param("id"), in.ReleaseID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *PluginHandler) PublishEvent(c *gin.Context) {
	var in pluginEventReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Type == "" {
		in.Type = "plugin.test"
	}
	if in.Data == nil {
		in.Data = map[string]any{"source": "api"}
	}
	h.manager.PublishEvent(c.Request.Context(), in.Type, in.Data)
	c.JSON(http.StatusOK, gin.H{"published": true, "type": in.Type})
}

func (h *PluginHandler) Market(c *gin.Context) {
	var (
		idx *plugins.MarketIndex
		err error
	)
	if q := c.Query("source"); q != "" {
		// 单源覆盖（向后兼容）
		idx, err = h.manager.FetchMarket(c.Request.Context(), q)
	} else {
		// 聚合全部已配置源
		idx, err = h.manager.FetchMarketMulti(c.Request.Context(), nil)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, idx)
}

func (h *PluginHandler) Install(c *gin.Context) {
	var in pluginInstallReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	options := plugins.MarketInstallOptions{
		AcceptedPermissions:         in.AcceptedPermissions,
		AcceptedPermissionsByPlugin: make(map[string][]string, len(in.AcceptedPermissionsByPlugin)),
	}
	for _, item := range in.AcceptedPermissionsByPlugin {
		if item.ID == "" {
			continue
		}
		options.AcceptedPermissionsByPlugin[item.ID] = item.AcceptedPermissions
	}
	if len(options.AcceptedPermissionsByPlugin) == 0 {
		options.AcceptedPermissionsByPlugin = nil
	}
	out, err := h.manager.InstallFromMarketWithOptions(c.Request.Context(), in.Source, in.ID, options)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *PluginHandler) Reload(c *gin.Context) {
	out, err := h.manager.ReloadExternal(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *PluginHandler) Uninstall(c *gin.Context) {
	if err := h.manager.Uninstall(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"uninstalled": c.Param("id")})
}

func (h *PluginHandler) validateAcceptedPermissions(c *gin.Context) error {
	if c.Request.ContentLength == 0 {
		return nil
	}
	var in pluginPermissionReq
	if err := c.ShouldBindJSON(&in); err != nil {
		return err
	}
	if in.AcceptedPermissions == nil {
		return errors.New("accepted_permissions is required")
	}
	p, err := h.manager.Get(c.Param("id"))
	if err != nil {
		return err
	}
	if !plugins.PermissionSetEqual(in.AcceptedPermissions, p.Permissions) {
		return errors.New("accepted permissions do not match plugin permissions")
	}
	return nil
}
