package v1

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

// ---- 插件市场运营层（对标 MSaber：百鸟朝凤/百宝袋/分神/天序） ----

// MarketCharts GET /plugins/market/charts —— 百鸟朝凤（老树洞 + 蹦蹦新窝）。
func (h *PluginHandler) MarketCharts(c *gin.Context) {
	out, err := h.manager.MarketCharts(c.Request.Context())
	if err != nil {
		// #370 三类失败分档见 marketOperErr。
		marketOperErr(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// MarketFavorites GET /plugins/market/favorites —— 百宝袋（收藏列表，市场详情级）。
func (h *PluginHandler) MarketFavorites(c *gin.Context) {
	out, err := h.manager.MarketFavorites(c.Request.Context())
	if err != nil {
		// #370 同 MarketCharts。
		marketOperErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"plugins": out})
}

// FavoritePlugin POST /plugins/market/favorites/:id —— 收藏（幂等）。
func (h *PluginHandler) FavoritePlugin(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.FavoritePlugin(id); err != nil {
		// #370 收藏失败最常见的是「市场运营层没装配」，说成「参数有误」会让人
		// 反复点、反复去改那个根本改不了的插件 id。
		marketOperErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"favorited": id})
}

// UnfavoritePlugin DELETE /plugins/market/favorites/:id —— 断缘（取消收藏，幂等）。
func (h *PluginHandler) UnfavoritePlugin(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.UnfavoritePlugin(id); err != nil {
		// #370 同 FavoritePlugin。
		marketOperErr(c, err)
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

// marketOperErr 插件市场运营出口（榜单 / 收藏 / 取消收藏）的失败分档。
//
// #370 这四条出口从前各写各的：收藏两条回 InvalidInput(400「请求参数有误」)，
// 榜单两条回 BadGateway(502「上游服务不可用，请稍后重试」)。而它们真正会遇到的失败有三类：
//
//	ErrMarketOperUnavailable 市场运营层没装配 —— 该去开配置，重试无用
//	ErrMarketUnreachable        市场源取不到（网络/上游非 2xx）—— 该稍后重试
//	其余                        请求侧问题（缺 id 之类）—— 400 是对的
//
// 两种旧错法都指错方向：收藏失败说「参数有误」让人去改那个改不了的插件 id；
// 榜单拉不到说「上游服务不可用，稍后重试」而真相是市场层没接上数据，重试一万次也一样。
//
// 刻意复用 instanceErr 对同一哨兵给出的档（DependencyMissing），不另立一套 ——
// 同一个失败在两条路上必须显示同一句话。
func marketOperErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, plugins.ErrMarketOperUnavailable):
		errcode.Respond(c, errcode.DependencyMissing.WithErr(err))
	case errors.Is(err, plugins.ErrMarketUnreachable):
		errcode.Respond(c, errcode.PluginMarketUnreachable.WithErr(err))
	default:
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
	}
}

// instanceErr 是「建分身」这条路的分档：单实例插件（该用主实例）、市场运营层没装配
// （该去开配置）、其它写失败。三种从前全被压成 400「请求参数有误」（#287）。
func instanceErr(err error) *errcode.Error {
	switch {
	case errors.Is(err, plugins.ErrSingleInstance):
		// 链层那句是「插件 X 为单实例插件，不支持分身」——只说是什么，不说下一步；
		// 界面要的是可执行的一句（直接用主实例），所以用注册文案，原句进日志。
		return errcode.PluginSingleInst.WithErr(err)
	case errors.Is(err, plugins.ErrMarketOperUnavailable):
		return errcode.DependencyMissing.WithErr(err)
	case errors.Is(err, oper.ErrPluginInstanceMissing):
		// 分身已被别处删掉/key 不对：说「刷新插件详情」，不是「保存失败」也不是「参数有误」。
		return errcode.PluginInstanceNotFound.WithErr(err)
	default:
		return errcode.SaveFailed.WithErr(err)
	}
}

// CreatePluginInstance POST /plugins/:id/instances —— 分神（建分身；单实例插件 400）。
func (h *PluginHandler) CreatePluginInstance(c *gin.Context) {
	id := c.Param("id")
	var in struct {
		InstanceKey string         `json:"instance_key" binding:"required,max=96"`
		Config      map[string]any `json:"config"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	inst, err := h.manager.CreatePluginInstance(id, in.InstanceKey, marshalConfig(in.Config))
	if err != nil {
		errcode.Respond(c, instanceErr(err))
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
		respondBindError(c, err)
		return
	}
	if err := h.manager.SavePluginInstanceConfig(id, key, marshalConfig(in.Config), *in.Enable); err != nil {
		// 走分身这条路共用的分档：市场运营层没装配是 DependencyMissing（重试无用，该去开配置），
		// 其它写失败是 SaveFailed。从前这里一律 400「请求参数有误」，让人去改那个改不了的插件 id。
		errcode.Respond(c, instanceErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"saved": key})
}

// DeletePluginInstance DELETE /plugins/:id/instances/:key —— 归墟分身（幂等）。
func (h *PluginHandler) DeletePluginInstance(c *gin.Context) {
	id, key := c.Param("id"), c.Param("key")
	if err := h.manager.DeletePluginInstance(id, key); err != nil {
		// 「本来就没有」要区别于别的失败：从前一律 400「请求参数有误」，
		// 用户以为按钮坏了，其实该刷新页面（同 #204 那一族）。
		respondRecordError(c, err, errcode.PluginInstanceNotFound, errcode.InvalidInput, errcode.InvalidInput)
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
		respondBindError(c, err)
		return
	}
	if err := h.manager.ReorderPluginInstances(id, in.IDs); err != nil {
		// 同 SavePluginInstanceConfig：复用分身出口的共用分档，不再压成「参数有误」。
		errcode.Respond(c, instanceErr(err))
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
