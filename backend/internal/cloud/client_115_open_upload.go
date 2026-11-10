// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

// 115 开放平台秒传/分片上传实现。从 client_115_open.go 迁出。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Upload 115开放：init 双哈希秒传（含二次校验循环）→ OSS PutObject（STS + callback）。
func (c *client115Open) Upload(ctx context.Context, dst string, src *LocalSource) error {
	if _, err := c.statCloud(ctx, dst); err == nil {
		return nil
	}
	cid, err := c.MkdirP(ctx, cloudDirOf(dst))
	if err != nil {
		return err
	}
	sha1v, err := src.SHA1OrCalc()
	if err != nil {
		return err
	}
	preID, err := fileSHA1Prefix(src.Path, 128*1024)
	if err != nil {
		return err
	}
	form := url.Values{
		"file_name": {pathBase(src.Path)},
		"file_size": {strconv.FormatInt(src.Size, 10)},
		"target":    {"U_1_" + cid},
		"fileid":    {sha1v},
		"preid":     {preID},
	}
	var init *uploadInitOpen
	for attempt := 0; attempt < 3; attempt++ {
		sc, rb, err := c.doHeal(ctx, "POST", api115Open+"/open/upload/init", []byte(form.Encode()))
		if err != nil {
			return fmt.Errorf("115_open init: %w", err)
		}
		if sc >= 300 {
			return fmt.Errorf("115_open init: HTTP %d", sc)
		}
		var body struct {
			State bool `json:"state"`
			uploadInitOpen
		}
		if err := json.Unmarshal(rb, &body); err != nil {
			return fmt.Errorf("115_open init parse: %w", err)
		}
		init = &body.uploadInitOpen
		if init.Status == 2 {
			return nil // 秒传成功
		}
		if (init.Status == 6 || init.Status == 7 || init.Status == 8) && init.SignKey != "" && init.SignCheck != "" {
			sv, err := fileSHA1Range(src.Path, init.SignCheck)
			if err != nil {
				return err
			}
			form.Set("sign_key", init.SignKey)
			form.Set("sign_val", sv)
			continue // 二次校验重发
		}
		break
	}
	if init == nil || init.Bucket == "" || init.Object == "" {
		return fmt.Errorf("115_open 上传未命中秒传且未返回 OSS 参数（status=%v）", initStatus(init))
	}
	// OSS STS
	resp, err := c.hc.Do(ctx, "GET", api115Open+"/open/upload/get_token", nil, c.authHeaders(), "", true)
	if err != nil {
		return fmt.Errorf("115_open get_token: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("115_open get_token: HTTP %d", resp.StatusCode)
	}
	var tok struct {
		AccessKeyID     string `json:"AccessKeyId"`
		AccessKeySecret string `json:"AccessKeySecret"`
		SecurityToken   string `json:"SecurityToken"`
		Endpoint        string `json:"endpoint"`
	}
	if err := json.Unmarshal(resp.Body, &tok); err != nil {
		return fmt.Errorf("115_open get_token parse: %w", err)
	}
	if tok.AccessKeyID == "" || tok.Endpoint == "" {
		return fmt.Errorf("115_open get_token: 空 STS")
	}
	// PutObject 直传：OSS V1 Header 签名（Date 用 x-oss-date 规避代理改写）。
	f, err := os.Open(src.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	host := strings.TrimPrefix(tok.Endpoint, "https://")
	u := "https://" + init.Bucket + "." + host + "/" + init.Object
	date := quarkTime()
	resource := "/" + init.Bucket + "/" + init.Object
	sign := ossSignV1(tok.AccessKeySecret, "PUT", "", mimeOf(src.Path), date,
		"x-oss-callback:"+b64(init.Callback)+"\nx-oss-callback-var:"+b64(init.CallbackVr)+"\n"+
			"x-oss-date:"+date+"\nx-oss-security-token:"+tok.SecurityToken+"\n", resource)
	if _, err := streamHTTP(ctx, "PUT", u, f, src.Size, map[string]string{
		"Authorization":        "OSS " + tok.AccessKeyID + ":" + sign,
		"Content-Type":         mimeOf(src.Path),
		"x-oss-date":           date,
		"x-oss-security-token": tok.SecurityToken,
		"x-oss-callback":       b64(init.Callback),
		"x-oss-callback-var":   b64(init.CallbackVr),
		"User-Agent":           "aliyun-sdk-android/2.9.1",
	}, uploadHTTPTimeout); err != nil {
		return fmt.Errorf("115_open oss put: %w", err)
	}
	return nil
}

func initStatus(i *uploadInitOpen) any {
	if i == nil {
		return "nil"
	}
	return i.Status
}

// DownloadURL 115开放：POST /open/ufile/downurl（form pick_code；UA 绑定下载需同 UA）。
