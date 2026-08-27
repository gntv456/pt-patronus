package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// ---- 夸克 / 123 / 115开放 / 阿里 的文件管理器实现 ----
//
// 契约（alist drivers + 123 SDK）：
//   夸克：move/rename POST /file（task 异步，提交即返）；copy POST /file/copy；
//         delete POST /file/delete {action_type:2,file_list:[fid],exclude_fids:[]}；
//         离线 POST /task  {task_type:2(磁力)/4(http?)...}；转存 POST /task {task_type:1, share_url,...}
//   123： move POST /api/v1/file/move_item?fileID=&toParentFileID=
//         rename POST /api/v1/file/rename?fileID=&fileName=（query）
//         delete POST /api/v1/file/trash?fileIDs=
//   115开放：move POST /open/ufile/move {file_id,to_cid}；copy /open/ufile/copy；
//         rename POST /open/ufile/update {file_id,file_name}；delete /open/ufile/delete {file_id}
//   阿里：  move POST /adrive/v1.0/openFile/move {file_id,to_parent_file_id,...}；
//         rename POST /adrive/v1.0/openFile/update {file_id,name,...}；
//         delete(move to recycle) POST /adrive/v1.0/openFile/recycle {file_id}

// ---- 夸克 ----

var _ FileManager = (*clientQuark)(nil)

func (c *clientQuark) Mkdir(ctx context.Context, parent, name string) error {
	_, err := c.MkdirP(ctx, joinCloud(parent, name))
	return err
}

// quarkTaskAction POST /file 的 action 型任务（move/rename 共端点）。
func (c *clientQuark) fileAction(ctx context.Context, action string, body map[string]any) error {
	body["action"] = action
	b, _ := json.Marshal(body)
	resp, err := c.hc.Do(ctx, "POST", apiQuark+"/file?pr=ucpro&fr=pc", b, jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("quark %s: HTTP %d", action, resp.StatusCode)
	}
	var wrap struct {
		Status int    `json:"status"`
		Code   int    `json:"code"`
		Msg    string `json:"message"`
	}
	if json.Unmarshal(resp.Body, &wrap) == nil && (wrap.Status >= 400 || wrap.Code != 0) {
		return fmt.Errorf("quark %s 失败: %s", action, wrap.Msg)
	}
	return nil
}

func (c *clientQuark) Move(ctx context.Context, src, dstDir string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	dst, err := c.resolvePath(ctx, dstDir)
	if err != nil {
		return err
	}
	return c.fileAction(ctx, "file_move", map[string]any{
		"file_list": []string{f.PickCode}, "to_pdir_fid": dst,
	})
}

func (c *clientQuark) Copy(ctx context.Context, src, dstDir string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	dst, err := c.resolvePath(ctx, dstDir)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"file_list": []string{f.PickCode}, "to_pdir_fid": dst})
	resp, err := c.hc.Do(ctx, "POST", apiQuark+"/file/copy?pr=ucpro&fr=pc", body, jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("quark copy: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *clientQuark) Rename(ctx context.Context, src, newName string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	return c.fileAction(ctx, "file_rename", map[string]any{
		"fid": f.PickCode, "file_name": newName,
	})
}

func (c *clientQuark) Delete(ctx context.Context, src string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"action_type": 2, "file_list": []string{f.PickCode}, "exclude_fids": []string{},
	})
	resp, err := c.hc.Do(ctx, "POST", apiQuark+"/file/delete?pr=ucpro&fr=pc", body, jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("quark delete: HTTP %d", resp.StatusCode)
	}
	return nil
}

// OfflineAdd 夸克：POST /task（磁力 task_type=2，http task_type=4? 社区契约磁力=2 URL=4）。
func (c *clientQuark) OfflineAdd(ctx context.Context, dir, rawURL string) error {
	fid, err := c.resolvePath(ctx, dir)
	if err != nil {
		return err
	}
	taskType := 4
	if len(rawURL) > 7 && rawURL[:7] == "magnet:" {
		taskType = 2
	}
	body, _ := json.Marshal(map[string]any{
		"task_type": taskType, "url": rawURL, "parent_fid": fid,
	})
	resp, err := c.hc.Do(ctx, "POST", apiQuark+"/task?pr=ucpro&fr=pc", body, jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("quark offline: HTTP %d", resp.StatusCode)
	}
	return nil
}

// SaveShare 夸克：POST /task task_type=1（share_url + passcode）。
func (c *clientQuark) SaveShare(ctx context.Context, dir, shareURL, pwd string) error {
	fid, err := c.resolvePath(ctx, dir)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"task_type": 1, "share_url": shareURL, "passcode": pwd, "parent_fid": fid,
	})
	resp, err := c.hc.Do(ctx, "POST", apiQuark+"/task?pr=ucpro&fr=pc", body, jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("quark share: HTTP %d", resp.StatusCode)
	}
	return nil
}

// ---- 123（开放平台 API：query 型写接口） ----

var _ FileManager = (*client123)(nil)

func (c *client123) Mkdir(ctx context.Context, parent, name string) error {
	_, err := c.MkdirP(ctx, joinCloud(parent, name))
	return err
}

func (c *client123) Move(ctx context.Context, src, dstDir string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	if err := c.fillPickCode(ctx, src, &f); err != nil {
		return err
	}
	dst, err := c.resolvePath(ctx, dstDir)
	if err != nil {
		return err
	}
	return c.call123Query(ctx, "/api/v1/file/move_item",
		url.Values{"fileID": {f.PickCode}, "toParentFileID": {dst}}, "move")
}

func (c *client123) Copy(ctx context.Context, src, dstDir string) error {
	return fmt.Errorf("%w: 123 复制", ErrUploadUnsupported)
}

func (c *client123) Rename(ctx context.Context, src, newName string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	if err := c.fillPickCode(ctx, src, &f); err != nil {
		return err
	}
	return c.call123Query(ctx, "/api/v1/file/rename",
		url.Values{"fileID": {f.PickCode}, "fileName": {newName}}, "rename")
}

func (c *client123) Delete(ctx context.Context, src string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	if err := c.fillPickCode(ctx, src, &f); err != nil {
		return err
	}
	return c.call123Query(ctx, "/api/v1/file/trash",
		url.Values{"fileIDs": {f.PickCode}}, "trash")
}

func (c *client123) OfflineAdd(ctx context.Context, dir, rawURL string) error {
	return fmt.Errorf("%w: 123 离线下载", ErrUploadUnsupported)
}

func (c *client123) SaveShare(ctx context.Context, dir, shareURL, pwd string) error {
	return fmt.Errorf("%w: 123 分享转存", ErrUploadUnsupported)
}

// call123Query POST 空 body + query 参数（123 开放平台写接口风格）。
func (c *client123) call123Query(ctx context.Context, path string, q url.Values, op string) error {
	resp, err := c.hc.Do(ctx, "POST", api123Open+path+"?"+q.Encode(), []byte("{}"), c.authHeader(), "", true)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("123 %s: HTTP %d", op, resp.StatusCode)
	}
	var wrap struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
	}
	if json.Unmarshal(resp.Body, &wrap) == nil && wrap.Code != 0 {
		return fmt.Errorf("123 %s 失败: %s", op, wrap.Msg)
	}
	return nil
}

// ---- 115 开放平台 ----

var _ FileManager = (*client115Open)(nil)

func (c *client115Open) Mkdir(ctx context.Context, parent, name string) error {
	_, err := c.MkdirP(ctx, joinCloud(parent, name))
	return err
}

func (c *client115Open) Move(ctx context.Context, src, dstDir string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	dst, err := c.resolvePath(ctx, dstDir)
	if err != nil {
		return err
	}
	return c.openAction(ctx, "/open/ufile/move", url.Values{
		"file_id": {f.PickCode}, "to_cid": {dst},
	}, "move")
}

func (c *client115Open) Copy(ctx context.Context, src, dstDir string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	dst, err := c.resolvePath(ctx, dstDir)
	if err != nil {
		return err
	}
	return c.openAction(ctx, "/open/ufile/copy", url.Values{
		"file_id": {f.PickCode}, "to_cid": {dst},
	}, "copy")
}

func (c *client115Open) Rename(ctx context.Context, src, newName string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	return c.openAction(ctx, "/open/ufile/update", url.Values{
		"file_id": {f.PickCode}, "file_name": {newName},
	}, "rename")
}

func (c *client115Open) Delete(ctx context.Context, src string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	return c.openAction(ctx, "/open/ufile/delete", url.Values{
		"file_id": {f.PickCode},
	}, "delete")
}

func (c *client115Open) OfflineAdd(ctx context.Context, dir, rawURL string) error {
	return fmt.Errorf("%w: 115开放 离线下载（用 web 版或网盘 App）", ErrUploadUnsupported)
}

func (c *client115Open) SaveShare(ctx context.Context, dir, shareURL, pwd string) error {
	return fmt.Errorf("%w: 115开放 分享转存（用 web 版或网盘 App）", ErrUploadUnsupported)
}

// openAction /open/* form 型写接口（带 token 自愈）。
func (c *client115Open) openAction(ctx context.Context, path string, form url.Values, op string) error {
	sc, rb, err := c.doHeal(ctx, "POST", api115Open+path, []byte(form.Encode()))
	if err != nil {
		return err
	}
	if sc >= 300 {
		return fmt.Errorf("115_open %s: HTTP %d", op, sc)
	}
	var body struct {
		State bool   `json:"state"`
		Err   string `json:"error"`
	}
	if json.Unmarshal(rb, &body) == nil && !body.State {
		return fmt.Errorf("115_open %s 失败: %s", op, body.Err)
	}
	return nil
}

// ---- 阿里云盘 ----

var _ FileManager = (*clientAliyunOpen)(nil)

func (c *clientAliyunOpen) Mkdir(ctx context.Context, parent, name string) error {
	_, err := c.MkdirP(ctx, joinCloud(parent, name))
	return err
}

func (c *clientAliyunOpen) Move(ctx context.Context, src, dstDir string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	driveID, parentID, err := c.resolveParent(ctx, dstDir)
	if err != nil {
		return err
	}
	return c.call(ctx, "/adrive/v1.0/openFile/move", map[string]any{
		"drive_id": driveID, "file_id": f.PickCode,
		"to_parent_file_id": parentID, "auto_rename": true,
	}, nil)
}

func (c *clientAliyunOpen) Copy(ctx context.Context, src, dstDir string) error {
	return fmt.Errorf("%w: 阿里云盘复制（官方 OpenAPI 未开放 copy）", ErrUploadUnsupported)
}

func (c *clientAliyunOpen) Rename(ctx context.Context, src, newName string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	driveID, _, err := c.resolveParent(ctx, cloudDirOf(src))
	if err != nil {
		return err
	}
	return c.call(ctx, "/adrive/v1.0/openFile/update", map[string]any{
		"drive_id": driveID, "file_id": f.PickCode, "name": newName,
	}, nil)
}

func (c *clientAliyunOpen) Delete(ctx context.Context, src string) error {
	f, err := c.statCloud(ctx, src)
	if err != nil {
		return err
	}
	driveID, _, err := c.resolveParent(ctx, cloudDirOf(src))
	if err != nil {
		return err
	}
	return c.call(ctx, "/adrive/v1.0/openFile/recycle", map[string]any{
		"drive_id": driveID, "file_id": f.PickCode,
	}, nil)
}

func (c *clientAliyunOpen) OfflineAdd(ctx context.Context, dir, rawURL string) error {
	return fmt.Errorf("%w: 阿里云盘离线下载", ErrUploadUnsupported)
}

func (c *clientAliyunOpen) SaveShare(ctx context.Context, dir, shareURL, pwd string) error {
	return fmt.Errorf("%w: 阿里云盘分享转存", ErrUploadUnsupported)
}
