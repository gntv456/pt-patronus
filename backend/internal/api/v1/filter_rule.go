package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/site"
)

// FilterRuleHandler 处理资源过滤规则的 CRUD / 全局绑定 / 规则测试。
// 对标 MoviePilot-2 app/modules/filter + agent/tools/impl/*filter_rule*。
type FilterRuleHandler struct {
	store *filter.Store
	eng   *filter.Engine
}

// NewFilterRuleHandler 构造。
func NewFilterRuleHandler(store *filter.Store, eng *filter.Engine) *FilterRuleHandler {
	return &FilterRuleHandler{store: store, eng: eng}
}

// ListBuiltins 列出 22 条内置规则（只读，前端展示）。
func (h *FilterRuleHandler) ListBuiltins(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"rules": filter.ListBuiltins()})
}

// ListCustom 列出自定义规则。
func (h *FilterRuleHandler) ListCustom(c *gin.Context) {
	rs, err := h.store.GetCustomRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": strSliceSafe(rs)})
}

// CreateCustom 新建自定义规则。
func (h *FilterRuleHandler) CreateCustom(c *gin.Context) {
	var in filter.CustomRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	r, err := h.store.AddCustomRule(in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rule": r})
}

// UpdateCustom 更新自定义规则（:id 为原 id，可在 body 改新 id）。
func (h *FilterRuleHandler) UpdateCustom(c *gin.Context) {
	id := c.Param("id")
	var in filter.CustomRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	r, err := h.store.UpdateCustomRule(id, in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rule": r})
}

// DeleteCustom 删除自定义规则。
func (h *FilterRuleHandler) DeleteCustom(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteCustomRule(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// ListGroups 列出规则组。
func (h *FilterRuleHandler) ListGroups(c *gin.Context) {
	gs, err := h.store.GetGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": grpSliceSafe(gs)})
}

// CreateGroup 新建规则组（校验语法 + 引用 id 存在）。
func (h *FilterRuleHandler) CreateGroup(c *gin.Context) {
	var in filter.RuleGroupInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	g, err := h.store.AddGroup(in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"group": g})
}

// UpdateGroup 更新规则组。
func (h *FilterRuleHandler) UpdateGroup(c *gin.Context) {
	name := c.Param("name")
	var in filter.RuleGroupInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	g, err := h.store.UpdateGroup(name, in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"group": g})
}

// DeleteGroup 删除规则组（级联清理全局绑定悬空引用）。
func (h *FilterRuleHandler) DeleteGroup(c *gin.Context) {
	name := c.Param("name")
	if err := h.store.DeleteGroup(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": name})
}

// filterBindings 三个全局绑定的组名（搜索 / 订阅 / 洗版）。
type filterBindings struct {
	Search      []string `json:"search"`
	Subscribe   []string `json:"subscribe"`
	BestVersion []string `json:"best_version"`
}

// ListBindings 读取三个全局绑定。
func (h *FilterRuleHandler) ListBindings(c *gin.Context) {
	se, _ := h.store.GetBindings(filter.KeySearchFilterRuleGroups)
	su, _ := h.store.GetBindings(filter.KeySubscribeFilterRuleGroups)
	bv, _ := h.store.GetBindings(filter.KeyBestVersionFilterRuleGroups)
	c.JSON(http.StatusOK, filterBindings{
		Search: strSliceSafe2(se), Subscribe: strSliceSafe2(su), BestVersion: strSliceSafe2(bv),
	})
}

// SetBindings 写三个全局绑定。
func (h *FilterRuleHandler) SetBindings(c *gin.Context) {
	var b filterBindings
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pairs := map[string][]string{
		filter.KeySearchFilterRuleGroups:      b.Search,
		filter.KeySubscribeFilterRuleGroups:   b.Subscribe,
		filter.KeyBestVersionFilterRuleGroups: b.BestVersion,
	}
	for key, names := range pairs {
		if err := h.store.SetBindings(key, names); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// filterTestReq 规则测试请求体。
type filterTestReq struct {
	RuleString string            `json:"rule_string" binding:"required"`
	Torrents   []site.Torrent    `json:"torrents"`
	Media      *filter.MediaInfo `json:"media,omitempty"`
}

// Test 对单条 rule_string 求值，返回命中（带 pri）/ 被剔种子 / 层级明细。
func (h *FilterRuleHandler) Test(c *gin.Context) {
	var req filterTestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Torrents == nil {
		req.Torrents = []site.Torrent{}
	}
	matched, dropped, levels, err := h.eng.ApplyGroupForTest(c.Request.Context(), req.RuleString, req.Torrents, req.Media)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"matched": matched, "dropped": dropped, "levels": levels})
}

// strSliceSafe 确保 nil slice 序列化为 [] 而非 null（前端友好）。
func strSliceSafe2(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
func strSliceSafe(rs []filter.Rule) []filter.Rule {
	if rs == nil {
		return []filter.Rule{}
	}
	return rs
}
func grpSliceSafe(gs []filter.RuleGroup) []filter.RuleGroup {
	if gs == nil {
		return []filter.RuleGroup{}
	}
	return gs
}
