// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"syscall"
	"testing"
)

// TestDockerEngineUnavailableClassification 「引擎不在」必须与「引擎答错了」分家：
// 前者回 503 + 「先启动 Docker」，后者才回 502「请稍后重试」。
// 从前两者共用 502，Docker Desktop 没启动时（NAS/Windows 的日常状态）面板一直显示
// 一句永远不会实现的「稍后重试」，用户既不知道该启动 Docker，也不会想到去启动。
func TestDockerEngineUnavailableClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"管道文件不存在（Windows 命名管道）", fmt.Errorf("dial unix \\\\.\\pipe\\docker_engine: connect: The system cannot find the file specified."), true},
		{"Linux 套接字不存在", errors.New("dial unix /var/run/docker.sock: connect: no such file or directory"), true},
		{"连接被拒（daemon 没监听）", syscall.ECONNREFUSED, true},
		{"裸 fs.ErrNotExist", fs.ErrNotExist, true},
		{"明确说 daemon 没跑", errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock"), true},
		// 2026-10-13 真机冒烟（Windows 原生运行时）实际拨出的那一句，原样进用例：
		// 当时它落进 502「上游服务不可用，请稍后重试」，用户按提示等了一轮又一轮。
		{"Windows 拨 unix 套接字（实测原文）", errors.New(`Get "http://docker/containers/json?all=true": dial unix /var/run/docker.sock: connect: A socket operation encountered a dead network.`), true},
		{"Windows 拨命名管道被拒（实测原文形态）", errors.New("dial unix \\\\.\\pipe\\docker_engine: connect: No connection could be made because the target machine actively refused it."), true},
		// 反向对照：引擎答错了不能算「没在运行」，否则会把「去启动 Docker」误导给已经跑着的引擎。
		{"引擎回 404", errors.New("docker api: status 404: no such image: nginx:latest"), false},
		{"引擎回 500", errors.New("docker api: status 500: server error"), false},
		// 分类清单只用整句特征：裸 "not supported" 会把这条引擎的真实答复也读成「引擎不在」。
		{"引擎回 400 说不支持该操作", errors.New("docker API /containers/xxx/restart: 400 Bad Request: operation is not supported"), false},
		{"上下文取消（客户端断开）", context.Canceled, false},
		{"空错误", nil, false},
	}
	for _, tc := range cases {
		if got := dockerEngineUnavailable(tc.err); got != tc.want {
			t.Errorf("%s: dockerEngineUnavailable(%v)=%v want %v", tc.name, tc.err, got, tc.want)
		}
	}
}
