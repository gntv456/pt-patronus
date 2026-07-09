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
	Action string         `json:"action"`
	Input  map[string]any `json:"input"`
}

type pluginEventReq struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

type pluginBridgeReq struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

type pluginInstallReq struct {
	Source                      string                          `json:"source"`
	ID                          string                          `json:"id"`
	AcceptedPermissions         []string                        `json:"accepted_permissions"`
	AcceptedPermissionsByPlugin []pluginInstallPermissionAccept `json:"accepted_permissions_by_plugin"`
}

type pluginInstallPermissionAccept struct {
	ID                  string   `json:"id"`
	AcceptedPermissions []string `json:"accepted_permissions"`
}

type pluginPermissionReq struct {
	AcceptedPermissions []string `json:"accepted_permissions"`
}

type pluginRollbackReq struct {
	ReleaseID uint `json:"release_id"`
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
	idx, err := h.manager.FetchMarket(c.Request.Context(), c.Query("source"))
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
