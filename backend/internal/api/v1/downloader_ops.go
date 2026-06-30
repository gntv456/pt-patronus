package v1

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/downloader"
)

// parseLimit 解析 bytes/s 限速字段：空串→0（不限速）；非法/负数→error。
// 关键：禁止把无效输入静默当作 0（解除限速）执行。
func parseLimit(v string) (int64, error) {
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", v)
	}
	if n < 0 {
		return 0, fmt.Errorf("negative limit %q", v)
	}
	return n, nil
}

// opResult 统一处理写操作结果：成功 {ok:true}；失败 {ok:false,error,code?}。
// ErrNotImplemented 映射 code=not_implemented，前端据此隐藏对应 UI。
func opResult(c *gin.Context, err error) {
	if err != nil {
		resp := gin.H{"ok": false, "error": err.Error()}
		if errors.Is(err, downloader.ErrNotImplemented) {
			resp["code"] = "not_implemented"
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// UploadTorrent 上传 .torrent 文件添加种子（multipart：file + 可选表单字段）。
func (h *DownloaderHandler) UploadTorrent(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required: " + err.Error()})
		return
	}
	if file.Size > 8<<20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 8MB)"})
		return
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	opts := downloader.AddOptions{
		FileContent: data,
		FileName:    file.Filename,
		Category:    c.PostForm("category"),
		SavePath:    c.PostForm("save_path"),
		Paused:      c.PostForm("paused") == "true",
	}
	if t := c.PostForm("tags"); t != "" {
		opts.Tags = splitCSV(t)
	}
	if n, err := parseLimit(c.PostForm("upload_limit")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload_limit: " + err.Error()})
		return
	} else {
		opts.UploadLimit = n
	}
	if n, err := parseLimit(c.PostForm("download_limit")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "download_limit: " + err.Error()})
		return
	} else {
		opts.DownloadLimit = n
	}
	opResult(c, h.dl.Add(c.Request.Context(), id, opts))
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// —— 分类 / 标签 ——

// Categories 列出可用分类。
func (h *DownloaderHandler) Categories(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	cats, err := h.dl.ListCategories(c.Request.Context(), id)
	if err != nil {
		opResult(c, err) // TR 返回 not_implemented
		return
	}
	c.JSON(http.StatusOK, gin.H{"categories": cats})
}

// Tags 列出可用标签。
func (h *DownloaderHandler) Tags(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	tags, err := h.dl.ListTags(c.Request.Context(), id)
	if err != nil {
		opResult(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

type categoryReq struct {
	Hashes   []string `json:"hashes"`
	Category string   `json:"category"`
}

// SetCategoryOp 设置分类。
func (h *DownloaderHandler) SetCategoryOp(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req categoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetCategory(c.Request.Context(), id, req.Hashes, req.Category))
}

type tagsReq struct {
	Hashes []string `json:"hashes"`
	Tags   []string `json:"tags"`
	Mode   string   `json:"mode"` // set | add | remove
}

// TagsOp 标签操作（按 mode 分发）。
func (h *DownloaderHandler) TagsOp(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req tagsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	var err error
	switch req.Mode {
	case "add":
		err = h.dl.AddTags(ctx, id, req.Hashes, req.Tags)
	case "remove":
		err = h.dl.RemoveTags(ctx, id, req.Hashes, req.Tags)
	default: // set
		err = h.dl.SetTags(ctx, id, req.Hashes, req.Tags)
	}
	opResult(c, err)
}

// —— 生命周期 ——

// Recheck 强制重新校验。
func (h *DownloaderHandler) Recheck(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.Recheck(c.Request.Context(), id, req.Hashes))
}

// Reannounce 重新汇报。
func (h *DownloaderHandler) Reannounce(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.Reannounce(c.Request.Context(), id, req.Hashes))
}

type locationReq struct {
	Hashes   []string `json:"hashes"`
	Location string   `json:"location"`
}

// SetLocation 移动保存路径。
func (h *DownloaderHandler) SetLocation(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req locationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetLocation(c.Request.Context(), id, req.Hashes, req.Location))
}

// —— 限速 ——

type limitsReq struct {
	Hashes        []string `json:"hashes"`
	UploadLimit   int64    `json:"upload_limit"`
	DownloadLimit int64    `json:"download_limit"`
}

// SetLimits 设置单/多任务限速。
func (h *DownloaderHandler) SetLimits(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req limitsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetLimits(c.Request.Context(), id, req.Hashes, downloader.Limits{
		UploadLimit: req.UploadLimit, DownloadLimit: req.DownloadLimit,
	}))
}

// GetSpeedLimits 读取全局速度上限。
func (h *DownloaderHandler) GetSpeedLimits(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	lim, err := h.dl.GetSpeedLimits(c.Request.Context(), id)
	if err != nil {
		opResult(c, err)
		return
	}
	c.JSON(http.StatusOK, lim)
}

type speedLimitsReq struct {
	UploadLimit   int64 `json:"upload_limit"`
	DownloadLimit int64 `json:"download_limit"`
}

// SetSpeedLimits 设置全局速度上限。
func (h *DownloaderHandler) SetSpeedLimits(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req speedLimitsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetSpeedLimits(c.Request.Context(), id, downloader.Limits{
		UploadLimit: req.UploadLimit, DownloadLimit: req.DownloadLimit,
	}))
}

// —— 文件优先级 ——

// Files 列出某任务内文件。
func (h *DownloaderHandler) Files(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	hash := c.Query("hash")
	if hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash required"})
		return
	}
	files, err := h.dl.ListFiles(c.Request.Context(), id, hash)
	if err != nil {
		opResult(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

type filePrioReq struct {
	Hash     string `json:"hash"`
	Indices  []int  `json:"indices"`
	Priority int    `json:"priority"`
}

// SetFilePriority 设置文件优先级。
func (h *DownloaderHandler) SetFilePriority(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req filePrioReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetFilePriority(c.Request.Context(), id, req.Hash, req.Indices, req.Priority))
}

// —— Tracker ——

// Trackers 列出某任务的 tracker。
func (h *DownloaderHandler) Trackers(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	hash := c.Query("hash")
	if hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash required"})
		return
	}
	trk, err := h.dl.ListTrackers(c.Request.Context(), id, hash)
	if err != nil {
		opResult(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"trackers": trk})
}

type trackersReq struct {
	Hash string   `json:"hash"`
	URLs []string `json:"urls"`
}

// AddTrackers 追加 tracker。
func (h *DownloaderHandler) AddTrackers(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req trackersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.AddTrackers(c.Request.Context(), id, req.Hash, req.URLs))
}

// RemoveTrackers 移除 tracker。
func (h *DownloaderHandler) RemoveTrackers(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req trackersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.RemoveTrackers(c.Request.Context(), id, req.Hash, req.URLs))
}

// —— 做种限制 ——

type shareLimitsReq struct {
	Hashes           []string `json:"hashes"`
	RatioLimit       float64  `json:"ratio_limit"`        // -2=无限 -1=全局默认 >=0=值
	SeedingTimeLimit int      `json:"seeding_time_limit"` // 分钟，同上语义
}

// SetShareLimits 设置分享比率/做种时长上限。
func (h *DownloaderHandler) SetShareLimits(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req shareLimitsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetShareLimits(c.Request.Context(), id, req.Hashes, downloader.ShareLimits{
		RatioLimit:       req.RatioLimit,
		SeedingTimeLimit: req.SeedingTimeLimit,
	}))
}

// —— 队列优先级 ——

type queueReq struct {
	Hashes []string `json:"hashes"`
	Action string   `json:"action"` // top|up|down|bottom
}

// SetQueuePriority 调整队列优先级。
func (h *DownloaderHandler) SetQueuePriority(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req queueReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetQueuePriority(c.Request.Context(), id, req.Hashes, req.Action))
}

// —— 强制开始 ——

type forceStartReq struct {
	Hashes []string `json:"hashes"`
	Enable bool     `json:"enable"`
}

// SetForceStart 强制开始/取消强制。
func (h *DownloaderHandler) SetForceStart(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req forceStartReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetForceStart(c.Request.Context(), id, req.Hashes, req.Enable))
}

// —— 顺序下载 / 首尾优先 ——

// ToggleSequential 切换顺序下载。
func (h *DownloaderHandler) ToggleSequential(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.ToggleSequentialDownload(c.Request.Context(), id, req.Hashes))
}

// ToggleFirstLast 切换首尾优先。
func (h *DownloaderHandler) ToggleFirstLast(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.ToggleFirstLastPiece(c.Request.Context(), id, req.Hashes))
}

// —— Peers ——

// Peers 列出某任务当前连接的对等体。
func (h *DownloaderHandler) Peers(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	hash := c.Query("hash")
	if hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash required"})
		return
	}
	peers, err := h.dl.ListPeers(c.Request.Context(), id, hash)
	if err != nil {
		opResult(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"peers": peers})
}

// —— 分类增删 ——

type categoryEditReq struct {
	Name     string `json:"name"`
	SavePath string `json:"save_path"`
}

// CreateCategory 新建分类。
func (h *DownloaderHandler) CreateCategory(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req categoryEditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.CreateCategory(c.Request.Context(), id, req.Name, req.SavePath))
}

// EditCategory 编辑分类保存路径。
func (h *DownloaderHandler) EditCategory(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req categoryEditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.EditCategory(c.Request.Context(), id, req.Name, req.SavePath))
}

type deleteCategoriesReq struct {
	Names []string `json:"names"`
}

// DeleteCategories 删除分类。
func (h *DownloaderHandler) DeleteCategories(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req deleteCategoriesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.DeleteCategories(c.Request.Context(), id, req.Names))
}
