package v1

import (
	"errors"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
)

// DownloaderHandler 处理下载器 CRUD 与任务操作。
type DownloaderHandler struct {
	op     *oper.DownloaderOper
	dl     *chain.DownloadChain
	mgr    *manager.Manager
	siteOp *oper.SiteOper // 手动加种 URL 反查站点用（cookie 下载 .torrent）
}

// NewDownloaderHandler 构造。
func NewDownloaderHandler(op *oper.DownloaderOper, dl *chain.DownloadChain, mgr *manager.Manager, siteOp *oper.SiteOper) *DownloaderHandler {
	return &DownloaderHandler{op: op, dl: dl, mgr: mgr, siteOp: siteOp}
}

// Caps 返回各下载器类型的能力位矩阵（后端权威，供前端按类型门控 UI 控件、避免显示后端
// 会 ErrNotImplemented 的按钮）。GET /downloaders/caps。
func (h *DownloaderHandler) Caps(c *gin.Context) {
	c.JSON(http.StatusOK, downloader.AllCapabilities())
}

// List 列出全部下载器。
func (h *DownloaderHandler) List(c *gin.Context) {
	ds, err := h.op.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"downloaders": ds})
}

// Get 取单个下载器详情。
func (h *DownloaderHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	d, err := h.op.Get(id)
	if err != nil {
		errcode.Respond(c, errcode.DownloaderNotFound.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"downloader": d})
}

// Create 创建下载器。
func (h *DownloaderHandler) Create(c *gin.Context) {
	var in oper.DownloaderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	d, err := h.op.Create(in)
	if err != nil {
		if errors.Is(err, oper.ErrInvalidInput) {
			errcode.Respond(c, errcode.DownloaderInvalidInput.WithErr(err))
		} else {
			errcode.Respond(c, errcode.Internal.WithErr(err))
		}
		return
	}
	c.JSON(http.StatusOK, d)
}

// Update 更新下载器（失效缓存）。
func (h *DownloaderHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.DownloaderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	d, err := h.op.Update(id, in)
	if err != nil {
		if errors.Is(err, oper.ErrInvalidInput) {
			errcode.Respond(c, errcode.DownloaderInvalidInput.WithErr(err))
		} else {
			errcode.Respond(c, errcode.Internal.WithErr(err))
		}
		return
	}
	h.mgr.Invalidate(id)
	c.JSON(http.StatusOK, d)
}

// Delete 删除下载器。
func (h *DownloaderHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.op.Delete(id); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	h.mgr.Invalidate(id)
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Test 连接测试。
func (h *DownloaderHandler) Test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.dl.Test(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Torrents 列出任务（支持 category/state/tag/q 过滤 + limit/offset 分页 + sort/order 排序）。
// 不传 query 时返回全部（仅多 total/limit/offset 元字段），向后兼容。
func (h *DownloaderHandler) Torrents(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ts, err := h.dl.List(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"torrents": []any{}, "total": 0, "error": err.Error()})
		return
	}
	ts = filterTorrents(ts, c)
	sortTorrents(ts, c)
	total := len(ts)
	limit := clampQueryInt(c, "limit", 200, 1, 1000)
	offset := clampQueryInt(c, "offset", 0, 0, total)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	c.JSON(http.StatusOK, gin.H{"torrents": ts[offset:end], "total": total, "limit": limit, "offset": offset})
}

// clampQueryInt 解析 query 整数，越界钳到 [lo,hi]。
func clampQueryInt(c *gin.Context, key string, def, lo, hi int) int {
	v := atoiDefault(c.Query(key), def)
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// torrentStateGroup 把各下载器原始 state 归并为前端分组（对齐前端 torrentTableFilters.stateGroup）。
// qB 的做种族 uploading/stalledUP/queuedUP/forcedUP 以 UP 后缀识别（stalledUP 里无 upload/seed 字面量，
// 旧实现归到 other → 下载器页做种筛选漏掉绝大多数做种种子）；DL 后缀同理归下载族。
// checking（校验中）/moving（移动中）先排除再匹配后缀；仅字面 pause|stop 判暂停。
func torrentStateGroup(s string) string {
	low := strings.ToLower(s)
	switch {
	case strings.Contains(low, "error") || strings.Contains(low, "missing"):
		return "error"
	case strings.Contains(low, "pause") || strings.Contains(low, "stop"):
		return "paused"
	case strings.Contains(low, "check") || strings.Contains(low, "moving"):
		return "other"
	case strings.HasSuffix(low, "up") || strings.Contains(low, "upload") || strings.Contains(low, "seed"):
		return "seeding"
	case strings.HasSuffix(low, "dl") || strings.Contains(low, "download") || strings.Contains(low, "leech") || strings.Contains(low, "meta"):
		return "downloading"
	default:
		return "other"
	}
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// filterTorrents 按 category/state/tag/q 内存过滤（种子量级千以内可接受）。
func filterTorrents(ts []downloader.Torrent, c *gin.Context) []downloader.Torrent {
	cat := c.Query("category")
	st := c.Query("state")
	tag := c.Query("tag")
	q := strings.ToLower(c.Query("q"))
	if cat == "" && (st == "" || st == "all") && tag == "" && q == "" {
		return ts
	}
	out := make([]downloader.Torrent, 0, len(ts))
	for _, t := range ts {
		if cat != "" && t.Category != cat {
			continue
		}
		if st != "" && st != "all" && torrentStateGroup(t.State) != st {
			continue
		}
		if tag != "" && !containsString(t.Tags, tag) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.Name), q) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func cmpInt64(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func cmpFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// sortTorrents 按 sort/order 原地排序（默认 added_at desc）；用 cmp int 避免违反严格弱序。
func sortTorrents(ts []downloader.Torrent, c *gin.Context) {
	key := c.DefaultQuery("sort", "added_at")
	desc := c.DefaultQuery("order", "desc") != "asc"
	cmp := func(a, b downloader.Torrent) int {
		switch key {
		case "name":
			return strings.Compare(a.Name, b.Name)
		case "size":
			return cmpInt64(a.Size, b.Size)
		case "progress":
			return cmpFloat(a.Progress, b.Progress)
		case "ratio":
			return cmpFloat(a.Ratio, b.Ratio)
		case "dl_rate":
			return cmpInt64(a.DLRate, b.DLRate)
		case "up_rate":
			return cmpInt64(a.UPRate, b.UPRate)
		default:
			return cmpInt64(a.AddedAt.UnixNano(), b.AddedAt.UnixNano())
		}
	}
	sort.Slice(ts, func(i, j int) bool {
		r := cmp(ts[i], ts[j])
		if desc {
			return r > 0
		}
		return r < 0
	})
}

type addReq struct {
	URL           string   `json:"url" binding:"required,max=2048"` // magnet 链较长，给宽上界
	Category      string   `json:"category" binding:"max=128"`
	Tags          []string `json:"tags" binding:"dive,max=128"`
	SavePath      string   `json:"save_path" binding:"max=512"`
	Paused        bool     `json:"paused"`
	UploadLimit   int64    `json:"upload_limit"`   // bytes/s, 0 不限
	DownloadLimit int64    `json:"download_limit"` // bytes/s, 0 不限
}

// Add 添加种子。
func (h *DownloaderHandler) Add(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req addReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	opts := downloader.AddOptions{
		URL: req.URL, Category: req.Category, Tags: req.Tags, SavePath: req.SavePath, Paused: req.Paused,
		UploadLimit: req.UploadLimit, DownloadLimit: req.DownloadLimit,
	}
	// 手动加种：URL 可能来自 PT 站（需 cookie 下载），反查站点设 SiteID 让 DownloadChain.Add 处理。
	if h.siteOp != nil {
		if st, _ := h.siteOp.FindByDownloadURL(req.URL); st != nil {
			opts.SiteID = st.ID
		}
	}
	err := h.dl.Add(c.Request.Context(), id, opts)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type hashesReq struct {
	Hashes      []string `json:"hashes" binding:"dive,max=64"` // 单元素上限防巨型 hash 串 DoS（infohash 40 hex）
	DeleteFiles bool     `json:"delete_files"`
}

// Pause 暂停任务。
func (h *DownloaderHandler) Pause(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.dl.Pause(c.Request.Context(), id, req.Hashes); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Resume 恢复任务。
func (h *DownloaderHandler) Resume(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.dl.Resume(c.Request.Context(), id, req.Hashes); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Remove 删除任务。
func (h *DownloaderHandler) Remove(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.dl.Delete(c.Request.Context(), id, req.Hashes, req.DeleteFiles); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Stats 全局速率。
func (h *DownloaderHandler) Stats(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	st, err := h.dl.Stats(c.Request.Context(), id)
	if err != nil {
		// 502：下载器协议层故障（不可达/版本不支持），非客户端参数错误。
		errcode.Respond(c, errcode.DownloaderConnFailed.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, st)
}
