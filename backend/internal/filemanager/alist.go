package filemanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Alist 后端（REST API：/api/fs/list, /api/fs/put, /api/fs/get, /api/fs/remove, /api/fs/mkdir）。
type Alist struct {
	baseURL  string
	basePath string
	username string
	password string
	token    string
	hc       *http.Client
}

// NewAlist 构造。basePath 把所有操作约束在该子目录下（防 ../ 越界）。
func NewAlist(baseURL, username, password, basePath string) *Alist {
	return &Alist{
		baseURL:  strings.TrimRight(baseURL, "/"),
		basePath: basePath,
		username: username,
		password: password,
		hc:       &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Alist) Type() string { return "alist" }

func (a *Alist) call(ctx context.Context, method, p string, body map[string]any, out any) error {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+p, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", a.token)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return fmt.Errorf("alist: parse: %w", err)
	}
	if envelope.Code != 200 {
		return fmt.Errorf("alist: %s", envelope.Message)
	}
	if out != nil && len(envelope.Data) > 0 {
		return json.Unmarshal(envelope.Data, out)
	}
	return nil
}

// List 调 /api/fs/list。
func (a *Alist) List(ctx context.Context, p string) ([]Entry, error) {
	p = resolvePath(a.basePath, p)
	var raw []struct {
		Name string `json:"name"`
		Dir  bool   `json:"dir"`
		Size int64  `json:"size"`
	}
	if err := a.call(ctx, "list", "/api/fs/list", map[string]any{
		"path": p, "page": 1, "per_page": 0, "refresh": false,
	}, &raw); err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(raw))
	for _, r := range raw {
		out = append(out, Entry{Name: r.Name, Path: strings.TrimRight(p, "/") + "/" + r.Name, IsDir: r.Dir, Size: r.Size})
	}
	return out, nil
}

// Upload 调 /api/fs/put（流式上传需分片；此处用 put 接口直接传 bytes）。
func (a *Alist) Upload(ctx context.Context, p string, r io.Reader) error {
	p = resolvePath(a.basePath, p)
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, a.baseURL+"/api/fs/put", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("File-Path", p)
	req.Header.Set("As-Task", "false")
	if a.token != "" {
		req.Header.Set("Authorization", a.token)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("alist put: status %d", resp.StatusCode)
	}
	return nil
}

// Download 调 /api/fs/get 取直链后 GET。
func (a *Alist) Download(ctx context.Context, p string) (io.ReadCloser, error) {
	p = resolvePath(a.basePath, p)
	var raw struct {
		RawURL string `json:"raw_url"`
	}
	if err := a.call(ctx, "get", "/api/fs/get", map[string]any{"path": p}, &raw); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw.RawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("alist download: status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// Delete 调 /api/fs/remove。basePath 为空且 p="/" 时 dir 解析为空，原先 dir[:LastIndex] 会
// slice 越界 panic（每次必崩）——前置拒绝根操作，既防 panic 也防误删存储根。
func (a *Alist) Delete(ctx context.Context, p string) error {
	p = resolvePath(a.basePath, p)
	dir := strings.TrimRight(p, "/")
	if dir == "" {
		return fmt.Errorf("不允许删除存储根目录")
	}
	name := dir[strings.LastIndex(dir, "/")+1:]
	parent := dir[:strings.LastIndex(dir, "/")]
	return a.call(ctx, "remove", "/api/fs/remove", map[string]any{
		"dir": parent, "names": []string{name},
	}, nil)
}

// Mkdir 调 /api/fs/mkdir。
func (a *Alist) Mkdir(ctx context.Context, p string) error {
	p = resolvePath(a.basePath, p)
	return a.call(ctx, "mkdir", "/api/fs/mkdir", map[string]any{"path": p}, nil)
}

// Rename 调 /api/fs/rename（path 指定原文件/目录，name 为新名）。newName 经 sanitizeNewName
// 校验防 "/api/fs/rename" 收到含分隔符的恶意名（与 local/smb/webdav 统一）。
func (a *Alist) Rename(ctx context.Context, p, newName string) error {
	p = resolvePath(a.basePath, p)
	name, err := sanitizeNewName(newName)
	if err != nil {
		return err
	}
	return a.call(ctx, "rename", "/api/fs/rename", map[string]any{"path": p, "name": name}, nil)
}

// moveOrCopy 拆 src 为 src_dir+name，调 Alist 的 move/copy 端点（src_dir/names → dst_dir）。
// dir==""（basePath 空 + 根路径）前置拒绝，防 dir[:LastIndex] 越界 panic。
func (a *Alist) moveOrCopy(ctx context.Context, src, dstDir, endpoint, tag string) error {
	srcP := resolvePath(a.basePath, src)
	dstP := resolvePath(a.basePath, dstDir)
	dir := strings.TrimRight(srcP, "/")
	if dir == "" {
		return fmt.Errorf("不允许操作存储根目录")
	}
	name := dir[strings.LastIndex(dir, "/")+1:]
	srcDir := dir[:strings.LastIndex(dir, "/")]
	if srcDir == "" {
		srcDir = "/"
	}
	return a.call(ctx, tag, endpoint, map[string]any{
		"src_dir": srcDir, "dst_dir": dstP, "names": []string{name},
	}, nil)
}

// Move 调 /api/fs/move。
func (a *Alist) Move(ctx context.Context, src, dstDir string) error {
	return a.moveOrCopy(ctx, src, dstDir, "/api/fs/move", "move")
}

// Copy 调 /api/fs/copy。
func (a *Alist) Copy(ctx context.Context, src, dstDir string) error {
	return a.moveOrCopy(ctx, src, dstDir, "/api/fs/copy", "copy")
}
