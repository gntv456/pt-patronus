package v1

import (
	"github.com/hxpt/ptpatronus/internal/api/errcode"

	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
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
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
		respondBindError(c, err)
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
		respondBindError(c, err)
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
	// 判定在链层（工具面共用），这里只归类：删除失败从前是 200 + {ok:false}，
	// 拦截器不 unwrap，界面会把没删掉的东西报成成功 —— 现在给真状态码。
	if err := h.svc.Delete(id); err != nil {
		respondRecordError(c, err, errcode.CloudDriveNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// ---- 探活 / 配额 / 浏览 ----

func (h *CloudHandler) Test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	info, err := h.svc.Test(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
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
		// 从前这里不分原因一律 400「请求参数有误」：云盘被删掉/上游网络故障都算用户的错
		//（真机实测 GET /cloud/drives/999999/quota ⇒ 400 invalid_input）。
		respondRecordError(c, err, errcode.CloudDriveNotFound, errcode.InvalidInput, errcode.BadGateway)
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
		// 同 Quota：不存在 ⇒ 404 cloud_drive_not_found，上游失败 ⇒ 502，只有真是参数问题才 400。
		respondRecordError(c, err, errcode.CloudDriveNotFound, errcode.InvalidInput, errcode.BadGateway)
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

// ---- STRM 规则 ----

func (h *CloudHandler) ListRules(c *gin.Context) {
	rs, err := h.svc.ListRules()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
		respondBindError(c, err)
		return
	}
	r, err := h.svc.CreateRule(in)
	if err != nil {
		respondRecordError(c, err, nil, errcode.InvalidInput, errcode.SaveFailed)
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
		respondBindError(c, err)
		return
	}
	r, err := h.svc.UpdateRule(id, in)
	if err != nil {
		respondRecordError(c, err, errcode.StrmRuleNotFound, errcode.InvalidInput, errcode.SaveFailed)
		return
	}
	c.JSON(http.StatusOK, r)
}

func (h *CloudHandler) DeleteRule(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	// 判定在链层（工具面共用），这里只归类。
	if err := h.svc.DeleteRule(id); err != nil {
		respondRecordError(c, err, errcode.StrmRuleNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// GenerateRule 手动触发单条规则生成 STRM。
func (h *CloudHandler) GenerateRule(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.svc.GenerateStrm(c.Request.Context(), id)
	if err != nil {
		// GenerateStrm 先按规则 id 取规则、再按规则里的云盘 id 取账号，两处都可能是
		// record not found，所以文案同时点这两个对象，别把「规则被删了」说成参数有误。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			errcode.Respond(c, errcode.StrmRuleNotFound)
			return
		}
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, res)
}

// Generate 即时生成 STRM（文件页选中目录直接生成，不持久化为规则）。
func (h *CloudHandler) Generate(c *gin.Context) {
	var in chain.GenerateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	res, err := h.svc.GenerateInstant(c.Request.Context(), in)
	if err != nil {
		// #393 同文件 7 处同类出口里唯一手抄的一份（其余走 respondRecordError/respondCloudErr）。
		// 分类本身是对的，但手抄那版是裸 Respond(CloudDriveNotFound)：errcode.Respond 只在
		// e.Err 非 nil 时写日志，而这个 e 没有 Err ⇒ 连日志关联都丢了，运维只能看到一句 404。
		// 统一判据的 404 档用 notFound.WithErr(err)，原因进日志、响应仍是 404（不外泄内部细节）。
		// 注意这不是「给用户补一句原因」：respondRecordError 的 404 档同样只进日志，
		// 这里修的是**可诊断性**，不是措辞。
		respondCloudErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// GenerateAll 手动触发全部启用规则生成，并把跨规则汇总回给调用方
// （生成过多少、哪条规则整条失败）——原先只回 {ok:true}，全部规则失败也显示成功。
func (h *CloudHandler) GenerateAll(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.GenerateAll(c.Request.Context()))
}

// SyncSubscriptions 手动触发订阅追更匹配，并把这一轮的账面回给调用方
// （原先恒回 {ok:true}：读失败、无订阅、零链接都显示「匹配完成」）。
func (h *CloudHandler) SyncSubscriptions(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.SyncSubscriptions(c.Request.Context()))
}

func respondCloudErr(c *gin.Context, err error) {
	respondRecordError(c, err, errcode.CloudDriveNotFound, errcode.InvalidInput, errcode.Internal)
}
