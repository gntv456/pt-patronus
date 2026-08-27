package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"
)

// ---- 123云盘写侧：官方开放 API 上传（create → 预签名 PUT 分片 → complete → 轮询）+ 直链 ----
//
// 契约来源：okatu-loli/go-123pan（alist 123_open 驱动直接采用），逐字段核对：
//   - create body: parentFileID/filename/etag(md5)/size；响应 fileID/preuploadID/reuse/sliceSize
//   - get_upload_url body: preuploadID/sliceNo；响应 presignedURL（PUT 无 Authorization，
//     Content-Type: application/octet-stream）
//   - upload_complete body: preuploadID；响应 async/completed/fileID；async 时轮询
//     upload_async_result（≥1s 间隔）
//   - mkdir: POST /upload/v1/file/mkdir body {name,parentID}（注意非 parentFileID）→ dirID
//   - 直链: GET /api/v1/file/download_info?fileId= → data.downloadUrl
//
// 现有列目录走的 api.123pan.com 与开放平台 upload/download_info 并存（同一 token 两域可达）。

// 静态断言：client123 实现写侧。
var _ Writer = (*client123)(nil)

const api123Open = "https://open-api.123pan.com"

// MkdirP 123：逐级建目录（POST /upload/v1/file/mkdir）。
func (c *client123) MkdirP(ctx context.Context, dir string) (string, error) {
	parentID := "0"
	for _, seg := range splitSegments(dir) {
		raw, err := c.rawList(ctx, parentID)
		if err != nil {
			return "", err
		}
		found := ""
		for _, it := range raw {
			if it.isDir() && it.Name == seg {
				found = it.FileID.String()
				break
			}
		}
		if found != "" {
			parentID = found
			continue
		}
		body, _ := json.Marshal(map[string]any{"name": seg, "parentID": json.Number(parentID)})
		resp, err := c.hc.Do(ctx, "POST", api123Open+"/upload/v1/file/mkdir", body, c.authHeader(), "", true)
		if err != nil {
			return "", fmt.Errorf("123 mkdir %s: %w", seg, err)
		}
		if resp.StatusCode >= 300 {
			return "", fmt.Errorf("123 mkdir: HTTP %d", resp.StatusCode)
		}
		var wrap struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
			Data struct {
				DirID json.Number `json:"dirID"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &wrap); err != nil {
			return "", fmt.Errorf("123 mkdir parse: %w", err)
		}
		if wrap.Code != 0 {
			return "", fmt.Errorf("123 mkdir %s 失败: %s", seg, wrap.Msg)
		}
		if id := wrap.Data.DirID.String(); id != "" {
			parentID = id
			continue
		}
		again, err := c.rawList(ctx, parentID)
		if err != nil {
			return "", err
		}
		ok := false
		for _, it := range again {
			if it.isDir() && it.Name == seg {
				parentID, ok = it.FileID.String(), true
				break
			}
		}
		if !ok {
			return "", fmt.Errorf("123 mkdir %s: 无法确认新目录 id", seg)
		}
	}
	return parentID, nil
}

// Upload 123：v1 预签名直传流程。
func (c *client123) Upload(ctx context.Context, dst string, src *LocalSource) error {
	if _, err := c.statCloud(ctx, dst); err == nil {
		return nil // 已存在幂等
	}
	parentID, err := c.MkdirP(ctx, cloudDir(dst))
	if err != nil {
		return err
	}
	md5v, err := fileMD5(src.Path)
	if err != nil {
		return err
	}
	// 1) create
	createBody, _ := json.Marshal(map[string]any{
		"parentFileID": json.Number(parentID),
		"filename":     pathBase(src.Path),
		"etag":         md5v,
		"size":         src.Size,
	})
	resp, err := c.hc.Do(ctx, "POST", api123Open+"/upload/v1/file/create", createBody, c.authHeader(), "", true)
	if err != nil {
		return fmt.Errorf("123 create: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("123 create: HTTP %d", resp.StatusCode)
	}
	var created struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			PreuploadID string      `json:"preuploadID"`
			Reuse       bool        `json:"reuse"`
			SliceSize   json.Number `json:"sliceSize"`
			FileID      json.Number `json:"fileID"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &created); err != nil {
		return fmt.Errorf("123 create parse: %w", err)
	}
	if created.Code != 0 {
		return fmt.Errorf("123 create 失败: %s", created.Msg)
	}
	if created.Data.Reuse {
		return nil // 秒传
	}
	sliceSize, err := created.Data.SliceSize.Int64()
	if err != nil || sliceSize <= 0 {
		return fmt.Errorf("123 create: 无效 sliceSize %s", created.Data.SliceSize.String())
	}
	// 2) 分片：逐片取 presignedURL 后裸 PUT（无 Authorization，Content-Type 固定 octet-stream）。
	f, err := os.Open(src.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, sliceSize)
	for sliceNo := 1; ; sliceNo++ {
		n, rerr := io.ReadFull(f, buf)
		if n == 0 {
			break
		}
		urlBody, _ := json.Marshal(map[string]any{
			"preuploadID": created.Data.PreuploadID, "sliceNo": sliceNo,
		})
		resp, err := c.hc.Do(ctx, "POST", api123Open+"/upload/v1/file/get_upload_url", urlBody, c.authHeader(), "", true)
		if err != nil {
			return fmt.Errorf("123 get_upload_url %d: %w", sliceNo, err)
		}
		var urlWrap struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
			Data struct {
				PresignedURL string `json:"presignedURL"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &urlWrap); err != nil {
			return fmt.Errorf("123 get_upload_url parse: %w", err)
		}
		if urlWrap.Code != 0 || urlWrap.Data.PresignedURL == "" {
			return fmt.Errorf("123 get_upload_url %d 失败: %s", sliceNo, urlWrap.Msg)
		}
		if _, err := streamHTTP(ctx, "PUT", urlWrap.Data.PresignedURL, bytes.NewReader(buf[:n]),
			int64(n), map[string]string{"Content-Type": "application/octet-stream"}, uploadHTTPTimeout); err != nil {
			return fmt.Errorf("123 put slice %d: %w", sliceNo, err)
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	// 3) complete（可能异步合并）
	compBody, _ := json.Marshal(map[string]any{"preuploadID": created.Data.PreuploadID})
	resp, err = c.hc.Do(ctx, "POST", api123Open+"/upload/v1/file/upload_complete", compBody, c.authHeader(), "", true)
	if err != nil {
		return fmt.Errorf("123 complete: %w", err)
	}
	var comp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Async     bool `json:"async"`
			Completed bool `json:"completed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &comp); err != nil {
		return fmt.Errorf("123 complete parse: %w", err)
	}
	if comp.Code != 0 {
		return fmt.Errorf("123 complete 失败: %s", comp.Msg)
	}
	if !comp.Data.Async || comp.Data.Completed {
		return nil
	}
	// 4) 异步轮询（≥1s 间隔，上限 10 分钟）
	for i := 0; i < 600; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		resp, err := c.hc.Do(ctx, "POST", api123Open+"/upload/v1/file/upload_async_result", compBody, c.authHeader(), "", true)
		if err != nil {
			return fmt.Errorf("123 async_result: %w", err)
		}
		var async struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
			Data struct {
				Completed bool `json:"completed"`
			} `json:"data"`
		}
		if json.Unmarshal(resp.Body, &async) == nil && async.Code == 0 && async.Data.Completed {
			return nil
		}
	}
	return fmt.Errorf("123 上传合并超时（10 分钟），请稍后在网盘确认")
}

// DownloadURL 123：GET /api/v1/file/download_info?fileId= → data.downloadUrl（可直接 GET）。
func (c *client123) DownloadURL(ctx context.Context, cloudPath, ua string) (*DirectLink, error) {
	f, err := c.statCloud(ctx, cloudPath)
	if err != nil {
		return nil, err
	}
	if f.IsDir {
		return nil, fmt.Errorf("123 直链: %s 是目录", cloudPath)
	}
	if f.PickCode == "" {
		// List 未带出 FileId——补一次父目录 rawList 取 id。
		if err := c.fillPickCode(ctx, cloudPath, &f); err != nil {
			return nil, err
		}
	}
	u := api123 + "/api/v1/file/download_info?fileId=" + urlQueryEscape(f.PickCode)
	resp, err := c.hc.Do(ctx, "GET", u, nil, c.authHeader(), "", true)
	if err != nil {
		return nil, fmt.Errorf("123 download_info: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("123 download_info: HTTP %d", resp.StatusCode)
	}
	var wrap struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			DownloadURL string `json:"downloadUrl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &wrap); err != nil {
		return nil, fmt.Errorf("123 download_info parse: %w", err)
	}
	if wrap.Code != 0 || wrap.Data.DownloadURL == "" {
		return nil, fmt.Errorf("123 直链解析失败: %s", wrap.Msg)
	}
	h := map[string]string{}
	if ua != "" {
		h["User-Agent"] = ua
	}
	return &DirectLink{URL: wrap.Data.DownloadURL, Header: h}, nil
}

// fillPickCode List 未带 FileId 时从 rawList 补（123 的 PickCode 复用为 FileId）。
func (c *client123) fillPickCode(ctx context.Context, cloudPath string, f *File) error {
	parentID, err := c.resolvePath(ctx, cloudDir(cloudPath))
	if err != nil {
		return err
	}
	raw, err := c.rawList(ctx, parentID)
	if err != nil {
		return err
	}
	name := pathBase(cloudPath)
	for _, it := range raw {
		if !it.isDir() && it.Name == name {
			f.PickCode = it.FileID.String()
			return nil
		}
	}
	return fmt.Errorf("123 直链: 文件不存在 %s", cloudPath)
}

// statCloud 123：父目录列表按名找。
func (c *client123) statCloud(ctx context.Context, dst string) (File, error) {
	files, err := c.List(ctx, cloudDir(dst))
	if err != nil {
		return File{}, err
	}
	name := pathBase(dst)
	for _, f := range files {
		if f.Name == name {
			return f, nil
		}
	}
	return File{}, os.ErrNotExist
}

// urlQueryEscape 数字 id 转义（等价 net/url.QueryEscape）。
func urlQueryEscape(s string) string { return url.QueryEscape(s) }
