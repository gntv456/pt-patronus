package cloud

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
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
