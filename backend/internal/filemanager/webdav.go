package filemanager

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// WebDAV 后端（PROPFIND/PUT/GET/DELETE/MKCOL）。
type WebDAV struct {
	baseURL  string
	basePath string
	username string
	password string
	hc       *http.Client
}

// NewWebDAV 构造。basePath 把所有操作约束在该子目录下（防 ../ 越界）。
func NewWebDAV(baseURL, username, password, basePath string) *WebDAV {
	return &WebDAV{
		baseURL:  strings.TrimRight(baseURL, "/"),
		basePath: basePath,
		username: username,
		password: password,
		hc:       &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *WebDAV) Type() string { return "webdav" }

func (w *WebDAV) req(ctx context.Context, method, urlPath string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, w.baseURL+urlPath, body)
	if err != nil {
		return nil, err
	}
	if w.username != "" {
		req.SetBasicAuth(w.username, w.password)
	}
	return w.hc.Do(req)
}

// List 用 PROPFIND Depth:1 列目录。
func (w *WebDAV) List(ctx context.Context, p string) ([]Entry, error) {
	p = ensureSlash(resolvePath(w.basePath, p))
	propfindBody := `<?xml version="1.0" encoding="utf-8"?>
<propfind xmlns="DAV:"><prop>
<displayname/><getcontentlength/><resourcetype/><getlastmodified/>
</prop></propfind>`
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", w.baseURL+p, strings.NewReader(propfindBody))
	if err != nil {
		return nil, err
	}
	if w.username != "" {
		req.SetBasicAuth(w.username, w.password)
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	resp, err := w.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("webdav PROPFIND: status %d", resp.StatusCode)
	}
	// PROPFIND 响应正常 <1MB；限制 4MB 防恶意/超大目录 PROPFIND 撑爆内存
	// （basePath 已约束，但目录内条目数仍可巨大）。LimitReader 读到上限即止。
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	entries, err := parseMultiStatus(data, p)
	if err != nil {
		return nil, err
	}
	// 去掉第一项（目录自身）
	out := entries
	if len(out) > 0 && out[0].Path == normalizePath(p) {
		out = out[1:]
	}
	return out, nil
}

// Upload 用 PUT 上传。
func (w *WebDAV) Upload(ctx context.Context, p string, r io.Reader) error {
	p = resolvePath(w.basePath, p)
	resp, err := w.req(ctx, http.MethodPut, p, r)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webdav PUT: status %d", resp.StatusCode)
	}
	return nil
}

// Download 用 GET 下载。
func (w *WebDAV) Download(ctx context.Context, p string) (io.ReadCloser, error) {
	p = resolvePath(w.basePath, p)
	resp, err := w.req(ctx, http.MethodGet, p, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("webdav GET: status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// Delete 删除。
func (w *WebDAV) Delete(ctx context.Context, p string) error {
	p = resolvePath(w.basePath, p)
	if p == resolvePath(w.basePath, "/") {
		return fmt.Errorf("不允许删除存储根目录")
	}
	resp, err := w.req(ctx, http.MethodDelete, p, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	// 404 = 服务器上压根没有这个路径。原先豁免它，于是删一个已被别人删掉的行
	// 界面报「已删除」而那行还在；与 local/rclone 统一成「不存在就是失败」。
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("webdav DELETE %s: %w", p, fs.ErrNotExist)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webdav DELETE: status %d", resp.StatusCode)
	}
	return nil
}

// Mkdir 用 MKCOL 建目录。
func (w *WebDAV) Mkdir(ctx context.Context, p string) error {
	p = resolvePath(w.basePath, p)
	resp, err := w.req(ctx, "MKCOL", ensureSlash(p), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusMethodNotAllowed {
		return fmt.Errorf("webdav MKCOL: status %d", resp.StatusCode)
	}
	return nil
}

// moveOrCopy 执行 WebDAV MOVE/COPY：src→dst，Overwrite 允许覆盖。
//
// 207 Multi-Status 处理：COPY/MOVE 一个目录（Depth:infinity）时，部分子资源失败
// （锁/权限）会让服务器返回 207，body 内含每个 href 的 status。原先仅判 >=300，而
// 207<300 被当成功 → 静默部分数据丢失/源目标分裂态。现解析 body，任一子资源 4xx/5xx 即报错。
func (w *WebDAV) moveOrCopy(ctx context.Context, method, srcRel, dstRel string) error {
	req, err := http.NewRequestWithContext(ctx, method, w.baseURL+srcRel, nil)
	if err != nil {
		return err
	}
	if w.username != "" {
		req.SetBasicAuth(w.username, w.password)
	}
	req.Header.Set("Destination", w.baseURL+dstRel)
	req.Header.Set("Overwrite", "T")
	resp, err := w.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 207 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		// 任一子资源返回 4xx/5xx 即视为部分失败。
		if strings.Contains(string(body), "HTTP/1.1 4") || strings.Contains(string(body), "HTTP/1.1 5") {
			return fmt.Errorf("webdav %s 部分子资源失败（207 含错误状态）", method)
		}
		return nil
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webdav %s: status %d", method, resp.StatusCode)
	}
	return nil
}

// Rename 同目录改名（WebDAV MOVE 到父目录下的 newName）。newName 经 sanitizeNewName 钳制，
// 防 `name="a/b"` 把文件移进子目录或 `name="../x"` 越界（违反"同目录改名"契约）。
func (w *WebDAV) Rename(ctx context.Context, p, newName string) error {
	if isResolvedStorageRoot(w.basePath, p) {
		return fmt.Errorf("不允许重命名存储根目录")
	}
	src := resolvePath(w.basePath, p)
	name, err := sanitizeNewName(newName)
	if err != nil {
		return err
	}
	dst := resolvePath(w.basePath, path.Join(path.Dir(normalizePath(src)), name))
	return w.moveOrCopy(ctx, "MOVE", src, dst)
}

// Move 移到 dstDir（保留文件名）。
func (w *WebDAV) Move(ctx context.Context, src, dstDir string) error {
	if isResolvedStorageRoot(w.basePath, src) {
		return fmt.Errorf("不允许移动存储根目录")
	}
	srcRel := resolvePath(w.basePath, src)
	dst := resolvePath(w.basePath, path.Join(normalizePath(resolvePath(w.basePath, dstDir)), path.Base(srcRel)))
	return w.moveOrCopy(ctx, "MOVE", srcRel, dst)
}

// Copy 复制到 dstDir（保留文件名）。
func (w *WebDAV) Copy(ctx context.Context, src, dstDir string) error {
	if isResolvedStorageRoot(w.basePath, src) {
		return fmt.Errorf("不允许复制存储根目录")
	}
	srcRel := resolvePath(w.basePath, src)
	dst := resolvePath(w.basePath, path.Join(normalizePath(resolvePath(w.basePath, dstDir)), path.Base(srcRel)))
	return w.moveOrCopy(ctx, "COPY", srcRel, dst)
}

// ── multistatus XML 解析 ──

type multistatus struct {
	XMLName   xml.Name `xml:"multistatus"`
	Responses []struct {
		Href     string `xml:"DAV: href"`
		Propstat []struct {
			Prop struct {
				Name         string `xml:"DAV: displayname"`
				Size         int64  `xml:"DAV: getcontentlength"`
				Resourcetype struct {
					Collection *struct{} `xml:"DAV: collection"`
				} `xml:"DAV: resourcetype"`
				Modified string `xml:"DAV: getlastmodified"`
			} `xml:"DAV: prop"`
		} `xml:"DAV: propstat"`
	} `xml:"DAV: response"`
}

func parseMultiStatus(data []byte, base string) ([]Entry, error) {
	var ms multistatus
	if err := xml.Unmarshal(data, &ms); err != nil {
		return nil, fmt.Errorf("parse multistatus: %w", err)
	}
	out := make([]Entry, 0, len(ms.Responses))
	for _, r := range ms.Responses {
		if len(r.Propstat) == 0 {
			continue
		}
		prop := r.Propstat[0].Prop
		name := prop.Name
		if name == "" {
			name = path.Base(strings.TrimRight(decode(r.Href), "/"))
		}
		e := Entry{
			Name:    name,
			Path:    normalizePath(decode(r.Href)),
			IsDir:   prop.Resourcetype.Collection != nil || strings.HasSuffix(decode(r.Href), "/"),
			Size:    prop.Size,
			ModTime: parseHTTPDate(prop.Modified),
		}
		out = append(out, e)
	}
	return out, nil
}

func ensureSlash(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

func normalizePath(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

// decode 解码 URL 编码的 href（含中文），并去掉可能的 host 前缀只留路径。
func decode(s string) string {
	unescaped, err := url.PathUnescape(s)
	if err != nil {
		unescaped = s
	}
	if strings.Contains(unescaped, "://") {
		if u := strings.Index(unescaped, "://"); u >= 0 {
			rest := unescaped[u+3:]
			if sl := strings.Index(rest, "/"); sl >= 0 {
				return rest[sl:]
			}
		}
	}
	return unescaped
}

func parseHTTPDate(s string) time.Time {
	t, err := http.ParseTime(s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ReadFile 读取文件内容（通用实现：Download 全读到内存）。
func (w *WebDAV) ReadFile(ctx context.Context, p string) ([]byte, error) {
	return genericReadFile(ctx, w, p)
}

// WriteFile 写入文件内容（通用实现：Upload 桥接）。
func (w *WebDAV) WriteFile(ctx context.Context, p string, data []byte) error {
	return genericWriteFile(ctx, w, p, data)
}

// DirSize 计算目录大小（通用实现：List 递归累加）。
func (w *WebDAV) DirSize(ctx context.Context, p string) (int64, error) {
	return genericDirSize(ctx, w, p)
}
