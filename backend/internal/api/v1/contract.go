package v1

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// ContractHandler 保种契约规则 CRUD + 现场检查。
type ContractHandler struct {
	svc *chain.ContractChain
}

func NewContractHandler(svc *chain.ContractChain) *ContractHandler {
	return &ContractHandler{svc: svc}
}

func (h *ContractHandler) List(c *gin.Context) {
	rs, err := h.svc.List()
	if err != nil {
		// 这条 GET 没有任何参数，回 400「请求参数有误」是把服务端读不出来推给用户手滑。
		errcode.Respond(c, errcode.ContractRulesUnreadable.WithErr(err))
		return
	}
	if rs == nil {
		rs = []models.ContractRule{}
	}
	c.JSON(http.StatusOK, gin.H{"rules": rs})
}

// Upsert 新增或更新契约（按 site_id 合并，每站一条）。
func (h *ContractHandler) Upsert(c *gin.Context) {
	var r models.ContractRule
	if err := c.ShouldBindJSON(&r); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if r.SiteID == 0 {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	if err := h.svc.Upsert(&r); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, r)
}

func (h *ContractHandler) Delete(c *gin.Context) {
	// 从前 `id, _ := strconv.ParseUint(...)` 把解析错误丢掉：`/contract/rules/abc` 会拿 id=0 去删，
	// GORM 未命中不报错 ⇒ 回 200 说「已删除」，而那条规则还在库里。
	id, ok := parseID(c)
	if !ok {
		return
	}
	if !requireExists(c, h.svc.Get, id, errcode.ContractNotFound) {
		return
	}
	opResult(c, h.svc.Delete(id))
}

// Check 现场检查单条契约，返回达标/违例详情。
func (h *ContractHandler) Check(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.svc.Check(c.Request.Context(), id)
	if err != nil {
		// 「规则未启用」是链层给出的根因，必须原样抵达用户；
		// 从前它被塞进 Status:"ok" 的响应体里、界面又不渲染 detail，
		// 用户看到的是一行全 0 的「实时」读数。
		if errors.Is(err, chain.ErrRuleDisabled) {
			errcode.Respond(c, errcode.InvalidInput.WithDetail(err))
			return
		}
		respondRecordError(c, err, errcode.ContractNotFound, errcode.InvalidInput, errcode.InvalidInput)
		return
	}
	c.JSON(http.StatusOK, res)
}
