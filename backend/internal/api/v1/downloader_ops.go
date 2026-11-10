// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
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
		resp := gin.H{"ok": false, "error": userErrText(err)}
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
		errcode.Respond(c, errcode.FieldRequired.WithErr(err))
		return
	}
	if file.Size > 8<<20 {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	src, err := file.Open()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
		errcode.Respond(c, errcode.InvalidRequest.WithErr(err))
		return
	} else {
		opts.UploadLimit = n
	}
	if n, err := parseLimit(c.PostForm("download_limit")); err != nil {
		errcode.Respond(c, errcode.InvalidRequest.WithErr(err))
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
