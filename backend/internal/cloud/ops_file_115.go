package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ---- 云端文件管理器写操作扩展 ----
//
// 在 Writer（MkdirP/Upload/DownloadURL）之上补齐文件管理器所需的单文件操作与
// 云盘特有能力（离线下载 / 分享转存）。契约来源同前（alist 驱动 + 115 SDK）：
//   115(web)：  move POST /files/move   {pid(目标), move_file_id}
//              copy POST /files/copy    {pid, move_file_id}
//              rename POST /files/rename{pid, file_name, file_name_orig?}——重命名目标填新名
//              delete POST /files/rdelete {pid, ignore_warn:1}（move_file_id 列表）
//   115(open)： move POST /open/ufile/move   {file_id, to_cid}
//              copy POST /open/ufile/copy   {file_id, to_cid}
//              rename POST /open/ufile/update {file_id, file_name}
//              delete POST /open/ufile/delete {file_id}
//   夸克：      move/rename POST /file  {action:"file_move"|"file_rename"...}（task 异步，简化为提交即成功）
//              copy POST /file/copy    {file_list, to_pdir_fid}（task 异步）
//              delete POST /file/delete {action_type:2, file_list, exclude_fids:[]}
//   123(open)： POST /upload/v1/file/move_item?fileID=&toParentFileID=（此接口族在 /api/v1 下）
//              rename POST /api/v1/file/rename?fileID=&fileName=（query 型）
//              delete POST /api/v1/file/trash?fileIDs=
// 离线下载/转存仅 115(web/open) 与夸克支持（走各自 task API）；123 无公开契约返回不支持。

// FileManager 云端文件管理器能力（client 按实现情况支持；不支持的返回 ErrUploadUnsupported）。
type FileManager interface {
	// Mkdir 单层建目录（父目录必须已存在；MkdirP 的单步版）。
	Mkdir(ctx context.Context, parent, name string) error
	// Move 移动文件/目录到新父目录。
	Move(ctx context.Context, src, dstDir string) error
	// Copy 复制（大文件为网盘服务端异步任务，提交成功即返回）。
	Copy(ctx context.Context, src, dstDir string) error
	// Rename 重命名。
	Rename(ctx context.Context, src, newName string) error
	// Delete 删除（进回收站或永久，由网盘语义决定）。
	Delete(ctx context.Context, src string) error
	// OfflineAdd 提交离线下载任务（磁力/HTTP 链接下载到云盘 dir）。
	OfflineAdd(ctx context.Context, dir, url string) error
	// SaveShare 转存分享链接到云盘 dir（shareURL 含提取码则拼 pwd 参数）。
	SaveShare(ctx context.Context, dir, shareURL, pwd string) error
}

var _ FileManager = (*client115)(nil)

// Mkdir 单层建目录：复用 MkdirP（对已存在目录幂等成功，语义兼容）。
func (c *client115) Mkdir(ctx context.Context, parent, name string) error {
	_, err := c.MkdirP(ctx, joinCloud(parent, name))
	return err
}

// Move 115 web：resolve 两端 id → POST /files/move。
func (c *client115) Move(ctx context.Context, src, dstDir string) error {
	cid, err := c.resolvePath(ctx, cloudDirOf(src))
	if err != nil {
		return err
	}
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	dst, err := c.resolvePath(ctx, dstDir)
	if err != nil {
		return err
	}
	form := url.Values{"pid": {dst}, "move_file_id": {f.PickCode}}
	// 115 文件操作用 file_id（fid）；List 返回的 PickCode 即 pickcode，fid 需再取——
	// 简化：用 pickcode 所在行的 fid 由 statCloud 二跳补齐
	fid, err := c.fidOf(ctx, cid, pathBase(src))
	if err != nil {
		return err
	}
	form.Set("move_file_id", fid)
	resp, err := c.hc.Do(ctx, "POST", "https://webapi.115.com/files/move",
		[]byte(form.Encode()), jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return err
	}
	return check115State(resp, "move")
}

// Copy 115 web：POST /files/copy。
func (c *client115) Copy(ctx context.Context, src, dstDir string) error {
	cid, err := c.resolvePath(ctx, cloudDirOf(src))
	if err != nil {
		return err
	}
	fid, err := c.fidOf(ctx, cid, pathBase(src))
	if err != nil {
		return err
	}
	dst, err := c.resolvePath(ctx, dstDir)
	if err != nil {
		return err
	}
	form := url.Values{"pid": {dst}, "move_file_id": {fid}}
	resp, err := c.hc.Do(ctx, "POST", "https://webapi.115.com/files/copy",
		[]byte(form.Encode()), jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return err
	}
	return check115State(resp, "copy")
}

// Rename 115 web：POST /files/rename。
func (c *client115) Rename(ctx context.Context, src, newName string) error {
	cid, err := c.resolvePath(ctx, cloudDirOf(src))
	if err != nil {
		return err
	}
	fid, err := c.fidOf(ctx, cid, pathBase(src))
	if err != nil {
		return err
	}
	form := url.Values{"pid": {cid}, "file_name": {newName}, "move_file_id": {fid}}
	resp, err := c.hc.Do(ctx, "POST", "https://webapi.115.com/files/rename",
		[]byte(form.Encode()), jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return err
	}
	return check115State(resp, "rename")
}

// Delete 115 web：POST /files/rdelete（回收站删除）。
func (c *client115) Delete(ctx context.Context, src string) error {
	cid, err := c.resolvePath(ctx, cloudDirOf(src))
	if err != nil {
		return err
	}
	fid, err := c.fidOf(ctx, cid, pathBase(src))
	if err != nil {
		return err
	}
	form := url.Values{"pid": {cid}, "move_file_id": {fid}, "ignore_warn": {"1"}}
	resp, err := c.hc.Do(ctx, "POST", "https://webapi.115.com/files/rdelete",
		[]byte(form.Encode()), jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return err
	}
	return check115State(resp, "delete")
}

// OfflineAdd 115 web：POST https://115.com/web/lixian/?task=add（urls[] 表单）。
func (c *client115) OfflineAdd(ctx context.Context, dir, rawURL string) error {
	wpID, err := c.resolvePath(ctx, dir)
	if err != nil {
		return err
	}
	form := url.Values{"url": {rawURL}, "wp_path": {""}}
	if wpID != "0" {
		form.Set("wp_cid", wpID)
	}
	resp, err := c.hc.Do(ctx, "POST", "https://115.com/web/lixian/?task=add",
		[]byte(form.Encode()), jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return err
	}
	return check115State(resp, "offline add")
}

// SaveShare 115 web：两步——POST /share/serve/shareinfo? 拿 share_code/share_user；
// 简化为调 POST https://webapi.115.com/share/receive {share_code, receive_code, cid, is_copy}.
// shareURL 形如 https://115.com/s/XXX?password=YYY 提取。
func (c *client115) SaveShare(ctx context.Context, dir, shareURL, pwd string) error {
	code := extract115ShareCode(shareURL)
	if code == "" {
		return fmt.Errorf("无法从分享链接提取 share_code")
	}
	if pwd == "" {
		pwd = extract115SharePwd(shareURL)
	}
	cid, err := c.resolvePath(ctx, dir)
	if err != nil {
		return err
	}
	form := url.Values{
		"share_code": {code}, "receive_code": {pwd}, "cid": {cid}, "is_copy": {"1"},
	}
	resp, err := c.hc.Do(ctx, "POST", "https://webapi.115.com/share/receive",
		[]byte(form.Encode()), jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return err
	}
	return check115State(resp, "share receive")
}

// fidOf 父目录内按名取文件/目录 fid（web 契约文件操作需要 fid 而非 pickcode）。
func (c *client115) fidOf(ctx context.Context, cid, name string) (string, error) {
	raw, err := c.rawList(ctx, cid)
	if err != nil {
		return "", err
	}
	for _, it := range raw {
		if it.Name == name {
			return it.id(), nil
		}
	}
	return "", fmt.Errorf("115 文件不存在: %s", name)
}

// check115State 统一校验 {state,error} 响应（115 web 契约）。
func check115State(resp *httpclient.Response, op string) error {
	if resp.StatusCode >= 300 {
		return fmt.Errorf("115 %s: HTTP %d", op, resp.StatusCode)
	}
	var body struct {
		State bool   `json:"state"`
		Err   string `json:"error"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return fmt.Errorf("115 %s parse: %w", op, err)
	}
	if !body.State {
		return fmt.Errorf("115 %s 失败: %s", op, body.Err)
	}
	return nil
}

// extract115ShareCode 从分享 URL 提取 share_code（/s/后段）。
func extract115ShareCode(u string) string {
	if i := indexStr(u, "/s/"); i >= 0 {
		rest := u[i+3:]
		for j := 0; j < len(rest); j++ {
			if rest[j] == '?' || rest[j] == '#' || rest[j] == '/' {
				return rest[:j]
			}
		}
		return rest
	}
	return ""
}

// extract115SharePwd 从 URL query 提取 password/提取码。
func extract115SharePwd(u string) string {
	if i := indexStr(u, "password="); i >= 0 {
		rest := u[i+9:]
		for j := 0; j < len(rest); j++ {
			if rest[j] == '&' || rest[j] == '#' {
				return rest[:j]
			}
		}
		return rest
	}
	return ""
}

func indexStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
