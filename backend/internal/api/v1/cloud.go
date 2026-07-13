package v1

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// CloudHandler 处理网盘 CRUD + 探活/配额/浏览 + STRM 规则 + 生成 + 订阅追更。
type CloudHandler struct {
	svc *chain.CloudChain
}

func NewCloudHandler(svc *chain.CloudChain) *CloudHandler {
	return &CloudHandler{svc: svc}
}

// ---- 网盘 CRUD ----

func (h *CloudHandler) List(c *gin.Context) {
	ds, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if ds == nil {
		ds = []models.CloudDrive{}
	}
	c.JSON(http.StatusOK, gin.H{"drives": ds})
}

func (h *CloudHandler) Create(c *gin.Context) {
	var in oper.CloudDriveInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	m, err := h.svc.Create(in)
	if err != nil {
		respondCloudErr(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *CloudHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.CloudDriveInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	m, err := h.svc.Update(id, in)
	if err != nil {
		respondCloudErr(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *CloudHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	opResult(c, h.svc.Delete(id))
}

// ---- 探活 / 配额 / 浏览 ----

func (h *CloudHandler) Test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	info, err := h.svc.Test(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "info": info})
}

func (h *CloudHandler) Quota(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	q, err := h.svc.Quota(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"quota": q})
}

// Browse 列目录（query: path=；空=网盘根/RootPath）。供 UI 文件夹选择器。
func (h *CloudHandler) Browse(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	files, err := h.svc.Browse(c.Request.Context(), id, c.Query("path"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

// ---- STRM 规则 ----

func (h *CloudHandler) ListRules(c *gin.Context) {
	rs, err := h.svc.ListRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rs == nil {
		rs = []models.StrmRule{}
	}
	c.JSON(http.StatusOK, gin.H{"rules": rs})
}

func (h *CloudHandler) CreateRule(c *gin.Context) {
	var in oper.StrmRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	r, err := h.svc.CreateRule(in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, r)
}

func (h *CloudHandler) UpdateRule(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.StrmRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	r, err := h.svc.UpdateRule(id, in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, r)
}

func (h *CloudHandler) DeleteRule(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	opResult(c, h.svc.DeleteRule(id))
}

// GenerateRule 手动触发单条规则生成 STRM。
func (h *CloudHandler) GenerateRule(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.svc.GenerateStrm(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// Generate 即时生成 STRM（文件页选中目录直接生成，不持久化为规则）。
func (h *CloudHandler) Generate(c *gin.Context) {
	var in chain.GenerateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.svc.GenerateInstant(c.Request.Context(), in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// GenerateAll 手动触发全部启用规则生成。
func (h *CloudHandler) GenerateAll(c *gin.Context) {
	h.svc.GenerateAll(c.Request.Context())
	opResult(c, nil)
}

// SyncSubscriptions 手动触发订阅追更匹配。
func (h *CloudHandler) SyncSubscriptions(c *gin.Context) {
	h.svc.SyncSubscriptions(c.Request.Context())
	opResult(c, nil)
}

// ---- STRM 记录 ----

func (h *CloudHandler) ListRecords(c *gin.Context) {
	var driveID uint
	if v := c.Query("drive_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			driveID = uint(n)
		}
	}
	rs, err := h.svc.ListRecords(driveID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rs == nil {
		rs = []models.StrmRecord{}
	}
	c.JSON(http.StatusOK, gin.H{"records": rs})
}

func (h *CloudHandler) DeleteRecord(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	opResult(c, h.svc.DeleteRecord(id))
}

// respondCloudErr 把 oper 错误映射到状态码：输入校验（类型非法）→ 400，其余 → 500。
func respondCloudErr(c *gin.Context, err error) {
	if errors.Is(err, oper.ErrInvalidInput) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
