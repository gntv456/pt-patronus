package v1

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// SiteHandler 处理站点 CRUD、连接测试、搜索、校准诊断。
type SiteHandler struct {
	oper   *oper.SiteOper
	siteCh *chain.SiteChain
	reg    *site.Registry
	hc     *httpclient.Client // 校准诊断用
}

// NewSiteHandler 构造 SiteHandler。
func NewSiteHandler(o *oper.SiteOper, sc *chain.SiteChain, reg *site.Registry, hc *httpclient.Client) *SiteHandler {
	return &SiteHandler{oper: o, siteCh: sc, reg: reg, hc: hc}
}

type inspectReq struct {
	URL     string                       `json:"url" binding:"required,max=512"`
	Cookie  string                       `json:"cookie" binding:"max=65536"`
	Fields  map[string]site.ElementQuery `json:"fields" binding:"dive"`
	NoLogin site.NoLoginDef              `json:"no_login"`
}

// Inspect 校准诊断：给定 URL+cookie+一组待试选择器，返回每个选择器抓到的文本 +
// 是否命中登录态 + 页面前 2KB。用于离线/在线校准 definitions/*.yaml 的 selectors。
func (h *SiteHandler) Inspect(c *gin.Context) {
	var req inspectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	res, err := site.Inspect(c.Request.Context(), h.hc, req.URL, req.Cookie, req.Fields, req.NoLogin)
	if err != nil {
		// 502：目标站点抓取失败，非客户端参数错误。
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, res)
}

// List 列出全部站点。
func (h *SiteHandler) List(c *gin.Context) {
	sites, err := h.oper.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"sites": sites})
}

// Get 取单个站点。
func (h *SiteHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	s, err := h.oper.Get(id)
	if err != nil {
		errcode.Respond(c, errcode.SiteNotFound)
		return
	}
	c.JSON(http.StatusOK, s)
}

// Create 创建站点。
func (h *SiteHandler) Create(c *gin.Context) {
	var in oper.SiteInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	s, err := h.oper.Create(in)
	if err != nil {
		// 同注册域站点已存在是用户可自行处理的冲突（409 带明确文案），
		// 其余（库故障等）才归 internal——避免「重复添加」被误报成服务内部错误。
		if errors.Is(err, oper.ErrDuplicateSite) {
			errcode.Respond(c, errcode.SiteDuplicate.WithErr(err))
			return
		}
		if errors.Is(err, oper.ErrInvalidBaseURL) {
			errcode.Respond(c, errcode.InvalidInput.WithErr(err))
			return
		}
		slog.Error("site create failed", "base_url", in.BaseURL, "err", err)
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// 站点限流热生效：新建站点（或改了 LimitInterval）后把 per-host 间隔同步到抓取客户端。
	if all, err := h.oper.List(); err == nil {
		h.siteCh.SyncHostLimits(all, h.hc)
	}
	c.JSON(http.StatusOK, s)
}

// Update 更新站点（密钥为空则保留原值）。
func (h *SiteHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.SiteInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	s, err := h.oper.Update(id, in)
	if err != nil {
		// 站点已被其他会话删除后再提交编辑 → 404，而非误报服务内部错误（500）。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			errcode.Respond(c, errcode.SiteNotFound)
			return
		}
		if errors.Is(err, oper.ErrDuplicateSite) {
			errcode.Respond(c, errcode.SiteDuplicate.WithErr(err))
			return
		}
		if errors.Is(err, oper.ErrInvalidBaseURL) {
			errcode.Respond(c, errcode.InvalidInput.WithErr(err))
			return
		}
		slog.Error("site update failed", "id", id, "err", err)
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// 站点限流热生效：LimitInterval 改动即时同步到抓取客户端（无需重启）。
	if all, err := h.oper.List(); err == nil {
		h.siteCh.SyncHostLimits(all, h.hc)
	}
	c.JSON(http.StatusOK, s)
}

// Delete 删除站点。
func (h *SiteHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	// 四轮 P2 修复：站点硬删不可恢复（Cookie/APIKey/Passkey 凭据随之失，仅能靠
	// 事先导出的备份），加 ConfirmRisk 二次确认（与运维破坏性操作对齐）。
	var in struct {
		ConfirmRisk bool `json:"confirm_risk"`
	}
	_ = c.ShouldBindJSON(&in)
	if !in.ConfirmRisk {
		errcode.Respond(c, errcode.RiskConfirmationRequired)
		return
	}
	if err := h.oper.Delete(id); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// 被删站点的 per-host 限流残留会导致抓取客户端仍按旧间隔节流：删除后重算剩余站点。
	if all, err := h.oper.List(); err == nil {
		h.siteCh.SyncHostLimits(all, h.hc)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// BatchDelete 批量删除站点（admin 专属）。返回删除成功数与失败 id 列表（不存在的）。
func (h *SiteHandler) BatchDelete(c *gin.Context) {
	var body struct {
		IDs         []uint `json:"ids" binding:"required,min=1,max=200,dive"`
		ConfirmRisk bool   `json:"confirm_risk"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	// 四轮 P2 修复：批量硬删（一次最多 200 站）要求 ConfirmRisk（同 Delete）。
	if !body.ConfirmRisk {
		errcode.Respond(c, errcode.RiskConfirmationRequired)
		return
	}
	failed, err := h.oper.DeleteMany(body.IDs)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if failed == nil {
		failed = []uint{}
	}
	// 与 Delete 同理：批量删除后重算剩余站点，清理残留 per-host 限流。
	if all, err := h.oper.List(); err == nil {
		h.siteCh.SyncHostLimits(all, h.hc)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": len(body.IDs) - len(failed), "failed": failed})
}

// Test 测试站点连接与登录态。
func (h *SiteHandler) Test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	s, err := h.oper.Get(id)
	if err != nil {
		errcode.Respond(c, errcode.SiteNotFound)
		return
	}
	level, err := h.siteCh.TestConnection(c.Request.Context(), s)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "level": level})
}

// Status 返回全部站点的上次联通状态缓存（瞬时读、不发网络请求）。
// 进页面 GET 此接口瞬时回显「上次检测结果」；后台逐站实时检测走 GET /sites/status/stream（SSE）。
// 响应逐条映射为与 SSE 帧（chain.SiteConnectionResult）一致的前端结构：
// error←error_msg、补 site_name（缓存表无站点名），否则运营矩阵「查看原因」与
// 站点详情页离线文案在缓存回显时读不到具体错误（两处均读 status.error）。
func (h *SiteHandler) Status(c *gin.Context) {
	list, err := h.oper.ListConnectivity()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	names := map[uint]string{}
	if sites, err := h.oper.List(); err == nil {
		for _, s := range sites {
			names[s.ID] = s.Name
		}
	}
	statuses := make([]gin.H, 0, len(list))
	for _, r := range list {
		statuses = append(statuses, gin.H{
			"site_id":     r.SiteID,
			"site_name":   names[r.SiteID],
			"status":      r.Status,
			"level":       r.Level,
			"error":       r.ErrorMsg,
			"success":     r.Success,
			"fail":        r.Fail,
			"avg_seconds": r.AvgSeconds,
			"checked_at":  r.CheckedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"statuses": statuses})
}

// StatusStream SSE 流式逐站检测：每站完成推一帧 event:status，结束推 event:done。
// 供站点运营矩阵/Dashboard「检测到一个更新一个」；每站结果顺带 upsert 进 site_connectivity 缓存。
// 完全照搬 SearchAllStream 的 beginSSE + chain 流式回调 + done 收尾模式。
func (h *SiteHandler) StatusStream(c *gin.Context) {
	emit := beginSSE(c.Writer)
	h.siteCh.TestConnectionsStream(c.Request.Context(), func(r chain.SiteConnectionResult) {
		emit("status", r)
	})
	emit("done", gin.H{})
}

// Checkin 立即对单个站点签到（站点行「签到」按钮）。返回适配器签到结果；
// 适配器不支持签到（如 mteam）或网络失败时 success=false 并带 error，前端据此提示。
func (h *SiteHandler) Checkin(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	s, err := h.oper.Get(id)
	if err != nil {
		errcode.Respond(c, errcode.SiteNotFound)
		return
	}
	r := chain.CheckinSiteResult{SiteID: s.ID, SiteName: s.Name}
	if res, e := h.siteCh.Checkin(c.Request.Context(), s, "manual"); e != nil {
		r.Error = e.Error()
	} else if res != nil {
		r.Success = res.Success
		r.Message = res.Message
		r.Bonus = res.Bonus
	}
	c.JSON(http.StatusOK, r)
}

// CheckinAll 一键签到全部启用站点（顶部「一键签到」按钮），并发执行，返回每站结果。
func (h *SiteHandler) CheckinAll(c *gin.Context) {
	results := h.siteCh.CheckinAll(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// CheckinRecords 列签到历史记录（?site_id=&limit=&offset=；按时间倒序）。
func (h *SiteHandler) CheckinRecords(c *gin.Context) {
	siteID, _ := strconv.ParseUint(c.Query("site_id"), 10, 64)
	limit := atoiDefault(c.Query("limit"), 100)
	offset := atoiDefault(c.Query("offset"), 0)
	records, err := h.siteCh.ListCheckinRecords(uint(siteID), limit, offset)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": records})
}

// ---- 共享解析辅助（包内多 handler 复用）----

func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.SiteInvalidID)
		return 0, false
	}
	return uint(id), true
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// parseUintSlice 解析逗号分隔的 uint 列表（如 "1,2,3"），忽略空串/非法项；空输入返回 nil。
func parseUintSlice(s string) []uint {
	if s == "" {
		return nil
	}
	out := make([]uint, 0, 4)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, uint(n))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
