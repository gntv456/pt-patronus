package cloud

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ---- 夸克写侧：上传（预创建→秒传hash→分片auth→OSS PUT→auth完成→finish）+ 直链 ----
//
// 契约来源：alist drivers/quark_uc/util.go（现行驱动源码逐字段核对）。
// 分片 auth_meta 为 OSS V1 string-to-sign 变体（x-oss-date / x-oss-user-agent 参与签名），
// 服务端 /file/upload/auth 返回 auth_key 作为 PUT 的 Authorization。

// 静态断言：clientQuark 实现写侧。
var _ Writer = (*clientQuark)(nil)

const quarkUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) quark-cloud-drive/2.5.20 Chrome/100.0.4896.160 Electron/18.3.5.4-b478491100 Safari/537.36 Channel/pckk_other_ch"
const quarkOSSUA = "aliyun-sdk-js/6.6.1 Chrome/98.0.4758.80 on Windows 10 64-bit"

// quarkHeaders 夸克 API 请求头（Referer/UA 为硬契约，缺失会被拒）。
func quarkHeaders() map[string]string {
	return map[string]string{
		"Referer": "https://pan.quark.cn/",
		"UA":      quarkUA,
	}
}

// quarkUploadPre 预创建响应。
type quarkUploadPre struct {
	TaskID    string        `json:"task_id"`
	Finish    bool          `json:"finish"`
	UploadID  string        `json:"uploadID"`
	ObjKey    string        `json:"obj_key"`
	Fid       string        `json:"fid"`
	Bucket    string        `json:"bucket"`
	UploadURL string        `json:"upload_url"`
	AuthInfo  string        `json:"auth_info"`
	Callback  quarkCallback `json:"callback"`
}

// quarkCallback 预创建下发的 OSS callback（complete 阶段签名与请求头都要用）。
type quarkCallback struct {
	Callback    string `json:"callback"`
	CallbackVar string `json:"callback_var"`
}
type quarkUploadPreWrap struct {
	Status int            `json:"status"`
	Code   int            `json:"code"`
	Msg    string         `json:"message"`
	Data   quarkUploadPre `json:"data"`
	// metadata.part_size 为分片大小。
	Metadata struct {
		PartSize int64 `json:"part_size"`
	} `json:"metadata"`
}

func (w *quarkUploadPreWrap) failed() bool {
	return w.Status >= 400 || (w.Code != 0)
}

// MkdirP 夸克：POST /file（dir_init_lock 表单字段）逐级建目录。
func (c *clientQuark) MkdirP(ctx context.Context, dir string) (string, error) {
	fid := "0"
	for _, seg := range splitSegments(dir) {
		raw, err := c.rawList(ctx, fid)
		if err != nil {
			return "", err
		}
		found := ""
		for _, it := range raw {
			if it.isDir() && it.Name == seg {
				found = it.FID
				break
			}
		}
		if found != "" {
			fid = found
			continue
		}
		body, _ := json.Marshal(map[string]any{
			"dir_init_lock": false, "dir_path": "", "file_name": seg, "pdir_fid": fid,
		})
		resp, err := c.hc.Do(ctx, "POST", apiQuark+"/file?pr=ucpro&fr=pc", body, jsonHeaders(), c.drive.Token, true)
		if err != nil {
			return "", fmt.Errorf("quark mkdir %s: %w", seg, err)
		}
		if resp.StatusCode >= 300 {
			return "", fmt.Errorf("quark mkdir: HTTP %d", resp.StatusCode)
		}
		var wrap struct {
			Status int    `json:"status"`
			Code   int    `json:"code"`
			Msg    string `json:"message"`
			Data   struct {
				FID string `json:"fid"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &wrap); err != nil {
			return "", fmt.Errorf("quark mkdir parse: %w", err)
		}
		if wrap.Status >= 400 || wrap.Code != 0 {
			return "", fmt.Errorf("quark mkdir %s 失败: %s", seg, wrap.Msg)
		}
		if wrap.Data.FID != "" {
			fid = wrap.Data.FID
			continue
		}
		again, err := c.rawList(ctx, fid)
		if err != nil {
			return "", err
		}
		ok := false
		for _, it := range again {
			if it.isDir() && it.Name == seg {
				fid, ok = it.FID, true
				break
			}
		}
		if !ok {
			return "", fmt.Errorf("quark mkdir %s: 无法确认新目录 fid", seg)
		}
	}
	return fid, nil
}

// Upload 夸克全流程。
func (c *clientQuark) Upload(ctx context.Context, dst string, src *LocalSource) error {
	if _, err := c.statCloud(ctx, dst); err == nil {
		return nil
	}
	pdir, err := c.MkdirP(ctx, cloudDir(dst))
	if err != nil {
		return err
	}
	// 1) 预创建
	preBody, _ := json.Marshal(map[string]any{
		"pdir_fid": pdir, "file_name": pathBase(src.Path), "size": src.Size,
		"format_type": mimeOf(src.Path), "l_created_at": nowUnixMilli(), "l_updated_at": nowUnixMilli(),
		"ccp_hash_update": true, "dir_name": "",
	})
	resp, err := c.hc.Do(ctx, "POST", apiQuark+"/file/upload/pre?pr=ucpro&fr=pc", preBody, jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return fmt.Errorf("quark pre: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("quark pre: HTTP %d", resp.StatusCode)
	}
	var pre quarkUploadPreWrap
	if err := json.Unmarshal(resp.Body, &pre); err != nil {
		return fmt.Errorf("quark pre parse: %w", err)
	}
	if pre.failed() {
		return fmt.Errorf("quark pre 失败: %s", pre.Msg)
	}
	// 2) 秒传 hash（md5+sha1）
	md5v, sha1v, err := fileMD5SHA1(src.Path)
	if err != nil {
		return err
	}
	hashBody, _ := json.Marshal(map[string]any{
		"md5": md5v, "sha1": sha1v, "task_id": pre.Data.TaskID,
	})
	resp, err = c.hc.Do(ctx, "POST", apiQuark+"/file/update/hash?pr=ucpro&fr=pc", hashBody, jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return fmt.Errorf("quark hash: %w", err)
	}
	var hashWrap struct {
		Status int `json:"status"`
		Code   int `json:"code"`
		Data   struct {
			Finish bool   `json:"finish"`
			Fid    string `json:"fid"`
		} `json:"data"`
	}
	if json.Unmarshal(resp.Body, &hashWrap) == nil && hashWrap.Data.Finish {
		return nil // 秒传成功
	}
	// 3) 分片上传
	if pre.Metadata.PartSize <= 0 {
		return fmt.Errorf("quark pre: 未返回 part_size")
	}
	f, err := os.Open(src.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	var etags []string
	buf := make([]byte, pre.Metadata.PartSize)
	for part := 1; ; part++ {
		n, rerr := io.ReadFull(f, buf)
		if n == 0 {
			break
		}
		chunk := buf[:n]
		etag, err := c.putPart(ctx, &pre, part, chunk, mimeOf(src.Path))
		if err != nil {
			return fmt.Errorf("quark part %d: %w", part, err)
		}
		etags = append(etags, etag)
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	// 4) complete（auth_meta 为 POST 签名，body 为分片 XML）
	if err := c.completeMultipart(ctx, &pre, etags); err != nil {
		return err
	}
	// 5) finish
	finBody, _ := json.Marshal(map[string]any{"obj_key": pre.Data.ObjKey, "task_id": pre.Data.TaskID})
	resp, err = c.hc.Do(ctx, "POST", apiQuark+"/file/upload/finish?pr=ucpro&fr=pc", finBody, jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return fmt.Errorf("quark finish: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("quark finish: HTTP %d", resp.StatusCode)
	}
	return nil
}

// quarkAuthResp /file/upload/auth 响应。
type quarkAuthResp struct {
	Status int    `json:"status"`
	Code   int    `json:"code"`
	Msg    string `json:"message"`
	Data   struct {
		AuthKey string            `json:"auth_key"`
		Headers map[string]string `json:"headers"`
	} `json:"data"`
}

// authMetaPart 分片 PUT 的 auth_meta（canonical string）。
func authMetaPart(mime, timeStr, bucket, objKey string, partNumber int, uploadID string) string {
	return "PUT\n\n" + mime + "\n" + timeStr + "\n" +
		"x-oss-date:" + timeStr + "\n" +
		"x-oss-user-agent:" + quarkOSSUA + "\n" +
		"/" + bucket + "/" + objKey + "?partNumber=" + strconv.Itoa(partNumber) + "&uploadId=" + uploadID
}

// authMetaComplete complete 阶段的 auth_meta。
func authMetaComplete(contentMd5, timeStr, callbackB64, bucket, objKey, uploadID string) string {
	return "POST\n" + contentMd5 + "\napplication/xml\n" + timeStr + "\n" +
		"x-oss-callback:" + callbackB64 + "\n" +
		"x-oss-date:" + timeStr + "\n" +
		"x-oss-user-agent:" + quarkOSSUA + "\n" +
		"/" + bucket + "/" + objKey + "?uploadId=" + uploadID
}

// quarkTime OSS 日期串（RFC1123 GMT）。
func quarkTime() string { return time.Now().UTC().Format(http.TimeFormat) }

// putPart 申请分片签名并 PUT 到 OSS。
func (c *clientQuark) putPart(ctx context.Context, pre *quarkUploadPreWrap, part int, chunk []byte, mime string) (string, error) {
	timeStr := quarkTime()
	meta := authMetaPart(mime, timeStr, pre.Data.Bucket, pre.Data.ObjKey, part, pre.Data.UploadID)
	authKey, _, err := c.uploadAuth(ctx, pre.Data.AuthInfo, meta, pre.Data.TaskID)
	if err != nil {
		return "", err
	}
	host := strings.TrimPrefix(pre.Data.UploadURL, "http://")
	u := "https://" + pre.Data.Bucket + "." + host + "/" + pre.Data.ObjKey +
		"?partNumber=" + strconv.Itoa(part) + "&uploadId=" + pre.Data.UploadID
	resp, err := streamHTTP(ctx, "PUT", u, bytes.NewReader(chunk), int64(len(chunk)), map[string]string{
		"Authorization":    authKey,
		"Content-Type":     mime,
		"Referer":          "https://pan.quark.cn/",
		"x-oss-date":       timeStr,
		"x-oss-user-agent": quarkOSSUA,
	}, uploadHTTPTimeout)
	if err != nil {
		return "", err
	}
	return resp.Header.Get("Etag"), nil
}

// uploadAuth POST /file/upload/auth 换 auth_key。
func (c *clientQuark) uploadAuth(ctx context.Context, authInfo, authMeta, taskID string) (authKey, callbackURL string, err error) {
	body, _ := json.Marshal(map[string]any{"auth_info": authInfo, "auth_meta": authMeta, "task_id": taskID})
	resp, err := c.hc.Do(ctx, "POST", apiQuark+"/file/upload/auth?pr=ucpro&fr=pc", body, jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("quark auth: HTTP %d", resp.StatusCode)
	}
	var w quarkAuthResp
	if err := json.Unmarshal(resp.Body, &w); err != nil {
		return "", "", fmt.Errorf("quark auth parse: %w", err)
	}
	if w.Status >= 400 || w.Code != 0 {
		return "", "", fmt.Errorf("quark auth 失败: %s", w.Msg)
	}
	return w.Data.AuthKey, "", nil
}

// completeMultipart complete 阶段：auth 签名 + OSS POST XML + 115 式 callback 头。
func (c *clientQuark) completeMultipart(ctx context.Context, pre *quarkUploadPreWrap, etags []string) error {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><CompleteMultipartUpload>`)
	for i, et := range etags {
		b.WriteString("<Part><PartNumber>")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString("</PartNumber><ETag>")
		b.WriteString(et)
		b.WriteString("</ETag></Part>")
	}
	b.WriteString("</CompleteMultipartUpload>")
	xmlBody := b.String()

	timeStr := quarkTime()
	contentMd5 := base64.StdEncoding.EncodeToString(md5sum([]byte(xmlBody)))
	headers := map[string]string{
		"Content-MD5":      contentMd5,
		"Content-Type":     "application/xml",
		"Referer":          "https://pan.quark.cn/",
		"x-oss-date":       timeStr,
		"x-oss-user-agent": quarkOSSUA,
	}
	var meta string
	if pre.Data.Callback.Callback != "" {
		// alist 精确契约：x-oss-callback=base64(json(callback对象))，参与 auth_meta 签名并透传请求头
		callbackB64 := b64(callbackVarQuark(pre.Data.Callback))
		meta = authMetaComplete(contentMd5, timeStr, callbackB64, pre.Data.Bucket, pre.Data.ObjKey, pre.Data.UploadID)
		headers["x-oss-callback"] = callbackB64
	} else {
		// 预创建未下发 callback：按无 callback 签名兜底（失败由任务队列重试）
		meta = authMetaComplete(contentMd5, timeStr, "", pre.Data.Bucket, pre.Data.ObjKey, pre.Data.UploadID)
	}
	authKey, _, err := c.uploadAuth(ctx, pre.Data.AuthInfo, meta, pre.Data.TaskID)
	if err != nil {
		return err
	}
	headers["Authorization"] = authKey
	host := strings.TrimPrefix(pre.Data.UploadURL, "http://")
	u := "https://" + pre.Data.Bucket + "." + host + "/" + pre.Data.ObjKey + "?uploadId=" + pre.Data.UploadID
	_, err = streamHTTP(ctx, "POST", u, strings.NewReader(xmlBody), int64(len(xmlBody)), headers, uploadHTTPTimeout)
	return err
}

// callbackVarQuark 序列化完整 callback 对象（callback+callback_var）为 JSON 供 base64。
func callbackVarQuark(cb quarkCallback) string {
	b, err := json.Marshal(cb)
	if err != nil {
		return cb.Callback
	}
	return string(b)
}

// DownloadURL 夸克：POST /file/download（fids 数组）→ data[0].download_url。
// 直链与请求时 Cookie/UA 绑定——下载必须带同 Cookie + Referer + quark UA。
func (c *clientQuark) DownloadURL(ctx context.Context, cloudPath, ua string) (*DirectLink, error) {
	f, err := c.statCloud(ctx, cloudPath)
	if err != nil {
		return nil, err
	}
	if f.IsDir {
		return nil, fmt.Errorf("夸克直链: %s 是目录", cloudPath)
	}
	body, _ := json.Marshal(map[string]any{"fids": []string{f.PickCode}})
	resp, err := c.hc.Do(ctx, "POST", apiQuark+"/file/download?pr=ucpro&fr=pc", body, jsonHeaders(), c.drive.Token, true)
	if err != nil {
		return nil, fmt.Errorf("quark download: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("quark download: HTTP %d", resp.StatusCode)
	}
	var wrap struct {
		Status int    `json:"status"`
		Code   int    `json:"code"`
		Msg    string `json:"message"`
		Data   []struct {
			DownloadURL string `json:"download_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &wrap); err != nil {
		return nil, fmt.Errorf("quark download parse: %w", err)
	}
	if wrap.Status >= 400 || wrap.Code != 0 || len(wrap.Data) == 0 || wrap.Data[0].DownloadURL == "" {
		return nil, fmt.Errorf("夸克直链解析失败: %s", wrap.Msg)
	}
	h := map[string]string{"Referer": "https://pan.quark.cn/"}
	if ua != "" {
		h["User-Agent"] = ua
	} else {
		h["User-Agent"] = quarkUA
	}
	if c.drive.Token != "" {
		h["Cookie"] = c.drive.Token
	}
	return &DirectLink{URL: wrap.Data[0].DownloadURL, Header: h}, nil
}

// statCloud 夸克：父目录列表按名找。
func (c *clientQuark) statCloud(ctx context.Context, dst string) (File, error) {
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

// fileMD5SHA1 一次读文件同时算 MD5（小写）与 SHA1（大写）。夸克秒传 hash 需要。
func fileMD5SHA1(path string) (md5s, sha1s string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	hm := md5.New()
	hs := sha1.New()
	r := io.TeeReader(f, hs)
	if _, err := io.Copy(hm, r); err != nil {
		return "", "", err
	}
	return hex.EncodeToString(hm.Sum(nil)), strings.ToUpper(hex.EncodeToString(hs.Sum(nil))), nil
}

// md5sum 字节切片 MD5 原始值。
func md5sum(b []byte) []byte {
	h := md5.New()
	h.Write(b)
	return h.Sum(nil)
}
