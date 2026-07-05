package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

type PluginHandler struct {
	manager *plugins.Manager
}

func NewPluginHandler(manager *plugins.Manager) *PluginHandler {
	return &PluginHandler{manager: manager}
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

type pluginInstallReq struct {
	Source string `json:"source"`
	ID     string `json:"id"`
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
	out, err := h.manager.InstallFromMarket(c.Request.Context(), in.Source, in.ID)
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
