// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// dockerListContainer 单个容器的精简视图（docker ps --format '{{json .}}' 对齐）。
type dockerListContainer struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	Status string `json:"Status"`
	Ports  string `json:"Ports"`
	State  string `json:"State"`
}

// dockerInspectContainer docker inspect 的精简视图。
type dockerInspectContainer struct {
	ID      string                 `json:"Id"`
	Name    string                 `json:"Name"`
	State   map[string]any         `json:"State"`
	Config  map[string]any         `json:"Config"`
	Network map[string]any         `json:"NetworkSettings"`
	Mounts  []map[string]any       `json:"Mounts"`
	Created string                 `json:"Created"`
	Raw     map[string]interface{} `json:"-"`
}

// dockerImage docker images --format '{{json .}}' 对齐。
type dockerImage struct {
	Repository string `json:"Repository"`
	Tag        string `json:"Tag"`
	ID         string `json:"ID"`
	CreatedAt  string `json:"CreatedAt"`
	Size       string `json:"Size"`
}

// dockerVolume docker volume ls --format '{{json .}}' 对齐。
type dockerVolume struct {
	Driver string `json:"Driver"`
	Name   string `json:"Name"`
}

// dockerNetwork docker network ls --format '{{json .}}' 对齐。
type dockerNetwork struct {
	ID     string `json:"ID"`
	Name   string `json:"Name"`
	Driver string `json:"Driver"`
	Scope  string `json:"Scope"`
}

// dockerStatsRow docker stats --no-stream --format '{{json .}}' 对齐。
type dockerStatsRow struct {
	Name      string `json:"Name"`
	CPUPerc   string `json:"CPUPerc"`
	MemUsage  string `json:"MemUsage"`
	MemPerc   string `json:"MemPerc"`
	NetIO     string `json:"NetIO"`
	BlockIO   string `json:"BlockIO"`
	PIDs      string `json:"PIDs"`
	Container string `json:"Container"`
}

// dockerListResp 聚合列表响应。
type dockerListResp struct {
	Containers []dockerListContainer `json:"containers"`
	Images     []dockerImage         `json:"images"`
	Volumes    []dockerVolume        `json:"volumes"`
	Networks   []dockerNetwork       `json:"networks"`
}

// dockerActionRequest 容器操作请求体。
type dockerActionRequest struct {
	ConfirmRisk bool `json:"confirm_risk"`
}

// composeRequest Compose 部署请求体。
type composeRequest struct {
	Path        string `json:"path" binding:"required"`
	ConfirmRisk bool   `json:"confirm_risk"`
}
