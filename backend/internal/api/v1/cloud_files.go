package v1

// 云盘文件管理器 / 云任务队列 / 115 开放授权 / 直链 / 记录 端点。
// 从 cloud.go 迁出;全部转发 CloudChain 服务层。

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/cloud"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
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

// cloudOpErr 云盘操作出口的失败分档。判据沿用本域既有的 respondRecordError
// （盘不存在⇒404、oper 自产的输入原因⇒400 且带那句话、其余⇒兜底），
// 只在它之上补两档「重试永远不会有用、要去配置而不是改参数」的处境。
// 从前这四条出口一律 400「请求参数有误，请检查后重试」，而 115 的 auth/poll 是纯上游调用、
// 整理缺的是媒体识别配置——用户没有任何输入可检查。
func cloudOpErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, chain.ErrCloudTokenUnreadable):
		// WithDetail：链层那句「去云盘里重新保存凭据、本次没发请求」原样抵达界面。
		errcode.Respond(c, errcode.CloudTokenUnreadable.WithDetail(err))
	case errors.Is(err, chain.ErrCloudMediaSearchUnconfigured):
		errcode.Respond(c, errcode.MediaSearchUnconfigured.WithDetail(err))
	default:
		respondRecordError(c, err, errcode.CloudDriveNotFound, errcode.InvalidInput, errcode.CloudOpUpstreamFailed)
	}
}

// cloudOrganizeReq 是 /cloud/organize 的 HTTP 入参。
//
// 为什么要在边界再搭一层：cloud.OrganizeOptions 是链层/任务队列共用的结构体，**没有 json tag**，
// 而前端按约定发 snake_case（drive_id / src_dir / rename）。encoding/json 对 "drive_id" 与
// 字段 DriveID 既不相等、大小写折叠后也不相等（多个下划线），于是过去所有键一律绑不上，
// 整理请求永远变成「整理 0 号盘」⇒ 404「云盘账号不存在」，这个功能从界面根本进不去。
// 给 OrganizeOptions 直接加 tag 会连带动到任务队列已落库的参数串，所以只在边界翻译。
type cloudOrganizeReq struct {
	DriveID       uint   `json:"drive_id" binding:"required"`
	SrcDir        string `json:"src_dir"`
	DstMovieDir   string `json:"dst_movie_dir"`
	DstTVDir      string `json:"dst_tv_dir"`
	MediaOnly     *bool  `json:"media_only"`
	KeepStructure *bool  `json:"keep_structure"`
	Rename        *bool  `json:"rename"`
	Max           int    `json:"max"`
}

// toOptions 把边界值翻成链层参数；指针字段留空＝沿用链层默认（不要在这里替用户猜默认值）。
func (r cloudOrganizeReq) toOptions() cloud.OrganizeOptions {
	in := cloud.OrganizeOptions{
		DriveID: r.DriveID, SrcDir: r.SrcDir,
		DstMovieDir: r.DstMovieDir, DstTVDir: r.DstTVDir,
		MediaOnly: r.MediaOnly == nil || *r.MediaOnly, Max: r.Max,
	}
	if r.KeepStructure != nil {
		in.KeepStructure = *r.KeepStructure
	}
	in.Rename = r.Rename == nil || *r.Rename
	return in
}

// OrganizeCloud POST /cloud/organize：云端整理（云盘文件识别→归位到分类目录）。
// 大目录建议走任务队列（kind=organize 的入队由 CloudTask 执行器串行消费）。
func (h *CloudHandler) OrganizeCloud(c *gin.Context) {
	var req cloudOrganizeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	res, err := h.svc.OrganizeCloud(c.Request.Context(), req.toOptions())
	if err != nil {
		cloudOpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// RepairStrm POST /cloud/strm-repair {drive_id?}：按当前 AppBaseURL/模板批量重写 .strm。
func (h *CloudHandler) RepairStrm(c *gin.Context) {
	var in struct {
		DriveID uint `json:"drive_id"`
	}
	// 空 body（io.EOF）= 修全部盘（合法用法，前端「全部修复」传 drive_id:0）；
	// 坏 JSON 不能静默落成 drive_id=0——那等于把「只想修一个盘」的请求放大成全盘重写。
	if err := c.ShouldBindJSON(&in); err != nil && !errors.Is(err, io.EOF) {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	res, err := h.svc.RepairStrm(c.Request.Context(), in.DriveID)
	if err != nil {
		cloudOpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
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
		// 这一步只跟 115 说话，没有任何「参数」可检查；app_id 不对是 115 给的答复。
		cloudOpErr(c, err)
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
		cloudOpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"confirmed": confirmed, "status": status, "token": tokenJSON})
}

// ---- 云端任务队列 ----

// ListTasks 任务列表（query: status/limit）。
func (h *CloudHandler) ListTasks(c *gin.Context) {
	// 请求里的 limit 可能被归一（留空/非法/超上限），必须回显生效值，否则界面以为拿到了自己要的条数。
	reqLimit, _ := strconv.Atoi(c.Query("limit"))
	limit := oper.NormalizeCloudTaskLimit(reqLimit)
	ts, err := h.svc.CloudTasks(c.Query("status"), limit)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if ts == nil {
		ts = []models.CloudTask{}
	}
	// total/truncated 与清单同批给出；计数读不出时保守报「可能被截断」且不编一个假 total
	//（口径同 keepseed / RSS 条目列表）。
	total, cntErr := h.svc.CountCloudTasks(c.Query("status"))
	truncated := cntErr != nil || total > int64(len(ts))
	if cntErr != nil {
		slog.Warn("cloud task count failed", "err", cntErr)
	}
	resp := gin.H{"tasks": ts, "limit": limit, "truncated": truncated}
	if cntErr == nil {
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
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
// 直链签名：.strm 渲染时追加 ?sign=<exp>.<hmac>，此处校验（URL 即凭证，防匿名枚举
// driveId+路径换全盘直链）。签名密钥未注入时为兼容模式不校验（与旧版一致）。
func (h *CloudHandler) Direct(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("driveId"), 10, 64)
	if err != nil || id == 0 {
		errcode.Respond(c, errcode.InvalidID)
		return
	}
	if !h.svc.VerifyDirectLinkSign(uint(id), c.Param("path"), c.Query("sign")) {
		// 签名无效/过期/缺失与记录不存在同口径 404：公开端点不泄露区别。
		errcode.Respond(c, errcode.NotFound.WithErr(fmt.Errorf("direct link sign mismatch")))
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
	if err := h.svc.DeleteRecord(id); err != nil {
		// 从前是 200 + {ok:false,error}：Web 与移动端都不读 .ok，等于把失败报成成功。
		respondRecordError(c, err, errcode.StrmRecordNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// respondCloudErr 把 oper 错误映射到状态码：输入校验（类型非法）→ 400，其余 → 500。
