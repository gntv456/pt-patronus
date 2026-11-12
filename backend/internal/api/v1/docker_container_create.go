// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"encoding/json"
	"fmt"
)

// LocalCreateContainer 创建并可选启动容器（Docker Engine API /containers/create）。
func (c *DockerLocalClient) LocalCreateContainer(ctx context.Context, req map[string]any) (string, error) {
	// body 结构：{"Image":"nginx","Name":"myweb","Env":[],"PortBindings":{},"Cmd":[],"Binds":[]...}
	// 直接透传给 Engine API，由前端组装标准 Docker API 格式。
	// 容器名只能走 ?name= query：Engine API 会忽略 body 里的 Name 字段，
	// 不传则 daemon 生成随机名，前端的「名称」输入会被静默丢弃。
	path := "/containers/create"
	if name, ok := req["Name"].(string); ok && name != "" {
		path += "?name=" + urlEncode(name)
	}
	raw, err := c.postRaw(ctx, path, req)
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("parse create response: %w", err)
	}
	return result.ID, nil
}

// LocalCreateFromSimple 接收简化的前端表单，组装成 Engine API 的完整请求体。
// 创建容器表单：镜像、名称、端口映射、环境变量、挂载、命令。
func (c *DockerLocalClient) LocalCreateFromSimple(ctx context.Context, in simpleCreateReq) (string, error) {
	body := map[string]any{
		"Image": in.Image,
	}
	if in.Name != "" {
		body["Name"] = in.Name // LocalCreateContainer 会把它转成 ?name= query（body Name 被 API 忽略）
		body["Hostname"] = in.Name
	}
	if len(in.Cmd) > 0 {
		body["Cmd"] = in.Cmd
	}
	if len(in.Env) > 0 {
		env := make([]string, 0, len(in.Env))
		for k, v := range in.Env {
			env = append(env, k+"="+v)
		}
		body["Env"] = env
	}

	// 端口绑定：{端口/协议: [{HostPort:"8080"}]}
	exposedPorts := map[string]struct{}{}
	portBindings := map[string][]map[string]any{}
	for _, p := range in.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		containerPort := fmt.Sprintf("%d/%s", p.Container, proto)
		exposedPorts[containerPort] = struct{}{}
		binding := map[string]any{}
		if p.Host > 0 {
			binding["HostPort"] = fmt.Sprintf("%d", p.Host)
		}
		portBindings[containerPort] = append(portBindings[containerPort], binding)
	}

	// HostConfig 组装（端口绑定 / 重启策略 / 资源限制 / 特权 / 网络模式）
	hc := map[string]any{
		"RestartPolicy": map[string]any{
			"Name": in.RestartPolicy,
		},
	}
	if len(exposedPorts) > 0 {
		body["ExposedPorts"] = exposedPorts
		hc["PortBindings"] = portBindings
	}
	if len(in.Binds) > 0 {
		hc["Binds"] = in.Binds
	}
	if in.Privileged {
		hc["Privileged"] = true
	}
	if in.NetworkMode != "" {
		hc["NetworkMode"] = in.NetworkMode
	}
	// CPU 限制：NanoCPUs = 核数 × 1e9
	if in.CPULimit > 0 {
		hc["NanoCpus"] = int64(in.CPULimit * 1e9)
	}
	// 内存限制：字节
	if in.MemoryLimitMB > 0 {
		hc["Memory"] = in.MemoryLimitMB * 1024 * 1024
	}
	body["HostConfig"] = hc

	return c.LocalCreateContainer(ctx, body)
}

// simpleCreateReq 创建容器的简化前端表单。
type simpleCreateReq struct {
	Image         string            `json:"image" binding:"required"`
	Name          string            `json:"name"`
	Cmd           []string          `json:"cmd"`
	Env           map[string]string `json:"env"`
	Ports         []portMapping     `json:"ports"`
	Binds         []string          `json:"binds"`
	RestartPolicy string            `json:"restart_policy"` // no|always|unless-stopped|on-failure
	AutoStart     bool              `json:"auto_start"`
	CPULimit      float64           `json:"cpu_limit"`       // CPU 核数限制，如 1.5 表示 1.5 核；0 为不限制
	MemoryLimitMB int64             `json:"memory_limit_mb"` // 内存上限 MB；0 为不限制
	Privileged    bool              `json:"privileged"`      // 特权模式
	NetworkMode   string            `json:"network_mode"`    // bridge|host|none|container:<name>|自定义网络名
}

// portMapping 端口映射项。
type portMapping struct {
	Container int    `json:"container"`
	Host      int    `json:"host"`
	Protocol  string `json:"protocol"` // tcp|udp，缺省 tcp
}

// LocalExecCheck 检查 exec 是否可用（Docker socket 可连即支持）。
func (c *DockerLocalClient) LocalExecCheck(ctx context.Context) bool {
	return c.Available()
}
