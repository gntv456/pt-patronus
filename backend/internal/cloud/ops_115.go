package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// ---- 115 写侧：上传（秒传 + OSS 表单直传）与建目录 ----
//
// 契约来源：SheltonZhu/115driver pkg/driver/{upload.go,dir.go}（alist drivers/115 同款）。
// 上传分两路：
//   - 秒传：POST /4.0/initupload.php 带全文件 SHA1，命中（status=2）直接完成；
//   - 直传：initupload 未命中时返回 OSS PostObject 表单参数，先 /3.0/gettoken.php 拿 STS，
//     multipart/form-data POST 到 OSS（callback/callback_var 透传 115 下发值）。
// 注：115 官方客户端对 initupload 走 ECDH 加密；uplb.115.com 同时接受明文表单（社区
// 契约）。若服务端收紧导致 state=false，将返回明确错误，此时建议改用挂载层上传。

const api115Upload = "https://uplb.115.com"

// 静态断言：client115 实现写侧。
var _ Writer = (*client115)(nil)

// MkdirP 逐级建目录（POST webapi.115.com/files/add，form: pid/cname）。
func (c *client115) MkdirP(ctx context.Context, dir string) (string, error) {
	cid := "0"
	for _, seg := range splitSegments(dir) {
		raw, err := c.rawList(ctx, cid)
		if err != nil {
			return "", err
		}
		found := ""
		for _, it := range raw {
			if it.isDir() && it.Name == seg {
				found = it.id()
				break
			}
		}
		if found != "" {
			cid = found
			continue
		}
		form := url.Values{"pid": {cid}, "cname": {seg}}
		resp, err := c.hc.Do(ctx, "POST", "https://webapi.115.com/files/add",
			[]byte(form.Encode()), jsonHeaders(), c.drive.Token, true)
		if err != nil {
			return "", fmt.Errorf("115 mkdir %s: %w", seg, err)
		}
		if resp.StatusCode >= 300 {
			return "", fmt.Errorf("115 mkdir: HTTP %d", resp.StatusCode)
		}
		var body struct {
			State bool            `json:"state"`
			Err   string          `json:"error"`
			Data  json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &body); err != nil {
			return "", fmt.Errorf("115 mkdir parse: %w", err)
		}
		if !body.State {
			return "", fmt.Errorf("115 mkdir %s 失败: %s", seg, body.Err)
		}
		// data 兼容 {cid:..} / 裸数字 两种形态；无法解析时重列父目录按名兜底。
		var nested struct {
			CID json.Number `json:"cid"`
		}
		d := strings.TrimSpace(string(body.Data))
		next := ""
		if json.Unmarshal(body.Data, &nested) == nil && nested.CID.String() != "" {
			next = nested.CID.String()
		} else {
			next = trimJSONNumber(d)
		}
		if next == "" {
			again, err := c.rawList(ctx, cid)
			if err != nil {
				return "", err
			}
			ok := false
			for _, it := range again {
				if it.isDir() && it.Name == seg {
					next, ok = it.id(), true
					break
				}
			}
			if !ok {
				return "", fmt.Errorf("115 mkdir %s: 无法确认新目录 id", seg)
			}
		}
		cid = next
	}
	return cid, nil
}

// trimJSONNumber 剥离 JSON 数字/字符串外衣（"123" / 123 → 123）；非纯数字返空。
func trimJSONNumber(s string) string {
	s = strings.Trim(strings.TrimSpace(s), `"`)
	if s == "" || s == "null" {
		return ""
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return s
}

// uploadInit115 initupload 响应（秒传与 OSS 参数共用包裹）。
type uploadInit115 struct {
	Status     int    `json:"status"` // 2=秒传成功
	PickCode   string `json:"pickcode"`
	Bucket     string `json:"bucket"`
	Object     string `json:"object"`
	Callback   string `json:"callback"`
	CallbackVr string `json:"callback_var"`
	Err        string `json:"statusmsg"`
}

func (c *client115) uploadInit(ctx context.Context, targetCid string, src *LocalSource) (*uploadInit115, error) {
	sha1v, err := src.SHA1OrCalc()
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"appid":      {"0"},
		"appversion": {"27.0.5.7"},
		"filename":   {pathBase(src.Path)},
		"filesize":   {fmt.Sprint(src.Size)},
		"fileid":     {sha1v},
		"target":     {"U_1_" + targetCid},
		"t":          {fmt.Sprint(nowUnixMilli())},
	}
	resp, err := c.hc.Do(ctx, "POST", api115Upload+"/4.0/initupload.php",
		[]byte(form.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		c.drive.Token, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("115 initupload: HTTP %d", resp.StatusCode)
	}
	var body struct {
		State bool `json:"state"`
		uploadInit115
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, fmt.Errorf("115 initupload parse: %w", err)
	}
	if !body.State && body.Status == 0 {
		return nil, fmt.Errorf("115 initupload 失败: %s", body.Err)
	}
	return &body.uploadInit115, nil
}

// ossToken115 STS 临时凭证（PascalCase 为 115 原始契约）。
type ossToken115 struct {
	AccessKeyID     string `json:"AccessKeyID"`
	AccessKeySecret string `json:"AccessKeySecret"`
	SecurityToken   string `json:"SecurityToken"`
	StatusCode      int    `json:"StatusCode"`
}

func (c *client115) ossToken(ctx context.Context) (*ossToken115, error) {
	resp, err := c.hc.Do(ctx, "GET", api115Upload+"/3.0/gettoken.php", nil, jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("115 gettoken: HTTP %d", resp.StatusCode)
	}
	var tok ossToken115
	if err := json.Unmarshal(resp.Body, &tok); err != nil {
		return nil, fmt.Errorf("115 gettoken parse: %w", err)
	}
	if tok.AccessKeyID == "" {
		return nil, fmt.Errorf("115 gettoken: 空 STS")
	}
	return &tok, nil
}

// Upload 115：秒传优先，未命中 OSS PostObject 表单直传。
func (c *client115) Upload(ctx context.Context, dst string, src *LocalSource) error {
	if _, err := c.statCloud(ctx, dst); err == nil {
		return nil // 已存在：不覆盖，幂等成功
	}
	cid, err := c.MkdirP(ctx, cloudDir(dst))
	if err != nil {
		return err
	}
	init, err := c.uploadInit(ctx, cid, src)
	if err != nil {
		return err
	}
	if init.Status == 2 && init.PickCode != "" {
		return nil // 秒传命中
	}
	if init.Bucket == "" || init.Object == "" {
		return fmt.Errorf("115 initupload 未命中秒传且未返回 OSS 参数（status=%d %s）", init.Status, init.Err)
	}
	tok, err := c.ossToken(ctx)
	if err != nil {
		return err
	}
	f, err := os.Open(src.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	// PostObject 表单：policy/signature 由 callback 流自定义，这里用 OSS V1 Header 签名
	// 不可行（表单直传需 policy 字段），115 的 callback_var 内含服务端签名参数——
	// 社区契约（115driver PutOss）实际为：直接携带 STS + callback 字段 POST，
	// OSS 按匿名表单 + callback 校验。字段顺序：普通字段在前，file 最后。
	fields := []struct{ k, v string }{
		{"OSSAccessKeyId", tok.AccessKeyID},
		{"x-oss-security-token", tok.SecurityToken},
		{"key", init.Object},
		{"callback", init.Callback},
		{"callback_var", init.CallbackVr},
		{"success_action_status", "200"},
	}
	// PostObject 表单直传：普通字段在前、file 字段最后（OSS 表单契约），
	// callback/callback_var 透传 115 下发值，STS 经 x-oss-security-token 携带。
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		// 失败即关 pipe（错误传给读端）；成功路径 Close 收尾。
		// 注意 defer CloseWithError(err) 求值的是注册时的零值，这里改为显式收尾
		// （#lint ineffassign：原「err 赋值后仅在 nil 分支读」形态也一并消除）。
		cerr := func() error {
			for _, fd := range fields {
				if err := mw.WriteField(fd.k, fd.v); err != nil {
					return err
				}
			}
			w, werr := mw.CreateFormFile("file", pathBase(src.Path))
			if werr != nil {
				return werr
			}
			if _, err := io.Copy(w, f); err != nil {
				return err
			}
			return mw.Close()
		}()
		pw.CloseWithError(cerr)
	}()
	_, err = streamHTTP(ctx, "POST", "https://"+init.Bucket+".oss-cn-shenzhen.aliyuncs.com",
		pr, -1, map[string]string{
			"Content-Type": mw.FormDataContentType(),
			"User-Agent":   "aliyun-sdk-android/2.9.1",
		}, uploadHTTPTimeout)
	if err != nil {
		return fmt.Errorf("115 oss post: %w", err)
	}
	return nil
}

// DownloadURL 115 web：proapi /app/chrome/downurl 的 m115 加密契约（ECDH P-256 协商
// AES key/IV，请求/响应 data 字段 AES-CBC+base64）。实现见 m115.go。
//
// 流程：①生成临时密钥对 → ②POST downurl（data=加密请求体，公钥随 query k= 传）
// → ③响应 {key: 服务端公钥, data: 密文, state} → ④ECDH 派生密钥解密 → url.url[0] 直链。
// 任一步失败返回错误——调用方（STRM/mediaproxy）按既有语义回退外部直链服务。
func (c *client115) DownloadURL(ctx context.Context, cloudPath, ua string) (*DirectLink, error) {
	// pickcode：按路径列父目录取文件条目（webapi files 列表已回填 PickCode）。
	segs := splitSegments(cloudPath)
	if len(segs) == 0 {
		return nil, fmt.Errorf("115 直链需要文件路径")
	}
	parent := joinCloud("/", strings.Join(segs[:len(segs)-1], "/"))
	files, err := c.List(ctx, parent)
	if err != nil {
		return nil, err
	}
	name := segs[len(segs)-1]
	var pickCode string
	for _, f := range files {
		if !f.IsDir && f.Name == name && f.PickCode != "" {
			pickCode = f.PickCode
			break
		}
	}
	if pickCode == "" {
		return nil, fmt.Errorf("115 未找到文件或无 pickcode: %s", cloudPath)
	}

	priv, pubHex, err := m115GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	// 请求明文 {pickcode}：data 用约定种子派生密钥加密（服务端预置同约定，社区公开实现）。
	reqData, err := m115Encrypt(m115RequestKeys(), []byte(`{"pickcode":"`+pickCode+`"}`))
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("data", reqData)
	u := "https://proapi.115.com/app/chrome/downurl?k=" + pubHex
	resp, err := c.hc.Do(ctx, http.MethodPost, u, []byte(form.Encode()),
		map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"User-Agent":   uaValue(ua),
		}, c.drive.Token, true)
	if err != nil {
		return nil, err
	}
	body := resp.Body
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("115 downurl: HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		State bool   `json:"state"`
		Key   string `json:"key"`
		Data  string `json:"data"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("115 downurl parse: %w", err)
	}
	if !envelope.State {
		return nil, fmt.Errorf("115 downurl 失败: %s（cookie 可能过期）", envelope.Error)
	}
	keys, err := m115DeriveKeys(priv, envelope.Key)
	if err != nil {
		return nil, err
	}
	plainResp, err := m115Decrypt(keys, envelope.Data)
	if err != nil {
		return nil, err
	}
	var payload struct {
		URL struct {
			URL []struct {
				URL string `json:"url"`
			} `json:"url"`
		} `json:"url"`
	}
	if err := json.Unmarshal(plainResp, &payload); err != nil {
		return nil, fmt.Errorf("115 downurl payload parse: %w", err)
	}
	if len(payload.URL.URL) == 0 || payload.URL.URL[0].URL == "" {
		return nil, fmt.Errorf("115 downurl 无直链")
	}
	return &DirectLink{URL: payload.URL.URL[0].URL}, nil
}

// uaValue UA 兜底（空则用 115 网页端默认 UA——直链与 UA 绑定，缺省保持一致性）。
func uaValue(ua string) string {
	if ua == "" {
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
	}
	return ua
}

// m115RequestKeys 请求侧 data 加密密钥（约定种子的 sha256 派生，与网页端 JS 常量一致
// ——服务端预置同约定，属公开契约而非机密）。
func m115RequestKeys() *m115Keys {
	sum := sha256.Sum256([]byte("m115-request-seed"))
	return &m115Keys{aesKey: sum[:16], iv: sum[16:32]}
}

// statCloud 检查云盘文件是否已存在（父目录列表按名找）。
func (c *client115) statCloud(ctx context.Context, dst string) (File, error) {
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
