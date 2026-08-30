package v1

// 云盘文件管理器 / 云任务队列 / 115 开放授权 / 直链 / 记录 端点。
// 从 cloud.go 迁出;全部转发 CloudChain 服务层。

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/cloud"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// ---- 云端文件管理器 ----

// fileOpReq 文件操作通用请求。
type fileOpReq struct {
	DriveID uint   `json:"drive_id" binding:"required"`
	Path    string `json:"path" binding:"required"`
	DstDir  string `json:"dst_dir"`
	NewName string `json:"new_name"`
	URL     string `json:"url"`
	Pwd     string `json:"pwd"`
}

// CloudMkdir POST /cloud/files/mkdir {drive_id, path(父), new_name}。
func (h *CloudHandler) CloudMkdir(c *gin.Context) {
	var in fileOpReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	opResult(c, h.svc.CloudMkdir(c.Request.Context(), in.DriveID, in.Path, in.NewName))
}

// CloudMove POST /cloud/files/move。
func (h *CloudHandler) CloudMove(c *gin.Context) {
	var in fileOpReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	opResult(c, h.svc.CloudMove(c.Request.Context(), in.DriveID, in.Path, in.DstDir))
}

// CloudCopy POST /cloud/files/copy。
func (h *CloudHandler) CloudCopy(c *gin.Context) {
	var in fileOpReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	opResult(c, h.svc.CloudCopy(c.Request.Context(), in.DriveID, in.Path, in.DstDir))
}

// CloudRename POST /cloud/files/rename。
func (h *CloudHandler) CloudRename(c *gin.Context) {
	var in fileOpReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	opResult(c, h.svc.CloudRename(c.Request.Context(), in.DriveID, in.Path, in.NewName))
}

// CloudDelete POST /cloud/files/delete。
func (h *CloudHandler) CloudDelete(c *gin.Context) {
	var in fileOpReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	opResult(c, h.svc.CloudDelete(c.Request.Context(), in.DriveID, in.Path))
}

// CloudOffline POST /cloud/files/offline {drive_id, path(目标目录), url}。
func (h *CloudHandler) CloudOffline(c *gin.Context) {
	var in fileOpReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	opResult(c, h.svc.CloudOfflineAdd(c.Request.Context(), in.DriveID, in.Path, in.URL))
}

// CloudShare POST /cloud/files/share-save {drive_id, path(目标目录), url(分享链接), pwd}。
func (h *CloudHandler) CloudShare(c *gin.Context) {
	var in fileOpReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	opResult(c, h.svc.CloudSaveShare(c.Request.Context(), in.DriveID, in.Path, in.URL, in.Pwd))
}

// OrganizeCloud POST /cloud/organize：云端整理（云盘文件识别→归位到分类目录）。
// 大目录建议走任务队列（kind=organize 的入队由 CloudTask 执行器串行消费）。
func (h *CloudHandler) OrganizeCloud(c *gin.Context) {
	var in cloud.OrganizeOptions
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	res, err := h.svc.OrganizeCloud(c.Request.Context(), in)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, res)
}

// RepairStrm POST /cloud/strm-repair {drive_id?}：按当前 AppBaseURL/模板批量重写 .strm。
func (h *CloudHandler) RepairStrm(c *gin.Context) {
	var in struct {
		DriveID uint `json:"drive_id"`
	}
	_ = c.ShouldBindJSON(&in)
	repaired, skipped, err := h.svc.RepairStrm(c.Request.Context(), in.DriveID)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"repaired": repaired, "skipped": skipped})
}

// ---- 115 开放平台设备码授权 ----

// Start115OpenAuth 发起授权（POST /cloud/115open/auth {app_id}）。返回扫码 URL 与
// 轮询参数（uid/time/sign + verifier，前端暂存供 poll）。
func (h *CloudHandler) Start115OpenAuth(c *gin.Context) {
	var in struct {
		AppID string `json:"app_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	code, verifier, err := h.svc.Start115OpenAuth(c.Request.Context(), in.AppID)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"uid": code.UID.String(), "time": code.Time.String(),
		"qrcode": code.QRCode, "sign": code.Sign, "verifier": verifier,
	})
}

// Poll115OpenAuth 轮询授权（POST /cloud/115open/poll）。confirmed=true 时返回 token
// （TokenPair JSON），前端可用它直接建盘（type=115_open）或提示回写。
func (h *CloudHandler) Poll115OpenAuth(c *gin.Context) {
	var in struct {
		UID      string `json:"uid" binding:"required"`
		Time     string `json:"time" binding:"required"`
		Sign     string `json:"sign" binding:"required"`
		Verifier string `json:"verifier" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	code := &cloud.DeviceCode115{
		UID: json.Number(in.UID), Time: json.Number(in.Time), Sign: in.Sign,
	}
	confirmed, status, tokenJSON, err := h.svc.Poll115OpenAuth(c.Request.Context(), code, in.Verifier)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"confirmed": confirmed, "status": status, "token": tokenJSON})
}

// ---- 云端任务队列 ----

// ListTasks 任务列表（query: status/limit）。
func (h *CloudHandler) ListTasks(c *gin.Context) {
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	ts, err := h.svc.CloudTasks(c.Query("status"), limit)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if ts == nil {
		ts = []models.CloudTask{}
	}
	c.JSON(http.StatusOK, gin.H{"tasks": ts})
}

// RetryTask 重试失败任务。
func (h *CloudHandler) RetryTask(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	opResult(c, h.svc.RetryCloudTask(id))
}

// DeleteTask 删除任务记录。
func (h *CloudHandler) DeleteTask(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	opResult(c, h.svc.DeleteCloudTask(id))
}

// ImportTree 目录树批量生成 STRM（115 导出目录树文本）：入队 import_tree 任务。
func (h *CloudHandler) ImportTree(c *gin.Context) {
	var in struct {
		DriveID   uint   `json:"drive_id" binding:"required"`
		Tree      string `json:"tree" binding:"required"`
		Base      string `json:"base"`
		OutputDir string `json:"output_dir" binding:"required"`
		Template  string `json:"template"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"tree": in.Tree, "base": in.Base, "output_dir": in.OutputDir, "template": in.Template,
	})
	if err := h.svc.EnqueueCloudTask(&models.CloudTask{
		DriveID: in.DriveID, Kind: "import_tree", Payload: string(payload),
	}); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"enqueued": true})
}

// Direct STRM 302 直链（公开：GET /cloud/direct/:driveId/*path——播放器不能带 JWT）。
// 把 .strm 内容里的本端 URL 解析为网盘限时直链并 302。夸克/115 直链与 UA/Cookie 绑定，
// 下载时播放器需携带响应头中返回的 Header（此处仅 302，Emby/播放器侧由直链服务保证）。
func (h *CloudHandler) Direct(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("driveId"), 10, 64)
	if err != nil || id == 0 {
		errcode.Respond(c, errcode.InvalidID)
		return
	}
	link, err := h.svc.ResolveDirectLink(c.Request.Context(), uint(id), c.Param("path"), c.Request.UserAgent())
	if err != nil {
		// 记录不存在/解析失败统一 404：公开端点不泄露内部细节。
		errcode.Respond(c, errcode.NotFound.WithErr(err))
		return
	}
	c.Redirect(http.StatusFound, link.URL)
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
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
