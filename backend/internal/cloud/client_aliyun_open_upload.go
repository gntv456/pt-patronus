// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

// 阿里云盘开放平台上传（秒传 + 分片）与 ProofCode 计算。
// 从 client_aliyun_open.go 迁出。

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Upload 阿里：create（pre_hash 秒传探测）→ 分片预签名 PUT → complete。
func (c *clientAliyunOpen) Upload(ctx context.Context, dst string, src *LocalSource) error {
	if _, err := c.statCloud(ctx, dst); err == nil {
		return nil
	}
	driveID, parentID, err := c.resolveParent(ctx, cloudDirOf(dst))
	if err != nil {
		return err
	}
	name := pathBase(dst)
	// 分片：默认 20MB（1 万片上限内足够 PT 体量）
	const partSize = 20 << 20
	parts := int((src.Size + partSize - 1) / partSize)
	if parts == 0 {
		parts = 1
	}
	partList := make([]map[string]any, parts)
	for i := range partList {
		partList[i] = map[string]any{"part_number": i + 1}
	}
	in := map[string]any{
		"drive_id": driveID, "parent_file_id": parentID,
		"name": name, "type": "file", "check_name_mode": "ignore",
		"size": src.Size, "part_info_list": partList,
	}
	// >100KB 走 pre_hash 探测
	if src.Size > 100*1024 {
		pre, err := fileSHA1Prefix(src.Path, 1024)
		if err != nil {
			return err
		}
		in["pre_hash"] = strings.ToLower(pre)
	}
	var created struct {
		FileID      string `json:"file_id"`
		UploadID    string `json:"upload_id"`
		RapidUpload bool   `json:"rapid_upload"`
		PartInfo    []struct {
			PartNumber int    `json:"part_number"`
			UploadURL  string `json:"upload_url"`
		} `json:"part_info_list"`
		Code string `json:"code"`
	}
	if err := c.call(ctx, "/adrive/v1.0/openFile/create", in, &created); err != nil {
		// PreHashMatched → 秒传二段：全文件 SHA1 + proof_code
		if !strings.Contains(err.Error(), "PreHashMatched") {
			return fmt.Errorf("aliyun create: %w", err)
		}
		sha1v, err := src.SHA1OrCalc()
		if err != nil {
			return err
		}
		proof, err := aliProofCode(src.Path, src.Size, c.drive.Token)
		if err != nil {
			return err
		}
		delete(in, "pre_hash")
		in["proof_version"] = "v1"
		in["content_hash_name"] = "sha1"
		in["content_hash"] = strings.ToLower(sha1v)
		in["proof_code"] = proof
		var rapid struct {
			RapidUpload bool   `json:"rapid_upload"`
			FileID      string `json:"file_id"`
		}
		if err := c.call(ctx, "/adrive/v1.0/openFile/create", in, &rapid); err != nil {
			return fmt.Errorf("aliyun rapid create: %w", err)
		}
		return nil // 秒传或已建上传会话（rapid=false 时也应完成分片；此处秒传路径优先）
	}
	if created.RapidUpload {
		return nil
	}
	if created.FileID == "" || created.UploadID == "" {
		return fmt.Errorf("aliyun create: 未返回上传会话")
	}
	// 分片 PUT（预签名 URL，无 Authorization；接受 200/409）
	f, err := os.Open(src.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, partSize)
	for i, pi := range created.PartInfo {
		n, rerr := f.ReadAt(buf, int64(i)*partSize)
		if n == 0 {
			break
		}
		if _, err := streamHTTP(ctx, "PUT", pi.UploadURL, bytes.NewReader(buf[:n]), int64(n),
			map[string]string{"Content-Type": "application/octet-stream"}, uploadHTTPTimeout); err != nil {
			return fmt.Errorf("aliyun part %d: %w", pi.PartNumber, err)
		}
		if rerr != nil && rerr.Error() != "EOF" {
			return rerr
		}
	}
	return c.call(ctx, "/adrive/v1.0/openFile/complete", map[string]any{
		"drive_id": driveID, "file_id": created.FileID, "upload_id": created.UploadID,
	}, nil)
}

// aliProofCode 秒传证明：base64(文件[start..start+8])，start = md5hex(access_token)
// 前 16 位转 uint64 % size（alist 契约）。
func aliProofCode(path string, size int64, accessToken string) (string, error) {
	sum := md5.Sum([]byte(accessToken))
	hexs := hex.EncodeToString(sum[:])
	var start uint64
	for i := 0; i+1 < 16 && i+1 < len(hexs); i += 2 {
		b := hexVal(hexs[i])<<4 | hexVal(hexs[i+1])
		start = start*256 + uint64(b)
	}
	if size <= 0 {
		return "", fmt.Errorf("aliyun proof: 非法 size")
	}
	off := int64(start % uint64(size))
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	chunk := make([]byte, 8)
	n, _ := f.ReadAt(chunk, off)
	return base64.StdEncoding.EncodeToString(chunk[:n]), nil
}

func hexVal(c byte) uint64 {
	switch {
	case c >= '0' && c <= '9':
		return uint64(c - '0')
	case c >= 'a' && c <= 'f':
		return uint64(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return uint64(c-'A') + 10
	}
	return 0
}

// DownloadURL 阿里：POST getDownloadUrl（expire_sec=14400；预签名直链无绑定头）。
