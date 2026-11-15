// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 产物托管与自动挂载：把「发版人手动把网盘直链填进 update-manifest.json」
// 这一步彻底自动化。
//
// 背景：1.03.31 发版后全部客户端 targets.url 都是空串（macos 连对象都没有），
// Check 对 url/image 全空的 target 强制 hasUpdate=false —— 除 docker 外所有端
// 的更新提示静默死链，且 url 回填是纯人工步骤、没有任何门禁兜住。
//
// 新机制（文件名约定 + 扫描挂载）：
//  1. CI（upload-release-to-server.yml）把 Release 资产按规范名 scp 到服务器
//     <DataDir>/downloads/：<platform>-<version>.<ext>，如 android-1.03.31.apk；
//     可附带 release.json（releaseName/releaseNotes/minVersion）。
//  2. 服务器启动 + 每小时扫描该目录（ArtifactsHandler 也可手动触发/直接上传），
//     自动计算 sha256、按 platform 分组取最新版本，生成自托管 overlay 清单
//     （SetOverlay 落 <DataDir>/update-manifest.json）。
//  3. targets.url 写服务器相对路径 /api/v1/system/downloads/<file>——客户端
//     （update_url.dart）对以 / 开头的 url 拼 baseUrl 绝对化，网盘绝对直链继续兼容。
//
// latestVersion 取扫描结果中的最大版本：服务器/agent 与客户端产物同目录托管，
// 同一清单同时服务 docker 自检与各端客户端。

// artifactPlatforms 允许挂载的平台键（= targets 键 = Platform* 常量口径）。
// web 托管给「不用 docker、反代单独部署前端」的场景；docker 是镜像无法文件托管，
// 不在扫描范围（docker 自检走官方源或人工 overlay）。
var artifactPlatforms = map[string]bool{
	"android": true, "ios": true, "ohos": true, "windows": true, "macos": true,
	"linux": true, "web": true,
	"server-linux-amd64": true, "server-linux-arm64": true,
	"agent-linux-amd64": true, "agent-linux-arm64": true,
	"extension": true,
}

// artifactExts 允许托管的文件后缀（tar.gz 特殊：双段后缀）。
var artifactExts = []string{
	".apk", ".hap", ".ipa", ".zip", ".dmg", ".exe", ".tar.gz", ".gz",
}

// ArtifactMaxBytes 单产物上限 3GB（HAP/APK/桌面 zip 都远小于此；防误传大文件撑爆 data 卷）。
const ArtifactMaxBytes = int64(3 << 30)

// ParseArtifactName 解析规范名 <platform>-<version>.<ext>。
// platform 可含连字符（server-linux-amd64）而 version 不含——从右向左切。
// 返回空 platform 表示非法/不认识（静默跳过，不让一个杂散文件挂掉整次扫描）。
func ParseArtifactName(name string) (platform, version string) {
	lower := strings.ToLower(name)
	ext := ""
	for _, e := range artifactExts {
		if strings.HasSuffix(lower, e) {
			ext = e
			break
		}
	}
	if ext == "" {
		return "", ""
	}
	base := strings.TrimSuffix(lower, ext)
	// tar.gz 后缀剥离后可能以 '.' 结尾（x.tar.gz → x.tar）——统一去尾点。
	base = strings.TrimSuffix(base, ".")
	// CI 产物名常带仓库名前缀（ptpatronus-macos.zip）——容错剥离，双口径都认。
	base = strings.TrimPrefix(base, "ptpatronus-")
	i := strings.LastIndex(base, "-")
	if i <= 0 {
		return "", ""
	}
	platform, version = base[:i], base[i+1:]
	if !artifactPlatforms[platform] || version == "" {
		return "", ""
	}
	return platform, version
}

// artifactBest 某 platform 当前扫描到的最优产物。
type artifactBest struct {
	version string
	file    string
}

// sha256File 流式计算文件 sha256（产物最大 3GB，必须流式不能整体读内存）。
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// releaseMeta release.json 的可选字段（CI/发版人放在 downloads/ 一起托管）。
type releaseMeta struct {
	ReleaseName  string `json:"releaseName"`
	ReleaseNotes string `json:"releaseNotes"`
	MinVersion   string `json:"minVersion"`
	IsForced     bool   `json:"isForced"`
}

// ScanArtifacts 扫描托管目录生成自托管清单。
// dir 下形如 <platform>-<version>.<ext> 的文件按 platform 分组取最新版本；
// release.json（可选）提供 releaseName/releaseNotes/minVersion/isForced，
// 缺失时保留 prev（当前生效清单）里的人工字段，避免一次扫描抹掉发布说明。
// 目录不存在/没有可挂载产物 → 返回 (nil, nil)：调用方不动现有清单。
func ScanArtifacts(dir string, prev *Manifest) (*Manifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	// platform → 最优产物：只记版本最大的那个。
	best := map[string]artifactBest{}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		platform, version := ParseArtifactName(e.Name())
		if platform == "" {
			continue
		}
		cur, ok := best[platform]
		if ok && compareVersions(version, cur.version) <= 0 {
			continue // 已记录的版本不比当前扫描的新：跳过
		}
		best[platform] = artifactBest{version: version, file: e.Name()}
	}
	if len(best) == 0 {
		return nil, nil
	}

	m := &Manifest{Targets: map[string]Target{}}
	// latestVersion：所有 platform 最新版本中的最大者（服务端与客户端同清单，
	// 版本号口径一致——同一发版 tag）。
	for _, b := range best {
		if m.LatestVersion == "" || compareVersions(b.version, m.LatestVersion) > 0 {
			m.LatestVersion = b.version
		}
	}
	for platform, b := range best {
		sum, err := sha256File(filepath.Join(dir, b.file))
		if err != nil {
			return nil, fmt.Errorf("sha256 %s: %w", b.file, err)
		}
		m.Targets[platform] = Target{
			URL:    "/api/v1/system/downloads/" + b.file,
			SHA256: sum,
		}
	}

	// 人工字段：release.json 优先，其次保留现清单。
	var meta *releaseMeta
	if raw, err := os.ReadFile(filepath.Join(dir, "release.json")); err == nil {
		var r releaseMeta
		bad := json.Unmarshal(raw, &r) != nil
		if !bad && (r.ReleaseName != "" || r.ReleaseNotes != "") {
			meta = &r
		}
	}
	if meta != nil {
		m.ReleaseName = meta.ReleaseName
		m.ReleaseNotes = meta.ReleaseNotes
		m.MinVersion = meta.MinVersion
		m.IsForced = meta.IsForced
	} else if prev != nil {
		m.ReleaseName = prev.ReleaseName
		m.ReleaseNotes = prev.ReleaseNotes
		m.MinVersion = prev.MinVersion
		m.IsForced = prev.IsForced
	}
	return m, nil
}

// ListArtifacts 管理端展示用：目录下全部规范产物的 {platform, version, file, size}。
func ListArtifacts(dir string) ([]map[string]any, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, err
	}
	out := []map[string]any{}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		platform, version := ParseArtifactName(e.Name())
		if platform == "" {
			continue
		}
		info, err := e.Info()
		size := int64(0)
		if err == nil {
			size = info.Size()
		}
		out = append(out, map[string]any{
			"platform": platform,
			"version":  version,
			"file":     e.Name(),
			"size":     size,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["file"].(string) < out[j]["file"].(string)
	})
	return out, nil
}

// ApplyArtifacts 扫描托管目录并把结果落为自托管 overlay 清单（SetOverlay）。
// 目录不存在/无可挂载产物时不动现有清单——启动扫描不会抹掉人工 overlay；
// 扫描结果只覆盖 latestVersion 与 targets.url/sha256，releaseName/releaseNotes/
// minVersion/isForced 按「release.json 优先，否则保留现清单」合并。
func (s *Service) ApplyArtifacts(dir string) error {
	m, err := ScanArtifacts(dir, s.readOverlay())
	if err != nil {
		return err
	}
	if m == nil || m.LatestVersion == "" {
		return nil
	}
	return s.SetOverlay(m)
}
