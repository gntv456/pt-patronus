package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ---- 阿里云盘 OpenAPI 客户端（openapi.alipan.com，Bearer access_token） ----
//
// 契约来源：alist drivers/aliyundrive_open。Token = access_token（refresh 由用户在
// 阿里开放平台侧或经本端 oauth 端点刷新——refresh 轮换存回 drive token 的能力由
// chain 层 token 自愈处理，客户端每次请求只用当前 Token）。
// 列目录：POST /adrive/v1.0/openFile/list（marker 翻页）。
// 上传：create（>100KB 带 pre_hash 秒传探测；PreHashMatched → sha1+proof_code 秒传）→
// 预签名 URL 分片 PUT（无 Authorization）→ complete。
// 直链：POST /adrive/v1.0/openFile/getDownloadUrl（expire_sec=14400，无需绑定头）。

const apiAliyunOpen = "https://openapi.alipan.com"

// 静态断言：clientAliyunOpen 实现 Client + Writer。
var (
	_ Client = (*clientAliyunOpen)(nil)
	_ Writer = (*clientAliyunOpen)(nil)
)

type clientAliyunOpen struct {
	drive          Drive // Token = access_token 或 TokenPair JSON（含 refresh_token/client 凭据）
	hc             httpDoer
	onTokenRefresh func(tp TokenPair) // 刷新回写（chain 注入）
}

// SetTokenRefreshHook 实现 cloud.TokenRefresher。
func (c *clientAliyunOpen) SetTokenRefreshHook(fn func(newToken string)) {
	c.onTokenRefresh = func(tp TokenPair) { fn(EncodeTokenPair(tp)) }
}

// bearer 当前 access_token。
func (c *clientAliyunOpen) bearer() string {
	return DecodeTokenPair(c.drive.Token).AccessToken
}

// tryRefresh OAuth refresh_token 换新（client_id/secret 取自 TokenPair 附加字段；
// 未存则报错提示重新授权）。
func (c *clientAliyunOpen) tryRefresh(ctx context.Context) error {
	tp := aliyunCred(DecodeTokenPair(c.drive.Token))
	if tp.RefreshToken == "" || tp.ClientID == "" || tp.ClientSecret == "" {
		return fmt.Errorf("aliyun: token 失效且缺少 refresh 凭据（请重新授权或填完整 token JSON）")
	}
	body, _ := json.Marshal(map[string]string{
		"client_id": tp.ClientID, "client_secret": tp.ClientSecret,
		"grant_type": "refresh_token", "refresh_token": tp.RefreshToken,
	})
	resp, err := c.hc.Do(ctx, "POST", apiAliyunOpen+"/oauth/access_token", body,
		map[string]string{"Content-Type": "application/json"}, "", true)
	if err != nil {
		return fmt.Errorf("aliyun refresh: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("aliyun refresh: HTTP %d", resp.StatusCode)
	}
	var np struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(resp.Body, &np); err != nil || np.AccessToken == "" {
		return fmt.Errorf("aliyun refresh: 响应无效")
	}
	nt := TokenPair{AccessToken: np.AccessToken, RefreshToken: np.RefreshToken,
		ClientID: tp.ClientID, ClientSecret: tp.ClientSecret}
	c.drive.Token = EncodeTokenPair(nt)
	if c.onTokenRefresh != nil {
		c.onTokenRefresh(nt)
	}
	return nil
}

// aliyunCred TokenPair 的阿里扩展字段（client 凭据随 token 持久化）。
type aliyunCredT = TokenPair

func aliyunCred(tp TokenPair) aliyunCredT { return tp }

func (c *clientAliyunOpen) Type() string { return "aliyun_open" }

func (c *clientAliyunOpen) authHeaders() map[string]string {
	h := jsonHeaders()
	h["Content-Type"] = "application/json"
	if tok := c.bearer(); tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	return h
}

// call POST JSON + 解析 {code,message} 错误信封。401/AccessTokenInvalid 自愈：刷新令牌重试一次。
func (c *clientAliyunOpen) call(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(ctx, "POST", apiAliyunOpen+path, body, c.authHeaders(), "", true)
	if err != nil {
		return err
	}
	if aliyunTokenInvalid(resp.StatusCode, resp.Body) {
		if rerr := c.tryRefresh(ctx); rerr == nil {
			resp, err = c.hc.Do(ctx, "POST", apiAliyunOpen+path, body, c.authHeaders(), "", true)
			if err != nil {
				return err
			}
		}
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("aliyun %s: HTTP %d %s", path, resp.StatusCode, aliyunErrMsg(resp.Body))
	}
	if out != nil && len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, out); err != nil {
			return fmt.Errorf("aliyun %s parse: %w", path, err)
		}
	}
	return nil
}

// aliyunTokenInvalid 令牌失效判定（HTTP 401 或业务 code AccessTokenInvalid/Expired/I400JD）。
func aliyunTokenInvalid(status int, body []byte) bool {
	if status == 401 {
		return true
	}
	s := string(body)
	return strings.Contains(s, "AccessTokenInvalid") ||
		strings.Contains(s, "AccessTokenExpired") ||
		strings.Contains(s, "I400JD")
}

// aliyunErrMsg 错误信封 message（截断）。
func aliyunErrMsg(body []byte) string {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &e) == nil && (e.Code != "" || e.Message != "") {
		msg := e.Code + ": " + e.Message
		if len(msg) > 120 {
			msg = msg[:120]
		}
		return msg
	}
	return ""
}

// aliFile 阿里文件对象（list/create 共用）。
type aliFile struct {
	FileID   string `json:"file_id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Type     string `json:"type"` // folder | file
	Category string `json:"category"`
}

func (f aliFile) isDir() bool { return f.Type == "folder" }

// driveID 惰性获取（default drive；缓存于调用内）。
func (c *clientAliyunOpen) driveID(ctx context.Context) (string, error) {
	var out struct {
		DefaultDriveID string `json:"default_drive_id"`
	}
	if err := c.call(ctx, "/adrive/v1.0/user/getDriveInfo", map[string]any{}, &out); err != nil {
		return "", err
	}
	if out.DefaultDriveID == "" {
		return "", fmt.Errorf("aliyun: 未返回 default_drive_id")
	}
	return out.DefaultDriveID, nil
}

// resolveParent 逐级解析目录到 file_id（根="root"）。
func (c *clientAliyunOpen) resolveParent(ctx context.Context, dir string) (driveID, parentID string, err error) {
	driveID, err = c.driveID(ctx)
	if err != nil {
		return "", "", err
	}
	parentID = "root"
	for _, seg := range splitSegments(dir) {
		items, _, err := c.listDir(ctx, driveID, parentID)
		if err != nil {
			return "", "", err
		}
		next := ""
		for _, it := range items {
			if it.isDir() && it.Name == seg {
				next = it.FileID
				break
			}
		}
		if next == "" {
			return "", "", fmt.Errorf("aliyun 路径不存在: %s", seg)
		}
		parentID = next
	}
	return driveID, parentID, nil
}

// listDir 单页循环 marker 翻页。
func (c *clientAliyunOpen) listDir(ctx context.Context, driveID, parentID string) ([]aliFile, string, error) {
	var all []aliFile
	marker := ""
	for page := 0; page < 200; page++ {
		var out struct {
			Items      []aliFile `json:"items"`
			NextMarker string    `json:"next_marker"`
		}
		in := map[string]any{
			"drive_id": driveID, "parent_file_id": parentID,
			"limit": 200, "order_by": "name", "order_direction": "ASC",
		}
		if marker != "" {
			in["marker"] = marker
		}
		if err := c.call(ctx, "/adrive/v1.0/openFile/list", in, &out); err != nil {
			return nil, "", err
		}
		all = append(all, out.Items...)
		if out.NextMarker == "" {
			return all, "", nil
		}
		marker = out.NextMarker
	}
	return all, "", nil
}

func (c *clientAliyunOpen) List(ctx context.Context, dir string) ([]File, error) {
	driveID, parentID, err := c.resolveParent(ctx, dir)
	if err != nil {
		return nil, err
	}
	items, _, err := c.listDir(ctx, driveID, parentID)
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(items))
	for _, it := range items {
		out = append(out, File{
			Name: it.Name, Path: joinCloud(dir, it.Name),
			IsDir: it.isDir(), Size: it.Size, PickCode: it.FileID,
		})
	}
	return out, nil
}

func (c *clientAliyunOpen) Test(ctx context.Context) (string, error) {
	driveID, err := c.driveID(ctx)
	if err != nil {
		return "", err
	}
	items, _, err := c.listDir(ctx, driveID, "root")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("阿里云盘 · 根目录 %d 个条目", len(items)), nil
}

func (c *clientAliyunOpen) Quota(ctx context.Context) (*Quota, error) {
	return &Quota{}, nil // 开放平台配额字段未暴露；best-effort 零值
}

// MkdirP 逐级建目录（POST /adrive/v1.0/openFile/create type=folder）。
func (c *clientAliyunOpen) MkdirP(ctx context.Context, dir string) (string, error) {
	// 成功路径只需 parentID（dir 已存在，直接返回其 id）；失败路径才需要 driveID
	// 逐级补建——resolveParent 的 driveID 返回值在成功分支未被消费，不接（ineffassign）。
	_, parentID, perr := c.resolveParent(ctx, dir)
	var driveID string
	if perr != nil {
		// 父目录缺失：逐级补建（resolveParent 报错则从根重建路径）
		var derr error
		driveID, derr = c.driveID(ctx)
		if derr != nil {
			return "", derr
		}
		parentID = "root"
		for _, seg := range splitSegments(dir) {
			items, _, err := c.listDir(ctx, driveID, parentID)
			if err != nil {
				return "", err
			}
			next := ""
			for _, it := range items {
				if it.isDir() && it.Name == seg {
					next = it.FileID
					break
				}
			}
			if next == "" {
				var created aliFile
				if err := c.call(ctx, "/adrive/v1.0/openFile/create", map[string]any{
					"drive_id": driveID, "parent_file_id": parentID,
					"name": seg, "type": "folder", "check_name_mode": "refuse",
				}, &created); err != nil {
					return "", fmt.Errorf("aliyun mkdir %s: %w", seg, err)
				}
				next = created.FileID
			}
			parentID = next
		}
		return parentID, nil
	}
	return parentID, nil
}

func (c *clientAliyunOpen) DownloadURL(ctx context.Context, cloudPath, ua string) (*DirectLink, error) {
	f, err := c.statCloud(ctx, cloudPath)
	if err != nil {
		return nil, err
	}
	if f.IsDir {
		return nil, fmt.Errorf("aliyun 直链: %s 是目录", cloudPath)
	}
	driveID, parentID, err := c.resolveParent(ctx, cloudDirOf(cloudPath))
	if err != nil {
		return nil, err
	}
	_ = parentID
	var out struct {
		URL string `json:"url"`
	}
	if err := c.call(ctx, "/adrive/v1.0/openFile/getDownloadUrl", map[string]any{
		"drive_id": driveID, "file_id": f.PickCode, "expire_sec": 14400,
	}, &out); err != nil {
		return nil, fmt.Errorf("aliyun 直链解析失败: %w", err)
	}
	if out.URL == "" {
		return nil, fmt.Errorf("aliyun 直链: 响应无 url")
	}
	h := map[string]string{}
	if ua != "" {
		h["User-Agent"] = ua
	}
	return &DirectLink{URL: out.URL, Header: h}, nil
}

// statCloud 父目录列表按名找。
func (c *clientAliyunOpen) statCloud(ctx context.Context, dst string) (File, error) {
	files, err := c.List(ctx, cloudDirOf(dst))
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
