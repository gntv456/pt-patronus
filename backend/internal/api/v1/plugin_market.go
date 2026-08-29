package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

// ---- 插件市场运营层（对标 MSaber：百鸟朝凤/百宝袋/分神/天序） ----

// MarketCharts GET /plugins/market/charts —— 百鸟朝凤（老树洞 + 蹦蹦新窝）。
func (h *PluginHandler) MarketCharts(c *gin.Context) {
	out, err := h.manager.MarketCharts(c.Request.Context())
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, out)
}

// MarketFavorites GET /plugins/market/favorites —— 百宝袋（收藏列表，市场详情级）。
func (h *PluginHandler) MarketFavorites(c *gin.Context) {
	out, err := h.manager.MarketFavorites(c.Request.Context())
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"plugins": out})
}

// FavoritePlugin POST /plugins/market/favorites/:id —— 收藏（幂等）。
func (h *PluginHandler) FavoritePlugin(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.FavoritePlugin(id); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"favorited": id})
}

// UnfavoritePlugin DELETE /plugins/market/favorites/:id —— 断缘（取消收藏，幂等）。
func (h *PluginHandler) UnfavoritePlugin(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.UnfavoritePlugin(id); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"unfavorited": id})
}

// PluginInstances GET /plugins/:id/instances —— 分身列表（按天序）。
func (h *PluginHandler) PluginInstances(c *gin.Context) {
	id := c.Param("id")
	insts, err := h.manager.PluginInstances(id)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"instances": insts, "single_instance": h.manager.SingleInstance(id)})
}

// CreatePluginInstance POST /plugins/:id/instances —— 分神（建分身；单实例插件 400）。
func (h *PluginHandler) CreatePluginInstance(c *gin.Context) {
	id := c.Param("id")
	var in struct {
		InstanceKey string         `json:"instance_key" binding:"required,max=96"`
		Config      map[string]any `json:"config"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	inst, err := h.manager.CreatePluginInstance(id, in.InstanceKey, marshalConfig(in.Config))
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, inst)
}

// SavePluginInstanceConfig PUT /plugins/:id/instances/:key —— 更新分身配置/启停。
func (h *PluginHandler) SavePluginInstanceConfig(c *gin.Context) {
	id, key := c.Param("id"), c.Param("key")
	var in struct {
		Config map[string]any `json:"config"`
		Enable *bool          `json:"enable" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.manager.SavePluginInstanceConfig(id, key, marshalConfig(in.Config), *in.Enable); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"saved": key})
}

// DeletePluginInstance DELETE /plugins/:id/instances/:key —— 归墟分身（幂等）。
func (h *PluginHandler) DeletePluginInstance(c *gin.Context) {
	id, key := c.Param("id"), c.Param("key")
	if err := h.manager.DeletePluginInstance(id, key); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": key})
}

// ReorderPluginInstances PUT /plugins/:id/instances/reorder —— 天序（拖拽排序持久化）。
// body: {ids: [3,1,2]}（前端拖拽后的实例 id 顺序）。
func (h *PluginHandler) ReorderPluginInstances(c *gin.Context) {
	id := c.Param("id")
	var in struct {
		IDs []uint `json:"ids" binding:"required,min=1,max=200"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.manager.ReorderPluginInstances(id, in.IDs); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"reordered": len(in.IDs)})
}

// marshalConfig map → JSON 字符串（nil → "{}"）。
func marshalConfig(cfg map[string]any) string {
	if len(cfg) == 0 {
		return "{}"
	}
	b, err := plugins.MarshalConfigMap(cfg)
	if err != nil {
		return "{}"
	}
	return b
}
