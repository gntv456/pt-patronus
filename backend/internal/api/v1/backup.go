package v1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/backup"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// BackupHandler 处理配置导出/导入。
type BackupHandler struct {
	svc    *backup.Service
	notice *notice.Service
}

// NewBackupHandler 构造。ns 为站内通知中心（备份完成/导入成功发通知；nil 跳过）。
func NewBackupHandler(svc *backup.Service, ns *notice.Service) *BackupHandler {
	return &BackupHandler{svc: svc, notice: ns}
}

// Export 导出自身格式备份（JSON，含明文密钥）。
func (h *BackupHandler) Export(c *gin.Context) {
	data, err := h.svc.Export(c.Request.Context())
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.Header("Content-Disposition", `attachment; filename="ptpatronus-backup.json"`)
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "application/json", data)
	if h.notice != nil {
		_ = h.notice.Notice(c.Request.Context(), notice.Input{
			Source: notice.SourceBackupDone,
			Title:  "备份导出完成",
			Body:   "配置已导出为 JSON 文件",
			Level:  "success",
		}, false)
	}
}

// Import 导入自身格式备份。
func (h *BackupHandler) Import(c *gin.Context) {
	data, err := readBody(c)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	sites, dls, err := h.svc.Import(c.Request.Context(), data)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if h.notice != nil {
		_ = h.notice.Notice(c.Request.Context(), notice.Input{
			Source: notice.SourceBackupImport,
			Title:  "备份导入成功",
			Body:   fmt.Sprintf("已导入 %d 个站点、%d 个下载器", sites, dls),
			Level:  "success",
		}, false)
	}
	c.JSON(http.StatusOK, gin.H{"imported_sites": sites, "imported_downloaders": dls})
}

// ImportMoviePilot 导入 MoviePilot 站点 JSON。
func (h *BackupHandler) ImportMoviePilot(c *gin.Context) {
	data, err := readBody(c)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	n, err := h.svc.ImportMoviePilot(c.Request.Context(), data)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"imported_sites": n})
}

// ImportPTD 导入 PT-Depiler (.ptd) 备份（multipart 文件上传，field=file）。
// .ptd 是 ZIP（含 manifest/metadata/cookies），后端解压转换，重复站点跳过。
func (h *BackupHandler) ImportPTD(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		errcode.Respond(c, errcode.BackupFileInvalid)
		return
	}
	if fh.Size > maxBackupSize {
		errcode.Respond(c, errcode.BackupFileTooLarge)
		return
	}
	f, err := fh.Open()
	if err != nil {
		errcode.Respond(c, errcode.BackupReadFailed)
		return
	}
	defer f.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, f); err != nil {
		errcode.Respond(c, errcode.BackupReadFailed)
		return
	}
	n, err := h.svc.ImportPTD(c.Request.Context(), buf.Bytes())
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if h.notice != nil {
		_ = h.notice.Notice(c.Request.Context(), notice.Input{
			Source: notice.SourceBackupImport,
			Title:  "PT-Depiler 备份导入成功",
			Body:   fmt.Sprintf("已从 .ptd 导入 %d 个站点", n),
			Level:  "success",
		}, false)
	}
	c.JSON(http.StatusOK, gin.H{"imported_sites": n})
}

// maxBackupSize 限制备份导入请求体大小（备份 JSON 正常 <几 MB，防认证用户 POST 超大 body OOM）。
const maxBackupSize = 64 << 20

func readBody(c *gin.Context) ([]byte, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBackupSize)
	var raw json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}
