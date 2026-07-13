package v1

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/filemanager"
)

// FileStorageHandler 处理存储后端 CRUD 与文件操作。
type FileStorageHandler struct {
	op     *oper.FileStorageOper
	fileCh *chain.FileManagerChain
	mgr    *filemanager.Manager
}

// NewFileStorageHandler 构造。
func NewFileStorageHandler(op *oper.FileStorageOper, fileCh *chain.FileManagerChain, mgr *filemanager.Manager) *FileStorageHandler {
	return &FileStorageHandler{op: op, fileCh: fileCh, mgr: mgr}
}

// List 列出全部存储后端。
func (h *FileStorageHandler) List(c *gin.Context) {
	fs, err := h.op.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"storages": fs})
}

// Create 创建存储后端。
func (h *FileStorageHandler) Create(c *gin.Context) {
	var in oper.FileStorageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	f, err := h.op.Create(in)
	if err != nil {
		if errors.Is(err, oper.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, f)
}

// Update 更新存储后端。
func (h *FileStorageHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.FileStorageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	f, err := h.op.Update(id, in)
	if err != nil {
		if errors.Is(err, oper.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	h.mgr.Invalidate(id)
	c.JSON(http.StatusOK, f)
}

// Delete 删除存储后端。
func (h *FileStorageHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.op.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.mgr.Invalidate(id)
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Files 列目录。
func (h *FileStorageHandler) Files(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	path := c.Query("path")
	if path == "" {
		path = "/"
	}
	entries, err := h.fileCh.List(c.Request.Context(), id, path)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"entries": []any{}, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}

// Upload 上传文件（multipart file 字段）。
func (h *FileStorageHandler) Upload(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	path := c.PostForm("path")
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer file.Close()
	if err := h.fileCh.Upload(c.Request.Context(), id, path, file); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Download 下载文件（流式）。
func (h *FileStorageHandler) Download(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	path := c.Query("path")
	rc, err := h.fileCh.Download(c.Request.Context(), id, path)
	if err != nil {
		// 下载出错返 500（而非 200+JSON）：客户端按字节流读 body，200 会把错误 JSON 当成文件内容。
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rc.Close()
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}

// Mkdir 建目录。
func (h *FileStorageHandler) Mkdir(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	path := c.Query("path")
	if err := h.fileCh.Mkdir(c.Request.Context(), id, path); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Test 探测存储连通性（200 包错，对齐下载器 Test 范式）。
func (h *FileStorageHandler) Test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.fileCh.Test(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteFile 删除文件/目录（path 走 query，与 Mkdir 一致；soft-fail）。
func (h *FileStorageHandler) DeleteFile(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	path := c.Query("path")
	if isRootPath(path) {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "不允许操作存储根目录"})
		return
	}
	if err := h.fileCh.Delete(c.Request.Context(), id, path); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Rename 同目录改名（path=原路径 + name=新名，走 query）。
func (h *FileStorageHandler) Rename(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	name := c.Query("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 必填"})
		return
	}
	renamePath := c.Query("path")
	if isRootPath(renamePath) {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "不允许操作存储根目录"})
		return
	}
	if err := h.fileCh.Rename(c.Request.Context(), id, renamePath, name); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Move 移动（from=源路径 + to=目标目录，走 query）。
func (h *FileStorageHandler) Move(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	from := c.Query("from")
	if isRootPath(from) {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "不允许移动存储根目录"})
		return
	}
	if err := h.fileCh.Move(c.Request.Context(), id, from, c.Query("to")); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Copy 复制（from=源路径 + to=目标目录，走 query）。
func (h *FileStorageHandler) Copy(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	from := c.Query("from")
	if isRootPath(from) {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "不允许复制存储根目录"})
		return
	}
	if err := h.fileCh.Copy(c.Request.Context(), id, from, c.Query("to")); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// isRootPath 判断虚拟路径是否指向存储根（"/" 或空）。破坏性操作（删/移/改/复制）
// 作用于根会清空/搬空整个存储，故在 handler 入口拒绝（各 backend 另有纵深防护，
// 防 AI 工具绕过 HTTP 直调 backend）。
func isRootPath(p string) bool {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	return path.Clean("/"+strings.TrimLeft(p, "/")) == "/"
}
