// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package update 实现在线更新检查：拉取官方版本清单(manifest)，与当前版本比较，
// 供服务端自检(Web UI 提示 docker pull 升级)与客户端中转(/system/update/check)。
//
// 检查/执行分离，并补 sha256 校验。
package update

import "time"

// Manifest 是一份多平台版本清单。由分发者维护在 UPDATE_MANIFEST_URL(如一个 gist
// raw 链接)，所有用户部署的后端拉取这一份，再经 /system/update/check 中转给各自的
// 客户端——客户端不直连官方源(避墙、不泄露官方地址)。管理员也可通过后台写一份
// 自托管清单(data/update-manifest.json)覆盖官方源。
type Manifest struct {
	LatestVersion string            `json:"latestVersion" binding:"max=32"`             // 最新版本号，如 0.70.01
	MinVersion    string            `json:"minVersion,omitempty" binding:"max=32"`      // 低于此版本强制更新(真·强制阻断)
	ReleaseName   string            `json:"releaseName,omitempty" binding:"max=128"`    // 版本标题
	ReleaseNotes  string            `json:"releaseNotes,omitempty" binding:"max=10000"` // 更新日志(markdown)
	Signature     string            `json:"signature,omitempty" binding:"max=256"`      // ed25519 签名（base64）；验证 manifest 真实性
	IsForced      bool              `json:"isForced,omitempty"`                         // 强制更新标志(软强制：提示不可忽略)
	PublishedAt   time.Time         `json:"publishedAt,omitempty"`
	Targets       map[string]Target `json:"targets" binding:"dive"` // key 见 Platform 常量
}

// Target 是某平台/形态的更新产物描述。不同形态用不同字段：
// docker → Image + Instructions(提示 docker compose pull，不在容器内自动替换)；
// android/windows/agent → URL + SHA256(外链下载，下载后校验)。
type Target struct {
	URL          string `json:"url,omitempty" binding:"max=512"`          // 下载地址(网盘/直链)
	SHA256       string `json:"sha256,omitempty" binding:"max=64"`        // 完整性校验；下载后比对，防中间人替换
	Image        string `json:"image,omitempty" binding:"max=256"`        // docker 镜像 tag(如 ghcr.io/x/ptpatronus:0.70.01)
	Instructions string `json:"instructions,omitempty" binding:"max=512"` // 更新命令(如 docker compose pull && docker compose up -d)
}

// Targets map 的 key 常量(对应各部署形态)。
const (
	PlatformDocker          = "docker"            // 服务端自检：Web UI 提示 docker pull 升级命令
	PlatformAndroid         = "android"           // 手机 APP：外链下载 APK
	PlatformIOS             = "ios"               // iPhone/iPad：外链下载页(实际安装走 AltStore/TestFlight)
	PlatformMacOS           = "macos"             // macOS 桌面：外链下载 zip，手动替换 .app
	PlatformOHOS            = "ohos"              // 鸿蒙：外链下载 HAP 页
	PlatformWindows         = "windows"           // Windows 桌面：外链下载 zip/exe
	PlatformAgentLinuxAmd64 = "agent-linux-amd64" // 监控 Agent
	PlatformAgentLinuxArm64 = "agent-linux-arm64"
)
