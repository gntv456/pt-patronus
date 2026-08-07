package v1

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/filemanager"
)

// filemanager_extra.go — 扩展文件操作 handler（在线编辑/压缩/解压/chmod/目录大小）。
// 独立成文件避免 filemanager.go 超 400 行。所有方法 soft-fail（200+ok:false+error）。

// ReadFile 读取文件内容（在线编辑加载/文本预览）。
func (h *FileStorageHandler) ReadFile(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	p := c.Query("path")
	data, err := h.fileCh.ReadFile(c.Request.Context(), id, p)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "content": string(data)})
}

type writeFileRequest struct {
	Content string `json:"content"`
}

// WriteFile 保存文件内容（在线编辑）。
func (h *FileStorageHandler) WriteFile(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	p := c.Query("path")
	if isRootPath(p) {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "不允许操作存储根目录"})
		return
	}
	var in writeFileRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.fileCh.WriteFile(c.Request.Context(), id, p, []byte(in.Content)); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Archive 压缩。query: src=源 path, dst=目标压缩包路径。
func (h *FileStorageHandler) Archive(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	src := c.Query("src")
	dst := c.Query("dst")
	if isRootPath(src) {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "不允许压缩存储根目录"})
		return
	}
	if err := h.fileCh.Archive(c.Request.Context(), id, src, dst); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Extract 解压。query: src=压缩包 path, dst=解压目标目录。
func (h *FileStorageHandler) Extract(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	src := c.Query("src")
	dst := c.Query("dst")
	if isRootPath(src) {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "不允许操作存储根目录"})
		return
	}
	if err := h.fileCh.Extract(c.Request.Context(), id, src, dst); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type chmodRequest struct {
	Mode      uint32 `json:"mode"`
	Recursive bool   `json:"recursive"`
}

// Chmod 修改权限。
func (h *FileStorageHandler) Chmod(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	p := c.Query("path")
	if isRootPath(p) {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "不允许操作存储根目录"})
		return
	}
	var in chmodRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.fileCh.Chmod(c.Request.Context(), id, p, in.Mode, in.Recursive); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DirSize 计算目录递归大小。
func (h *FileStorageHandler) DirSize(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	p := c.Query("path")
	size, err := h.fileCh.DirSize(c.Request.Context(), id, p)
	if err != nil {
		if errors.Is(err, filemanager.ErrUnsupported) {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": "该后端不支持计算目录大小"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "size": size, "size_str": humanSize(size)})
}

// humanSize 把字节数格式化为人类可读字符串。
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for d := n / unit; d >= unit; d /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return strconv.FormatFloat(float64(n)/float64(div), 'f', 2, 64) + " " + units[exp]
}
