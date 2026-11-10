// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ---- 本地文件摘要 ----

// fileMD5 计算文件 MD5（小写十六进制）。123 上传 create 的 etag 需要。
func fileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fileSHA1 计算文件 SHA1（大写十六进制）。115 秒传 fileid 需要（社区契约为大写）。
func fileSHA1(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil))), nil
}

// ErrUploadUnsupported 驱动不支持该操作（如 115 直链解析依赖加密契约，未内置实现）。
var ErrUploadUnsupported = errors.New("此网盘暂不支持该操作")

// uploadHTTPTimeout 流式上传/分片 PUT 的单请求超时（大文件低带宽下需宽裕）。
const uploadHTTPTimeout = 30 * time.Minute

// ---- 流式 HTTP（httpDoer 只收 []byte body；分片/整文件直传需 io.Reader） ----

// streamHTTP 用独立 http.Client 直传（预签名 URL / OSS 域名由网盘 API 下发，公网白名单
// 目标，无 SSRF 顾虑）。status<300 返回响应（Header 可取 Etag）；≥300 读 512B 错误体并返错。
func streamHTTP(ctx context.Context, method, url string, body io.Reader, size int64,
	headers map[string]string, timeout time.Duration) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if size > 0 {
		req.ContentLength = size
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	hc := &http.Client{Timeout: timeout}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return resp, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	return resp, nil
}

// ---- 路径小工具 ----

// pathBase 云盘路径最后一段（文件名/目录名）。
func pathBase(p string) string {
	segs := splitSegments(p)
	if len(segs) == 0 {
		return ""
	}
	return segs[len(segs)-1]
}

// cloudDir 云盘路径的父目录（根则为 "/"）。
func cloudDir(p string) string {
	segs := splitSegments(p)
	if len(segs) <= 1 {
		return "/"
	}
	return "/" + strings.Join(segs[:len(segs)-1], "/")
}

// nowUnixMilli 当前毫秒时间戳（网盘契约普遍要 t=毫秒）。
func nowUnixMilli() int64 { return time.Now().UnixMilli() }

// ---- OSS 签名原语（115 PutObject 直传 / 复用 OSS V1 签名规则） ----

// ossSignV1 OSS V1 Header 签名：base64(hmac-sha1(secret, VERB\nMD5\nCT\nDate\nCanonOSSHeaders+CanonResource))。
func ossSignV1(secret, verb, contentMD5, contentType, date, canonOSSHeaders, canonResource string) string {
	s := verb + "\n" + contentMD5 + "\n" + contentType + "\n" + date + "\n" + canonOSSHeaders + canonResource
	h := sha1HMAC(secret, s)
	return base64.StdEncoding.EncodeToString([]byte(h))
}

// sha1HMAC HMAC-SHA1 十六进制输出。
func sha1HMAC(key, data string) string {
	h := sha1.New()
	h.Write([]byte(key))
	m := func() interface{ Write([]byte) (int, error) } { return h }
	_, _ = m().Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// b64 base64 标准编码（callback 头用）。
func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// sha256Hex SHA-256 十六进制（保留给后续夸克 hash 字段如需）。

// mimeOf 按扩展名给上传 Content-Type（网盘契约普遍要求显式 MIME）。
func mimeOf(name string) string {
	switch lowerExt(name) {
	case ".mp4":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	case ".flv":
		return "video/x-flv"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".webm":
		return "video/webm"
	case ".ts", ".m2ts":
		return "video/mp2t"
	case ".iso":
		return "application/x-iso9660-image"
	case ".mp3":
		return "audio/mpeg"
	case ".flac":
		return "audio/flac"
	case ".srt", ".ass", ".ssa":
		return "text/plain"
	case ".nfo":
		return "application/xml"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

// lowerExt 小写扩展名（含点）。
func lowerExt(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	return strings.ToLower(name[i:])
}

// fileSHA1Prefix 文件前 n 字节 SHA1（大写十六进制）。115 开放平台 preid。
func fileSHA1Prefix(path string, n int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, io.LimitReader(f, n)); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil))), nil
}

// fileSHA1Range "start-end"（含端点，字节偏移）区间 SHA1（大写）。115 二次校验 sign_val。
func fileSHA1Range(path, rng string) (string, error) {
	parts := strings.SplitN(rng, "-", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("非法区间 %q", rng)
	}
	start, err1 := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	end, err2 := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err1 != nil || err2 != nil || start < 0 || end < start {
		return "", fmt.Errorf("非法区间 %q", rng)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	h := sha1.New()
	if _, err := io.Copy(h, io.LimitReader(f, end-start+1)); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil))), nil
}
